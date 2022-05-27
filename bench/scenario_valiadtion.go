package bench

import (
	"context"

	"github.com/isucon/isucandar"
)

// ベンチ実行後の整合性検証シナリオ
// isucandar.ValidateScenarioを満たすメソッド
// isucandar.Benchmark の validation ステップで実行される
func (sc *Scenario) ValidationScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("validation")
	defer report()

	ContestantLogger.Println("[ValidationScenario] 整合性チェックを開始します")
	defer ContestantLogger.Printf("[ValidationScenario] 整合性チェックを終了します")

	// SaaS管理者, 主催者, 参加者のagent作成
	admin := Account{
		Role: AccountRoleAdmin,
	}
	adminAg, err := admin.GetAgent(sc.Option)
	if err != nil {
		return err
	}

	organizer := Account{
		Role: AccountRoleOrganizer,
	}
	orgAg, err := organizer.GetAgent(sc.Option)
	if err != nil {
		return err
	}

	competitor := Account{
		Role: AccountRoleCompetitor,
	}
	compAg, err := competitor.GetAgent(sc.Option)
	if err != nil {
		return err
	}

	// SaaS管理API
	{
		res, err := PostTenantsAddAction(ctx, "name", adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200, 500)) // TODO
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetTenantsBillingAction(ctx, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200, 500))
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会主催者API
	{
		res, err := PostCompetitionsAddAction(ctx, "title", orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetititorsAddAction(ctx, "name", orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitorDisqualifiedAction(ctx, "competitor_id", orgAg)
		v := ValidateResponse("参加者を失格にする", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitionResultAction(ctx, "competition_id", orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitionFinishAction(ctx, "competition_id", orgAg)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetTenantBillingAction(ctx, orgAg)
		v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会参加者API
	{
		res, err := GetCompetitorAction(ctx, "competitor_id", compAg)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetCompetitionRankingAction(ctx, "competiton_id", compAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200, 404)) // TODO
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetCompetitionsAction(ctx, compAg)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200, 404)) // TODO
		if !v.IsEmpty() {
			return v
		}
	}

	return nil
}
