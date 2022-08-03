package bench

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

type playerScenarioWorker struct {
	worker *worker.Worker
}

func (playerScenarioWorker) String() string {
	return "PlayerScenarioWorker"
}
func (w *playerScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

// competition一覧を取り、rankingを参照するプレイヤー
func (sc *Scenario) PlayerScenarioWorker(step *isucandar.BenchmarkStep, p int32, tenantName, playerID string) (Worker, error) {
	scTag := ScenarioTagPlayer
	sc.playerWorkerKick(tenantName, playerID)
	if tenantName == "isucon" {
		scTag += "HeavyTenant"
	}

	w, err := worker.NewWorker(
		func(ctx context.Context, _ int) {
			if sc.Option.Reproduce {
				if err := sc.PlayerScenarioReproduce(ctx, step, scTag, tenantName, playerID); err != nil {
					sc.ScenarioError(scTag, err)
					SleepWithCtx(ctx, SleepOnError)
				}
			} else {
				if err := sc.PlayerScenario(ctx, step, scTag, tenantName, playerID); err != nil {
					sc.ScenarioError(scTag, err)
					SleepWithCtx(ctx, SleepOnError)
				}
			}
		},
		worker.WithLoopCount(1),
		worker.WithMaxParallelism(1),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return &playerScenarioWorker{
		worker: w,
	}, nil
}

// 本来意図していた挙動版
func (sc *Scenario) PlayerScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag, tenantName, playerID string) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerID)
	if err != nil {
		return err
	}

	SlowResponseCount := 0

	for {
	RETRY_CHOOSE_COMP:
		var competitions []isuports.CompetitionDetail
		// 大会を取得する
		res, err, txt := GetPlayerCompetitionsAction(ctx, playerAg)
		msg := fmt.Sprintf("%s %s", playerAc, txt)
		v := ValidateResponseWithMsg("テナント内の大会情報取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				competitions = r.Data.Competitions
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else if v.Canceled {
			return nil
		} else if v.Canceled {
			// contextの打ち切りでloopを抜ける
			return nil
		} else {
			sc.AddErrorCount()
			return v
		}

		// 大会がなければsleepして選び直す
		if len(competitions) == 0 {
			sleepms := 500 + rand.Intn(500)
			SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
			goto RETRY_CHOOSE_COMP
		}

		for i := 0; i < ConstPlayerScenarioCompetitionLoopCount; i++ {
			// 大会を一つ選ぶ
			// 開催中の大会があれば90%でそれを選ぶ
			compIndex := -1
			if rand.Intn(100) < 90 {
				for i, comp := range competitions {
					if !comp.IsFinished {
						compIndex = i
						break
					}
				}
			}

			// なければなんでも良いので一つ選ぶ
			if compIndex < 0 {
				compIndex = rand.Intn(len(competitions))
			}
			comp := competitions[compIndex]

			// 大会ランキングを取得する
			// 20%の確率でrankAfterをつけてもう一度リクエストする
			playerIDs := []string{}
			rankAfterCheck := rand.Intn(100) < 20
			rankAfter := ""
			for {
				requestTime := time.Now()
				res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, rankAfter, playerAg)

				// 表示に1.2秒以上3回かかったら離脱する
				if (time.Millisecond * 1200) < time.Since(requestTime) {
					SlowResponseCount++
					if 3 <= SlowResponseCount {
						sc.PlayerDelCountAdd(1)
						return nil
					}
				}

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
				if !rankAfterCheck || len(playerIDs) == 0 {
					break
				}
				rankAfterCheck = false // rankAfterCheckは一回だけで良い
				rankAfter = strconv.Itoa(rand.Intn(len(playerIDs)))
				sleepms := rand.Intn(500)
				SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
			}

			// 大会参加者を何人か見る
			// 参加者がいなければ大会一覧の取得からやり直す
			if len(playerIDs) == 0 {
				sleepms := 500 + rand.Intn(500)
				SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
				goto RETRY_CHOOSE_COMP
			}

			playerCount := randomRange([]int{3, 5})
			for j := 0; j < playerCount; j++ {
				playerIndex := rand.Intn(len(playerIDs))

				res, err, txt := GetPlayerAction(ctx, playerIDs[playerIndex], playerAg)
				msg := fmt.Sprintf("%s %s", playerAc, txt)
				v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
					WithSuccessResponse(func(r ResponseAPIPlayer) error {
						_ = r
						return nil
					}),
				)
				if v.IsEmpty() {
					sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
				} else if v.Canceled {
					return nil
				} else {
					sc.AddErrorCount()
					return v
				}
				sleepms := randomRange([]int{1000, 2000})
				SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
			}
		}
	}

	return nil
}

// 以下は予選開催時の状態
func (sc *Scenario) PlayerScenarioReproduce(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag, tenantName, playerID string) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerID)
	if err != nil {
		return err
	}

	SlowResponseCount := 0

	for {
		var competitions []isuports.CompetitionDetail
		// 大会を取得する
		for {
			res, err, txt := GetPlayerCompetitionsAction(ctx, playerAg)
			msg := fmt.Sprintf("%s %s", playerAc, txt)
			v := ValidateResponseWithMsg("テナント内の大会情報取得", step, res, err, msg, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitions) error {
					competitions = r.Data.Competitions
					return nil
				}),
			)
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
			} else if v.Canceled {
				return nil
			} else if v.Canceled {
				// contextの打ち切りでloopを抜ける
				return nil
			} else {
				sc.AddErrorCount()
				return v
			}

			if len(competitions) != 0 {
				break
			}

			// NOTE: worker発火直後はcompetitionsが無いので登録されるまで待つ
			sleepms := 500 + rand.Intn(500)
			SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
		}

		for i := 0; i < ConstPlayerScenarioCompetitionLoopCount; i++ {
			// 大会を一つ選ぶ
			// 開催中の大会があれば90%でそれを選ぶ
			compIndex := -1
			if rand.Intn(100) < 90 {
				for i, comp := range competitions {
					if !comp.IsFinished {
						compIndex = i
						break
					}
				}
			}

			// なければなんでも良いので一つ選ぶ
			if compIndex < 0 {
				compIndex = rand.Intn(len(competitions))
			}
			comp := competitions[compIndex]

			playerIDs := []string{}
			rankAfterCheck := rand.Intn(100) < 20
			rankAfter := ""
			for {
				requestTime := time.Now()
				res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, rankAfter, playerAg)

				// 表示に1.2秒以上3回かかったら離脱する
				if (time.Millisecond * 1200) < time.Since(requestTime) {
					SlowResponseCount++
					if 3 <= SlowResponseCount {
						sc.PlayerDelCountAdd(1)
						return nil
					}
				}

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
				if !rankAfterCheck || len(playerIDs) == 0 {
					break
				}
				rankAfterCheck = false // rankAfterCheckは一回だけで良い
				rankAfter = strconv.Itoa(rand.Intn(len(playerIDs)))
			}

			// 大会参加者を何人か見る
			// もし参加者がいなければ大会を選び直す
			// 	NOTE: 予選後のメモ
			// 		予選開催時、考慮漏れのため「空のランキングが返ってきた場合（スコア入稿が完了していない場合）はsleep無しでranking取得を繰り返す」という挙動になっていた
			// 		そのため想定から外れ、PlayerAPIをほぼ叩かずrankingを叩いてスコアを稼ぐという解法があった。
			// 		ただし、スコア入稿の速度によってrankingを叩く回数が大きくブレるため、ベンチマークスコアのブレも大きくなる傾向にあった。
			if len(playerIDs) == 0 {
				continue
			}

			playerCount := randomRange([]int{3, 5})
			for j := 0; j < playerCount; j++ {
				playerIndex := rand.Intn(len(playerIDs))

				res, err, txt := GetPlayerAction(ctx, playerIDs[playerIndex], playerAg)
				msg := fmt.Sprintf("%s %s", playerAc, txt)
				v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
					WithSuccessResponse(func(r ResponseAPIPlayer) error {
						_ = r
						return nil
					}),
				)
				if v.IsEmpty() {
					sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
				} else if v.Canceled {
					return nil
				} else {
					sc.AddErrorCount()
					return v
				}
				sleepms := randomRange([]int{1000, 2000})
				SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
			}
		}
	}

	return nil
}
