package bench

const (
	ConstMaxError         = 30
	ConstMaxCriticalError = 10

	// NewTenantScenario

	// PopularTenantScenario

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
	// NOTE: 初期データ範囲
	// 1: ISUコングロマリット(巨大テナント)
	ConstPopularTenantScenarioIDRange        = []int{1, 29}  // 破壊的変更NGで
	ConstValidateScenarioAdminBillingIDRange = []int{12, 29} // 整合性チェックで利用
	ConstAdminBillingValidateScenarioIDRange = []int{30, 69} // 大会追加OK
	ConstPlayerValidateScenarioIDRange       = []int{70, 99} // 破壊的変更OK
)
