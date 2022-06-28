package bench

import (
	"context"
	"math/rand"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucon12-qualify/data"
)

type OrganizerJobConfig struct {
	scTag      ScenarioTag
	tenantName string // 対象テナント
	players    map[string]*PlayerData
}

func (sc *Scenario) OrganizerJob(ctx context.Context, step *isucandar.BenchmarkStep, orgAg *agent.Agent, scTag ScenarioTag, conf *OrganizerJobConfig) error {
	for {
		// TODO: 一つのテナントに対して大会を2,3個くらい同時開催するのを想定してもいいのではないか
		// 大会を1つ作成し、スコアを入稿し、Closeする
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
				return v
			}
		}

		// 大会結果入稿
		// TODO: 増やし方を考える
		var score ScoreRows
		for count := 0; count < 5; count++ {
			for _, player := range conf.players {
				score = append(score, &ScoreRow{
					PlayerID: player.ID,
					Score:    rand.Intn(1000),
				})
			}
			csv := score.CSV()
			AdminLogger.Printf("CSV入稿 %d回目 len(%d)", count+1, len(csv))
			res, err := PostOrganizerCompetitionResultAction(ctx, comp.ID, []byte(csv), orgAg)
			v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
					_ = r
					return nil
				}))
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionResult, scTag)
			} else {
				if !v.Canceled {
					return v
				}
				break
			}
		}

		// 大会結果確定 x 1
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
				return v
			}
		}

		// テナント請求ダッシュボードの閲覧
		{
			res, err := GetOrganizerBillingAction(ctx, orgAg)
			v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPIBilling) error {
					_ = r
					return nil
				}))

			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
			} else {
				return v
			}
		}
	}
	return nil
}
