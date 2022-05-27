package main

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
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	_ "github.com/mattn/go-sqlite3"
)

const (
	tenantDBSchemaFilePath = "../sql/20_schema_tenant.sql"
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
	config.DBName = getEnv("ISUCON_DB_NAME", "isucon_listen80")
	config.ParseTime = true

	dsn := config.FormatDSN()
	return sqlx.Open("mysql", dsn)
}

func tenantDBPath(tenantID string) string {
	tenantDBDir := getEnv("ISUCON_TENANT_DB_DIR", "./tenants")
	return filepath.Join(tenantDBDir, tenantID+".db")
}

func connectTenantDB(tenantID string) (*sqlx.DB, error) {
	p := tenantDBPath(tenantID)
	return sqlx.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw", p))
}

func connectTenantDBByHost(c echo.Context) (*sqlx.DB, error) {
	baseHost := getEnv("ISUCON_BASE_HOSTNAME", ".isuports.isucon.local")
	host := c.Request().Host
	if !strings.HasSuffix(host, baseHost) {
		return nil, fmt.Errorf("host is not contains %s: %s", baseHost, host)
	}
	tenantIdentifier := strings.TrimSuffix(host, baseHost)

	tenantDB, err := connectTenantDB(tenantIdentifier)
	if err != nil {
		return nil, fmt.Errorf("error connectTenantDB: %w", err)
	}
	return tenantDB, nil
}

func createTenantDB(tenantID string) error {
	p := tenantDBPath(tenantID)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("sqlite3 %s < %s", p, tenantDBSchemaFilePath))
	return cmd.Run()
}

func dispenseID(ctx context.Context) (int64, error) {
	ret, err := centerDB.ExecContext(ctx, "REPLACE INTO `id_generator` (`stub`) VALUES (?);", "a")
	if err != nil {
		return 0, fmt.Errorf("error REPLACE INTO `id_generator`: %w", err)
	}
	return ret.LastInsertId()
}

var centerDB *sqlx.DB

func main() {
	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.DEBUG)

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// for benchmarker
	e.POST("/initialize", initializeHandler)

	// for tenant endpoint
	// 参加者操作
	e.POST("/organizer/api/players/add", playersAddHandler)
	e.POST("/organizer/api/player/:competitior_id/disqualified", playerDisqualifiedHandler)
	// 大会操作
	e.POST("/organizer/api/competitions/add", competitionsAddHandler)
	e.POST("/organizer/api/competition/:competition_id/finish", competitionFinishHandler)
	e.POST("/organizer/api/competition/:competition_id/result", competitionResultHandler)
	// テナント操作
	e.GET("/organizer/api/billing", billingHandler)
	// 参加者からの閲覧
	e.GET("/player/api/player/:player_identifier", playerHandler)
	e.GET("/player/api/competition/:competition_id/ranking", competitionRankingHandler)
	e.GET("/player/api/competitions", competitionsHandler)

	// for admin endpoint
	e.POST("/admin/api/tenants/add", tenantsAddHandler)
	e.GET("/admin/api/tenants/billing", tenantsBillingHandler)

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

type successResult struct {
	Success bool `json:"status"`
	Data    any  `json:"data,omitempty"`
}

type failureResult struct {
	Success bool   `json:"status"`
	Message string `json:"message"`
}

type role int

const (
	roleAdmin role = iota + 1
	roleOrganizer
	rolePlayer
)

var (
	errNotPermitted = errors.New("this role is not permitted")
)

type viewer struct {
	role             role
	playerIdentifier string
	tenantIdentifier string
}

func parseViewer(c echo.Context) (*viewer, error) {
	// TODO: JWTをパースして権限などを確認する
	return nil, nil
}

func parseViewerMustAdmin(c echo.Context) (*viewer, error) {
	v, err := parseViewer(c)
	if err != nil {
		return nil, fmt.Errorf("error parseViewer:%w", err)
	}
	if v.role != roleAdmin {
		return nil, errNotPermitted
	}
	return v, nil
}

func parseViewerMustOrganizer(c echo.Context) (*viewer, error) {
	v, err := parseViewer(c)
	if err != nil {
		return nil, fmt.Errorf("error parseViewer:%w", err)
	}
	if v.role != roleOrganizer {
		return nil, errNotPermitted
	}
	return v, nil
}

func parseViewerIgnoreDisqualified(c echo.Context, competitionID int64) (*viewer, error) {
	ctx := c.Request().Context()

	v, err := parseViewer(c)
	if err != nil {
		return nil, fmt.Errorf("error parseViewer:%w", err)
	}
	if v.role == rolePlayer {
		tenantDB, err := connectTenantDBByHost(c)
		if err != nil {
			return nil, fmt.Errorf("error connectTenantDBByHost: %w", err)
		}
		p, err := retrievePlayerByIdentifier(c.Request().Context(), tenantDB, v.playerIdentifier)
		if err != nil {
			return nil, fmt.Errorf("error retrievePlayerByIdentifier: %w", err)
		}
		if p.IsDisqualified {
			return nil, errNotPermitted
		}

		now := time.Now()
		id, err := dispenseID(ctx)
		if err != nil {
			return nil, fmt.Errorf("error dispenseID: %w", err)
		}
		t, err := retrieveTenantByIdentifier(ctx, v.tenantIdentifier)
		if err != nil {
			return nil, fmt.Errorf("error retrieveTenantByIdentifier: %w", err)
		}
		if _, err := centerDB.ExecContext(
			ctx,
			"INSERT INTO access_log (id, identifier, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at)",
			id, p.Identifier, t.ID, competitionID, now, now,
		); err != nil {
			return nil, fmt.Errorf("error Insert access_log: %w", err)
		}
		return nil, errNotPermitted
	}
	return v, nil
}

type tenantRow struct {
	ID         int64
	Identifier string
	Name       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func retrieveTenantByIdentifier(ctx context.Context, identifier string) (*tenantRow, error) {
	var t tenantRow
	if err := centerDB.SelectContext(ctx, &t, "SELECT * FROM tenant WHERE identifier = ?", identifier); err != nil {
		return nil, fmt.Errorf("error Select tenant: %w", err)
	}
	return &t, nil
}

type dbOrTx interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type accessLogRow struct {
	ID            int64
	Identifier    string
	TenantID      int64
	CompetitionID int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type playerRow struct {
	ID             int64
	Identifier     string
	Name           string
	IsDisqualified bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func retrievePlayerByIdentifier(ctx context.Context, tenantDB dbOrTx, identifier string) (*playerRow, error) {
	var c playerRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM player WHERE identifier = ?", identifier); err != nil {
		return nil, fmt.Errorf("error Select player: %w", err)
	}
	return &c, nil
}

func retrievePlayer(ctx context.Context, tenantDB dbOrTx, id int64) (*playerRow, error) {
	var c playerRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM player WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select player: %w", err)
	}
	return &c, nil
}

type competitionRow struct {
	ID         int64
	Title      string
	FinishedAt sql.NullTime
	FinishedID sql.NullInt64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func retrieveCompetition(ctx context.Context, tenantDB dbOrTx, id int64) (*competitionRow, error) {
	var c competitionRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM competition WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select competition: %w", err)
	}
	return &c, nil
}

type playerScoreRow struct {
	ID            int64
	PlayerID      int64
	CompetitionID int64
	Score         int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func tenantsAddHandler(c echo.Context) error {
	_, err := parseViewerMustAdmin(c)
	if err != nil {
		return fmt.Errorf("error parseViewerMustAdmin: %w", err)
	}

	name := c.FormValue("name")

	ctx := c.Request().Context()
	tx, err := centerDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error centerDB.BeginTxx: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "LOCK TABLE `tenant` WRITE"); err != nil {
		tx.Rollback()
		return fmt.Errorf("error Lock table: %w", err)
	}
	id, err := dispenseID(ctx)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error dispenseID: %w", err)
	}
	identifier := strconv.FormatInt(id, 10)
	now := time.Now()
	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO `tenant` (`id`, `identifier`, `name`, `created_at`, `updated_at`)",
		id, identifier, name, now, now,
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error Insert tenant: %w", err)
	}

	if err := createTenantDB(identifier); err != nil {
		tx.Rollback()
		return fmt.Errorf("error createTenantDB: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error tx.Commit: %w", err)
	}

	if err := c.JSON(http.StatusOK, successResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type billingReport struct {
	CompetitionID    int64
	CompetitionTitle string
	PlayerCount      int64
	BillingYen       int64
}

func billingReportByCompetition(ctx context.Context, tenantDB dbOrTx, competitonID int64) (*billingReport, error) {
	comp, err := retrieveCompetition(ctx, tenantDB, competitonID)
	if err != nil {
		return nil, fmt.Errorf("error retrieveCompetition: %w", err)
	}

	aals := []accessLogRow{}
	if err := centerDB.SelectContext(
		ctx,
		aals,
		"SELECT * FROM access_log WHERE competition_id = ?",
		comp.ID,
	); err != nil {
		return nil, fmt.Errorf("error Select access_log: %w", err)
	}
	billingMap := map[string]int64{}
	for _, aal := range aals {
		// competition.finished_idよりも大きいidの場合は、終了後にアクセスしたとみなしてアクセスしたとみなさない
		if comp.FinishedID.Valid && comp.FinishedID.Int64 < aal.ID {
			continue
		}
		billingMap[aal.Identifier] = 10
	}

	css := []playerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&css,
		"SELECT * FROM player_score WHERE competition_id = ?",
		comp.ID,
	); err != nil {
		return nil, fmt.Errorf("error Select count player_score: %w", err)
	}
	for _, cs := range css {
		player, err := retrievePlayer(ctx, tenantDB, cs.PlayerID)
		if err != nil {
			return nil, fmt.Errorf("error retrievePlayer: %w", err)
		}
		if _, ok := billingMap[player.Identifier]; ok {
			billingMap[player.Identifier] = 100
		} else {
			billingMap[player.Identifier] = 50
		}
	}

	var billingYen int64
	for _, v := range billingMap {
		billingYen += v
	}
	return &billingReport{
		CompetitionID:    comp.ID,
		CompetitionTitle: comp.Title,
		PlayerCount:      int64(len(css)),
		BillingYen:       billingYen,
	}, nil
}

type tenantsBillingHandlerResult struct {
	Tenants []tenantBilling
}

type tenantBilling struct {
	TenantIdentifier string `json:"tenant_identifier"`
	TenantName       string `json:"tenant_name"`
	Billing          int64  `json:"billing"`
}

func tenantsBillingHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if _, err := parseViewerMustAdmin(c); err != nil {
		return fmt.Errorf("error parseViewerMustAdmin: %w", err)
	}

	// テナントごとに
	//   大会ごとに
	//     scoreに登録されているplayerでアクセスした人 * 100
	//     scoreに登録されているplayerでアクセスしていない人 * 50
	//     scoreに登録されていないplayerでアクセスした人 * 10
	//   を合計したものを
	// テナントの課金とする
	ts := []tenantRow{}
	if err := centerDB.SelectContext(ctx, &ts, "SELECT * FROM tenant ORDER BY id ASC"); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	tenantBillings := make([]tenantBilling, 0, len(ts))
	for _, t := range ts {
		tb := tenantBilling{
			TenantIdentifier: t.Identifier,
			TenantName:       t.Name,
		}
		tenantDB, err := connectTenantDB(t.Identifier)
		if err != nil {
			return fmt.Errorf("error connectTenantDB: %w", err)
		}
		defer tenantDB.Close()
		cs := []competitionRow{}
		if err := tenantDB.SelectContext(
			ctx,
			&cs,
			"SELECT * FROM competition",
		); err != nil {
			return fmt.Errorf("error Select competition: %w", err)
		}
		for _, comp := range cs {
			report, err := billingReportByCompetition(ctx, tenantDB, comp.ID)
			if err != nil {
				return fmt.Errorf("error billingReportByCompetition: %w", err)
			}
			tb.Billing += report.BillingYen
		}
		tenantBillings = append(tenantBillings, tb)
	}
	if err := c.JSON(http.StatusOK, successResult{
		Success: true,
		Data:    tenantBillings,
	}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type playerDetail struct {
	Identifier     string `json:"identifier"`
	Name           string `json:"name"`
	IsDisqualified bool   `json:"is_disqualified"`
}

type playersAddHandlerResult struct {
	Players []playerDetail `json:"players"`
}

func playersAddHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerMustOrganizer(c); err != nil {
		return fmt.Errorf("error parseViewerMustOrganizer: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	params, err := c.FormParams()
	if err != nil {
		return fmt.Errorf("error c.FormParams: %w", err)
	}
	names := params["name"]

	now := time.Now()
	ttx, err := tenantDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error tenantDB.BeginTxx: %w", err)
	}
	pds := make([]playerDetail, 0, len(names))
	for _, name := range names {
		id, err := dispenseID(ctx)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error dispenseID: %w", err)
		}

		if _, err := ttx.ExecContext(
			ctx,
			"INSERT INTO player (id, identifier, name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, id, name, false, now, now,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error Insert player at tenantDB: %w", err)
		}
		p, err := retrievePlayer(ctx, ttx, id)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		pds = append(pds, playerDetail{
			Identifier:     p.Identifier,
			Name:           p.Name,
			IsDisqualified: p.IsDisqualified,
		})
	}
	if err := ttx.Commit(); err != nil {
		return fmt.Errorf("error ttx.Commit: %w", err)
	}

	res := playersAddHandlerResult{
		Players: pds,
	}
	if err := c.JSON(http.StatusOK, successResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type playerDisqualifiedHandlerResult struct {
	Player playerDetail `json:"player"`
}

func playerDisqualifiedHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerMustOrganizer(c); err != nil {
		return fmt.Errorf("error parseViewerMustOrganizer: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	identifier := c.Param("player_identifier")

	now := time.Now()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE player SET is_disqualified = ?, updated_at = ? WHERE identifier = ?",
		true, now, identifier,
	); err != nil {
		return fmt.Errorf("error Update player: %w", err)
	}
	p, err := retrievePlayerByIdentifier(ctx, tenantDB, identifier)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByIdentifier: %w", err)
	}

	res := playerDisqualifiedHandlerResult{
		Player: playerDetail{
			Identifier:     p.Identifier,
			Name:           p.Name,
			IsDisqualified: p.IsDisqualified,
		},
	}
	if err := c.JSON(http.StatusOK, successResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type competitionDetail struct {
	ID         int64
	Title      string
	IsFinished bool
}

type competitionsAddHandlerResult struct {
	Competition competitionDetail
}

func competitionsAddHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerMustOrganizer(c); err != nil {
		return fmt.Errorf("error parseViewerMustOrganizer: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
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
		"INSERT INTO competition (id, title, finished_at, finished_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, title, sql.NullTime{}, sql.NullInt64{}, now, now,
	); err != nil {
		return fmt.Errorf("error Insert competition: %w", err)
	}

	res := competitionsAddHandlerResult{
		Competition: competitionDetail{
			ID:         id,
			Title:      title,
			IsFinished: false,
		},
	}
	if err := c.JSON(http.StatusOK, successResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitionFinishHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerMustOrganizer(c); err != nil {
		return fmt.Errorf("error parseViewerMustOrganizer: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	idStr := c.Param("competition_id")
	var id int64
	if id, err = strconv.ParseInt(idStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
	}

	finishedID, err := dispenseID(ctx)
	if err != nil {
		return fmt.Errorf("error dispenseID: %w", err)
	}

	now := time.Now()
	if _, err := tenantDB.ExecContext(
		ctx,
		"UPDATE competition SET finished_at = ?, finished_id = ?, updated_at = ? WHERE id = ?",
		now, finishedID, now, id,
	); err != nil {
		return fmt.Errorf("error Update competition: %w", err)
	}

	if err := c.JSON(http.StatusOK, successResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitionResultHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerMustOrganizer(c); err != nil {
		return fmt.Errorf("error parseViewerMustOrganizer: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	competitionIDStr := c.Param("competition_id")
	var competitionID int64
	if competitionID, err = strconv.ParseInt(competitionIDStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
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
	if !reflect.DeepEqual(headers, []string{"player_identifier", "score"}) {
		return fmt.Errorf("not match header: %#v", headers)
	}
	ttx, err := tenantDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error tenantDB.BeginTxx: %w", err)
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
		playerIdentifier, scoreStr := row[0], row[1]
		c, err := retrievePlayerByIdentifier(ctx, tenantDB, playerIdentifier)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrievePlayerByIdentifier: %w", err)
		}
		var score int64
		if score, err = strconv.ParseInt(scoreStr, 10, 64); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error strconv.ParseUint: %w", err)
		}
		id, err := dispenseID(ctx)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error dispenseID: %w", err)
		}
		if _, err := ttx.ExecContext(
			ctx,
			"REPLACE INTO player_score (id, player_id, competition_id, score, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, c.ID, competitionID, score, now, now,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error Update competition: %w", err)
		}
	}

	if err := ttx.Commit(); err != nil {
		return fmt.Errorf("error txx.Commit: %w", err)
	}

	if err := c.JSON(http.StatusOK, successResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type billingHandlerResult struct {
	Reports []billingReport
}

func billingHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerMustOrganizer(c); err != nil {
		return fmt.Errorf("error parseViewerMustOrganizer: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	cs := []competitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition ORDER BY id ASC",
	); err != nil {
		return fmt.Errorf("error Select competition: %w", err)
	}
	tbrs := make([]billingReport, 0, len(cs))
	for _, comp := range cs {
		report, err := billingReportByCompetition(ctx, tenantDB, comp.ID)
		if err != nil {
			return fmt.Errorf("error billingReportByCompetition: %w", err)
		}
		tbrs = append(tbrs, *report)
	}

	res := successResult{
		Success: true,
		Data: billingHandlerResult{
			Reports: tbrs,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type playerScoreDetail struct {
	CompetitionTitle string `json:"competition_title"`
	Score            int64  `json:"score"`
}

type playerHandlerResult struct {
	Name   string              `json:"name"`
	Scores []playerScoreDetail `json:"scores"`
}

func playerHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerIgnoreDisqualified(c, 0); err != nil {
		return fmt.Errorf("error parseViewerIgnoreDisqualified: %w", err)
	}
	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	ci := c.Param("player_identifier")

	co, err := retrievePlayerByIdentifier(ctx, tenantDB, ci)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByIdentifier: %w", err)
	}
	css := []playerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&css,
		"SELECT * FROM player_score WHERE player_id = ? ORDER BY competition_id ASC",
		co.ID,
	); err != nil {
		return fmt.Errorf("error Select player_score: %w", err)
	}
	csds := make([]playerScoreDetail, 0, len(css))
	for _, cs := range css {
		comp, err := retrieveCompetition(ctx, tenantDB, cs.CompetitionID)
		if err != nil {
			return fmt.Errorf("error retrieveCompetition: %w", err)
		}
		csds = append(csds, playerScoreDetail{
			CompetitionTitle: comp.Title,
			Score:            cs.Score,
		})
	}

	res := successResult{
		Success: true,
		Data: playerHandlerResult{
			Name:   co.Name,
			Scores: csds,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type competitionRank struct {
	Rank             int64  `json:"rank"`
	Score            int64  `json:"score"`
	PlayerIdentifier string `json:"player_identifier"`
	CompetitiorName  string `json:"competitior_name"`
}

type competitionRankingHandlerResult struct {
	Ranks []competitionRank `json:"ranks"`
}

func competitionRankingHandler(c echo.Context) error {
	ctx := c.Request().Context()

	competitionIDStr := c.Param("competition_id")
	var competitionID int64
	var err error
	if competitionID, err = strconv.ParseInt(competitionIDStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
	}

	if _, err := parseViewerIgnoreDisqualified(c, competitionID); err != nil {
		return fmt.Errorf("error parseViewerIgnoreDisqualified: %w", err)
	}

	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	var rankAfter int64
	rankAfterStr := c.QueryParam("rank_after")
	if rankAfterStr != "" {
		if rankAfter, err = strconv.ParseInt(rankAfterStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: %w", err)
		}
	}

	css := []playerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&css,
		"SELECT * FROM player_score WHERE competition_id = ? ORDER BY score DESC, player_id DESC",
		competitionID,
	); err != nil {
		return fmt.Errorf("error Select player_score: %w", err)
	}
	crs := make([]competitionRank, 0, len(css))
	for i, cs := range css {
		co, err := retrievePlayer(ctx, tenantDB, cs.PlayerID)
		if err != nil {
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		if int64(i) < rankAfter {
			continue
		}
		crs = append(crs, competitionRank{
			Rank:             int64(i + 1),
			Score:            cs.Score,
			PlayerIdentifier: co.Identifier,
			CompetitiorName:  co.Name,
		})
	}

	res := successResult{
		Success: true,
		Data: competitionRankingHandlerResult{
			Ranks: crs,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type competitionsHandlerResult struct {
	Competitions []competitionDetail
}

func competitionsHandler(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := parseViewerIgnoreDisqualified(c, 0); err != nil {
		return fmt.Errorf("error parseViewerIgnoreDisqualified: %w", err)
	}

	tenantDB, err := connectTenantDBByHost(c)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByHost: %w", err)
	}
	defer tenantDB.Close()

	cs := []competitionRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&cs,
		"SELECT * FROM competition ORDER BY id ASC",
	); err != nil {
		return fmt.Errorf("error Select competition: %w", err)
	}
	cds := make([]competitionDetail, 0, len(cs))
	for _, comp := range cs {
		cds = append(cds, competitionDetail{
			ID:         comp.ID,
			Title:      comp.Title,
			IsFinished: comp.FinishedAt.Valid,
		})
	}

	res := successResult{
		Success: true,
		Data: competitionsHandlerResult{
			Competitions: cds,
		},
	}
	if err := c.JSON(http.StatusOK, res); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

func initializeHandler(c echo.Context) error {
	// TODO: SaaS管理者かチェック

	// constに定義されたmax_idより大きいIDのtenantを削除
	// constに定義されたmax_idより大きいIDのaccess_logを削除
	// constに定義されたmax_idにid_generatorを戻す
	// 残ったtenantのうち、max_idより大きいcompetition, player, player_scoreを削除

	return nil
}
