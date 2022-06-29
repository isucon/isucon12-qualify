package bench

import (
	"context"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

type adminBillingScenarioWorker struct {
	worker *worker.Worker
}

func (adminBillingScenarioWorker) String() string {
	return "AdminBillingScenarioWorker"
}
func (w *adminBillingScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

// ずっと/admin/billingを見続けるシナリオ
// 指定回数エラーが出るまで繰り返し、並列動作はしない

func (sc *Scenario) AdminBillingScenarioWorker(step *isucandar.BenchmarkStep, p int32) (Worker, error) {
	scTag := ScenarioTagAdmin
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.AdminBillingScenario(ctx, step, scTag); err != nil {
			sc.ScenarioError(scTag, err)
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
	return &adminBillingScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) AdminBillingScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag) error {
	report := timeReporter("admin billingを見続けるシナリオ")
	defer report()

	sc.ScenarioStart(scTag)

	opt := sc.Option
	opt.RequestTimeout = time.Second * 60 // AdminBillingのみタイムアウトを60秒まで許容
	admin := &Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerID:   "admin",
		Option:     opt,
	}
	if err := admin.SetJWT(sc.RawKey, true); err != nil {
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
				beforeTenantID = r.Data.Tenants[len(r.Data.Tenants)-1].ID
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else {
			return v
		}
		// id=1が重いので、light modeなら一回で終わる
		if sc.Option.LoadType == LoadTypeLight {
			completed = true
		}

	}
	// Billingが見終わったら新規テナントを追加する
	newTenantWorker, err := sc.NewTenantScenarioWorker(step, 1)
	if err != nil {
		return err
	}
	sc.WorkerCh <- newTenantWorker

	return nil
}
