package polyglot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"val-analyzer/internal/dataprovider"
)

// newTestAnnotateEvent builds a *core.RequestEvent good enough to call
// handleAnnotateDatasource/handleAnnotateTable directly: e.App is needed
// for the record lookup/save, and the request needs a real
// Content-Type: application/json header - BindBody errors without one
// (verified against the vendored pocketbase module's own BindBody tests).
func newTestAnnotateEvent(t *testing.T, app core.App, target, body string) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return &core.RequestEvent{App: app, Event: router.Event{Response: rec, Request: req}}, rec
}

// TestHandleAnnotateTable_CuratedFields proves the new good_for/bad_for/
// known_gaps/example_queries fields follow the same pointer-means-
// "only touch if provided" semantics description/query_guidance already
// have: omitting a field leaves it unchanged, sending an empty
// string/array explicitly clears it.
func TestHandleAnnotateTable_CuratedFields(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ: "widgets",
		catalog: []dataprovider.TableCatalog{
			{Name: "widgets", Columns: []dataprovider.ColumnCatalog{{Name: "sku", Type: "TEXT"}}},
		},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})
	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	tableRec, err := app.FindFirstRecordByFilter("tables", "name = 'widgets'")
	if err != nil {
		t.Fatalf("finding tables row: %v", err)
	}
	id := tableRec.Id

	// Set all four new fields.
	body := `{"id":"` + id + `","good_for":"aggregate queries","bad_for":"row-level lookups","known_gaps":"none yet","example_queries":[{"question":"how many widgets?","sql":"SELECT COUNT(*) FROM widgets"}]}`
	e, rec := newTestAnnotateEvent(t, app, "/tables/annotate", body)
	if err := handleAnnotateTable()(e); err != nil {
		t.Fatalf("handleAnnotateTable: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	after, err := app.FindRecordById("tables", id)
	if err != nil {
		t.Fatalf("re-reading table: %v", err)
	}
	if after.GetString("good_for") != "aggregate queries" {
		t.Errorf("good_for = %q, want %q", after.GetString("good_for"), "aggregate queries")
	}
	if after.GetString("bad_for") != "row-level lookups" {
		t.Errorf("bad_for = %q, want %q", after.GetString("bad_for"), "row-level lookups")
	}
	if after.GetString("known_gaps") != "none yet" {
		t.Errorf("known_gaps = %q, want %q", after.GetString("known_gaps"), "none yet")
	}
	var queries []ExampleQuery
	if err := after.UnmarshalJSONField("example_queries", &queries); err != nil {
		t.Fatalf("unmarshaling example_queries: %v", err)
	}
	if len(queries) != 1 || queries[0].Question != "how many widgets?" {
		t.Errorf("unexpected example_queries: %+v", queries)
	}

	// Omitting every annotate field must leave everything unchanged.
	e2, rec2 := newTestAnnotateEvent(t, app, "/tables/annotate", `{"id":"`+id+`"}`)
	if err := handleAnnotateTable()(e2); err != nil {
		t.Fatalf("handleAnnotateTable (omit): %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
	stillThere, err := app.FindRecordById("tables", id)
	if err != nil {
		t.Fatalf("re-reading table: %v", err)
	}
	if stillThere.GetString("good_for") != "aggregate queries" {
		t.Errorf("expected good_for to survive an omitted patch, got %q", stillThere.GetString("good_for"))
	}

	// An explicit empty string/array clears.
	e3, rec3 := newTestAnnotateEvent(t, app, "/tables/annotate", `{"id":"`+id+`","good_for":"","example_queries":[]}`)
	if err := handleAnnotateTable()(e3); err != nil {
		t.Fatalf("handleAnnotateTable (clear): %v", err)
	}
	if rec3.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec3.Code)
	}
	cleared, err := app.FindRecordById("tables", id)
	if err != nil {
		t.Fatalf("re-reading table: %v", err)
	}
	if cleared.GetString("good_for") != "" {
		t.Errorf("expected good_for cleared, got %q", cleared.GetString("good_for"))
	}
	if cleared.GetString("bad_for") != "row-level lookups" {
		t.Errorf("expected bad_for (not part of the clear patch) to survive, got %q", cleared.GetString("bad_for"))
	}
	var clearedQueries []ExampleQuery
	if err := cleared.UnmarshalJSONField("example_queries", &clearedQueries); err != nil {
		t.Fatalf("unmarshaling example_queries: %v", err)
	}
	if len(clearedQueries) != 0 {
		t.Errorf("expected example_queries cleared, got %+v", clearedQueries)
	}
}

// TestHandleAnnotateDatasource_GlossaryAndExampleQueries mirrors
// TestHandleAnnotateTable_CuratedFields for the datasource-level glossary/
// example_queries fields.
func TestHandleAnnotateDatasource_GlossaryAndExampleQueries(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{typ: "widgets"}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})
	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	body := `{"name":"widgets","glossary":[{"term":"SKU","definition":"Stock Keeping Unit"}],"example_queries":[{"question":"list all widgets","sql":"SELECT * FROM widgets"}]}`
	e, rec := newTestAnnotateEvent(t, app, "/datasources/annotate", body)
	if err := handleAnnotateDatasource()(e); err != nil {
		t.Fatalf("handleAnnotateDatasource: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	after, err := app.FindFirstRecordByFilter("datasources", "name = 'widgets'")
	if err != nil {
		t.Fatalf("re-reading datasource: %v", err)
	}
	var glossary []GlossaryEntry
	if err := after.UnmarshalJSONField("glossary", &glossary); err != nil {
		t.Fatalf("unmarshaling glossary: %v", err)
	}
	if len(glossary) != 1 || glossary[0].Term != "SKU" {
		t.Errorf("unexpected glossary: %+v", glossary)
	}

	// Omitting glossary/example_queries leaves them unchanged.
	e2, rec2 := newTestAnnotateEvent(t, app, "/datasources/annotate", `{"name":"widgets"}`)
	if err := handleAnnotateDatasource()(e2); err != nil {
		t.Fatalf("handleAnnotateDatasource (omit): %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
	stillThere, err := app.FindFirstRecordByFilter("datasources", "name = 'widgets'")
	if err != nil {
		t.Fatalf("re-reading datasource: %v", err)
	}
	var stillGlossary []GlossaryEntry
	if err := stillThere.UnmarshalJSONField("glossary", &stillGlossary); err != nil {
		t.Fatalf("unmarshaling glossary: %v", err)
	}
	if len(stillGlossary) != 1 {
		t.Errorf("expected glossary to survive an omitted patch, got %+v", stillGlossary)
	}

	// An explicit empty array clears.
	e3, rec3 := newTestAnnotateEvent(t, app, "/datasources/annotate", `{"name":"widgets","glossary":[]}`)
	if err := handleAnnotateDatasource()(e3); err != nil {
		t.Fatalf("handleAnnotateDatasource (clear): %v", err)
	}
	if rec3.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec3.Code)
	}
	cleared, err := app.FindFirstRecordByFilter("datasources", "name = 'widgets'")
	if err != nil {
		t.Fatalf("re-reading datasource: %v", err)
	}
	var clearedGlossary []GlossaryEntry
	if err := cleared.UnmarshalJSONField("glossary", &clearedGlossary); err != nil {
		t.Fatalf("unmarshaling glossary: %v", err)
	}
	if len(clearedGlossary) != 0 {
		t.Errorf("expected glossary cleared, got %+v", clearedGlossary)
	}
}
