package bench

import (
	"time"

	"github.com/isucon/isucandar/agent"
)

type Option struct {
	TargetURL                string
	RequestTimeout           time.Duration
	InitializeRequestTimeout time.Duration
	ExitErrorOnFail          bool
	Duration                 time.Duration
	PrepareOnly              bool
	SkipPrepare              bool
	DataDir                  string
	Debug                    bool
}

func (o Option) String() string {
	return "TargetURL: " + o.TargetURL + ", RequestTimeout: " + o.RequestTimeout.String() + ", InitializeRequestTimeout: " + o.InitializeRequestTimeout.String()
}

func (o Option) NewAgent(forInitialize bool) (*agent.Agent, error) {
	agentOptions := []agent.AgentOption{
		agent.WithBaseURL(o.TargetURL),
		agent.WithCloneTransport(agent.DefaultTransport),
	}

	// initialize 用の agent.Agent はタイムアウト時間が違うのでオプションを調整
	if forInitialize {
		agentOptions = append(agentOptions, agent.WithTimeout(o.InitializeRequestTimeout))
	} else {
		agentOptions = append(agentOptions, agent.WithTimeout(o.RequestTimeout))
	}

	// オプションに従って agent.Agent を生成
	return agent.NewAgent(agentOptions...)
}
