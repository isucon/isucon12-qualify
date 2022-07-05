package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
)

type tenantBillingValidateWorker struct {
	worker *worker.Worker
}

func (tenantBillingValidateWorker) String() string {
	return "TenantBillingValidateWorker"
}
func (w *tenantBillingValidateWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

func (sc *Scenario) TenantBillingValidateWorker(step *isucandar.BenchmarkStep, p int32) (*tenantBillingValidateWorker, error) {
	scTag := ScenarioTagTenantBillingValidate
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.TenantBillingValidate(ctx, step); err != nil {
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

	return &tenantBillingValidateWorker{
		worker: w,
	}, nil
}

// TODO: 1テナントで複数大会作成する
func (sc *Scenario) TenantBillingValidate(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("テナント請求検証シナリオ")
	defer report()
	scTag := ScenarioTagTenantBillingValidate
	sc.ScenarioStart(scTag)

	_, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	tenant := data.CreateTenant(false)
	{
		res, err := PostAdminTenantsAddAction(ctx, tenant.Name, tenant.DisplayName, adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTAdminTenantsAdd, scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.Name, "organizer")
	if err != nil {
		return err
	}

	// player作成
	playerNum := ConstTenantBillingValidateScenarioPlayerNum
	players := make(map[string]*PlayerData, playerNum)
	playerIDs := []string{}
	playerDisplayNames := make([]string, playerNum)
	for i := 0; i < playerNum; i++ {
		playerDisplayNames = append(playerDisplayNames, data.RandomString(16))
	}

	{
		AdminLogger.Printf("[%s] [tenant:%s] Playerを追加します players: %d", scTag, tenant.Name, playerNum)
		res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		v := ValidateResponse("参加者追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				for _, pl := range r.Data.Players {
					playerIDs = append(playerIDs, pl.ID)
					players[pl.DisplayName] = &PlayerData{
						ID:          pl.ID,
						DisplayName: pl.DisplayName,
					}
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayersAdd, scTag)
		} else {
			sc.AddCriticalCount()
			return v
		}
	}

	// 大会作成
	comp := &CompetitionData{
		Title: data.RandomString(24),
	}
	{
		res, err := PostOrganizerCompetitionsAddAction(ctx, comp.Title, orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				comp.ID = r.Data.Competition.ID
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, scTag)
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	// Billingの内訳を作成
	// [0 < scored < visitor < noScored < playerNum]
	scoredIndex := randomRange([]int{0, playerNum - 1})
	visitorsIndex := randomRange([]int{scoredIndex, playerNum - 0})
	// noScoredIndex := playerNum

	scoredPlayers := playerIDs[:scoredIndex]         // スコア登録者
	visitors := playerIDs[scoredIndex:visitorsIndex] // スコア未登録、ランキング参照者
	// noScoredPlayers := playerIDs[visitorsIndex:noScoredIndex] // スコア未登録、ランキング未参照者

	scoredPlayersBilling := len(scoredPlayers) * 100
	visitorsBilling := len(visitors) * 10

	// scoredPlayers スコア登録
	var scores ScoreRows
	{
		for i, playerID := range scoredPlayers {
			scores = append(scores, &ScoreRow{
				PlayerID: playerID,
				Score:    100 + i,
			})
		}
		csv := scores.CSV()
		AdminLogger.Printf("[%s] [tenant:%s] CSV入稿 (rows:%d, len:%d)", scTag, tenant.Name, len(scores), len(csv))
		res, err := PostOrganizerCompetitionScoreAction(ctx, comp.ID, []byte(csv), orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err,
			WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				if r.Data.Rows != int64(len(scores)) {
					return fmt.Errorf("大会結果CSV入稿レスポンスのRowsが異なります (want: %d, got: %d)", len(scores), r.Data.Rows)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionScore, scTag)
		} else {
			sc.AddCriticalCount()
			return v
		}
	}

	// visitor ranking参照
	// for _, playerID := range append(scoredPlayers, visitors...) {
	for _, playerID := range visitors {
		_, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.Name, playerID)
		if err != nil {
			return err
		}

		res, err := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", playerAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				for _, rank := range r.Data.Ranks {
					playerIDs = append(playerIDs, rank.PlayerID)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 大会終了
	{
		res, err := PostOrganizerCompetitionFinishAction(ctx, comp.ID, orgAg)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionFinish, scTag)
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	SleepWithCtx(ctx, time.Second*3)

	res, err := GetOrganizerBillingAction(ctx, orgAg)
	v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
		WithSuccessResponse(func(r ResponseAPIBilling) error {
			if 1 != len(r.Data.Reports) {
				return fmt.Errorf("請求レポートの数が違います (want: %d, got: %d)", 1, len(r.Data.Reports))
			}

			report := r.Data.Reports[0]
			if comp.ID != report.CompetitionID {
				return fmt.Errorf("対象の大会のIDが違います (want: %s, got: %s)", comp.ID, report.CompetitionID)
			}
			// score登録者 rankingアクセスあり: 100 yen x 1 player
			// score未登録者 rankingアクセスあり:  10 yen x 1 player
			if report.PlayerCount != int64(len(scoredPlayers)) {
				return fmt.Errorf("大会の参加者数が違います competitionID: %s (want: %d, got: %d)", comp.ID, len(scoredPlayers), report.PlayerCount)
			}
			if report.VisitorCount != int64(len(visitors)) {
				return fmt.Errorf("大会の閲覧者数が違います competitionID: %s (want: %d, got: %d)", comp.ID, len(visitors), report.VisitorCount)
			}
			if report.BillingPlayerYen != int64(scoredPlayersBilling) {
				return fmt.Errorf("大会の請求金額内訳(参加者分)が違います competitionID: %s (want: %d, got: %d)", comp.ID, scoredPlayersBilling, report.BillingPlayerYen)
			}
			if report.BillingVisitorYen != int64(visitorsBilling) {
				return fmt.Errorf("大会の請求金額内訳(閲覧者)が違います competitionID: %s (want: %d, got: %d)", comp.ID, visitorsBilling, report.BillingVisitorYen)
			}
			billingYen := int64(scoredPlayersBilling + visitorsBilling)
			if billingYen != report.BillingYen {
				return fmt.Errorf("大会の請求金額合計が違います competitionID: %s (want: %d, got: %d)", comp.ID, billingYen, report.BillingYen)
			}
			return nil
		}),
	)
	if v.IsEmpty() {
		sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
	} else {
		sc.AddCriticalCount()
		return v
	}

	return nil
}
