package isuports

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattn/go-sqlite3"
	proxy "github.com/shogo82148/go-sql-proxy"
)

var traceLogEncoder *json.Encoder

func initializeSQLLogger() (string, io.Closer, error) {
	traceFilePath := getEnv("ISUCON_SQLITE_TRACE_FILE", "")
	if traceFilePath == "" {
		return "sqlite3", io.NopCloser(nil), nil
	}

	traceLogFile, err := os.OpenFile(traceFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("cannot open ISUCON_SQLITE_TRACE_FILE: %w", err)
	}

	traceLogEncoder = json.NewEncoder(traceLogFile)
	traceLogEncoder.SetEscapeHTML(false)
	driverName := "sqlite3-with-trace"
	sql.Register(driverName, proxy.NewProxyContext(&sqlite3.SQLiteDriver{}, &proxy.HooksContext{
		PreExec:   traceLogPre,
		PostExec:  traceLogPostExec,
		PreQuery:  traceLogPre,
		PostQuery: traceLogPostQuery,
	}))
	return driverName, traceLogFile, nil
}

func traceLogPre(_ context.Context, _ *proxy.Stmt, _ []driver.NamedValue) (interface{}, error) {
	return time.Now(), nil
}

type sqlTraceLog struct {
	Time         string        `json:"time"`
	Statement    string        `json:"statement"`
	Args         []interface{} `json:"args"`
	QueryTime    float64       `json:"query_time"`
	AffectedRows int64         `json:"affected_rows"`
}

func traceLogPostExec(_ context.Context, ctx interface{}, stmt *proxy.Stmt, args []driver.NamedValue, result driver.Result, _ error) error {
	if traceLogEncoder == nil {
		return nil
	}
	starts := ctx.(time.Time)
	queryTime := time.Since(starts)

	argsValues := make([]any, 0, len(args))
	for _, arg := range args {
		argsValues = append(argsValues, arg.Value)
	}
	var affected int64
	if result != nil {
		var err error
		affected, err = result.RowsAffected()
		if err != nil {
			return fmt.Errorf("error driver.Result.RowsAffected at traceLogPost: %w", err)
		}
	}

	log := sqlTraceLog{
		Time:         starts.Format(time.RFC3339),
		Statement:    stmt.QueryString,
		Args:         argsValues,
		QueryTime:    queryTime.Seconds(),
		AffectedRows: affected,
	}
	if err := traceLogEncoder.Encode(log); err != nil {
		return fmt.Errorf("error encode.Encode at traceLogPostExec: %w", err)
	}
	return nil
}

func traceLogPostQuery(_ context.Context, ctx interface{}, stmt *proxy.Stmt, args []driver.NamedValue, result driver.Rows, _ error) error {
	if traceLogEncoder == nil {
		return nil
	}
	starts := ctx.(time.Time)
	queryTime := time.Since(starts)

	argsValues := make([]any, 0, len(args))
	for _, arg := range args {
		argsValues = append(argsValues, arg.Value)
	}
	log := sqlTraceLog{
		Time:         starts.Format(time.RFC3339),
		Statement:    stmt.QueryString,
		Args:         argsValues,
		QueryTime:    queryTime.Seconds(),
		AffectedRows: 0,
	}
	if err := traceLogEncoder.Encode(log); err != nil {
		return fmt.Errorf("error encode.Encode at traceLogPostQuery: %w", err)
	}
	return nil
}
