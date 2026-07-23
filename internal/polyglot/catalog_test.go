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
