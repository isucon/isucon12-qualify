package bench

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
	"github.com/isucon/isucandar/score"
	"github.com/isucon/isucandar/worker"
)

var (
	Debug     = false
	MaxErrors = 30
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
	ScorePOSTTenantsAdd    score.ScoreTag = "GET /admin/api/tenants/add"
	ScoreGETTenantsBilling score.ScoreTag = "GET /admin/api/tenants/billing"

	// for organizer endpoint
	// 参加者操作
	ScorePOSTCompetititorsAdd       score.ScoreTag = "POST /organizer/api/competitors/add"
	ScorePOSTCompetitorDisqualified score.ScoreTag = "POST /organizer/api/competitor/:competitior_id/disqualified"
	// 大会操作
	ScorePOSTCompetitionsAdd   score.ScoreTag = "POST /organizer/api/competitions/add"
	ScorePOSTCompetitionFinish score.ScoreTag = "POST /organizer/api/competition/:competition_id/finish"
	ScorePOSTCompetitionResult score.ScoreTag = "POST /organizer/api/competition/:competition_id/result"
	// テナント操作
	ScoreGETTenantBilling score.ScoreTag = "GET /organizer/api/billing"

	// for player
	// 参加者からの閲覧
	ScoreGETCompetitor         score.ScoreTag = "GET /api/competitor/:competitor_id"
	ScoreGETCompetitionRanking score.ScoreTag = "GET /api/competition/:competition_id/ranking"
	ScoreGETCometitions        score.ScoreTag = "GET /api/competitions"
)

type TenantData struct {
	DisplayName string
	Name        string
}

// オプションと全データを持つシナリオ構造体
type Scenario struct {
	mu sync.RWMutex

	Option Option

	// TODO: シナリオを回すのに必要な全データをしまう定義が列挙される
	// 必要になりそうなメモ
	// AdminUsers // SaaS管理者
	// Tenants // = 主催者一覧
	// +- Billing // 請求情報
	// +- Competitions // 大会一覧
	// | +- Comeptitors // 大会参加者一覧
	// | +- Disqualifieds // 失格済み参加者一覧
	// +- Organizer // 主催者(1 tenantに1人)
	// ひとまずTenantName, CompetitionID, PlayerNameが入ったデータ
	InitialData InitialDataRows

	lastPlaylistCreatedAt   time.Time
	rateGetPopularPlaylists int32

	Errors failure.Errors
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Prepare ステップで実行される
func (s *Scenario) Prepare(ctx context.Context, step *isucandar.BenchmarkStep) error {
	// Prepareは60秒以内に完了
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

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

	// POST /initialize へ初期化リクエスト実行
	res, err := PostInitializeAction(ctx, ag)
	if v := ValidateResponse("初期化", step, res, err, WithStatusCode(200)); !v.IsEmpty() {
		return fmt.Errorf("初期化リクエストに失敗しました %v", v)
	}

	// TODO: 初期データをロードする kayac/isucon2022/benchのLoad
	s.InitialData, err = GetInitialData()
	if err != nil {
		return fmt.Errorf("初期データのロードに失敗しました %s", err)
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

	// SaaS管理者ユーザーシナリオ
	AdminCase, err := s.AdminScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	// 主催者(テナントオーナー)シナリオ
	organizerCase, err := s.OrganizerScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	// 参加者シナリオ
	playerCase, err := s.PlayerScenarioWorker(step, 1)
	if err != nil {
		return err
	}

	workers := []*worker.Worker{
		AdminCase,
		organizerCase,
		playerCase,
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
		s.loadAdjustor(ctx, step, organizerCase, AdminCase)
	}()
	wg.Wait()
	return nil
}

// 並列数の調整
func (s *Scenario) loadAdjustor(ctx context.Context, step *isucandar.BenchmarkStep, workers ...*worker.Worker) {
	tk := time.NewTicker(time.Second * 10) // TODO: 適切な値にする
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
		addParallels := int32(1)
		if diff := total - prevErrors; diff > 0 {
			ContestantLogger.Printf("エラーが%d件増えました(現在%d件)", diff, total)
		} else {
			ContestantLogger.Println("並列数を1追加します")
			addParallels = 1
		}
		for _, w := range workers {
			w.AddParallelism(addParallels)
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
