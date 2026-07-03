// Package store is the PocketBase data-access layer: typed wrappers around
// core.Record for the players/seasons/matches/match_players collections.
package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type Player struct {
	ID                string
	PUUID             string
	Name              string
	Tag               string
	Region            string
	LastSyncedMatchAt time.Time
}

type PlayerStore struct {
	app core.App
}

func NewPlayerStore(app core.App) *PlayerStore {
	return &PlayerStore{app: app}
}

func (s *PlayerStore) FindByPUUID(puuid string) (Player, bool, error) {
	rec, err := s.app.FindFirstRecordByFilter("players", "riot_puuid = {:puuid}", dbx.Params{"puuid": puuid})
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, false, nil
	}
	if err != nil {
		return Player{}, false, err
	}
	return recordToPlayer(rec), true, nil
}

// FindByRiotID looks up a cached player by their last-known name#tag, so
// callers can avoid an account-lookup API call when we've already resolved
// this Riot ID before. A miss here doesn't necessarily mean the player has
// never been cached (e.g. they may have changed their name), just that a
// fresh lookup is required.
func (s *PlayerStore) FindByRiotID(name, tag string) (Player, bool, error) {
	rec, err := s.app.FindFirstRecordByFilter("players", "riot_name = {:name} && riot_tag = {:tag}", dbx.Params{"name": name, "tag": tag})
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, false, nil
	}
	if err != nil {
		return Player{}, false, err
	}
	return recordToPlayer(rec), true, nil
}

// Upsert creates a players row for the given PUUID if none exists, or
// refreshes its name/tag/region if it does. Empty fields on p are not
// applied over existing non-empty values, so opportunistic upserts of
// other match participants (where we may not know their region) don't
// clobber data resolved elsewhere.
func (s *PlayerStore) Upsert(p Player) (Player, error) {
	col, err := s.app.FindCollectionByNameOrId("players")
	if err != nil {
		return Player{}, err
	}

	rec, err := s.app.FindFirstRecordByFilter("players", "riot_puuid = {:puuid}", dbx.Params{"puuid": p.PUUID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Player{}, err
	}

	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("riot_puuid", p.PUUID)
	}

	if p.Name != "" {
		rec.Set("riot_name", p.Name)
	}
	if p.Tag != "" {
		rec.Set("riot_tag", p.Tag)
	}
	if p.Region != "" {
		rec.Set("region", p.Region)
	}

	if err := s.app.Save(rec); err != nil {
		return Player{}, err
	}

	return recordToPlayer(rec), nil
}

func (s *PlayerStore) UpdateLastSyncedMatchAt(id string, t time.Time) error {
	rec, err := s.app.FindRecordById("players", id)
	if err != nil {
		return err
	}
	rec.Set("last_synced_match_at", t)
	return s.app.Save(rec)
}

func recordToPlayer(rec *core.Record) Player {
	return Player{
		ID:                rec.Id,
		PUUID:             rec.GetString("riot_puuid"),
		Name:              rec.GetString("riot_name"),
		Tag:               rec.GetString("riot_tag"),
		Region:            rec.GetString("region"),
		LastSyncedMatchAt: rec.GetDateTime("last_synced_match_at").Time(),
	}
}
