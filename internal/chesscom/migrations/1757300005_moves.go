package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// moves is the round/kill-level granularity for chess.com's domain: one
// row per ply, SAN + clock time only - no per-move fen_after, which would
// need a real chess move-generation engine to replay SAN into board state
// (see internal/chesscom's schema notes on why that's explicitly out of
// scope).
func init() {
	m.Register(func(app core.App) error {
		games, err := app.FindCollectionByNameOrId("games")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("moves")
		c.Fields.Add(
			&core.RelationField{Name: "game", CollectionId: games.Id, Required: true, CascadeDelete: true},
			&core.NumberField{Name: "ply", Required: true, OnlyInt: true},
			// move_number/color are redundant with ply (parity/((ply+1)/2))
			// but kept as their own columns for query convenience, the same
			// idiom rounds/kills already use for redundant-but-convenient
			// columns alongside a relation that technically implies them.
			&core.NumberField{Name: "move_number", OnlyInt: true},
			&core.TextField{Name: "color", Max: 8}, // 'white' | 'black'
			&core.TextField{Name: "san", Required: true},
			// Null is expected for daily/correspondence games with no
			// %clk annotations, not an error - see internal/chesscom/pgn.
			&core.NumberField{Name: "clock_seconds"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_moves_game_ply", true, "game, ply", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("moves")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
