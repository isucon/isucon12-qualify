package isuports

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/gofrs/flock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	tenantDBSchemaFilePath = "../sql/tenant/10_schema.sql"
	cookieName             = "isuports_session"
	initializeScript       = "../sql/init.sh"
)

var (
	tenantNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	sqliteDriverName = "sqlite3"
)

func getEnv(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

func connectCenterDB() (*sqlx.DB, error) {
	config := mysql.NewConfig()
	config.Net = "tcp"
	config.Addr = getEnv("ISUCON_DB_HOST", "127.0.0.1") + ":" + getEnv("ISUCON_DB_PORT", "3306")
	config.User = getEnv("ISUCON_DB_USER", "isucon")
	config.Passwd = getEnv("ISUCON_DB_PASSWORD", "isucon")
	config.DBName = getEnv("ISUCON_DB_NAME", "isuports")
	config.ParseTime = true

	dsn := config.FormatDSN()
	return sqlx.Open("mysql", dsn)
}

func tenantDBPath(id int64) string {
	tenantDBDir := getEnv("ISUCON_TENANT_DB_DIR", "../tenant_db")
	return filepath.Join(tenantDBDir, fmt.Sprintf("%d.db", id))
}

func connectToTenantDB(id int64) (*sqlx.DB, error) {
	p := tenantDBPath(id)
	return sqlx.Open(sqliteDriverName, fmt.Sprintf("file:%s?mode=rw", p))
}

func createTenantDB(id int64) error {
	p := tenantDBPath(id)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("sqlite3 %s < %s", p, tenantDBSchemaFilePath))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error exec sqlite3 %s < %s, out=%s: %w", p, tenantDBSchemaFilePath, string(out), err)
	}
	return nil
}

func dispenseID(ctx context.Context) (string, error) {
	var id int64
	var lastErr error
	for i := 0; i < 100; i++ {
		var ret sql.Result
		ret, err := centerDB.ExecContext(ctx, "REPLACE INTO id_generator (stub) VALUES (?);", "a")
		if err != nil {
			if merr, ok := err.(*mysql.MySQLError); ok && merr.Number == 1213 { // deadlock
				lastErr = fmt.Errorf("error REPLACE INTO id_generator: %w", err)
				continue
			}
			return "", fmt.Errorf("error REPLACE INTO id_generator: %w", err)
		}
		id, err = ret.LastInsertId()
		if err != nil {
			return "", fmt.Errorf("error ret.LastInsertId: %w", err)
		}
		break
	}
	if id != 0 {
		return strconv.FormatInt(id, 10), nil
	}
	return "", lastErr
}

var centerDB *sqlx.DB

func Run() {
	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.DEBUG)

	// sqliteのクエリログを出力する設定
	// 環境変数 ISUCON_SQLITE_TRACE_FILE を設定すると、そのファイルにクエリログをJSON形式で出力する
	var (
		sqlLogger io.Closer
		err       error
	)
	sqliteDriverName, sqlLogger, err = initializeSQLLogger()
	if err != nil {
		e.Logger.Panicf("error initializeSQLLogger: %s", err)
	}
	defer sqlLogger.Close()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// for benchmarker
	e.POST("/initialize", initializeHandler)

	// for tenant endpoint
	// 参加者操作
	e.GET("/api/organizer/players", playersListHandler)
	e.POST("/api/organizer/players/add", playersAddHandler)
	e.POST("/api/organizer/player/:player_id/disqualified", playerDisqualifiedHandler)
	// 大会操作
	e.POST("/api/organizer/competitions/add", competitionsAddHandler)
	e.POST("/api/organizer/competition/:competition_id/finish", competitionFinishHandler)
	e.POST("/api/organizer/competition/:competition_id/result", competitionResultHandler)
	// テナント操作
	e.GET("/api/organizer/billing", billingHandler)
	// 参加者からの閲覧
	e.GET("/api/player/player/:player_id", playerHandler)
	e.GET("/api/player/competition/:competition_id/ranking", competitionRankingHandler)
	e.GET("/api/player/competitions", competitionsHandler)

	// for admin endpoint
	e.POST("/api/admin/tenants/add", tenantsAddHandler)
	e.GET("/api/admin/tenants/billing", tenantsBillingHandler)

	e.HTTPErrorHandler = errorResponseHandler

	centerDB, err = connectCenterDB()
	if err != nil {
		e.Logger.Fatalf("failed to connect db: %v", err)
		return
	}
	centerDB.SetMaxOpenConns(10)
	defer centerDB.Close()

	port := getEnv("SERVER_APP_PORT", "3000")
	e.Logger.Infof("starting isuports server on : %s ...", port)
	serverPort := fmt.Sprintf(":%s", port)
	e.Logger.Fatal(e.Start(serverPort))
}

func errorResponseHandler(err error, c echo.Context) {
	c.Logger().Errorf("error at %s: %s", c.Path(), err.Error())
	if errors.Is(err, errNotPermitted) {
		c.JSON(http.StatusForbidden, FailureResult{
			Success: false,
			Message: err.Error(),
		})
		return
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		c.JSON(he.Code, FailureResult{
			Success: false,
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusInternalServerError, FailureResult{
		Success: false,
		Message: err.Error(),
	})
}

type SuccessResult struct {
	Success bool `json:"status"`
	Data    any  `json:"data,omitempty"`
}

type FailureResult struct {
	Success bool   `json:"status"`
	Message string `json:"message"`
}

type Role int

const (
	RoleAdmin Role = iota + 1
	RoleOrganizer
	RolePlayer
)

var (
	errNotPermitted = errors.New("this role is not permitted")
)

// アクセスしてきた人の情報
type Viewer struct {
	role       Role
	playerID   string
	tenantName string
	tenantID   int64
}

// リクエストヘッダをパースしてViewerを返す
func parseViewer(c echo.Context) (*Viewer, error) {
	cookie, err := c.Request().Cookie(cookieName)
	if err != nil {
		return nil, fmt.Errorf("error c.Request().Cookie: %w", err)
	}
	tokenStr := cookie.Value

	keyFilename := getEnv("ISUCON_JWT_KEY_FILE", "./public.pem")
	keysrc, err := os.ReadFile(keyFilename)
	if err != nil {
		return nil, fmt.Errorf("error os.ReadFile: keyFilename=%s: %w", keyFilename, err)
	}
	key, _, err := jwk.DecodePEM(keysrc)
	if err != nil {
		return nil, fmt.Errorf("error jwk.DecodePEM: %w", err)
	}

	token, err := jwt.Parse(
		[]byte(tokenStr),
		jwt.WithKey(jwa.RS256, key),
	)
	if err != nil {
		if jwt.IsValidationError(err) {
			return nil, echo.ErrBadRequest
		}
		return nil, fmt.Errorf("error parse: %w", err)
	}
	if token.Subject() == "" {
		return nil, echo.ErrBadRequest
	}

	var r Role
	tr, ok := token.Get("role")
	if !ok {
		return nil, fmt.Errorf("token is invalid, not have role field: %s", tokenStr)
	}
	switch tr {
	case "admin":
		r = RoleAdmin
	case "organizer":
		r = RoleOrganizer
	case "player":
		r = RolePlayer
	default:
		return nil, fmt.Errorf("token is invalid, unknown role: %s", tokenStr)
	}
	aud := token.Audience()
	if len(aud) != 1 {
		return nil, fmt.Errorf("token is invalid, aud field is few or too many: %s", tokenStr)
	}

	// JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
	baseHost := getEnv("ISUCON_BASE_HOSTNAME", ".t.isucon.dev")
	tenantName := strings.TrimSuffix(c.Request().Host, baseHost)
	if tenantName != aud[0] {
		return nil, fmt.Errorf("token is invalid, tenant name is not match with %s: %s", c.Request().Host, tokenStr)
	}

	// SaaS管理者用ドメイン
	if r == RoleAdmin && tenantName == "admin" {
		return &Viewer{
			role:       r,
			playerID:   token.Subject(),
			tenantName: "admin",
		}, nil
	}

	// テナントの存在確認
	var tenant TenantRow
	if err := centerDB.GetContext(
		context.Background(),
		&tenant,
		"SELECT * FROM tenant WHERE name = ?",
		tenantName,
	); err != nil {
		return nil, fmt.Errorf("error Select tenant: name=%s, %w", tenantName, err)
	}

	v := &Viewer{
		role:       r,
		playerID:   token.Subject(),
		tenantName: tenant.Name,
		tenantID:   tenant.ID,
	}
	return v, nil
}

type TenantRow struct {
	ID          int64  `db:"id"`
	Name        string `db:"name"`
	DisplayName string `db:"display_name"`
	CreatedAt   int64  `db:"created_at"`
	UpdatedAt   int64  `db:"updated_at"`
}

type dbOrTx interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type PlayerRow struct {
	TenantID       int64  `db:"tenant_id"`
	ID             string `db:"id"`
	DisplayName    string `db:"display_name"`
	IsDisqualified bool   `db:"is_disqualified"`
	CreatedAt      int64  `db:"created_at"`
	UpdatedAt      int64  `db:"updated_at"`
}

func retrievePlayer(ctx context.Context, tenantDB dbOrTx, id string) (*PlayerRow, error) {
	var p PlayerRow
	if err := tenantDB.GetContext(ctx, &p, "SELECT * FROM player WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select player: id=%s, %w", id, err)
	}
	return &p, nil
}

type CompetitionRow struct {
	TenantID   int64         `db:"tenant_id"`
	ID         string        `db:"id"`
	Title      string        `db:"title"`
	FinishedAt sql.NullInt64 `db:"finished_at"`
	CreatedAt  int64         `db:"created_at"`
	UpdatedAt  int64         `db:"updated_at"`
}

func retrieveCompetition(ctx context.Context, tenantDB dbOrTx, id string) (*CompetitionRow, error) {
	var c CompetitionRow
	if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM competition WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select competition: id=%s, %w", id, err)
	}
	return &c, nil
}

type PlayerScoreRow struct {
	TenantID      int64  `db:"tenant_id"`
	ID            string `db:"id"`
	PlayerID      string `db:"player_id"`
	CompetitionID string `db:"competition_id"`
	Score         int64  `db:"score"`
	RowNumber     int64  `db:"row_number"`
	CreatedAt     int64  `db:"created_at"`
	UpdatedAt     int64  `db:"updated_at"`
}

func lockFilePath(id int64) string {
	tenantDBDir := getEnv("ISUCON_TENANT_DB_DIR", "../tenant_db")
	return filepath.Join(tenantDBDir, fmt.Sprintf("%d.lock", id))
}

func flockByTenantID(tenantID int64) (io.Closer, error) {
	p := lockFilePath(tenantID)

	fl := flock.New(p)
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("error flock.Lock: path=%s, %w", p, err)
	}
	return fl, nil
}

type TenantDetail struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type TenantsAddHandlerResult struct {
	Tenant TenantDetail `json:"tenant"`
}

func tenantsAddHandler(c echo.Context) error {
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.tenantName != "admin" {
		// admin: SaaS管理者用の特別なテナント名
		return echo.ErrNotFound
	} else if v.role != RoleAdmin {
		return errNotPermitted
	}

	displayName := c.FormValue("display_name")
	name := c.FormValue("name")
	if err := validateTenantName(name); err != nil {
		c.Logger().Errorf("failed to validateTenantName: %v", name, err)
		return echo.ErrBadRequest
	}

	ctx := context.Background()
	now := time.Now().Unix()
	insertRes, err := centerDB.ExecContext(
		ctx,
		"INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		name, displayName, now, now,
	)
	if err != nil {
		if merr, ok := err.(*mysql.MySQLError); ok && merr.Number == 1062 { // duplicate entry
			c.Logger().Errorf("failed to insert tenant: %v", err)
			return echo.ErrBadRequest
		}
		return fmt.Errorf(
			"error Insert tenant: name=%s, displayName=%s, createdAt=%d, updatedAt=%d, %w",
			name, displayName, now, now, err,
		)
	}

	id, err := insertRes.LastInsertId()
	if err != nil {
		return fmt.Errorf("error get LastInsertId: %w", err)
	}
	if err := createTenantDB(id); err != nil {
		return fmt.Errorf("error createTenantDB: id=%d name=%s %w", id, name, err)
	}

	res := TenantsAddHandlerResult{
		Tenant: TenantDetail{
			Name:        name,
			DisplayName: displayName,
		},
	}
	if err := c.JSON(http.StatusOK, SuccessResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func validateTenantName(name string) error {
	if tenantNameRegexp.MatchString(name) {
		return nil
	}
	return fmt.Errorf("invalid tenant name: %s", name)
}

type BillingReport struct {
	CompetitionID    string `json:"competition_id"`
	CompetitionTitle string `json:"competition_title"`
	PlayerCount      int64  `json:"player_count"`
	BillingYen       int64  `json:"billing_yen"`
}

type VisitHistoryRow struct {
	PlayerID      string `db:"player_id"`
	TenantID      int64  `db:"tenant_id"`
	CompetitionID string `db:"competition_id"`
	CreatedAt     int64  `db:"created_at"`
	UpdatedAt     int64  `db:"updated_at"`
}

type VisitHistorySummaryRow struct {
	PlayerID     string `db:"player_id"`
	MinCreatedAt int64  `db:"min_created_at"`
}

func billingReportByCompetition(ctx context.Context, tenantDB dbOrTx, tenantID int64, competitonID string) (*BillingReport, error) {
	comp, err := retrieveCompetition(ctx, tenantDB, competitonID)
	if err != nil {
		return nil, fmt.Errorf("error retrieveCompetition: %w", err)
	}

	vhs := []VisitHistorySummaryRow{}
	if err := centerDB.SelectContext(
		ctx,
		&vhs,
		"SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id",
		tenantID,
		comp.ID,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error Select visit_history: tenantID=%d, competitionID=%s, %w", tenantID, comp.ID, err)
	}
	billingMap := map[string]int64{}
	for _, vh := range vhs {
		// competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
		if comp.FinishedAt.Valid && comp.FinishedAt.Int64 < vh.MinCreatedAt {
			continue
		}
		// scoreに登録されていないplayerでアクセスした人 * 10
		billingMap[vh.PlayerID] = 10
	}

	// player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
	fl, err := flockByTenantID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("error flockByTenantID: %w", err)
	}
	defer fl.Close()
	scoredPlayerIDs := []string{}
	if err := tenantDB.SelectContext(
		ctx,
		&scoredPlayerIDs,
		"SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?",
		tenantID, comp.ID,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error Select count player_score: tenantID=%d, competitionID=%s, %w", tenantID, competitonID, err)
	}
	for _, pid := range scoredPlayerIDs {
		if _, ok := billingMap[pid]; ok {
			// scoreに登録されているplayerでアクセスした人 * 100
			billingMap[pid] = 100
		} else {
			// scoreに登録されているplayerでアクセスしていない人 * 50
			billingMap[pid] = 50
		}
	}

	var billingYen int64
	// 大会が終了している場合は課金を計算する(開催中の場合は常に 0)
	if comp.FinishedAt.Valid {
		for _, v := range billingMap {
			billingYen += v
		}
	}
	return &BillingReport{
		CompetitionID:    comp.ID,
		CompetitionTitle: comp.Title,
		PlayerCount:      int64(len(scoredPlayerIDs)),
		BillingYen:       billingYen,
	}, nil
}

type TenantWithBilling struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	BillingYen  int64  `json:"billing"`
}

type TenantsBillingHandlerResult struct {
	Tenants []TenantWithBilling `json:"tenants"`
}

func tenantsBillingHandler(c echo.Context) error {
	if c.Request().Host != getEnv("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev") {
		return echo.ErrNotFound
	}

	ctx := context.Background()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleAdmin {
		return errNotPermitted
	}

	before := c.QueryParam("before")
	var beforeID int64
	if before != "" {
		var err error
		beforeID, err = strconv.ParseInt(before, 10, 64)
		if err != nil {
			return fmt.Errorf("error strconv.ParseInt at before: %w", err)
		}
	}
	// テナントごとに
	//   大会ごとに
	//     scoreに登録されているplayerでアクセスした人 * 100
	//     scoreに登録されているplayerでアクセスしていない人 * 50
	//     scoreに登録されていないplayerでアクセスした人 * 10
	//   を合計したものを
	// テナントの課金とする
	ts := []TenantRow{}
	if err := centerDB.SelectContext(ctx, &ts, "SELECT * FROM tenant ORDER BY id DESC"); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	tenantBillings := make([]TenantWithBilling, 0, len(ts))
	for _, t := range ts {
		if beforeID != 0 && beforeID <= t.ID {
			continue
		}
		tb := TenantWithBilling{
			ID:          strconv.FormatInt(t.ID, 10),
			Name:        t.Name,
			DisplayName: t.DisplayName,
		}
		tenantDB, err := connectToTenantDB(t.ID)
		if err != nil {
			return fmt.Errorf("error connectToTenantDB: %w", err)
		}
		defer tenantDB.Close()
		cs := []CompetitionRow{}
		if err := tenantDB.SelectContext(
			ctx,
			&cs,
			"SELECT * FROM competition WHERE tenant_id=?",
			t.ID,
		); err != nil {
			return fmt.Errorf("error Select competition: %w", err)
		}
		for _, comp := range cs {
			report, err := billingReportByCompetition(ctx, tenantDB, t.ID, comp.ID)
			if err != nil {
				return fmt.Errorf("error billingReportByCompetition: %w", err)
			}
			tb.BillingYen += report.BillingYen
		}
		tenantBillings = append(tenantBillings, tb)
		if len(tenantBillings) >= 20 {
			break
		}
	}
	if err := c.JSON(http.StatusOK, SuccessResult{
		Success: true,
		Data: TenantsBillingHandlerResult{
			Tenants: tenantBillings,
		},
	}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type PlayerDetail struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	IsDisqualified bool   `json:"is_disqualified"`
}

type PlayersListHandlerResult struct {
	Players []PlayerDetail `json:"players"`
}

func playersListHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	var pls []PlayerRow
	if err := tenantDB.SelectContext(
		ctx,
		&pls,
		"SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC",
		v.tenantID,
	); err != nil {
		return fmt.Errorf("error Select player: %w", err)
	}
	var pds []PlayerDetail
	for _, p := range pls {
		pds = append(pds, PlayerDetail{
			ID:             p.ID,
			DisplayName:    p.DisplayName,
			IsDisqualified: p.IsDisqualified,
		})
	}

	res := PlayersListHandlerResult{
		Players: pds,
	}
	if err := c.JSON(http.StatusOK, SuccessResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type PlayersAddHandlerResult struct {
	Players []PlayerDetail `json:"players"`
}

func playersAddHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	params, err := c.FormParams()
	if err != nil {
		return fmt.Errorf("error c.FormParams: %w", err)
	}
	displayNames := params["display_name"]

	pds := make([]PlayerDetail, 0, len(displayNames))
	for _, displayName := range displayNames {
		id, err := dispenseID(ctx)
		if err != nil {
			return fmt.Errorf("error dispenseID: %w", err)
		}

		now := time.Now().Unix()
		if _, err := tenantDB.ExecContext(
			ctx,
			"INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, v.tenantID, displayName, false, now, now,
		); err != nil {
			return fmt.Errorf(
				"error Insert player at tenantDB: id=%s, displayName=%s, isDisqualified=%t, createdAt=%d, updatedAt=%d, %w",
				id, displayName, false, now, now, err,
			)
		}
		p, err := retrievePlayer(ctx, tenantDB, id)
		if err != nil {
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		pds = append(pds, PlayerDetail{
			ID:             p.ID,
			DisplayName:    p.DisplayName,
			IsDisqualified: p.IsDisqualified,
		})
	}

	res := PlayersAddHandlerResult{
		Players: pds,
	}
	if err := c.JSON(http.StatusOK, SuccessResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type PlayerDisqualifiedHandlerResult struct {
	Player PlayerDetail `json:"player"`
}

func playerDisqualifiedHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	playerID := c.Param("player_id")

	now := time.Now().Unix()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?",
		true, now, playerID,
	); err != nil {
		return fmt.Errorf(
			"error Update player: isDisqualified=%t, updatedAt=%d, id=%s, %w",
			true, now, playerID, err,
		)
	}
	p, err := retrievePlayer(ctx, tenantDB, playerID)
	if err != nil {
		return fmt.Errorf("error retrievePlayer: %w", err)
	}

	res := PlayerDisqualifiedHandlerResult{
		Player: PlayerDetail{
			ID:             p.ID,
			DisplayName:    p.DisplayName,
			IsDisqualified: p.IsDisqualified,
		},
	}
	if err := c.JSON(http.StatusOK, SuccessResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type CompetitionDetail struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	IsFinished bool   `json:"is_finished"`
}

type CompetitionsAddHandlerResult struct {
	Competition CompetitionDetail `json:"competition"`
}

func competitionsAddHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	title := c.FormValue("title")

	now := time.Now().Unix()
	id, err := dispenseID(ctx)
	if err != nil {
		return fmt.Errorf("error dispenseID: %w", err)
	}
	if _, err := tenantDB.ExecContext(
		ctx,
		"INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, v.tenantID, title, sql.NullInt64{}, now, now,
	); err != nil {
		return fmt.Errorf(
			"error Insert competition: id=%s, tenant_id=%d, title=%s, finishedAt=null, createdAt=%d, updatedAt=%d, %w",
			id, v.tenantID, title, now, now, err,
		)
	}

	res := CompetitionsAddHandlerResult{
		Competition: CompetitionDetail{
			ID:         id,
			Title:      title,
			IsFinished: false,
		},
	}
	if err := c.JSON(http.StatusOK, SuccessResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitionFinishHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	id := c.Param("competition_id")
	if id == "" {
		return echo.ErrBadRequest
	}
	_, err = retrieveCompetition(ctx, tenantDB, id)
	if err != nil {
		// 存在しない大会
		if errors.Is(err, sql.ErrNoRows) {
			return echo.ErrNotFound
		}
		return fmt.Errorf("error retrieveCompetition: %w", err)
	}

	now := time.Now().Unix()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?",
		now, now, id,
	); err != nil {
		return fmt.Errorf(
			"error Update competition: finishedAt=%d, updatedAt=%d, id=%s, %w",
			now, now, id, err,
		)
	}

	if err := c.JSON(http.StatusOK, SuccessResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitionResultHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	competitionID := c.Param("competition_id")
	if competitionID == "" {
		return echo.ErrBadRequest
	}
	comp, err := retrieveCompetition(ctx, tenantDB, competitionID)
	if err != nil {
		// 存在しない大会
		if errors.Is(err, sql.ErrNoRows) {
			return echo.ErrNotFound
		}
		return fmt.Errorf("error retrieveCompetition: %w", err)
	}
	if comp.FinishedAt.Valid {
		res := FailureResult{
			Success: false,
			Message: "competition is finished",
		}
		if err := c.JSON(http.StatusBadRequest, res); err != nil {
			return fmt.Errorf("error c.JSON: %w", err)
		}
	}

	fh, err := c.FormFile("scores")
	if err != nil {
		return fmt.Errorf("error c.FormFile(scores): %w", err)
	}
	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("error fh.Open FormFile(scores): %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return fmt.Errorf("error r.Read at header: %w", err)
	}
	if !reflect.DeepEqual(headers, []string{"player_id", "score"}) {
		return fmt.Errorf("not match header: %#v", headers)
	}

	// / DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
	fl, err := flockByTenantID(v.tenantID)
	if err != nil {
		return fmt.Errorf("error flockByTenantID: %w", err)
	}
	defer fl.Close()
	var rowNumber int64
	playerScoreRows := []PlayerScoreRow{}
	for {
		rowNumber++
		row, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error r.Read at rows: %w", err)
		}
		if len(row) != 2 {
			return fmt.Errorf("row must have two columns: %#v", row)
		}
		playerID, scoreStr := row[0], row[1]
		if _, err := retrievePlayer(ctx, tenantDB, playerID); err != nil {
			// 存在しないプレイヤーが含まれている
			if errors.Is(err, sql.ErrNoRows) {
				return echo.ErrBadRequest
			}
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		var score int64
		if score, err = strconv.ParseInt(scoreStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: scoreStr=%s, %w", scoreStr, err)
		}
		id, err := dispenseID(ctx)
		if err != nil {
			return fmt.Errorf("error dispenseID: %w", err)
		}
		now := time.Now().Unix()
		playerScoreRows = append(playerScoreRows, PlayerScoreRow{
			ID:            id,
			TenantID:      v.tenantID,
			PlayerID:      playerID,
			CompetitionID: competitionID,
			Score:         score,
			RowNumber:     rowNumber,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	if _, err := tenantDB.ExecContext(
		ctx,
		"DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?",
		v.tenantID,
		competitionID,
	); err != nil {
		return fmt.Errorf("error Delete player_score: tenantID=%d, competitionID=%s, %w", v.tenantID, competitionID, err)
	}
	for _, ps := range playerScoreRows {
		if _, err := tenantDB.NamedExecContext(
			ctx,
			"INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_number, created_at, updated_at) VALUES (:id, :tenant_id, :player_id, :competition_id, :score, :row_number, :created_at, :updated_at)",
			ps,
		); err != nil {
			return fmt.Errorf(
				"error Insert player_score: id=%s, tenant_id=%d, playerID=%s, competitionID=%s, score=%d, rowNumber=%d, createdAt=%d, updatedAt=%d, %w",
				ps.ID, ps.TenantID, ps.PlayerID, ps.CompetitionID, ps.Score, ps.RowNumber, ps.CreatedAt, ps.UpdatedAt, err,
			)

		}
	}

	if err := c.JSON(http.StatusOK, SuccessResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type BillingHandlerResult struct {
	Reports []BillingReport `json:"reports"`
}

func billingHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	var t TenantRow
	if err := centerDB.GetContext(
		ctx,
		&t,
		"SELECT * FROM tenant WHERE id = ?",
		v.tenantID,
	); err != nil {
		return fmt.Errorf("error Select tenant: name=%s, %w", v.tenantName, err)
	}

	cs := []CompetitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC",
		v.tenantID,
	); err != nil {
		return fmt.Errorf("error Select competition: %w", err)
	}
	tbrs := make([]BillingReport, 0, len(cs))
	for _, comp := range cs {
		report, err := billingReportByCompetition(ctx, tenantDB, t.ID, comp.ID)
		if err != nil {
			return fmt.Errorf("error billingReportByCompetition: %w", err)
		}
		tbrs = append(tbrs, *report)
	}

	res := SuccessResult{
		Success: true,
		Data: BillingHandlerResult{
			Reports: tbrs,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type PlayerScoreDetail struct {
	CompetitionTitle string `json:"competition_title"`
	Score            int64  `json:"score"`
}

type PlayerHandlerResult struct {
	Player PlayerDetail        `json:"player"`
	Scores []PlayerScoreDetail `json:"scores"`
}

func playerHandler(c echo.Context) error {
	ctx := context.Background()

	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	vp, err := retrievePlayer(ctx, tenantDB, v.playerID)
	if err != nil {
		return fmt.Errorf("error retrievePlayer from viewer: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	playerID := c.Param("player_id")
	if playerID == "" {
		return echo.ErrBadRequest
	}

	// playerの存在確認
	p, err := retrievePlayer(ctx, tenantDB, playerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.ErrNotFound
		}
		return fmt.Errorf("error retrievePlayer: %w", err)
	}
	cs := []CompetitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition ORDER BY created_at ASC",
	); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("error Select competition: %w", err)
	}

	// player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
	fl, err := flockByTenantID(v.tenantID)
	if err != nil {
		return fmt.Errorf("error flockByTenantID: %w", err)
	}
	defer fl.Close()
	pss := make([]PlayerScoreRow, 0, len(cs))
	for _, c := range cs {
		ps := PlayerScoreRow{}
		if err := tenantDB.GetContext(
			ctx,
			&ps,
			// 最後にCSVに登場したスコアを採用する = row_numberが一番行もの
			"SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_number DESC LIMIT 1",
			v.tenantID,
			c.ID,
			p.ID,
		); err != nil {
			// 行がない = スコアが記録されてない
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return fmt.Errorf("error Select player_score: tenantID=%d, competitionID=%s, playerID=%s, %w", v.tenantID, c.ID, p.ID, err)
		}
		pss = append(pss, ps)
	}

	psds := make([]PlayerScoreDetail, 0, len(pss))
	for _, ps := range pss {
		comp, err := retrieveCompetition(ctx, tenantDB, ps.CompetitionID)
		if err != nil {
			return fmt.Errorf("error retrieveCompetition: %w", err)
		}
		psds = append(psds, PlayerScoreDetail{
			CompetitionTitle: comp.Title,
			Score:            ps.Score,
		})
	}

	res := SuccessResult{
		Success: true,
		Data: PlayerHandlerResult{
			Player: PlayerDetail{
				ID:             p.ID,
				DisplayName:    p.DisplayName,
				IsDisqualified: p.IsDisqualified,
			},
			Scores: psds,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type CompetitionRank struct {
	Rank              int64  `json:"rank"`
	Score             int64  `json:"score"`
	PlayerID          string `json:"player_id"`
	PlayerDisplayName string `json:"player_display_name"`
}

type CompetitionRankingHandlerResult struct {
	Ranks []CompetitionRank `json:"ranks"`
}

func competitionRankingHandler(c echo.Context) error {
	ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	competitionID := c.Param("competition_id")
	if competitionID == "" {
		return echo.ErrBadRequest
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	// 大会の存在確認
	if _, err := retrieveCompetition(ctx, tenantDB, competitionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.ErrNotFound
		}
		return fmt.Errorf("error retrieveCompetition: %w", err)
	}

	player, err := retrievePlayer(ctx, tenantDB, v.playerID)
	if err != nil {
		return fmt.Errorf("error retrievePlayer from viewer: %w", err)
	}
	if player.IsDisqualified {
		return errNotPermitted
	}

	now := time.Now().Unix()
	var tenant TenantRow
	if err := centerDB.GetContext(ctx, &tenant, "SELECT * FROM tenant WHERE id = ?", v.tenantID); err != nil {
		return fmt.Errorf("error Select tenant: id=%d, %w", v.tenantID, err)
	}

	if _, err := centerDB.ExecContext(
		ctx,
		"INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		player.ID, tenant.ID, competitionID, now, now,
	); err != nil {
		return fmt.Errorf(
			"error Insert visit_history: playerID=%s, tenantID=%d, competitionID=%s, createdAt=%d, updatedAt=%d, %w",
			player.ID, tenant.ID, competitionID, now, now, err,
		)
	}

	var rankAfter int64
	rankAfterStr := c.QueryParam("rank_after")
	if rankAfterStr != "" {
		if rankAfter, err = strconv.ParseInt(rankAfterStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: rankAfterStr=%s, %w", rankAfterStr, err)
		}
	}

	// player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
	fl, err := flockByTenantID(v.tenantID)
	if err != nil {
		return fmt.Errorf("error flockByTenantID: %w", err)
	}
	defer fl.Close()
	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_number DESC",
		tenant.ID,
		competitionID,
	); err != nil {
		return fmt.Errorf("error Select player_score: tenantID=%d, competitionID=%s, %w", tenant.ID, competitionID, err)
	}
	ranks := make([]CompetitionRank, 0, len(pss))
	scoredPlayerSet := make(map[string]struct{}, len(pss))
	for _, ps := range pss {
		// player_scoreが同一player_id内ではrow_numberの降順でソートされているので
		// 現れたのが2回目以降のplayer_idはより大きいrow_numberでスコアが出ているとみなせる
		if _, ok := scoredPlayerSet[ps.PlayerID]; ok {
			continue
		}
		scoredPlayerSet[ps.PlayerID] = struct{}{}
		p, err := retrievePlayer(ctx, tenantDB, ps.PlayerID)
		if err != nil {
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		ranks = append(ranks, CompetitionRank{
			Score:             ps.Score,
			PlayerID:          p.ID,
			PlayerDisplayName: p.DisplayName,
		})
	}
	sort.Slice(ranks, func(i, j int) bool {
		return ranks[i].Score > ranks[j].Score
	})
	pagedRanks := make([]CompetitionRank, 0, 100)
	for i, rank := range ranks {
		if int64(i) < rankAfter {
			continue
		}
		pagedRanks = append(pagedRanks, CompetitionRank{
			Rank:              int64(i + 1),
			Score:             rank.Score,
			PlayerID:          rank.PlayerID,
			PlayerDisplayName: rank.PlayerDisplayName,
		})
		if len(pagedRanks) >= 100 {
			break
		}
	}

	res := SuccessResult{
		Success: true,
		Data: CompetitionRankingHandlerResult{
			Ranks: pagedRanks,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type CompetitionsHandlerResult struct {
	Competitions []CompetitionDetail
}

func competitionsHandler(c echo.Context) error {
	ctx := context.Background()

	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	vp, err := retrievePlayer(ctx, tenantDB, v.playerID)
	if err != nil {
		return fmt.Errorf("error retrievePlayer from viewer: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	cs := []CompetitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC",
		v.tenantID,
	); err != nil {
		return fmt.Errorf("error Select competition: %w", err)
	}
	cds := make([]CompetitionDetail, 0, len(cs))
	for _, comp := range cs {
		cds = append(cds, CompetitionDetail{
			ID:         comp.ID,
			Title:      comp.Title,
			IsFinished: comp.FinishedAt.Valid,
		})
	}

	res := SuccessResult{
		Success: true,
		Data: CompetitionsHandlerResult{
			Competitions: cds,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type InitializeHandlerResult struct {
	Lang   string `json:"lang"`
	Appeal string `json:"appeal"`
}

func initializeHandler(c echo.Context) error {
	out, err := exec.Command(initializeScript).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error exec.Command: %s %e", string(out), err)
	}
	res := InitializeHandlerResult{
		Lang: "go",
		// 頑張ったポイントやこだわりポイントがあれば書いてください
		// 競技中の最後に計測したものを参照して、講評記事などで使わせていただきます
		Appeal: "",
	}
	if err := c.JSON(http.StatusOK, SuccessResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}
