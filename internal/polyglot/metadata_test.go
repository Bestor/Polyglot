package polyglot

import (
	"context"
	"testing"

	"val-analyzer/internal/dataprovider"
	_ "val-analyzer/internal/migrations"
)

func TestBuildMetadata(t *testing.T) {
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

	dsRec, err := app.FindFirstRecordByFilter("datasources", "name = 'widgets'")
	if err != nil {
		t.Fatalf("finding datasources row: %v", err)
	}
	dsRec.Set("description", "widget inventory")
	dsRec.Set("query_guidance", "always filter by sku")
	if err := app.Save(dsRec); err != nil {
		t.Fatalf("annotating datasource: %v", err)
	}

	tableRec, err := app.FindFirstRecordByFilter("tables", "name = 'widgets'")
	if err != nil {
		t.Fatalf("finding tables row: %v", err)
	}
	tableRec.Set("description", "cached widgets")
	if err := app.Save(tableRec); err != nil {
		t.Fatalf("annotating table: %v", err)
	}

	metadata, err := buildMetadata(app)
	if err != nil {
		t.Fatalf("buildMetadata: %v", err)
	}

	if len(metadata.Datasources) != 1 || metadata.Datasources[0].Name != "widgets" {
		t.Fatalf("expected one datasource named widgets, got %+v", metadata.Datasources)
	}
	if metadata.Datasources[0].Description != "widget inventory" {
		t.Errorf("expected curated datasource description, got %q", metadata.Datasources[0].Description)
	}
	if metadata.Datasources[0].QueryGuidance != "always filter by sku" {
		t.Errorf("expected curated datasource query_guidance, got %q", metadata.Datasources[0].QueryGuidance)
	}

	var widgetsTable *TableDescription
	for i := range metadata.Tables {
		if metadata.Tables[i].Name == "widgets" {
			widgetsTable = &metadata.Tables[i]
		}
	}
	if widgetsTable == nil {
		t.Fatal("widgets table missing from metadata")
	}
	if widgetsTable.Datasource != "widgets" {
		t.Errorf("expected table tagged with datasource %q, got %q", "widgets", widgetsTable.Datasource)
	}
	if widgetsTable.Description != "cached widgets" {
		t.Errorf("expected curated table description, got %q", widgetsTable.Description)
	}
	if len(widgetsTable.Columns) != 1 || widgetsTable.Columns[0].Name != "sku" {
		t.Fatalf("expected one sku column, got %+v", widgetsTable.Columns)
	}
	if widgetsTable.Columns[0].Type != "TEXT" {
		t.Errorf("expected column type TEXT, got %q", widgetsTable.Columns[0].Type)
	}
}

func TestBuildMetadata_EmptyCatalog(t *testing.T) {
	app := newTestApp(t)
	metadata, err := buildMetadata(app)
	if err != nil {
		t.Fatalf("buildMetadata: %v", err)
	}
	if len(metadata.Datasources) != 0 || len(metadata.Tables) != 0 {
		t.Errorf("expected an empty catalog on a fresh app, got %+v", metadata)
	}
}
