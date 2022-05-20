package bench

import (
	"context"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (s *Scenario) NormalWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		s.NormalScenario(ctx, step)
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

func (s *Scenario) NormalScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("normal")
	defer report()

	// TODO: シナリオが書かれる

	return nil
}
