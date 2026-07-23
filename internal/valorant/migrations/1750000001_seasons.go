package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("seasons")
		c.Fields.Add(
			&core.TextField{Name: "season_id", Required: true},
			&core.TextField{Name: "short_code"},
			// Hand-curated human label (e.g. "Episode 7 Act 3") since
			// HenrikDev's content endpoint doesn't reliably expose it.
			&core.TextField{Name: "label"},
			&core.TextField{Name: "parent_season_id"},
			&core.BoolField{Name: "is_active"},
			&core.DateField{Name: "start_date"},
			&core.DateField{Name: "end_date"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_seasons_season_id", true, "season_id", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("seasons")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
