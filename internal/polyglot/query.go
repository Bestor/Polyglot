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
// datasources bookkeeping collection (which holds provider config,
// including secrets like API keys). Excluding it from GET /metadata only
// hides it from discovery - it doesn't stop a caller from directly
// writing SELECT * FROM datasources, so this is a real, if blunt, guard:
// a word-boundary text match rather than a SQL parser. That's sound for
// its actual purpose - SQLite has no way to read a table's rows without
// naming it as a literal token in the statement, so any query that could
// return datasources.config must contain this substring.
var reservedTablePattern = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(datasourcesCollection) + `\b`)

// handleQuery implements GET /query: run a caller-supplied ANSI SQL
// SELECT/WITH statement and return matching rows as JSON objects keyed by
// column name, per openapi/polyglot.yaml.
func handleQuery(query ai.QueryFunc) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		sqlText := e.Request.URL.Query().Get("sql")
		if strings.TrimSpace(sqlText) == "" {
			return e.BadRequestError("sql query parameter is required", nil)
		}
		if reservedTablePattern.MatchString(sqlText) {
			return e.BadRequestError(fmt.Sprintf("querying %q is not allowed", datasourcesCollection), nil)
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
