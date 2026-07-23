package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// A datasource's identity moves from provider Type() to a user-chosen
// name, since multiple onboarded datasources can now share one provider
// type (e.g. two onboarded SQLite files, or two http_sql connections) -
// see internal/polyglot/registry.go. description/query_guidance are the
// new connection-level AI guidance fields (distinct from any one table's -
// e.g. "this connection is a columnar OLAP warehouse, queries without a
// partition filter will be slow").
func init() {
	m.Register(func(app core.App) error {
		datasources, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}

		datasources.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.TextField{Name: "description"},
			&core.TextField{Name: "query_guidance"},
		)
		if err := app.Save(datasources); err != nil {
			return err
		}

		// Backfill any pre-existing row's name from its type - defensive:
		// core polyglot's own data directory is expected to be created
		// fresh going forward (see the two-binary cutover plan), but this
		// keeps the migration correct for a test app or an already-running
		// instance either way.
		if _, err := app.DB().NewQuery(`UPDATE datasources SET name = type WHERE name = '' OR name IS NULL`).Execute(); err != nil {
			return err
		}

		datasources, err = app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}
		datasources.RemoveIndex("idx_datasources_type")
		datasources.AddIndex("idx_datasources_name", true, "name", "")
		return app.Save(datasources)
	}, func(app core.App) error {
		datasources, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}
		datasources.RemoveIndex("idx_datasources_name")
		datasources.AddIndex("idx_datasources_type", true, "type", "")
		datasources.Fields.RemoveByName("name")
		datasources.Fields.RemoveByName("description")
		datasources.Fields.RemoveByName("query_guidance")
		return app.Save(datasources)
	})
}
