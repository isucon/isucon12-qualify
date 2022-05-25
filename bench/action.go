package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/isucon/isucandar/agent"
)

var globalPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func PostInitializeAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/initialize", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetRootAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/")
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func PostTenantsAddAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func GetTenantsBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func PostCompetititorsAddAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func PostCompetitorDisqualifiedAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func PostCompetitionsAddAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func PostCompetitionFinishAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func PostCompetitionResultAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func GetTenantBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func GetCompetitorAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func GetCompetitionRankingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}
func GetCompetitionsAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	return GetRootAction(ctx, ag)
}

func newRequestBody(obj any) (*bytes.Buffer, func(), error) {
	b := globalPool.Get().(*bytes.Buffer)
	reset := func() {
		b.Reset()
		globalPool.Put(b)
	}
	if err := json.NewEncoder(b).Encode(obj); err != nil {
		reset()
		return nil, nil, err
	}
	return b, reset, nil
}
