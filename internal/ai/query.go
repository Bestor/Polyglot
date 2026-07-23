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
	// maxCellBytes bounds any single returned value, independent of
	// maxQueryRows - without this, a query touching a wide column (e.g.
	// matches.raw_json, a JSON blob up to 4MiB per row - see
	// internal/migrations/1750000006_matches.go) could return a single
	// cell large enough on its own to blow well past an LLM caller's
	// entire context window, regardless of row count. Confirmed the hard
	// way: a single-digit number of matches.raw_json rows produced a
	// >1M-token request that Claude's API rejected outright.
	maxCellBytes = 8 * 1024
	// maxQueryResponseBytes bounds the cumulative size of the whole
	// result (post cell-truncation), independent of maxQueryRows - many
	// moderately-sized rows can still add up to more than a caller should
	// receive in one response.
	maxQueryResponseBytes = 200 * 1024
	queryTimeout          = 10 * time.Second
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
		return RunReadOnlyQuery(ctx, db, sqlText)
	}, nil
}

// RunReadOnlyQuery executes sqlText against db, enforcing the row/cell/
// cumulative safety caps below. db must already be physically incapable of
// writing (e.g. opened with mode=ro) - shared by NewReadOnlyExecutor
// (polyglot's own data.db) and any connect-style dataprovider.Instance
// (e.g. internal/providers/sqlite) that opens its own read-only connection
// to an onboarded file.
func RunReadOnlyQuery(ctx context.Context, db *sql.DB, sqlText string) (QueryResult, error) {
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
	var totalBytes int
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

		rowBytes := 0
		for i, v := range values {
			truncatedVal, wasTruncated, size := truncateCell(v)
			values[i] = truncatedVal
			rowBytes += size
			if wasTruncated {
				result.Truncated = true
			}
		}

		if len(result.Rows) > 0 && totalBytes+rowBytes > maxQueryResponseBytes {
			// Already have at least one row - stop here rather than
			// exceed the cumulative budget with one more.
			result.Truncated = true
			break
		}

		result.Rows = append(result.Rows, values)
		totalBytes += rowBytes
		if totalBytes > maxQueryResponseBytes {
			// Even this one row alone exceeded the budget (e.g. a
			// single wide-column cell) - it's already appended above
			// so the caller isn't left with zero rows for a query
			// that otherwise made sense, but stop immediately.
			result.Truncated = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, err
	}

	slog.Info("ai: sql query complete", "rows", len(result.Rows), "truncated", result.Truncated, "duration_ms", time.Since(start).Milliseconds())
	slog.Debug("ai: sql query complete", "sql", trimmed)

	return result, nil
}

// truncateCell caps a single scanned value at maxCellBytes, replacing an
// oversized string/[]byte with a truncated prefix plus a marker noting the
// original size - the only column types actually capable of being huge
// (e.g. matches.raw_json). size is the byte cost to count toward the
// cumulative maxQueryResponseBytes budget: the post-truncation length for a
// truncated value, or the true length otherwise. Other scanned types
// (numbers, bools, nil, time.Time, ...) are inherently small/bounded, so
// they're returned unchanged with a small nominal size.
func truncateCell(v any) (result any, truncated bool, size int) {
	switch val := v.(type) {
	case string:
		if len(val) > maxCellBytes {
			return fmt.Sprintf("%s...(truncated, %d bytes total)", val[:maxCellBytes], len(val)), true, maxCellBytes
		}
		return val, false, len(val)
	case []byte:
		if len(val) > maxCellBytes {
			return fmt.Sprintf("%s...(truncated, %d bytes total)", val[:maxCellBytes], len(val)), true, maxCellBytes
		}
		return val, false, len(val)
	default:
		return v, false, 8
	}
}
