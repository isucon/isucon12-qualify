package bench

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
)

var (
	Debug = false

	// これ以上エラーが出たら打ち切り
	MaxErrors = 30

	// エラーが発生したらこの時間だけSleepする(暴発防止)
	SleepOnError = time.Second
)

const (
	ErrFailedLoadJSON failure.StringCode = "load-json"
	ErrCannotNewAgent failure.StringCode = "agent"
	ErrInvalidRequest failure.StringCode = "request"
)

type TenantData struct {
	DisplayName string
	Name        string
}

// isucandar worker.Workerを実装する
type Worker interface {
	String() string
	Process(context.Context)
}

// オプションと全データを持つシナリオ構造体
type Scenario struct {
	Option Option
	Errors failure.Errors

	ScenarioScoreMap   sync.Map // map[string]*int64
	ScenarioCountMap   map[ScenarioTag][]int
	ScenarioCountMutex sync.Mutex

	InitialData        InitialDataRows
	DisqualifiedPlayer map[string]struct{}
	RawKey             *rsa.PrivateKey

	WorkerCh chan Worker
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Prepare ステップで実行される
func (sc *Scenario) Prepare(ctx context.Context, step *isucandar.BenchmarkStep) error {
	// Prepareは60秒以内に完了
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	sc.DisqualifiedPlayer = map[string]struct{}{}
	sc.ScenarioCountMutex = sync.Mutex{}

	sc.ScenarioScoreMap = sync.Map{}
	sc.ScenarioCountMap = make(map[ScenarioTag][]int)
	for _, key := range ScenarioTagList {
		n := int64(0)
		sc.ScenarioScoreMap.Store(string(key), &n)
		sc.ScenarioCountMap[key] = []int{0, 0}
	}

	// GET /initialize 用ユーザーエージェントの生成
	b, err := url.Parse(sc.Option.TargetURL)
	if err != nil {
		return failure.NewError(ErrCannotNewAgent, err)
	}
	ag, err := sc.Option.NewAgent(b.Scheme+"://admin."+b.Host, true)
	if err != nil {
		return failure.NewError(ErrCannotNewAgent, err)
	}

	if sc.Option.SkipPrepare {
		return nil
	}

	debug := Debug
	defer func() {
		Debug = debug
	}()
	Debug = true // prepareは常にデバッグログを出す

	// 各シナリオに必要なデータの用意
	{
		keyFilename := getEnv("ISUCON_JWT_KEY_FILE", "./isuports.pem")
		keysrc, err := os.ReadFile(keyFilename)
		if err != nil {
			return fmt.Errorf("error os.ReadFile: %w", err)
		}
		sc.InitialData, err = GetInitialData()
		if err != nil {
			return fmt.Errorf("初期データのロードに失敗しました %s", err)
		}

		block, _ := pem.Decode([]byte(keysrc))
		if block == nil {
			return fmt.Errorf("error pem.Decode: block is nil")
		}
		sc.RawKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("error x509.ParsePKCS1PrivateKey: %w", err)
		}
		sc.InitialData, err = GetInitialData()
		if err != nil {
			return fmt.Errorf("初期データのロードに失敗しました %s", err)
		}
	}

	// POST /initialize へ初期化リクエスト実行
	res, err := PostInitializeAction(ctx, ag)
	if v := ValidateResponse("初期化", step, res, err, WithStatusCode(200)); !v.IsEmpty() {
		return fmt.Errorf("初期化リクエストに失敗しました %v", v)
	}

	// 検証シナリオを1回まわす
	if err := sc.ValidationScenario(ctx, step); err != nil {
		fmt.Println(err)
		return fmt.Errorf("整合性チェックに失敗しました")
	}

	ContestantLogger.Printf("整合性チェックに成功しました")
	return nil
}

// ベンチ本編
// isucandar.LoadScenario を満たすメソッド
// isucandar.Benchmark の Load ステップで実行される
func (sc *Scenario) Load(ctx context.Context, step *isucandar.BenchmarkStep) error {
	if sc.Option.PrepareOnly {
		return nil
	}
	ContestantLogger.Println("負荷テストを開始します")
	defer ContestantLogger.Println("負荷テストを終了します")
	wg := &sync.WaitGroup{}

	sc.WorkerCh = make(chan Worker, 10)

	// 最初に起動するシナリオ
	// AdminBillingを見続けて新規テナントを追加する
	{
		wkr, err := sc.AdminBillingScenarioWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// // 最初から回る新規テナント
	{
		wkr, err := sc.NewTenantScenarioWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// 軽いテナント
	// TODO: deprecated
	{
		wkr, err := sc.ExistingTenantScenarioWorker(step, 1, false)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}
	// 重いテナント
	// TODO: deprecated
	{
		wkr, err := sc.ExistingTenantScenarioWorker(step, 1, true)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// プレイヤー
	// TODO: deprecated
	{
		wkr, err := sc.PlayerScenarioWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// workerを起動する
	for {
		select {
		case <-ctx.Done():
			break
		case w := <-sc.WorkerCh:
			wg.Add(1)
			go func(w Worker) {
				defer wg.Done()
				wkr := w
				AdminLogger.Printf("workerを増やします (%s)", wkr)
				wkr.Process(ctx)
			}(w)
			// TODO: エラー総数で打ち切りにする？専用のchannelで待ち受ける？5秒ごとにstep.Errorsを確認してもいいかも
		}
	}
	wg.Wait()

	return nil
}
