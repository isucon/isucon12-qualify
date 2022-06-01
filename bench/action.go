package bench

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/isucon/isucandar/agent"
)

func PostInitializeAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/initialize", nil)
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
	form := url.Values{}
	form.Set("display_name", name)
	req, err := ag.POST("/admin/api/tenants/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func GetAdminTenantsBillingAction(ctx context.Context, beforeTenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/admin/api/tenants/billing?before=" + beforeTenantName)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerPlayersAddAction(ctx context.Context, playerName, tenantName string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("name", playerName) // TODO: bulk insertできるらしい
	req, err := ag.POST("/organizer/api/players/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = tenantName + "." + ag.BaseURL.Host // 無理やり tenant-010001.localhost:3000みたいなものを生成する

	return ag.Do(ctx, req)
}

func PostOrganizerApiPlayerDisqualifiedAction(ctx context.Context, playerName, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/organizer/api/player/"+playerName+"/disqualified", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = tenantName + "." + ag.BaseURL.Host // 無理やり tenant-010001.localhost:3000みたいなものを生成する

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitonsAddAction(ctx context.Context, title, tenantName string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("title", title)
	req, err := ag.POST("/organizer/api/competitions/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = tenantName + "." + ag.BaseURL.Host // 無理やり tenant-010001.localhost:3000みたいなものを生成する
	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionFinishAction(ctx context.Context, competitionId int64, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/organizer/api/competition/"+strconv.FormatInt(competitionId, 10)+"/finish", nil)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionResultAction(ctx context.Context, competitionId int64, ag *agent.Agent) (*http.Response, error) {
	// multipart/form-dataをあとでいれる
	req, err := ag.POST("/organizer/api/competition/"+strconv.FormatInt(competitionId, 10)+"/result", nil)
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
