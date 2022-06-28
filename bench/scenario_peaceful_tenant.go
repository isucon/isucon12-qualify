package bench

import (
	"context"
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
			time.Sleep(SleepOnError)
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

	// TODO: 破壊的なシナリオ用IDを考える とりあえず後ろ20件
	index := int64((len(sc.InitialDataTenant) - 20) + rand.Intn(20))
	tenant := sc.InitialDataTenant[index]

	_, orgAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.TenantName, "organizer")
	if err != nil {
		return err
	}

	// player一覧を取る
	var playerIDs []string
	{
		res, err := GetOrganizerPlayersListAction(ctx, orgAg)
		v := ValidateResponse("テナントのプレイヤー一覧取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersList) error {
				for _, player := range r.Data.Players {
					playerIDs = append(playerIDs, player.ID)
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
	playerID := playerIDs[rand.Intn(len(playerIDs))]

	// プレイヤーを1人失格にする
	{
		res, err := PostOrganizerApiPlayerDisqualifiedAction(ctx, playerID, orgAg)
		v := ValidateResponse("プレイヤーを失格にする", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayerDisqualified, scTag)
		} else {
			return v
		}
	}

	_, playerAg, err := sc.GetAccountAndAgent(AccountRoleOrganizer, tenant.TenantName, playerID)
	if err != nil {
		return err
	}

	{
		res, err := GetPlayerCompetitionsAction(ctx, playerAg)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else {
			return v
		}
	}

	// sleep 1s ~ 1.5s
	sleepms := 1000 + rand.Intn(500)
	time.Sleep(time.Millisecond * time.Duration(sleepms))

	return nil
}
