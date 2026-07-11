package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/ingest"
)

// maxMatchesPerRequest bounds how many matches a single table update will
// fetch/save in one call, so a request can never block unboundedly under
// the data source's rate limit - regardless of whether it's a plain
// "most recent N" sync or a date-range sync that pages backward through
// history.
const maxMatchesPerRequest = 100

// defaultMatchCount is used when a matches update doesn't specify count.
const defaultMatchCount = 50

func handleAsk(ing *ingest.Service, schema []ai.TableDescription, query ai.QueryFunc, provider ai.Provider) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req AskRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if strings.TrimSpace(req.Question) == "" {
			return e.BadRequestError("question is required", nil)
		}

		ctx := e.Request.Context()
		matchesSynced := 0
		updaters := []ai.TableUpdater{playersUpdater(ing), matchesUpdater(ing, &matchesSynced)}
		tables := ai.BuildTableSpecs(schema, updaters)

		resp, err := provider.Answer(ctx, ai.Request{
			Question: req.Question,
			Tables:   tables,
			Query:    query,
		})
		if err != nil {
			slog.Error("api: provider failed to answer question", "question", req.Question, "error", err)
			return e.InternalServerError("failed to answer question", err)
		}

		return e.JSON(http.StatusOK, AskResponse{
			Answer:        resp.Answer,
			MatchesSynced: matchesSynced,
		})
	}
}

// playersUpdater lets the AI resolve a Riot ID into a cached player
// identity without necessarily syncing any match history.
func playersUpdater(ing *ingest.Service) ai.TableUpdater {
	return ai.TableUpdater{
		Table:       "players",
		Description: "Resolve a Riot ID (name#tag) into a cached player identity. Does not fetch match history - use the matches update for that.",
		Args: []ai.UpdateArg{
			{Name: "name", Type: "string", Description: "Riot ID name, before the #.", Required: true},
			{Name: "tag", Type: "string", Description: "Riot ID tag, after the #.", Required: true},
		},
		Run: func(ctx context.Context, args map[string]any) (ai.UpdateOutcome, error) {
			name, _ := args["name"].(string)
			tag, _ := args["tag"].(string)
			if name == "" || tag == "" {
				return ai.UpdateOutcome{}, fmt.Errorf("players update requires non-empty name and tag")
			}

			player, err := ing.ResolvePlayer(ctx, name, tag)
			if err != nil {
				slog.Error("api: players update failed", "name", name, "tag", tag, "error", err)
				return ai.UpdateOutcome{}, fmt.Errorf("failed to resolve %s#%s: %w", name, tag, err)
			}

			return ai.UpdateOutcome{
				Summary: fmt.Sprintf("resolved %s#%s to puuid %s (region %s)", player.Name, player.Tag, player.PUUID, player.Region),
			}, nil
		},
	}
}

// matchesUpdater lets the AI sync a player's match history, either by a
// plain recency count or by a date range - see ingest.SyncOptions.
func matchesUpdater(ing *ingest.Service, matchesSynced *int) ai.TableUpdater {
	return ai.TableUpdater{
		Table: "matches",
		Description: "Fetch and cache a player's matches from the upstream Valorant API, populating matches and all related per-match tables " +
			"(match_teams, match_players, rounds, round_player_stats, damage_events, kills, kill_assists, event_player_locations). " +
			"The upstream API only exposes \"most recent N matches\" plus an offset - there is no native date filter - so when a date range " +
			"is given this pages backward through history until it's covered, bounded by the count safety cap.",
		Args: []ai.UpdateArg{
			{Name: "player_tag", Type: "string", Description: "The player's Riot ID as name#tag, e.g. \"Orbest#NA1\".", Required: true},
			{Name: "start_date", Type: "string", Description: "ISO-8601 date (e.g. \"2026-05-01\"), the earliest match start date to ensure is cached. Omit for a plain most-recent-matches sync.", Required: false},
			{Name: "end_date", Type: "string", Description: "ISO-8601 date, the latest match start date to ensure is cached. Defaults to now if start_date is given.", Required: false},
			{Name: "count", Type: "integer", Description: fmt.Sprintf("Safety cap on how many matches to fetch this call, up to %d. Defaults to %d.", maxMatchesPerRequest, defaultMatchCount), Required: false},
		},
		Run: func(ctx context.Context, args map[string]any) (ai.UpdateOutcome, error) {
			playerTag, _ := args["player_tag"].(string)
			name, tag, ok := splitRiotID(playerTag)
			if !ok {
				return ai.UpdateOutcome{}, fmt.Errorf("matches update requires player_tag in the form name#tag, got %q", playerTag)
			}

			opts := ingest.SyncOptions{MaxMatches: defaultMatchCount}
			if c, ok := args["count"].(float64); ok && c > 0 {
				opts.MaxMatches = int(c)
			}
			if opts.MaxMatches > maxMatchesPerRequest {
				opts.MaxMatches = maxMatchesPerRequest
			}

			if sd, ok := args["start_date"].(string); ok && sd != "" {
				since, err := parseFlexibleDate(sd)
				if err != nil {
					return ai.UpdateOutcome{}, fmt.Errorf("invalid start_date %q: %w", sd, err)
				}
				opts.Since = &since
			}
			if ed, ok := args["end_date"].(string); ok && ed != "" {
				until, err := parseFlexibleDate(ed)
				if err != nil {
					return ai.UpdateOutcome{}, fmt.Errorf("invalid end_date %q: %w", ed, err)
				}
				opts.Until = &until
			}

			player, err := ing.ResolvePlayer(ctx, name, tag)
			if err != nil {
				slog.Error("api: matches update failed to resolve player", "name", name, "tag", tag, "error", err)
				return ai.UpdateOutcome{}, fmt.Errorf("failed to resolve %s#%s: %w", name, tag, err)
			}

			coverage, err := ing.CheckCoverage(player, opts.Until)
			if err != nil {
				slog.Error("api: matches update failed to check cache coverage", "name", name, "tag", tag, "error", err)
				return ai.UpdateOutcome{}, fmt.Errorf("failed to check cache coverage for %s#%s: %w", name, tag, err)
			}
			if coverageSufficient(coverage, opts) {
				slog.Info("api: matches update skipped upstream sync, cache already covers request",
					"name", name, "tag", tag, "puuid", player.PUUID, "cached_count", coverage.Count)
				return ai.UpdateOutcome{
					Summary: fmt.Sprintf("cache already covers this request for %s#%s (puuid %s) - %d matches cached, no upstream sync needed",
						player.Name, player.Tag, player.PUUID, coverage.Count),
				}, nil
			}

			result, err := ing.SyncPlayerMatches(ctx, player, opts)
			if err != nil {
				slog.Error("api: matches update failed to sync", "name", name, "tag", tag, "error", err)
				return ai.UpdateOutcome{}, fmt.Errorf("failed to sync matches for %s#%s: %w", name, tag, err)
			}
			*matchesSynced += result.Fetched

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

			return ai.UpdateOutcome{Summary: summary}, nil
		},
	}
}

// coverageSufficient decides whether the local cache already satisfies a
// matches update request, so it can be skipped without calling the
// upstream API at all. If HistoryExhausted is set (e.g. from an earlier
// /api/warm all=true call), the player's entire match history is already
// cached, so any request - however far back - is trivially satisfied.
// Otherwise, in date-range mode, sufficient means the cache already has a
// match at or before the requested start date (see
// store.MatchStore.PlayerCoverage for what that guarantees). In
// count-only mode, sufficient means at least as many matches are already
// cached as requested - this doesn't guarantee freshness (new matches may
// have been played since the last sync), but re-checking that on every
// question would defeat the point of caching.
func coverageSufficient(coverage ingest.CoverageResult, opts ingest.SyncOptions) bool {
	if coverage.HistoryExhausted {
		return true
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
// YYYY-MM-DD date, since the model may give either.
func parseFlexibleDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected an RFC3339 timestamp or YYYY-MM-DD date")
}
