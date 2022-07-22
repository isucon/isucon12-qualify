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
	ErrFailedPrepare failure.StringCode = "fail-prepare"
	ErrFailedLoad    failure.StringCode = "fail-load"

	ErrNormalError   failure.StringCode = "error-normal"
	ErrCriticalError failure.StringCode = "error-critical"
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
	InitialDataTenant  InitialDataTenantRows
	DisqualifiedPlayer map[string]struct{}
	RawKey             *rsa.PrivateKey

	WorkerCh        chan Worker
	ErrorCh         chan struct{}
	CriticalErrorCh chan struct{}

	HeavyTenantCount int

	CompetitionAddLog *CompactLogger
	TenantAddLog      *CompactLogger
	PlayerAddCountMu  sync.Mutex
	PlayerAddCount    int
	PlayerDelCountMu  sync.Mutex
	PlayerDelCount    int

	addedWorkerCountMap map[string]int
	batchwg             sync.WaitGroup

	kickedWorkerPlayerIDMap map[string]struct{}
	kickedWorkerMu          sync.Mutex
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

	sc.HeavyTenantCount = 0

	batchwg := sync.WaitGroup{}
	sc.CompetitionAddLog = NewCompactLog(ContestantLogger, batchwg)
	sc.TenantAddLog = NewCompactLog(ContestantLogger, batchwg)
	sc.PlayerAddCountMu = sync.Mutex{}
	sc.PlayerAddCount = 0
	sc.PlayerDelCountMu = sync.Mutex{}
	sc.PlayerDelCount = 0
	sc.batchwg = batchwg
	sc.addedWorkerCountMap = make(map[string]int)

	sc.kickedWorkerPlayerIDMap = make(map[string]struct{})
	sc.kickedWorkerMu = sync.Mutex{}

	// GET /initialize 用ユーザーエージェントの生成
	b, err := url.Parse(sc.Option.TargetURL)
	if err != nil {
		return err
	}
	ag, err := sc.Option.NewAgent(b.Scheme+"://admin."+b.Host, true)
	if err != nil {
		return err
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
	var lang string
	res, err := PostInitializeAction(ctx, ag)
	if v := ValidateResponse("初期化", step, res, err, WithStatusCode(200),
		WithSuccessResponse(func(r ResponseAPIInitialize) error {
			if r.Data.Lang == "" {
				return fmt.Errorf("Initializeのレスポンスには実装言語`lang`を含めてください")
			}
			lang = r.Data.Lang
			return nil
		}),
	); !v.IsEmpty() {
		ContestantLogger.Printf("初期化リクエストに失敗しました")
		return failure.NewError(ErrFailedPrepare, v)
	}
	ContestantLogger.Printf("初期化リクエストに成功しました 実装言語:%s", lang)

	// 検証シナリオを1回まわす
	if err := sc.ValidationScenario(ctx, step); err != nil {
		ContestantLogger.Printf("整合性チェックに失敗しました")
		return failure.NewError(ErrFailedPrepare, err)
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

	// 軽いテナント(id!=1)を見るworker
	{
		wkr, err := sc.PopularTenantScenarioWorker(step, 1, false)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	// 初期から回る新規テナントシナリオ
	{
		wkr, err := sc.NewTenantScenarioWorker(step, nil, 1)
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

	logTicker := time.NewTicker(time.Second * 5) // 5秒置きに溜まったログを出力する
	defer logTicker.Stop()

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
			sc.CountWorker(w.String())
			wg.Add(1)
			go func(w Worker) {
				defer wg.Done()
				wkr := w
				defer sc.CountdownWorker(ctx, wkr.String())
				wkr.Process(ctx)
			}(w)

		case <-sc.ErrorCh:
			step.AddError(ErrNormalError)
			errorCount++
			AdminLogger.Println("Normal Error")

		case <-sc.CriticalErrorCh:
			step.AddError(ErrCriticalError)
			criticalCount++
			AdminLogger.Println("Critical Error")

		case <-logTicker.C:
			sc.CompetitionAddLog.Log()
			sc.TenantAddLog.Log()
			sc.workerAddLogPrint()
			sc.PlayerAddCountPrint()
			sc.PlayerDelCountPrint()
		}

		if 100 <= (errorCount*1)+(criticalCount*10) {
			ContestantLogger.Println("エラーによる減点が100%になったので負荷走行を打ち切ります")
			step.AddError(ErrFailedLoad)
			end = true
		}

		if end {
			ContestantLogger.Printf("負荷走行を終了します")
			break
		}
	}
	step.Cancel()
	wg.Wait()
	sc.batchwg.Wait()

	return nil
}

func (sc *Scenario) PlayerAddCountAdd(num int) {
	sc.batchwg.Add(1)
	go func(num int) {
		defer sc.batchwg.Done()
		sc.PlayerAddCountMu.Lock()
		defer sc.PlayerAddCountMu.Unlock()
		sc.PlayerAddCount += num
	}(num)
}

func (sc *Scenario) PlayerAddCountPrint() {
	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.PlayerAddCountMu.Lock()
		defer sc.PlayerAddCountMu.Unlock()
		if sc.PlayerAddCount != 0 {
			ContestantLogger.Printf("参加者が%d人増えました", sc.PlayerAddCount)
			sc.PlayerAddCount = 0
		}
	}()
}

func (sc *Scenario) PlayerDelCountAdd(num int) {
	sc.batchwg.Add(1)
	go func(num int) {
		defer sc.batchwg.Done()
		sc.PlayerDelCountMu.Lock()
		defer sc.PlayerDelCountMu.Unlock()
		sc.PlayerDelCount += num
	}(num)
}

func (sc *Scenario) PlayerDelCountPrint() {
	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.PlayerDelCountMu.Lock()
		defer sc.PlayerDelCountMu.Unlock()
		if sc.PlayerDelCount != 0 {
			ContestantLogger.Printf("leaderboardの表示に1秒以上かかったため%d人の参加者が離脱しました。", sc.PlayerDelCount)
			sc.PlayerDelCount = 0
		}
	}()
}

func (sc *Scenario) CountWorker(name string) {
	// lockするし急ぎではないので後回し
	sc.batchwg.Add(1)
	go func(name string) {
		defer sc.batchwg.Done()
		sc.WorkerCountMutex.Lock()
		defer sc.WorkerCountMutex.Unlock()
		if _, ok := sc.WorkerCountMap[name]; !ok {
			sc.WorkerCountMap[name] = 1
		} else {
			sc.WorkerCountMap[name]++
		}

		if _, ok := sc.addedWorkerCountMap[name]; !ok {
			sc.addedWorkerCountMap[name] = 1
		} else {
			sc.addedWorkerCountMap[name]++
		}
	}(name)
}

func (sc *Scenario) CountdownWorker(ctx context.Context, name string) {
	// ctxが切られたら減算しない
	if ctx.Err() != nil {
		return
	}
	sc.WorkerCountMutex.Lock()
	defer sc.WorkerCountMutex.Unlock()
	if _, ok := sc.WorkerCountMap[name]; ok {
		sc.WorkerCountMap[name]--
	}
}

func (sc *Scenario) PrintWorkerCount() {
	sc.WorkerCountMutex.Lock()
	defer sc.WorkerCountMutex.Unlock()
	AdminLogger.Printf("WorkerCount: %s", pp.Sprint(sc.WorkerCountMap))
}

// workerの追加ログ, worker種別毎に出す
func (sc *Scenario) workerAddLogPrint() {
	// lockするし急ぎではないので後回し
	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.WorkerCountMutex.Lock()
		defer sc.WorkerCountMutex.Unlock()

		for k, v := range sc.addedWorkerCountMap {
			AdminLogger.Printf("workerを追加しました [%s](num:%d)", k, v)
		}
		sc.addedWorkerCountMap = make(map[string]int)
	}()
}

// 同じplayerのPlayerWorkerを起動しないようにチェック
func (sc *Scenario) playerWorkerKick(tenantName, playerID string) {
	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.kickedWorkerMu.Lock()
		defer sc.kickedWorkerMu.Unlock()
		key := fmt.Sprintf("%s_%s", tenantName, playerID)
		if _, ok := sc.kickedWorkerPlayerIDMap[key]; !ok {
			sc.kickedWorkerPlayerIDMap[key] = struct{}{}
		}
	}()
}

func (sc *Scenario) checkPlayerWorkerKicked(tenantName, playerID string) bool {
	key := fmt.Sprintf("%s_%s", tenantName, playerID)
	sc.kickedWorkerMu.Lock()
	defer sc.kickedWorkerMu.Unlock()
	_, ok := sc.kickedWorkerPlayerIDMap[key]
	return ok
}
