package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)

// 負荷をかけるより整合性チェックをメインにしたい
// - 失格にする前後で/player/...が403になること

func (sc *Scenario) PlayerScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.PlayerScenario(ctx, step); err != nil {
			AdminLogger.Printf("[PlayerScenario]: %v", err)
			time.Sleep(SleepOnError)
		}
	},
		// 無限回繰り返す
		worker.WithInfinityLoop(),
		// worker.WithUnlimitedParallelism(),
		// 10並列くらいを最大にする TODO: 要調整
		worker.WithMaxParallelism(10),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return w, nil
}

func (sc *Scenario) PlayerScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("大会参加者の整合性チェックシナリオ")
	defer report()
	scTag := ScenarioTag("PlayerScenario")
	// AdminLogger.Printf("%s start\n", scTag)

	// 初期データから一人選ぶ
	data := sc.InitialData.Choise()
	player := Account{
		Role:       AccountRolePlayer,
		TenantName: data.TenantName,
		PlayerID:   data.PlayerID,
		Option:     sc.Option,
	}
	if err := player.SetJWT(sc.RawKey); err != nil {
		return err
	}
	playerAg, err := player.GetAgent()
	if err != nil {
		return err
	}

	// 失格じゃないPlayerならすべて正しく閲覧できることを確認
	_, ok := sc.DisqualifiedPlayer[data.PlayerID]
	if !data.IsDisqualified && !ok {
		sc.DisqualifiedPlayer[data.PlayerID] = struct{}{}

		if err := sc.playerScenarioRequest(ctx, step, playerAg, data); err != nil {
			return err
		}

		// 失格にする
		organizer := Account{
			Role:       AccountRoleOrganizer,
			TenantName: data.TenantName,
			PlayerID:   "organizer",
			Option:     sc.Option,
		}
		if err := organizer.SetJWT(sc.RawKey); err != nil {
			return err
		}
		orgAg, err := organizer.GetAgent()
		if err != nil {
			return err
		}

		res, err := PostOrganizerApiPlayerDisqualifiedAction(ctx, data.PlayerID, orgAg)
		v := ValidateResponse("プレイヤーを失格にする", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayerDisqualified) error {
				if !r.Data.Player.IsDisqualified {
					return fmt.Errorf("プレイヤーが失格になっていません player.id: %s", r.Data.Player.ID)
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayerDisqualified, scTag)
		} else {
			return v
		}

		// 失格にした後は反映まで少し猶予をもたせる
		time.Sleep(time.Millisecond * 100)
	}

	// 失格の参加者は403 forbidden
	if err := sc.playerScenarioRequestDisqualify(ctx, step, playerAg, data); err != nil {
		return err
	}

	return nil
}

func (sc *Scenario) playerScenarioRequestDisqualify(ctx context.Context, step *isucandar.BenchmarkStep, playerAg *agent.Agent, data *InitialDataRow) error {
	scTag := ScenarioTag("PlayerScenario")
	{
		res, err := GetPlayerAction(ctx, data.PlayerID, playerAg)
		v := ValidateResponse("参加者と戦績情報取得", step, res, err, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerDetails, scTag)
		} else {
			return v
		}
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, data.CompetitionID, "", playerAg)
		v := ValidateResponse("大会内のランキング取得", step, res, err, WithStatusCode(403))
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerRanking, scTag)
		} else {
			return v
		}
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
	return nil
}

func (sc *Scenario) playerScenarioRequest(ctx context.Context, step *isucandar.BenchmarkStep, playerAg *agent.Agent, data *InitialDataRow) error {
	scTag := ScenarioTag("PlayerScenario")
	{
		res, err := GetPlayerAction(ctx, data.PlayerID, playerAg)
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
	}
	{
		res, err := GetPlayerCompetitionRankingAction(ctx, data.CompetitionID, "", playerAg)
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
	{
		res, err := GetPlayerCompetitionsAction(ctx, playerAg)
		v := ValidateResponse("テナント内の大会情報取得", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitions) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETPlayerCompetitions, scTag)
		} else {
			return v
		}
	}
	return nil
}
