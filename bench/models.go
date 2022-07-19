package bench

import (
	crand "crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
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

type Account struct {
	mu         sync.RWMutex
	Agent      *agent.Agent
	Option     Option // SetJWT時にGetAgentをしたいのでしぶしぶ含めた
	Role       string
	TenantName string // JWTのaudience adminの場合は空（あるいは無視）
	PlayerID   string // JWTのsubject

	// Invalid JWT用 セットされていなければ有効な鍵がセットされる
	InvalidRSAKey  bool
	InvalidKeyArgo bool
}

func (ac *Account) String() string {
	return fmt.Sprintf("tenant:%s role:%s playerID:%s", ac.TenantName, ac.Role, ac.PlayerID)
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
func (ac *Account) SetJWT(rawkey *rsa.PrivateKey, isValidExp bool) error {
	ag, err := ac.GetAgent()
	if err != nil {
		return fmt.Errorf("error GetAgent: %w", err)
	}

	var expTime time.Time
	if isValidExp {
		expTime = time.Now().Add(time.Hour)
	} else {
		expTime = time.Now().Add(-time.Hour)
	}

	token := jwt.New()
	token.Set("iss", "isuports")
	token.Set("sub", ac.PlayerID)
	token.Set("aud", ac.TenantName)
	token.Set("role", ac.Role)
	token.Set("exp", expTime.Unix())

	signOpts := []jwt.SignOption{}
	if ac.InvalidRSAKey {
		key, err := rsa.GenerateKey(crand.Reader, 2048)
		if err != nil {
			return fmt.Errorf("rsa.GenerateKey: %s", err)
		}
		signOpts = append(signOpts, jwt.WithKey(jwa.RS256, key))
	} else if ac.InvalidKeyArgo {
		signOpts = append(signOpts, jwt.WithKey(jwa.RS512, rawkey))
	} else {
		signOpts = append(signOpts, jwt.WithKey(jwa.RS256, rawkey))
	}

	signedToken, err := jwt.Sign(token, signOpts...)
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

type LoadModel interface {
	InitialDataRow | InitialDataTenantRow
}

func LoadFromJSONFile[T LoadModel](jsonFile string) ([]*T, error) {
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

type InitialDataRow data.BenchmarkerSource
type InitialDataRows []*InitialDataRow

func GetInitialData() (InitialDataRows, error) {
	data, err := LoadFromJSONFile[InitialDataRow]("./benchmarker.json")
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (idrs InitialDataRows) Choise() *InitialDataRow {
	n := rand.Intn(len(idrs))
	return idrs[n]
}

type InitialDataTenantRow data.BenchmarkerTenantSource
type InitialDataTenantRows []*InitialDataTenantRow

func GetInitialDataTenant() (InitialDataTenantRows, error) {
	data, err := LoadFromJSONFile[InitialDataTenantRow]("./benchmarker_tenant.json")
	if err != nil {
		return nil, err
	}
	if len(data) < 100 {
		return nil, fmt.Errorf("初期テナントデータの量が足りません (want:%d got:%d)", 100, len(data))
	}

	sort.Slice(data, func(i, j int) bool {
		return data[i].TenantID < data[j].TenantID
	})
	return data, nil
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

func (srs ScoreRows) PlayerIDs() []string {
	idsMap := map[string]struct{}{}
	for _, row := range srs {
		if _, ok := idsMap[row.PlayerID]; !ok {
			idsMap[row.PlayerID] = struct{}{}
		}
	}
	ids := []string{}
	for key, _ := range idsMap {
		ids = append(ids, key)
	}
	return ids
}

type CompetitionData struct {
	ID    string
	Title string
}
type PlayerData struct {
	ID          string
	DisplayName string
}
