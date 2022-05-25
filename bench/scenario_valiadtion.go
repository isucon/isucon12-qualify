package bench

import (
	"context"

	"github.com/isucon/isucandar"
)

// ベンチ実行後の整合性検証シナリオ
// isucandar.ValidateScenarioを満たすメソッド
// isucandar.Benchmark の validation ステップで実行される
func (s *Scenario) ValidationScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("validation")
	defer report()

	ContestantLogger.Println("[ValidationScenario] 整合性チェックを開始します")
	defer ContestantLogger.Printf("[ValidationScenario] 整合性チェックを終了します")

	ag, _ := s.Option.NewAgent(false)

	{
		res, err := PostTenantsAddAction(ctx, ag)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetTenantsBillingAction(ctx, ag)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetititorsAddAction(ctx, ag)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitorDisqualifiedAction(ctx, ag)
		v := ValidateResponse("参加者を失格にする", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitionsAddAction(ctx, ag)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitionFinishAction(ctx, ag)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostCompetitionResultAction(ctx, ag)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetTenantBillingAction(ctx, ag)
		v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetCompetitorAction(ctx, ag)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetCompetitionRankingAction(ctx, ag)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetCompetitionsAction(ctx, ag)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}

	return nil
}
