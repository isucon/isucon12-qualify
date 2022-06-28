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
			time.Sleep(SleepOnError)
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
			return v
		}
	}

	for i := 0; i < 100; i++ {
		if rand.Intn(100) < 30 {
			res, err := GetPlayerAction(ctx, playerID, playerAg)
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
		} else {
			for _, comp := range competitions {
				{
					res, err := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", playerAg)
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
			}
		}
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(1000)))

	}
	return nil
}
