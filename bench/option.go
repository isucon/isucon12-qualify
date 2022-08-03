package bench

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/isucon/isucandar/agent"
)

type Option struct {
	TargetURL                string
	TargetAddr               string
	RequestTimeout           time.Duration
	InitializeRequestTimeout time.Duration
	ExitErrorOnFail          bool
	Duration                 time.Duration
	PrepareOnly              bool
	SkipPrepare              bool
	DataDir                  string
	Debug                    bool
	StrictPrepare            bool
	Reproduce                bool
}

func (o Option) String() string {
	return fmt.Sprintf(
		"TargetURL: %s, TargetAddr: %s, RequestTimeout: %s, InitializeRequestTimeout: %s, StrictPrepare: %v, ReproduceMode: %v",
		o.TargetURL,
		o.TargetAddr,
		o.RequestTimeout.String(),
		o.InitializeRequestTimeout.String(),
		o.StrictPrepare,
		o.Reproduce,
	)
}

func (o Option) NewTransport() *http.Transport {
	if o.TargetAddr == "" {
		return agent.DefaultTransport.Clone()
	}
	dialContextFunc := func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, o.TargetAddr)
	}
	dialFunc := func(network, addr string) (net.Conn, error) {
		return dialContextFunc(context.Background(), network, addr)
	}
	trs := agent.DefaultTransport.Clone()
	trs.DialContext = dialContextFunc
	trs.Dial = dialFunc
	trs.IdleConnTimeout = 5 * time.Second // 大量にclientができるので永続接続は短め
	return trs
}

func (o Option) NewAgent(targetURL string, forInitialize bool) (*agent.Agent, error) {
	trs := o.NewTransport()
	agentOptions := []agent.AgentOption{
		agent.WithBaseURL(targetURL),
		agent.WithTransport(trs),
		agent.WithNoCache(),
	}

	// initialize 用の agent.Agent はタイムアウト時間が違うのでオプションを調整
	if forInitialize {
		agentOptions = append(agentOptions, agent.WithTimeout(o.InitializeRequestTimeout))
	} else {
		agentOptions = append(agentOptions, agent.WithTimeout(o.RequestTimeout))
	}
	// AdminLogger.Println("new agent for:", targetURL)
	// オプションに従って agent.Agent を生成
	return agent.NewAgent(agentOptions...)
}
