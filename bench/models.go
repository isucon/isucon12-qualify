package bench

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/isucon/isucandar/agent"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	AccountRoleAdmin     = "admin"
	AccountRoleOrganizer = "organizer"
	AccountRolePlayer    = "player"
)

// TODO: 一旦何が必要かまだわからないのでAccount、いずれ分離したりするかも
type Account struct {
	mu      sync.RWMutex
	Agent   *agent.Agent
	Option  Option // SetJWT時にGetAgentをしたいのでしぶしぶ含めた
	Role    string
	BaseURL string // {admin,tenant} endpoint
}

// SetJWT Agentがなければ作って、JWTをcookieに入れる
func (ac *Account) SetJWT(sub, aud string) error {
	ag, err := ac.GetAgent()
	if err != nil {
		return fmt.Errorf("error Account.GetAgent: %w", err)
	}

	pemkey := getEnv("ISUCON_JWT_KEY", "")

	block, _ := pem.Decode([]byte(pemkey))
	rawkey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("error x509.ParsePKCS1PrivateKey: %w", err)
	}

	token := jwt.New()
	token.Set("iss", "isuports")
	token.Set("aud", aud)
	token.Set("sub", sub)
	token.Set("role", ac.Role)
	token.Set("exp", time.Now().Add(24*time.Hour).Unix())

	signedToken, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, rawkey))
	if err != nil {
		return fmt.Errorf("error jwt.Sign: %w", err)
	}

	path, err := url.Parse(ac.BaseURL)
	if err != nil {
		return fmt.Errorf("error url.Parse(%s): %w", ac.BaseURL, err)
	}
	ag.HttpClient.Jar.SetCookies(path, []*http.Cookie{
		&http.Cookie{
			Name:  "isuports_session",
			Value: string(signedToken),
		},
	})
	return nil
}

func (ac *Account) GetAgent() (*agent.Agent, error) {
	ac.mu.RLock()
	ag := ac.Agent
	ac.mu.RUnlock()
	if ag != nil {
		return ag, nil
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()

	ag, err := ac.Option.NewAgent(false)
	if err != nil {
		return nil, err
	}
	ac.Agent = ag
	return ag, nil
}

// TODO ここはwebappのgoからもってこれそう
type Tenant struct{}
type Tenants []*Tenant

type Competition struct{}
type Competitions []*Competition

type Competitor struct{}
type Competitors []*Competitor
