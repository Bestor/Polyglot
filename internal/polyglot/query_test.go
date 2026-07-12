package polyglot

import (
	"reflect"
	"testing"

	"val-analyzer/internal/ai"
)

func TestQueryResultToRows(t *testing.T) {
	result := ai.QueryResult{
		Columns: []string{"name", "tag"},
		Rows: [][]any{
			{"OrBest", "NA1"},
			{"PFM_18", "GOAT"},
		},
	}

	got := queryResultToRows(result)
	want := []map[string]any{
		{"name": "OrBest", "tag": "NA1"},
		{"name": "PFM_18", "tag": "GOAT"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queryResultToRows() = %#v, want %#v", got, want)
	}
}

func TestQueryResultToRows_Empty(t *testing.T) {
	got := queryResultToRows(ai.QueryResult{Columns: []string{"name"}})
	if len(got) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(got))
	}
}
