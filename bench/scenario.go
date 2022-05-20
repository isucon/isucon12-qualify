package bench

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/failure"
	"github.com/isucon/isucandar/score"
	"github.com/isucon/isucandar/worker"
)

var (
	Debug     = false
	MaxErrors = 30
)

const (
	ErrFailedLoadJSON failure.StringCode = "load-json"
	ErrCannotNewAgent failure.StringCode = "agent"
	ErrInvalidRequest failure.StringCode = "request"
)

// シナリオで発生するスコアのタグ
const (
	ScoreGETRoot score.ScoreTag = "GET /"
	// TODO: ここにエンドポイント毎のタグが列挙される
)

// オプションと全データを持つシナリオ構造体
type Scenario struct {
	mu sync.RWMutex

	Option Option

	// TODO: シナリオを回す全データをしまう定義が列挙される

	lastPlaylistCreatedAt   time.Time
	rateGetPopularPlaylists int32

	Errors failure.Errors
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Prepare ステップで実行される
func (s *Scenario) Prepare(ctx context.Context, step *isucandar.BenchmarkStep) error {
	// Prepareは60秒以内に完了
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// GET /initialize 用ユーザーエージェントの生成
	ag, err := s.Option.NewAgent(true)
	if err != nil {
		return failure.NewError(ErrCannotNewAgent, err)
	}

	if s.Option.SkipPrepare {
		return nil
	}

	debug := Debug
	defer func() {
		Debug = debug
	}()
	Debug = true // prepareは常にデバッグログを出す

	// POST /initialize へ初期化リクエスト実行
	res, err := GetInitializeAction(ctx, ag)
	if v := ValidateResponse("初期化", step, res, err, WithStatusCode(200)); !v.IsEmpty() {
		return fmt.Errorf("初期化リクエストに失敗しました %v", v)
	}

	// 検証シナリオを1回まわす
	if err := s.ValidationScenario(ctx, step); err != nil {
		return fmt.Errorf("整合性チェックに失敗しました")
	}

	ContestantLogger.Printf("整合性チェックに成功しました")
	return nil
}

// isucandar.PrepeareScenario を満たすメソッド
// isucandar.Benchmark の Load ステップで実行される
func (s *Scenario) Load(ctx context.Context, step *isucandar.BenchmarkStep) error {
	if s.Option.PrepareOnly {
		return nil
	}
	ContestantLogger.Println("負荷テストを開始します")
	defer ContestantLogger.Println("負荷テストを終了します")
	wg := &sync.WaitGroup{}

	// 通常シナリオ
	normalCase, err := s.NormalWorker(step, 1)
	if err != nil {
		return err
	}

	workers := []*worker.Worker{
		normalCase,
	}
	for _, w := range workers {
		wg.Add(1)
		worker := w
		go func() {
			defer wg.Done()
			worker.Process(ctx)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.loadAdjustor(ctx, step, normalCase)
	}()
	wg.Wait()
	return nil
}

// TODO: これなに
func (s *Scenario) loadAdjustor(ctx context.Context, step *isucandar.BenchmarkStep, workers ...*worker.Worker) {
	tk := time.NewTicker(time.Second)
	var prevErrors int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
		errors := step.Result().Errors.Count()
		total := errors["load"]
		if total >= int64(MaxErrors) {
			ContestantLogger.Printf("負荷テストを打ち切ります (エラー数:%d)", total)
			AdminLogger.Printf("%#v", errors)
			step.Result().Score.Close()
			step.Cancel()
			return
		}
		addParallels := int32(1)
		if diff := total - prevErrors; diff > 0 {
			ContestantLogger.Printf("エラーが%d件増えました(現在%d件)", diff, total)
		} else {
			ContestantLogger.Println("ユーザーが増えます")
			addParallels = 1
		}
		for _, w := range workers {
			w.AddParallelism(addParallels)
		}
		prevErrors = total
	}
}

var nullFunc = func() {}

func timeReporter(name string) func() {
	if !Debug {
		return nullFunc
	}
	start := time.Now()
	return func() {
		AdminLogger.Printf("Scenario:%s elapsed:%s", name, time.Since(start))
	}
}
