package polyglot

import (
	"context"
	"testing"

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
