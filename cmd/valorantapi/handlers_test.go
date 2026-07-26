package main

import (
	"context"
	"testing"

	"val-analyzer/internal/valorant"
)

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
