package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("games")
		c.Fields.Add(
			&core.TextField{Name: "chesscom_uuid", Required: true},
			&core.TextField{Name: "url"},
			&core.TextField{Name: "time_control"},
			&core.TextField{Name: "time_class", Max: 16},
			&core.TextField{Name: "rules", Max: 32},
			&core.TextField{Name: "fen"},
			&core.TextField{Name: "eco"},
			// chess.com only ever reports when a game ended, never when it
			// started.
			&core.DateField{Name: "end_time"},
			&core.BoolField{Name: "rated"},
			// The raw PGN text, kept so a future PGN-parser fix can be
			// applied retroactively via reparse_moves with zero upstream
			// calls - see internal/chesscom/functions.go. Max must be set
			// explicitly: PocketBase's TextField defaults an unset Max (0)
			// to 5000 characters, not unlimited - a long game's movetext
			// with a %clk annotation on every move (see internal/chesscom/
			// pgn) routinely exceeds that (observed live: a 100+-move game
			// easily runs well past 5000 chars).
			&core.TextField{Name: "pgn", Max: 1 << 20},
			&core.JSONField{Name: "raw_json", MaxSize: 4 << 20},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_games_uuid", true, "chesscom_uuid", "")
		c.AddIndex("idx_games_end_time", false, "end_time", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("games")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
