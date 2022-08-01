package bench

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
)

type adminBillingScenarioWorker struct {
	worker *worker.Worker
}

func (adminBillingScenarioWorker) String() string {
	return "AdminBillingScenarioWorker"
}
func (w *adminBillingScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

// ずっと/admin/billingを見続けるシナリオ
func (sc *Scenario) AdminBillingScenarioWorker(step *isucandar.BenchmarkStep, p int32) (Worker, error) {
	scTag := ScenarioTagAdminBilling
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.AdminBillingScenario(ctx, step, scTag); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
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
	adminAc := &Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerID:   "admin",
		Option:     opt,
	}
	if err := adminAc.SetJWT(sc.RawKey, true); err != nil {
		return err
	}
	adminAg, err := adminAc.GetAgent()
	if err != nil {
		return err
	}

	// 1ページ目から最後まで辿る
	// 最初はbeforeが空, ただし初回のみテナント追加と最新の取得がかぶらないように初期データのIDを入れる
	beforeTenantID := ""
	if sc.HeavyTenantCount == 0 {
		beforeTenantID = "100"
	}
	completed := false
	for !completed {
		res, err, txt := GetAdminTenantsBillingAction(ctx, beforeTenantID, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("テナント別の請求ダッシュボード", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				if len(r.Data.Tenants) == 0 {
					completed = true
					return nil
				}

				// IDは降順である必要がある
				beforeID := int64(0)
				for _, tenant := range r.Data.Tenants {
					id, err := strconv.ParseInt(tenant.ID, 10, 64)
					if err != nil {
						return fmt.Errorf("tenant IDの形が違います %s", tenant.ID)
					}

					if beforeID != 0 && beforeID < id {
						return fmt.Errorf("tenant IDが降順ではありません (before: %d, got: %d)", beforeID, id)
					}
					beforeID = id

				}

				beforeTenantID = r.Data.Tenants[len(r.Data.Tenants)-1].ID
				return nil
			}),
		)

		// 無限forになるのでcontext打ち切りを確認する
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else if v.Canceled {
			// contextの打ち切りでloopを抜ける
			return nil
		} else {
			// ErrorCountで打ち切りがあるので、ここでreturn ValidateErrorは返さずリトライする
			// ただしsleepを挟む
			sc.AddErrorCount()
			SleepWithCtx(ctx, time.Millisecond*100)
		}

	}

	// Billingが見終わったら新規テナントを追加する
	tenant := data.CreateTenant(data.TenantTagGeneral)
	{
		res, err, txt := PostAdminTenantsAddAction(ctx, tenant.Name, tenant.DisplayName, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("新規テナント作成", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTAdminTenantsAdd, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount()
			return v
		}
		sc.TenantAddLog.Printf("テナント「%s」を作成しました", tenant.DisplayName)
	}

	newTenantWorker, err := sc.NewTenantScenarioWorker(step, tenant, 1)
	if err != nil {
		return err
	}
	sc.WorkerCh <- newTenantWorker

	// 重いテナント(id=1)を見るworker
	if sc.HeavyTenantCount == 0 {
		wkr, err := sc.PopularTenantScenarioWorker(step, 1, true)
		if err != nil {
			return err
		}
		sc.WorkerCh <- wkr
		sc.HeavyTenantCount++
	}

	return nil
}
