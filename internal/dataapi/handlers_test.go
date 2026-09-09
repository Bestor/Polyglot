package dataapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
)

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)
	if _, err := core.NewMigrationsRunner(app, core.AppMigrations).Up(); err != nil {
		t.Fatalf("running app migrations: %v", err)
	}
	return app
}

func TestBuildFunctionsResponse(t *testing.T) {
	functions := []Function{
		{
			Name:        "sync_matches",
			Description: "Fetch and cache a player's matches.",
			Args: []FunctionArg{
				{Name: "player_tag", Type: "string", Description: "The player's Riot ID.", Required: true},
				{Name: "count", Type: "integer", Description: "How many matches.", Required: false},
			},
			Run: func(ctx context.Context, args map[string]any) (FunctionOutcome, error) {
				return FunctionOutcome{}, nil
			},
		},
		{
			Name:        "sync_seasons",
			Description: "Fetch the season list.",
		},
	}

	resp := BuildFunctionsResponse(functions)

	if len(resp.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(resp.Functions))
	}

	sync := resp.Functions[0]
	if sync.Name != "sync_matches" || sync.Description != "Fetch and cache a player's matches." {
		t.Errorf("unexpected sync_matches shape: %+v", sync)
	}
	if len(sync.Args) != 2 {
		t.Fatalf("expected 2 args, got %+v", sync.Args)
	}
	if sync.Args[0].Name != "player_tag" || sync.Args[0].Type != "string" || !sync.Args[0].Required {
		t.Errorf("unexpected first arg: %+v", sync.Args[0])
	}
	if sync.Args[1].Name != "count" || sync.Args[1].Required {
		t.Errorf("unexpected second arg: %+v", sync.Args[1])
	}

	seasons := resp.Functions[1]
	if seasons.Name != "sync_seasons" || len(seasons.Args) != 0 {
		t.Errorf("unexpected sync_seasons shape: %+v", seasons)
	}
}

// TestHandleSchema_DetectsRelations proves HandleSchema resolves a real
// PocketBase *core.RelationField's CollectionId into the target table's
// name - the mechanical foreign-key-detection path core polyglot's
// httpsql provider relies on (see internal/providers/httpsql, which
// decodes this response directly into dataprovider.TableCatalog with no
// reshaping). Builds its own minimal two-collection fixture directly
// (rather than depending on any domain's migrations) since this package
// has no domain of its own to borrow one from.
func TestHandleSchema_DetectsRelations(t *testing.T) {
	app := newTestApp(t)

	owners := core.NewBaseCollection("owners")
	owners.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(owners); err != nil {
		t.Fatalf("saving owners collection: %v", err)
	}

	widgets := core.NewBaseCollection("widgets")
	widgets.Fields.Add(
		&core.RelationField{Name: "owner", CollectionId: owners.Id},
		&core.TextField{Name: "label"},
	)
	if err := app.Save(widgets); err != nil {
		t.Fatalf("saving widgets collection: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/schema", nil)
	e := &core.RequestEvent{App: app, Event: router.Event{Response: rec, Request: req}}

	if err := HandleSchema(app)(e); err != nil {
		t.Fatalf("HandleSchema: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp schemaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	var widgetsTable *schemaTable
	for i := range resp.Tables {
		if resp.Tables[i].Name == "widgets" {
			widgetsTable = &resp.Tables[i]
		}
	}
	if widgetsTable == nil {
		t.Fatal("widgets table missing from schema")
	}

	var owner *schemaColumn
	for i := range widgetsTable.Columns {
		if widgetsTable.Columns[i].Name == "owner" {
			owner = &widgetsTable.Columns[i]
		}
	}
	if owner == nil {
		t.Fatal("owner column missing from widgets")
	}
	if owner.ReferencesTable != "owners" {
		t.Errorf("references_table = %q, want %q", owner.ReferencesTable, "owners")
	}
	if owner.ReferencesColumn != "id" {
		t.Errorf("references_column = %q, want %q", owner.ReferencesColumn, "id")
	}

	// A plain, non-relation column must not get a relation.
	var label *schemaColumn
	for i := range widgetsTable.Columns {
		if widgetsTable.Columns[i].Name == "label" {
			label = &widgetsTable.Columns[i]
		}
	}
	if label == nil {
		t.Fatal("label column missing from widgets")
	}
	if label.ReferencesTable != "" || label.ReferencesColumn != "" {
		t.Errorf("expected no relation on a plain text column, got references_table=%q references_column=%q",
			label.ReferencesTable, label.ReferencesColumn)
	}
}
