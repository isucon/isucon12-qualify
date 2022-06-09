package bench

import (
	"context"
	"fmt"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (sc *Scenario) OrganizerScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		sc.OrganizerScenario(ctx, step)
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

type CompetitionData struct {
	ID    int64
	Title string
}
type PlayerData struct {
	Name        string
	DisplayName string
}

func (sc *Scenario) OrganizerScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("新規追加: 主催者シナリオ")
	defer report()
	scTag := ScenarioTag("OrganizerScenario")

	// 各テナント
	//  大会の作成 x N
	//  以下大会ごとに繰り返し
	//      参加者登録 x N
	//      大会結果入稿 x 1
	//      参加者を失格状態にする x N
	//      大会結果確定 x 1
	//  テナント請求ダッシュボードの閲覧 x N

	// TODO: 初期データ読むまで最初にテナントここで作ってみる
	admin := Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerName: "admin",
		Option:     sc.Option,
	}
	if err := admin.SetJWT(sc.RawKey); err != nil {
		return err
	}
	adminAg, err := admin.GetAgent()
	if err != nil {
		return err
	}

	var tenantName string // TODO: 初期データから持ってくる scenario.Prepare()の中で入れる
	{
		res, err := PostAdminTenantsAddAction(ctx, "first", adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				tenantName = r.Data.Tenant.Name
				return nil
			}),
		)
		if !v.IsEmpty() {
			return v
		}
	}

	organizer := Account{
		Role:       AccountRoleOrganizer,
		TenantName: tenantName,
		PlayerName: "organizer",
		Option:     sc.Option,
	}

	if err := organizer.SetJWT(sc.RawKey); err != nil {
		return err
	}
	orgAg, err := organizer.GetAgent()
	if err != nil {
		return err
	}

	//  大会の作成 x N
	// TODO: 初期データから持ってくる
	comps := []*CompetitionData{
		&CompetitionData{
			Title: "first",
		},
		&CompetitionData{
			Title: "second",
		},
	}
	for _, comp := range comps {
		// 大会の作成
		res, err := PostOrganizerCompetitonsAddAction(ctx, comp.Title, orgAg)
		v := ValidateResponse("新規大会追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPICompetitionsAdd) error {
				comp.ID = r.Data.Competition.ID
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, scTag)
		} else {
			return v
		}

		// 参加者登録 x N
		// TODO: 初期データから持ってくる
		players := map[string]*PlayerData{
			"player 1": &PlayerData{
				DisplayName: "player 1",
			},
			"player 2": &PlayerData{
				DisplayName: "player 2",
			},
		}
		var playerDisplayNames []string
		for key, _ := range players {
			playerDisplayNames = append(playerDisplayNames, key)
		}

		{
			res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
			v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
					if len(r.Data.Players) != len(playerDisplayNames) {
						return fmt.Errorf("作成された大会参加者の数が違います got: %d expect: %d", len(r.Data.Players), len(playerDisplayNames))
					}
					for _, pl := range r.Data.Players {
						players[pl.DisplayName] = &PlayerData{
							Name:        pl.Name,
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

		// 大会結果入稿 x 1
		{
			csv := "player_name,score"
			for _, player := range players {
				csv += fmt.Sprintf("\n%s,%d", player.Name, 100)
			}
			res, err := PostOrganizerCompetitionResultAction(ctx, comp.ID, []byte(csv), orgAg)
			v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
					_ = r
					return nil
				}),
			)
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionResult, scTag)
			} else {
				return v
			}
		}

		// 参加者を失格状態にする x N
		{
			index := 0
			for _, player := range players {
				// 5%の人は失格
				index++
				if index%100 > 5 {
					continue
				}
				res, err := PostOrganizerApiPlayerDisqualifiedAction(ctx, player.Name, orgAg)
				v := ValidateResponse("参加者を失格にする", step, res, err, WithStatusCode(200),
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
		}

		// テナント請求ダッシュボードの閲覧 x 1
		{
			res, err := GetOrganizerBillingAction(ctx, orgAg)
			v := ValidateResponse("テナント内の請求情報", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPIBilling) error {
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

		// 大会結果確定 x 1
		{
			res, err := PostOrganizerCompetitionFinishAction(ctx, comp.ID, orgAg)
			v := ValidateResponse("大会終了", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionRankingFinish) error {
					_ = r
					return nil
				}),
			)
			if v.IsEmpty() {
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionFinish, scTag)
			} else {
				return v
			}
		}
	}

	return nil
}
