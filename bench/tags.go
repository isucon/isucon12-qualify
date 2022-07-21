package bench

import "github.com/isucon/isucandar/score"

// シナリオで発生するスコアのタグ
const (
	ScorePOSTAdminTenantsAdd             score.ScoreTag = "POST /api/admin/tenants/add"
	ScoreGETAdminTenantsBilling          score.ScoreTag = "GET /api/admin/tenants/billing"
	ScoreGETOrganizerPlayersList         score.ScoreTag = "GET /api/organizer/players/list"
	ScorePOSTOrganizerPlayersAdd         score.ScoreTag = "POST /api/organizer/players/add"
	ScorePOSTOrganizerPlayerDisqualified score.ScoreTag = "POST /api/organizer/player/:player_name/disqualified"
	ScorePOSTOrganizerCompetitionsAdd    score.ScoreTag = "POST /api/organizer/competitions/add"
	ScorePOSTOrganizerCompetitionFinish  score.ScoreTag = "POST /api/organizer/competition/:competition_id/finish"
	ScorePOSTOrganizerCompetitionScore   score.ScoreTag = "POST /api/organizer/competition/:competition_id/score"
	ScoreGETOrganizerBilling             score.ScoreTag = "GET /api/organizer/billing"
	ScoreGETOrganizerCompetitions        score.ScoreTag = "GET /api/organizer/competitions"
	ScoreGETPlayerDetails                score.ScoreTag = "GET /api/player/player/:player_name"
	ScoreGETPlayerRanking                score.ScoreTag = "GET /api/player/competition/:competition_id/ranking"
	ScoreGETPlayerCompetitions           score.ScoreTag = "GET /api/player/competitions"
)

// シナリオ分別用タグ
type ScenarioTag string

func (st ScenarioTag) String() string {
	return string(st)
}

const (
	ScenarioTagAdminBilling            ScenarioTag = "AdminBilling"
	ScenarioTagOrganizerNewTenant      ScenarioTag = "OrganizerNewTenant"
	ScenarioTagOrganizerPopularTenant  ScenarioTag = "OrganizerPopularTenant"
	ScenarioTagOrganizerPeacefulTenant ScenarioTag = "OrganizerPeacefulTenant"
	ScenarioTagPlayer                  ScenarioTag = "Player"
	ScenarioTagPlayerValidate          ScenarioTag = "PlayerValidate"
	ScenarioTagTenantBillingValidate   ScenarioTag = "TenantBillingValidate"
	ScenarioTagAdminBillingValidate    ScenarioTag = "AdminBillingValidate"
)

// ScoreTag毎の倍率
var ResultScoreMap = map[score.ScoreTag]int64{
	ScoreGETOrganizerBilling:            10,
	ScorePOSTAdminTenantsAdd:            10,
	ScorePOSTOrganizerCompetitionsAdd:   10,
	ScorePOSTOrganizerCompetitionScore:  10,
	ScorePOSTOrganizerPlayersAdd:        10,
	ScorePOSTOrganizerCompetitionFinish: 10,

	ScorePOSTOrganizerPlayerDisqualified: 1,
	ScoreGETAdminTenantsBilling:          1,
	ScoreGETOrganizerPlayersList:         1,
	ScoreGETOrganizerCompetitions:        1,
	ScoreGETPlayerDetails:                1,
	ScoreGETPlayerRanking:                1,
	ScoreGETPlayerCompetitions:           1,
}

// 各tagのリスト
var (
	ScenarioTagList = []ScenarioTag{
		ScenarioTagAdminBilling,
		ScenarioTagOrganizerNewTenant,
		ScenarioTagOrganizerPopularTenant,
		ScenarioTagOrganizerPeacefulTenant,
		ScenarioTagPlayer,
		ScenarioTagPlayerValidate,
		ScenarioTagTenantBillingValidate,
		ScenarioTagAdminBillingValidate,
	}
	ScoreTagList = []score.ScoreTag{
		ScorePOSTAdminTenantsAdd,
		ScoreGETAdminTenantsBilling,
		ScorePOSTOrganizerPlayersAdd,
		ScorePOSTOrganizerPlayerDisqualified,
		ScorePOSTOrganizerCompetitionsAdd,
		ScorePOSTOrganizerCompetitionFinish,
		ScorePOSTOrganizerCompetitionScore,
		ScoreGETOrganizerBilling,
		ScoreGETPlayerDetails,
		ScoreGETPlayerRanking,
		ScoreGETPlayerCompetitions,
	}
)
