package polyglot

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"val-analyzer/internal/dataprovider"
	_ "val-analyzer/internal/migrations"
)

func TestBuildMetadata(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	if _, err := core.NewMigrationsRunner(app, core.AppMigrations).Up(); err != nil {
		t.Fatalf("running app migrations: %v", err)
	}

	active := []ActiveInstance{
		{
			Type: "valorant",
			Tables: []dataprovider.TableSpec{
				{
					Name:        "players",
					Description: "cached players",
					Fields: []dataprovider.FieldSpec{
						{Name: "riot_puuid", Description: "stable id"},
					},
				},
			},
			Functions: []dataprovider.Function{
				{
					Name:        "resolve_player",
					Description: "resolve a player",
					Args: []dataprovider.FunctionArg{
						{Name: "name", Type: "string", Required: true},
						{Name: "tag", Type: "string", Required: true},
					},
				},
			},
		},
	}

	metadata, err := buildMetadata(app, active)
	if err != nil {
		t.Fatalf("buildMetadata: %v", err)
	}

	if len(metadata.Tables) == 0 {
		t.Fatal("expected at least one table in metadata")
	}

	var playersTable *TableDescription
	for i := range metadata.Tables {
		if metadata.Tables[i].Name == "players" {
			playersTable = &metadata.Tables[i]
		}
	}
	if playersTable == nil {
		t.Fatal("players table missing from metadata")
	}
	if playersTable.Datasource != "valorant" {
		t.Errorf("expected players table tagged with datasource %q, got %q", "valorant", playersTable.Datasource)
	}

	if len(metadata.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(metadata.Functions))
	}
	if metadata.Functions[0].Name != "resolve_player" {
		t.Errorf("expected resolve_player, got %q", metadata.Functions[0].Name)
	}
	if metadata.Functions[0].Datasource != "valorant" {
		t.Errorf("expected function tagged with datasource %q, got %q", "valorant", metadata.Functions[0].Datasource)
	}
	if len(metadata.Functions[0].Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(metadata.Functions[0].Args))
	}
	if !metadata.Functions[0].Args[0].Required {
		t.Error("expected name arg to be required")
	}
}
