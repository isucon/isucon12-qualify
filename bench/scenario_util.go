package bench

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isucon/isucandar"
	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/score"
	"github.com/k0kubun/pp/v3"
)

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

func getEnv(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

// どのシナリオから加算されたスコアかをカウントしならがスコアを追加する
func (sc *Scenario) AddScoreByScenario(step *isucandar.BenchmarkStep, scoreTag score.ScoreTag, scenarioTag ScenarioTag) {
	key := fmt.Sprintf("%s", scenarioTag)
	value, ok := sc.ScenarioScoreMap.Load(key)
	if ok {
		if ptr, ok := value.(*int64); ok {
			atomic.AddInt64(ptr, ResultScoreMap[scoreTag])
		} else {
			log.Printf("error failed ScenarioScoreMap.Load type assertion: key(%s)\n", key)
		}
	} else {
		n := ResultScoreMap[scoreTag]
		sc.ScenarioScoreMap.Store(key, &n)
	}
	step.AddScore(scoreTag)
}

// シナリオ毎のスコア表示
func (sc *Scenario) PrintScenarioScoreMap() {
	ssmap := map[string]int64{}
	sc.ScenarioScoreMap.Range(func(key, value any) bool {
		tag, okKey := key.(string)
		scorePtr, okVal := value.(*int64)
		if !okKey || !okVal {
			log.Printf("error failed ScenarioScoreMap.Load type assertion: key(%s)\n", key)
			return false
		}
		scoreVal := atomic.LoadInt64(scorePtr)
		ssmap[string(tag)] = scoreVal

		return true
	})
	AdminLogger.Printf("ScenarioScoreMap: %s", pp.Sprint(ssmap))
}

// シナリオ回転数
func (sc *Scenario) ScenarioStart(scTag ScenarioTag) {
	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.ScenarioCountMutex.Lock()
		defer sc.ScenarioCountMutex.Unlock()

		if _, ok := sc.ScenarioCountMap[scTag]; !ok {
			sc.ScenarioCountMap[scTag] = []int{0, 0}
		}
		sc.ScenarioCountMap[scTag][0] += 1
	}()
}

func (sc *Scenario) ScenarioError(scTag ScenarioTag, err error) {
	// ctx.Doneで切られたなら何もしない
	if ve, ok := err.(ValidationError); ok && ve.Canceled {
		return
	}

	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.ScenarioCountMutex.Lock()
		defer sc.ScenarioCountMutex.Unlock()

		if _, ok := sc.ScenarioCountMap[scTag]; !ok {
			sc.ScenarioCountMap[scTag] = []int{0, 0}
		}
		sc.ScenarioCountMap[scTag][1] += 1
	}()
}

func (sc *Scenario) PrintScenarioCount() {
	sc.batchwg.Add(1)
	go func() {
		defer sc.batchwg.Done()
		sc.ScenarioCountMutex.Lock()
		defer sc.ScenarioCountMutex.Unlock()
		scmap := map[ScenarioTag]string{}
		for key, value := range sc.ScenarioCountMap {
			scmap[key] = fmt.Sprintf("count: %d (error: %d)", value[0], value[1])
		}
		AdminLogger.Printf("ScenarioCount: %s", pp.Sprint(scmap))
	}()
}

// Accountを作成してAccountとagent.Agentを返す
func (sc *Scenario) GetAccountAndAgent(role, tenantName, playerID string) (*Account, *agent.Agent, error) {
	ac := &Account{
		Role:       role,
		TenantName: tenantName,
		PlayerID:   playerID,
		Option:     sc.Option,
	}
	if err := ac.SetJWT(sc.RawKey, true); err != nil {
		return ac, nil, err
	}
	agent, err := ac.GetAgent()
	if err != nil {
		return ac, nil, err
	}
	return ac, agent, nil
}

func (sc *Scenario) AddCriticalCount() {
	sc.CriticalErrorCh <- struct{}{}
}

func (sc *Scenario) AddErrorCount() {
	sc.ErrorCh <- struct{}{}
}

func SleepWithCtx(ctx context.Context, sleepTime time.Duration) {
	tick := time.After(sleepTime)
	select {
	case <-ctx.Done():
	case <-tick:
	}
	return
}

// arg: []int{start, end}
// NOTE: start,endを範囲に含む (start <= n <= end)
func randomRange(rg []int) int {
	return rg[0] + rand.Intn(rg[1]-rg[0]+1)
}

type CompactLogger struct {
	logger *log.Logger
	logs   []string
	mu     sync.Mutex
	wg     sync.WaitGroup
}

func NewCompactLog(lgr *log.Logger, wg sync.WaitGroup) *CompactLogger {
	return &CompactLogger{
		logger: lgr,
		logs:   []string{},
		mu:     sync.Mutex{},
		wg:     wg,
	}
}

func (cl *CompactLogger) Printf(format string, args ...any) {
	// lockするし急ぎではないので後回し
	cl.wg.Add(1)
	go func(l string) {
		defer cl.wg.Done()
		cl.mu.Lock()
		defer cl.mu.Unlock()
		cl.logs = append(cl.logs, l)
	}(fmt.Sprintf(format, args...))
}

func (cl *CompactLogger) Log() {
	// lockするし急ぎではないので後回し
	cl.wg.Add(1)
	go func() {
		defer cl.wg.Done()
		cl.mu.Lock()
		defer cl.mu.Unlock()
		if 0 < len(cl.logs) {
			cl.logger.Printf("%s (類似のログ計%d件)", cl.logs[0], len(cl.logs))
		}
		cl.logs = []string{}
	}()
}
