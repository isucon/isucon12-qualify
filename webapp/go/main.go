package main

import (
	"fmt"
	"os"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

func getEnv(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

func connectDB() (*sqlx.DB, error) {
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

var db *sqlx.DB

func main() {
	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.DEBUG)

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	dummy := func(c echo.Context) error { return nil }
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
	e.POST("/api/competition/:competition_id/result", competitionPostResultHandler)
	// テナント操作
	e.GET("/api/tenant/billing", dummy)
	// 参加者からの閲覧
	e.GET("/api/competitor/:competitor_id", dummy)
	e.GET("/api/competition/:competition_id/ranking", dummy)
	e.GET("/api/competitions", dummy)

	// for benchmarker
	e.POST("/initialize", dummy)

	var err error
	db, err = connectDB()
	if err != nil {
		e.Logger.Fatalf("failed to connect db: %v", err)
		return
	}
	db.SetMaxOpenConns(10)
	defer db.Close()

	port := getEnv("SERVER_APP_PORT", "3000")
	e.Logger.Infof("starting isuports server on : %s ...", port)
	serverPort := fmt.Sprintf(":%s", port)
	e.Logger.Fatal(e.Start(serverPort))
}

func tenantsAddHandler(c echo.Context) error {
	// TODO: SaaS管理者かどうかをチェック

	// tenantテーブルでテーブルロック
	// identifier 発行 => id_generatorの文字列
	// tenant テーブルへINSERT

	// テナント初期化スキーマをsqliteコマンドで実行
	// ロック解放
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

func competitionPostResultHandler(c echo.Context) error {
	// TODO: テナント管理者かチェック

	// アップロードされたCSVを読みながらテナントDBのcompetitor_scoreテーブルにループクエリでINSERT
	return nil
}
