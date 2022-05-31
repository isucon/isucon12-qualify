package bench

import (
	"context"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/worker"
)

func (sc *Scenario) OwnerScenarioWorker(step *isucandar.BenchmarkStep, p int32) (*worker.Worker, error) {
	w, err := worker.NewWorker(func(ctx context.Context, _ int) {
		sc.OwnerScenario(ctx, step)
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

func (sc *Scenario) OwnerScenario(ctx context.Context, step *isucandar.BenchmarkStep) error {
	report := timeReporter("主催者シナリオ")
	defer report()

	// TODO: シナリオが書かれる
	//  1テナントに主催者は1人とする
	//  大会の作成 x N
	//  以下大会ごとに繰り返し
	//      参加者登録 x N
	//      大会結果入稿 x 1
	//      参加者を失格状態にする x N
	//      テナント請求ダッシュボードの閲覧 x N
	//      大会結果確定 x 1

	//  大会の作成 x N
	comps := Competitions{
		&Competition{},
	}
	for comp := range comps {
		_ = comp
		// 参加者登録 x N
		{
			compters := Competitors{
				&Competitor{},
			}
			for c := range compters {
				_ = c
			}
		}
		// 大会結果入稿 x 1
		{
		}
		// 参加者を失格状態にする x N
		{
		}
		// テナント請求ダッシュボードの閲覧 x N
		{
		}
		// 大会結果確定 x 1
		{
		}
	}

	return nil
}
