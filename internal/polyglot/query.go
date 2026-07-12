package polyglot

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
)

type QueryResponse struct {
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"row_count"`
	Truncated bool             `json:"truncated"`
}

// handleQuery implements GET /query: run a caller-supplied ANSI SQL
// SELECT/WITH statement and return matching rows as JSON objects keyed by
// column name, per openapi/polyglot.yaml.
func handleQuery(query ai.QueryFunc) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		sqlText := e.Request.URL.Query().Get("sql")
		if strings.TrimSpace(sqlText) == "" {
			return e.BadRequestError("sql query parameter is required", nil)
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
