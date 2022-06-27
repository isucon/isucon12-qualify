package bench

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucon12-qualify/data"
)

type OrganizerJobConfig struct {
	scTag             ScenarioTag
	tenantName        string // 対象テナント
	addPlayerNum      int    // 一度に追加する合計プレイヤー数
	addPlayerTimes    int    // 追加する回数
	rankingRequestNum int    // 1人あたりのランキングを確認する回数, 初期実装:75
}

func (sc *Scenario) OrganizerJob(ctx context.Context, step *isucandar.BenchmarkStep, orgAg *agent.Agent, scTag ScenarioTag, conf *OrganizerJobConfig) error {
	for {
		// TODO: 一つのテナントに対して大会を2,3個くらい同時開催するのを想定してもいいのではないか
		// 参加者登録 addPlayerNum * addPlayerTimes
		AdminLogger.Printf("Playerを追加します tenant: %s players: %d", conf.tenantName, conf.addPlayerNum*conf.addPlayerTimes)
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
		wg := sync.WaitGroup{}
		for _, player := range players {
			wg.Add(1)
			go func(player *PlayerData) {
				defer wg.Done()

				_, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, conf.tenantName, player.ID)
				if err != nil {
					AdminLogger.Println(scTag, err)
					return
				}

				var ve ValidationError
				var ok bool
				for loopCount := 0; loopCount < conf.rankingRequestNum; loopCount++ {
					err := sc.tenantPlayerJob(ctx, step, &tenantPlayerJobConfig{
						playerAgent:   playerAg,
						scTag:         conf.scTag,
						tenantName:    conf.tenantName,
						playerID:      player.ID,
						competitionID: comp.ID,
					})
					if err != nil {
						// ctxが終了のエラーでなければ何らかのエラー
						if ve, ok = err.(ValidationError); ok && !ve.Canceled {
							AdminLogger.Println(scTag, err)
							return
						}
						break
					}
					time.Sleep(time.Millisecond * 200)
				}
				AdminLogger.Printf("Playerリクエストおわりplayer: %s %d回", player.ID, conf.rankingRequestNum)
				return
			}(player)
		}

		// 大会結果入稿
		// TODO: 大きくしていく
		wg.Add(1)
		go func() {
			defer wg.Done()
			var score ScoreRows

			for count := 0; count < 30; count++ {
				for _, player := range players {
					// CSV入稿
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
						AdminLogger.Println("unexpected error: ", scTag, v)
						return
					}
					break
				}
			}
		}()

		wg.Wait()

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
	playerAgent   *agent.Agent
	scTag         ScenarioTag
	tenantName    string // 対象テナント
	playerID      string // 対象プレイヤー
	competitionID string // 対象大会
}

func (sc *Scenario) tenantPlayerJob(ctx context.Context, step *isucandar.BenchmarkStep, conf *tenantPlayerJobConfig) error {
	{
		res, err := GetPlayerAction(ctx, conf.playerID, conf.playerAgent)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, conf.scTag)
		} else {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, conf.competitionID, "", conf.playerAgent)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, conf.scTag)
		} else {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionsAction(ctx, conf.playerAgent)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, conf.scTag)
		} else {
			return v
		}
	}

	return nil
}
