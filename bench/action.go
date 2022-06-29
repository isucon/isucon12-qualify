package bench

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
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

func PostAdminTenantsAddAction(ctx context.Context, name, displayName string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("name", name)
	form.Set("display_name", displayName)
	req, err := ag.POST("/api/admin/tenants/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func GetAdminTenantsBillingAction(ctx context.Context, beforeTenantID string, ag *agent.Agent) (*http.Response, error) {
	path := "/api/admin/tenants/billing"
	if beforeTenantID != "" {
		path = fmt.Sprintf("%s?before=%s", path, beforeTenantID)
	}
	req, err := ag.GET(path)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetOrganizerPlayersListAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/organizer/players")
	if err != nil {
		return nil, err
	}
	return ag.Do(ctx, req)
}

func PostOrganizerPlayersAddAction(ctx context.Context, playerDisplayNames []string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	for _, displayName := range playerDisplayNames {
		form.Add("display_name", displayName)
	}
	req, err := ag.POST("/api/organizer/players/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func PostOrganizerApiPlayerDisqualifiedAction(ctx context.Context, playerID string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/api/organizer/player/"+playerID+"/disqualified", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionsAddAction(ctx context.Context, title string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("title", title)
	req, err := ag.POST("/api/organizer/competitions/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionFinishAction(ctx context.Context, competitionId string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/api/organizer/competition/"+competitionId+"/finish", nil)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionResultAction(ctx context.Context, competitionId string, csv []byte, ag *agent.Agent) (*http.Response, error) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("scores", "nandemoii")
	if err != nil {
		mw.Close()
		return nil, err
	}
	fw.Write(csv)

	mw.Close()

	req, err := ag.POST("/api/organizer/competition/"+competitionId+"/result", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return ag.Do(ctx, req)
}

func GetOrganizerBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/organizer/billing")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetOrganizerCompetitionsAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/organizer/competitions")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerAction(ctx context.Context, playerID string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/player/player/" + playerID)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionRankingAction(ctx context.Context, competitionID string, rankAfter string, ag *agent.Agent) (*http.Response, error) {
	path := fmt.Sprintf("/api/player/competition/%s/ranking", competitionID)
	if rankAfter != "" {
		path += fmt.Sprintf("?rank_after=%s", rankAfter)
	}
	req, err := ag.GET(path)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionsAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/player/competitions")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}
