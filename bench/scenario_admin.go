package bench

import (
	"context"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (sc *Scenario) AdminScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		sc.AdminScenario(ctx, step)
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

func (sc *Scenario) AdminScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("SaaS管理者シナリオ")
	defer report()

	// テナント作成 x N
	// テナント毎の請求一覧の閲覧 x N

	admin := Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerName: "admin",
		Option:     sc.Option,
	}
	if err := admin.SetJWT(); err != nil {
		return err
	}
	adminAg, err := admin.GetAgent()
	if err != nil {
		return err
	}

	displayNames := []string{
		"first", "second",
	}
	tenants := map[string]*TenantData{}

	for _, displayName := range displayNames {
		res, err := PostAdminTenantsAddAction(ctx, displayName, adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				tenants[displayName] = &TenantData{
					Name:        r.Data.Tenant.Name,
					DisplayName: r.Data.Tenant.DisplayName,
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			step.AddScore(ScorePOSTTenantsAdd)
		} else {
			return v
		}
	}

	for _, tenant := range tenants {
		res, err := GetAdminTenantsBillingAction(ctx, tenant.Name, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			step.AddScore(ScoreGETTenantsBilling)
		} else {
			return v
		}
	}

	return nil
}
