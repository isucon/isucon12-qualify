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

func connectTenantDBByViewer(ctx context.Context, v *viewer) (*sqlx.DB, error) {
	t, err := retrieveTenantByIdentifier(ctx, v.tenantIdentifier)
	if err != nil {
		return nil, fmt.Errorf("error retrieveTenantByIdentifier: %w", err)
	}
	tenantDB, err := connectTenantDB(t.Identifier)
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

	// for admin endpoint
	e.POST("/api/tenants/add", tenantsAddHandler)
	e.GET("/api/tenants/billing", tenantsBillingHandler)

	// for tenant endpoint
	// 参加者操作
	e.POST("/api/competitors/add", competitorsAddHandler)
	e.POST("/api/competitor/:competitior_id/disqualified", competitorsDisqualifiedHandler)
	// 大会操作
	e.POST("/api/competitions/add", competitionsAddHandler)
	e.POST("/api/competition/:competition_id/finish", competitionFinishHandler)
	e.POST("/api/competition/:competition_id/result", competitionResultHandler)
	// テナント操作
	e.GET("/api/tenant/billing", tenantBillingHandler)
	// 参加者からの閲覧
	e.GET("/api/competitor/:competitor_identifier", competitorHandler)
	e.GET("/api/competition/:competition_id/ranking", competitionRankingHandler)
	e.GET("/api/competitions", competitionsHandler)

	// for benchmarker
	e.POST("/initialize", initializeHandler)

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

type role int

const (
	roleAdmin role = iota + 1
	roleOrganizer
	roleCompetitor
)

type viewer struct {
	role              role
	accountIdentifier string
	tenantIdentifier  string
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

func parseViewerIgnoreDisqualified(c echo.Context) (*viewer, error) {
	v, err := parseViewer(c)
	if err != nil {
		return nil, fmt.Errorf("error parseViewer:%w", err)
	}
	if v.role == roleCompetitor {
		a, err := retrieveAccountByIdentifier(c.Request().Context(), v.accountIdentifier)
		if err != nil {
			return nil, fmt.Errorf("error retrieveAccountByIdentifier: %w", err)
		}
		if a.Role == "disqualified_competitor" {
			return nil, errNotPermitted
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

type accountRow struct {
	ID         int64
	Identifier string
	Name       string
	TenantID   int64
	Role       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func retrieveAccountByIdentifier(ctx context.Context, identifier string) (*accountRow, error) {
	var a accountRow
	if err := centerDB.SelectContext(ctx, &a, "SELECT * FROM account WHERE identifier = ?", identifier); err != nil {
		return nil, fmt.Errorf("error Select account: %w", err)
	}
	return &a, nil
}

func retrieveAccount(ctx context.Context, id int64) (*accountRow, error) {
	var a accountRow
	if err := centerDB.SelectContext(ctx, &a, "SELECT * FROM account WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select account: %w", err)
	}
	return &a, nil
}

type accountAccessLogRow struct {
	ID            int64
	AccountID     int64
	CompetitionID int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type competitorRow struct {
	ID         int64
	Identifier string
	Name       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func retrieveCompetitorByIdentifier(ctx context.Context, tenantDB *sqlx.DB, identifier string) (*competitorRow, error) {
	var c competitorRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM competitor WHERE identifier = ?", identifier); err != nil {
		return nil, fmt.Errorf("error Select competitor: %w", err)
	}
	return &c, nil
}

func retrieveCompetitor(ctx context.Context, tenantDB *sqlx.DB, id int64) (*competitorRow, error) {
	var c competitorRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM competitor WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select competitor: %w", err)
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

func retrieveCompetition(ctx context.Context, tenantDB *sqlx.DB, id int64) (*competitionRow, error) {
	var c competitionRow
	if err := tenantDB.SelectContext(ctx, &c, "SELECT * FROM competition WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("error Select competition: %w", err)
	}
	return &c, nil
}

type competitorScoreRow struct {
	ID            int64
	CompetitorID  int64
	CompetitionID int64
	Score         int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

var (
	errNotPermitted = errors.New("this role is not permitted")
)

func tenantsAddHandler(c echo.Context) error {
	_, err := parseViewerMustAdmin(c)
	if err != nil {
		return fmt.Errorf("error parseViewerMustAdmin: %w", err)
	}

	name := c.FormValue("name")
	icon, err := c.FormFile("icon")
	if err != nil {
		return fmt.Errorf("error retrieve icon from FormFile: %w", err)
	}
	iconFile, err := icon.Open()
	if err != nil {
		return fmt.Errorf("error icon.Open: %w", err)
	}
	defer iconFile.Close()
	iconBytes, err := io.ReadAll(iconFile)
	if err != nil {
		return fmt.Errorf("error io.ReadAll: %w", err)
	}

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
		"INSERT INTO `tenant` (`id`, `identifier`, `name`, `image`, `created_at`, `updated_at`)",
		id, identifier, name, iconBytes, now, now,
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error Insert `tenant`: %w", err)
	}

	if err := createTenantDB(identifier); err != nil {
		tx.Rollback()
		return fmt.Errorf("error createTenantDB: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error tx.Commit: %w", err)
	}
	return nil
}

type competitorScore struct {
	CompetitionID        int64
	CompetitorIdentifier string
	Score                int64
}

func listScoresAll(ctx context.Context, tenantID string) ([]competitorScore, error) {
	tenantDB, err := connectTenantDB(tenantID)
	if err != nil {
		return nil, fmt.Errorf("error connectTenantDB: %w", err)
	}
	defer tenantDB.Close()

	scores := []competitorScore{}
	rows, err := tenantDB.QueryxContext(
		ctx,
		"SELECT competition_id, competitor_id, score FROM competitor_score",
	)
	if err != nil {
		return nil, fmt.Errorf("error SELECT competitor_score: %w", err)
	}
	for rows.Next() {
		var competitionID, competitorID, score int64
		if err := rows.Scan(&competitionID, &competitorID, &score); err != nil {
			return nil, fmt.Errorf("error rows.Scan: %w", err)
		}
		var c competitorRow
		if err := tenantDB.GetContext(ctx, &c, "SELECT * FROM competitor WHERE id = ?", competitorID); err != nil {
			return nil, fmt.Errorf("erorr SELECT competitor: %w", err)
		}
		scores = append(scores, competitorScore{
			CompetitionID:        competitionID,
			CompetitorIdentifier: c.Identifier,
			Score:                score,
		})
	}

	return scores, nil
}

type tenantBillingReport struct {
	CompetitionID    int64
	CompetitionTitle string
	CompetitorCount  int64
	BillingYen       int64
}

func billingReportByCompetition(ctx context.Context, tenantDB *sqlx.DB, competitonID int64) (*tenantBillingReport, error) {
	comp, err := retrieveCompetition(ctx, tenantDB, competitonID)
	if err != nil {
		return nil, fmt.Errorf("error retrieveCompetition: %w", err)
	}

	aals := []accountAccessLogRow{}
	if err := centerDB.SelectContext(
		ctx,
		aals,
		"SELECT * FROM account_access_log WHERE competition_id = ?",
		comp.ID,
	); err != nil {
		return nil, fmt.Errorf("error Select account_access_log: %w", err)
	}
	billingMap := map[string]int64{}
	for _, aal := range aals {
		a, err := retrieveAccount(ctx, aal.AccountID)
		if err != nil {
			return nil, fmt.Errorf("error retrieveAccount: %w", err)
		}
		billingMap[a.Identifier] = 10
	}

	css := []competitorScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&css,
		"SELECT * FROM competitor_score WHERE competition_id = ?",
		comp.ID,
	); err != nil {
		return nil, fmt.Errorf("error Select count competitor_score: %w", err)
	}
	for _, cs := range css {
		competitor, err := retrieveCompetitor(ctx, tenantDB, cs.CompetitorID)
		if err != nil {
			return nil, fmt.Errorf("error retrieveCompetitor: %w", err)
		}
		if _, ok := billingMap[competitor.Identifier]; ok {
			billingMap[competitor.Identifier] = 100
		} else {
			billingMap[competitor.Identifier] = 50
		}
	}

	var billingYen int64
	for _, v := range billingMap {
		billingYen += v
	}
	return &tenantBillingReport{
		CompetitionID:    comp.ID,
		CompetitionTitle: comp.Title,
		CompetitorCount:  int64(len(css)),
		BillingYen:       billingYen,
	}, nil
}

type successResult struct {
	Success bool `json:"status"`
	Data    any  `json:"data"`
}

type failureResult struct {
	Success bool   `json:"status"`
	Message string `json:"message"`
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
	_, err := parseViewerMustAdmin(c)
	if err != nil {
		return fmt.Errorf("error parseViewerMustAdmin: %w", err)
	}

	// テナントごとに
	//   大会ごとに
	//     scoreに登録されているaccountでアクセスした人 * 100
	//     scoreに登録されているaccountでアクセスしていない人 * 50
	//     scoreに登録されていないaccountでアクセスした人 * 10
	//   を合計したものを
	// テナントの課金とする
	conn, err := centerDB.Connx(ctx)
	if err != nil {
		return fmt.Errorf("error centerDB.Conxx: %w", err)
	}
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE account_score (
	competition_id BIGINT UNSIGNED NOT NULL,
	account_identifier VARCAHR(191) NOT NULL,
	score BIGINT UNSIGNED NOT NULL
);
	`)
	if err != nil {
		return fmt.Errorf("error CREATE TEMPORARY TABLE account_score: %w", err)
	}
	tenantIDs := []string{}
	if err := conn.SelectContext(ctx, &tenantIDs, "SELECT id FROM tenant"); err != nil {
		return fmt.Errorf("error Select tenant: %w", err)
	}
	for _, tenantID := range tenantIDs {
		scores, err := listScoresAll(ctx, tenantID)
		if err != nil {
			return fmt.Errorf("error listScoresAll: %w", err)
		}
		for _, score := range scores {
			_, err := conn.ExecContext(
				ctx,
				"INSERT INTO account_score (competition_id, account_identifier, score) VALUES (?, ?, ?)",
				score.CompetitionID, score.CompetitorIdentifier, score.Score,
			)
			if err != nil {
				return fmt.Errorf("error INSERT account_score: %w", err)
			}
		}

	}
	tenantBillings := make([]tenantBilling, 0, len(tenantIDs))
	err = conn.SelectContext(ctx, &tenantBillings, `
WITH
q1 AS (
  SELECT
    tenant_id,
    competition_id,
    CASE account_access_log.id IS NULL WHEN 1 THEN 50 ELSE 100 END AS billing_scored,
    0 billing_accessed
  FROM account_score
  INNER JOIN account ON account_score.account_identifier = account.identifier
  LEFT OUTER JOIN account_access_log ON
    account_score.competition_id = account_access_log.competition_id AND
    account.account_account_id = account_access_log.account_id
  UNION ALL
  SELECT
    tenant_id,
    competition_id,
    0 AS billing_scored,
    10 AS billing_accessed
  FROM account_access_log
  INNER JOIN account ON account_access_log.account_id = account.id
),
q2 AS (
  SELECT tenant_id, competition_id,
  CASE SUM(billing_scored) > SUM(billing_accessed) WHEN 1 THEN SUM(billing_scored) ELSE SUM(billing_accessed) END AS billing
  GROUP BY tenant_id, competition_id
)
SELECT
  tenant.identifier AS tenant_identifier, tenant.name AS tenant_name, SUM(q1.billing)
FROM q2 JOIN tenant ON q1.tennant_id = tenant.id GROUP BY q1.tenant_id
	`)
	if err != nil {
		return fmt.Errorf("error retrieve tenantBillings: %w", err)
	}
	if err := c.JSON(http.StatusOK, successResult{
		Success: true,
		Data:    tenantBillings,
	}); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}
	return nil
}

func competitorsAddHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerMustOrganizer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}
	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByViewer: %w", err)
	}
	defer tenantDB.Close()

	t, err := retrieveTenantByIdentifier(ctx, v.tenantIdentifier)
	if err != nil {
		return fmt.Errorf("error retrieveTenantByIdentifier: %w", err)
	}

	params, err := c.FormParams()
	if err != nil {
		return fmt.Errorf("error c.FormParams: %w", err)
	}
	names := params["name"]

	now := time.Now()
	tx, err := centerDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error centerDB.BeginTxx: %w", err)
	}
	ttx, err := tenantDB.BeginTxx(ctx, nil)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error tenantDB.BeginTxx: %w", err)
	}
	for _, name := range names {
		id, err := dispenseID(ctx)
		if err != nil {
			tx.Rollback()
			ttx.Rollback()
			return fmt.Errorf("error dispenseID: %w", err)
		}

		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO account (id, identifier, name, tenant_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			id, id, name, t.ID, "competitor", now, now,
		); err != nil {
			tx.Rollback()
			ttx.Rollback()
			return fmt.Errorf("error Insert account at centerDB: %w", err)
		}

		if _, err := ttx.ExecContext(
			ctx,
			"INSERT INTO competitor (id, identifier, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			id, id, name, now, now,
		); err != nil {
			tx.Rollback()
			ttx.Rollback()
			return fmt.Errorf("error Insert account at tenantDB: %w", err)
		}
	}
	if err := ttx.Commit(); err != nil {
		tx.Rollback()
		return fmt.Errorf("error ttx.Commit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error tx.Commit: %w", err)
	}
	return nil
}

func competitorsDisqualifiedHandler(c echo.Context) error {
	ctx := c.Request().Context()

	_, err := parseViewerMustOrganizer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}

	idStr := c.Param("competitor_id")
	var id int64
	if id, err = strconv.ParseInt(idStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
	}

	now := time.Now()
	if _, err := centerDB.ExecContext(
		ctx,
		"UPDATE account SET role = ?, updated_at = ? WHERE id = ?",
		"disqualified_competitor", now, id,
	); err != nil {
		return fmt.Errorf("error Update account: %w", err)
	}

	return nil
}

func competitionsAddHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerMustOrganizer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}
	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByViewer: %w", err)
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

	return nil
}

func competitionFinishHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerMustOrganizer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}
	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByViewer: %w", err)
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

	return nil
}

func competitionResultHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerMustOrganizer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}
	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByViewer: %w", err)
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
	if !reflect.DeepEqual(headers, []string{"competitor_identifier", "score"}) {
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
		competitorIdentifier, scoreStr := row[0], row[1]
		c, err := retrieveCompetitorByIdentifier(ctx, tenantDB, competitorIdentifier)
		if err != nil {
			ttx.Rollback()
			return fmt.Errorf("error retrieveCompetitorByIdentifier: %w", err)
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
			"REPLACE INTO competitor_score (id, competitor_id, competition_id, score, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, c.ID, competitionID, score, now, now,
		); err != nil {
			ttx.Rollback()
			return fmt.Errorf("error Update competition: %w", err)
		}
	}

	if err := ttx.Commit(); err != nil {
		return fmt.Errorf("error txx.Commit: %w", err)
	}

	return nil
}

type tenantBillingHandlerResult struct {
	Reports []tenantBillingReport
}

func tenantBillingHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerMustOrganizer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}
	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDBByViewer: %w", err)
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
	tbrs := make([]tenantBillingReport, 0, len(cs))
	for _, comp := range cs {
		report, err := billingReportByCompetition(ctx, tenantDB, comp.ID)
		if err != nil {
			return fmt.Errorf("error billingReportByCompetition: %w", err)
		}
		tbrs = append(tbrs, *report)
	}

	ret := successResult{
		Success: true,
		Data: tenantBillingHandlerResult{
			Reports: tbrs,
		},
	}
	if err := c.JSON(http.StatusOK, ret); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type competitorScoreDetail struct {
	CompetitionTitle string `json:"competition_title"`
	Score            int64  `json:"score"`
}

type competitorHandlerResult struct {
	Name   string                  `json:"name"`
	Scores []competitorScoreDetail `json:"scores"`
}

func competitorHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerIgnoreDisqualified(c)
	if err != nil {
		return fmt.Errorf("error parseViewerIgnoreDisqualified: %w", err)
	}

	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDB: %w", err)
	}
	defer tenantDB.Close()

	ci := c.Param("competitor_identifier")

	co, err := retrieveCompetitorByIdentifier(ctx, tenantDB, ci)
	if err != nil {
		return fmt.Errorf("error retrieveCompetitorByIdentifier: %w", err)
	}
	css := []competitorScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&css,
		"SELECT * FROM competitor_score WHERE competitor_id = ? ORDER BY competition_id ASC",
		co.ID,
	); err != nil {
		return fmt.Errorf("error Select competitor_score: %w", err)
	}
	csds := make([]competitorScoreDetail, 0, len(css))
	for _, cs := range css {
		comp, err := retrieveCompetition(ctx, tenantDB, cs.CompetitionID)
		if err != nil {
			return fmt.Errorf("error retrieveCompetition: %w", err)
		}
		csds = append(csds, competitorScoreDetail{
			CompetitionTitle: comp.Title,
			Score:            cs.Score,
		})
	}

	ret := successResult{
		Success: true,
		Data: competitorHandlerResult{
			Name:   co.Name,
			Scores: []competitorScoreDetail{},
		},
	}
	if err := c.JSON(http.StatusOK, ret); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type competitionRank struct {
	Rank                 int64  `json:"rank"`
	Score                int64  `json:"score"`
	CompetitorIdentifier string `json:"competitor_identifier"`
	CompetitiorName      string `json:"competitior_name"`
}

type competitionRankingHandlerResult struct {
	Ranks []competitionRank `json:"ranks"`
}

func competitionRankingHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerIgnoreDisqualified(c)
	if err != nil {
		return fmt.Errorf("error parseViewerIgnoreDisqualified: %w", err)
	}

	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDB: %w", err)
	}
	defer tenantDB.Close()

	competitionIDStr := c.Param("competition_id")
	var competitionID int64
	if competitionID, err = strconv.ParseInt(competitionIDStr, 10, 64); err != nil {
		return fmt.Errorf("error strconv.ParseUint: %w", err)
	}

	var rankAfter int64
	rankAfterStr := c.QueryParam("rank_after")
	if rankAfterStr != "" {
		if rankAfter, err = strconv.ParseInt(rankAfterStr, 10, 64); err != nil {
			return fmt.Errorf("error strconv.ParseUint: %w", err)
		}
	}

	css := []competitorScoreRow{}
	if err := tenantDB.SelectContext(
		ctx,
		&css,
		"SELECT * FROM competitor_score WHERE competition_id = ? ORDER BY score DESC, competitor_id DESC",
		competitionID,
	); err != nil {
		return fmt.Errorf("error Select competitor_score: %w", err)
	}
	crs := make([]competitionRank, 0, len(css))
	for i, cs := range css {
		co, err := retrieveCompetitor(ctx, tenantDB, cs.CompetitorID)
		if err != nil {
			return fmt.Errorf("error retrieveCompetitor: %w", err)
		}
		if int64(i) < rankAfter {
			continue
		}
		crs = append(crs, competitionRank{
			Rank:                 int64(i + 1),
			Score:                cs.Score,
			CompetitorIdentifier: co.Identifier,
			CompetitiorName:      co.Name,
		})
	}

	ret := successResult{
		Success: true,
		Data: competitionRankingHandlerResult{
			Ranks: crs,
		},
	}
	if err := c.JSON(http.StatusOK, ret); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

type competitionDetail struct {
	ID         int64
	Title      string
	IsFinished bool
}

type competitionsHandlerResult struct {
	Competitions []competitionDetail
}

func competitionsHandler(c echo.Context) error {
	ctx := c.Request().Context()

	v, err := parseViewerIgnoreDisqualified(c)
	if err != nil {
		return fmt.Errorf("error parseViewerIgnoreDisqualified: %w", err)
	}

	tenantDB, err := connectTenantDBByViewer(ctx, v)
	if err != nil {
		return fmt.Errorf("error connectTenantDB: %w", err)
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

	ret := successResult{
		Success: true,
		Data: competitionsHandlerResult{
			Competitions: cds,
		},
	}
	if err := c.JSON(http.StatusOK, ret); err != nil {
		return fmt.Errorf("error c.JSON: %w", err)
	}

	return nil
}

func initializeHandler(c echo.Context) error {
	// TODO: SaaS管理者かチェック

	// constに定義されたmax_idより大きいIDのtenantを削除
	// constに定義されたmax_idより大きいIDのaccountを削除
	// constに定義されたmax_idより大きいIDのaccount_access_logを削除
	// constに定義されたmax_idにid_generatorを戻す
	// 残ったtenantのうち、max_idより大きいcompetition, competitor, competitor_scoreを削除

	return nil
}
