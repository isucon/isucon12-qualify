package bench

import (
	"context"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

// ずっと/admin/billingを見続けるシナリオ
// 指定回数エラーが出るまで繰り返し、並列動作はしない
func (sc *Scenario) AdminBillingScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.AdminBillingScenario(ctx, step); err != nil {
			AdminLogger.Printf("[AdminBillingScenario]: %s", err)
			time.Sleep(SleepOnError)
		}
	},
		worker.WithInfinityLoop(),
		worker.WithMaxParallelism(1),
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
	AdminLogger.Printf("%s start\n", scTag)

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

	// 1ページ目から最後まで辿る
	beforeTenantID := "" // 最初はbeforeが空
	completed := false
	for !completed {
		res, err := GetAdminTenantsBillingAction(ctx, beforeTenantID, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				if len(r.Data.Tenants) == 0 {
					completed = true
					return nil
				}
				for _, tenant := range r.Data.Tenants {
					AdminLogger.Printf("%s: %d yen", tenant.Name, tenant.BillingYen)
				}
				beforeTenantID = r.Data.Tenants[len(r.Data.Tenants)-1].ID
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else {
			return v
		}
	}
	return nil
}
