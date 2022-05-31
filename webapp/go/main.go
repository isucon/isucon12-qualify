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
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	_ "github.com/mattn/go-sqlite3"
)

const (
	tenantDBSchemaFilePath = "../sql/20_schema_tenant.sql"
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

	dsn := config.FormatDSN()
	return sqlx.Open("mysql", dsn)
}

func tenantDBPath(name string) string {
	tenantDBDir := getEnv("ISUCON_TENANT_DB_DIR", "./tenants")
	return filepath.Join(tenantDBDir, name+".db")
}

func connectToTenantDB(name string) (*sqlx.DB, error) {
	p := tenantDBPath(name)
	return sqlx.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw", p))
}

func getTenantName(c echo.Context) (string, error) {
	baseHost := getEnv("ISUCON_BASE_HOSTNAME", ".isuports.isucon.local")
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
	c.JSON(http.StatusInternalServerError, failureResult{
		Success: false,
		Message: err.Error(),
	})
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
	role       role
	playerName string
	tenantName string
}

func parseViewer(c echo.Context) (*viewer, error) {
	cookie, err := c.Request().Cookie(cookieName)
	if err != nil {
		return nil, fmt.Errorf("error c.Request().Cookie: %w", err)
	}
	tokenStr := cookie.Value

	keysrc := getEnv("ISUCON_JWT_KEY", "")
	key, err := jwk.ParseKey([]byte(keysrc))
	if err != nil {
		return nil, fmt.Errorf("error jwk.ParseKey: %w", err)
	}

	token, err := jwt.Parse([]byte(tokenStr), jwt.WithKey(jwa.RS256, key))
	if err != nil {
		return nil, fmt.Errorf("error parseViewer: %w", err)
	}
	var r role
	tr, ok := token.Get("role")
	if !ok {
		return nil, fmt.Errorf("token is invalid, not have role field: %s", tokenStr)
	}
	switch tr {
	case "admin":
		r = roleAdmin
	case "organizer":
		r = roleOrganizer
	case "player":
		r = rolePlayer
	default:
		return nil, fmt.Errorf("token is invalid, unknown role: %s", tokenStr)
	}
	aud := token.Audience()
	if len(aud) != 1 {
		return nil, fmt.Errorf("token is invalid, aud field is few or too many: %s", tokenStr)
	}

	v := &viewer{
		role:       r,
		playerName: token.Subject(),
		tenantName: aud[0],
	}
	return v, nil
}

type tenantRow struct {
	ID          int64
	Name        string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type dbOrTx interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type accessLogRow struct {
	ID            int64
	PlayerName    string
	TenantID      int64
	CompetitionID int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type playerRow struct {
	ID             int64
	Name           string
	DisplayName    string
	IsDisqualified bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func retrievePlayerByName(ctx context.Context, tenantDB dbOrTx, name string) (*playerRow, error) {
	var c playerRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM player WHERE name = ?", name); err != nil {
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

type tenantDetail struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type tenantsAddHandlerResult struct {
	Tenant tenantDetail `json:"tenant"`
}

func tenantsAddHandler(c echo.Context) error {
	if c.Request().Host != getEnv("ISUCON_ADMIN_HOSTNAME", "isuports-admin.isucon.local") {
		return echo.ErrNotFound
	}

	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != roleAdmin {
		return errNotPermitted
	}

	displayName := c.FormValue("display_name")

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
	name := strconv.FormatInt(id, 10)
	now := time.Now()
	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO `tenant` (`id`, `name`, `display_name`, `created_at`, `updated_at`)",
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

	res := tenantsAddHandlerResult{
		Tenant: tenantDetail{
			Name:        name,
			DisplayName: displayName,
		},
	}
	if err := c.JSON(http.StatusOK, successResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type billingReport struct {
	CompetitionID    int64  `json:"competition_id"`
	CompetitionTitle string `json:"competition_title"`
	PlayerCount      int64  `json:"player_count"`
	BillingYen       int64  `json:"billing_yen"`
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
		// competition.finished_atよりもあとの場合は、終了後にアクセスしたとみなしてアクセスしたとみなさない
		if comp.FinishedAt.Valid && comp.FinishedAt.Time.Before(aal.UpdatedAt) {
			continue
		}
		billingMap[aal.PlayerName] = 10
	}

	pss := []playerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE competition_id = ?",
		comp.ID,
	); err != nil {
		return nil, fmt.Errorf("error Select count player_score: %w", err)
	}
	for _, ps := range pss {
		player, err := retrievePlayer(ctx, tenantDB, ps.PlayerID)
		if err != nil {
			return nil, fmt.Errorf("error retrievePlayer: %w", err)
		}
		if _, ok := billingMap[player.Name]; ok {
			billingMap[player.Name] = 100
		} else {
			billingMap[player.Name] = 50
		}
	}

	var billingYen int64
	for _, v := range billingMap {
		billingYen += v
	}
	return &billingReport{
		CompetitionID:    comp.ID,
		CompetitionTitle: comp.Title,
		PlayerCount:      int64(len(pss)),
		BillingYen:       billingYen,
	}, nil
}

type tenantWithBilling struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	BillingYen  int64  `json:"billing"`
}

type tenantsBillingHandlerResult struct {
	Tenants []tenantWithBilling `json:"tenants"`
}

func tenantsBillingHandler(c echo.Context) error {
	if c.Request().Host != getEnv("ISUCON_ADMIN_HOSTNAME", "isuports-admin.isucon.local") {
		return echo.ErrNotFound
	}

	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role != roleAdmin {
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
	ts := []tenantRow{}
	if err := centerDB.SelectContext(ctx, &ts, "SELECT * FROM tenant ORDER BY name ASC"); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	tenantBillings := make([]tenantWithBilling, 0, len(ts))
	for _, t := range ts {
		if before != "" && before > t.Name {
			continue
		}
		tb := tenantWithBilling{
			Name:        t.Name,
			DisplayName: t.DisplayName,
		}
		tenantDB, err := connectToTenantDB(t.Name)
		if err != nil {
			return fmt.Errorf("error connectToTenantDB: %w", err)
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
			tb.BillingYen += report.BillingYen
		}
		tenantBillings = append(tenantBillings, tb)
		if len(tenantBillings) >= 20 {
			break
		}
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
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	IsDisqualified bool   `json:"is_disqualified"`
}

type playersAddHandlerResult struct {
	Players []playerDetail `json:"players"`
}

func playersAddHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role == roleOrganizer {
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
	pds := make([]playerDetail, 0, len(displayNames))
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
		pds = append(pds, playerDetail{
			Name:           p.Name,
			DisplayName:    p.DisplayName,
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
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role == roleOrganizer {
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

	res := playerDisqualifiedHandlerResult{
		Player: playerDetail{
			Name:           p.Name,
			DisplayName:    p.DisplayName,
			IsDisqualified: p.IsDisqualified,
		},
	}
	if err := c.JSON(http.StatusOK, successResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

type competitionDetail struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	IsFinished bool   `json:"is_finished"`
}

type competitionsAddHandlerResult struct {
	Competition competitionDetail `json:"competition"`
}

func competitionsAddHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role == roleOrganizer {
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
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role == roleOrganizer {
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

	if err := c.JSON(http.StatusOK, successResult{Success: true}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitionResultHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role == roleOrganizer {
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
		res := failureResult{
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
		c, err := retrievePlayerByName(ctx, tenantDB, playerName)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrievePlayerByName: %w", err)
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
	Reports []billingReport `json:"reports"`
}

func billingHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if v, err := parseViewer(c); err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	} else if v.role == roleOrganizer {
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
	Player playerDetail        `json:"player"`
	Scores []playerScoreDetail `json:"scores"`
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
	pss := []playerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE player_id = ? ORDER BY competition_id ASC",
		p.ID,
	); err != nil {
		return fmt.Errorf("error Select player_score: %w", err)
	}
	psds := make([]playerScoreDetail, 0, len(pss))
	for _, ps := range pss {
		comp, err := retrieveCompetition(ctx, tenantDB, ps.CompetitionID)
		if err != nil {
			return fmt.Errorf("error retrieveCompetition: %w", err)
		}
		psds = append(psds, playerScoreDetail{
			CompetitionTitle: comp.Title,
			Score:            ps.Score,
		})
	}

	res := successResult{
		Success: true,
		Data: playerHandlerResult{
			Player: playerDetail{
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

type competitionRank struct {
	Rank              int64  `json:"rank"`
	Score             int64  `json:"score"`
	PlayerName        string `json:"player_name"`
	PlayerDisplayName string `json:"player_display_name"`
}

type competitionRankingHandlerResult struct {
	Ranks []competitionRank `json:"ranks"`
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
	defer tenantDB.Close()

	vp, err := retrievePlayerByName(c.Request().Context(), tenantDB, v.playerName)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

	now := time.Now()
	id, err := dispenseID(ctx)
	if err != nil {
		return fmt.Errorf("error dispenseID: %w", err)
	}
	var t tenantRow
	if err := centerDB.SelectContext(ctx, &t, "SELECT * FROM tenant WHERE name = ?", v.tenantName); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}

	if _, err := centerDB.ExecContext(
		ctx,
		"INSERT INTO access_log (id, player_name, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at)",
		id, vp.Name, t.ID, competitionID, now, now,
	); err != nil {
		return fmt.Errorf("error Insert access_log: %w", err)
	}

	var rankAfter int64
	rankAfterStr := c.QueryParam("rank_after")
	if rankAfterStr != "" {
		if rankAfter, err = strconv.ParseInt(rankAfterStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: %w", err)
		}
	}

	pss := []playerScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&pss,
		"SELECT * FROM player_score WHERE competition_id = ? ORDER BY score DESC, player_id DESC",
		competitionID,
	); err != nil {
		return fmt.Errorf("error Select player_score: %w", err)
	}
	crs := make([]competitionRank, 0, len(pss))
	for i, ps := range pss {
		co, err := retrievePlayer(ctx, tenantDB, ps.PlayerID)
		if err != nil {
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		if int64(i) < rankAfter {
			continue
		}
		crs = append(crs, competitionRank{
			Rank:              int64(i + 1),
			Score:             ps.Score,
			PlayerName:        co.Name,
			PlayerDisplayName: co.DisplayName,
		})
		if len(crs) >= 100 {
			break
		}
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

	vp, err := retrievePlayerByName(c.Request().Context(), tenantDB, v.playerName)
	if err != nil {
		return fmt.Errorf("error retrievePlayerByName: %w", err)
	}
	if vp.IsDisqualified {
		return errNotPermitted
	}

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

const initializeMaxID = 10000 // 仮

type initializeHandlerResult struct {
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
	// constに定義されたmax_idより大きいIDのaccess_logを削除
	if _, err := centerDB.ExecContext(
		ctx,
		"DELETE FROM access_log WHERE id > ?",
		initializeMaxID,
	); err != nil {
		return fmt.Errorf("error Delete access_log: %w", err)
	}
	// constに定義されたmax_idにid_generatorを戻す
	if _, err := centerDB.ExecContext(
		ctx,
		"UPDATE id_generator SET id = ? WHERE stub = ?",
		initializeMaxID, "a",
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
	}

	res := initializeHandlerResult{
		Lang: "go",
		// 頑張ったポイントやこだわりポイントがあれば書いてください
		// 競技中の最後に計測したものを参照して、講評記事などで使わせていただきます
		Appeal: "",
	}
	if err := c.JSON(http.StatusOK, successResult{Success: true, Data: res}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}
