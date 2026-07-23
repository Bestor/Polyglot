package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("maps")
		c.Fields.Add(
			&core.TextField{Name: "map_id", Required: true},
			&core.TextField{Name: "name"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_maps_map_id", true, "map_id", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("maps")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
