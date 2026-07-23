package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		maps, err := app.FindCollectionByNameOrId("maps")
		if err != nil {
			return err
		}
		seasons, err := app.FindCollectionByNameOrId("seasons")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("matches")
		c.Fields.Add(
			&core.TextField{Name: "match_id", Required: true},
			&core.RelationField{Name: "map", CollectionId: maps.Id},
			&core.TextField{Name: "game_version"},
			&core.TextField{Name: "queue_id"},        // e.g. "competitive"
			&core.TextField{Name: "queue_mode_type"}, // e.g. "Standard"
			&core.RelationField{Name: "season", CollectionId: seasons.Id},
			// Always populated even if the "season" relation lookup misses,
			// so ingestion never blocks on the seasons table being complete.
			&core.TextField{Name: "season_id_raw"},
			&core.TextField{Name: "platform"},
			&core.TextField{Name: "region"},
			&core.TextField{Name: "cluster"},
			&core.DateField{Name: "started_at"},
			&core.NumberField{Name: "game_length_ms", OnlyInt: true},
			&core.BoolField{Name: "is_completed"},
			&core.JSONField{Name: "raw_json", MaxSize: 4 << 20},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_matches_match_id", true, "match_id", "")
		c.AddIndex("idx_matches_started_at", false, "started_at", "")
		c.AddIndex("idx_matches_season", false, "season", "")
		c.AddIndex("idx_matches_map", false, "map", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
