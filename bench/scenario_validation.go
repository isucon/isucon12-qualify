package bench

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/isucon/isucandar"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
	"golang.org/x/sync/errgroup"
)

var (
	notExistID              = "0000000000"  // 存在しない想定のID(competition, player用)
	notExistName            = "null-null"   // 存在しない想定のName(tenant用)
	playerNum               = 100           // 検証で作成する参加者数
	disqualifiedPlayerIndex = playerNum - 2 // 失格プレイヤー
	noScorePlayerIndex      = playerNum - 1 // スコア未登録プレイヤー この値以降にはスコアは登録されない
)

// ベンチ実行前の整合性検証シナリオ
// isucandar.ValidateScenarioを満たすメソッド
// isucandar.Benchmark の validation ステップで実行される
func (sc *Scenario) ValidationScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("validation")
	defer report()

	ContestantLogger.Println("[ValidationScenario] 整合性チェックを開始します")
	defer ContestantLogger.Printf("[ValidationScenario] 整合性チェックを終了します")

	tenantName := "valid-tenantid"
	tenantDisplayName := "valid-Tenantname"

	// SaaS管理者のagent作成
	adminAc, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}
	// SaaS管理API
	{
		res, err, txt := PostAdminTenantsAddAction(ctx, tenantName, tenantDisplayName, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("新規テナント作成", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				if tenantDisplayName != r.Data.Tenant.DisplayName {
					return fmt.Errorf("作成したテナントのDisplayNameが違います (want: %s, got: %s)", tenantDisplayName, r.Data.Tenant.DisplayName)
				}
				if tenantName != r.Data.Tenant.Name {
					return fmt.Errorf("作成したテナントのNameが違います (want: %s, got: %s)", tenantName, r.Data.Tenant.Name)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	eg := errgroup.Group{}
	eg.Go(func() error {
		return allAPISuccessCheck(ctx, sc, step, tenantName, tenantDisplayName)
	})
	eg.Go(func() error {
		return rankingCheck(ctx, sc, step)
	})
	eg.Go(func() error {
		return badRequestCheck(ctx, sc, step)
	})
	eg.Go(func() error {
		return invalidJWTCheck(ctx, sc, step)
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	if n := len(step.Result().Errors.All()); n != 0 {
		return fmt.Errorf("エラーが%d件あります", n)
	}

	return nil
}

// すべてのAPIを一通り正常系チェック
// 失敗したらエラーで終了する
func allAPISuccessCheck(ctx context.Context, sc *Scenario, step *isucandar.BenchmarkStep, tenantName, tenantDisplayName string) error {
	// SaaS管理者のagent作成
	adminAc, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}
	// 大会主催者API
	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenantName, "organizer")
	if err != nil {
		return err
	}

	// プレイヤー追加
	var playerIDs []string
	var playerDisplayNames []string
	for i := 0; i < playerNum; i++ {
		playerDisplayNames = append(playerDisplayNames, fmt.Sprintf("validate_player%d", i))
	}
	{
		res, err, txt := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナントへプレイヤー追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				if playerNum != len(r.Data.Players) {
					return fmt.Errorf("追加されたプレイヤー数が違います (want: %d, got: %d)", playerNum, len(r.Data.Players))
				}
				for _, pl := range r.Data.Players {
					if pl.IsDisqualified {
						return fmt.Errorf("新規追加されたプレイヤーは失格になっていない必要があります player.id: %s", pl.ID)
					}
					var ok bool
					for _, n := range playerDisplayNames {
						if n == pl.DisplayName {
							ok = true
							break
						}
					}
					if !ok {
						return fmt.Errorf("新規追加したプレイヤーのDisplayNameが見つかりません")
					}
					playerIDs = append(playerIDs, pl.ID)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
		// NOTE: 不正リクエストチェックなし
	}

	// プレイヤー一覧取得
	{
		res, err, txt := GetOrganizerPlayersListAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナントのプレイヤー一覧取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				if len(playerIDs) != len(r.Data.Players) {
					return fmt.Errorf("プレイヤー数が違います (want: %d, got: %d)", playerNum, len(r.Data.Players))
				}

				mapResponsePlayerID := map[string]struct{}{}
				for _, pl := range r.Data.Players {
					if pl.IsDisqualified {
						return fmt.Errorf("新規追加されたプレイヤーは失格になっていない必要があります player.id: %s", pl.ID)
					}
					mapResponsePlayerID[pl.ID] = struct{}{}
				}

				var ok bool
				for _, pid := range playerIDs {
					if _, ok = mapResponsePlayerID[pid]; !ok {
						return fmt.Errorf("新規追加されたプレイヤーが一覧に含まれていません: id:%s", pid)
					}
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
		// NOTE: 不正リクエストチェックなし
	}

	var competitionID string
	competitionTitle := "validate_competition"
	{
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, competitionTitle, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("新規大会追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				if competitionTitle != r.Data.Competition.Title {
					return fmt.Errorf("追加された大会の名前が違います (want: %s, got: %s)", competitionTitle, r.Data.Competition.Title)
				}
				if r.Data.Competition.IsFinished {
					return fmt.Errorf("新規追加された大会は開催中である必要があります competition.title: %s, competition.id: %s", r.Data.Competition.Title, r.Data.Competition.ID)
				}
				competitionID = r.Data.Competition.ID
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
		// NOTE: 不正リクエストチェックなし
	}

	{
		idx := disqualifiedPlayerIndex
		res, err, txt := PostOrganizerApiPlayerDisqualifiedAction(ctx, playerIDs[idx], orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("プレイヤーを失格にする", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				if !r.Data.Player.IsDisqualified {
					return fmt.Errorf("プレイヤーが失格になっていません player.id: %s", r.Data.Player.ID)
				}
				if playerIDs[idx] != r.Data.Player.ID {
					return fmt.Errorf("失格にしたプレイヤーが違います (want: %s, got: %s)", playerIDs[idx], r.Data.Player.ID)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 大会結果CSV入稿
	var score ScoreRows
	{
		// NOTE:
		//	失格済みのプレイヤーは含まれていても問題ない
		// 	最後の一人はスコア未登録+ranking参照済みユーザーとしてbilling検証に利用する
		// 後ろのプレイヤーはスコア未登録(noScorePlayerIndex以降)
		for i, playerID := range playerIDs[:noScorePlayerIndex] {
			score = append(score, &ScoreRow{
				PlayerID: playerID,
				Score:    100 + i,
			})
		}

		csv := score.CSV()
		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, competitionID, []byte(csv), orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("大会結果CSV入稿", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				_ = r // responseは空
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}

	}
	// 大会参加者API
	playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerIDs[0])
	if err != nil {
		return err
	}

	{
		checkPlayerIndex := 10 // < disqualifiedPlayerIndex
		res, err, txt := GetPlayerAction(ctx, playerIDs[checkPlayerIndex], playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("プレイヤーと戦績情報取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if playerIDs[checkPlayerIndex] != r.Data.Player.ID {
					return fmt.Errorf("参照したプレイヤー名が違います (want: %s, got: %s)", playerIDs[checkPlayerIndex], r.Data.Player.ID)
				}
				if 1 != len(r.Data.Scores) {
					return fmt.Errorf("参加した大会数が違います (want: %d, got: %d)", 1, len(r.Data.Scores))
				}
				if competitionTitle != r.Data.Scores[0].CompetitionTitle {
					return fmt.Errorf("参加した大会IDが違います (want: %s, got: %s)", competitionTitle, r.Data.Scores[checkPlayerIndex].CompetitionTitle)
				}
				if int64(100+checkPlayerIndex) != r.Data.Scores[0].Score {
					return fmt.Errorf("スコアが違います (want: %d, got: %d)", 100+checkPlayerIndex, r.Data.Scores[0].Score)
				}

				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// ランキング確認
	// NOTE: 最終的なランキングが正しいことは大会終了APIを叩いた後に確認
	{
		//rank_after未指定
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, "", playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("大会内のランキング取得: ページングなし", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if len(score) != len(r.Data.Ranks) && 100 < len(r.Data.Ranks) {
					return fmt.Errorf("大会のランキングの結果の数が違います(最大100件) (want: %d, got: %d)", len(score), len(r.Data.Ranks))
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}
	{
		// rank_afterで最後の1件だけを取るように指定する
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, strconv.Itoa(len(score)-1), playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("大会内のランキング取得: ページングあり", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if 1 != len(r.Data.Ranks) {
					return fmt.Errorf("rank_after指定時の大会のランキングの結果の数が違います (want: %d, got: %d)", 1, len(r.Data.Ranks))
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}
	// スコア未登録プレイヤーがランキングを参照する
	{
		idx := noScorePlayerIndex
		noScorePlayerAc, noScorePlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerIDs[idx])
		if err != nil {
			return err
		}

		res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, "", noScorePlayerAg)
		msg := fmt.Sprintf("%s %s", noScorePlayerAc, txt)
		v := ValidateResponseWithMsg("大会内のランキング取得: スコア未登録プレイヤー", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if len(score) != len(r.Data.Ranks) && 100 < len(r.Data.Ranks) {
					return fmt.Errorf("大会のランキングの結果の数が違います(最大100件) (want: %d, got: %d)", len(score), len(r.Data.Ranks))
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 大会の終了(organizer/competition/finish)後は反映まで3sの猶予がある
	SleepWithCtx(ctx, time.Second*3)

	// 主催者API 大会の終了
	{
		res, err, txt := PostOrganizerCompetitionFinishAction(ctx, competitionID, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("大会終了", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r // responseは空
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}
	// 最終的なランキングが正しいことを確認
	{
		// NOTE: 失格者はランキングから除外しない
		rankingNum := len(score)
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, "", playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("大会内のランキング取得: ランキングが正しいことを確認", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if rankingNum != len(r.Data.Ranks) && 100 < len(r.Data.Ranks) {
					return fmt.Errorf("大会のランキングの結果の数が違います(最大100件) (want: %d, got: %d)", rankingNum, len(r.Data.Ranks))
				}

				// ランキングチェック
				// playerIDsのindexの順にscoreが大きいのでランキングは逆順になる
				// playerIDs[0]は最下位 = (noScorePlayerIndex-1)位(失格1)
				playerIDScoreMap := map[string]isuports.CompetitionRank{}
				for _, rank := range r.Data.Ranks {
					playerIDScoreMap[rank.PlayerID] = rank
				}
				for index, playerID := range playerIDs[:noScorePlayerIndex] {
					rank := int64(noScorePlayerIndex - index)
					if rank != playerIDScoreMap[playerID].Rank {
						return fmt.Errorf("ランキングのランクが違います (want: %d, got: %d)", rank, playerIDScoreMap[playerID].Rank)
					}
					score := int64(100 + index)
					if score != playerIDScoreMap[playerID].Score {
						return fmt.Errorf("ランキングのスコアが違います (want: %d, got: %d)", score, playerIDScoreMap[playerID].Score)
					}
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 主催者API テナント内請求情報確認
	{
		res, err, txt := GetOrganizerBillingAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナント内の請求情報", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBilling) error {
				if 1 != len(r.Data.Reports) {
					return fmt.Errorf("請求レポートの数が違います (want: %d, got: %d)", 1, len(r.Data.Reports))
				}
				if competitionID != r.Data.Reports[0].CompetitionID {
					return fmt.Errorf("対象の大会のIDが違います (want: %s, got: %s)", competitionID, r.Data.Reports[0].CompetitionID)
				}
				// score登録者 rankingアクセスあり: 100 yen x 1 player
				// score未登録者 rankingアクセスあり:  10 yen x 1 player
				if r.Data.Reports[0].PlayerCount != int64(len(score)) {
					return fmt.Errorf("大会の参加者数が違います competitionID: %s (want: %d, got: %d)", competitionID, len(score), r.Data.Reports[0].PlayerCount)
				}
				if r.Data.Reports[0].VisitorCount != 1 {
					return fmt.Errorf("大会の閲覧者数が違います competitionID: %s (want: %d, got: %d)", competitionID, 1, r.Data.Reports[0].VisitorCount)
				}
				if r.Data.Reports[0].BillingPlayerYen != int64(len(score)*100) {
					return fmt.Errorf("大会の請求金額内訳(参加者分)が違います competitionID: %s (want: %d, got: %d)", competitionID, 100, r.Data.Reports[0].BillingPlayerYen)
				}
				if r.Data.Reports[0].BillingVisitorYen != 10 {
					return fmt.Errorf("大会の請求金額内訳(閲覧者)が違います competitionID: %s (want: %d, got: %d)", competitionID, 10, r.Data.Reports[0].BillingVisitorYen)
				}
				billingYen := int64((100 * len(score)) + (10 * 1))
				if billingYen != r.Data.Reports[0].BillingYen {
					return fmt.Errorf("大会の請求金額合計が違います competitionID: %s (want: %d, got: %d)", competitionID, billingYen, r.Data.Reports[0].BillingYen)
				}

				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
		// NOTE: 不正リクエストチェックなし
	}

	// 大会一覧取得(player API)
	{
		res, err, txt := GetPlayerCompetitionsAction(ctx, playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("テナント内の大会情報取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				if 1 != len(r.Data.Competitions) {
					return fmt.Errorf("テナントに含まれる大会の数が違います (want: %d, got: %d)", 1, len(r.Data.Competitions))
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
		// NOTE: 不正なリクエストチェックはなし
	}

	// 大会一覧取得(organizer API)
	{
		res, err, txt := GetOrganizerCompetitionsAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("主催者API テナント内の大会一覧取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				if 1 != len(r.Data.Competitions) {
					return fmt.Errorf("テナントに含まれる大会の数が違います (want: %d, got: %d)", 1, len(r.Data.Competitions))
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	{
		// ページング無しで今回操作したテナントが含まれていることを確認
		res, err, txt := GetAdminTenantsBillingAction(ctx, "", adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("テナント別の請求ダッシュボード(最大10件)", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				// 初期データがあるので上限ま取ってこれる
				if 10 != len(r.Data.Tenants) {
					return fmt.Errorf("請求ダッシュボードの結果の数が違います (want: %d, got: %d)", len(r.Data.Tenants), 10)
				}
				tenantNameMap := make(map[string]struct{})
				for _, tenant := range r.Data.Tenants {
					tenantNameMap[tenant.DisplayName] = struct{}{}
				}
				if _, ok := tenantNameMap[tenantDisplayName]; !ok {
					return fmt.Errorf("請求ダッシュボードの結果に作成したテナントがありません")
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	{
		// ページングで初期データ範囲のBillingが正しいか確認
		checkTenantCursor := int64(randomRange([]int{20, 99})) // ID=2~99のどれかのテナントでチェック
		res, err, txt := GetAdminTenantsBillingAction(ctx, fmt.Sprintf("%d", checkTenantCursor), adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("テナント別の請求ダッシュボード: 初期データチェック", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				if 10 != len(r.Data.Tenants) {
					return fmt.Errorf("請求ダッシュボードの結果の数が違います (want: %d, got: %d)", len(r.Data.Tenants), 10)
				}
				tenantIDs := []int64{}
				for _, tenant := range r.Data.Tenants {
					// 初期データと照らし合わせてbillingが合っているか確認
					index, err := strconv.ParseInt(tenant.ID, 10, 64)
					if err != nil {
						return fmt.Errorf("TenantIDの形が違います tenantName:%v (got: %v)", tenant.Name, tenant.ID)
					}
					tenantIDs = append(tenantIDs, index)
					initialTenant, ok := sc.InitialDataTenant[index]
					if !ok {
						return fmt.Errorf("初期データに存在しないTenantIDです tenantName:%v (got: %v)", tenant.Name, tenant.ID)
					}
					if tenant.BillingYen != initialTenant.Billing {
						return fmt.Errorf("Billingの結果が違います tenantName:%v (want: %v got: %v)", tenant.Name, initialTenant.Billing, tenant.BillingYen)
					}
					AdminLogger.Printf("success, %v", tenant)
				}
				sort.Slice(tenantIDs, func(i, j int) bool { return tenantIDs[i] < tenantIDs[j] })
				if tenantIDs[0] != int64(checkTenantCursor-10) || tenantIDs[len(tenantIDs)-1] != int64(checkTenantCursor-1) {
					return fmt.Errorf("取得したテナントIDの範囲が違います (want: %v~%v got: %v~%v)",
						checkTenantCursor-10, checkTenantCursor-1, tenantIDs[0], tenantIDs[len(tenantIDs)-1],
					)
				}

				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// テナント内の大会毎のBillingが正しいことを確認
	{
		checkTenantCursor := int64(randomRange([]int{2, 99})) // ID=2~99のどれかのテナントでチェック
		initDataTenant := sc.InitialDataTenant[checkTenantCursor]
		_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, initDataTenant.TenantName, "organizer")
		if err != nil {
			return err
		}
		res, err, txt := GetOrganizerBillingAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナント内の請求情報", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBilling) error {
				if len(initDataTenant.Competitions) != len(r.Data.Reports) {
					return fmt.Errorf("請求レポートの数が違います (want: %d, got: %d)", len(initDataTenant.Competitions), len(r.Data.Reports))
				}
				reportCompMap := map[string]isuports.BillingReport{}
				for _, r := range r.Data.Reports {
					reportCompMap[r.CompetitionID] = r
				}
				for _, comp := range initDataTenant.Competitions {
					reportComp, ok := reportCompMap[comp.ID]
					if !ok {
						return fmt.Errorf("対象の大会がありません tenantName:%v (want: %v)", initDataTenant.TenantName, comp.ID)
					}
					// TODO: 対応するか悩み中 InitialDataの参加者数がわからない
					// score登録者 rankingアクセスあり: 100 yen x 1 player
					// score未登録者 rankingアクセスあり:  10 yen x 1 player
					// if r.Data.Reports[0].PlayerCount != int64(len(score)) {
					// 	return fmt.Errorf("大会の参加者数が違います competitionID: %s (want: %d, got: %d)", competitionID, len(score), r.Data.Reports[0].PlayerCount)
					// }
					// if r.Data.Reports[0].VisitorCount != 1 {
					// 	return fmt.Errorf("大会の閲覧者数が違います competitionID: %s (want: %d, got: %d)", competitionID, 1, r.Data.Reports[0].VisitorCount)
					// }
					// if r.Data.Reports[0].BillingPlayerYen != int64(len(score)*100) {
					// 	return fmt.Errorf("大会の請求金額内訳(参加者分)が違います competitionID: %s (want: %d, got: %d)", competitionID, 100, r.Data.Reports[0].BillingPlayerYen)
					// }
					// if r.Data.Reports[0].BillingVisitorYen != 10 {
					// 	return fmt.Errorf("大会の請求金額内訳(閲覧者)が違います competitionID: %s (want: %d, got: %d)", competitionID, 10, r.Data.Reports[0].BillingVisitorYen)
					// }
					if comp.Billing != reportComp.BillingYen {
						return fmt.Errorf("大会の請求金額合計が違います tenantName:%v competitionID: %v (want: %v, got: %v)", initDataTenant.TenantName, comp.ID, comp.Billing, reportComp.BillingYen)
					}
					AdminLogger.Printf("success: %v", comp)
				}

				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
		// NOTE: 不正リクエストチェックなし
	}
	return nil
}

// ランキングの結果が最大100件なことを確認
func rankingCheck(ctx context.Context, sc *Scenario, step *isucandar.BenchmarkStep) error {
	tenantName := "rankingcheck-tenantid"
	tenantDisplayName := "rankingCheck-Tenantname"
	competitionName := "ranking_check_competition"
	var competitionID string

	// SaaS管理者のagent作成
	adminAc, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	// テナント追加
	{
		res, err, txt := PostAdminTenantsAddAction(ctx, tenantName, tenantDisplayName, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("新規テナント作成", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				if tenantDisplayName != r.Data.Tenant.DisplayName {
					return fmt.Errorf("作成したテナントのDisplayNameが違います (want: %s, got: %s)", tenantDisplayName, r.Data.Tenant.DisplayName)
				}
				if tenantName != r.Data.Tenant.Name {
					return fmt.Errorf("作成したテナントのNameが違います (want: %s, got: %s)", tenantName, r.Data.Tenant.Name)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenantName, "organizer")
	if err != nil {
		return err
	}

	// 大会を作成
	{
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, competitionName, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("新規大会追加)", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				competitionID = r.Data.Competition.ID
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// プレイヤーを101人追加
	var pIDs []string
	{
		var names []string
		for i := 0; i < 101; i++ {
			names = append(names, fmt.Sprintf("ranking_check_%d", i))
		}

		res, err, txt := PostOrganizerPlayersAddAction(ctx, names, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナントへプレイヤー101人追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				for _, pl := range r.Data.Players {
					pIDs = append(pIDs, pl.ID)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return &v
		}
	}
	playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, pIDs[0])
	if err != nil {
		return err
	}

	// スコアを101人登録
	var rankingCheckScore ScoreRows
	{
		for i, playerID := range pIDs {
			rankingCheckScore = append(rankingCheckScore, &ScoreRow{
				PlayerID: playerID,
				Score:    100 + i,
			})
		}
		csv := rankingCheckScore.CSV()
		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, competitionID, []byte(csv), orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("大会結果CSV入稿", step, res, err, msg,
			WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				if r.Data.Rows != int64(len(rankingCheckScore)) {
					return fmt.Errorf("大会結果CSV入稿レスポンスのRowsが異なります (want: %d, got: %d)", len(rankingCheckScore), r.Data.Rows)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return &v
		}
	}

	// 結果を引く
	{
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, "", playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("大会内のランキング取得: ページングなし,上限100件", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				if 100 != len(r.Data.Ranks) {
					return fmt.Errorf("大会のランキングの結果の最大は100件である必要があります (want: %d, got: %d)", 100, len(r.Data.Ranks))
				}
				return nil
			}),
		)
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return &v
		}
	}

	// playerDetailsを取ってスコアを確認
	// 別スコアを入稿

	// 結果を引く
	// {
	// 	res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, "", playerAg)
	// 	msg := fmt.Sprintf("%s %s", playerAc, txt)
	// 	v := ValidateResponseWithMsg("大会内のランキング取得: 結果が更新されている", step, res, err, fmt.Sprintf("%s %s competitionID:%s", msg, playerAc, competitionID), WithStatusCode(200),
	// 		WithSuccessResponse(
	// 			func(r ResponseAPICompetitionRanking) error {
	// 				if len(r.Data.Ranks) != 0 && len(rankingCheckScore) != len(r.Data.Ranks) {
	// 					return fmt.Errorf("大会のランキングの結果が違います (want: %d, got: %d)", len(rankingCheckScore), len(r.Data.Ranks))
	// 				}
	// 				if r.Data.Ranks[0].PlayerID != rankingCheckScore[0].PlayerID {
	// 					return fmt.Errorf("大会のランキングのPlayerIDが違います (want: %v, got: %v)", r.Data.Ranks[0].PlayerID, rankingCheckScore[0].PlayerID)
	// 				}
	// 				if r.Data.Ranks[0].Score != int64(rankingCheckScore[0].Score) {
	// 					return fmt.Errorf("大会のランキングのスコアが違います (want: %v, got: %v)", r.Data.Ranks[0].Score, rankingCheckScore[0].Score)
	// 				}
	// 				return nil
	// 			}),
	// 	)
	// 	if !v.IsEmpty() {
	// 		return &v
	// 	}
	// }

	// playerDetailsを取ってスコアが変わっていることを確認する

	return nil
}

// 不正リクエストチェック
func badRequestCheck(ctx context.Context, sc *Scenario, step *isucandar.BenchmarkStep) error {
	msg := "badRequestCheck"
	tenantName := "badrequest-tenantid"
	tenantDisplayName := "badRequestCheck-Tenantname"

	// SaaS管理者のagent作成
	_, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	// SaaS管理API
	{
		res, err, txt := PostAdminTenantsAddAction(ctx, tenantName, tenantDisplayName, adminAg)
		_ = txt
		v := ValidateResponseWithMsg("新規テナント作成", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				if tenantDisplayName != r.Data.Tenant.DisplayName {
					return fmt.Errorf("作成したテナントのDisplayNameが違います (want: %s, got: %s)", tenantDisplayName, r.Data.Tenant.DisplayName)
				}
				if tenantName != r.Data.Tenant.Name {
					return fmt.Errorf("作成したテナントのNameが違います (want: %s, got: %s)", tenantName, r.Data.Tenant.Name)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenantName, "organizer")
	if err != nil {
		return err
	}

	// 必要なデータ作成
	// プレイヤー追加
	var playerIDs []string
	var playerDisplayNames []string
	for i := 0; i < playerNum; i++ {
		playerDisplayNames = append(playerDisplayNames, fmt.Sprintf("validate_player%d", i))
	}
	{
		res, err, txt := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		_ = txt
		v := ValidateResponseWithMsg("テナントへプレイヤー追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				if playerNum != len(r.Data.Players) {
					return fmt.Errorf("追加されたプレイヤー数が違います (want: %d, got: %d)", playerNum, len(r.Data.Players))
				}
				for _, pl := range r.Data.Players {
					playerIDs = append(playerIDs, pl.ID)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// テナント追加 不正リクエストチェック
	{
		invalidNames := map[string]int{
			"valid-tenantid":   http.StatusBadRequest, // 重複するname
			"INVALID_TENANTID": http.StatusBadRequest, // 不正なname
		}
		for name, code := range invalidNames {
			res, err, txt := PostAdminTenantsAddAction(ctx, name, fmt.Sprintf("name_%s", name), adminAg)
			_ = txt
			v := ValidateResponseWithMsg("新規テナント作成 不正リクエスト", step, res, err, msg, WithStatusCode(code))
			if !v.IsEmpty() {
				return v
			}
		}
	}

	// 不正リクエスト: 存在しないプレイヤーを失格にする
	{
		res, err, txt := PostOrganizerApiPlayerDisqualifiedAction(ctx, notExistID, orgAg)
		_ = txt
		v := ValidateResponseWithMsg("プレイヤーを失格にする: 不正リクエスト(存在しないプレイヤー)", step, res, err, msg, WithStatusCode(404))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 大会作成(正常リクエスト)
	competitionTitle := "badrequest_check_competition"
	var competitionID string
	{
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, competitionTitle, orgAg)
		_ = txt
		v := ValidateResponseWithMsg("新規大会追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				if competitionTitle != r.Data.Competition.Title {
					return fmt.Errorf("追加された大会の名前が違います (want: %s, got: %s)", competitionTitle, r.Data.Competition.Title)
				}
				if r.Data.Competition.IsFinished {
					return fmt.Errorf("新規追加された大会は開催中である必要があります competition.title: %s, competition.id: %s", r.Data.Competition.Title, r.Data.Competition.ID)
				}
				competitionID = r.Data.Competition.ID
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// スコア入稿 不正リクエストチェック
	{
		// 存在しない大会
		csv := ScoreRows{}.CSV()
		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, notExistID, []byte(csv), orgAg)
		_ = txt
		v := ValidateResponseWithMsg("大会結果CSV入稿: 不正リクエスト(存在しない大会)", step, res, err, msg, WithStatusCode(404))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}

		// 存在しないプレイヤーが含まれるCSVを入稿
		invalidScore := ScoreRows{&ScoreRow{
			PlayerID: notExistID,
			Score:    1,
		}}

		invalidCSV := invalidScore.CSV()
		res, err, txt = PostOrganizerCompetitionScoreAction(ctx, competitionID, []byte(invalidCSV), orgAg)
		_ = txt
		v = ValidateResponseWithMsg("大会結果CSV入稿: 不正リクエスト(存在しないプレイヤー)", step, res, err, msg, WithStatusCode(400))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}

		// カラムの並び順が逆のCSVを入稿
		invalidCSV = "score,player_id\n1,invalid_csv"
		res, err, txt = PostOrganizerCompetitionScoreAction(ctx, competitionID, []byte(invalidCSV), orgAg)
		_ = txt
		v = ValidateResponseWithMsg("大会結果CSV入稿: 不正リクエスト(カラムの並び順が違う)", step, res, err, msg, WithStatusCode(400))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}

		// 余計なカラムがあるCSVを入稿
		invalidCSV = "score,player_id,superfluity\n1,invalid_csv,dasoku"
		res, err, txt = PostOrganizerCompetitionScoreAction(ctx, competitionID, []byte(invalidCSV), orgAg)
		_ = txt
		v = ValidateResponseWithMsg("大会結果CSV入稿: 不正リクエスト(余計なカラムがあるCSV)", step, res, err, msg, WithStatusCode(400))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	_, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerIDs[0])
	if err != nil {
		return err
	}

	// 不正リクエストチェック
	// 存在しないプレイヤー
	{
		res, err, txt := GetPlayerAction(ctx, notExistID, playerAg)
		_ = txt
		v := ValidateResponseWithMsg("プレイヤーと戦績情報取得", step, res, err, msg, WithStatusCode(404))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 不正リクエストチェック
	// 存在しない大会
	{
		res, err, txt := GetPlayerCompetitionRankingAction(ctx, notExistID, "", playerAg)
		_ = txt
		v := ValidateResponseWithMsg("大会内のランキング取得", step, res, err, msg, WithStatusCode(404))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 失格にする（正常リクエスト）
	{
		idx := disqualifiedPlayerIndex
		res, err, txt := PostOrganizerApiPlayerDisqualifiedAction(ctx, playerIDs[idx], orgAg)
		_ = txt
		v := ValidateResponseWithMsg("プレイヤーを失格にする", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				if !r.Data.Player.IsDisqualified {
					return fmt.Errorf("プレイヤーが失格になっていません player.id: %s", r.Data.Player.ID)
				}
				if playerIDs[idx] != r.Data.Player.ID {
					return fmt.Errorf("失格にしたプレイヤーが違います (want: %s, got: %s)", playerIDs[idx], r.Data.Player.ID)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 失格者がランキングを参照しようとする
	{
		idx := disqualifiedPlayerIndex
		_, disqualifiedPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerIDs[idx])
		if err != nil {
			return err
		}

		res, err, txt := GetPlayerCompetitionRankingAction(ctx, competitionID, "", disqualifiedPlayerAg)
		_ = txt
		v := ValidateResponseWithMsg("大会内のランキング取得: 失格済みプレイヤー", step, res, err, msg, WithStatusCode(403))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}
	// 不正リクエストチェック
	// 存在しない大会
	{
		res, err, txt := PostOrganizerCompetitionFinishAction(ctx, notExistID, orgAg)
		_ = txt
		v := ValidateResponseWithMsg("大会終了: 不正リクエスト(存在しない大会)", step, res, err, msg, WithStatusCode(404))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 大会を終了する (正常リクエスト)
	{
		res, err, txt := PostOrganizerCompetitionFinishAction(ctx, competitionID, orgAg)
		_ = txt
		v := ValidateResponseWithMsg("大会終了", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r // responseは空
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// 不正リクエストチェック 終了済みの大会へスコアを入稿する
	{
		csv := ScoreRows{}.CSV()
		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, competitionID, []byte(csv), orgAg)
		_ = txt
		v := ValidateResponseWithMsg("大会結果CSV入稿: 不正リクエスト(終了済みの大会)", step, res, err, msg, WithStatusCode(400))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	return nil
}

// 不正リクエスト 無効なJWT
func invalidJWTCheck(ctx context.Context, sc *Scenario, step *isucandar.BenchmarkStep) error {
	msg := "invalidJWTCheck"
	tenantName := "invalid-jwt-tenant"
	tenantDisplayName := "valid-Tenantname"

	// SaaS管理者のagent作成
	adminAc, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}
	// SaaS管理API
	{
		res, err, txt := PostAdminTenantsAddAction(ctx, tenantName, tenantDisplayName, adminAg)
		_ = txt
		v := ValidateResponseWithMsg("新規テナント作成", step, res, err, fmt.Sprintf("%s %s", msg, adminAc), WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				if tenantDisplayName != r.Data.Tenant.DisplayName {
					return fmt.Errorf("作成したテナントのDisplayNameが違います (want: %s, got: %s)", tenantDisplayName, r.Data.Tenant.DisplayName)
				}
				if tenantName != r.Data.Tenant.Name {
					return fmt.Errorf("作成したテナントのNameが違います (want: %s, got: %s)", tenantName, r.Data.Tenant.Name)
				}
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	// exp切れ
	{
		ac := &Account{
			Role:       AccountRoleAdmin,
			TenantName: "admin",
			PlayerID:   "admin",
			Option:     sc.Option,
		}
		if err := ac.SetJWT(sc.RawKey, false); err != nil {
			return err
		}
		invalidAdminAg, err := ac.GetAgent()
		if err != nil {
			return err
		}

		res, err, txt := PostAdminTenantsAddAction(ctx, tenantName, "invalid_JWT_tenant_add", invalidAdminAg)
		_ = txt
		v := ValidateResponseWithMsg("新規テナント作成: 不正リクエスト(exp切れのJWT)", step, res, err, msg, WithStatusCode(401))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 存在しないテナント
	{
		_, invalidOrgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, notExistName, "organizer")
		if err != nil {
			return err
		}
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, notExistName, invalidOrgAg)
		_ = txt
		v := ValidateResponseWithMsg("新規大会追加: 不正リクエスト(存在しないテナント)", step, res, err, msg, WithStatusCode(401))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}

	// 存在しないプレイヤー
	{
		_, invalidPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, notExistID)
		if err != nil {
			return err
		}
		res, err, txt := GetPlayerCompetitionsAction(ctx, invalidPlayerAg)
		_ = txt
		v := ValidateResponseWithMsg("テナント内の大会情報取得: 不正なリクエスト(存在しないプレイヤー)", step, res, err, msg, WithStatusCode(401))
		if !v.IsEmpty() && sc.Option.StrictPrepare {
			return v
		}
	}
	return nil
}
