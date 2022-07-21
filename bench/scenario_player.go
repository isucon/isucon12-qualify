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

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.PlayerScenario(ctx, step, scTag, tenantName, playerID); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
		}
	},
		// 無限回繰り返す
		worker.WithInfinityLoop(),
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

func (sc *Scenario) PlayerScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag, tenantName, playerID string) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	playerAc, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerID)
	if err != nil {
		return err
	}

	var competitions []isuports.CompetitionDetail
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
			res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, rankAfter, playerAg)
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
			} else {
				sc.AddErrorCount()
				return v
			}
			if !rankAfterCheck || len(playerIDs) == 0 {
				break
			}
			rankAfterCheck = false // rankAfterCheckは一回だけで良い
			rankAfter = strconv.Itoa(rand.Intn(len(playerIDs) - 1))
			AdminLogger.Println("DEBUG: ranking after check", rankAfter)
		}

		if len(playerIDs) == 0 {
			sleepms := randomRange([]int{500, 1000})
			SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
			continue
		}

		// 大会参加者を何人か見る
		playerCount := rand.Intn(ConstPlayerScenarioMaxPlayerCount)
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
			} else {
				sc.AddErrorCount()
				return v
			}
			sleepms := randomRange([]int{1000, 2000})
			SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
		}
	}

	return nil
}
