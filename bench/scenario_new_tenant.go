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
		AdminLogger.Printf("Playerを追加します tenant: %s players: %d", tenant.Name, addPlayerNum)
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

	// 大会のランキングを参照するプレイヤーのworker
	for _, player := range players {
		wkr, err := sc.PlayerScenarioWorker(step, 1, tenant.Name, player.ID)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
	}

	orgJobConf := &OrganizerJobConfig{
		scTag:      scTag,
		tenantName: tenant.Name,
		players:    players,
	}
	for {
		if err := sc.OrganizerJob(ctx, step, orgAg, scTag, orgJobConf); err != nil {
			return err
		}
	}

	return nil
}
