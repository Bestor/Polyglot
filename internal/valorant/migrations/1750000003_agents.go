package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("agents")
		c.Fields.Add(
			&core.TextField{Name: "agent_id", Required: true},
			&core.TextField{Name: "name"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_agents_agent_id", true, "agent_id", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("agents")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
