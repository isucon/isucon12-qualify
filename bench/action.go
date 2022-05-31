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
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/initialize", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetRootAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostAdminTenantsAddAction(ctx context.Context, name string, ag *agent.Agent) (*http.Response, error) {
	body, reset, err := newRequestBody(struct {
		DisplayName string `json:"display_name"`
	}{
		DisplayName: name,
	})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/admin/api/tenants/add", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetAdminTenantsBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/admin/api/tenants/billing")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerPlayersAddAction(ctx context.Context, name string, ag *agent.Agent) (*http.Response, error) {
	body, reset, err := newRequestBody(struct {
		Name string `json:"name"`
	}{
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/organizer/api/players/add", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerApiPlayerDisqualifiedAction(ctx context.Context, player string, ag *agent.Agent) (*http.Response, error) {
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/organizer/api/player/"+player+"/disqualified", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitonsAddAction(ctx context.Context, title string, ag *agent.Agent) (*http.Response, error) {
	body, reset, err := newRequestBody(struct {
		Title string `json:"title"`
	}{
		Title: title,
	})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/organizer/api/competitions/add", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionFinishAction(ctx context.Context, competition string, ag *agent.Agent) (*http.Response, error) {
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/organizer/api/competition/"+competition+"/finish", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionResultAction(ctx context.Context, competition string, ag *agent.Agent) (*http.Response, error) {
	// multipart/form-dataをあとでいれる
	body, reset, err := newRequestBody(struct{}{})
	if err != nil {
		return nil, err
	}
	defer reset()
	req, err := ag.POST("/organizer/api/competition/"+competition+"/result", body)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetOrganizerBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/organizer/api/billing")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerAction(ctx context.Context, player string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/player/" + player)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionRankingAction(ctx context.Context, competition string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/competiton/" + competition + "/ranking")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionsAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/competitions")
	if err != nil {
		return nil, err
	}

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
