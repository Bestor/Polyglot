package ai

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	_ "val-analyzer/internal/migrations"
)

func TestBuildSchema(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	if _, err := core.NewMigrationsRunner(app, core.AppMigrations).Up(); err != nil {
		t.Fatalf("running app migrations: %v", err)
	}

	schema, err := BuildSchema(app)
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}

	if len(schema) != len(tableNames) {
		t.Fatalf("expected %d tables, got %d", len(tableNames), len(schema))
	}

	var matchPlayers *TableDescription
	for i := range schema {
		if schema[i].Name == "match_players" {
			matchPlayers = &schema[i]
		}
	}
	if matchPlayers == nil {
		t.Fatal("match_players table missing from schema")
	}

	var headshotsDescribed bool
	for _, col := range matchPlayers.Columns {
		if col.Name == "headshots" {
			headshotsDescribed = col.Description != ""
		}
	}
	if !headshotsDescribed {
		t.Error("expected match_players.headshots to have a non-empty description")
	}
}
