package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	maxQueryRows = 500
	queryTimeout = 10 * time.Second
)

// ErrNotReadOnly is returned when the given SQL text isn't a SELECT/WITH
// statement, so callers (e.g. an HTTP handler) can distinguish a bad
// request from a real execution failure.
var ErrNotReadOnly = errors.New("only SELECT/WITH queries are allowed")

// NewReadOnlyExecutor opens a second, independent connection to
// PocketBase's own SQLite data file in read-only mode and returns a
// QueryFunc bound to it. mode=ro is the actual security boundary here: the
// connection is physically incapable of writing regardless of the SQL
// text. The statement-prefix check below is defense in depth on top of
// that, not the primary guarantee.
func NewReadOnlyExecutor(dataDir string) (QueryFunc, error) {
	dbPath := filepath.Join(dataDir, "data.db")
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening read-only db connection: %w", err)
	}

	return func(ctx context.Context, sqlText string) (QueryResult, error) {
		trimmed := strings.TrimSpace(sqlText)
		start := time.Now()
		slog.Debug("ai: sql query", "sql", trimmed)

		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
			slog.Warn("ai: sql query rejected", "sql", trimmed, "error", ErrNotReadOnly)
			return QueryResult{}, ErrNotReadOnly
		}

		qCtx, cancel := context.WithTimeout(ctx, queryTimeout)
		defer cancel()

		rows, err := db.QueryContext(qCtx, trimmed)
		if err != nil {
			slog.Error("ai: sql query failed", "sql", trimmed, "error", err, "duration_ms", time.Since(start).Milliseconds())
			return QueryResult{}, err
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return QueryResult{}, err
		}

		result := QueryResult{Columns: cols}
		for rows.Next() {
			if len(result.Rows) >= maxQueryRows {
				// rows.Next() just advanced past a real row beyond the cap,
				// so there's strictly more data than what's being returned.
				result.Truncated = true
				break
			}

			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return QueryResult{}, err
			}
			result.Rows = append(result.Rows, values)
		}
		if err := rows.Err(); err != nil {
			return QueryResult{}, err
		}

		slog.Info("ai: sql query complete", "rows", len(result.Rows), "truncated", result.Truncated, "duration_ms", time.Since(start).Milliseconds())
		slog.Debug("ai: sql query complete", "sql", trimmed)

		return result, nil
	}, nil
}
