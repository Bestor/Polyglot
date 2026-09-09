package store

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/chesscom/data_sources"
	"val-analyzer/internal/chesscom/pgn"
)

type GameStore struct {
	app core.App
}

func NewGameStore(app core.App) *GameStore {
	return &GameStore{app: app}
}

func (s *GameStore) Exists(uuid string) (bool, error) {
	_, err := s.app.FindFirstRecordByFilter("games", "chesscom_uuid = {:uuid}", dbx.Params{"uuid": uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// SaveGame persists one game and its full normalized breakdown (both
// sides' game_players rows, and the parsed move list) in a single
// transaction. Both White and Black are opportunistically upserted into
// players (as a minimal stub if not already known) - the same idiom
// MatchStore.SaveMatch uses for every player referenced in a Valorant
// match, so a later question about an opponent who already showed up in a
// cached game doesn't require resolving them separately.
func (s *GameStore) SaveGame(g data_sources.Game, moves []pgn.Move) error {
	return s.app.RunInTransaction(func(txApp core.App) error {
		gamesCol, err := txApp.FindCollectionByNameOrId("games")
		if err != nil {
			return err
		}
		gamePlayersCol, err := txApp.FindCollectionByNameOrId("game_players")
		if err != nil {
			return err
		}

		gameRec := core.NewRecord(gamesCol)
		gameRec.Set("chesscom_uuid", g.UUID)
		gameRec.Set("url", g.URL)
		gameRec.Set("time_control", g.TimeControl)
		gameRec.Set("time_class", g.TimeClass)
		gameRec.Set("rules", g.Rules)
		gameRec.Set("fen", g.FEN)
		gameRec.Set("eco", g.ECO)
		if !g.EndTime.IsZero() {
			gameRec.Set("end_time", g.EndTime)
		}
		gameRec.Set("rated", g.Rated)
		gameRec.Set("pgn", g.PGN)
		if len(g.Raw) > 0 {
			gameRec.Set("raw_json", string(g.Raw))
		}
		if err := txApp.Save(gameRec); err != nil {
			return err
		}
		gameID := gameRec.Id

		players := NewPlayerStore(txApp)
		sides := []struct {
			color    string
			gp       data_sources.GamePlayer
			accuracy *float64
		}{
			{"white", g.White, g.WhiteAccuracy},
			{"black", g.Black, g.BlackAccuracy},
		}
		for _, side := range sides {
			player, err := players.UpsertStub(side.gp.Username)
			if err != nil {
				return err
			}

			rec := core.NewRecord(gamePlayersCol)
			rec.Set("game", gameID)
			rec.Set("player", player.ID)
			rec.Set("color", side.color)
			rec.Set("username_snapshot", side.gp.Username)
			rec.Set("rating", side.gp.Rating)
			rec.Set("result", side.gp.Result)
			rec.Set("won", side.gp.Result == "win")
			if side.accuracy != nil {
				rec.Set("accuracy", *side.accuracy)
			}
			if err := txApp.Save(rec); err != nil {
				return err
			}
		}

		return saveMoves(txApp, gameID, moves)
	})
}

// ReplaceMoves deletes and re-inserts a game's moves from a freshly
// re-parsed list - used by the reparse_moves function to apply a PGN
// parser fix retroactively, with zero upstream calls, since the raw PGN
// text is already persisted on the game row.
func (s *GameStore) ReplaceMoves(gameID string, moves []pgn.Move) error {
	return s.app.RunInTransaction(func(txApp core.App) error {
		existing, err := txApp.FindRecordsByFilter("moves", "game = {:game}", "", 0, 0, dbx.Params{"game": gameID})
		if err != nil {
			return err
		}
		for _, rec := range existing {
			if err := txApp.Delete(rec); err != nil {
				return err
			}
		}
		return saveMoves(txApp, gameID, moves)
	})
}

func saveMoves(app core.App, gameID string, moves []pgn.Move) error {
	movesCol, err := app.FindCollectionByNameOrId("moves")
	if err != nil {
		return err
	}
	for _, mv := range moves {
		rec := core.NewRecord(movesCol)
		rec.Set("game", gameID)
		rec.Set("ply", mv.Ply)
		rec.Set("move_number", mv.MoveNumber)
		rec.Set("color", mv.Color)
		rec.Set("san", mv.SAN)
		if mv.ClockSeconds != nil {
			rec.Set("clock_seconds", *mv.ClockSeconds)
		}
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// StoredGame is the minimal shape reparse_moves needs: enough to re-parse
// a game's already-persisted PGN without any upstream call.
type StoredGame struct {
	ID  string
	PGN string
}

// GamesForUsername returns every stored game a given username played in
// (either side), for reparse_moves' per-username mode - matched against
// game_players.username_snapshot (the historical record of who played a
// given game), not a live profile lookup. A raw joined dbx query, not
// FindRecordsByFilter: PocketBase's own filter DSL doesn't recognize SQL
// functions like LOWER() (see PlayerStore.findOrNewByUsername's own doc
// comment on the same constraint).
func (s *GameStore) GamesForUsername(username string) ([]StoredGame, error) {
	var rows []struct {
		ID  string `db:"id"`
		PGN string `db:"pgn"`
	}
	err := s.app.DB().NewQuery(`
		SELECT DISTINCT games.id AS id, games.pgn AS pgn
		FROM games
		JOIN game_players ON game_players.game = games.id
		WHERE LOWER(game_players.username_snapshot) = LOWER({:username})
	`).Bind(dbx.Params{"username": username}).All(&rows)
	if err != nil {
		return nil, err
	}

	games := make([]StoredGame, 0, len(rows))
	for _, r := range rows {
		games = append(games, StoredGame{ID: r.ID, PGN: r.PGN})
	}
	return games, nil
}

// AllGames returns every stored game, for reparse_moves' no-username (all
// players) mode.
func (s *GameStore) AllGames() ([]StoredGame, error) {
	recs, err := s.app.FindRecordsByFilter("games", "", "", 0, 0, nil)
	if err != nil {
		return nil, err
	}
	return recordsToStoredGames(recs), nil
}

func recordsToStoredGames(recs []*core.Record) []StoredGame {
	games := make([]StoredGame, 0, len(recs))
	for _, rec := range recs {
		games = append(games, StoredGame{ID: rec.Id, PGN: rec.GetString("pgn")})
	}
	return games
}
