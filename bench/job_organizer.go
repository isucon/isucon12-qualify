package bench

import (
	"context"
	"math/rand"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucon12-qualify/data"
)

type OrganizerJobConfig struct {
	tenantName        string // 対象テナント
	addPlayerNum      int    // 一度に追加する合計プレイヤー数
	addPlayerTimes    int    // 追加する回数
	rankingRequestNum int    // 1人あたりのランキングを確認する回数, 初期実装:75
}

func (sc *Scenario) OrganizerJob(ctx context.Context, step *isucandar.BenchmarkStep, orgAg *agent.Agent, scTag ScenarioTag, conf *OrganizerJobConfig) error {
	for {
		// TODO: 一つのテナントに対して大会を2,3個くらい同時開催するのを想定してもいいのではないか
		// 参加者登録 addPlayerNum * addPlayerTimes
		players := make(map[string]*PlayerData, conf.addPlayerNum*conf.addPlayerTimes)
		for times := 0; times < conf.addPlayerTimes; times++ {
			playerDisplayNames := make([]string, conf.addPlayerNum)
			for i := 0; i < conf.addPlayerNum; i++ {
				playerDisplayNames = append(playerDisplayNames, data.RandomString(16))
			}
			res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
			v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
					for _, pl := range r.Data.Players {
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
				return v
			}
		}

		// 大会を1つ作成し、プレイヤーが登録し、リザルトを確認し続ける
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

		// 大会のランキングを参照するプレイヤーたち
		for loopCount := 0; loopCount < conf.rankingRequestNum; loopCount++ {
			var err error
			var ve ValidationError
			var ok bool
			for _, player := range players {
				err = sc.tenantPlayerJob(ctx, step, &tenantPlayerJobConfig{
					tenantName:    conf.tenantName,
					playerID:      player.ID,
					competitionID: comp.ID,
				})
				if err != nil {
					// ctxが終了のエラーでなければ何らかのエラー
					if ve, ok = err.(ValidationError); ok && !ve.Canceled {
						return err
					}
					break
				}
			}
			// ctx終了で抜けてきた場合はloop終了
			if err != nil && ve.Canceled {
				break
			}
		}

		// 大会結果入稿
		// TODO: 大きくしていく
		var score ScoreRows
		for _, player := range players {
			// 巨大CSV入稿
			// データ量かさ増し用の無効なデータ
			for i := 0; i < 1; i++ {
				score = append(score, &ScoreRow{
					PlayerID: player.ID,
					Score:    1,
				})
			}

			score = append(score, &ScoreRow{
				PlayerID: player.ID,
				Score:    rand.Intn(1000),
			})
		}
		csv := score.CSV()
		{
			res, err := PostOrganizerCompetitionResultAction(ctx, comp.ID, []byte(csv), orgAg)
			v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
					_ = r
					return nil
				}))
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionResult, scTag)
			} else {
				return v
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

type tenantPlayerJobConfig struct {
	tenantName    string // 対象テナント
	playerID      string // 対象プレイヤー
	competitionID string // 対象大会
}

func (sc *Scenario) tenantPlayerJob(ctx context.Context, step *isucandar.BenchmarkStep, conf *tenantPlayerJobConfig) error {
	scTag := ScenarioTagOrganizerNewTenant
	_, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, conf.tenantName, conf.playerID)
	if err != nil {
		return err
	}

	{
		res, err := GetPlayerAction(ctx, conf.playerID, playerAg)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, conf.competitionID, "", playerAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionsAction(ctx, playerAg)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else {
			return v
		}
	}

	return nil
}
