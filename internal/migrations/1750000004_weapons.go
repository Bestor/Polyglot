package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("weapons")
		c.Fields.Add(
			// weapon_id is not marked Required: the bomb's "weapon" in kill
			// events can carry an empty id, and Required rejects empty
			// strings on TextField.
			&core.TextField{Name: "weapon_id"},
			&core.TextField{Name: "name"},
			&core.TextField{Name: "type"}, // 'Weapon' | 'Ability' | 'Bomb'
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_weapons_weapon_id", true, "weapon_id", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("weapons")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
