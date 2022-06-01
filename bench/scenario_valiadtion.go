package bench

import (
	"context"
	"fmt"
	"log"

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
		Role:    AccountRoleAdmin,
		BaseURL: "http://localhost:3000/", // bench起動時のtarget urlかな
		Option:  sc.Option,
	}
	if err := admin.SetJWT("admin", "admin"); err != nil {
		return err
	}
	adminAg, err := admin.GetAgent()
	if err != nil {
		return err
	}

	organizer := Account{
		Role:    AccountRoleOrganizer,
		BaseURL: "http://localhost:3000/",
		Option:  sc.Option,
	}
	if err := organizer.SetJWT("validate_tenantname", "organizer"); err != nil {
		return err
	}
	orgAg, err := organizer.GetAgent()
	if err != nil {
		return err
	}

	player := Account{
		Role:    AccountRolePlayer,
		BaseURL: "http://localhost:3000/",
		Option:  sc.Option,
	}
	if err := player.SetJWT("validate_tenantname", "validate_playername"); err != nil {
		return err
	}
	playerAg, err := player.GetAgent()
	if err != nil {
		return err
	}

	// SaaS管理API
	var tenantName string
	{
		res, err := PostAdminTenantsAddAction(ctx, "validate_tenantname", adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				fmt.Printf("%+v\n", r)
				// tenantName = r.Data.Tenant.Name
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	log.Println(tenantName)
	// var playersName []string
	// var competitionId int
	// var competitionName string
	{
		res, err := GetAdminTenantsBillingAction(ctx, 111 /*tenant id*/, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会主催者API
	{
		res, err := PostOrganizerCompetitonsAddAction(ctx, "validate_competition", "tenant-010001", orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerPlayersAddAction(ctx, "validate_playername", "tenant-010001", orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerApiPlayerDisqualifiedAction(ctx, "validate_playername", "tenant-010001", orgAg)
		v := ValidateResponse("参加者を失格にする", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerCompetitionResultAction(ctx, "competition_id", orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerCompetitionFinishAction(ctx, "competition_id", orgAg)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetOrganizerBillingAction(ctx, orgAg)
		v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会参加者API
	{
		res, err := GetPlayerAction(ctx, "validate_playername", playerAg)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, "validate_playername", playerAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionsAction(ctx, playerAg)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}

	return nil
}
