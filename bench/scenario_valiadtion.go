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

	ContestantLogger.Println("[ValidationScenario] 整合性チェックを開始します")
	defer ContestantLogger.Printf("[ValidationScenario] 整合性チェックを終了します")

	ag, _ := s.Option.NewAgent(false)

	// TODO: 検証シナリオがココに書かれる
	{
		res, err := DummyAction(ctx, ag)
		v := ValidateResponse("ダミー", step, res, err, WithStatusCode(200))
		if !v.IsEmpty() {
			return v
		}
	}

	return nil
}
