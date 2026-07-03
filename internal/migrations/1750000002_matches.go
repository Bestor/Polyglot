package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		seasons, err := app.FindCollectionByNameOrId("seasons")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("matches")
		c.Fields.Add(
			&core.TextField{Name: "match_id", Required: true},
			&core.TextField{Name: "map"},
			&core.TextField{Name: "mode"},
			&core.TextField{Name: "queue"},
			&core.RelationField{Name: "season", CollectionId: seasons.Id},
			// Always populated even if the "season" relation lookup misses,
			// so ingestion never blocks on the seasons table being complete.
			&core.TextField{Name: "season_id_raw"},
			&core.DateField{Name: "game_start"},
			&core.NumberField{Name: "rounds_played", OnlyInt: true},
			&core.JSONField{Name: "raw_json", MaxSize: 2 << 20},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_matches_match_id", true, "match_id", "")
		c.AddIndex("idx_matches_game_start", false, "game_start", "")
		c.AddIndex("idx_matches_season", false, "season", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
