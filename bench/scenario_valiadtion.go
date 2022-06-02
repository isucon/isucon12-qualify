package bench

import (
	"context"
	"fmt"

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

	// 検証で作成する参加者数 結果のScoreも同数作成する
	playerNum := 300

	// SaaS管理者, 主催者, 参加者のagent作成
	admin := Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerName: "admin",
		Option:     sc.Option,
	}
	if err := admin.SetJWT(); err != nil {
		return err
	}
	adminAg, err := admin.GetAgent()
	if err != nil {
		return err
	}

	// SaaS管理API
	tenantDisplayName := "validate_tenantname"
	var tenantName string
	{
		res, err := PostAdminTenantsAddAction(ctx, tenantDisplayName, adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				if tenantDisplayName != r.Data.Tenant.DisplayName {
					return fmt.Errorf("作成したテナントのDisplayNameが違います (want: %s, got: %s)", tenantDisplayName, r.Data.Tenant.DisplayName)
				}
				tenantName = r.Data.Tenant.Name
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetAdminTenantsBillingAction(ctx, tenantName, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				if 1 > len(r.Data.Tenants) {
					return fmt.Errorf("請求ダッシュボードの結果が足りません")
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会主催者API
	organizer := Account{
		Role:       AccountRoleOrganizer,
		TenantName: tenantName,
		PlayerName: "organizer",
		Option:     sc.Option,
	}
	if err := organizer.SetJWT(); err != nil {
		return err
	}
	orgAg, err := organizer.GetAgent()
	if err != nil {
		return err
	}

	competitionName := "validate_competition"
	var competitionId int64
	{
		res, err := PostOrganizerCompetitonsAddAction(ctx, competitionName, orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				if competitionName != r.Data.Competition.Title {
					return fmt.Errorf("追加された大会の名前が違います (want: %s, got: %s)", competitionName, r.Data.Competition.Title)
				}
				competitionId = r.Data.Competition.ID
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	var playerDisplayNames []string
	for i := 0; i < playerNum; i++ {
		playerDisplayNames = append(playerDisplayNames, fmt.Sprintf("validate_player%d", i))
	}
	var playerNames []string
	{
		res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				if playerNum != len(r.Data.Players) {
					return fmt.Errorf("追加されたプレイヤー数が違います (want: %d, got: %d)", playerNum, len(r.Data.Players))
				}
				for _, pl := range r.Data.Players {
					playerNames = append(playerNames, pl.Name)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := PostOrganizerApiPlayerDisqualifiedAction(ctx, playerNames[1], orgAg)
		v := ValidateResponse("参加者を失格にする", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				if playerNames[1] != r.Data.Player.Name {
					return fmt.Errorf("失格にした参加者が違います (want: %s, got: %s)", playerNames[1], r.Data.Player.Name)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	var score ScoreRows
	for i, playerName := range playerNames {
		score = append(score, &ScoreRow{
			PlayerName: playerName,
			Score:      100 * i,
		})
	}
	{
		csv := score.CSV()
		res, err := PostOrganizerCompetitionResultAction(ctx, competitionId, []byte(csv), orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				_ = r // responseは空
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
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r // responseは空
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
				if 1 != len(r.Data.Reports) {
					return fmt.Errorf("請求レポートの数が違います (want: %d, got: %d)", 1, len(r.Data.Reports))
				}
				if competitionId != r.Data.Reports[0].CompetitionID {
					return fmt.Errorf("対象の大会のIDが違います (want: %d, got: %d)", competitionId, r.Data.Reports[0].CompetitionID)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会参加者API
	player := Account{
		Role:       AccountRolePlayer,
		TenantName: tenantName,
		PlayerName: playerNames[0],
		Option:     sc.Option,
	}
	if err := player.SetJWT(); err != nil {
		return err
	}
	playerAg, err := player.GetAgent()
	if err != nil {
		return err
	}

	{
		res, err := GetPlayerAction(ctx, playerNames[0], playerAg)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if playerNames[0] != r.Data.Player.Name {
					return fmt.Errorf("参照した参加者名が違います (want: %s, got: %s)", playerNames[0], r.Data.Player.Name)
				}
				if 1 != len(r.Data.Scores) {
					return fmt.Errorf("参加した大会数が違います (want: %d, got: %d)", 1, len(r.Data.Scores))
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, competitionId, playerAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if len(score) != len(r.Data.Ranks) && 100 < len(r.Data.Ranks) {
					return fmt.Errorf("大会のランキングの結果の数が違います(最大100件) (want: %d, got: %d)", len(score), len(r.Data.Ranks))
				}
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
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				if 1 != len(r.Data.Competitions) {
					return fmt.Errorf("テナントに含まれる大会の数が違います (want: %d, got: %d)", 1, len(r.Data.Competitions))
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	return nil
}
