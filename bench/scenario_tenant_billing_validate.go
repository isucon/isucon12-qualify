package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

type tenantBillingValidateWorker struct {
	worker *worker.Worker
}

func (tenantBillingValidateWorker) String() string {
	return "TenantBillingValidateWorker"
}
func (w *tenantBillingValidateWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

// 3回までリトライOK
func (sc *Scenario) TenantBillingValidateWorker(step *isucandar.BenchmarkStep, p int32) (*tenantBillingValidateWorker, error) {
	scTag := ScenarioTagTenantBillingValidate
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.TenantBillingValidate(ctx, step); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
		}
	},
		worker.WithLoopCount(3),
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

func (sc *Scenario) TenantBillingValidate(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("テナント請求検証シナリオ")
	defer report()
	scTag := ScenarioTagTenantBillingValidate
	sc.ScenarioStart(scTag)

	adminAc, adminAg, err := sc.GetAccountAndAgent(AccountRoleAdmin, "admin", "admin")
	if err != nil {
		return err
	}

	tenant := data.CreateTenant(data.TenantTagGeneral)
	{
		res, err, txt := PostAdminTenantsAddAction(ctx, tenant.Name, tenant.DisplayName, adminAg)
		msg := fmt.Sprintf("%s %s", adminAc, txt)
		v := ValidateResponseWithMsg("新規テナント作成", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTAdminTenantsAdd, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.Name, "organizer")
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
		res, err, txt := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("参加者追加", step, res, err, msg, WithStatusCode(200),
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
		} else if v.Canceled {
			return nil
		} else {
			sc.AddCriticalCount()
			return v
		}
	}

	checkReports := map[string]isuports.BillingReport{}

	for {
		// 大会作成
		comp := &CompetitionData{
			Title: data.FakeCompetitionName(),
		}
		{
			res, err, txt := PostOrganizerCompetitionsAddAction(ctx, comp.Title, orgAg)
			msg := fmt.Sprintf("%s %s", orgAc, txt)
			v := ValidateResponseWithMsg("新規大会追加", step, res, err, msg, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
					comp.ID = r.Data.Competition.ID
					return nil
				}))

			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, scTag)
			} else if v.Canceled {
				return nil
			} else {
				sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
				return v
			}
			sc.CompetitionAddLog.Printf("大会「%s」を作成しました", comp.Title)
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
			res, err, txt := PostOrganizerCompetitionScoreAction(ctx, comp.ID, []byte(csv), orgAg)
			msg := fmt.Sprintf("%s %s", orgAc, txt)
			v := ValidateResponseWithMsg("大会結果CSV入稿", step, res, err, msg, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
					if r.Data.Rows != int64(len(scores)) {
						return fmt.Errorf("大会結果CSV入稿レスポンスのRowsが異なります (want: %d, got: %d)", len(scores), r.Data.Rows)
					}
					return nil
				}),
			)
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionScore, scTag)
			} else if v.Canceled {
				return nil
			} else {
				sc.AddCriticalCount()
				return v
			}
		}

		// visitor ranking参照
		// for _, playerID := range append(scoredPlayers, visitors...) {
		for _, playerID := range visitors {
			playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.Name, playerID)
			if err != nil {
				return err
			}

			res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", playerAg)
			msg := fmt.Sprintf("%s %s", playerAc, txt)
			v := ValidateResponseWithMsg("大会内のランキング取得", step, res, err, msg, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
					for _, rank := range r.Data.Ranks {
						playerIDs = append(playerIDs, rank.PlayerID)
					}
					return nil
				}),
			)
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
			} else if v.Canceled {
				return nil
			} else {
				sc.AddErrorCount()
				return v
			}
		}

		checkReports[comp.ID] = isuports.BillingReport{
			CompetitionID:     comp.ID,
			CompetitionTitle:  comp.Title,
			PlayerCount:       int64(len(scoredPlayers)),
			VisitorCount:      int64(len(visitors)),
			BillingPlayerYen:  int64(scoredPlayersBilling),
			BillingVisitorYen: int64(visitorsBilling),
			BillingYen:        int64(scoredPlayersBilling + visitorsBilling),
		}

		// 大会終了
		{
			res, err, txt := PostOrganizerCompetitionFinishAction(ctx, comp.ID, orgAg)
			msg := fmt.Sprintf("%s %s", orgAc, txt)
			v := ValidateResponseWithMsg("大会終了", step, res, err, msg, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
					_ = r
					return nil
				}))

			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionFinish, scTag)
			} else if v.Canceled {
				return nil
			} else {
				sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
				return v
			}
		}

		// 3秒の猶予がある
		SleepWithCtx(ctx, time.Second*3)

		res, err, txt := GetOrganizerBillingAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナント内の請求情報", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBilling) error {
				if len(checkReports) != len(r.Data.Reports) {
					return fmt.Errorf("請求レポートの数が違います (want: %d, got: %d)", 1, len(r.Data.Reports))
				}

				competitionIDMap := map[string]isuports.BillingReport{}
				for _, report := range r.Data.Reports {
					competitionIDMap[report.CompetitionID] = report
				}
				if diff := cmp.Diff(checkReports, competitionIDMap); diff != "" {
					return fmt.Errorf("Billingの結果が違います (-want +got):\n%s", diff)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
		} else if v.Canceled {
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	return nil
}
