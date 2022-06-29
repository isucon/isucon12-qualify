package bench

import (
	"context"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
)

type newTenantScenarioWorker struct {
	worker *worker.Worker
}

func (newTenantScenarioWorker) String() string {
	return "NewTenantScenarioWorker"
}
func (w *newTenantScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

func (sc *Scenario) NewTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*newTenantScenarioWorker, error) {
	scTag := ScenarioTagOrganizerNewTenant
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.NewTenantScenario(ctx, step); err != nil {
			sc.ScenarioError(scTag, err)
			time.Sleep(SleepOnError)
		}
	},
		worker.WithInfinityLoop(),
		worker.WithUnlimitedParallelism(),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)

	return &newTenantScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) NewTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("新規テナントシナリオ")
	defer report()
	scTag := ScenarioTagOrganizerNewTenant
	sc.ScenarioStart(scTag)

	_, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	tenant := data.CreateTenant(false)
	{
		res, err := PostAdminTenantsAddAction(ctx, tenant.Name, tenant.DisplayName, adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTAdminTenantsAdd, scTag)
		} else {
			return v
		}
	}

	_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.Name, "organizer")
	if err != nil {
		return err
	}

	// player作成
	// 参加者登録 addPlayerNum
	addPlayerNum := 100
	players := make(map[string]*PlayerData, addPlayerNum)
	playerDisplayNames := make([]string, addPlayerNum)
	for i := 0; i < addPlayerNum; i++ {
		playerDisplayNames = append(playerDisplayNames, data.RandomString(16))
	}

	{
		AdminLogger.Printf("[%s] [tenant:%s] Playerを追加します players: %d", scTag, tenant.Name, addPlayerNum)
		res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				for _, pl := range r.Data.Players {
					players[pl.DisplayName] = &PlayerData{
						ID:          pl.ID,
						DisplayName: pl.DisplayName,
					}
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayersAdd, scTag)
		} else {
			return v
		}
	}

	// プレイヤーのworker
	{
		// TODO: 要調整 10人くらいで試してみる
		i := 0
		for _, player := range players {
			if 10 < i {
				break
			}
			i++
			wkr, err := sc.PlayerScenarioWorker(step, 1, tenant.Name, player.ID)
			if err != nil {
				return err
			}
			sc.WorkerCh <- wkr
		}
	}

	orgJobConf := &OrganizerJobConfig{
		orgAg:       orgAg,
		scTag:       scTag,
		tenantName:  tenant.Name,
		scoreRepeat: 2,
	}

	// 大会を開催し、ダッシュボードを受け取ったら再び大会を開催する
	for {
		if err := sc.OrganizerJob(ctx, step, orgJobConf); err != nil {
			return err
		}

		// テナント請求ダッシュボードの閲覧
		{
			res, err := GetOrganizerBillingAction(ctx, orgAg)
			v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPIBilling) error {
					_ = r
					return nil
				}))

			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
			} else {
				return v
			}
		}
		orgJobConf.scoreRepeat++
	}

	return nil
}
