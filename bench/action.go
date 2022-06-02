package bench

import (
	"bytes"
	"context"
	"mime/multipart"
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

func PostOrganizerPlayersAddAction(ctx context.Context, playerNames []string, tenantName string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	for _, name := range playerNames {
		form.Add("display_name", name)
	}
	req, err := ag.POST("/organizer/api/players/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func PostOrganizerApiPlayerDisqualifiedAction(ctx context.Context, playerName, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/organizer/api/player/"+playerName+"/disqualified", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionFinishAction(ctx context.Context, competitionId int64, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/organizer/api/competition/"+strconv.FormatInt(competitionId, 10)+"/finish", nil)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionResultAction(ctx context.Context, competitionId int64, csv []byte, ag *agent.Agent) (*http.Response, error) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("scores", "nandemoii")
	if err != nil {
		mw.Close()
		return nil, err
	}
	fw.Write(csv)

	mw.Close()

	req, err := ag.POST("/organizer/api/competition/"+strconv.FormatInt(competitionId, 10)+"/result", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return ag.Do(ctx, req)
}

func GetOrganizerBillingAction(ctx context.Context, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/organizer/api/billing")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerAction(ctx context.Context, playerName, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/player/" + playerName)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionRankingAction(ctx context.Context, competitionID int64, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/competition/" + strconv.FormatInt(competitionID, 10) + "/ranking")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionsAction(ctx context.Context, tenantName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/competitions")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}
