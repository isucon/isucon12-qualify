package bench

import (
	"context"
	"math/rand"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
)

func (sc *Scenario) ExistingTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32, isHeavyTenant bool) (*worker.Worker, error) {
	var scTag ScenarioTag
	if isHeavyTenant {
		scTag = "ExistingTenantScenario_HevaryTenant"
	} else {
		scTag = "ExistingTenantScenario"
	}

	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		if err := sc.ExistingTenantScenario(ctx, step, scTag, isHeavyTenant); err != nil {
			sc.ScenarioError(scTag, err)
			time.Sleep(SleepOnError)
		}
	},
		// // 無限回繰り返す
		worker.WithInfinityLoop(),
		worker.WithUnlimitedParallelism(),
	)
	if err != nil {
		return nil, err
	}
	w.SetParallelism(p)
	return w, nil
}

func (sc *Scenario) ExistingTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep, scTag ScenarioTag, isHeavyTenant bool) error {
	report := timeReporter("既存テナントシナリオ")
	defer report()

	// isHeavyTenantに応じて重いデータかそれ以外を引く
	var tenantName string
	if isHeavyTenant {
		tenantName = "isucon"
	} else {
		var data *InitialDataRow
		for {
			data = sc.InitialData.Choise()
			if data.TenantName != "isucon" {
				break
			}
		}
		tenantName = data.TenantName
	}
	sc.ScenarioStart(scTag)

	organizer := Account{
		Role:       AccountRoleOrganizer,
		TenantName: tenantName,
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

	// Player作成、大会作成、スコア入稿、大会終了、billing閲覧
	playerNum := 100
	players := make(map[string]*PlayerData, playerNum)
	{
		playerDisplayNames := make([]string, playerNum)
		for i := 0; i < playerNum; i++ {
			playerDisplayNames = append(playerDisplayNames, data.RandomString(16))
		}

		res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
				for _, pl := range r.Data.Players {
					players[pl.DisplayName] = &PlayerData{
						ID:          pl.ID,
						DisplayName: pl.DisplayName,
					}
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerPlayersAdd, scTag)
		} else {
			return v
		}
	}

	var competitionID string
	{
		res, err := PostOrganizerCompetitionsAddAction(ctx, data.RandomString(16), orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				competitionID = r.Data.Competition.ID
				return nil
			}))

		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, scTag)
		} else {
			return v
		}
	}

	{
		var score ScoreRows
		for _, player := range players {
			score = append(score, &ScoreRow{
				PlayerID: player.ID,
				Score:    rand.Int() % 1000,
			})
		}
		csv := score.CSV()
		res, err := PostOrganizerCompetitionResultAction(ctx, competitionID, []byte(csv), orgAg)
		v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
				_ = r // responseは空
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionResult, scTag)
		} else {
			return v
		}
	}

	{
		res, err := PostOrganizerCompetitionFinishAction(ctx, competitionID, orgAg)
		v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
				_ = r // responseは空
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionFinish, scTag)
		} else {
			return v
		}
	}

	// テナント請求ダッシュボードの閲覧
	// NOTE: playerのrankingアクセスがないのでvisit_historyを見に行くようなものは上で作ったものに関して影響がほぼない.初期データで作成された分のみ
	{
		res, err := GetOrganizerBillingAction(ctx, orgAg)
		v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIBilling) error {
				// TODO: 簡単に内容チェック
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScoreGETOrganizerBilling, scTag)
		} else {
			return v
		}
	}

	return nil
}
