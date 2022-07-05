package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

type adminBillingValidateWorker struct {
	worker *worker.Worker
}

func (adminBillingValidateWorker) String() string {
	return "AdminBillingValidateWorker"
}
func (w *adminBillingValidateWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

func (sc *Scenario) AdminBillingValidateWorker(step *isucandar.BenchmarkStep, p int32) (*adminBillingValidateWorker, error) {
	scTag := ScenarioTagAdminBillingValidate
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.AdminBillingValidate(ctx, step); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
		}
	},
		worker.WithInfinityLoop(),
		worker.WithUnlimitedParallelism(),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)

	return &adminBillingValidateWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) AdminBillingValidate(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("SaaS管理者請求検証シナリオ")
	defer report()
	scTag := ScenarioTagAdminBillingValidate
	sc.ScenarioStart(scTag)

	_, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	// 初期データからテナント選ぶ
	index := randomRange(ConstAdminBillingValidateScenarioIDRange)
	// tenant := sc.InitialDataTenant[int64(index)]

	// indexが含まれる区間がとれるAdminBillingのbefore
	var billingBeforeTenantID string
	{
		rangeEnd := ConstAdminBillingValidateScenarioIDRange[1]
		n := index + 10
		if rangeEnd < n {
			n = rangeEnd
		}
		billingBeforeTenantID = fmt.Sprintf("%d", sc.InitialDataTenant[int64(n)].TenantID)
	}

	// 最初の状態のBilling
	var billingResultTenants []isuports.TenantWithBilling
	{
		res, err := GetAdminTenantsBillingAction(ctx, billingBeforeTenantID, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				billingResultTenants = r.Data.Tenants
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 大会を開催、Billing確定まで進める
	// _, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.Name, "organizer")
	// if err != nil {
	// 	return err
	// }

	// 反映まで3秒まで猶予がある
	SleepWithCtx(ctx, time.Second*3)

	// 反映確認
	{
		res, err := GetAdminTenantsBillingAction(ctx, billingBeforeTenantID, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				if diff := cmp.Diff(billingResultTenants, r.Data.Tenants); diff != "" {
					return fmt.Errorf("AdminBillingの結果が違います (-want +got): %s", diff)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	return nil
}
