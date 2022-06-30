package bench

import (
	"context"
	"math/rand"
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

	_, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenantName, playerID)
	if err != nil {
		return err
	}

	var competitions []isuports.CompetitionDetail
	for {
		{
			res, err := GetPlayerCompetitionsAction(ctx, playerAg)
			v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200),
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
		}

		// NOTE: worker発火直後はcompetitionsが無いので登録されるまで待つ
		if len(competitions) != 0 {
			break
		}
		sleepms := 500 + rand.Intn(500)
		SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
	}

	// 大会を一つ選ぶ
	loopCount := 10
	for i := 0; i < loopCount; i++ {
		compIndex := rand.Intn(len(competitions))
		comp := competitions[compIndex]
		playerIDs := []string{}

		{
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

		if len(playerIDs) == 0 {
			continue
		}

		// 大会参加者をn人くらい見る
		maxPlayerCount := 10
		playerCount := rand.Intn(maxPlayerCount)
		for j := 0; j < playerCount; j++ {
			playerIndex := rand.Intn(len(playerIDs))
			res, err := GetPlayerAction(ctx, playerIDs[playerIndex], playerAg)
			v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(200),
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
		}
	}

	sleepms := 1000 + rand.Intn(1000)
	SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))

	return nil
}
