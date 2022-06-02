package main

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucon12-qualify/bench"
)

const (
	DefaultTargetURL                = "http://localhost"
	DefaultRequestTimeout           = time.Second * 15
	DefaultInitializeRequestTimeout = time.Second * 30
	DefaultDuration                 = time.Minute
)

func main() {
	rand.Seed(time.Now().UnixNano())

	// ベンチマークオプションの生成
	option := bench.Option{}

	// 各フラグとベンチマークオプションのフィールドを紐付ける
	flag.StringVar(&option.TargetURL, "target-url", DefaultTargetURL, "Benchmark target URL")
	flag.DurationVar(&option.RequestTimeout, "request-timeout", DefaultRequestTimeout, "Default request timeout")
	flag.DurationVar(&option.InitializeRequestTimeout, "initialize-request-timeout", DefaultInitializeRequestTimeout, "Initialize request timeout")
	flag.DurationVar(&option.Duration, "duration", DefaultDuration, "Benchmark duration")
	flag.BoolVar(&option.ExitErrorOnFail, "exit-error-on-fail", true, "Exit error on fail")
	flag.BoolVar(&option.PrepareOnly, "prepare-only", false, "Prepare only")
	flag.BoolVar(&option.SkipPrepare, "skip-prepare", false, "Skip prepare")
	flag.StringVar(&option.DataDir, "data-dir", "data", "Data directory")
	flag.BoolVar(&option.Debug, "debug", false, "Debug mode")

	// コマンドライン引数のパースを実行
	// この時点で各フィールドに値が設定されます
	flag.Parse()

	// 現在の設定を大会運営向けロガーに出力
	bench.AdminLogger.Print(option)
	bench.Debug = option.Debug

	// シナリオの生成
	scenario := &bench.Scenario{
		Option: option,
	}

	// ベンチマークの生成
	benchmark, err := isucandar.NewBenchmark(
		isucandar.WithLoadTimeout(option.Duration),
	)
	if err != nil {
		bench.ContestantLogger.Println(err)
		return
	}

	// ベンチマークにシナリオを追加
	benchmark.AddScenario(scenario)
	// TODO: add...

	// main で最上位の context.Context を生成
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ベンチマーク開始
	result := benchmark.Start(ctx)

	time.Sleep(time.Second) // 結果が揃うまでちょっと待つ

	// エラーを表示
	for i, err := range result.Errors.All() {
		// 選手向けにエラーメッセージが表示される
		bench.ContestantLogger.Printf("ERROR[%d] %v", i, err)
		if i+1 >= bench.MaxErrors {
			bench.ContestantLogger.Printf("ERRORは最大%d件まで表示しています", bench.MaxErrors)
			break
		}
		// 大会運営向けにスタックトレース付きエラーメッセージが表示される
		//		bench.AdminLogger.Printf("%+v", err)
	}

	// prepare only の場合はエラーが1件でもあればエラーで終了
	if option.PrepareOnly {
		if len(result.Errors.All()) > 0 {
			os.Exit(1)
		}
		return
	}

	// スコア表示
	score, addition, deduction := SumScore(result)
	bench.ContestantLogger.Printf("SCORE: %d (+%d %d)", score, addition, -deduction)
	bench.ContestantLogger.Printf("RESULT: %#v", result.Score.Breakdown())

	// 0点以下(fail)ならエラーで終了
	if option.ExitErrorOnFail && score <= 0 {
		os.Exit(1)
	}
}

func SumScore(result *isucandar.BenchmarkResult) (int64, int64, int64) {
	score := result.Score
	// 各タグに倍率を設定
	score.Set(bench.ScoreGETRoot, 1)
	score.Set(bench.ScorePOSTTenantsAdd, 1)
	score.Set(bench.ScoreGETTenantsBilling, 1)
	score.Set(bench.ScorePOSTCompetititorsAdd, 1)
	score.Set(bench.ScorePOSTCompetitorDisqualified, 1)
	score.Set(bench.ScorePOSTCompetitionsAdd, 1)
	score.Set(bench.ScorePOSTCompetitionFinish, 1)
	score.Set(bench.ScorePOSTCompetitionResult, 1)
	score.Set(bench.ScoreGETTenantBilling, 1)
	score.Set(bench.ScoreGETCompetitor, 1)
	score.Set(bench.ScoreGETCompetitionRanking, 1)
	score.Set(bench.ScoreGETCometitions, 1)

	// 加点分の合算
	addition := score.Sum()

	// エラーは1つ10点減点
	deduction := len(result.Errors.All()) * 10

	// 合計(0を下回ったら0点にする)
	sum := addition - int64(deduction)
	if sum < 0 {
		sum = 0
	}

	return sum, addition, int64(deduction)
}
