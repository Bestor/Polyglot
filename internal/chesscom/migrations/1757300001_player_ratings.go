package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// player_ratings holds one upserted, current-value snapshot per rating
// category - not a time series. Which fields are populated depends on the
// category: chess_bullet/blitz/rapid/daily populate every field; tactics
// only best_rating+best_date; puzzle_rush_best/puzzle_rush_daily only
// best_rating+attempts; fide only last_rating. One nullable-column table
// rather than a second table, since the union of fields across every
// category chess.com reports is small.
func init() {
	m.Register(func(app core.App) error {
		players, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("player_ratings")
		c.Fields.Add(
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true, CascadeDelete: true},
			&core.TextField{Name: "category", Required: true},
			&core.NumberField{Name: "last_rating", OnlyInt: true},
			&core.NumberField{Name: "last_rd", OnlyInt: true},
			&core.DateField{Name: "last_date"},
			&core.NumberField{Name: "best_rating", OnlyInt: true},
			&core.DateField{Name: "best_date"},
			&core.TextField{Name: "best_game_url"},
			&core.NumberField{Name: "wins", OnlyInt: true},
			&core.NumberField{Name: "losses", OnlyInt: true},
			&core.NumberField{Name: "draws", OnlyInt: true},
			&core.NumberField{Name: "attempts", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_player_ratings_player_category", true, "player, category", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("player_ratings")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
