package bench

import (
	"context"
	"fmt"
	"math/rand"
	"time"

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

	adminAc, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	// beforeTenantIDを範囲から決める
	rangeStart := ConstAdminBillingValidateScenarioIDRange[0] + 10
	rangeEnd := ConstAdminBillingValidateScenarioIDRange[1] + 1
	index := randomRange([]int{rangeStart, rangeEnd})
	billingBeforeTenantID := fmt.Sprintf("%d", sc.InitialDataTenant[index].TenantID)

	// 最初の状態のBilling
	var billingResultTenants []isuports.TenantWithBilling
	{
		res, err, txt := GetAdminTenantsBillingAction(ctx, billingBeforeTenantID, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("テナント別の請求ダッシュボード", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				billingResultTenants = r.Data.Tenants
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// レスポンスの中から対象のテナントを選ぶ
	targetIndex := rand.Intn(len(billingResultTenants))
	targetTenant := billingResultTenants[targetIndex].Name

	// 大会を開催、Billing確定まで進める
	orgAc, _, err := sc.GetAccountAndAgent(AccountRoleOrganizer, targetTenant, "organizer")
	if err != nil {
		return err
	}

	conf := &OrganizerJobConfig{
		orgAc:           orgAc,
		scTag:           scTag,
		tenantName:      targetTenant,
		scoreRepeat:     1,
		scoreInterval:   0,
		addScoreNum:     0,
		playerWorkerNum: 10,
	}
	jobResult, err := sc.OrganizerJob(ctx, step, conf)
	if err != nil {
		return err
	}

	// 反映まで3秒まで猶予がある
	SleepWithCtx(ctx, time.Second*3)

	// 反映確認

	// チェック項目
	// 合計金額が正しく増えていること
	sumYen := int64(0)
	for _, t := range billingResultTenants {
		sumYen += t.BillingYen
	}
	// OrganizerJobによって増えた値段を加算
	sumYen += int64(jobResult.ScoredPlayerNum * 100)

	{
		res, err, txt := GetAdminTenantsBillingAction(ctx, billingBeforeTenantID, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("テナント別の請求ダッシュボード", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				resultYen := int64(0)
				for _, t := range r.Data.Tenants {
					resultYen += t.BillingYen
				}
				if resultYen != sumYen {
					return fmt.Errorf("全テナントの合計金額が正しくありません (want: %d, got: %d)", sumYen, resultYen)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	return nil
}
