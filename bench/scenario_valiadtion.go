package bench

import (
	"context"

	"github.com/isucon/isucandar"
)

// 整合性検証シナリオ
// 自分で作ったplaylistを直後に削除したりするので、並列で実行するとfavした他人のplaylistが削除されて壊れる可能性がある
// 負荷テスト中には実行してはいけない
func (s *Scenario) ValidationScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("validation")
	defer report()

	ContestantLogger.Println("整合性チェックを開始します")
	defer ContestantLogger.Printf("整合性チェックを終了します")

	ag, _ := s.Option.NewAgent(false)
	_ = ag

	return nil
}
