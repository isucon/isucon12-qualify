package bench

import (
	"context"
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

	// SaaS管理API
	var tenantName string
	{
		res, err := PostAdminTenantsAddAction(ctx, "validate_tenantname", adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				tenantName = r.Data.Tenant.Name
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	log.Println(tenantName)
	{
		res, err := GetAdminTenantsBillingAction(ctx, tenantName, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会主催者API
	organizer := Account{
		Role:    AccountRoleOrganizer,
		BaseURL: "http://localhost:3000/",
		Option:  sc.Option,
	}
	if err := organizer.SetJWT(tenantName, "organizer"); err != nil {
		return err
	}
	orgAg, err := organizer.GetAgent()
	if err != nil {
		return err
	}

	competitionName := "validate_competition"
	var competitionId int64
	{
		res, err := PostOrganizerCompetitonsAddAction(ctx, competitionName, tenantName, orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				competitionId = r.Data.Competition.ID
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	playerName := "validate_playername"
	{
		res, err := PostOrganizerPlayersAddAction(ctx, playerName, tenantName, orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerApiPlayerDisqualifiedAction(ctx, playerName, tenantName, orgAg)
		v := ValidateResponse("参加者を失格にする", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerCompetitionResultAction(ctx, competitionId, orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBase) error { // TODO: あれ？
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerCompetitionFinishAction(ctx, competitionId, orgAg)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBase) error { // TODO: あれ？
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetOrganizerBillingAction(ctx, orgAg)
		v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBilling) error {
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会参加者API
	player := Account{
		Role:    AccountRolePlayer,
		BaseURL: "http://localhost:3000/",
		Option:  sc.Option,
	}
	if err := player.SetJWT(tenantName, playerName); err != nil {
		return err
	}
	playerAg, err := player.GetAgent()
	if err != nil {
		return err
	}

	{
		res, err := GetPlayerAction(ctx, playerName, playerAg)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, playerName, playerAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBase) error { // TODO: あれ？
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionsAction(ctx, playerAg)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBase) error { // TODO: あれ？
				_ = r
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	return nil
}
