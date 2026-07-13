package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// datasources is polyglot's own bookkeeping collection - the persisted
// registry of onboarded DataProvider instances (type + config, including
// secrets like an API key). It is never listed in any provider's
// TableSpec, so it's automatically excluded from GET /metadata; GET
// /query additionally rejects any statement referencing it by name (see
// internal/polyglot/query.go's reservedTablePattern). Keep this
// collection's name in sync with internal/polyglot.datasourcesCollection.
func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("datasources")
		c.Fields.Add(
			&core.TextField{Name: "type", Required: true},
			&core.JSONField{Name: "config", MaxSize: 1 << 20},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_datasources_type", true, "type", "")
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
