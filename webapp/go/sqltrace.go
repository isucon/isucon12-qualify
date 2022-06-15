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

func initializeSQLLogger() (io.Closer, error) {
	var (
		traceLogFile io.WriteCloser
		enc          *json.Encoder
	)
	if traceFilePath := getEnv("ISUCON_SQLITE_TRACE_FILE", ""); traceFilePath != "" {
		var err error
		traceLogFile, err = os.OpenFile(traceFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("cannot open ISUCON_SQLITE_TRACE_FILE: %w", err)
		}
		enc = json.NewEncoder(traceLogFile)
	} else {
		return io.NopCloser(nil), nil
	}
	sql.Register("sqlite3-with-trace", proxy.NewProxyContext(&sqlite3.SQLiteDriver{}, &proxy.HooksContext{
		PreExec: func(_ context.Context, _ *proxy.Stmt, _ []driver.NamedValue) (interface{}, error) {
			return time.Now(), nil
		},
		PostExec: func(_ context.Context, ctx interface{}, stmt *proxy.Stmt, args []driver.NamedValue, result driver.Result, _ error) error {
			if enc == nil {
				return nil
			}
			enc := json.NewEncoder(traceLogFile)
			enc.SetEscapeHTML(false)
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
					return fmt.Errorf("error driver.Result.RowsAffected at PostExec: %w", err)
				}
			}

			sqlLog := struct {
				Time         string        `json:"time"`
				Statement    string        `json:"statement"`
				Args         []interface{} `json:"args"`
				QueryTime    float64       `json:"query_time"`
				AffectedRows int64         `json:"affected_rows"`
			}{
				Time:         starts.Format(time.RFC3339),
				Statement:    stmt.QueryString,
				Args:         argsValues,
				QueryTime:    queryTime.Seconds(),
				AffectedRows: affected,
			}
			if err := enc.Encode(sqlLog); err != nil {
				return fmt.Errorf("error encode.Encode at PostExec: %w", err)
			}
			return nil
		},
	}))
	return traceLogFile, nil
}
