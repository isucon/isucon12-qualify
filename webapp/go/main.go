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
	e.POST("/api/tenants/add", dummy)
	e.GET("/api/tenants/billing", dummy)

	// for tenant endpoint
	// 参加者操作
	e.POST("/api/competitors/add", dummy)
	e.POST("/api/competitor/:competitior_id/disqualified", dummy)
	// 大会操作
	e.POST("/api/competitions/add", dummy)
	e.POST("/api/competition/:competition_id/finish", dummy)
	e.GET("/api/competition/:competition_id/result", dummy)
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
	return nil
}
