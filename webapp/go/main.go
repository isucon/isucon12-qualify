package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	e.GET("/api/competitor/:competitor_id", competitorHandler)
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

func tenantsAddHandler(c echo.Context) error {
	// TODO: SaaS管理者かどうかをチェック
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

func tenantsBillingHandler(c echo.Context) error {
	// TODO: SaaS管理者かどうかをチェック

	// テナントごとに
	//   大会ごとに
	//     scoreに登録されているaccountでアクセスした人 * 100
	//     scoreに登録されているaccountでアクセスしていない人 * 50
	//     scoreに登録されていないaccountでアクセスした人 * 10
	//   を合計したものを
	// テナントの課金とする
	return nil
}

func competitorsAddHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// 管理DBのaccountにinsert
	// テナントDBのcompetitorにinsert

	return nil
}

func competitorsDisqualifiedHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// 管理DBのaccountを`disqualified_competitor`にする
	return nil
}

func competitionsAddHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// テナントDBのcompetitionテーブルにinsert
	return nil
}

func competitionFinishHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// テナントDBのcompetitionテーブルのfinished_atを現在時刻を入れるようにupdate
	return nil
}

func competitionResultHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// アップロードされたCSVを読みながらテナントDBのcompetitor_scoreテーブルにループクエリでINSERT
	return nil
}

func tenantBillingHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// 大会ごとに
	//   scoreに登録されているaccountでアクセスした人 * 100
	//   scoreに登録されているaccountでアクセスしていない人 * 50
	//   scoreに登録されていないaccountでアクセスした人 * 10
	// を合計したものを計算する
	return nil
}

func competitorHandler(c echo.Context) error {
	// TODO: テナント管理者 or テナント参加者 or SaaS管理者かチェック
	// TODO: 失格者かチェック

	// テナントDBからcompetitorを取ってくる
	// テナントDBからcompetitor_idでcompetitor_scoreを取ってくる
	//    ループクエリでcompetitionを取ってくる
	return nil
}

func competitionRankingHandler(c echo.Context) error {
	// TODO: テナント管理者 or テナント参加者 or SaaS管理者かチェック
	// TODO: 失格者かチェック

	// テナントDBからcompetition_idでcompetitor_scoreを取ってくる
	//   ループクエリでテナントDBのcompetitorを引いて名前を埋め込む
	//   上から数えた順位を作成する。同じスコアなら同じ順位とする
	return nil
}

func competitionsHandler(c echo.Context) error {
	// TODO: テナント管理者 or テナント参加者 or SaaS管理者かチェック
	// TODO: 失格者かチェック

	// テナントDBからcompetition一覧を取ってくる
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
