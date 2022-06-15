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

func tenantDBPath(name string) string {
	tenantDBDir := getEnv("ISUCON_TENANT_DB_DIR", "../tenant_db")
	return filepath.Join(tenantDBDir, name+".db")
}

func connectToTenantDB(name string) (*sqlx.DB, error) {
	p := tenantDBPath(name)
	// sqlite3-with-trace は initializeSQLLogger() で設定されるクエリログ出力機能付きドライバ
	// 必要ない場合は sqlite3 に変更する
	return sqlx.Open("sqlite3-with-trace", fmt.Sprintf("file:%s?mode=rw", p))
}

func getTenantName(c echo.Context) (string, error) {
	baseHost := getEnv("ISUCON_BASE_HOSTNAME", ".t.isucon.dev")
	host := c.Request().Host
	if !strings.HasSuffix(host, baseHost) {
		return "", fmt.Errorf("host is not contains %s: %s", baseHost, host)
	}
	tenantName := strings.TrimSuffix(host, baseHost)

	return tenantName, nil
}

func createTenantDB(name string) error {
	p := tenantDBPath(name)

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
	sqlLogger, err := initializeSQLLogger()
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
	e.POST("/organizer/api/players/add", playersAddHandler)
	e.POST("/organizer/api/player/:player_id/disqualified", playerDisqualifiedHandler)
	// 大会操作
	e.POST("/organizer/api/competitions/add", competitionsAddHandler)
	e.POST("/organizer/api/competition/:competition_id/finish", competitionFinishHandler)
	e.POST("/organizer/api/competition/:competition_id/result", competitionResultHandler)
	// テナント操作
	e.GET("/organizer/api/billing", billingHandler)
	// 参加者からの閲覧
	e.GET("/player/api/player/:player_id", playerHandler)
	e.GET("/player/api/competition/:competition_id/ranking", competitionRankingHandler)
	e.GET("/player/api/competitions", competitionsHandler)

	// for admin endpoint
	e.POST("/admin/api/tenants/add", tenantsAddHandler)
	e.GET("/admin/api/tenants/billing", tenantsBillingHandler)

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

type Viewer struct {
	role       Role
	playerID   string
	tenantName string
}

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

	v := &Viewer{
		role:       r,
		playerID:   token.Subject(),
		tenantName: aud[0],
	}
	return v, nil
}

type TenantRow struct {
	ID          int64     `db:"id"`
	Name        string    `db:"name"`
	DisplayName string    `db:"display_name"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type dbOrTx interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type PlayerRow struct {
	ID             string    `db:"id"`
	DisplayName    string    `db:"display_name"`
	IsDisqualified bool      `db:"is_disqualified"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

func retrievePlayer(ctx context.Context, tenantDB dbOrTx, id string) (*PlayerRow, error) {
	var c PlayerRow
	if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM player WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select player: id=%s, %w", id, err)
	}
	return &c, nil
}

type CompetitionRow struct {
	ID         string       `db:"id"`
	Title      string       `db:"title"`
	FinishedAt sql.NullTime `db:"finished_at"`
	CreatedAt  time.Time    `db:"created_at"`
	UpdatedAt  time.Time    `db:"updated_at"`
}

func retrieveCompetition(ctx context.Context, tenantDB dbOrTx, id string) (*CompetitionRow, error) {
	var c CompetitionRow
	if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM competition WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select competition: id=%s, %w", id, err)
	}
	return &c, nil
}

type PlayerScoreRow struct {
	PlayerID      string    `db:"player_id"`
	CompetitionID string    `db:"competition_id"`
	Score         int64     `db:"score"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type TenantDetail struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type TenantsAddHandlerResult struct {
	Tenant TenantDetail `json:"tenant"`
}

func tenantsAddHandler(c echo.Context) error {
	if c.Request().Host != getEnv("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev") {
		return echo.ErrNotFound
	}

	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleAdmin {
		return errNotPermitted
	}

	displayName := c.FormValue("display_name")
	name := c.FormValue("name")
	if err := validateTenantName(name); err != nil {
		c.Logger().Errorf("failed to validateTenantName: %v", name, err)
		return echo.ErrBadRequest
	}

	ctx := c.Request().Context()
	tx, err := centerDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error centerDB.BeginTxx: %w", err)
	}
	now := time.Now()
	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		name, displayName, now, now,
	)
	if err != nil {
		tx.Rollback()
		if merr, ok := err.(*mysql.MySQLError); ok && merr.Number == 1062 { // duplicate entry
			c.Logger().Errorf("failed to insert tenant: %v", err)
			return echo.ErrBadRequest
		}
		return fmt.Errorf(
			"error Insert tenant: name=%s, displayName=%s, createdAt=%s, updatedAt=%s, %w",
			name, displayName, now, now, err,
		)
	}

	if err := createTenantDB(name); err != nil {
		tx.Rollback()
		return fmt.Errorf("error createTenantDB: name=%s %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error tx.Commit: %w", err)
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
	PlayerID      string    `db:"player_id"`
	TenantID      int64     `db:"tenant_id"`
	CompetitionID string    `db:"competition_id"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type VisitHistorySummaryRow struct {
	PlayerID     string    `db:"player_id"`
	MinCreatedAt time.Time `db:"min_created_at"`
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
		if comp.FinishedAt.Valid && comp.FinishedAt.Time.Before(vh.MinCreatedAt) {
			continue
		}
		// scoreに登録されていないplayerでアクセスした人 * 10
		billingMap[vh.PlayerID] = 10
	}

	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE competition_id = ?",
		comp.ID,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error Select count player_score: competitionID=%s, %w", competitonID, err)
	}
	for _, ps := range pss {
		player, err := retrievePlayer(ctx, tenantDB, ps.PlayerID)
		if err != nil {
			return nil, fmt.Errorf("error retrievePlayer: %w", err)
		}
		if _, ok := billingMap[player.ID]; ok {
			// scoreに登録されているplayerでアクセスした人 * 100
			billingMap[player.ID] = 100
		} else {
			// scoreに登録されているplayerでアクセスしていない人 * 50
			billingMap[player.ID] = 50
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
		PlayerCount:      int64(len(pss)),
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

	ctx := c.Request().Context()
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
		tenantDB, err := connectToTenantDB(t.Name)
		if err != nil {
			return fmt.Errorf("error connectToTenantDB: %w", err)
		}
		defer tenantDB.Close()
		cs := []CompetitionRow{}
		if err := tenantDB.SelectContext(
			ctx,
			&cs,
			"SELECT * FROM competition",
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

type PlayersAddHandlerResult struct {
	Players []PlayerDetail `json:"players"`
}

func playersAddHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	params, err := c.FormParams()
	if err != nil {
		return fmt.Errorf("error c.FormParams: %w", err)
	}
	displayNames := params["display_name"]

	now := time.Now()
	ttx, err := tenantDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error tenantDB.BeginTxx: %w", err)
	}
	pds := make([]PlayerDetail, 0, len(displayNames))
	for _, displayName := range displayNames {
		id, err := dispenseID(ctx)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error dispenseID: %w", err)
		}

		if _, err := ttx.ExecContext(
			ctx,
			"INSERT INTO player (id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			id, displayName, false, now, now,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf(
				"error Insert player at tenantDB: id=%s, displayName=%s, isDisqualified=%t, createdAt=%s, updatedAt=%s, %w",
				id, displayName, false, now, now, err,
			)
		}
		p, err := retrievePlayer(ctx, ttx, id)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		pds = append(pds, PlayerDetail{
			ID:             p.ID,
			DisplayName:    p.DisplayName,
			IsDisqualified: p.IsDisqualified,
		})
	}
	if err := ttx.Commit(); err != nil {
		return fmt.Errorf("error ttx.Commit: %w", err)
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
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	playerID := c.Param("player_id")

	now := time.Now()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?",
		true, now, playerID,
	); err != nil {
		return fmt.Errorf(
			"error Update player: isDisqualified=%t, updatedAt=%s, id=%s, %w",
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
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	title := c.FormValue("title")

	now := time.Now()
	id, err := dispenseID(ctx)
	if err != nil {
		return fmt.Errorf("error dispenseID: %w", err)
	}
	if _, err := tenantDB.ExecContext(
		ctx,
		"INSERT INTO competition (id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, title, sql.NullTime{}, now, now,
	); err != nil {
		return fmt.Errorf(
			"error Insert competition: id=%s, title=%s, finishedAt=null, createdAt=%s, updatedAt=%s, %w",
			id, title, now, now, err,
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
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	id := c.Param("competition_id")
	if id == "" {
		return echo.ErrBadRequest
	}

	now := time.Now()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?",
		now, now, id,
	); err != nil {
		return fmt.Errorf(
			"error Update competition: finishedAt=%s, updatedAt=%s, id=%s, %w",
			now, now, id, err,
		)
	}

	if err := c.JSON(http.StatusOK, SuccessResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitionResultHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
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

	now := time.Now()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return fmt.Errorf("error r.Read at header: %w", err)
	}
	if !reflect.DeepEqual(headers, []string{"player_id", "score"}) {
		return fmt.Errorf("not match header: %#v", headers)
	}
	ttx, err := tenantDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error tenantDB.BeginTxx: %w", err)
	}
	if _, err := ttx.ExecContext(
		ctx,
		"DELETE FROM player_score WHERE competition_id = ?",
		competitionID,
	); err != nil {
		return fmt.Errorf("error Delete player_score: competitionID=%s, %w", competitionID, err)
	}
	for {
		row, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			ttx.Rollback()
			return fmt.Errorf("error r.Read at rows: %w", err)
		}
		if len(row) != 2 {
			ttx.Rollback()
			return fmt.Errorf("row must have two columns: %#v", row)
		}
		playerID, scoreStr := row[0], row[1]
		player, err := retrievePlayer(ctx, tenantDB, playerID)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		var score int64
		if score, err = strconv.ParseInt(scoreStr, 10, 64); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error strconv.ParseUint: scoreStr=%s, %w", scoreStr, err)
		}
		if _, err := ttx.ExecContext(
			ctx,
			"REPLACE INTO player_score (player_id, competition_id, score, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			player.ID, competitionID, score, now, now,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf(
				"error Replace player_score: playerID=%s, competitionID=%s, score=%d, createdAt=%s, updatedAt=%s, %w",
				player.ID, competitionID, score, now, now, err,
			)
		}
	}

	if err := ttx.Commit(); err != nil {
		return fmt.Errorf("error txx.Commit: %w", err)
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
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != RoleOrganizer {
		return errNotPermitted
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	var t TenantRow
	if err := centerDB.GetContext(
		ctx,
		&t,
		"SELECT * FROM tenant WHERE name = ?",
		tenantName,
	); err != nil {
		return fmt.Errorf("error Select tenant: name=%s, %w", tenantName, err)
	}

	cs := []CompetitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition ORDER BY created_at DESC",
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
	CompetitionCreatedAt time.Time `json:"-"`
	CompetitionTitle     string    `json:"competition_title"`
	Score                int64     `json:"score"`
}

type PlayerHandlerResult struct {
	Player PlayerDetail        `json:"player"`
	Scores []PlayerScoreDetail `json:"scores"`
}

func playerHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
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
	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE player_id = ? ORDER BY competition_id ASC",
		p.ID,
	); err != nil {
		return fmt.Errorf("error Select player_score: playerID=%s, %w", p.ID, err)
	}
	psds := make([]PlayerScoreDetail, 0, len(pss))
	for _, ps := range pss {
		comp, err := retrieveCompetition(ctx, tenantDB, ps.CompetitionID)
		if err != nil {
			return fmt.Errorf("error retrieveCompetition: %w", err)
		}
		psds = append(psds, PlayerScoreDetail{
			CompetitionCreatedAt: comp.CreatedAt,
			CompetitionTitle:     comp.Title,
			Score:                ps.Score,
		})
	}
	// 大会作成日時で降順ソートする
	sort.Slice(psds, func(i, j int) bool {
		psd1, psd2 := psds[i], psds[j]
		return psd1.CompetitionCreatedAt.After(psd2.CompetitionCreatedAt)
	})

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
	ctx := c.Request().Context()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	competitionID := c.Param("competition_id")
	if competitionID == "" {
		return echo.ErrBadRequest
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
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

	vp, err := retrievePlayer(c.Request().Context(), tenantDB, v.playerID)
	if err != nil {
		return fmt.Errorf("error retrievePlayer from viewer: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	now := time.Now()
	var t TenantRow
	if err := centerDB.GetContext(ctx, &t, "SELECT * FROM tenant WHERE name = ?", v.tenantName); err != nil {
		return fmt.Errorf("error Select tenant: name=%s, %w", v.tenantName, err)
	}

	if _, err := centerDB.ExecContext(
		ctx,
		"INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		vp.ID, t.ID, competitionID, now, now,
	); err != nil {
		return fmt.Errorf(
			"error Insert visit_history: playerID=%s, tenantID=%d, competitionID=%s, createdAt=%s, updatedAt=%s, %w",
			vp.ID, t.ID, competitionID, now, now, err,
		)
	}

	var rankAfter int64
	rankAfterStr := c.QueryParam("rank_after")
	if rankAfterStr != "" {
		if rankAfter, err = strconv.ParseInt(rankAfterStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: rankAfterStr=%s, %w", rankAfterStr, err)
		}
	}

	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE competition_id = ? ORDER BY score DESC, player_id DESC",
		competitionID,
	); err != nil {
		return fmt.Errorf("error Select player_score: competitionID=%s, %w", competitionID, err)
	}
	crs := make([]CompetitionRank, 0, len(pss))
	for i, ps := range pss {
		co, err := retrievePlayer(ctx, tenantDB, ps.PlayerID)
		if err != nil {
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		if int64(i) < rankAfter {
			continue
		}
		crs = append(crs, CompetitionRank{
			Rank:              int64(i + 1),
			Score:             ps.Score,
			PlayerID:          co.ID,
			PlayerDisplayName: co.DisplayName,
		})
		if len(crs) >= 100 {
			break
		}
	}

	res := SuccessResult{
		Success: true,
		Data: CompetitionRankingHandlerResult{
			Ranks: crs,
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
	ctx := c.Request().Context()

	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}
	defer tenantDB.Close()

	vp, err := retrievePlayer(c.Request().Context(), tenantDB, v.playerID)
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
		"SELECT * FROM competition ORDER BY created_at DESC",
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
