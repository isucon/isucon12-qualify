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

	_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.Name, "organizer")
	if err != nil {
		return err
	}

	jobConf := &OrganizerJobConfig{
		tenantName:        tenant.Name,
		addPlayerTimes:    20,
		addPlayerNum:      5,
		rankingRequestNum: 10,
	}
	if sc.Option.LoadType == LoadTypeLight {
		jobConf.rankingRequestNum = 3
	}

	for {
		if err := sc.OrganizerJob(ctx, step, orgAg, scTag, jobConf); err != nil {
			return err
		}
	}

	return nil
}
