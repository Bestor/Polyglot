package polyglot

import (
	"context"
	"encoding/json"
	"strings"
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
		functionsCapable: true,
		functions: []dataprovider.FunctionCatalog{
			{Name: "restock", Description: "restocks widgets", Args: []dataprovider.FunctionArg{
				{Name: "sku", Type: "string", Description: "which widget", Required: true},
			}},
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
	dsRec.Set("glossary", []GlossaryEntry{{Term: "SKU", Definition: "Stock Keeping Unit"}})
	dsRec.Set("example_queries", []ExampleQuery{{Question: "list all widgets", SQL: "SELECT * FROM widgets"}})
	if err := app.Save(dsRec); err != nil {
		t.Fatalf("annotating datasource: %v", err)
	}

	tableRec, err := app.FindFirstRecordByFilter("tables", "name = 'widgets'")
	if err != nil {
		t.Fatalf("finding tables row: %v", err)
	}
	tableRec.Set("description", "cached widgets")
	tableRec.Set("good_for", "aggregate stock queries")
	tableRec.Set("bad_for", "per-warehouse breakdowns")
	tableRec.Set("known_gaps", "pre-2026 stock history was never backfilled")
	tableRec.Set("example_queries", []ExampleQuery{{Question: "how many widgets?", SQL: "SELECT COUNT(*) FROM widgets"}})
	tableRec.Set("row_count", 7)
	tableRec.Set("sample_rows", []map[string]any{{"sku": "A1"}})
	if err := app.Save(tableRec); err != nil {
		t.Fatalf("annotating table: %v", err)
	}

	columnRec, err := app.FindFirstRecordByFilter("columns", "name = 'sku'")
	if err != nil {
		t.Fatalf("finding sku column: %v", err)
	}
	columnRec.Set("references_table", "products")
	columnRec.Set("references_column", "id")
	if err := app.Save(columnRec); err != nil {
		t.Fatalf("annotating column: %v", err)
	}

	functionRec, err := app.FindFirstRecordByFilter("functions", "name = 'restock'")
	if err != nil {
		t.Fatalf("finding functions row: %v", err)
	}
	functionRec.Set("query_guidance", "confirm with the user before restocking")
	if err := app.Save(functionRec); err != nil {
		t.Fatalf("annotating function: %v", err)
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
	if len(metadata.Datasources[0].Glossary) != 1 || metadata.Datasources[0].Glossary[0].Term != "SKU" {
		t.Errorf("expected curated datasource glossary, got %+v", metadata.Datasources[0].Glossary)
	}
	if len(metadata.Datasources[0].ExampleQueries) != 1 {
		t.Errorf("expected curated datasource example_queries, got %+v", metadata.Datasources[0].ExampleQueries)
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
	if widgetsTable.GoodFor != "aggregate stock queries" {
		t.Errorf("expected curated good_for, got %q", widgetsTable.GoodFor)
	}
	if widgetsTable.BadFor != "per-warehouse breakdowns" {
		t.Errorf("expected curated bad_for, got %q", widgetsTable.BadFor)
	}
	if widgetsTable.KnownGaps != "pre-2026 stock history was never backfilled" {
		t.Errorf("expected curated known_gaps, got %q", widgetsTable.KnownGaps)
	}
	if len(widgetsTable.ExampleQueries) != 1 {
		t.Errorf("expected curated table example_queries, got %+v", widgetsTable.ExampleQueries)
	}
	if widgetsTable.RowCount != 7 {
		t.Errorf("expected introspected row_count 7, got %d", widgetsTable.RowCount)
	}
	if len(widgetsTable.SampleRows) != 1 {
		t.Errorf("expected introspected sample_rows, got %+v", widgetsTable.SampleRows)
	}
	if len(widgetsTable.Columns) != 1 || widgetsTable.Columns[0].Name != "sku" {
		t.Fatalf("expected one sku column, got %+v", widgetsTable.Columns)
	}
	if widgetsTable.Columns[0].Type != "TEXT" {
		t.Errorf("expected column type TEXT, got %q", widgetsTable.Columns[0].Type)
	}
	if widgetsTable.Columns[0].ReferencesTable != "products" || widgetsTable.Columns[0].ReferencesColumn != "id" {
		t.Errorf("expected introspected relation, got references_table=%q references_column=%q",
			widgetsTable.Columns[0].ReferencesTable, widgetsTable.Columns[0].ReferencesColumn)
	}

	if len(metadata.Functions) != 1 {
		t.Fatalf("expected one function, got %+v", metadata.Functions)
	}
	restock := metadata.Functions[0]
	if restock.Name != "restock" || restock.Datasource != "widgets" {
		t.Errorf("unexpected function identity: %+v", restock)
	}
	if restock.Description != "restocks widgets" {
		t.Errorf("expected auto-derived description, got %q", restock.Description)
	}
	if restock.QueryGuidance != "confirm with the user before restocking" {
		t.Errorf("expected curated query_guidance, got %q", restock.QueryGuidance)
	}
	if len(restock.Args) != 1 || restock.Args[0].Name != "sku" || !restock.Args[0].Required {
		t.Errorf("unexpected args: %+v", restock.Args)
	}
}

func TestBuildMetadata_EmptyCatalog(t *testing.T) {
	app := newTestApp(t)
	metadata, err := buildMetadata(app)
	if err != nil {
		t.Fatalf("buildMetadata: %v", err)
	}
	if len(metadata.Datasources) != 0 || len(metadata.Tables) != 0 || len(metadata.Functions) != 0 {
		t.Errorf("expected an empty catalog on a fresh app, got %+v", metadata)
	}
}

// TestBuildMetadata_UnannotatedArrayFieldsAreEmptyNotNull is a regression
// test for a real bug caught live: a JSON-array curated/introspected field
// that was never Set on a record (glossary/example_queries/sample_rows,
// before any annotation or a reconcile pass that populates them) unmarshals
// to a nil Go slice, which then re-serializes as JSON `null` rather than
// `[]` - and every webui page assumed an array, so `null.length` crashed
// the Data Explorer's datasource page outright the first time a datasource
// had glossary set but example_queries never touched. Asserts against the
// actual marshaled JSON, since that's where the bug was observable - the
// Go value being nil internally was never the problem by itself.
func TestBuildMetadata_UnannotatedArrayFieldsAreEmptyNotNull(t *testing.T) {
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

	// Deliberately no annotation at all - this exercises every array field
	// at its true just-onboarded default.
	metadata, err := buildMetadata(app)
	if err != nil {
		t.Fatalf("buildMetadata: %v", err)
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshaling metadata: %v", err)
	}
	body := string(encoded)

	for _, field := range []string{`"glossary":null`, `"example_queries":null`, `"sample_rows":null`} {
		if strings.Contains(body, field) {
			t.Errorf("expected no null array fields in the response, found %q in %s", field, body)
		}
	}
	for _, field := range []string{`"glossary":[]`, `"example_queries":[]`, `"sample_rows":[]`} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %q in the response, got %s", field, body)
		}
	}
}
