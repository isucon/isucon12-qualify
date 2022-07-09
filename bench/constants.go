package bench

const (
	ConstMaxError         = 30
	ConstMaxCriticalError = 10

	// TODO: 以下ほぼすべて要調整

	// NewTenantScenario
	ConstNewTenantScenarioPlayerWorkerNum = 10 // 作成するplayer worker数

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

	ConstPopularTenantScenarioIDRange        = []int{0, 39}  // 利用する初期データのテナントID幅
	ConstAdminBillingValidateScenarioIDRange = []int{40, 69} // 利用する初期データのテナントID幅
	ConstPeacefulTenantScenarioIDRange       = []int{70, 99} // 利用する初期データのテナントID幅
)
