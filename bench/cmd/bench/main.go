package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
	"github.com/isucon/isucandar/score"
	"github.com/isucon/isucon12-qualify/bench"
	"github.com/k0kubun/pp/v3"

	benchrun "github.com/isucon/isucon12-portal/bench-tool.go/benchrun"
	isuxportalResources "github.com/isucon/isucon12-portal/proto.go/isuxportal/resources"
)

const (
	DefaultTargetURL                = "https://t.isucon.dev"
	DefaultRequestTimeout           = time.Second * 30
	DefaultInitializeRequestTimeout = time.Second * 30
	DefaultDuration                 = time.Minute
	DefaultLoadType                 = bench.LoadTypeDefault
	DefaultStrictPrepare            = true
)

func main() {
	rand.Seed(time.Now().UnixNano())

	// ベンチマークオプションの生成
	option := bench.Option{}

	// 各フラグとベンチマークオプションのフィールドを紐付ける
	flag.StringVar(&option.TargetURL, "target-url", DefaultTargetURL, "Benchmark target URL")
	flag.StringVar(&option.TargetAddr, "target-addr", "", "Benchmark target address e.g. host:port")
	flag.DurationVar(&option.RequestTimeout, "request-timeout", DefaultRequestTimeout, "Default request timeout")
	flag.DurationVar(&option.InitializeRequestTimeout, "initialize-request-timeout", DefaultInitializeRequestTimeout, "Initialize request timeout")
	flag.DurationVar(&option.Duration, "duration", DefaultDuration, "Benchmark duration")
	flag.BoolVar(&option.ExitErrorOnFail, "exit-error-on-fail", false, "Exit error on fail")
	flag.BoolVar(&option.PrepareOnly, "prepare-only", false, "Prepare only")
	flag.BoolVar(&option.SkipPrepare, "skip-prepare", false, "Skip prepare")
	flag.StringVar(&option.DataDir, "data-dir", "data", "Data directory")
	flag.BoolVar(&option.Debug, "debug", false, "Debug mode")
	flag.StringVar(&option.LoadType, "load-type", DefaultLoadType, fmt.Sprintf("load type [%s,%s] Default: %s", bench.LoadTypeDefault, bench.LoadTypeLight, DefaultLoadType))
	flag.BoolVar(&option.StrictPrepare, "strict-prepare", DefaultStrictPrepare, "strict prepare mode. default: true")

	// コマンドライン引数のパースを実行
	// この時点で各フィールドに値が設定されます
	flag.Parse()

	// supervisorから起動された場合はベンチ先アドレスをISUXBENCH_TARGETから読む
	if os.Getenv("ISUXBENCH_TARGET") != "" {
		option.TargetAddr = fmt.Sprintf("%s:%d", os.Getenv("ISUXBENCH_TARGET"), 443)
	}

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

	// main で最上位の context.Context を生成
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// データ生成器の初期化
	bench.InitializeData()

	// ベンチマーク開始
	result := benchmark.Start(ctx)

	time.Sleep(time.Second) // 結果が揃うまでちょっと待つ

	// エラーの原因を集計する
	unexpectedErrors := []error{}
	validateErrors := []error{}
	existFailLog := false

	errAll := result.Errors.All()
	for _, err := range errAll {
		fail := false
		isValidateError := false
		for _, errCode := range failure.GetErrorCodes(err) {
			switch errCode {
			case string(bench.ErrValidation): // validationErrorで出るもの
				isValidateError = true
			case string(bench.ErrFailedLoad), string(bench.ErrFailedPrepare): // portal上はfailを出す
				fail = true
			default: // isucandar系など
			}
		}

		if isValidateError {
			validateErrors = append(validateErrors, err)
			continue
		}
		if fail {
			existFailLog = true
			continue
		}

		// 上記のどれでも無い場合は意図しないエラー
		unexpectedErrors = append(unexpectedErrors, err)
	}

	// benchの問題でエラーが出た場合はexit 1
	if len(unexpectedErrors) != 0 {
		for _, err := range unexpectedErrors {
			log.Printf("bench unexpected error: %v\n", err)
		}
		os.Exit(1)
	}

	// エラーを表示
	for i, err := range validateErrors {
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
	scenario.PrintScenarioScoreMap()
	scenario.PrintScenarioCount()
	scenario.PrintWorkerCount()
	addition := SumScore(result)
	deduction := int64(len(validateErrors) * 10)

	score := addition - deduction
	if score < 0 {
		score = 0
	}

	isPassed := 0 < score && !existFailLog
	bench.ContestantLogger.Printf("PASSED: %v", isPassed)
	bench.ContestantLogger.Printf("SCORE: %d (+%d %d)", score, addition, -deduction)
	br := AllTagBreakdown(result)
	tags := make([]string, 0, len(br))
	for tag, score := range br {
		tags = append(tags, fmt.Sprintf("%s: %d", tag, score))
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i] < tags[j]
	})
	for _, tag := range tags {
		fmt.Println(tag)
	}
	bench.AdminLogger.Printf("%s", pp.Sprint(AllTagBreakdown(result)))

	// supervisorから起動された場合はreportを送信
	if os.Getenv("ISUXBENCH_REPORT_FD") != "" {
		mustReport(&isuxportalResources.BenchmarkResult{
			Finished: true,
			Passed:   isPassed,
			Score:    score,
			ScoreBreakdown: &isuxportalResources.BenchmarkResult_ScoreBreakdown{
				Raw:       addition,
				Deduction: deduction,
			},
			Execution: &isuxportalResources.BenchmarkResult_Execution{
				Reason: "TODO",
			},
			SurveyResponse: &isuxportalResources.SurveyResponse{
				Language: "galaxy", // TODO /initialize で取得した言語を入れる
			},
		})
	}

	// failならエラーで終了
	if option.ExitErrorOnFail && !isPassed {
		os.Exit(1)
	}
}

// 結果が0のタグを含めたbreakdownを返す
func AllTagBreakdown(result *isucandar.BenchmarkResult) score.ScoreTable {
	bd := result.Score.Breakdown()
	for _, tag := range bench.ScoreTagList {
		if _, ok := bd[tag]; !ok {
			bd[tag] = int64(0)
		}
	}
	return bd
}

func SumScore(result *isucandar.BenchmarkResult) int64 {
	score := result.Score
	// 各タグに倍率を設定
	for scoreTag, value := range bench.ResultScoreMap {
		score.Set(scoreTag, value)
	}

	return score.Sum()
}

func mustReport(res *isuxportalResources.BenchmarkResult) {
	r, err := benchrun.NewReporter(true)
	if err != nil {
		panic(err)
	}
	if err := r.Report(res); err != nil {
		panic(err)
	}
}
