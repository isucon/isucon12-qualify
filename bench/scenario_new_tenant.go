package bench

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (sc *Scenario) NewTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		sc.NewTenantScenario(ctx, step)
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

func (sc *Scenario) NewTenantScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("新規テナント: SaaS管理者シナリオ")
	defer report()

	playerNum := 100 // 1テナント当たりの作成する参加者数

	admin := Account{
		Role:       AccountRoleAdmin,
		TenantName: "admin",
		PlayerName: "admin",
		Option:     sc.Option,
	}
	if err := admin.SetJWT(); err != nil {
		return err
	}
	adminAg, err := admin.GetAgent()
	if err != nil {
		return err
	}

	displayNames := []string{
		RandomString(16),
	}
	tenants := map[string]*TenantData{}

	for _, displayName := range displayNames {
		res, err := PostAdminTenantsAddAction(ctx, displayName, adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				tenants[displayName] = &TenantData{
					Name:        r.Data.Tenant.Name,
					DisplayName: r.Data.Tenant.DisplayName,
				}
				return nil
			}),
		)
		if v.IsEmpty() {
			step.AddScore(ScorePOSTAdminTenantsAdd)
		} else {
			return v
		}
	}

	for _, tenant := range tenants {
		res, err := GetAdminTenantsBillingAction(ctx, tenant.Name, adminAg)
		v := ValidateResponse("テナント別の請求ダッシュボード", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsBilling) error {
				_ = r
				return nil
			}),
		)
		if v.IsEmpty() {
			step.AddScore(ScoreGETAdminTenantsBilling)
		} else {
			return v
		}
	}

	// 複数作ったうちの1つに負荷をかける
	if tenant, ok := tenants[displayNames[0]]; ok {
		var tenantName string = tenant.Name
	} else {
		return fmt.Errorf("error: tenants[%s] not exist", displayNames[0])
	}

	organizer := Account{
		Role:       AccountRoleOrganizer,
		TenantName: tenantName,
		PlayerName: "organizer",
		Option:     sc.Option,
	}

	if err := organizer.SetJWT(); err != nil {
		return err
	}
	orgAg, err := organizer.GetAgent()
	if err != nil {
		return err
	}

	// 参加者登録 x N
	players := make(map[string]*PlayerData, playerNum)

	{
		playerDisplayNames := make([]string, playerNum)
		for i := 0; i < playerNum; i++ {
			playerDisplayNames = append(playerDisplayNames, RandomString(16))
		}
		res, err := PostOrganizerPlayersAddAction(ctx, playerDisplayNames, orgAg)
		v := ValidateResponse("大会参加者追加", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPIPlayersAdd) error {
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
			step.AddScore(ScorePOSTOrganizerPlayersAdd)
		} else {
			return v
		}
	}

	//  大会の作成 x N
	competitionNum := 10
	var comps []*CompetitionData
	for i := 0; i < competitionNum; i++ {
		comps = append(comps, &CompetitionData{
			Title: RandomString(24),
		})
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
			step.AddScore(ScorePOSTOrganizerCompetitionsAdd)
		} else {
			return v
		}

		// 大会結果入稿 x 1
		{
			var score ScoreRows
			for _, player := range players {
				score = append(score, &ScoreRow{
					PlayerName: player.Name,
					Score:      rand.Intn(1000),
				})
			}
			csv := score.CSV()
			res, err := PostOrganizerCompetitionResultAction(ctx, comp.ID, []byte(csv), orgAg)
			v := ValidateResponse("大会結果CSV入稿", step, res, err, WithStatusCode(200),
				WithSuccessResponse(func(r ResponseAPICompetitionResult) error {
					_ = r
					return nil
				}),
			)
			if v.IsEmpty() {
				step.AddScore(ScorePOSTOrganizerCompetitionResult)
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
					step.AddScore(ScorePOSTOrganizerPlayerDisqualified)
				} else {
					return v
				}
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
				step.AddScore(ScorePOSTOrganizerCompetitionFinish)
			} else {
				return v
			}
		}
		// TODO 結果確認

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
				step.AddScore(ScoreGETOrganizerBilling)
			} else {
				return v
			}
		}
	}

	return nil
}
