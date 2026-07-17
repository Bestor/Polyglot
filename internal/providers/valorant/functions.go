package valorant

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/providers/valorant/ingest"
)

// maxSyncMatchesPerCall bounds how many matches a single sync_matches call
// will fetch/save, so a request can never block unboundedly under the
// data source's rate limit - regardless of whether it's a plain "most
// recent N" sync or a date-range sync that pages backward through
// history.
const maxSyncMatchesPerCall = 100

// defaultSyncMatchCount is used when a sync_matches call doesn't specify count.
const defaultSyncMatchCount = 50

// syncSeasonsFunction fetches the full competitive season/act list from
// the upstream Valorant API and caches it, so matches.season_id_raw can be
// resolved to a human-curated seasons row. Takes no args - it's always a
// full refresh, safe to call repeatedly.
func syncSeasonsFunction(ing *ingest.Service) dataprovider.Function {
	return dataprovider.Function{
		Name: "sync_seasons",
		Description: "Fetch the full list of competitive seasons/acts from the upstream Valorant API and cache them, so matches.season_id_raw " +
			"can be resolved to a seasons row. Always a full refresh (the season list is small and infrequently updated) - safe to call repeatedly.",
		Run: func(ctx context.Context, args map[string]any) (dataprovider.FunctionOutcome, error) {
			result, err := ing.SyncSeasons(ctx)
			if err != nil {
				slog.Error("valorant: sync_seasons failed", "error", err)
				return dataprovider.FunctionOutcome{}, fmt.Errorf("failed to sync seasons: %w", err)
			}

			return dataprovider.FunctionOutcome{
				Summary: fmt.Sprintf("synced %d seasons", result.Count),
				Data: map[string]any{
					"count": result.Count,
				},
			}, nil
		},
	}
}

// backfillMatchSeasonsFunction links any already-cached match whose season
// relation is still empty to its seasons row, now that one may exist -
// see ingest.Service.BackfillMatchSeasons. Takes no args; safe to call
// repeatedly (a no-op once every match with a resolvable season_id_raw has
// been linked).
func backfillMatchSeasonsFunction(ing *ingest.Service) dataprovider.Function {
	return dataprovider.Function{
		Name: "backfill_match_seasons",
		Description: "Link any already-cached match whose season relation is still empty (but season_id_raw is set) to its seasons row, now " +
			"that sync_seasons may have since fetched it. A match's season is only resolved best-effort at ingest time, so a match synced " +
			"before its season was ever cached is permanently missing the link otherwise - this repairs that retroactively. Safe to call " +
			"repeatedly; a no-op once nothing is left to link.",
		Run: func(ctx context.Context, args map[string]any) (dataprovider.FunctionOutcome, error) {
			result, err := ing.BackfillMatchSeasons(ctx)
			if err != nil {
				slog.Error("valorant: backfill_match_seasons failed", "error", err)
				return dataprovider.FunctionOutcome{}, fmt.Errorf("failed to backfill match seasons: %w", err)
			}

			return dataprovider.FunctionOutcome{
				Summary: fmt.Sprintf("linked %d matches to a season, %d still have no matching season cached", result.Updated, result.Skipped),
				Data: map[string]any{
					"updated": result.Updated,
					"skipped": result.Skipped,
				},
			}, nil
		},
	}
}

// resolvePlayerFunction lets a caller resolve a Riot ID (name#tag) into a
// cached player identity, without necessarily syncing any match history.
func resolvePlayerFunction(ing *ingest.Service) dataprovider.Function {
	return dataprovider.Function{
		Name:        "resolve_player",
		Description: "Resolve a Riot ID (name#tag) into a cached player identity. Does not fetch match history - use sync_matches for that.",
		Args: []dataprovider.FunctionArg{
			{Name: "name", Type: "string", Description: "Riot ID name, before the #.", Required: true},
			{Name: "tag", Type: "string", Description: "Riot ID tag, after the #.", Required: true},
		},
		Run: func(ctx context.Context, args map[string]any) (dataprovider.FunctionOutcome, error) {
			name, _ := args["name"].(string)
			tag, _ := args["tag"].(string)
			if name == "" || tag == "" {
				return dataprovider.FunctionOutcome{}, fmt.Errorf("resolve_player requires non-empty name and tag")
			}

			player, err := ing.ResolvePlayer(ctx, name, tag)
			if err != nil {
				slog.Error("valorant: resolve_player failed", "name", name, "tag", tag, "error", err)
				return dataprovider.FunctionOutcome{}, fmt.Errorf("failed to resolve %s#%s: %w", name, tag, err)
			}

			return dataprovider.FunctionOutcome{
				Summary: fmt.Sprintf("resolved %s#%s to puuid %s (region %s)", player.Name, player.Tag, player.PUUID, player.Region),
				Data: map[string]any{
					"puuid":  player.PUUID,
					"name":   player.Name,
					"tag":    player.Tag,
					"region": player.Region,
				},
			}, nil
		},
	}
}

// syncMatchesFunction fetches and caches a player's matches from the
// upstream Valorant API, either by a plain recency count or by a date
// range - see ingest.SyncOptions.
func syncMatchesFunction(ing *ingest.Service) dataprovider.Function {
	return dataprovider.Function{
		Name: "sync_matches",
		Description: "Fetch and cache a player's matches from the upstream Valorant API, populating matches and all related per-match tables " +
			"(match_teams, match_players, rounds, round_player_stats, damage_events, kills, kill_assists, event_player_locations). " +
			"The upstream API only exposes \"most recent N matches\" plus an offset - there is no native date filter - so when a date range " +
			"is given this pages backward through history until it's covered, bounded by the count safety cap (or, if full_history is set, " +
			"by nothing but the upstream's actual history - this can take a long time for a prolific player, but /warm is asynchronous so " +
			"that's fine to let run in the background).",
		Args: []dataprovider.FunctionArg{
			{Name: "player_tag", Type: "string", Description: "The player's Riot ID as name#tag, e.g. \"Orbest#NA1\".", Required: true},
			{Name: "start_date", Type: "string", Description: "ISO-8601 date (e.g. \"2026-05-01\"), the earliest match start date to ensure is cached. Omit for a plain most-recent-matches sync.", Required: false},
			{Name: "end_date", Type: "string", Description: "ISO-8601 date, the latest match start date to ensure is cached. Defaults to now if start_date is given.", Required: false},
			{Name: "count", Type: "integer", Description: fmt.Sprintf("Safety cap on how many matches to fetch this call, up to %d. Defaults to %d. Ignored if full_history is true.", maxSyncMatchesPerCall, defaultSyncMatchCount), Required: false},
			{Name: "full_history", Type: "boolean", Description: "Sync a player's entire match history instead of a bounded count - pages backward until the upstream API itself runs out of matches. Only set this when a user has explicitly asked for their full/entire history, not for a normal question.", Required: false},
		},
		Run: func(ctx context.Context, args map[string]any) (dataprovider.FunctionOutcome, error) {
			playerTag, _ := args["player_tag"].(string)
			name, tag, ok := splitRiotID(playerTag)
			if !ok {
				return dataprovider.FunctionOutcome{}, fmt.Errorf("sync_matches requires player_tag in the form name#tag, got %q", playerTag)
			}

			opts := ingest.SyncOptions{MaxMatches: defaultSyncMatchCount}
			if full, ok := args["full_history"].(bool); ok && full {
				opts.All = true
			}
			if c, ok := args["count"].(float64); ok && c > 0 {
				opts.MaxMatches = int(c)
			}
			if opts.MaxMatches > maxSyncMatchesPerCall {
				opts.MaxMatches = maxSyncMatchesPerCall
			}

			if sd, ok := args["start_date"].(string); ok && sd != "" {
				since, err := parseFlexibleDate(sd)
				if err != nil {
					return dataprovider.FunctionOutcome{}, fmt.Errorf("invalid start_date %q: %w", sd, err)
				}
				opts.Since = &since
			}
			if ed, ok := args["end_date"].(string); ok && ed != "" {
				until, err := parseFlexibleDate(ed)
				if err != nil {
					return dataprovider.FunctionOutcome{}, fmt.Errorf("invalid end_date %q: %w", ed, err)
				}
				opts.Until = &until
			}

			player, err := ing.ResolvePlayer(ctx, name, tag)
			if err != nil {
				slog.Error("valorant: sync_matches failed to resolve player", "name", name, "tag", tag, "error", err)
				return dataprovider.FunctionOutcome{}, fmt.Errorf("failed to resolve %s#%s: %w", name, tag, err)
			}

			coverage, err := ing.CheckCoverage(player, opts.Until)
			if err != nil {
				slog.Error("valorant: sync_matches failed to check cache coverage", "name", name, "tag", tag, "error", err)
				return dataprovider.FunctionOutcome{}, fmt.Errorf("failed to check cache coverage for %s#%s: %w", name, tag, err)
			}
			if coverageSufficient(coverage, opts) {
				slog.Info("valorant: sync_matches skipped upstream sync, cache already covers request",
					"name", name, "tag", tag, "puuid", player.PUUID, "cached_count", coverage.Count)
				return dataprovider.FunctionOutcome{
					Summary: fmt.Sprintf("cache already covers this request for %s#%s (puuid %s) - %d matches cached, no upstream sync needed",
						player.Name, player.Tag, player.PUUID, coverage.Count),
					Data: map[string]any{
						"puuid":             player.PUUID,
						"fetched":           0,
						"skipped":           coverage.Count,
						"history_exhausted": coverage.HistoryExhausted,
					},
				}, nil
			}

			result, err := ing.SyncPlayerMatches(ctx, player, opts)
			if err != nil {
				slog.Error("valorant: sync_matches failed to sync", "name", name, "tag", tag, "error", err)
				return dataprovider.FunctionOutcome{}, fmt.Errorf("failed to sync matches for %s#%s: %w", name, tag, err)
			}

			summary := fmt.Sprintf("synced %d new matches (skipped %d already cached) for %s#%s, puuid %s",
				result.Fetched, result.Skipped, player.Name, player.Tag, player.PUUID)
			if opts.Since != nil {
				switch {
				case result.OldestFetched.IsZero():
					summary += fmt.Sprintf("; no matches found on or after %s", opts.Since.Format("2006-01-02"))
				case result.OldestFetched.After(*opts.Since):
					summary += fmt.Sprintf("; requested window starting %s may not be fully covered - oldest match seen was %s (either the safety cap was hit or that's the start of the player's history)",
						opts.Since.Format("2006-01-02"), result.OldestFetched.Format("2006-01-02"))
				}
			}

			return dataprovider.FunctionOutcome{
				Summary: summary,
				Data: map[string]any{
					"puuid":             player.PUUID,
					"fetched":           result.Fetched,
					"skipped":           result.Skipped,
					"history_exhausted": result.HistoryExhausted,
				},
			}, nil
		},
	}
}

// coverageSufficient decides whether the local cache already satisfies a
// sync_matches call, so it can be skipped without calling the upstream API
// at all. See ingest.CoverageResult and store.Player.HistoryExhausted for
// the underlying guarantees.
func coverageSufficient(coverage ingest.CoverageResult, opts ingest.SyncOptions) bool {
	if coverage.HistoryExhausted {
		return true
	}
	// A full-history request is never satisfied short of HistoryExhausted
	// - falling through to the MaxMatches check below would incorrectly
	// treat "cache already has >= the leftover default count" as
	// sufficient, even though most of the player's history is still
	// unfetched.
	if opts.All {
		return false
	}
	if opts.Since != nil {
		return coverage.Count > 0 && !coverage.Oldest.After(*opts.Since)
	}
	return coverage.Count >= opts.MaxMatches
}

// splitRiotID splits "Name#Tag" on the last '#'. Both parts must be
// non-empty.
func splitRiotID(s string) (name, tag string, ok bool) {
	i := strings.LastIndex(s, "#")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
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
