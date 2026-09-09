package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type MonthStore struct {
	app core.App
}

func NewMonthStore(app core.App) *MonthStore {
	return &MonthStore{app: app}
}

// IsSynced reports whether playerID's (year, month) archive has already
// been fully ingested. This is what replaces Valorant's HistoryExhausted
// flag entirely: chess.com's archive list is complete and finite, and
// every month but the current one is immutable once closed, so a synced
// row for a non-current month is provably complete forever - see
// internal/chesscom/ingest.
func (s *MonthStore) IsSynced(playerID string, year, month int) (bool, error) {
	_, err := s.app.FindFirstRecordByFilter("player_months",
		"player = {:player} && year = {:year} && month = {:month}",
		dbx.Params{"player": playerID, "year": year, "month": month})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpsertSynced records that playerID's (year, month) archive has been
// fetched, with gameCount games observed in it - safe to call repeatedly
// (e.g. every time the still-open current month is re-fetched).
func (s *MonthStore) UpsertSynced(playerID string, year, month, gameCount int) error {
	col, err := s.app.FindCollectionByNameOrId("player_months")
	if err != nil {
		return err
	}

	rec, err := s.app.FindFirstRecordByFilter("player_months",
		"player = {:player} && year = {:year} && month = {:month}",
		dbx.Params{"player": playerID, "year": year, "month": month})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("player", playerID)
		rec.Set("year", year)
		rec.Set("month", month)
	}

	rec.Set("synced_at", time.Now())
	rec.Set("game_count", gameCount)

	return s.app.Save(rec)
}
