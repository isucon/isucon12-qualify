package bench

import (
	"context"

	"github.com/isucon/isucandar"
)

// ベンチ実行後の整合性検証シナリオ
// isucandar.ValidateScenarioを満たすメソッド
// isucandar.Benchmark の validation ステップで実行される
func (s *Scenario) ValidationScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("validation")
	defer report()

	ContestantLogger.Println("整合性チェックを開始します")
	defer ContestantLogger.Printf("整合性チェックを終了します")

	ag, _ := s.Option.NewAgent(false)

	// TODO: 検証シナリオがココに書かれる
	DummyAction(ctx, ag)

	return nil
}
