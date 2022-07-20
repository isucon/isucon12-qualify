package bench

const (
	ConstMaxError         = 30
	ConstMaxCriticalError = 10

	// NewTenantScenario
	ConstNewTenantScenarioPlayerWorkerNum = 20 // 作成するplayer worker数

	// PopularTenantScenario
	ConstPopularTenantScenarioScoreRepeat = 2   // 一周のスコア入稿回数
	ConstPopularTenantScenarioAddScoreNum = 100 // 1度のスコア入稿で増える数

	// PlayerScenario
	ConstPlayerScenarioCompetitionLoopCount = 10 // 一周でいくつ大会を見るか
	ConstPlayerScenarioMaxPlayerCount       = 10 // 大会1つあたり何人のプレイヤー詳細を見るか(最大値)

	// PeacefulTenantScenario

	// TenantBillingValidateScenario
	ConstTenantBillingValidateScenarioPlayerNum = 100 // TenantBilling検証用テナントのplayer数

	// AdminBillingValidateScenario
	ConstAdminBillingValidateScenarioPlayerNum = 100 // TenantBilling検証用テナントのplayer数
)

var (
	// NOTE: 初期データ範囲について(0-based)
	//       0: 巨大テナント
	//   1 ~ 39: 人気テナント
	//  40 ~ 69: AdminBilling検証用テナント
	//  70 ~ 99: 破壊的操作テナント
	// 100 ~ : prepare validate tenant

	ConstPopularTenantScenarioIDRange        = []int{1, 29}
	ConstValidateScenarioAdminBillingIDRange = []int{12, 29}
	ConstPlayerValidateScenarioIDRange       = []int{30, 49}
	ConstAdminBillingValidateScenarioIDRange = []int{50, 69}
	ConstPeacefulTenantScenarioIDRange       = []int{70, 99}
)
