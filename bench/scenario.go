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
	"github.com/isucon/isucandar/score"
	"github.com/isucon/isucandar/worker"
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

// シナリオ分別用タグ
type ScenarioTag string

const (
	ScenarioTagAdmin                   ScenarioTag = "Admin"
	ScenarioTagOrganizerNewTenant      ScenarioTag = "OrganizerNewTenant"
	ScenarioTagOrganizerPopularTenant  ScenarioTag = "OrganizerPopularTenant"
	ScenarioTagOrganizerPeacefulTenant ScenarioTag = "OrganizerPeacefulTenant"
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

	// TODO: 要調整 初期から万単位がでるので*1/100みたいなのをしたい
	ScoreGETPlayerDetails:      1,
	ScoreGETPlayerRanking:      1,
	ScoreGETPlayerCompetitions: 1,
}

// 各tagのリスト
var (
	ScenarioTagList = []ScenarioTag{
		ScenarioTagAdmin,
		ScenarioTagOrganizerNewTenant,
		ScenarioTagOrganizerPopularTenant,
		ScenarioTagOrganizerPeacefulTenant,
	}
	ScoreTagList = []score.ScoreTag{
		ScorePOSTAdminTenantsAdd,
		ScoreGETAdminTenantsBilling,
		ScorePOSTOrganizerPlayersAdd,
		ScorePOSTOrganizerPlayerDisqualified,
		ScorePOSTOrganizerCompetitionsAdd,
		ScorePOSTOrganizerCompetitionFinish,
		ScorePOSTOrganizerCompetitionResult,
		ScoreGETOrganizerBilling,
		ScoreGETPlayerDetails,
		ScoreGETPlayerRanking,
		ScoreGETPlayerCompetitions,
	}
)

type TenantData struct {
	DisplayName string
	Name        string
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

	WorkerCh chan *worker.Worker // TODO: 何も考えていない
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Prepare ステップで実行される
func (sc *Scenario) Prepare(ctx context.Context, step *isucandar.BenchmarkStep) error {
	// Prepareは60秒以内に完了
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	sc.ScenarioScoreMap = sync.Map{}
	sc.ScenarioCountMap = make(map[ScenarioTag][]int)
	for _, key := range ScenarioTagList {
		n := int64(0)
		sc.ScenarioScoreMap.Store(string(key), &n)
		sc.ScenarioCountMap[key] = []int{0, 0}
	}

	sc.ScenarioCountMutex = sync.Mutex{}
	sc.DisqualifiedPlayer = map[string]struct{}{}

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

	sc.WorkerCh = make(chan *worker.Worker, 1)

	// 最初に起動するシナリオ
	{
		adminBillingWorker, err := sc.AdminBillingScenarioWorker(step, 1)
		if err != nil {
			return err
		}
		// select channel前なのでdeadlock対策でgoroutineに置いておく
		go func(w *worker.Worker) {
			sc.WorkerCh <- w
		}(adminBillingWorker)
	}

	// workerを起動する
	for {
		select {
		case <-ctx.Done():
			break
		case w := <-sc.WorkerCh:
			wg.Add(1)
			go func() {
				defer wg.Done()
				AdminLogger.Printf("なにかのworkerが発火しました(%+v)", w)
				w.Process(ctx)
			}()
			// TODO: エラー総数で打ち切りにする？専用のchannelで待ち受ける？
		}
	}
	wg.Wait()

	return nil
}
