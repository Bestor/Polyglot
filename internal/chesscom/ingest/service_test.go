package ingest

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"val-analyzer/internal/chesscom/data_sources"
	_ "val-analyzer/internal/chesscom/migrations"
	"val-analyzer/internal/chesscom/store"
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

// fakeSource scripts GetArchiveList as a fixed list and GetMonthGames by
// (year, month), so tests can assert exactly how many upstream calls a
// sync made - mirrors internal/valorant/ingest/service_test.go's own
// fakeSource pattern.
type fakeSource struct {
	archives        []data_sources.GameArchive
	gamesByMonth    map[string][]data_sources.Game
	monthGamesCalls []string
}

func monthCallKey(year, month int) string {
	return fmt.Sprintf("%04d-%02d", year, month)
}

func (f *fakeSource) GetPlayerProfile(ctx context.Context, username string) (data_sources.Profile, error) {
	return data_sources.Profile{Username: username, PlayerID: 1}, nil
}

func (f *fakeSource) GetPlayerStats(ctx context.Context, username string) ([]data_sources.RatingSnapshot, error) {
	return nil, nil
}

func (f *fakeSource) GetArchiveList(ctx context.Context, username string) ([]data_sources.GameArchive, error) {
	return f.archives, nil
}

func (f *fakeSource) GetMonthGames(ctx context.Context, username string, year, month int) ([]data_sources.Game, error) {
	key := monthCallKey(year, month)
	f.monthGamesCalls = append(f.monthGamesCalls, key)
	return f.gamesByMonth[key], nil
}

var _ data_sources.Source = (*fakeSource)(nil)

func newTestGame(uuid, whiteUsername, blackUsername string) data_sources.Game {
	return data_sources.Game{
		UUID:      uuid,
		PGN:       "[Event \"Test\"]\n\n1. e4 e5 1-0\n",
		TimeClass: "blitz",
		Rules:     "chess",
		White:     data_sources.GamePlayer{Username: whiteUsername, Rating: 1500, Result: "win"},
		Black:     data_sources.GamePlayer{Username: blackUsername, Rating: 1490, Result: "checkmated"},
	}
}

func newTestPlayer(t *testing.T, players *store.PlayerStore, username string) store.Player {
	t.Helper()
	p, err := players.UpsertProfile(store.Player{Username: username, PlayerID: 1})
	if err != nil {
		t.Fatalf("upserting test player: %v", err)
	}
	return p
}

// TestSyncGames_ReconnectStop confirms a plain (no Since, no All) walk
// stops as soon as it reconnects with an already-synced month, never
// calling GetMonthGames for it - the airtight version of Valorant's own
// reconnect-early-stop, safe here because a synced non-current month is
// provably complete forever.
func TestSyncGames_ReconnectStop(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	ratings := store.NewRatingStore(app)
	months := store.NewMonthStore(app)
	games := store.NewGameStore(app)

	player := newTestPlayer(t, players, "tester")
	if err := months.UpsertSynced(player.ID, 2024, 1, 5); err != nil {
		t.Fatalf("seeding synced month: %v", err)
	}

	src := &fakeSource{
		archives: []data_sources.GameArchive{
			{Year: 2024, Month: 1}, // already synced, older
			{Year: 2024, Month: 2}, // current (newest in the list)
		},
		gamesByMonth: map[string][]data_sources.Game{
			monthCallKey(2024, 2): {newTestGame("game-1", "tester", "opponent")},
		},
	}

	svc := NewService(src, players, ratings, months, games)
	result, err := svc.SyncGames(context.Background(), player, SyncOptions{MaxMonths: 24})
	if err != nil {
		t.Fatalf("SyncGames: %v", err)
	}

	if result.GamesFetched != 1 {
		t.Errorf("GamesFetched = %d, want 1", result.GamesFetched)
	}
	want := []string{monthCallKey(2024, 2)}
	if !reflect.DeepEqual(src.monthGamesCalls, want) {
		t.Errorf("GetMonthGames calls = %v, want %v (reconnect stop must skip 2024/01)", src.monthGamesCalls, want)
	}
}

// TestSyncGames_AllModeIgnoresReconnectStop confirms All mode keeps
// walking past an already-synced month to reach an older, not-yet-synced
// one - it only skips (never re-fetches) the synced month itself.
func TestSyncGames_AllModeIgnoresReconnectStop(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	ratings := store.NewRatingStore(app)
	months := store.NewMonthStore(app)
	games := store.NewGameStore(app)

	player := newTestPlayer(t, players, "tester")
	if err := months.UpsertSynced(player.ID, 2024, 2, 3); err != nil {
		t.Fatalf("seeding synced month: %v", err)
	}

	src := &fakeSource{
		archives: []data_sources.GameArchive{
			{Year: 2024, Month: 1}, // not yet synced, oldest
			{Year: 2024, Month: 2}, // already synced, middle
			{Year: 2024, Month: 3}, // current
		},
		gamesByMonth: map[string][]data_sources.Game{
			monthCallKey(2024, 1): {newTestGame("old-game", "tester", "opp1")},
			monthCallKey(2024, 3): {newTestGame("current-game", "tester", "opp2")},
		},
	}

	svc := NewService(src, players, ratings, months, games)
	result, err := svc.SyncGames(context.Background(), player, SyncOptions{All: true})
	if err != nil {
		t.Fatalf("SyncGames: %v", err)
	}

	if result.GamesFetched != 2 {
		t.Errorf("GamesFetched = %d, want 2", result.GamesFetched)
	}
	want := []string{monthCallKey(2024, 3), monthCallKey(2024, 1)}
	if !reflect.DeepEqual(src.monthGamesCalls, want) {
		t.Errorf("GetMonthGames calls = %v, want %v (All must skip only the synced middle month, not stop there)", src.monthGamesCalls, want)
	}
}

// TestSyncGames_SinceModeIgnoresReconnectStop confirms a Since-bounded walk
// also keeps going past an already-synced month within the requested
// range, for the same reason All mode does.
func TestSyncGames_SinceModeIgnoresReconnectStop(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	ratings := store.NewRatingStore(app)
	months := store.NewMonthStore(app)
	games := store.NewGameStore(app)

	player := newTestPlayer(t, players, "tester")
	if err := months.UpsertSynced(player.ID, 2024, 2, 3); err != nil {
		t.Fatalf("seeding synced month: %v", err)
	}

	src := &fakeSource{
		archives: []data_sources.GameArchive{
			{Year: 2024, Month: 1},
			{Year: 2024, Month: 2},
			{Year: 2024, Month: 3}, // current
		},
		gamesByMonth: map[string][]data_sources.Game{
			monthCallKey(2024, 1): {newTestGame("old-game", "tester", "opp1")},
			monthCallKey(2024, 3): {newTestGame("current-game", "tester", "opp2")},
		},
	}

	since := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := NewService(src, players, ratings, months, games)
	result, err := svc.SyncGames(context.Background(), player, SyncOptions{MaxMonths: 24, Since: &since})
	if err != nil {
		t.Fatalf("SyncGames: %v", err)
	}

	if result.GamesFetched != 2 {
		t.Errorf("GamesFetched = %d, want 2", result.GamesFetched)
	}
	want := []string{monthCallKey(2024, 3), monthCallKey(2024, 1)}
	if !reflect.DeepEqual(src.monthGamesCalls, want) {
		t.Errorf("GetMonthGames calls = %v, want %v (Since must skip only the synced middle month, not stop there)", src.monthGamesCalls, want)
	}
}

// TestSyncGames_CurrentMonthAlwaysRefetchedAndDeduped confirms the current
// month is re-fetched even though it was already marked synced by a prior
// call, and that a game already stored (by uuid) within it is skipped
// rather than double-counted.
func TestSyncGames_CurrentMonthAlwaysRefetchedAndDeduped(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	ratings := store.NewRatingStore(app)
	months := store.NewMonthStore(app)
	games := store.NewGameStore(app)

	player := newTestPlayer(t, players, "tester")

	existingGame := newTestGame("existing-game", "tester", "opp1")
	if err := games.SaveGame(existingGame, nil); err != nil {
		t.Fatalf("seeding existing game: %v", err)
	}
	if err := months.UpsertSynced(player.ID, 2024, 3, 1); err != nil {
		t.Fatalf("seeding synced current month: %v", err)
	}

	src := &fakeSource{
		archives: []data_sources.GameArchive{{Year: 2024, Month: 3}},
		gamesByMonth: map[string][]data_sources.Game{
			monthCallKey(2024, 3): {existingGame, newTestGame("new-game", "tester", "opp2")},
		},
	}

	svc := NewService(src, players, ratings, months, games)
	result, err := svc.SyncGames(context.Background(), player, SyncOptions{MaxMonths: 24})
	if err != nil {
		t.Fatalf("SyncGames: %v", err)
	}

	if len(src.monthGamesCalls) != 1 {
		t.Errorf("GetMonthGames calls = %v, want exactly 1 call (current month always re-fetched despite being marked synced)", src.monthGamesCalls)
	}
	if result.GamesFetched != 1 {
		t.Errorf("GamesFetched = %d, want 1 (only new-game; existing-game deduped by uuid)", result.GamesFetched)
	}
}

// TestCheckGamesCoverage covers the coverage-check's core cases: All and
// no-Since are never covered from cache alone; a fully-synced range not
// touching the still-open current month is covered; a range touching the
// current month never is.
func TestCheckGamesCoverage(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	ratings := store.NewRatingStore(app)
	months := store.NewMonthStore(app)
	games := store.NewGameStore(app)
	player := newTestPlayer(t, players, "tester")

	now := time.Now().UTC()
	prevYear, prevMonth := prevMonthOf(now.Year(), int(now.Month()))
	twoAgoYear, twoAgoMonth := prevMonthOf(prevYear, prevMonth)

	if err := months.UpsertSynced(player.ID, prevYear, prevMonth, 3); err != nil {
		t.Fatalf("seeding synced month: %v", err)
	}
	if err := months.UpsertSynced(player.ID, twoAgoYear, twoAgoMonth, 2); err != nil {
		t.Fatalf("seeding synced month: %v", err)
	}

	sinceTwoAgo := time.Date(twoAgoYear, time.Month(twoAgoMonth), 1, 0, 0, 0, 0, time.UTC)
	sincePrev := time.Date(prevYear, time.Month(prevMonth), 1, 0, 0, 0, 0, time.UTC)
	untilPrev := sincePrev

	tests := []struct {
		name string
		opts SyncOptions
		want bool
	}{
		{"All is never covered", SyncOptions{All: true, Since: &sinceTwoAgo}, false},
		{"no Since is never covered", SyncOptions{}, false},
		{"fully synced range not touching the current month is covered", SyncOptions{Since: &sinceTwoAgo, Until: &untilPrev}, true},
		{"a range touching the current month (Until defaults to now) is never covered", SyncOptions{Since: &sincePrev}, false},
	}

	svc := NewService(&fakeSource{}, players, ratings, months, games)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.CheckGamesCoverage(player, tt.opts)
			if err != nil {
				t.Fatalf("CheckGamesCoverage: %v", err)
			}
			if got.Covered != tt.want {
				t.Errorf("Covered = %v, want %v", got.Covered, tt.want)
			}
		})
	}
}

func prevMonthOf(year, month int) (int, int) {
	month--
	if month == 0 {
		month = 12
		year--
	}
	return year, month
}

// TestReparseMoves confirms a stored game's moves get rewritten from its
// already-persisted PGN, with zero upstream calls (the fakeSource here has
// no scripted responses at all - ReparseMoves must never call it).
func TestReparseMoves(t *testing.T) {
	app := newTestApp(t)
	players := store.NewPlayerStore(app)
	ratings := store.NewRatingStore(app)
	months := store.NewMonthStore(app)
	games := store.NewGameStore(app)
	newTestPlayer(t, players, "tester")

	game := newTestGame("game-1", "tester", "opponent")
	if err := games.SaveGame(game, nil); err != nil {
		t.Fatalf("seeding game with no moves: %v", err)
	}

	svc := NewService(&fakeSource{}, players, ratings, months, games)
	result, err := svc.ReparseMoves("tester")
	if err != nil {
		t.Fatalf("ReparseMoves: %v", err)
	}
	if result.GamesReparsed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v, want {GamesReparsed:1 Failed:0}", result)
	}

	stored, err := games.GamesForUsername("tester")
	if err != nil || len(stored) != 1 {
		t.Fatalf("GamesForUsername: %v, %d results", err, len(stored))
	}

	var moveCount int
	err = app.DB().Select("COUNT(*)").From("moves").
		Where(dbx.NewExp("game = {:g}", dbx.Params{"g": stored[0].ID})).
		Row(&moveCount)
	if err != nil {
		t.Fatalf("counting moves: %v", err)
	}
	if moveCount != 2 {
		t.Errorf("moves count = %d, want 2 (e4, e5 from the fixture PGN)", moveCount)
	}
}
