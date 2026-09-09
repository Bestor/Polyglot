// Package store is the PocketBase data-access layer for internal/chesscom:
// typed wrappers around core.Record for the players/player_ratings/
// player_months/games/game_players/moves collections, mirroring
// internal/valorant/store's own pattern.
package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type Player struct {
	ID       string
	Username string
	// PlayerID is 0 until this username has actually been resolved via
	// GetPlayerProfile - see the players migration's doc comment on why
	// it's neither required nor unique.
	PlayerID          int64
	Name              string
	Title             string
	CountryCode       string
	Status            string
	AvatarURL         string
	Followers         int
	JoinedAt          time.Time
	LastSyncedGamesAt time.Time
}

type PlayerStore struct {
	app core.App
}

func NewPlayerStore(app core.App) *PlayerStore {
	return &PlayerStore{app: app}
}

// FindByUsername looks up a cached player case-insensitively - chess.com
// usernames are case-insensitive account identifiers, and a caller (human
// or AI) may mention one in any casing.
func (s *PlayerStore) FindByUsername(username string) (Player, bool, error) {
	var id string
	err := s.app.DB().Select("id").
		From("players").
		Where(dbx.NewExp("LOWER(chesscom_username) = LOWER({:username})", dbx.Params{"username": username})).
		Limit(1).
		Row(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, false, nil
	}
	if err != nil {
		return Player{}, false, err
	}

	rec, err := s.app.FindRecordById("players", id)
	if err != nil {
		return Player{}, false, err
	}
	return recordToPlayer(rec), true, nil
}

// UpsertProfile creates or fully refreshes a player's row from a resolved
// GetPlayerProfile call - unlike UpsertStub, every field here is known and
// authoritative, so nothing is conditionally preserved.
func (s *PlayerStore) UpsertProfile(p Player) (Player, error) {
	rec, _, err := s.findOrNewByUsername(p.Username)
	if err != nil {
		return Player{}, err
	}

	rec.Set("chesscom_username", p.Username)
	rec.Set("chesscom_player_id", p.PlayerID)
	rec.Set("name", p.Name)
	rec.Set("title", p.Title)
	rec.Set("country_code", p.CountryCode)
	rec.Set("status", p.Status)
	rec.Set("avatar_url", p.AvatarURL)
	rec.Set("followers", p.Followers)
	if !p.JoinedAt.IsZero() {
		rec.Set("joined_at", p.JoinedAt)
	}

	if err := s.app.Save(rec); err != nil {
		return Player{}, err
	}
	return recordToPlayer(rec), nil
}

// UpsertStub opportunistically caches a player encountered only as a
// game's opponent (username/rating/result, never a full profile) - creates
// a minimal row keyed on username if none exists yet, and never overwrites
// an existing row's already-resolved profile fields, since a stub is
// strictly less informative than whatever a prior resolve_player/
// sync_stats call may have already filled in.
func (s *PlayerStore) UpsertStub(username string) (Player, error) {
	rec, isNew, err := s.findOrNewByUsername(username)
	if err != nil {
		return Player{}, err
	}
	if !isNew {
		return recordToPlayer(rec), nil
	}

	rec.Set("chesscom_username", username)
	if err := s.app.Save(rec); err != nil {
		return Player{}, err
	}
	return recordToPlayer(rec), nil
}

func (s *PlayerStore) UpdateLastSyncedGamesAt(id string, t time.Time) error {
	rec, err := s.app.FindRecordById("players", id)
	if err != nil {
		return err
	}
	rec.Set("last_synced_games_at", t)
	return s.app.Save(rec)
}

// findOrNewByUsername looks up a player case-insensitively via a raw dbx
// query, not FindFirstRecordByFilter - PocketBase's own filter DSL (which
// FindFirstRecordByFilter's filter string is parsed with) doesn't
// recognize SQL functions like LOWER(), unlike a raw dbx.NewExp query
// against the underlying SQLite table directly. Mirrors FindByUsername's
// own working pattern.
func (s *PlayerStore) findOrNewByUsername(username string) (rec *core.Record, isNew bool, err error) {
	col, err := s.app.FindCollectionByNameOrId("players")
	if err != nil {
		return nil, false, err
	}

	var id string
	err = s.app.DB().Select("id").
		From("players").
		Where(dbx.NewExp("LOWER(chesscom_username) = LOWER({:username})", dbx.Params{"username": username})).
		Limit(1).
		Row(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewRecord(col), true, nil
	}

	rec, err = s.app.FindRecordById("players", id)
	if err != nil {
		return nil, false, err
	}
	return rec, false, nil
}

func recordToPlayer(rec *core.Record) Player {
	return Player{
		ID:                rec.Id,
		Username:          rec.GetString("chesscom_username"),
		PlayerID:          int64(rec.GetInt("chesscom_player_id")),
		Name:              rec.GetString("name"),
		Title:             rec.GetString("title"),
		CountryCode:       rec.GetString("country_code"),
		Status:            rec.GetString("status"),
		AvatarURL:         rec.GetString("avatar_url"),
		Followers:         rec.GetInt("followers"),
		JoinedAt:          rec.GetDateTime("joined_at").Time(),
		LastSyncedGamesAt: rec.GetDateTime("last_synced_games_at").Time(),
	}
}
