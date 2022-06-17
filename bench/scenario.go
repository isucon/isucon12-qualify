package bench

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
	"github.com/isucon/isucandar/score"
	"github.com/isucon/isucandar/worker"
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
)

// シナリオで発生するスコアのタグ
const (
	ScoreGETRoot score.ScoreTag = "GET /"

	// for admin endpoint
	ScorePOSTAdminTenantsAdd    score.ScoreTag = "POST /admin/api/tenants/add"
	ScoreGETAdminTenantsBilling score.ScoreTag = "GET /admin/api/tenants/billing"

	// for organizer endpoint
	// 参加者操作
	ScorePOSTOrganizerPlayersAdd         score.ScoreTag = "POST /organizer/api/players/add"
	ScorePOSTOrganizerPlayerDisqualified score.ScoreTag = "POST /organizer/api/player/:player_name/disqualified"
	// 大会操作
	ScorePOSTOrganizerCompetitionsAdd   score.ScoreTag = "POST /organizer/api/competitions/add"
	ScorePOSTOrganizerCompetitionFinish score.ScoreTag = "POST /organizer/api/competition/:competition_id/finish"
	ScorePOSTOrganizerCompetitionResult score.ScoreTag = "POST /organizer/api/competition/:competition_id/result"
	// テナント操作
	ScoreGETOrganizerBilling score.ScoreTag = "GET /organizer/api/billing"

	// for player
	// 参加者からの閲覧
	ScoreGETPlayerDetails      score.ScoreTag = "GET /player/api/player/:player_name"
	ScoreGETPlayerRanking      score.ScoreTag = "GET /player/api/competition/:competition_id/ranking"
	ScoreGETPlayerCompetitions score.ScoreTag = "GET /player/api/competitions"
)

// ScoreTag毎の倍率
var ResultScoreMap = map[score.ScoreTag]int64{
	ScorePOSTAdminTenantsAdd:             1,
	ScoreGETAdminTenantsBilling:          1,
	ScorePOSTOrganizerPlayersAdd:         1,
	ScorePOSTOrganizerPlayerDisqualified: 1,
	ScorePOSTOrganizerCompetitionsAdd:    1,
	ScorePOSTOrganizerCompetitionFinish:  1,
	ScorePOSTOrganizerCompetitionResult:  1,
	ScoreGETOrganizerBilling:             1,

	// TODO: 要調整 Player類はどうしても爆速で回るので加点はなし
	// エラーが発生したら減点、MaxErrors数発生したらbench打ち切り
	ScoreGETPlayerDetails:      0,
	ScoreGETPlayerRanking:      0,
	ScoreGETPlayerCompetitions: 0,
}

type TenantData struct {
	DisplayName string
	Name        string
}

// オプションと全データを持つシナリオ構造体
type Scenario struct {
	Option Option
	Errors failure.Errors

	ScenarioScoreMap sync.Map // map[string]*int64

	InitialData        InitialDataRows
	DisqualifiedPlayer map[string]struct{}
	RawKey             *rsa.PrivateKey
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Prepare ステップで実行される
func (s *Scenario) Prepare(ctx context.Context, step *isucandar.BenchmarkStep) error {
	// Prepareは60秒以内に完了
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	s.ScenarioScoreMap = sync.Map{}
	s.DisqualifiedPlayer = map[string]struct{}{}

	// GET /initialize 用ユーザーエージェントの生成
	b, err := url.Parse(s.Option.TargetURL)
	if err != nil {
		return failure.NewError(ErrCannotNewAgent, err)
	}
	ag, err := s.Option.NewAgent(b.Scheme+"://admin."+b.Host, true)
	if err != nil {
		return failure.NewError(ErrCannotNewAgent, err)
	}

	if s.Option.SkipPrepare {
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
		s.InitialData, err = GetInitialData()
		if err != nil {
			return fmt.Errorf("初期データのロードに失敗しました %s", err)
		}

		block, _ := pem.Decode([]byte(keysrc))
		if block == nil {
			return fmt.Errorf("error pem.Decode: block is nil")
		}
		s.RawKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("error x509.ParsePKCS1PrivateKey: %w", err)
		}
		s.InitialData, err = GetInitialData()
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
	if err := s.ValidationScenario(ctx, step); err != nil {
		fmt.Println(err)
		return fmt.Errorf("整合性チェックに失敗しました")
	}

	ContestantLogger.Printf("整合性チェックに成功しました")
	return nil
}

// ベンチ本編
// isucandar.LoadScenario を満たすメソッド
// isucandar.Benchmark の Load ステップで実行される
func (s *Scenario) Load(ctx context.Context, step *isucandar.BenchmarkStep) error {
	if s.Option.PrepareOnly {
		return nil
	}
	ContestantLogger.Println("負荷テストを開始します")
	defer ContestantLogger.Println("負荷テストを終了します")
	wg := &sync.WaitGroup{}

	// 新規テナントシナリオ
	newTenantCase, err := s.NewTenantScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	// 初期データテナントシナリオ
	existingTenantCase, err := s.ExistingTenantScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	// 初期データプレイヤー整合性チェックシナリオ
	playerCase, err := s.PlayerScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	// admin billingを見るシナリオ
	adminBillingCase, err := s.AdminBillingScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	AdminLogger.Printf("%d workers", len([]*worker.Worker{
		newTenantCase,
		existingTenantCase,
		playerCase,
		adminBillingCase,
	}))

	workers := []*worker.Worker{
		newTenantCase,
		// playerCase,
		existingTenantCase,
		adminBillingCase,
	}
	for _, w := range workers {
		wg.Add(1)
		worker := w
		go func() {
			defer wg.Done()
			worker.Process(ctx)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.loadAdjustor(ctx, step,
			existingTenantCase,
		)
	}()
	wg.Wait()

	return nil
}

// 並列数の調整
func (s *Scenario) loadAdjustor(ctx context.Context, step *isucandar.BenchmarkStep, workers ...*worker.Worker) {
	tk := time.NewTicker(time.Second * 1) // TODO: 適切な値にする
	var prevErrors int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
		errors := step.Result().Errors.Count()
		total := errors["load"]
		if total >= int64(MaxErrors) {
			ContestantLogger.Printf("負荷テストを打ち切ります (エラー数:%d)", total)
			AdminLogger.Printf("%#v", errors)
			step.Result().Score.Close()
			step.Cancel()
			return
		}
		if diff := total - prevErrors; diff > 0 {
			ContestantLogger.Printf("エラーが%d件増えました(現在%d件)", diff, total)
		} else {
			ContestantLogger.Println("並列数を1追加します")
			for _, w := range workers {
				w.AddParallelism(1)
			}
		}
		prevErrors = total
	}
}

var nullFunc = func() {}

func timeReporter(name string) func() {
	if !Debug {
		return nullFunc
	}
	start := time.Now()
	return func() {
		AdminLogger.Printf("Scenario:%s elapsed:%s", name, time.Since(start))
	}
}

func getEnv(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

// どのシナリオから加算されたスコアかをカウントしならがスコアを追加する
type ScenarioTag string

func (sc *Scenario) AddScoreByScenario(step *isucandar.BenchmarkStep, scoreTag score.ScoreTag, scenarioTag ScenarioTag) {
	key := fmt.Sprintf("%s", scenarioTag)
	value, ok := sc.ScenarioScoreMap.Load(key)
	if ok {
		if ptr, ok := value.(*int64); ok {
			atomic.AddInt64(ptr, ResultScoreMap[scoreTag])
		} else {
			log.Printf("error failed ScenarioScoreMap.Load type assertion: key(%s)\n", key)
		}
	} else {
		n := ResultScoreMap[scoreTag]
		sc.ScenarioScoreMap.Store(key, &n)
	}
	step.AddScore(scoreTag)
}

// シナリオ毎のスコア表示
func (sc *Scenario) PrintScenarioScoreMap() {
	ssmap := map[string]int64{}
	sc.ScenarioScoreMap.Range(func(key, value any) bool {
		tag, okKey := key.(string)
		scorePtr, okVal := value.(*int64)
		if !okKey || !okVal {
			log.Printf("error failed ScenarioScoreMap.Load type assertion: key(%s)\n", key)
			return false
		}
		scoreVal := atomic.LoadInt64(scorePtr)
		ssmap[string(tag)] = scoreVal

		return true
	})
	AdminLogger.Println(pp.Sprint(ssmap))
}
