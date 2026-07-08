// Package ingest resolves players and syncs their match history from a
// data_sources.Source into the PocketBase cache, so repeated questions
// about the same matches never re-hit the upstream API.
package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"val-analyzer/internal/data_sources"
	"val-analyzer/internal/store"
)

// platform is currently always "pc" since that's the only platform the
// tracked players use; revisit if console support is ever needed.
const platform = "pc"

// matchListPageSize is the per-request page size used when paging through
// a player's match history (mirrors henrik.maxMatchListSize; kept as a
// separate constant since ingest doesn't import the henrik package).
const matchListPageSize = 25

// maxSyncPages defensively bounds the backward-paging loop in
// SyncPlayerMatches, independent of opts.MaxMatches, so a data source that
// somehow never returns an empty page (or never reaches opts.Since) can't
// loop forever.
const maxSyncPages = 20

type Service struct {
	source  data_sources.Source
	players *store.PlayerStore
	matches *store.MatchStore
	seasons *store.SeasonStore
}

func NewService(source data_sources.Source, players *store.PlayerStore, matches *store.MatchStore, seasons *store.SeasonStore) *Service {
	return &Service{source: source, players: players, matches: matches, seasons: seasons}
}

// ResolvePlayer returns the cached player record for the given Riot ID,
// only calling the data source's account lookup on a cache miss. The
// player's region comes from the data source's own account lookup (the
// HenrikDev account endpoint reports it), not from the caller, since a
// Riot ID uniquely determines it.
func (s *Service) ResolvePlayer(ctx context.Context, name, tag string) (store.Player, error) {
	if cached, ok, err := s.players.FindByRiotID(name, tag); err != nil {
		return store.Player{}, err
	} else if ok {
		return cached, nil
	}

	account, err := s.source.GetAccountByRiotID(ctx, name, tag)
	if err != nil {
		return store.Player{}, err
	}

	if cached, ok, err := s.players.FindByPUUID(account.PUUID); err != nil {
		return store.Player{}, err
	} else if ok {
		return cached, nil
	}

	return s.players.Upsert(store.Player{
		PUUID:  account.PUUID,
		Name:   account.Name,
		Tag:    account.Tag,
		Region: account.Region,
	})
}

// CoverageResult reports what's already cached locally for a player within
// a requested window, so a caller can decide whether SyncPlayerMatches (and
// the upstream API calls it makes) is even necessary. See
// store.MatchStore.PlayerCoverage for the coverage guarantee/limitations.
type CoverageResult struct {
	Count  int
	Oldest time.Time
}

// CheckCoverage reports the local cache's coverage for player as of until
// (nil = as of now) without touching the upstream data source.
func (s *Service) CheckCoverage(player store.Player, until *time.Time) (CoverageResult, error) {
	count, oldest, err := s.matches.PlayerCoverage(player.ID, until)
	if err != nil {
		return CoverageResult{}, err
	}
	return CoverageResult{Count: count, Oldest: oldest}, nil
}

type SyncOptions struct {
	// MaxMatches bounds how many uncached matches are fetched in a single
	// call, so one request can never block for an unbounded amount of time
	// under the data source's rate limit. Always enforced, regardless of
	// whether Since/Until are set.
	MaxMatches int
	// Since and Until optionally bound the sync to matches started within
	// [Since, Until]. SyncPlayerMatches always pages backward through the
	// player's match history (the data source only exposes "most recent
	// N" plus an offset, not a real date filter) until MaxMatches is
	// reached, the upstream history is exhausted, or - only when Since is
	// set - the oldest match on a page predates Since. Until defaults to
	// "now" when Since is set.
	Since, Until *time.Time
}

type SyncResult struct {
	Fetched int
	Skipped int
	// OldestFetched is the StartedAt of the oldest match this call fetched
	// or observed already-cached within [Since, Until], zero if none.
	// Compare against Since to tell whether a date-range sync actually
	// covered the requested window or gave up early (MaxMatches/
	// maxSyncPages/upstream history exhausted).
	OldestFetched time.Time
}

// SyncPlayerMatches fetches the player's match list, then fetches and
// caches full detail for every match not already stored, up to
// opts.MaxMatches, optionally restricted to opts.Since/opts.Until. Season
// resolution is best-effort: if a match's season isn't cached yet, the
// match is still stored with season_id_raw set and no season relation.
func (s *Service) SyncPlayerMatches(ctx context.Context, player store.Player, opts SyncOptions) (SyncResult, error) {
	var result SyncResult
	var latestSynced time.Time
	offset := 0

	for page := 0; page < maxSyncPages; page++ {
		entries, err := s.source.GetMatchList(ctx, player.Region, platform, player.PUUID, matchListPageSize, offset)
		if err != nil {
			return result, err
		}
		slog.Debug("ingest: fetched match list page", "puuid", player.PUUID, "page", page, "offset", offset, "count", len(entries))
		if len(entries) == 0 {
			break
		}

		for _, entry := range entries {
			// last_synced_match_at tracks how fresh our view of this
			// player's matches is in general, independent of any
			// Since/Until window this particular call asked for.
			if entry.StartedAt.After(latestSynced) {
				latestSynced = entry.StartedAt
			}

			if opts.Until != nil && entry.StartedAt.After(*opts.Until) {
				continue
			}
			if opts.Since != nil && entry.StartedAt.Before(*opts.Since) {
				continue
			}
			if result.OldestFetched.IsZero() || entry.StartedAt.Before(result.OldestFetched) {
				result.OldestFetched = entry.StartedAt
			}

			exists, err := s.matches.Exists(entry.MatchID)
			if err != nil {
				return result, err
			}
			if exists {
				result.Skipped++
				continue
			}
			if result.Fetched >= opts.MaxMatches {
				return result, nil
			}

			detail, err := s.source.GetMatch(ctx, player.Region, entry.MatchID)
			if err != nil {
				return result, err
			}

			var seasonRecordID string
			if detail.SeasonID != "" {
				if season, ok, err := s.seasons.FindBySeasonID(detail.SeasonID); err != nil {
					return result, err
				} else if ok {
					seasonRecordID = season.ID
				}
			}

			if err := s.matches.SaveMatch(detail, seasonRecordID); err != nil {
				return result, err
			}
			result.Fetched++
		}

		offset += len(entries)
		if opts.Since != nil && entries[len(entries)-1].StartedAt.Before(*opts.Since) {
			break
		}
	}

	if !latestSynced.IsZero() {
		if err := s.players.UpdateLastSyncedMatchAt(player.ID, latestSynced); err != nil {
			return result, err
		}
	}

	return result, nil
}

// SyncMoreByPUUID re-syncs additional matches for an already-resolved
// player identified by PUUID. This is the entry point an AI provider uses
// when it determines, from querying the cache, that it doesn't have enough
// matches for a question and needs more before it can answer.
func (s *Service) SyncMoreByPUUID(ctx context.Context, puuid string, opts SyncOptions) (SyncResult, error) {
	player, ok, err := s.players.FindByPUUID(puuid)
	if err != nil {
		return SyncResult{}, err
	}
	if !ok {
		return SyncResult{}, fmt.Errorf("unknown player puuid %q", puuid)
	}

	return s.SyncPlayerMatches(ctx, player, opts)
}
