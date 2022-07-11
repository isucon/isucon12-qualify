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
	"github.com/k0kubun/pp/v3"
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
	ErrFailedBench    failure.StringCode = "fail"
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
	WorkerCountMap     map[string]int
	ScenarioCountMutex sync.Mutex
	WorkerCountMutex   sync.Mutex

	InitialData        InitialDataRows
	InitialDataTenant  InitialDataTenantMap
	DisqualifiedPlayer map[string]struct{}
	RawKey             *rsa.PrivateKey

	WorkerCh        chan Worker
	ErrorCh         chan struct{}
	CriticalErrorCh chan struct{}
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Prepare ステップで実行される
func (sc *Scenario) Prepare(ctx context.Context, step *isucandar.BenchmarkStep) error {
	// Prepareは60秒以内に完了
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	sc.DisqualifiedPlayer = map[string]struct{}{}
	sc.ScenarioCountMutex = sync.Mutex{}

	sc.WorkerCountMap = make(map[string]int)
	sc.WorkerCountMutex = sync.Mutex{}

	sc.ScenarioScoreMap = sync.Map{}
	sc.ScenarioCountMap = make(map[ScenarioTag][]int)
	for _, key := range ScenarioTagList {
		n := int64(0)
		sc.ScenarioScoreMap.Store(string(key), &n)
		sc.ScenarioCountMap[key] = []int{0, 0}
	}

	sc.WorkerCh = make(chan Worker, 10)
	sc.CriticalErrorCh = make(chan struct{}, 10)
	sc.ErrorCh = make(chan struct{}, 10)

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
		sc.InitialDataTenant, err = GetInitialDataTenant()
		if err != nil {
			return fmt.Errorf("初期データ(テナント)のロードに失敗しました %s", err)
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
	ContestantLogger.Println("負荷走行を開始します")
	defer AdminLogger.Println("負荷走行を終了しました")
	wg := &sync.WaitGroup{}

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

	// 重いテナント(id=1)を見るworker
	{
		wkr, err := sc.PopularTenantScenarioWorker(step, 1, true)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// 軽いテナント(id!=1)を見るworker
	// TODO: 現状増やすきっかけが無いので初期から並列数多くてもよいかも
	{
		wkr, err := sc.PopularTenantScenarioWorker(step, 1, false)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// 破壊的な変更を許容するシナリオ
	{
		wkr, err := sc.PeacefulTenantScenarioWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// Tenant Billingの整合性をチェックするシナリオ
	{
		wkr, err := sc.TenantBillingValidateWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// Admin Billingの整合性をチェックするシナリオ
	{
		wkr, err := sc.AdminBillingValidateWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// PlayerHandlerの整合性をチェックするシナリオ
	{
		wkr, err := sc.PlayerValidateScenarioWorker(step, 1)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	errorCount := 0
	criticalCount := 0
	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	ticker := time.NewTicker(time.Millisecond * 100)
	// 	defer ticker.Stop()
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case <-ticker.C:
	// 		}
	// 		sc.AddErrorCount()
	// 		AdminLogger.Println("debug error +1")
	// 	}
	// }()

	for {
		end := false
		select {
		case <-ctx.Done():
			end = true
		case w := <-sc.WorkerCh: // workerを起動する
			// debug: 一つのworkerのみを立ち上げる
			// if w.String() != "PlayerValidateScenarioWorker" {
			// 	continue
			// }
			wg.Add(1)
			sc.CountWorker(w.String())
			go func(w Worker) {
				defer wg.Done()
				wkr := w
				defer sc.CountdownWorker(ctx, wkr.String())
				wkr.Process(ctx)
			}(w)
		case <-sc.ErrorCh:
			errorCount++
		case <-sc.CriticalErrorCh:
			errorCount++
			criticalCount++
		}

		if ConstMaxError <= errorCount {
			ContestantLogger.Printf("エラーが%d件を越えたので負荷走行を打ち切ります", ConstMaxError)
			step.Result().Errors.Add(ErrFailedBench)
			end = true
		}

		if ConstMaxCriticalError <= criticalCount {
			ContestantLogger.Printf("Criticalなエラーが%d件を越えたので負荷走行を打ち切ります", ConstMaxCriticalError)
			step.Result().Errors.Add(ErrFailedBench)
			end = true
		}

		if end {
			ContestantLogger.Printf("負荷走行を終了します")
			break
		}
	}
	step.Cancel()
	wg.Wait()

	return nil
}

func (sc Scenario) CountWorker(name string) {
	sc.WorkerCountMutex.Lock()
	defer sc.WorkerCountMutex.Unlock()
	if _, ok := sc.WorkerCountMap[name]; !ok {
		sc.WorkerCountMap[name] = 0
	}
	sc.WorkerCountMap[name]++
	AdminLogger.Printf("workerを増やします [%s](%d)", name, sc.WorkerCountMap[name])
}

func (sc Scenario) CountdownWorker(ctx context.Context, name string) {
	// ctxが切られたら減算しない
	if ctx.Err() != nil {
		return
	}
	sc.WorkerCountMutex.Lock()
	defer sc.WorkerCountMutex.Unlock()
	sc.WorkerCountMap[name]--
}

func (sc *Scenario) PrintWorkerCount() {
	AdminLogger.Printf("WorkerCount: %s", pp.Sprint(sc.WorkerCountMap))
}
