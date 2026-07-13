package store

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/providers/valorant/data_sources"
)

type Season struct {
	ID             string
	SeasonID       string
	ShortCode      string
	Label          string
	ParentSeasonID string
	IsActive       bool
}

type SeasonStore struct {
	app core.App
}

func NewSeasonStore(app core.App) *SeasonStore {
	return &SeasonStore{app: app}
}

func (s *SeasonStore) FindBySeasonID(seasonID string) (Season, bool, error) {
	rec, err := s.app.FindFirstRecordByFilter("seasons", "season_id = {:id}", dbx.Params{"id": seasonID})
	if errors.Is(err, sql.ErrNoRows) {
		return Season{}, false, nil
	}
	if err != nil {
		return Season{}, false, err
	}
	return recordToSeason(rec), true, nil
}

// Upsert stores a season fetched from the data source. It never overwrites
// an existing hand-curated "label", since the provider's content endpoint
// doesn't reliably supply a human-friendly name.
func (s *SeasonStore) Upsert(season data_sources.Season) (Season, error) {
	col, err := s.app.FindCollectionByNameOrId("seasons")
	if err != nil {
		return Season{}, err
	}

	rec, err := s.app.FindFirstRecordByFilter("seasons", "season_id = {:id}", dbx.Params{"id": season.SeasonID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Season{}, err
	}

	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("season_id", season.SeasonID)
	}

	rec.Set("short_code", season.ShortCode)
	rec.Set("parent_season_id", season.ParentSeasonID)
	rec.Set("is_active", season.IsActive)

	if err := s.app.Save(rec); err != nil {
		return Season{}, err
	}

	return recordToSeason(rec), nil
}

func recordToSeason(rec *core.Record) Season {
	return Season{
		ID:             rec.Id,
		SeasonID:       rec.GetString("season_id"),
		ShortCode:      rec.GetString("short_code"),
		Label:          rec.GetString("label"),
		ParentSeasonID: rec.GetString("parent_season_id"),
		IsActive:       rec.GetBool("is_active"),
	}
}
