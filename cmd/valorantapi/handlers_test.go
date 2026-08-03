package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"

	"val-analyzer/internal/valorant"
	_ "val-analyzer/internal/valorant/migrations"
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
	functions := []valorant.Function{
		{
			Name:        "sync_matches",
			Description: "Fetch and cache a player's matches.",
			Args: []valorant.FunctionArg{
				{Name: "player_tag", Type: "string", Description: "The player's Riot ID.", Required: true},
				{Name: "count", Type: "integer", Description: "How many matches.", Required: false},
			},
			Run: func(ctx context.Context, args map[string]any) (valorant.FunctionOutcome, error) {
				return valorant.FunctionOutcome{}, nil
			},
		},
		{
			Name:        "sync_seasons",
			Description: "Fetch the season list.",
		},
	}

	resp := buildFunctionsResponse(functions)

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

// TestHandleSchema_DetectsRelations proves handleSchema resolves a real
// PocketBase *core.RelationField's CollectionId into the target table's
// name - the mechanical foreign-key-detection path core polyglot's
// httpsql provider relies on (see internal/providers/httpsql, which
// decodes this response directly into dataprovider.TableCatalog with no
// reshaping). match_players.player is a real relation to players in
// internal/valorant/migrations, not a fake - this is the same relation an
// AI conversation once hallucinated a wrong join column for.
func TestHandleSchema_DetectsRelations(t *testing.T) {
	app := newTestApp(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/schema", nil)
	e := &core.RequestEvent{App: app, Event: router.Event{Response: rec, Request: req}}

	if err := handleSchema(app)(e); err != nil {
		t.Fatalf("handleSchema: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp schemaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	var matchPlayers *schemaTable
	for i := range resp.Tables {
		if resp.Tables[i].Name == "match_players" {
			matchPlayers = &resp.Tables[i]
		}
	}
	if matchPlayers == nil {
		t.Fatal("match_players table missing from schema")
	}

	var player *schemaColumn
	for i := range matchPlayers.Columns {
		if matchPlayers.Columns[i].Name == "player" {
			player = &matchPlayers.Columns[i]
		}
	}
	if player == nil {
		t.Fatal("player column missing from match_players")
	}
	if player.ReferencesTable != "players" {
		t.Errorf("references_table = %q, want %q", player.ReferencesTable, "players")
	}
	if player.ReferencesColumn != "id" {
		t.Errorf("references_column = %q, want %q", player.ReferencesColumn, "id")
	}

	// A plain, non-relation column must not get a relation.
	var matchId *schemaColumn
	for i := range matchPlayers.Columns {
		if matchPlayers.Columns[i].Name == "party_id" {
			matchId = &matchPlayers.Columns[i]
		}
	}
	if matchId == nil {
		t.Fatal("party_id column missing from match_players")
	}
	if matchId.ReferencesTable != "" || matchId.ReferencesColumn != "" {
		t.Errorf("expected no relation on a plain text column, got references_table=%q references_column=%q",
			matchId.ReferencesTable, matchId.ReferencesColumn)
	}
}
