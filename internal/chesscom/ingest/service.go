// Package ingest resolves players and syncs their game history from a
// data_sources.Source into the PocketBase cache, so repeated questions
// about the same games never re-hit chess.com's API. Mirrors
// internal/valorant/ingest's own role, but the sync algorithm itself is
// genuinely simpler: chess.com's archive list is complete and finite (see
// SyncGames' own doc comment), unlike Valorant's "most recent N + offset"
// match list, which needs backward paging and a HistoryExhausted flag
// this package has no equivalent of at all.
package ingest

import (
	"context"
	"fmt"
	"time"

	"val-analyzer/internal/chesscom/data_sources"
	"val-analyzer/internal/chesscom/pgn"
	"val-analyzer/internal/chesscom/store"
)

type Service struct {
	source  data_sources.Source
	players *store.PlayerStore
	ratings *store.RatingStore
	months  *store.MonthStore
	games   *store.GameStore
}

func NewService(source data_sources.Source, players *store.PlayerStore, ratings *store.RatingStore, months *store.MonthStore, games *store.GameStore) *Service {
	return &Service{source: source, players: players, ratings: ratings, months: months, games: games}
}

// ResolvePlayer returns the cached player record for username, only
// calling GetPlayerProfile when there's no cached row yet, or the cached
// row is still just an opportunistic stub (PlayerID == 0 - see
// store.PlayerStore.UpsertStub, created when this username was previously
// seen only as a game's opponent, never resolved directly).
func (s *Service) ResolvePlayer(ctx context.Context, username string) (store.Player, error) {
	if cached, ok, err := s.players.FindByUsername(username); err != nil {
		return store.Player{}, err
	} else if ok && cached.PlayerID != 0 {
		return cached, nil
	}

	profile, err := s.source.GetPlayerProfile(ctx, username)
	if err != nil {
		return store.Player{}, err
	}
	return s.players.UpsertProfile(profileToPlayer(profile))
}

func profileToPlayer(p data_sources.Profile) store.Player {
	return store.Player{
		Username:    p.Username,
		PlayerID:    p.PlayerID,
		Name:        p.Name,
		Title:       p.Title,
		CountryCode: p.CountryCode,
		Status:      p.Status,
		AvatarURL:   p.AvatarURL,
		Followers:   p.Followers,
		JoinedAt:    p.JoinedAt,
	}
}

// SyncStatsResult reports how many rating categories were refreshed.
type SyncStatsResult struct {
	CategoriesSynced int
}

// SyncStats fetches the player's full profile and rating stats from
// upstream and does a full, unconditional refresh - mirrors Valorant's
// sync_seasons' own "always refresh, no coverage logic" idiom, since
// chess.com's stats endpoint always reports each category's complete
// current standing in one call; there's nothing to conditionally skip.
func (s *Service) SyncStats(ctx context.Context, username string) (SyncStatsResult, error) {
	profile, err := s.source.GetPlayerProfile(ctx, username)
	if err != nil {
		return SyncStatsResult{}, err
	}
	player, err := s.players.UpsertProfile(profileToPlayer(profile))
	if err != nil {
		return SyncStatsResult{}, err
	}

	snapshots, err := s.source.GetPlayerStats(ctx, username)
	if err != nil {
		return SyncStatsResult{}, err
	}
	for _, snap := range snapshots {
		rating := store.Rating{
			Category:   snap.Category,
			LastRating: snap.LastRating, LastRD: snap.LastRD, LastDate: snap.LastDate,
			BestRating: snap.BestRating, BestDate: snap.BestDate, BestGameURL: snap.BestGameURL,
			Wins: snap.Wins, Losses: snap.Losses, Draws: snap.Draws, Attempts: snap.Attempts,
		}
		if err := s.ratings.Upsert(player.ID, rating); err != nil {
			return SyncStatsResult{}, err
		}
	}

	return SyncStatsResult{CategoriesSynced: len(snapshots)}, nil
}

// SyncOptions controls one SyncGames call.
type SyncOptions struct {
	// MaxMonths bounds how many *new* (non-current) months are fetched in
	// a single call, so one request can never block for an unbounded
	// amount of time. Always enforced regardless of Since/Until - ignored
	// entirely when All is true. The still-open current month is always
	// fetched regardless of this cap, since it can gain games at any time
	// and doesn't count as a "new" month in this sense.
	MaxMonths int
	// Since/Until optionally bound the sync to months within [Since,
	// Until], compared at month granularity - day-of-month is ignored.
	// Until nil means no upper bound (equivalent to "now", since there
	// are never any months newer than the current one in the archive
	// list anyway).
	Since, Until *time.Time
	// All, when true, ignores MaxMonths and fetches every not-yet-synced
	// month, however many there are - intended for an explicit,
	// deliberate cache-warming call, not the AI's per-question sync.
	All bool
}

// SyncGamesResult reports what one SyncGames call actually did.
type SyncGamesResult struct {
	GamesFetched  int
	MonthsFetched int
}

// SyncGames walks the player's archive list newest-first (chess.com
// returns it oldest-first) and fetches any month not already fully
// synced, always re-fetching the still-open current month regardless of
// its prior sync state.
//
// This is genuinely simpler than Valorant's SyncPlayerMatches, not just
// superficially different:
//   - No HistoryExhausted flag: Valorant needs one because its upstream
//     never reports a total upfront - an empty page is the only signal
//     you've reached the start. GetArchiveList returns every month in one
//     call, so "is this player fully synced" is always freshly computable
//     without persisting a boolean.
//   - The reconnect-stop (breaking out of the walk on the first
//     already-synced month, on a plain call with no Since bound) is
//     airtight here, not best-effort like Valorant's own equivalent -
//     each month fetch is a *complete* listing, and immutable once the
//     month closes, so a player_months row for a non-current month is
//     provably complete forever. A Since-bounded walk must NOT stop at
//     the first synced month it meets, though - an already-synced recent
//     month says nothing about whether an *older* requested month was
//     ever synced, so it only skips (continue) a synced month, walking on
//     until it passes below the Since bound entirely.
//   - Dedup is pure skip-and-never-update: a finished game's facts never
//     change, and an in-progress game never appears in an archive until
//     it ends.
func (s *Service) SyncGames(ctx context.Context, player store.Player, opts SyncOptions) (SyncGamesResult, error) {
	var result SyncGamesResult

	archives, err := s.source.GetArchiveList(ctx, player.Username)
	if err != nil {
		return result, err
	}
	if len(archives) == 0 {
		return result, nil
	}
	currentIdx := len(archives) - 1

	fetchedMonths := 0
	for i := currentIdx; i >= 0; i-- {
		a := archives[i]
		isCurrent := i == currentIdx
		key := monthKey(a.Year, a.Month)

		if opts.Until != nil && key > monthKeyForTime(*opts.Until) {
			continue // newer than the requested window's upper bound
		}
		if opts.Since != nil && key < monthKeyForTime(*opts.Since) {
			break // walking newest->oldest: every remaining month is also older than Since
		}

		if !isCurrent {
			synced, err := s.months.IsSynced(player.ID, a.Year, a.Month)
			if err != nil {
				return result, err
			}
			if synced {
				if opts.Since == nil && !opts.All {
					break // plain walk: every older month was already covered by a prior sync
				}
				continue // Since-bounded or All: this month is done, keep walking for others
			}
			if !opts.All && fetchedMonths >= opts.MaxMonths {
				break
			}
		}

		games, err := s.source.GetMonthGames(ctx, player.Username, a.Year, a.Month)
		if err != nil {
			return result, err
		}

		for _, g := range games {
			exists, err := s.games.Exists(g.UUID)
			if err != nil {
				return result, err
			}
			if exists {
				continue
			}

			moves, err := pgn.Parse(g.PGN)
			if err != nil {
				return result, fmt.Errorf("parsing pgn for game %s: %w", g.UUID, err)
			}
			if err := s.games.SaveGame(g, moves); err != nil {
				return result, err
			}
			result.GamesFetched++
		}

		if err := s.months.UpsertSynced(player.ID, a.Year, a.Month, len(games)); err != nil {
			return result, err
		}
		if !isCurrent {
			fetchedMonths++
		}
	}
	result.MonthsFetched = fetchedMonths

	if err := s.players.UpdateLastSyncedGamesAt(player.ID, time.Now()); err != nil {
		return result, err
	}

	return result, nil
}

// CoverageResult reports whether the local cache already satisfies a
// requested sync, so CheckGamesCoverage's caller can skip SyncGames (and
// the upstream calls it makes) entirely.
type CoverageResult struct {
	Covered bool
}

// CheckGamesCoverage reports coverage for opts without touching the
// upstream data source at all.
//
// A Since-bounded request is airtight, unlike Valorant's own best-effort
// coverage check (see store.MatchStore.PlayerCoverage's own doc comment on
// why a sync cut short there could in principle leave a gap): chess.com's
// archive months are immutable once closed, so a player_months row for a
// non-current month is provably complete forever. The one thing that can
// never be proven covered from cache alone is the still-open current
// month - it can always have gained a new game since the last check.
//
// A plain (no Since) or All request is never sufficient from cache alone
// either, for the same honest reason Valorant's own coverageSufficient
// reaches for its plain case: only a fresh call can reveal whether a new
// month/game has appeared since the last check.
func (s *Service) CheckGamesCoverage(player store.Player, opts SyncOptions) (CoverageResult, error) {
	if opts.All || opts.Since == nil {
		return CoverageResult{}, nil
	}

	until := time.Now()
	if opts.Until != nil {
		until = *opts.Until
	}
	currentKey := monthKeyForTime(time.Now())

	for key := monthKeyForTime(*opts.Since); key <= monthKeyForTime(until); key++ {
		if key == currentKey {
			return CoverageResult{}, nil
		}
		year, month := yearMonthFromKey(key)
		synced, err := s.months.IsSynced(player.ID, year, month)
		if err != nil {
			return CoverageResult{}, err
		}
		if !synced {
			return CoverageResult{}, nil
		}
	}

	return CoverageResult{Covered: true}, nil
}

// ReparseMovesResult reports how many games' moves were rewritten.
type ReparseMovesResult struct {
	GamesReparsed int
	Failed        int
}

// ReparseMoves re-parses already-stored PGN text for every game username
// played (or every stored game, if username is empty) and rewrites that
// game's moves - zero upstream calls, a direct parallel to Valorant's
// BackfillMatchSeasons repair-without-re-hitting-upstream idiom, enabled
// by storing raw PGN on every game row. A game whose PGN fails to parse is
// counted in Failed and left with its previous moves, rather than
// aborting the whole call.
func (s *Service) ReparseMoves(username string) (ReparseMovesResult, error) {
	var games []store.StoredGame
	var err error
	if username == "" {
		games, err = s.games.AllGames()
	} else {
		games, err = s.games.GamesForUsername(username)
	}
	if err != nil {
		return ReparseMovesResult{}, err
	}

	var result ReparseMovesResult
	for _, g := range games {
		moves, err := pgn.Parse(g.PGN)
		if err != nil {
			result.Failed++
			continue
		}
		if err := s.games.ReplaceMoves(g.ID, moves); err != nil {
			return result, err
		}
		result.GamesReparsed++
	}
	return result, nil
}

// monthKey packs (year, month) into a single comparable integer, so month
// comparisons are plain integer arithmetic.
func monthKey(year, month int) int { return year*12 + month }

func monthKeyForTime(t time.Time) int { return monthKey(t.Year(), int(t.Month())) }

func yearMonthFromKey(key int) (year, month int) {
	key--
	return key / 12, (key % 12) + 1
}
