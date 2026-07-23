package polyglot

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
)

type QueryResponse struct {
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"row_count"`
	Truncated bool             `json:"truncated"`
}

// reservedTablePattern blocks GET /query from ever reading polyglot's own
// bookkeeping collections (datasources holds vault path references;
// tables/columns hold the curated catalog - none of it is meant to be
// read back through this endpoint). Applied unconditionally, on every
// path - both the default (no ?datasource=) executor against polyglot's
// own db, and every named datasource's own Instance.Query. The latter
// matters because a caller can otherwise route around a text-only guard
// scoped to just one path: nothing here assumes a named datasource's
// connection is physically incapable of reaching polyglot's own tables
// (a misconfigured/malicious sqlite datasource pointed elsewhere already
// has its own dedicated guard - see internal/providers/sqlite's
// rejectOwnDataDir - but this check is cheap, unconditional, and correct
// defense in depth regardless). A word-boundary text match rather than a
// SQL parser: SQLite has no way to read a table's rows without naming it
// as a literal token in the statement, so any query that could return
// datasources.config/tables.description/columns.description must contain
// one of these substrings.
var reservedTablePattern = regexp.MustCompile(`(?i)\b(` + regexp.QuoteMeta(datasourcesCollection) + `|tables|columns)\b`)

// handleQuery implements GET /query: run a caller-supplied ANSI SQL
// SELECT/WITH statement and return matching rows as JSON objects keyed by
// column name, per openapi/polyglot.yaml. An optional ?datasource= routes
// to that datasource's own Instance.Query instead of polyglot's own
// bookkeeping db - omitting it is byte-for-byte the default behavior.
func handleQuery(defaultQuery ai.QueryFunc, reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		sqlText := e.Request.URL.Query().Get("sql")
		if strings.TrimSpace(sqlText) == "" {
			return e.BadRequestError("sql query parameter is required", nil)
		}
		if reservedTablePattern.MatchString(sqlText) {
			return e.BadRequestError("querying polyglot's own bookkeeping tables is not allowed", nil)
		}

		query := defaultQuery
		if dsName := e.Request.URL.Query().Get("datasource"); dsName != "" {
			inst, ok := reg.Instance(dsName)
			if !ok {
				return e.BadRequestError(fmt.Sprintf("unknown datasource %q", dsName), nil)
			}
			query = inst.Query
		}

		result, err := query(e.Request.Context(), sqlText)
		if err != nil {
			if errors.Is(err, ai.ErrNotReadOnly) {
				return e.BadRequestError(err.Error(), nil)
			}
			return e.InternalServerError("query failed", err)
		}

		return e.JSON(http.StatusOK, QueryResponse{
			Rows:      queryResultToRows(result),
			RowCount:  len(result.Rows),
			Truncated: result.Truncated,
		})
	}
}

// queryResultToRows reshapes ai.QueryResult's columnar Columns/Rows into
// the row-object form ({column_name: value}) the /query response uses.
func queryResultToRows(result ai.QueryResult) []map[string]any {
	rows := make([]map[string]any, len(result.Rows))
	for i, row := range result.Rows {
		obj := make(map[string]any, len(result.Columns))
		for j, col := range result.Columns {
			obj[col] = row[j]
		}
		rows[i] = obj
	}
	return rows
}
