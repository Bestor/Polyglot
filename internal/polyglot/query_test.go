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

func TestReservedTablePatternBlocksDatasources(t *testing.T) {
	blocked := []string{
		"SELECT * FROM datasources",
		"select * from DataSources",
		"SELECT   *   FROM   datasources",
		"WITH x AS (SELECT * FROM datasources) SELECT * FROM x",
	}
	for _, sql := range blocked {
		if !reservedTablePattern.MatchString(sql) {
			t.Errorf("expected reservedTablePattern to match %q", sql)
		}
	}

	allowed := []string{
		"SELECT * FROM players",
		"SELECT * FROM matches WHERE match_id = 'datasources_1'", // substring within a token doesn't count
	}
	for _, sql := range allowed {
		if reservedTablePattern.MatchString(sql) {
			t.Errorf("expected reservedTablePattern NOT to match %q", sql)
		}
	}
}
