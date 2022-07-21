package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
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

	// 初期データからテナントを選ぶ
	index := randomRange(ConstPlayerValidateScenarioIDRange)
	tenant := sc.InitialDataTenant[index]

	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.TenantName, "organizer")
	if err != nil {
		return err
	}

	playerIDs := []string{}
	disqualifiedPlayerIDs := []string{}
	{
		res, err, txt := GetOrganizerPlayersListAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナントのプレイヤー一覧取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for _, pl := range r.Data.Players {
					if pl.IsDisqualified {
						disqualifiedPlayerIDs = append(disqualifiedPlayerIDs, pl.ID)
					} else {
						playerIDs = append(playerIDs, pl.ID)
					}
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerPlayersList, scTag)
		} else {
			return v
		}
	}

	checkerPlayerID := playerIDs[0]
	disqualifyingdPlayerID := playerIDs[1]

	checkerPlayerAc, checkerPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.TenantName, checkerPlayerID)
	if err != nil {
		return err
	}

	// ScoreGETPlayer
	//		IsDisqualifyが更新されていること
	//		Scores更新
	// ScoreGETPlayerRanking
	//		ranks
	//		score
	// ScoreGETPlayerCompetitions
	//		competitions
	//			IsFinished

	// cacheさせる
	{
		pid := playerIDs[1]
		res, err, txt := GetPlayerAction(ctx, pid, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if r.Data.Player.ID != pid {
					return fmt.Errorf("PlayerIDが違います (want:%s got:%s)", pid, r.Data.Player.ID)
				}
				if false != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います playerID: %s (want %v got:%v)", pid, false, r.Data.Player.IsDisqualified)
				}
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

	// 状態を更新する
	{
		pid := disqualifyingdPlayerID
		res, err, txt := PostOrganizerApiPlayerDisqualifiedAction(ctx, pid, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("プレイヤーを失格にする", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithCacheControlPrivate(),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				if !r.Data.Player.IsDisqualified {
					return fmt.Errorf("プレイヤーが失格になっていません player.id: %s", r.Data.Player.ID)
				}
				if disqualifyingdPlayerID != r.Data.Player.ID {
					return fmt.Errorf("失格にしたプレイヤーが違います (want: %s, got: %s)", disqualifyingdPlayerID, r.Data.Player.ID)
				}
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

	// 反映されていることをチェック
	{
		pid := disqualifyingdPlayerID
		res, err, txt := GetPlayerAction(ctx, pid, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("参加者と戦績情報取得", step, res, err, msg, WithStatusCode(200),
			WithContentType("application/json"),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if r.Data.Player.ID != pid {
					return fmt.Errorf("PlayerIDが違います (want:%s got:%s)", pid, r.Data.Player.ID)
				}
				if true != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います playerID: %s (want %v got:%v)", pid, true, r.Data.Player.IsDisqualified)
				}
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
	// TODO

	SleepWithCtx(ctx, time.Second*3)

	return nil
}
