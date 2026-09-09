package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// game_players is a join row per side (white/black), not denormalized
// white_*/black_* columns on games - chess always has exactly two sides,
// but normalizing still wins for side-relative queries (e.g. "average
// opponent rating by player" becomes a clean self-join) - see
// internal/chesscom's schema notes, weighed against match_players'
// own precedent for Valorant's variable-arity matches.
func init() {
	m.Register(func(app core.App) error {
		games, err := app.FindCollectionByNameOrId("games")
		if err != nil {
			return err
		}
		players, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("game_players")
		c.Fields.Add(
			&core.RelationField{Name: "game", CollectionId: games.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true, CascadeDelete: true},
			&core.TextField{Name: "color", Required: true, Max: 8}, // 'white' | 'black'
			&core.TextField{Name: "username_snapshot"},
			// rating at the time of this game - distinct from
			// player_ratings' current-value snapshot.
			&core.NumberField{Name: "rating", OnlyInt: true},
			&core.TextField{Name: "result"}, // chess.com's raw result string
			&core.BoolField{Name: "won"},    // derived: result == "win"
			// Non-nil only for the minority of games chess.com has
			// analyzed for accuracy.
			&core.NumberField{Name: "accuracy"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_game_players_game_color", true, "game, color", "")
		c.AddIndex("idx_game_players_player", false, "player", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("game_players")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
