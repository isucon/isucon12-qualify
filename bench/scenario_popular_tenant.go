package bench

import (
	"context"
	"fmt"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
)

type popularTenantScenarioWorker struct {
	worker *worker.Worker
}

func (popularTenantScenarioWorker) String() string {
	return "PopularTenantScenarioWorker"
}
func (w *popularTenantScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

func (sc *Scenario) PopularTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32, isHeavyTenant bool) (Worker, error) {
	scTag := ScenarioTagOrganizerPopularTenant
	if isHeavyTenant {
		scTag = scTag + "HeavyTenant"
	}

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.PopularTenantScenario(ctx, step, scTag, isHeavyTenant); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
		}
	},
		// // 無限回繰り返す
		worker.WithInfinityLoop(),
		worker.WithMaxParallelism(20),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return &popularTenantScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) PopularTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag, isHeavyTenant bool) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	var tenantName string
	if isHeavyTenant {
		tenantName = sc.InitialDataTenant[0].TenantName
	} else {
		// 初期データからテナントを選ぶ
		index := randomRange(ConstPopularTenantScenarioIDRange)
		tenantName = sc.InitialDataTenant[index].TenantName
	}
	AdminLogger.Println(scTag, tenantName)

	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenantName, "organizer")
	if err != nil {
		return err
	}

	// 大会を開催し、ダッシュボードを受け取ったら再び大会を開催する
	orgJobConf := &OrganizerJobConfig{
		// 入稿サイズ: scoreRepeat * maxScoredPlayer
		orgAc:           orgAc,
		scTag:           scTag,
		tenantName:      tenantName,
		scoreRepeat:     1,
		addScoreNum:     100,
		scoreInterval:   500, // 結果の検証時には3s、負荷かける用は1s
		playerWorkerNum: 5,   // CSV入稿と同時に立つworker数
		maxScoredPlayer: 300,
	}

	for {
		orgJobConf.newPlayerWorkerNum = 5
		if _, err := sc.OrganizerJob(ctx, step, orgJobConf); err != nil {
			return err
		}

		// テナント請求ダッシュボードの閲覧
		{
			res, err, txt := GetOrganizerBillingAction(ctx, orgAg)
			msg := fmt.Sprintf("%s %s", orgAc, txt)
			v := ValidateResponseWithMsg("テナント内の請求情報", step, res, err, msg, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPIBilling) error {
					_ = r
					return nil
				}))

			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
			} else if v.Canceled {
				return nil
			} else {
				sc.AddErrorCount()
				return v
			}
		}

		// player数を増やし、スコアを大きくする
		addPlayerNum := randomRange([]int{80, 120})
		playerDisplayNames := make([]string, addPlayerNum)
		for i := 0; i < addPlayerNum; i++ {
			playerDisplayNames = append(playerDisplayNames, data.RandomString(16))
		}

		{
			res, err, txt := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
			msg := fmt.Sprintf("%s %s", orgAc, txt)
			v := ValidateResponseWithMsg("大会参加者追加", step, res, err, msg, WithStatusCode(200))
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayersAdd, scTag)
			} else if v.Canceled {
				return nil
			} else {
				sc.AddCriticalCount()
				return v
			}
		}
		if orgJobConf.maxScoredPlayer <= 1000 {
			orgJobConf.maxScoredPlayer += 200
		}
		if 1000 < orgJobConf.maxScoredPlayer {
			orgJobConf.maxScoredPlayer = 1000
		}
	}

	return nil
}
