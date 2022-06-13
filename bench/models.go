package bench

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucon12-qualify/data"
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
	mu         sync.RWMutex
	Agent      *agent.Agent
	Option     Option // SetJWT時にGetAgentをしたいのでしぶしぶ含めた
	Role       string
	TenantName string // JWTのaudience adminの場合は空（あるいは無視）
	PlayerID   string // JWTのsubject

	// Option.TargetURL: http://t.isucon.dev Role: adminなら
	// GetRequestURLは http://admin.t.isucon.dev
}

// {admin,[tenantName]}.t.isucon.dev 的なURLを組み立てる
func (ac *Account) GetRequestURL() string {
	base, _ := url.Parse(ac.Option.TargetURL) // url.Parseに失敗するTargetURLが渡されたときはprepareで落ちるので大丈夫
	var subdomain string
	switch ac.Role {
	case AccountRoleAdmin:
		subdomain = "admin"
	default:
		subdomain = ac.TenantName
	}
	return base.Scheme + "://" + subdomain + "." + base.Host
}

// SetJWT Agentがなければ作って、JWTをcookieに入れる
func (ac *Account) SetJWT(rawkey *rsa.PrivateKey) error {
	ag, err := ac.GetAgent()
	if err != nil {
		return fmt.Errorf("error GetAgent: %w", err)
	}

	token := jwt.New()
	token.Set("iss", "isuports")
	token.Set("sub", ac.PlayerID)
	token.Set("aud", ac.TenantName)
	token.Set("role", ac.Role)
	token.Set("exp", time.Now().Add(24*time.Hour).Unix())

	signedToken, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, rawkey))
	if err != nil {
		return fmt.Errorf("error jwt.Sign: %w", err)
	}

	reqURL := ac.GetRequestURL()
	path, err := url.Parse(reqURL)
	if err != nil {
		return fmt.Errorf("error url.Parse(%s): %w", reqURL, err)
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

	ag, err := ac.Option.NewAgent(ac.GetRequestURL(), false)
	if err != nil {
		return nil, err
	}
	ac.Agent = ag
	return ag, nil
}

type Model interface {
	InitialDataRow
}

func LoadFromJSONFile[T Model](jsonFile string) ([]*T, error) {
	// 引数に渡されたファイルを開く
	file, err := os.Open(jsonFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	objects := make([]*T, 0, 10000) // 大きく確保しておく
	// JSON 形式としてデコード
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&objects); err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("failed to decode json: %w", err)
		}
	}
	return objects, nil
}

func GetInitialData() (InitialDataRows, error) {
	data, err := LoadFromJSONFile[InitialDataRow]("./benchmarker.json")
	if err != nil {
		return nil, err
	}
	return data, nil
}

type InitialDataRow data.BenchmarkerSource
type InitialDataRows []*InitialDataRow

func (idrs InitialDataRows) Choise() *InitialDataRow {
	n := rand.Intn(len(idrs))
	return idrs[n]
}

type ScoreRow struct {
	PlayerID string
	Score    int
}

type ScoreRows []*ScoreRow

func (srs ScoreRows) CSV() string {
	csv := fmt.Sprintf("player_id,score")
	for _, row := range srs {
		csv += fmt.Sprintf("\n%s,%d", row.PlayerID, row.Score)
	}
	return csv
}

type CompetitionData struct {
	ID    string
	Title string
}
type PlayerData struct {
	ID          string
	Name        string
	DisplayName string
}
