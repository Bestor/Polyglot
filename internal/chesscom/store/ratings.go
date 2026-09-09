package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Rating is one category's current-value snapshot for a player. Which
// fields are populated depends on Category - see the player_ratings
// migration's doc comment (chess_bullet/blitz/rapid/daily populate every
// field; tactics only BestRating+BestDate; puzzle_rush only
// BestRating+Attempts; fide only LastRating).
type Rating struct {
	Category            string
	LastRating, LastRD  int
	LastDate            time.Time
	BestRating          int
	BestDate            time.Time
	BestGameURL         string
	Wins, Losses, Draws int
	Attempts            int
}

type RatingStore struct {
	app core.App
}

func NewRatingStore(app core.App) *RatingStore {
	return &RatingStore{app: app}
}

// Upsert replaces playerID's snapshot for r.Category - always a full
// overwrite, never a partial merge, mirroring sync_stats' own "always a
// full refresh" semantics: chess.com's stats endpoint always reports a
// category's complete current standing in one call, so there's nothing to
// conditionally preserve the way PlayerStore.UpsertStub must for an
// opportunistically-cached opponent.
func (s *RatingStore) Upsert(playerID string, r Rating) error {
	col, err := s.app.FindCollectionByNameOrId("player_ratings")
	if err != nil {
		return err
	}

	rec, err := s.app.FindFirstRecordByFilter("player_ratings",
		"player = {:player} && category = {:category}",
		dbx.Params{"player": playerID, "category": r.Category})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("player", playerID)
		rec.Set("category", r.Category)
	}

	rec.Set("last_rating", r.LastRating)
	rec.Set("last_rd", r.LastRD)
	if !r.LastDate.IsZero() {
		rec.Set("last_date", r.LastDate)
	}
	rec.Set("best_rating", r.BestRating)
	if !r.BestDate.IsZero() {
		rec.Set("best_date", r.BestDate)
	}
	rec.Set("best_game_url", r.BestGameURL)
	rec.Set("wins", r.Wins)
	rec.Set("losses", r.Losses)
	rec.Set("draws", r.Draws)
	rec.Set("attempts", r.Attempts)

	return s.app.Save(rec)
}
