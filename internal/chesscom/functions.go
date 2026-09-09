package chesscom

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"val-analyzer/internal/chesscom/ingest"
	"val-analyzer/internal/dataapi"
)

// defaultSyncMonthCount/maxSyncMonthCount bound sync_games' "new (non-
// current) months per call" safety cap - named distinctly from
// sync_matches' own "count" (a match count) since "count" here would
// ambiguously suggest a game count, not a month count, to an LLM caller.
const (
	defaultSyncMonthCount = 24
	maxSyncMonthCount     = 60
)

func resolvePlayerFunction(ing *ingest.Service) dataapi.Function {
	return dataapi.Function{
		Name:        "resolve_player",
		Description: "Resolve a chess.com username into a cached player identity. Does not fetch game history - use sync_games for that.",
		Args: []dataapi.FunctionArg{
			{Name: "username", Type: "string", Description: "The chess.com username.", Required: true},
		},
		Run: func(ctx context.Context, args map[string]any) (dataapi.FunctionOutcome, error) {
			username, _ := args["username"].(string)
			if username == "" {
				return dataapi.FunctionOutcome{}, fmt.Errorf("resolve_player requires a non-empty username")
			}

			player, err := ing.ResolvePlayer(ctx, username)
			if err != nil {
				slog.Error("chesscom: resolve_player failed", "username", username, "error", err)
				return dataapi.FunctionOutcome{}, fmt.Errorf("failed to resolve %s: %w", username, err)
			}

			return dataapi.FunctionOutcome{
				Summary: fmt.Sprintf("resolved %s to player_id %d", player.Username, player.PlayerID),
				Data: map[string]any{
					"username":  player.Username,
					"player_id": player.PlayerID,
					"title":     player.Title,
					"country":   player.CountryCode,
				},
			}, nil
		},
	}
}

// syncStatsFunction always does a full, unconditional refresh - mirrors
// Valorant's sync_seasons' own "always refresh, no coverage logic" idiom,
// since chess.com's stats endpoint always reports every category's
// complete current standing in one call.
func syncStatsFunction(ing *ingest.Service) dataapi.Function {
	return dataapi.Function{
		Name: "sync_stats",
		Description: "Fetch and cache a chess.com player's profile and rating stats across every category chess.com reports (bullet/blitz/rapid/" +
			"daily, chess960 variants, tactics, puzzle rush, fide, ...). Always a full refresh - safe to call repeatedly.",
		Args: []dataapi.FunctionArg{
			{Name: "username", Type: "string", Description: "The chess.com username.", Required: true},
		},
		Run: func(ctx context.Context, args map[string]any) (dataapi.FunctionOutcome, error) {
			username, _ := args["username"].(string)
			if username == "" {
				return dataapi.FunctionOutcome{}, fmt.Errorf("sync_stats requires a non-empty username")
			}

			result, err := ing.SyncStats(ctx, username)
			if err != nil {
				slog.Error("chesscom: sync_stats failed", "username", username, "error", err)
				return dataapi.FunctionOutcome{}, fmt.Errorf("failed to sync stats for %s: %w", username, err)
			}

			return dataapi.FunctionOutcome{
				Summary: fmt.Sprintf("synced %d rating categories for %s", result.CategoriesSynced, username),
				Data:    map[string]any{"categories_synced": result.CategoriesSynced},
			}, nil
		},
	}
}

func syncGamesFunction(ing *ingest.Service) dataapi.Function {
	return dataapi.Function{
		Name: "sync_games",
		Description: "Fetch and cache a chess.com player's games, populating games/game_players/moves. Chess.com's own archive list is complete " +
			"and finite (unlike Valorant's most-recent-N match history), so this walks it from the most recent month backward, stopping once " +
			"it reconnects with already-cached months - always re-fetching the still-open current month regardless, since it can gain new " +
			"games at any time.",
		Args: []dataapi.FunctionArg{
			{Name: "username", Type: "string", Description: "The chess.com username.", Required: true},
			{Name: "start_date", Type: "string", Description: "ISO-8601 date (e.g. \"2026-05-01\"), the earliest month to ensure is cached - day-of-month is ignored. Omit for a plain most-recent-months sync.", Required: false},
			{Name: "end_date", Type: "string", Description: "ISO-8601 date, the latest month to ensure is cached - day-of-month is ignored. Defaults to now if start_date is given.", Required: false},
			{Name: "max_months", Type: "integer", Description: fmt.Sprintf("Safety cap on how many new (non-current) months to fetch this call, up to %d. Defaults to %d. Ignored if full_history is true.", maxSyncMonthCount, defaultSyncMonthCount), Required: false},
			{Name: "full_history", Type: "boolean", Description: "Sync a player's entire game history instead of a bounded number of months - only set this when a user has explicitly asked for their full/entire history, not for a normal question.", Required: false},
		},
		Run: func(ctx context.Context, args map[string]any) (dataapi.FunctionOutcome, error) {
			username, _ := args["username"].(string)
			if username == "" {
				return dataapi.FunctionOutcome{}, fmt.Errorf("sync_games requires a non-empty username")
			}

			opts := ingest.SyncOptions{MaxMonths: defaultSyncMonthCount}
			if full, ok := args["full_history"].(bool); ok && full {
				opts.All = true
			}
			if mm, ok := args["max_months"].(float64); ok && mm > 0 {
				opts.MaxMonths = int(mm)
			}
			if opts.MaxMonths > maxSyncMonthCount {
				opts.MaxMonths = maxSyncMonthCount
			}

			if sd, ok := args["start_date"].(string); ok && sd != "" {
				since, err := parseFlexibleDate(sd)
				if err != nil {
					return dataapi.FunctionOutcome{}, fmt.Errorf("invalid start_date %q: %w", sd, err)
				}
				opts.Since = &since
			}
			if ed, ok := args["end_date"].(string); ok && ed != "" {
				until, err := parseFlexibleDate(ed)
				if err != nil {
					return dataapi.FunctionOutcome{}, fmt.Errorf("invalid end_date %q: %w", ed, err)
				}
				opts.Until = &until
			}

			player, err := ing.ResolvePlayer(ctx, username)
			if err != nil {
				slog.Error("chesscom: sync_games failed to resolve player", "username", username, "error", err)
				return dataapi.FunctionOutcome{}, fmt.Errorf("failed to resolve %s: %w", username, err)
			}

			coverage, err := ing.CheckGamesCoverage(player, opts)
			if err != nil {
				slog.Error("chesscom: sync_games failed to check cache coverage", "username", username, "error", err)
				return dataapi.FunctionOutcome{}, fmt.Errorf("failed to check cache coverage for %s: %w", username, err)
			}
			if coverage.Covered {
				slog.Info("chesscom: sync_games skipped upstream sync, cache already covers request", "username", username)
				return dataapi.FunctionOutcome{
					Summary: fmt.Sprintf("cache already covers this request for %s - no upstream sync needed", player.Username),
					Data:    map[string]any{"games_fetched": 0, "skipped": true},
				}, nil
			}

			result, err := ing.SyncGames(ctx, player, opts)
			if err != nil {
				slog.Error("chesscom: sync_games failed to sync", "username", username, "error", err)
				return dataapi.FunctionOutcome{}, fmt.Errorf("failed to sync games for %s: %w", username, err)
			}

			return dataapi.FunctionOutcome{
				Summary: fmt.Sprintf("synced %d new games across %d months for %s", result.GamesFetched, result.MonthsFetched, player.Username),
				Data: map[string]any{
					"games_fetched":  result.GamesFetched,
					"months_fetched": result.MonthsFetched,
				},
			}, nil
		},
	}
}

// reparseMovesFunction is a direct parallel to Valorant's
// backfill_match_seasons repair-without-re-hitting-upstream idiom, enabled
// for free by storing raw PGN on every game row.
func reparseMovesFunction(ing *ingest.Service) dataapi.Function {
	return dataapi.Function{
		Name: "reparse_moves",
		Description: "Re-parse already-stored PGN text into the moves table for every game a username played (or every stored game, if " +
			"username is omitted) - a repair action with zero upstream calls, useful if a PGN-parser bug is found and fixed after games were " +
			"already ingested.",
		Args: []dataapi.FunctionArg{
			{Name: "username", Type: "string", Description: "Limit to games this username played in. Omit to re-parse every stored game.", Required: false},
		},
		Run: func(ctx context.Context, args map[string]any) (dataapi.FunctionOutcome, error) {
			username, _ := args["username"].(string)

			result, err := ing.ReparseMoves(username)
			if err != nil {
				slog.Error("chesscom: reparse_moves failed", "username", username, "error", err)
				return dataapi.FunctionOutcome{}, fmt.Errorf("failed to reparse moves: %w", err)
			}

			return dataapi.FunctionOutcome{
				Summary: fmt.Sprintf("reparsed moves for %d games (%d failed to parse)", result.GamesReparsed, result.Failed),
				Data: map[string]any{
					"games_reparsed": result.GamesReparsed,
					"failed":         result.Failed,
				},
			}, nil
		},
	}
}

// parseFlexibleDate accepts either a full RFC3339 timestamp or a bare
// YYYY-MM-DD date, since a caller (human or AI) may give either.
func parseFlexibleDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected an RFC3339 timestamp or YYYY-MM-DD date")
}

// Functions returns every warm-triggerable action cmd/chesscomapi's /warm
// handler can dispatch to, keyed by Function.Name.
func Functions(ing *ingest.Service) []dataapi.Function {
	return []dataapi.Function{
		resolvePlayerFunction(ing),
		syncStatsFunction(ing),
		syncGamesFunction(ing),
		reparseMovesFunction(ing),
	}
}
