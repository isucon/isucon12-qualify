package bench

import (
	"context"
	"fmt"
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

	// 初期データからテナント選ぶ
	index := randomRange(ConstAdminBillingValidateScenarioIDRange)
	tenant := sc.InitialDataTenant[int64(index)]

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
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 大会を開催、Billing確定まで進める
	orgAc, _, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.TenantName, "organizer")
	if err != nil {
		return err
	}

	conf := &OrganizerJobConfig{
		orgAc:         orgAc,
		scTag:         scTag,
		tenantName:    tenant.TenantName,
		scoreRepeat:   1,
		scoreInterval: 0,
		addScoreNum:   0,
	}
	if err := sc.OrganizerJob(ctx, step, conf); err != nil {
		return err
	}

	// 反映まで3秒まで猶予がある
	SleepWithCtx(ctx, time.Second*3)

	// 反映確認

	// チェック項目
	// 合計金額が増えていること
	// TODO: 必要に応じて追加, ただしOrganizerJobによって増えた金額は現状取れない
	sumYen := int64(0)
	for _, t := range billingResultTenants {
		sumYen += t.BillingYen
	}

	{
		res, err, txt := GetAdminTenantsBillingAction(ctx, billingBeforeTenantID, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("テナント別の請求ダッシュボード", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				resultYen := int64(0)
				for _, t := range r.Data.Tenants {
					resultYen += t.BillingYen
				}
				if resultYen <= sumYen {
					ContestantLogger.Println("(CIでこの文章を見た方へ) 初期実装で「全テナントの合計金額が正しくありません」エラーが起きた様子を探しています。benchの不具合の可能性があります。コメントのリンクを #154 へ貼って頂けると嬉しいです") // TODO: 消す
					return fmt.Errorf("全テナントの合計金額が正しくありません 金額は増えている必要があります (want: >%d, got:%d)", sumYen, resultYen)
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
