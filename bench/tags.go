package bench

import "github.com/isucon/isucandar/score"

// シナリオで発生するスコアのタグ
const (
	ScoreGETRoot score.ScoreTag = "GET /"

	// for admin endpoint
	ScorePOSTAdminTenantsAdd    score.ScoreTag = "POST /admin/api/tenants/add"
	ScoreGETAdminTenantsBilling score.ScoreTag = "GET /admin/api/tenants/billing"

	// for organizer endpoint
	// 参加者操作
	ScoreGETOrganizerPlayersList         score.ScoreTag = "GET /organizer/api/players/list"
	ScorePOSTOrganizerPlayersAdd         score.ScoreTag = "POST /organizer/api/players/add"
	ScorePOSTOrganizerPlayerDisqualified score.ScoreTag = "POST /organizer/api/player/:player_name/disqualified"
	// 大会操作
	ScorePOSTOrganizerCompetitionsAdd   score.ScoreTag = "POST /organizer/api/competitions/add"
	ScorePOSTOrganizerCompetitionFinish score.ScoreTag = "POST /organizer/api/competition/:competition_id/finish"
	ScorePOSTOrganizerCompetitionResult score.ScoreTag = "POST /organizer/api/competition/:competition_id/result"
	// テナント操作
	ScoreGETOrganizerBilling      score.ScoreTag = "GET /organizer/api/billing"
	ScoreGETOrganizerCompetitions score.ScoreTag = "GET /api/organizer/competitions"

	// for player
	// 参加者からの閲覧
	ScoreGETPlayerDetails      score.ScoreTag = "GET /player/api/player/:player_name"
	ScoreGETPlayerRanking      score.ScoreTag = "GET /player/api/competition/:competition_id/ranking"
	ScoreGETPlayerCompetitions score.ScoreTag = "GET /player/api/competitions"
)

// シナリオ分別用タグ
type ScenarioTag string

func (st ScenarioTag) String() string {
	return string(st)
}

const (
	ScenarioTagAdmin                   ScenarioTag = "Admin"
	ScenarioTagOrganizerNewTenant      ScenarioTag = "OrganizerNewTenant"
	ScenarioTagOrganizerPopularTenant  ScenarioTag = "OrganizerPopularTenant"
	ScenarioTagOrganizerPeacefulTenant ScenarioTag = "OrganizerPeacefulTenant"
	ScenarioTagPlayer                  ScenarioTag = "Player"
)

// ScoreTag毎の倍率
var ResultScoreMap = map[score.ScoreTag]int64{
	ScorePOSTAdminTenantsAdd:             1,
	ScoreGETAdminTenantsBilling:          1,
	ScorePOSTOrganizerPlayersAdd:         1,
	ScoreGETOrganizerPlayersList:         1,
	ScorePOSTOrganizerPlayerDisqualified: 1,
	ScorePOSTOrganizerCompetitionsAdd:    1,
	ScorePOSTOrganizerCompetitionFinish:  1,
	ScorePOSTOrganizerCompetitionResult:  1,
	ScoreGETOrganizerBilling:             1,
	ScoreGETOrganizerCompetitions:        1,
	ScoreGETPlayerDetails:                1,
	ScoreGETPlayerRanking:                1,
	ScoreGETPlayerCompetitions:           1,
}

// 各tagのリスト
var (
	ScenarioTagList = []ScenarioTag{
		ScenarioTagAdmin,
		ScenarioTagOrganizerNewTenant,
		ScenarioTagOrganizerPopularTenant,
		ScenarioTagOrganizerPeacefulTenant,
		ScenarioTagPlayer,
	}
	ScoreTagList = []score.ScoreTag{
		ScorePOSTAdminTenantsAdd,
		ScoreGETAdminTenantsBilling,
		ScorePOSTOrganizerPlayersAdd,
		ScorePOSTOrganizerPlayerDisqualified,
		ScorePOSTOrganizerCompetitionsAdd,
		ScorePOSTOrganizerCompetitionFinish,
		ScorePOSTOrganizerCompetitionResult,
		ScoreGETOrganizerBilling,
		ScoreGETPlayerDetails,
		ScoreGETPlayerRanking,
		ScoreGETPlayerCompetitions,
	}
)
