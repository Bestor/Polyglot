package polyglot

import (
	"context"
	"strings"
	"testing"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/jobstore"
)

// TestReconcileCatalog_NeverClobbersCuratedAnnotations is the single most
// important behavior to lock in: re-running catalog reconciliation after a
// human has annotated a table/column must never overwrite that curation.
func TestReconcileCatalog_NeverClobbersCuratedAnnotations(t *testing.T) {
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
		t.Fatalf("expected a tables row for widgets: %v", err)
	}
	tableRec.Set("description", "human-curated description")
	tableRec.Set("query_guidance", "human-curated guidance")
	if err := app.Save(tableRec); err != nil {
		t.Fatalf("saving annotation: %v", err)
	}

	columnRec, err := app.FindFirstRecordByFilter("columns", "name = 'sku'")
	if err != nil {
		t.Fatalf("expected a columns row for sku: %v", err)
	}
	columnRec.Set("description", "human-curated column note")
	if err := app.Save(columnRec); err != nil {
		t.Fatalf("saving column annotation: %v", err)
	}

	job, err := reg.Reconcile(app, "widgets")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	finished := waitForJob(t, jobs, job.ID)
	if finished.Status != jobstore.Succeeded {
		t.Fatalf("expected the second reconcile to succeed, got %+v", finished)
	}

	tableAfter, err := app.FindFirstRecordByFilter("tables", "name = 'widgets'")
	if err != nil {
		t.Fatalf("re-reading table: %v", err)
	}
	if tableAfter.GetString("description") != "human-curated description" {
		t.Errorf("expected description to survive reconcile, got %q", tableAfter.GetString("description"))
	}
	if tableAfter.GetString("query_guidance") != "human-curated guidance" {
		t.Errorf("expected query_guidance to survive reconcile, got %q", tableAfter.GetString("query_guidance"))
	}

	columnAfter, err := app.FindFirstRecordByFilter("columns", "name = 'sku'")
	if err != nil {
		t.Fatalf("re-reading column: %v", err)
	}
	if columnAfter.GetString("description") != "human-curated column note" {
		t.Errorf("expected column description to survive reconcile, got %q", columnAfter.GetString("description"))
	}
}

// TestReconcileCatalog_AddsAndRemovesTables proves reconcileCatalog
// actually tracks live schema changes, not just a no-op re-save: a table
// that disappears from Instance.Catalog() is deleted (cascading its
// columns), and a newly appeared one is added.
func TestReconcileCatalog_AddsAndRemovesTables(t *testing.T) {
	app := newTestApp(t)
	provider := &mutableFakeProvider{
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

	provider.instance.catalog = []dataprovider.TableCatalog{
		{Name: "gadgets", Columns: []dataprovider.ColumnCatalog{{Name: "id", Type: "TEXT"}}},
	}

	job, err := reg.Reconcile(app, "widgets")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	waitForJob(t, jobs, job.ID)

	if _, err := app.FindFirstRecordByFilter("tables", "name = 'widgets'"); err == nil {
		t.Error("expected the widgets table row to have been deleted")
	}
	if _, err := app.FindFirstRecordByFilter("tables", "name = 'gadgets'"); err != nil {
		t.Error("expected a new gadgets table row to have been created")
	}
	if _, err := app.FindFirstRecordByFilter("columns", "name = 'sku'"); err == nil {
		t.Error("expected the sku column to have been cascade-deleted along with its table")
	}
}

// TestReconcileCatalog_ComputesTableStats proves reconcileTableStats
// populates row_count/sample_rows/last_updated from the datasource's own
// Query method (not any provider-specific introspection) - and that
// last_updated is only set when the table has an "updated" column, per
// its own best-effort doc comment.
func TestReconcileCatalog_ComputesTableStats(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ: "widgets",
		catalog: []dataprovider.TableCatalog{
			{Name: "widgets", Columns: []dataprovider.ColumnCatalog{
				{Name: "sku", Type: "TEXT"},
				{Name: "updated", Type: "TEXT"},
			}},
		},
		queryFunc: func(sqlText string) (ai.QueryResult, error) {
			switch {
			case strings.Contains(sqlText, "COUNT(*)"):
				return ai.QueryResult{Columns: []string{"n"}, Rows: [][]any{{int64(42)}}}, nil
			case strings.Contains(sqlText, "LIMIT 5"):
				return ai.QueryResult{Columns: []string{"sku"}, Rows: [][]any{{"A1"}, {"B2"}}}, nil
			case strings.Contains(sqlText, "MAX("):
				return ai.QueryResult{Columns: []string{"m"}, Rows: [][]any{{"2026-07-29T04:08:26Z"}}}, nil
			default:
				return ai.QueryResult{}, nil
			}
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
	if tableRec.GetInt("row_count") != 42 {
		t.Errorf("row_count = %d, want 42", tableRec.GetInt("row_count"))
	}
	var sampleRows []map[string]any
	if err := tableRec.UnmarshalJSONField("sample_rows", &sampleRows); err != nil {
		t.Fatalf("unmarshaling sample_rows: %v", err)
	}
	if len(sampleRows) != 2 {
		t.Errorf("expected 2 sample rows, got %+v", sampleRows)
	}
	if tableRec.GetDateTime("last_updated").Time().IsZero() {
		t.Error("expected last_updated to be set for a table with an updated column")
	}
}

// TestReconcileCatalog_NoUpdatedColumn_SkipsFreshness proves a table
// without an "updated" column never issues the MAX(...) freshness query
// and leaves last_updated unset - the heuristic isn't a universal
// guarantee.
func TestReconcileCatalog_NoUpdatedColumn_SkipsFreshness(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ: "widgets",
		catalog: []dataprovider.TableCatalog{
			{Name: "widgets", Columns: []dataprovider.ColumnCatalog{{Name: "sku", Type: "TEXT"}}},
		},
		queryFunc: func(sqlText string) (ai.QueryResult, error) {
			if strings.Contains(sqlText, "MAX(") {
				t.Error("did not expect a freshness query for a table with no updated column")
			}
			return ai.QueryResult{}, nil
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
	if !tableRec.GetDateTime("last_updated").Time().IsZero() {
		t.Errorf("expected last_updated to be unset, got %v", tableRec.GetDateTime("last_updated"))
	}
}

// TestReconcileCatalog_RefreshesRelations proves references_table/
// references_column are introspected like type, not curated like
// description: set on create, and refreshed (not preserved) when the live
// source's relation target changes between two reconcile passes.
func TestReconcileCatalog_RefreshesRelations(t *testing.T) {
	app := newTestApp(t)
	provider := &mutableFakeProvider{
		catalog: []dataprovider.TableCatalog{
			{Name: "match_players", Columns: []dataprovider.ColumnCatalog{
				{Name: "player", Type: "TEXT", ReferencesTable: "players", ReferencesColumn: "id"},
			}},
		},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	colRec, err := app.FindFirstRecordByFilter("columns", "name = 'player'")
	if err != nil {
		t.Fatalf("finding player column: %v", err)
	}
	if colRec.GetString("references_table") != "players" || colRec.GetString("references_column") != "id" {
		t.Errorf("expected relation set on create, got references_table=%q references_column=%q",
			colRec.GetString("references_table"), colRec.GetString("references_column"))
	}

	// The relation's target changes (e.g. a schema change upstream) -
	// references_table/references_column must refresh, unlike a curated
	// field.
	provider.instance.catalog = []dataprovider.TableCatalog{
		{Name: "match_players", Columns: []dataprovider.ColumnCatalog{
			{Name: "player", Type: "TEXT", ReferencesTable: "accounts", ReferencesColumn: "id"},
		}},
	}
	job, err := reg.Reconcile(app, "widgets")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	waitForJob(t, jobs, job.ID)

	after, err := app.FindFirstRecordByFilter("columns", "name = 'player'")
	if err != nil {
		t.Fatalf("re-reading player column: %v", err)
	}
	if after.GetString("references_table") != "accounts" {
		t.Errorf("expected references_table refreshed to %q, got %q", "accounts", after.GetString("references_table"))
	}
}

// mutableFakeProvider lets a test change what Catalog() returns between
// two reconcile passes, unlike fakeProvider's fixed catalog.
type mutableFakeProvider struct {
	catalog  []dataprovider.TableCatalog
	instance *fakeInstance
}

func (p *mutableFakeProvider) Type() string { return "widgets" }
func (p *mutableFakeProvider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{{Name: "api_key", Type: "string", Required: true}}
}
func (p *mutableFakeProvider) New(ctx context.Context, config map[string]any) (dataprovider.Instance, error) {
	p.instance = &fakeInstance{catalog: p.catalog}
	return p.instance, nil
}

// TestReconcileCatalog_SkipsFunctionsForNonFunctionRunnerInstance proves
// reconcileCatalog's dataprovider.FunctionRunner type-assertion is a plain,
// silent skip for a provider that doesn't implement it (e.g. sqlite) - no
// error, no functions rows.
func TestReconcileCatalog_SkipsFunctionsForNonFunctionRunnerInstance(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{typ: "widgets"} // functionsCapable: false
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	records, err := app.FindAllRecords("functions")
	if err != nil {
		t.Fatalf("listing functions: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected zero functions rows for a non-FunctionRunner instance, got %d", len(records))
	}
}

// TestReconcileCatalog_Functions_InsertsAndRefreshes proves a
// FunctionRunner-capable instance's live functions get inserted with their
// description/args.
func TestReconcileCatalog_Functions_InsertsAndRefreshes(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ:              "widgets",
		functionsCapable: true,
		functions: []dataprovider.FunctionCatalog{
			{Name: "sync", Description: "syncs things", Args: []dataprovider.FunctionArg{
				{Name: "id", Type: "string", Description: "the id", Required: true},
			}},
		},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	rec, err := app.FindFirstRecordByFilter("functions", "name = 'sync'")
	if err != nil {
		t.Fatalf("expected a functions row for sync: %v", err)
	}
	if rec.GetString("description") != "syncs things" {
		t.Errorf("description = %q, want %q", rec.GetString("description"), "syncs things")
	}
	var args []dataprovider.FunctionArg
	if err := rec.UnmarshalJSONField("args", &args); err != nil {
		t.Fatalf("unmarshaling args: %v", err)
	}
	if len(args) != 1 || args[0].Name != "id" || !args[0].Required {
		t.Errorf("unexpected args: %+v", args)
	}
}

// TestReconcileCatalog_Functions_PreservesQueryGuidanceAcrossReconcile
// mirrors TestReconcileCatalog_NeverClobbersCuratedAnnotations for
// functions: query_guidance must survive a reconcile, but - unlike
// tables/columns' description - a function's own description/args are
// expected to change freely, since they always mirror the live source.
func TestReconcileCatalog_Functions_PreservesQueryGuidanceAcrossReconcile(t *testing.T) {
	app := newTestApp(t)
	provider := &mutableFakeFunctionProvider{
		functions: []dataprovider.FunctionCatalog{{Name: "sync", Description: "v1 description"}},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	functionRec, err := app.FindFirstRecordByFilter("functions", "name = 'sync'")
	if err != nil {
		t.Fatalf("expected a functions row for sync: %v", err)
	}
	functionRec.Set("query_guidance", "human-curated guidance")
	if err := app.Save(functionRec); err != nil {
		t.Fatalf("saving annotation: %v", err)
	}

	provider.instance.functions = []dataprovider.FunctionCatalog{{Name: "sync", Description: "v2 description - changed"}}

	job, err := reg.Reconcile(app, "widgets")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	waitForJob(t, jobs, job.ID)

	after, err := app.FindFirstRecordByFilter("functions", "name = 'sync'")
	if err != nil {
		t.Fatalf("re-reading function: %v", err)
	}
	if after.GetString("query_guidance") != "human-curated guidance" {
		t.Errorf("expected query_guidance to survive reconcile, got %q", after.GetString("query_guidance"))
	}
	if after.GetString("description") != "v2 description - changed" {
		t.Errorf("expected description to refresh from the live source, got %q", after.GetString("description"))
	}
}

// TestReconcileCatalog_Functions_AddsAndRemoves mirrors
// TestReconcileCatalog_AddsAndRemovesTables for functions.
func TestReconcileCatalog_Functions_AddsAndRemoves(t *testing.T) {
	app := newTestApp(t)
	provider := &mutableFakeFunctionProvider{
		functions: []dataprovider.FunctionCatalog{{Name: "sync"}},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	provider.instance.functions = []dataprovider.FunctionCatalog{{Name: "resolve"}}

	job, err := reg.Reconcile(app, "widgets")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	waitForJob(t, jobs, job.ID)

	if _, err := app.FindFirstRecordByFilter("functions", "name = 'sync'"); err == nil {
		t.Error("expected the sync function row to have been deleted")
	}
	if _, err := app.FindFirstRecordByFilter("functions", "name = 'resolve'"); err != nil {
		t.Error("expected a new resolve function row to have been created")
	}
}

// mutableFakeFunctionProvider is mutableFakeProvider's equivalent for
// testing function reconciliation - lets a test change what Functions()
// returns between two reconcile passes.
type mutableFakeFunctionProvider struct {
	functions []dataprovider.FunctionCatalog
	instance  *fakeFunctionInstance
}

func (p *mutableFakeFunctionProvider) Type() string { return "widgets" }
func (p *mutableFakeFunctionProvider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{{Name: "api_key", Type: "string", Required: true}}
}
func (p *mutableFakeFunctionProvider) New(ctx context.Context, config map[string]any) (dataprovider.Instance, error) {
	p.instance = &fakeFunctionInstance{fakeInstance: &fakeInstance{}, functions: p.functions}
	return p.instance, nil
}
