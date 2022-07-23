package bench

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucon12-qualify/data"
	"golang.org/x/sync/errgroup"
)

type OrganizerJobConfig struct {
	orgAc              *Account
	scTag              ScenarioTag
	tenantName         string // 対象テナント
	scoreRepeat        int
	scoreInterval      int // スコアCSVを入稿するインターバル
	addScoreNum        int // 一度の再投稿時に増えるスコアの数
	playerWorkerNum    int // CSV入稿と同時にrankingを取るplayer worker数
	newPlayerWorkerNum int // 新規に建てられる永続PlayerWorker数
	maxScoredPlayer    int // id=1等の巨大テナントの場合は全プレイヤーにスコアを与えると重いので上限をつける 0=上限なし
}

type OrganizerJobResult struct {
	ScoredPlayerNum int
}

// 大会を作成, スコアを増やしながら入れる, 確定する
func (sc *Scenario) OrganizerJob(ctx context.Context, step *isucandar.BenchmarkStep, conf *OrganizerJobConfig) (*OrganizerJobResult, error) {
	orgAg, err := conf.orgAc.GetAgent()
	if err != nil {
		return nil, err
	}

	// 大会を1つ作成し、スコアを入稿し、Closeする
	comp := &CompetitionData{
		Title: data.FakeCompetitionName(),
	}

	// player一覧を取る
	players := make(map[string]*PlayerData)
	playerIDs := []string{}
	qualifyPlayerIDs := []string{}
	{
		res, err, txt := GetOrganizerPlayersListAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", conf.orgAc, txt)
		v := ValidateResponseWithMsg("テナントのプレイヤー一覧取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for i, player := range r.Data.Players {
					// 扱うプレイヤー数の上限
					if conf.maxScoredPlayer != 0 && conf.maxScoredPlayer <= i {
						break
					}

					playerIDs = append(playerIDs, player.ID)
					if !player.IsDisqualified {
						qualifyPlayerIDs = append(qualifyPlayerIDs, player.ID)
					}
					players[player.ID] = &PlayerData{
						ID:          player.ID,
						DisplayName: player.DisplayName,
					}
				}
				if len(playerIDs) == 0 || len(qualifyPlayerIDs) == 0 {
					return fmt.Errorf("テナントにプレイヤーがいません")
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerPlayersList, conf.scTag)
		} else if v.Canceled {
			return nil, v
		} else {
			sc.AddErrorCount()
			return nil, v
		}
	}

	{
		res, err, txt := PostOrganizerCompetitionsAddAction(ctx, comp.Title, orgAg)
		msg := fmt.Sprintf("%s %s", conf.orgAc, txt)
		v := ValidateResponseWithMsg("新規大会追加", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				comp.ID = r.Data.Competition.ID
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, conf.scTag)
		} else if v.Canceled {
			return nil, v
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return nil, v
		}
		sc.CompetitionAddLog.Printf("大会「%s」を作成しました。参加者が増えます。", comp.Title)
	}

	scoredPlayerIDs := []string{}
	// 大会結果入稿
	// 全員スコアが1件ある状態がスタート
	var score ScoreRows
	for _, player := range players {
		score = append(score, &ScoreRow{
			PlayerID: player.ID,
			Score:    rand.Intn(1000),
		})
	}
	scoredPlayerIDs = score.PlayerIDs()
	eg := errgroup.Group{}
	mu := sync.Mutex{}

	doneCh := make(chan struct{})
	for i := 0; i < conf.playerWorkerNum; i++ {
		eg.Go(func() error {
			idx := rand.Intn(len(qualifyPlayerIDs))
			playerID := qualifyPlayerIDs[idx]
			playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, conf.tenantName, playerID)
			if err != nil {
				return err
			}

			isScoreDone := false
			for {
				select {
				case <-doneCh:
					isScoreDone = true
				default:
				}

				res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", playerAg)
				msg := fmt.Sprintf("%s %s", playerAc, txt)
				v := ValidateResponseWithMsg("大会内のランキング取得", step, res, err, msg, WithStatusCode(200),
					WithSuccessResponse(func(r ResponseAPICompetitionRanking) error {
						mu.Lock()
						defer mu.Unlock()
						for _, rank := range r.Data.Ranks {
							playerIDs = append(playerIDs, rank.PlayerID)
						}
						return nil
					}),
				)
				if v.IsEmpty() {
					sc.AddScoreByScenario(step, ScoreGETPlayerRanking, conf.scTag)
				} else if v.Canceled {
					return nil
				} else {
					sc.AddErrorCount()
					return v
				}

				if isScoreDone {
					break
				}
				duration := 1000 + rand.Intn(1000)
				SleepWithCtx(ctx, time.Millisecond*time.Duration(duration))
			}
			return nil
		})
	}

	eg.Go(func() error {
		defer close(doneCh)
		for count := 0; count < conf.scoreRepeat; count++ {
			mu.Lock()
			for i := 0; i < conf.addScoreNum; i++ {
				index := rand.Intn(len(playerIDs))
				player := players[playerIDs[index]]
				score = append(score, &ScoreRow{
					PlayerID: player.ID,
					Score:    rand.Intn(1000),
				})
			}
			mu.Unlock()
			csv := score.CSV()
			scoredPlayerIDs = score.PlayerIDs()

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
			} else if v.Canceled {
				return nil
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
		return nil
	})

	// PlayerWorker追加
	// checkPlayerWorkerKickedがlockをとるのでbatchへ逃がすが、エラーを取りたいのでerrGroupを流用
	eg.Go(func() error {
		i := 0
		added := 0
		for _, playerID := range qualifyPlayerIDs {
			if conf.newPlayerWorkerNum < i {
				break
			}
			if sc.checkPlayerWorkerKicked(conf.tenantName, playerID) {
				continue
			}
			i++
			wkr, err := sc.PlayerScenarioWorker(step, 1, conf.tenantName, playerID)
			if err != nil {
				return err
			}
			sc.WorkerCh <- wkr
			added++
		}
		sc.PlayerAddCountAdd(added)
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
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
		} else if v.Canceled {
			return nil, v
		} else {
			sc.AddCriticalCount() // OrganizerAPI 更新系はCritical Error
			return nil, v
		}
	}

	return &OrganizerJobResult{
		ScoredPlayerNum: len(scoredPlayerIDs),
	}, nil
}
