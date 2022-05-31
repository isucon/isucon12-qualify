package bench

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/isucon/isucandar/agent"
)

// TODO: ユーザーの挙動みたいなのがここに入る、たぶん
const (
	AccountRoleAdmin      = "admin"
	AccountRoleOrganizer  = "organizer"
	AccountRoleCompetitor = "competitor"
)

// TODO: 一旦何が必要かまだわからないのでAccount、いずれ分離したりするかも
type Account struct {
	mu    sync.RWMutex
	Agent *agent.Agent
	// Agent.httpClient.Jarにjwtが入るのでログイン叩くのは考えなくて大丈夫っぽ
	// もしベンチマーカーで生成する話になったら考えるけどset-cookieすればよさそうだ
	Role    string
	baseURL string // {admin,tenant} endpoint
}

func (ac *Account) SetJWT(sub, aud string) error {
	if ac.Agent == nil {
		return fmt.Errorf("Account.Agent is nil")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "isu",
		"sub": sub,
		"aud": aud,
		"exp": time.Now().Add(time.Hour * 24),
	})

	// Sign and get the complete encoded token as a string using the secret
	var hmacSampleSecret []byte
	tokenString, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		return err
	}
	_ = tokenString // TODO: ac.Agentに埋める
	return nil
}

func (ac *Account) GetAgent(opt Option) (*agent.Agent, error) {
	ac.mu.RLock()
	ag := ac.Agent
	ac.mu.RUnlock()
	if ag != nil {
		return ag, nil
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()

	ag, err := opt.NewAgent(false)
	if err != nil {
		return nil, err
	}
	ac.Agent = ag
	return ag, nil
}

type Tenant struct {
	// `id` BIGINT UNSIGNED NOT NULL,
	// `identifier` VARCHAR(191) NOT NULL,
	// `name` VARCHAR(191) NOT NULL,
	// `image` LONGBLOB NOT NULL,
}
type Tenants []*Tenant

type Competition struct {
	// `id` INTEGER NOT NULL PRIMARY KEY,
	// `title` TEXT NOT NULL,
}
type Competitions []*Competition

type Competitor struct {
	// `id` INTEGER PRIMARY KEY,
	// `identifier` TEXT NOT NULL UNIQUE,
	// `name` TEXT NOT NULL,
}
type Competitors []*Competitor
