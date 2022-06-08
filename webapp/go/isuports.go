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
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kayac/go-katsubushi"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sync/singleflight"
)

const (
	tenantDBSchemaFilePath = "../sql/tenant/10_schema.sql"
	cookieName             = "isuports_session"
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
	config.InterpolateParams = true

	dsn := config.FormatDSN()
	return sqlx.Open("mysql", dsn)
}

func tenantDBPath(name string) string {
	tenantDBDir := getEnv("ISUCON_TENANT_DB_DIR", "../tenant_db")
	return filepath.Join(tenantDBDir, name+".db")
}

var tenantDBCache = make(map[string]*sqlx.DB, 1000)
var mu sync.Mutex

func connectToTenantDB(name string) (*sqlx.DB, error) {
	p := tenantDBPath(name)
	mu.Lock()
	defer mu.Unlock()
	if db, ok := tenantDBCache[p]; ok {
		return db, nil
	}
	if db, err := sqlx.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw", p)); err != nil {
		return nil, err
	} else {
		tenantDBCache[p] = db
		return db, nil
	}
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
	db, err := sqlx.Open("sqlite3", fmt.Sprintf("file:%s?mode=rwc", p))
	if err != nil {
		return err
	}
	for _, query := range []string{
		`CREATE TABLE "competition" (
			"id" INTEGER NOT NULL PRIMARY KEY,
			"title" TEXT NOT NULL,
			"finished_at" DATETIME NULL,
			"created_at" DATETIME NOT NULL,
			"updated_at" DATETIME NOT NULL
		)`,
		`CREATE TABLE player (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			is_disqualified INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE player_score (
			id INTEGER PRIMARY KEY,
			player_id INTEGER NOT NULL,
			competition_id INTEGER NOT NULL,
			score INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE (player_id, competition_id)
		)`,
		`CREATE TABLE IF NOT EXISTS billing_report (
			competition_id INTEGER NOT NULL PRIMARY KEY,
			competition_title TEXT NOT NULL,
			player_count INTEGER NOT NULL,
			billing_yen INTEGER NOT NULL
		)`,
	} {
		if _, err = db.ExecContext(context.Background(), query); err != nil {
			return err
		}
	}
	return nil
}

var idg, _ = katsubushi.NewGenerator(1)

func dispenseID(ctx context.Context) (int64, error) {
	id, err := idg.NextID()
	return int64(id), err
}

var centerDB *sqlx.DB

func Run() {
	e := echo.New()
	e.Debug = false
	e.Logger.SetLevel(log.ERROR)
	e.Logger.SetOutput(io.Discard)
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// for benchmarker
	e.POST("/initialize", initializeHandler)

	// for tenant endpoint
	// 参加者操作
	e.POST("/organizer/api/players/add", playersAddHandler)
	e.POST("/organizer/api/player/:player_name/disqualified", playerDisqualifiedHandler)
	// 大会操作
	e.POST("/organizer/api/competitions/add", competitionsAddHandler)
	e.POST("/organizer/api/competition/:competition_id/finish", competitionFinishHandler)
	e.POST("/organizer/api/competition/:competition_id/result", competitionResultHandler)
	// テナント操作
	e.GET("/organizer/api/billing", billingHandler)
	// 参加者からの閲覧
	e.GET("/player/api/player/:player_name", playerHandler)
	e.GET("/player/api/competition/:competition_id/ranking", competitionRankingHandler)
	e.GET("/player/api/competitions", competitionsHandler)

	// for admin endpoint
	e.POST("/admin/api/tenants/add", tenantsAddHandler)
	e.GET("/admin/api/tenants/billing", tenantsBillingHandler)

	e.HTTPErrorHandler = errorResponseHandler

	var err error
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
	playerName string
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
		return nil, fmt.Errorf("error os.ReadFile: %w", err)
	}
	key, _, err := jwk.DecodePEM(keysrc)
	if err != nil {
		return nil, fmt.Errorf("error jwk.DecodePEM: %w", err)
	}

	token, err := jwt.Parse([]byte(tokenStr), jwt.WithKey(jwa.RS256, key))
	if err != nil {
		return nil, fmt.Errorf("error parse: %w", err)
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
		playerName: token.Subject(),
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
	ID             int64     `db:"id"`
	Name           string    `db:"name"`
	DisplayName    string    `db:"display_name"`
	IsDisqualified bool      `db:"is_disqualified"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

func retrievePlayerByName(ctx context.Context, tenantDB dbOrTx, name string) (*PlayerRow, error) {
	var c PlayerRow
	if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM player WHERE name = ?", name); err != nil {
		return nil, fmt.Errorf("error Select player: %w", err)
	}
	return &c, nil
}

func retrievePlayer(ctx context.Context, tenantDB dbOrTx, id int64) (*PlayerRow, error) {
	var c PlayerRow
	if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM player WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select player: %w", err)
	}
	return &c, nil
}

type CompetitionRow struct {
	ID         int64        `db:"id"`
	Title      string       `db:"title"`
	FinishedAt sql.NullTime `db:"finished_at"`
	CreatedAt  time.Time    `db:"created_at"`
	UpdatedAt  time.Time    `db:"updated_at"`
}

func retrieveCompetition(ctx context.Context, tenantDB dbOrTx, id int64) (*CompetitionRow, error) {
	var c CompetitionRow
	if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM competition WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select competition: %w", err)
	}
	return &c, nil
}

type PlayerScoreRow struct {
	ID            int64     `db:"id"`
	PlayerID      int64     `db:"player_id"`
	CompetitionID int64     `db:"competition_id"`
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

	ctx := c.Request().Context()
	tx, err := centerDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error centerDB.BeginTxx: %w", err)
	}
	id, err := dispenseID(ctx)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error dispenseID: %w", err)
	}
	name := fmt.Sprintf("tenant-%d", id)
	now := time.Now()
	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO `tenant` (`id`, `name`, `display_name`, `created_at`, `updated_at`) VALUES (?, ?, ?, ?, ?)",
		id, name, displayName, now, now,
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error Insert tenant: %w", err)
	}

	if err := createTenantDB(name); err != nil {
		tx.Rollback()
		return fmt.Errorf("error createTenantDB: %w", err)
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

type BillingReport struct {
	CompetitionID    int64  `json:"competition_id" db:"competition_id"`
	CompetitionTitle string `json:"competition_title" db:"competition_title"`
	PlayerCount      int64  `json:"player_count" db:"player_count"`
	BillingYen       int64  `json:"billing_yen" db:"billing_yen"`
}

type VisitHistoryRow struct {
	PlayerName    string    `db:"player_name"`
	TenantID      int64     `db:"tenant_id"`
	CompetitionID int64     `db:"competition_id"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type VisitHistorySummaryRow struct {
	PlayerName   string    `db:"player_name"`
	MinCreatedAt time.Time `db:"min_created_at"`
}

var billingGroup = singleflight.Group{}

func billingReportByCompetition(ctx context.Context, tenantDB dbOrTx, tenantID, competitonID int64) (*BillingReport, error) {
	key := strconv.FormatInt(competitonID, 10)
	_r, err, _ := billingGroup.Do(key, func() (interface{}, error) {
		return billingReportByCompetitionInternal(ctx, tenantDB, tenantID, competitonID)
	})
	if err != nil {
		return nil, err
	}
	// log.Infof("billingReportByCompetition: competitionID=%s shared=%v", competitonID, shared)
	return _r.(*BillingReport), nil
}

func billingReportByCompetitionInternal(ctx context.Context, tenantDB dbOrTx, tenantID, competitonID int64) (*BillingReport, error) {
	comp, err := retrieveCompetition(ctx, tenantDB, competitonID)
	if err != nil {
		return nil, fmt.Errorf("error retrieveCompetition: %w", err)
	}
	// 確定済みならテーブルから返せる
	var report BillingReport
	err = tenantDB.GetContext(ctx, &report, `SELECT * FROM billing_report WHERE competition_id=?`, competitonID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error billingReportByCompetitionID: %w", err)
	}
	if report.CompetitionID != 0 {
		return &report, nil
	}

	vhs := []VisitHistorySummaryRow{}
	if err := centerDB.SelectContext(
		ctx,
		&vhs,
		"SELECT player_name, MIN(min_created_at) AS min_created_at FROM visit_history_s WHERE tenant_id = ? AND competition_id = ? GROUP BY player_name",
		tenantID,
		comp.ID,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error Select visit_history: %w", err)
	}
	billingMap := map[string]int64{}
	for _, vh := range vhs {
		// competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
		if comp.FinishedAt.Valid && comp.FinishedAt.Time.Before(vh.MinCreatedAt) {
			continue
		}
		// scoreに登録されていないplayerでアクセスした人 * 10
		billingMap[vh.PlayerName] = 10
	}

	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE competition_id = ?",
		comp.ID,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error Select count player_score: %w", err)
	}
	for _, ps := range pss {
		player, err := retrievePlayer(ctx, tenantDB, ps.PlayerID)
		if err != nil {
			return nil, fmt.Errorf("error retrievePlayer: %w", err)
		}
		if _, ok := billingMap[player.Name]; ok {
			// scoreに登録されているplayerでアクセスした人 * 100
			billingMap[player.Name] = 100
		} else {
			// scoreに登録されているplayerでアクセスしていない人 * 50
			billingMap[player.Name] = 50
		}
	}

	var billingYen int64
	for _, v := range billingMap {
		billingYen += v
	}
	r := &BillingReport{
		CompetitionID:    comp.ID,
		CompetitionTitle: comp.Title,
		PlayerCount:      int64(len(pss)),
		BillingYen:       billingYen,
	}

	if _, err := tenantDB.ExecContext(ctx,
		`REPLACE INTO billing_report (competition_id, competition_title, player_count, billing_yen) VALUES (?, ?, ?, ?)`,
		r.CompetitionID, r.CompetitionTitle, r.PlayerCount, r.BillingYen,
	); err != nil {
		return nil, err
	}

	return r, nil
}

type TenantWithBilling struct {
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
	// テナントごとに
	//   大会ごとに
	//     scoreに登録されているplayerでアクセスした人 * 100
	//     scoreに登録されているplayerでアクセスしていない人 * 50
	//     scoreに登録されていないplayerでアクセスした人 * 10
	//   を合計したものを
	// テナントの課金とする
	ts := []TenantRow{}
	if err := centerDB.SelectContext(ctx, &ts, "SELECT * FROM tenant ORDER BY name ASC"); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	tenantBillings := make([]TenantWithBilling, 0, len(ts))
	for _, t := range ts {
		if before != "" && before > t.Name {
			continue
		}
		tb := TenantWithBilling{
			Name:        t.Name,
			DisplayName: t.DisplayName,
		}
		tenantDB, err := connectToTenantDB(t.Name)
		if err != nil {
			return fmt.Errorf("error connectToTenantDB: %w", err)
		}

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
	Name           string `json:"name"`
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
			"INSERT INTO player (id, name, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, id, displayName, false, now, now,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error Insert player at tenantDB: %w", err)
		}
		p, err := retrievePlayer(ctx, ttx, id)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		pds = append(pds, PlayerDetail{
			Name:           p.Name,
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

	name := c.Param("player_name")

	now := time.Now()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE player SET is_disqualified = ?, updated_at = ? WHERE name = ?",
		true, now, name,
	); err != nil {
		return fmt.Errorf("error Update player: %w", err)
	}
	p, err := retrievePlayerByName(ctx, tenantDB, name)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}

	res := PlayerDisqualifiedHandlerResult{
		Player: PlayerDetail{
			Name:           p.Name,
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
	ID         int64  `json:"id"`
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
		return fmt.Errorf("error Insert competition: %w", err)
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

	idStr := c.Param("competition_id")
	var id int64
	if id, err = strconv.ParseInt(idStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
	}

	now := time.Now()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?",
		now, now, id,
	); err != nil {
		return fmt.Errorf("error Update competition: %w", err)
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

	competitionIDStr := c.Param("competition_id")
	var competitionID int64
	if competitionID, err = strconv.ParseInt(competitionIDStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
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
		return fmt.Errorf("error c.FormFile: %w", err)
	}
	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("error fh.Open: %w", err)
	}
	defer f.Close()

	now := time.Now()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return fmt.Errorf("error r.Read at header: %w", err)
	}
	if !reflect.DeepEqual(headers, []string{"player_name", "score"}) {
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
		return fmt.Errorf("error Delete player_score: %w", err)
	}
	playerScoreRows := make([]*PlayerScoreRow, 0, 100)
	playerScoreByName := make(map[string]int64)
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
		playerName, scoreStr := row[0], row[1]
		var score int64
		if score, err = strconv.ParseInt(scoreStr, 10, 64); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error strconv.ParseUint: %w", err)
		}
		playerScoreByName[playerName] = score
	}
	playerNames := make([]string, 0, len(playerScoreByName))
	for playerName, _ := range playerScoreByName {
		playerNames = append(playerNames, playerName)
	}
	players := make([]*PlayerRow, 0, len(playerNames))
	q, params, err := sqlx.In("SELECT id, name FROM player WHERE name IN(?)", playerNames)
	if err := tenantDB.SelectContext(ctx, &players, q, params...); err != nil {
		return fmt.Errorf("error Select players: %w", err)
	}

	for _, player := range players {
		id, _ := dispenseID(ctx)
		playerScoreRows = append(playerScoreRows, &PlayerScoreRow{
			ID:            id,
			PlayerID:      player.ID,
			CompetitionID: competitionID,
			Score:         playerScoreByName[player.Name],
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	if len(playerScoreRows) > 0 {
		if _, err := ttx.NamedExecContext(
			ctx,
			`INSERT INTO player_score (id, player_id, competition_id, score, created_at, updated_at)
			VALUES (:id, :player_id, :competition_id, :score, :created_at, :updated_at)`,
			playerScoreRows,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error BULK INSERT player_score: %w", err)
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

	var t TenantRow
	if err := centerDB.GetContext(
		ctx,
		&t,
		"SELECT * FROM tenant WHERE name = ?",
		tenantName,
	); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}

	cs := []CompetitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition ORDER BY id ASC",
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

	vp, err := retrievePlayerByName(c.Request().Context(), tenantDB, v.playerName)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	pn := c.Param("player_name")

	p, err := retrievePlayerByName(ctx, tenantDB, pn)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}
	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE player_id = ? ORDER BY competition_id ASC",
		p.ID,
	); err != nil {
		return fmt.Errorf("error Select player_score: %w", err)
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
				Name:           p.Name,
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
	PlayerName        string `json:"player_name"`
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

	competitionIDStr := c.Param("competition_id")
	var competitionID int64
	if competitionID, err = strconv.ParseInt(competitionIDStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
	}

	tenantName, err := getTenantName(c)
	if err != nil {
		return fmt.Errorf("error getTenantName: %w", err)
	}
	tenantDB, err := connectToTenantDB(tenantName)
	if err != nil {
		return fmt.Errorf("error connectToTenantDB: %w", err)
	}

	vp, err := retrievePlayerByName(c.Request().Context(), tenantDB, v.playerName)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	now := time.Now()
	var t TenantRow
	if err := centerDB.GetContext(ctx, &t, "SELECT id, name FROM tenant WHERE name = ?", v.tenantName); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}

	var count = struct {
		C int64 `db:"c"`
	}{}
	if err := centerDB.GetContext(
		ctx,
		&count,
		"SELECT count(*) AS c FROM visit_history_s WHERE tenant_id = ? AND competition_id = ? AND player_name = ?",
		t.ID, competitionID, vp.Name,
	); err != nil {
		return fmt.Errorf("error Select visit_history_s: %w", err)
	}
	if count.C == 0 {
		// 初回アクセス時だけ記録する
		if _, err := centerDB.ExecContext(
			ctx,
			"INSERT INTO visit_history_s (player_name, tenant_id, competition_id, min_created_at) VALUES (?, ?, ?, ?)",
			vp.Name, t.ID, competitionID, now,
		); err != nil {
			return fmt.Errorf("error Insert visit_history: %w", err)
		}
	}

	var rankAfter int64
	rankAfterStr := c.QueryParam("rank_after")
	if rankAfterStr != "" {
		if rankAfter, err = strconv.ParseInt(rankAfterStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: %w", err)
		}
	}

	pss := []PlayerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE competition_id = ? ORDER BY score DESC, player_id DESC",
		competitionID,
	); err != nil {
		return fmt.Errorf("error Select player_score: %w", err)
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
			PlayerName:        co.Name,
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

	vp, err := retrievePlayerByName(c.Request().Context(), tenantDB, v.playerName)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	cs := []CompetitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition ORDER BY id ASC",
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

const initializeMaxID = 2678400000

var initializeMaxVisitHistoryCreatedAt = time.Date(2022, 05, 31, 23, 59, 59, 0, time.UTC)

type InitializeHandlerResult struct {
	Lang   string `json:"lang"`
	Appeal string `json:"appeal"`
}

func initializeHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// constに定義されたmax_idより大きいIDのtenantを削除
	dtns := []string{}
	if err := centerDB.SelectContext(
		ctx,
		&dtns,
		"SELECT name FROM tenant WHERE id > ?",
		initializeMaxID,
	); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	for _, tn := range dtns {
		p := tenantDBPath(tn)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error os.Remove: %w", err)
		}
	}
	if _, err := centerDB.ExecContext(
		ctx,
		"DELETE FROM tenant WHERE id > ?",
		initializeMaxID,
	); err != nil {
		return fmt.Errorf("error Delete tenant: %w", err)
	}
	// constに定義されたmax_visit_historyより大きいCreatedAtのvisit_historyを削除
	if _, err := centerDB.ExecContext(
		ctx,
		"DELETE FROM visit_history_s WHERE min_created_at > ?",
		initializeMaxVisitHistoryCreatedAt,
	); err != nil {
		return fmt.Errorf("error Delete visit_history: %w", err)
	}
	// constに定義されたmax_idにid_generatorを戻す
	if _, err := centerDB.ExecContext(
		ctx,
		"UPDATE id_generator SET id = ? WHERE stub = ?",
		initializeMaxID, "a",
	); err != nil {
		return fmt.Errorf("error Update id_generator: %w", err)
	}
	if _, err := centerDB.ExecContext(
		ctx,
		fmt.Sprintf("ALTER TABLE id_generator AUTO_INCREMENT = %d", initializeMaxID),
	); err != nil {
		return fmt.Errorf("error Update id_generator: %w", err)
	}

	// 残ったtenantのうち、max_idより大きいcompetition, player, player_scoreを削除
	utns := []string{}
	if err := centerDB.SelectContext(
		ctx,
		&utns,
		"SELECT name FROM tenant",
	); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	for _, tn := range utns {
		err := func() error {
			tenantDB, err := connectToTenantDB(tn)
			if err != nil {
				return fmt.Errorf("error connectToTenantDB: %w", err)
			}
			if _, err := tenantDB.ExecContext(ctx, "DELETE FROM competition WHERE id > ?", initializeMaxID); err != nil {
				return fmt.Errorf("error Delete competition: tenant=%s %w", tn, err)
			}
			if _, err := tenantDB.ExecContext(ctx, "DELETE FROM player WHERE id > ?", initializeMaxID); err != nil {
				return fmt.Errorf("error Delete player: tenant=%s %w", tn, err)
			}
			if _, err := tenantDB.ExecContext(ctx, "DELETE FROM player_score WHERE id > ?", initializeMaxID); err != nil {
				return fmt.Errorf("error Delete player: tenant=%s %w", tn, err)
			}
			// billing確定テーブルを作る
			if _, err := tenantDB.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS billing_report (
					competition_id INTEGER NOT NULL PRIMARY KEY,
					competition_title TEXT NOT NULL,
					player_count INTEGER NOT NULL,
					billing_yen INTEGER NOT NULL
				)`); err != nil {
				return fmt.Errorf("error Create billing_report: tenant=%s %w", tn, err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
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
