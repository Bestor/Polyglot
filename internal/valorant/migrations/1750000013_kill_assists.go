package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		kills, err := app.FindCollectionByNameOrId("kills")
		if err != nil {
			return err
		}
		players, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("kill_assists")
		c.Fields.Add(
			&core.RelationField{Name: "kill", CollectionId: kills.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "assister", CollectionId: players.Id, Required: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_kill_assists_kill_assister", true, "kill, assister", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("kill_assists")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
