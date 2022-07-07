package bench

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

type peacefulTenantScenarioWorker struct {
	worker *worker.Worker
}

func (peacefulTenantScenarioWorker) String() string {
	return "PeacefulTenantScenarioWorker"
}
func (w *peacefulTenantScenarioWorker) Process(ctx context.Context) { w.worker.Process(ctx) }

func (sc *Scenario) PeacefulTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32) (Worker, error) {
	scTag := ScenarioTagOrganizerPeacefulTenant

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.PeacefulTenantScenario(ctx, step, scTag); err != nil {
			sc.ScenarioError(scTag, err)
			SleepWithCtx(ctx, SleepOnError)
		}
	},
		// // 無限回繰り返す
		worker.WithInfinityLoop(),
		worker.WithMaxParallelism(1),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return &peacefulTenantScenarioWorker{
		worker: w,
	}, nil
}

func (sc *Scenario) PeacefulTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag) error {
	report := timeReporter(string(scTag))
	defer report()
	sc.ScenarioStart(scTag)

	index := int64(randomRange(ConstPeacefulTenantScenarioIDRange))
	tenant := sc.InitialDataTenant[index]

	orgAc, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.TenantName, "organizer")
	if err != nil {
		return err
	}

	// player一覧を取る
	var playerIDs []string
	{
		res, err, txt := GetOrganizerPlayersListAction(ctx, orgAg)
		msg := fmt.Sprintf("%s %s", orgAc, txt)
		v := ValidateResponseWithMsg("テナントのプレイヤー一覧取得", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for _, player := range r.Data.Players {
					// 失格じゃないプレイヤーを列挙する
					if !player.IsDisqualified {
						playerIDs = append(playerIDs, player.ID)
					}
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerPlayersList, scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}
	n := rand.Intn(len(playerIDs) - 1)
	disqualifyPlayerID := playerIDs[n]
	checkerPlayerID := playerIDs[n+1]

	disqualifiedPlayerAc, disqualifiedPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.TenantName, disqualifyPlayerID)
	if err != nil {
		return err
	}
	checkerPlayerAc, checkerPlayerAg, err := sc.GetAccountAndAgent(AccountRolePlayer, tenant.TenantName, checkerPlayerID)
	if err != nil {
		return err
	}

	// 失格前に失格にするプレイヤーを見に行く
	{
		res, err, txt := GetPlayerAction(ctx, disqualifyPlayerID, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("プレイヤーと戦績情報取得: 失格前", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if disqualifyPlayerID != r.Data.Player.ID {
					return fmt.Errorf("参照したプレイヤー名が違います (want: %s, got: %s)", disqualifyPlayerID, r.Data.Player.ID)
				}
				if false != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います (want: %v, got: %v)", false, r.Data.Player.IsDisqualified)
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

	// プレイヤーを1人失格にする
	{
		res, err, txt := PostOrganizerApiPlayerDisqualifiedAction(ctx, disqualifyPlayerID, orgAg)
		msg := fmt.Sprintf("%s %s", disqualifiedPlayerAc, txt)
		v := ValidateResponseWithMsg("プレイヤーを失格にする", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayerDisqualified, scTag)
		} else {
			sc.AddCriticalCount() // Organizer APIの更新系はcritical
			return v
		}
	}

	// 失格プレイヤーで情報を見に行く 403
	{
		res, err, txt := GetPlayerCompetitionsAction(ctx, disqualifiedPlayerAg)
		msg := fmt.Sprintf("%s %s", disqualifiedPlayerAc, txt)
		v := ValidateResponseWithMsg("テナント内の大会情報取得:  失格済みプレイヤーは403で弾く", step, res, err, msg, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else {
			sc.AddErrorCount()
			return v
		}
	}

	// 失格プレイヤーを見に行く IsDisqualifiedが更新されていることをチェック
	{
		res, err, txt := GetPlayerAction(ctx, disqualifyPlayerID, checkerPlayerAg)
		msg := fmt.Sprintf("%s %s", checkerPlayerAc, txt)
		v := ValidateResponseWithMsg("プレイヤーと戦績情報取得: 失格済みプレイヤーを見に行く", step, res, err, msg, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayer) error {
				if disqualifyPlayerID != r.Data.Player.ID {
					return fmt.Errorf("参照したプレイヤー名が違います (want: %s, got: %s)", disqualifyPlayerID, r.Data.Player.ID)
				}
				if true != r.Data.Player.IsDisqualified {
					return fmt.Errorf("失格状態が違います (want: %v, got: %v)", true, r.Data.Player.IsDisqualified)
				}

				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else {
			sc.AddCriticalCount() // 反映されていないのはCritical
			return v
		}
	}

	// sleep 1.0s ~ 2.0s
	sleepms := 1000 + rand.Intn(1000)
	SleepWithCtx(ctx, time.Millisecond*time.Duration(sleepms))

	return nil
}
