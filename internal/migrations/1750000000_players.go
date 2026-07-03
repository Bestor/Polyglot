package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("players")
		c.Fields.Add(
			&core.TextField{Name: "riot_puuid", Required: true},
			&core.TextField{Name: "riot_name", Required: true},
			&core.TextField{Name: "riot_tag", Required: true},
			&core.TextField{Name: "region", Max: 16},
			&core.DateField{Name: "last_synced_match_at"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_players_puuid", true, "riot_puuid", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
