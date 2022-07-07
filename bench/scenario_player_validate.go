package bench

import (
	"context"
	"math/rand"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

type playerValidateScenarioWorker struct {
	worker *worker.Worker
}

func (playerValidateScenarioWorker) String() string {
	return "PlayerValidateScenarioWorker"
}
func (w *playerValidateScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

// PlayerHandlerが不正な値を返さないかチェックする
func (sc *Scenario) PlayerValidateScenarioWorker(step *isucandar.BenchmarkStep, p int32) (Worker, error) {
	scTag := ScenarioTagPlayerValidate

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.PlayerValidateScenario(ctx, step, scTag); err != nil {
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
	return &playerValidateScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) PlayerValidateScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	// TODO: score入稿するので破壊シナリオから選ばないとダメ、直す
	initialData := sc.InitialData.Choise()
	_, playerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, initialData.TenantName, initialData.PlayerID)
	if err != nil {
		return err
	}

	// player handlerを舐める（cacheさせる）
	// score入稿
	// なめ直して更新されていることを確認

	var competitions []isuports.CompetitionDetail
	for {
		res, err, txt := GetPlayerCompetitionsAction(ctx, playerAg)
		_ = txt
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

		// NOTE: worker発火直後はcompetitionsが無いので登録されるまで待つ
		if len(competitions) != 0 {
			break
		}
		sleepms := 500 + rand.Intn(500)
		SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
	}

	for i := 0; i < ConstPlayerScenarioCompetitionLoopCount; i++ {
		// 大会を一つ選ぶ
		compIndex := rand.Intn(len(competitions))
		comp := competitions[compIndex]
		playerIDs := []string{}

		{
			res, err, txt := GetPlayerCompetitionRankingAction(ctx, comp.ID, "", playerAg)
			_ = txt
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

		// 大会参加者を何人か見る
		playerCount := rand.Intn(ConstPlayerScenarioMaxPlayerCount)
		for j := 0; j < playerCount; j++ {
			playerIndex := rand.Intn(len(playerIDs))
			res, err, txt := GetPlayerAction(ctx, playerIDs[playerIndex], playerAg)
			_ = txt
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
			sleepms := rand.Intn(5000)
			SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))
		}
	}

	return nil
}
