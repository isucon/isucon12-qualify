package bench

import (
	"context"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

type existingTenantScenarioWorker struct {
	worker *worker.Worker
}

func (existingTenantScenarioWorker) String() string {
	return "ExistingTenantScenarioWorker"
}
func (w *existingTenantScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

func (sc *Scenario) ExistingTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32, isHeavyTenant bool) (Worker, error) {
	scTag := ScenarioTagOrganizerPopularTenant

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.ExistingTenantScenario(ctx, step, scTag, isHeavyTenant); err != nil {
			sc.ScenarioError(scTag, err)
			time.Sleep(SleepOnError)
		}
	},
		// // 無限回繰り返す
		worker.WithInfinityLoop(),
		worker.WithUnlimitedParallelism(),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return &existingTenantScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) ExistingTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag, isHeavyTenant bool) error {
	report := timeReporter("既存テナントシナリオ")
	defer report()
	sc.ScenarioStart(scTag)

	var tenantName string
	if isHeavyTenant {
		tenantName = "isucon"
	} else {
		var data *InitialDataRow
		for {
			data = sc.InitialData.Choise()
			if data.TenantName != "isucon" {
				break
			}
		}
		tenantName = data.TenantName
	}

	_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenantName, "organizer")
	if err != nil {
		return err
	}

	// 大会を開催し、ダッシュボードを受け取ったら再び大会を開催する
	orgJobConf := &OrganizerJobConfig{
		orgAg:       orgAg,
		scTag:       scTag,
		tenantName:  tenantName,
		scoreRepeat: 2,
	}

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
