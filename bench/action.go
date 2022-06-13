package bench

import (
	"bytes"
	"context"
	"fmt"
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

func PostAdminTenantsAddAction(ctx context.Context, name, displayName string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("name", name)
	form.Set("display_name", displayName)
	req, err := ag.POST("/admin/api/tenants/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func GetAdminTenantsBillingAction(ctx context.Context, beforeTenantID string, ag *agent.Agent) (*http.Response, error) {
	path := "/admin/api/tenants/billing"
	if beforeTenantID != "" {
		path = fmt.Sprintf("%s?before=%s", path, beforeTenantID)
	}
	req, err := ag.GET(path)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func PostOrganizerPlayersAddAction(ctx context.Context, playerNames []string, ag *agent.Agent) (*http.Response, error) {
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

func PostOrganizerApiPlayerDisqualifiedAction(ctx context.Context, playerName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/organizer/api/player/"+playerName+"/disqualified", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return ag.Do(ctx, req)
}

func PostOrganizerCompetitonsAddAction(ctx context.Context, title string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("title", title)
	req, err := ag.POST("/organizer/api/competitions/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return ag.Do(ctx, req)
}

func PostOrganizerCompetitionFinishAction(ctx context.Context, competitionId int64, ag *agent.Agent) (*http.Response, error) {
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

func GetOrganizerBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/organizer/api/billing")
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerAction(ctx context.Context, playerName string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/player/api/player/" + playerName)
	if err != nil {
		return nil, err
	}

	return ag.Do(ctx, req)
}

func GetPlayerCompetitionRankingAction(ctx context.Context, competitionID int64, rankAfter int, ag *agent.Agent) (*http.Response, error) {
	path := fmt.Sprintf("/player/api/competition/%s/ranking", strconv.FormatInt(competitionID, 10))
	if rankAfter > 1 {
		path += fmt.Sprintf("?rank_after=%d", rankAfter)
	}
	req, err := ag.GET(path)
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
