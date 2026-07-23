package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"val-analyzer/internal/valorant/data_sources"
	_ "val-analyzer/internal/valorant/migrations"
	"val-analyzer/internal/valorant/store"
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

// fakeSource scripts GetMatchList as a fixed sequence of pages (ignoring
// the offset/size SyncPlayerMatches actually passes - tests only need
// deterministic, ordered responses, not real pagination-offset math) and
// GetMatch by match ID, so tests can assert exactly how many upstream
// calls a sync made.
type fakeSource struct {
	pages          [][]data_sources.MatchListEntry
	details        map[string]data_sources.MatchDetail
	matchListCalls int
	getMatchCalls  []string
}

func (f *fakeSource) GetAccountByRiotID(ctx context.Context, name, tag string) (data_sources.Account, error) {
	return data_sources.Account{}, nil
}

func (f *fakeSource) GetMatchList(ctx context.Context, region, platform, puuid string, size, offset int) ([]data_sources.MatchListEntry, error) {
	if f.matchListCalls >= len(f.pages) {
		f.matchListCalls++
		return nil, nil
	}
	page := f.pages[f.matchListCalls]
	f.matchListCalls++
	return page, nil
}

func (f *fakeSource) GetMatch(ctx context.Context, region, matchID string) (data_sources.MatchDetail, error) {
	f.getMatchCalls = append(f.getMatchCalls, matchID)
	return f.details[matchID], nil
}

func (f *fakeSource) GetSeasons(ctx context.Context) ([]data_sources.Season, error) {
	return nil, nil
}

var _ data_sources.Source = (*fakeSource)(nil)

func newDetail(matchID string, startedAt time.Time) data_sources.MatchDetail {
	return data_sources.MatchDetail{MatchID: matchID, Map: "Ascent", MapID: "map-ascent", Region: "na", StartedAt: startedAt}
}

// seedCachedMatch saves a match directly, standing in for a match a prior
// sync already cached.
func seedCachedMatch(t *testing.T, matches *store.MatchStore, matchID string, startedAt time.Time) {
	t.Helper()
	if err := matches.SaveMatch(newDetail(matchID, startedAt), ""); err != nil {
		t.Fatalf("seeding cached match %q: %v", matchID, err)
	}
}

func newTestPlayer(t *testing.T, players *store.PlayerStore, historyExhausted bool) store.Player {
	t.Helper()
	p, err := players.Upsert(store.Player{PUUID: "puuid-1", Name: "OrBest", Tag: "NA1", Region: "na"})
	if err != nil {
		t.Fatalf("upserting test player: %v", err)
	}
	if historyExhausted {
		if err := players.MarkHistoryExhausted(p.ID); err != nil {
			t.Fatalf("marking history exhausted: %v", err)
		}
		p.HistoryExhausted = true
	}
	return p
}

// TestSyncPlayerMatches_ReconnectEarlyStop is the direct regression test
// for the production bug: a player whose history was fully walked once
// before (HistoryExhausted=true, mirroring OrBest#NA1's real cached state)
// must still have new matches picked up by a plain sync_matches call, and
// the walk must stop as soon as it reconnects with an already-cached
// match rather than always paging out to MaxMatches/maxSyncPages.
func TestSyncPlayerMatches_ReconnectEarlyStop(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	matches := store.NewMatchStore(app)
	seasons := store.NewSeasonStore(app)

	player := newTestPlayer(t, players, true)

	now := time.Now().UTC().Truncate(time.Second)
	oldMatch := "old-1"
	seedCachedMatch(t, matches, oldMatch, now.Add(-72*time.Hour))

	newest := now
	newer := now.Add(-1 * time.Hour)
	src := &fakeSource{
		pages: [][]data_sources.MatchListEntry{
			{
				{MatchID: "new-1", StartedAt: newest},
				{MatchID: "new-2", StartedAt: newer},
				{MatchID: oldMatch, StartedAt: now.Add(-72 * time.Hour)},
			},
			// Should never be reached: the early stop must fire on
			// oldMatch, in the first (and only) page.
			{
				{MatchID: "should-not-be-fetched", StartedAt: now.Add(-96 * time.Hour)},
			},
		},
		details: map[string]data_sources.MatchDetail{
			"new-1": newDetail("new-1", newest),
			"new-2": newDetail("new-2", newer),
		},
	}

	svc := NewService(src, players, matches, seasons)
	result, err := svc.SyncPlayerMatches(context.Background(), player, SyncOptions{MaxMatches: 50})
	if err != nil {
		t.Fatalf("SyncPlayerMatches: %v", err)
	}

	if result.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2 (new-1, new-2)", result.Fetched)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (oldMatch)", result.Skipped)
	}
	if src.matchListCalls != 1 {
		t.Errorf("GetMatchList called %d times, want 1 - the reconnect early stop should have prevented a second page fetch", src.matchListCalls)
	}
	if len(src.getMatchCalls) != 2 {
		t.Errorf("GetMatch called for %v, want exactly [new-1 new-2]", src.getMatchCalls)
	}

	updated, ok, err := players.FindByPUUID(player.PUUID)
	if err != nil || !ok {
		t.Fatalf("FindByPUUID after sync: ok=%v err=%v", ok, err)
	}
	if !updated.LastSyncedMatchAt.Equal(newest) {
		t.Errorf("LastSyncedMatchAt = %v, want %v", updated.LastSyncedMatchAt, newest)
	}
}

// TestSyncPlayerMatches_AllModeIgnoresReconnectStop confirms a full-history
// walk (opts.All) is not cut short by the reconnect optimization - it must
// keep paging past an already-cached match to reach genuinely new-to-us
// (older) matches and, eventually, the true end of history.
func TestSyncPlayerMatches_AllModeIgnoresReconnectStop(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	matches := store.NewMatchStore(app)
	seasons := store.NewSeasonStore(app)

	player := newTestPlayer(t, players, false)

	now := time.Now().UTC().Truncate(time.Second)
	cachedMatch := "cached-1"
	seedCachedMatch(t, matches, cachedMatch, now.Add(-48*time.Hour))

	src := &fakeSource{
		pages: [][]data_sources.MatchListEntry{
			{
				{MatchID: "new-1", StartedAt: now},
				{MatchID: cachedMatch, StartedAt: now.Add(-48 * time.Hour)},
			},
			{
				{MatchID: "new-2", StartedAt: now.Add(-96 * time.Hour)},
			},
			{}, // empty page: true end of history
		},
		details: map[string]data_sources.MatchDetail{
			"new-1": newDetail("new-1", now),
			"new-2": newDetail("new-2", now.Add(-96*time.Hour)),
		},
	}

	svc := NewService(src, players, matches, seasons)
	result, err := svc.SyncPlayerMatches(context.Background(), player, SyncOptions{All: true})
	if err != nil {
		t.Fatalf("SyncPlayerMatches: %v", err)
	}

	if result.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2 (new-1, new-2)", result.Fetched)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (cachedMatch)", result.Skipped)
	}
	if !result.HistoryExhausted {
		t.Error("HistoryExhausted = false, want true after an empty page")
	}
	if src.matchListCalls != 3 {
		t.Errorf("GetMatchList called %d times, want 3 (All mode must not stop early on reconnect)", src.matchListCalls)
	}
}

// TestSyncPlayerMatches_SinceModeIgnoresReconnectStop confirms a
// date-bounded walk keeps paging past an already-cached match that's still
// within the requested window, stopping only via its own oldest-vs-Since
// check.
func TestSyncPlayerMatches_SinceModeIgnoresReconnectStop(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	matches := store.NewMatchStore(app)
	seasons := store.NewSeasonStore(app)

	player := newTestPlayer(t, players, false)

	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-36 * time.Hour)
	cachedMatch := "cached-1"
	seedCachedMatch(t, matches, cachedMatch, now.Add(-24*time.Hour))

	src := &fakeSource{
		pages: [][]data_sources.MatchListEntry{
			{
				{MatchID: "new-1", StartedAt: now},
				{MatchID: cachedMatch, StartedAt: now.Add(-24 * time.Hour)},
			},
			{
				// Beyond Since - triggers the existing oldest-vs-Since break.
				{MatchID: "too-old", StartedAt: now.Add(-48 * time.Hour)},
			},
		},
		details: map[string]data_sources.MatchDetail{
			"new-1": newDetail("new-1", now),
		},
	}

	svc := NewService(src, players, matches, seasons)
	result, err := svc.SyncPlayerMatches(context.Background(), player, SyncOptions{MaxMatches: 50, Since: &since})
	if err != nil {
		t.Fatalf("SyncPlayerMatches: %v", err)
	}

	if result.Fetched != 1 {
		t.Errorf("Fetched = %d, want 1 (new-1)", result.Fetched)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (cachedMatch)", result.Skipped)
	}
	if src.matchListCalls != 2 {
		t.Errorf("GetMatchList called %d times, want 2 (Since mode must not stop early on reconnect)", src.matchListCalls)
	}
}
