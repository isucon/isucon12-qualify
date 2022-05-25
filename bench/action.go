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

func PostTenantsAddAction(ctx context.Context, name string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	body, reset, err := newRequestBody(struct {
		Name string `json:"name"`
	}{
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/api/tenants/add", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetTenantsBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/api/tenants/billing")
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func PostCompetititorsAddAction(ctx context.Context, name string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	body, reset, err := newRequestBody(struct {
		Name string `json:"name"`
	}{
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/api/competitors/add", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func PostCompetitorDisqualifiedAction(ctx context.Context, competitor string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/api/competitor/"+competitor+"/disqualified", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func PostCompetitionsAddAction(ctx context.Context, title string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	body, reset, err := newRequestBody(struct {
		Title string `json:"title"`
	}{
		Title: title,
	})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/api/competitions/add", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func PostCompetitionFinishAction(ctx context.Context, competition string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/api/competition/"+competition+"/finish", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func PostCompetitionResultAction(ctx context.Context, competition string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	// multipart/form-dataをあとでいれる
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/api/competition/"+competition+"/result", body)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetTenantBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/api/tenant/billing")
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetCompetitorAction(ctx context.Context, competitor string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/api/competitor/" + competitor)
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetCompetitionRankingAction(ctx context.Context, competition string, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/api/competiton/" + competition + "/ranking")
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
}

func GetCompetitionsAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	// リクエストを生成
	req, err := ag.GET("/api/competitions/")
	if err != nil {
		return nil, err
	}

	// リクエストを実行
	return ag.Do(ctx, req)
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
