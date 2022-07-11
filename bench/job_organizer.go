package bench

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucon12-qualify/data"
)

type OrganizerJobConfig struct {
	orgAc         *Account
	scTag         ScenarioTag
	tenantName    string // 対象テナント
	scoreRepeat   int
	scoreInterval int // スコアCSVを入稿するインターバル
	addScoreNum   int // 一度の再投稿時に増えるスコアの数
}

// 大会を作成, スコアを増やしながら入れる, 確定する
func (sc *Scenario) OrganizerJob(ctx context.Context, step *isucandar.BenchmarkStep, conf *OrganizerJobConfig) error {
	orgAg, err := conf.orgAc.GetAgent()
	if err != nil {
		return err
	}

	// 大会を1つ作成し、スコアを入稿し、Closeする
	comp := &CompetitionData{
		Title: data.RandomString(24),
	}

	// player一覧を取る
	players := make(map[string]*PlayerData)
	playerIDs := []string{}
	{
		res, err, txt := GetOrganizerPlayersListAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", conf.orgAc, txt)
		v := ValidateResponseWithMsg("テナントのプレイヤー一覧取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for _, player := range r.Data.Players {
					playerIDs = append(playerIDs, player.ID)
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
		ContestantLogger.Println("大会を作成します。")
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, comp.Title, orgAg)
		msg := fmt.Sprintf("%s %s", conf.orgAc, txt)
		v := ValidateResponseWithMsg("新規大会追加", step, res, err, msg, WithStatusCode(200),
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
	// 全員スコアが1件ある状態がスタート
	var score ScoreRows
	for _, player := range players {
		score = append(score, &ScoreRow{
			PlayerID: player.ID,
			Score:    rand.Intn(1000),
		})
	}

	for count := 0; count < conf.scoreRepeat; count++ {
		for i := 0; i < conf.addScoreNum; i++ {
			index := rand.Intn(len(playerIDs))
			player := players[playerIDs[index]]
			score = append(score, &ScoreRow{
				PlayerID: player.ID,
				Score:    rand.Intn(1000),
			})
		}
		csv := score.CSV()
		AdminLogger.Printf("[%s] [tenant:%s] CSV入稿 %d回目 (rows:%d, len:%d)", conf.scTag, conf.tenantName, count+1, len(score)-1, len(csv))

		res, err, txt := PostOrganizerCompetitionScoreAction(ctx, comp.ID, []byte(csv), orgAg)
		msg := fmt.Sprintf("%s %s", conf.orgAc, txt)
		v := ValidateResponseWithMsg("大会結果CSV入稿", step, res, err, msg, WithStatusCode(200),
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

		SleepWithCtx(ctx, time.Millisecond*time.Duration(conf.scoreInterval))
	}

	// 大会結果確定 x 1
	{
		res, err, txt := PostOrganizerCompetitionFinishAction(ctx, comp.ID, orgAg)
		msg := fmt.Sprintf("%s %s", conf.orgAc, txt)
		v := ValidateResponseWithMsg("大会終了", step, res, err, msg, WithStatusCode(200),
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
