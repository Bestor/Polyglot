// Package ingest resolves players and syncs their match history from a
// data_sources.Source into the PocketBase cache, so repeated questions
// about the same matches never re-hit the upstream API.
package ingest

import (
	"context"
	"fmt"
	"time"

	"val-analyzer/internal/data_sources"
	"val-analyzer/internal/store"
)

// platform is currently always "pc" since that's the only platform the
// tracked players use; revisit if console support is ever needed.
const platform = "pc"

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
// only calling the data source's account lookup on a cache miss.
func (s *Service) ResolvePlayer(ctx context.Context, name, tag, region string) (store.Player, error) {
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
		Region: region,
	})
}

type SyncOptions struct {
	// MaxMatches bounds how many uncached matches are fetched in a single
	// call, so one request can never block for an unbounded amount of time
	// under the data source's rate limit.
	MaxMatches int
}

type SyncResult struct {
	Fetched int
	Skipped int
}

// SyncPlayerMatches fetches the player's match list, then fetches and
// caches full detail for every match not already stored, up to
// opts.MaxMatches. Season resolution is best-effort: if a match's season
// isn't cached yet, the match is still stored with season_id_raw set and
// no season relation.
func (s *Service) SyncPlayerMatches(ctx context.Context, player store.Player, region string, opts SyncOptions) (SyncResult, error) {
	entries, err := s.source.GetMatchList(ctx, region, platform, player.PUUID, opts.MaxMatches)
	if err != nil {
		return SyncResult{}, err
	}

	var result SyncResult
	var latest time.Time

	for _, entry := range entries {
		if entry.StartedAt.After(latest) {
			latest = entry.StartedAt
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
			continue
		}

		detail, err := s.source.GetMatch(ctx, region, entry.MatchID)
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

	if !latest.IsZero() {
		if err := s.players.UpdateLastSyncedMatchAt(player.ID, latest); err != nil {
			return result, err
		}
	}

	return result, nil
}

// SyncMoreByPUUID re-syncs additional matches for an already-resolved
// player identified by PUUID. This is the entry point an AI provider uses
// when it determines, from querying the cache, that it doesn't have enough
// matches for a question and needs more before it can answer. region is
// required since the data source's match endpoints are region-scoped and
// aren't recoverable from the PUUID alone.
func (s *Service) SyncMoreByPUUID(ctx context.Context, puuid, region string, opts SyncOptions) (SyncResult, error) {
	player, ok, err := s.players.FindByPUUID(puuid)
	if err != nil {
		return SyncResult{}, err
	}
	if !ok {
		return SyncResult{}, fmt.Errorf("unknown player puuid %q", puuid)
	}

	return s.SyncPlayerMatches(ctx, player, region, opts)
}
