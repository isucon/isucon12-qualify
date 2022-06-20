package bench

import (
	"context"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (sc *Scenario) ExistingTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32, isHeavyTenant bool) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.ExistingTenantScenario(ctx, step, isHeavyTenant); err != nil {
			AdminLogger.Printf("[ExistingTenantScenario]: %v", err)
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
	return w, nil
}

func (sc *Scenario) ExistingTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep, isHeavyTenant bool) error {
	report := timeReporter("既存テナントシナリオ")
	defer report()
	var scTag ScenarioTag

	// isHeavyTenantに応じて重いデータかそれ以外を引く
	var tenantName string
	if isHeavyTenant {
		scTag = "ExistingTenantScenario_HevaryTenant"
		tenantName = "isucon"
	} else {
		scTag = "ExistingTenantScenario"
		var data *InitialDataRow
		for {
			data = sc.InitialData.Choise()
			if data.TenantName != "isucon" {
				break
			}
		}
		tenantName = data.TenantName
	}
	sc.ScenarioStart(scTag)

	organizer := Account{
		Role:       AccountRoleOrganizer,
		TenantName: tenantName,
		PlayerID:   "organizer",
		Option:     sc.Option,
	}

	if err := organizer.SetJWT(sc.RawKey); err != nil {
		return err
	}
	orgAg, err := organizer.GetAgent()
	if err != nil {
		return err
	}

	// テナント請求ダッシュボードの閲覧 x 1
	{
		res, err := GetOrganizerBillingAction(ctx, orgAg)
		v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBilling) error {
				// TODO: 簡単に内容チェック
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
		} else {
			return v
		}
	}

	return nil
}
