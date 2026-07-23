package polyglot

import (
	"context"
	"reflect"
	"testing"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
	_ "val-analyzer/internal/migrations"
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

func TestReservedTablePatternBlocksBookkeepingTables(t *testing.T) {
	blocked := []string{
		"SELECT * FROM datasources",
		"select * from DataSources",
		"SELECT   *   FROM   datasources",
		"WITH x AS (SELECT * FROM datasources) SELECT * FROM x",
		"SELECT * FROM tables",
		"SELECT * FROM columns",
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

func TestHandleQuery_RoutesToNamedDatasource(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{typ: "widgets"}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	inst, ok := reg.Instance("widgets")
	if !ok {
		t.Fatal("expected widgets instance to be active")
	}
	if _, err := inst.Query(context.Background(), "SELECT 1"); err != nil {
		t.Fatalf("expected the named instance's Query to be callable, got %v", err)
	}

	if _, ok := reg.Instance("unknown"); ok {
		t.Fatal("expected no instance for an unonboarded name")
	}
}
