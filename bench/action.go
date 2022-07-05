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
	"time"

	"github.com/isucon/isucandar/agent"
)

func PostInitializeAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/initialize", nil)
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

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
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

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func GetOrganizerPlayersListAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/organizer/players")
	if err != nil {
		return nil, err
	}
	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
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

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func PostOrganizerApiPlayerDisqualifiedAction(ctx context.Context, playerID string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/api/organizer/player/"+playerID+"/disqualified", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func PostOrganizerCompetitionsAddAction(ctx context.Context, title string, ag *agent.Agent) (*http.Response, error) {
	form := url.Values{}
	form.Set("title", title)
	req, err := ag.POST("/api/organizer/competitions/add", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func PostOrganizerCompetitionFinishAction(ctx context.Context, competitionId string, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.POST("/api/organizer/competition/"+competitionId+"/finish", nil)
	if err != nil {
		return nil, err
	}

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func PostOrganizerCompetitionScoreAction(ctx context.Context, competitionId string, csv []byte, ag *agent.Agent) (*http.Response, error) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("scores", "nandemoii")
	if err != nil {
		mw.Close()
		return nil, err
	}
	fw.Write(csv)

	mw.Close()

	req, err := ag.POST("/api/organizer/competition/"+competitionId+"/score", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func GetOrganizerBillingAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/organizer/billing")
	if err != nil {
		return nil, err
	}

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
}

func GetOrganizerCompetitionsAction(ctx context.Context, ag *agent.Agent) (*http.Response, error) {
	req, err := ag.GET("/api/organizer/competitions")
	if err != nil {
		return nil, err
	}

	return RequestWithRetry(ctx, func() (*http.Response, error) {
		return ag.Do(ctx, req)
	})
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

// 429 Too Many Requestsの場合にretry after分待ってretryする
func RequestWithRetry(ctx context.Context, fn func() (*http.Response, error)) (*http.Response, error) {
	var res *http.Response
	var err error

	for {
		res, err = fn()
		if err != nil {
			break
		}

		if res.StatusCode != http.StatusTooManyRequests {
			break
		}

		ra := res.Header.Get("retry-after")

		if len(ra) != 1 {
			err = fmt.Errorf("invalid retry-after header")
			break
		}

		var sec int
		sec, err = strconv.Atoi(string(ra[0]))
		if err != nil {
			break
		}

		if sec < 0 {
			err = fmt.Errorf("invalid retry-after header")
			break
		}

		AdminLogger.Printf("RequestWithRetry retry: %ds %v", sec, res.Request.URL.Path)
		SleepWithCtx(ctx, time.Second*time.Duration(sec))
	}
	return res, err
}
