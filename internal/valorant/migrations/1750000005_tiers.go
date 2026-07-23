package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("tiers")
		c.Fields.Add(
			// Not Required: tier_id 0 ("Unrated") is a valid value, and
			// NumberField's Required rejects zero.
			&core.NumberField{Name: "tier_id", OnlyInt: true},
			&core.TextField{Name: "name"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_tiers_tier_id", true, "tier_id", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("tiers")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
