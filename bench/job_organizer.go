package bench

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucon12-qualify/data"
)

type OrganizerJobConfig struct {
	orgAg       *agent.Agent
	scTag       ScenarioTag
	tenantName  string // 対象テナント
	scoreRepeat int
}

// 大会を作成, スコアを増やしながら入れる, 確定する
// TODO: 一つのテナントに対して大会を2,3個くらい同時開催するのを想定してもいいのではないか
func (sc *Scenario) OrganizerJob(ctx context.Context, step *isucandar.BenchmarkStep, conf *OrganizerJobConfig) error {
	// 大会を1つ作成し、スコアを入稿し、Closeする
	comp := &CompetitionData{
		Title: data.RandomString(24),
	}

	// player一覧を取る
	players := make(map[string]*PlayerData)
	{
		res, err := GetOrganizerPlayersListAction(ctx, conf.orgAg)
		v := ValidateResponse("テナントのプレイヤー一覧取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for _, player := range r.Data.Players {
					players[player.ID] = &PlayerData{
						ID:          player.ID,
						DisplayName: player.DisplayName,
					}
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerPlayersList, conf.scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	{
		res, err := PostOrganizerCompetitionsAddAction(ctx, comp.Title, conf.orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				comp.ID = r.Data.Competition.ID
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, conf.scTag)
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	// 大会結果入稿
	// TODO: 増やし方を考える 毎度全員分スコアが増えるのはやりすぎ
	var score ScoreRows
	for count := 0; count < conf.scoreRepeat; count++ {
		for _, player := range players {
			score = append(score, &ScoreRow{
				PlayerID: player.ID,
				Score:    rand.Intn(1000),
			})
		}
		csv := score.CSV()

		AdminLogger.Printf("[%s] [tenant:%s] CSV入稿 %d回目 len(%d)", conf.scTag, conf.tenantName, count+1, len(csv))
		res, err := PostOrganizerCompetitionScoreAction(ctx, comp.ID, []byte(csv), conf.orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				_ = r
				if r.Data.Rows != int64(len(score)) {
					return fmt.Errorf("入稿したCSVの行数が正しくありません %d != %d", r.Data.Rows, len(score))
				}
				return nil
			}))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionScore, conf.scTag)
		} else {
			if v.Canceled {
				// context.Doneによって打ち切られた場合はエラーカウントしない
				return nil
			}
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	// 大会結果確定 x 1
	{
		res, err := PostOrganizerCompetitionFinishAction(ctx, comp.ID, conf.orgAg)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionFinish, conf.scTag)
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return v
		}
	}

	return nil
}
