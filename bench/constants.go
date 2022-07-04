package bench

const (
	ConstMaxError         = 30
	ConstMaxCriticalError = 10

	// TODO: 以下ほぼすべて要調整

	// NewTenantScenario
	ConstNewTenantScenarioPlayerWorkerNum = 10 // 作成するplayer worker数

	// PopularTenantScenario
	ConstPopularTenantScenarioScoreRepeat = 2
	ConstPopularTenantScenarioAddScoreNum = 100 // 1度のスコア入稿で増える数

	// PlayerScenario
	ConstPlayerScenarioCompetitionLoopCount = 10 // 一周でいくつ大会を見るか
	ConstPlayerScenarioMaxPlayerCount       = 10 // 大会1つあたり何人のプレイヤー詳細を見るか(最大値)

	// PeacefulTenantScenario
	ConstPeacefulTenantScenarioIDRange = 20 // 破壊的シナリオを許容するtenantID幅 後ろn件

	// BillingValidateScenario
	ConstBillingValidateScenarioPlayerNum = 100
)
