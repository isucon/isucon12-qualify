package bench

import (
	"context"
	"math/rand"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
	"github.com/isucon/isucon12-qualify/data"
	isuports "github.com/isucon/isucon12-qualify/webapp/go"
)

var scTag = ScenarioTag("NewTenantScenario")

func (sc *Scenario) NewTenantScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		sc.NewTenantScenario(ctx, step)
	},
		// 無限回繰り返す
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
	ContestantLogger.Println("NewTenantScenario start")

	playerNum := 100 // 1テナント当たりの作成する参加者数

	admin := &Account{
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

	tenants := []*isuports.TenantRow{
		data.CreateTenant(false),
	}
	for _, tenant := range tenants {
		res, err := PostAdminTenantsAddAction(ctx, tenant.Name, tenant.DisplayName, adminAg)
		v := ValidateResponse("新規テナント作成", step, res, err, WithStatusCode(200),
			WithSuccessResponse(func(r ResponseAPITenantsAdd) error {
				return nil
			}),
		)
		if v.IsEmpty() {
			sc.AddScoreByScenario(step, ScorePOSTAdminTenantsAdd, scTag)
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
			sc.AddScoreByScenario(step, ScoreGETAdminTenantsBilling, scTag)
		} else {
			return v
		}
	}

	// 複数作ったうちの1つに負荷をかける
	tenant := tenants[0]
	organizer := Account{
		Role:       AccountRoleOrganizer,
		TenantName: tenant.Name,
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

	// 参加者登録 x N
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

	//  大会の作成 x N
	competitionNum := 10
	var comps []*CompetitionData
	for i := 0; i < competitionNum; i++ {
		comps = append(comps, &CompetitionData{
			Title: data.RandomString(24),
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
			sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionsAdd, scTag)
		} else {
			return v
		}

		// 大会のランキングを参照するプレイヤーたち
		var i = 0
		for _, player := range players {
			i++
			if 10 < i {
				break
			}
			if err := sc.tenantPlayerScenario(ctx, step, &tenantPlayerScenarioData{
				tenantName:    tenantName,
				playerName:    player.Name,
				competitionID: comp.ID,
			}); err != nil {
				return err
			}
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
				sc.AddScoreByScenario(step, ScorePOSTOrganizerCompetitionResult, scTag)
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

	ContestantLogger.Println("NewTenantScenario end")
	return nil
}

type tenantPlayerScenarioData struct {
	tenantName    string
	playerName    string
	competitionID int64
}

func (sc *Scenario) tenantPlayerScenario(ctx context.Context, step *isucandar.BenchmarkStep, data *tenantPlayerScenarioData) error {
	player := Account{
		Role:       AccountRolePlayer,
		TenantName: data.tenantName,
		PlayerName: data.playerName,
		Option:     sc.Option,
	}
	if err := player.SetJWT(sc.RawKey); err != nil {
		return err
	}
	playerAg, err := player.GetAgent()
	if err != nil {
		return err
	}

	{
		res, err := GetPlayerAction(ctx, data.playerName, playerAg)
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
		res, err := GetPlayerCompetitionRankingAction(ctx, data.competitionID, 1, playerAg)
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
