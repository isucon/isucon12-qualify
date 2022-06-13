package bench

import (
	"context"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (sc *Scenario) AdminBillingScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		sc.AdminBillingScenario(ctx, step)
	},
		// 無限回繰り返す
		worker.WithInfinityLoop(),
		worker.WithUnlimitedParallelism(),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return w, nil
}

func (sc *Scenario) AdminBillingScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("admin billingを見続けるシナリオ")
	defer report()

	scTag := ScenarioTag("AdminBillingScenario")

	admin := &Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerID:   "admin",
		Option:     sc.Option,
	}
	if err := admin.SetJWT(sc.RawKey); err != nil {
		return err
	}
	adminAg, err := admin.GetAgent()
	if err != nil {
		return err
	}

	// ランダムで初期データから取るとid=1を引いて一つも結果を取らないことがあるので
	// ページングなしで順に辿っていく
	res, err := GetAdminTenantsBillingAction(ctx, "", adminAg)
	v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
		WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
			_ = r
			return nil
		}),
	)
	if v.IsEmpty() {
		sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
	} else {
		return v
	}

	return nil
}
