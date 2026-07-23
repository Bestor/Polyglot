package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// tables/columns are polyglot's own persisted catalog snapshot: reconciled
// (inserted/deleted/type-refreshed, but description/query_guidance never
// overwritten) from each active datasource's live Instance.Catalog() by
// internal/polyglot/catalog.go, and served as-is (no live calls) by
// GET /metadata - see internal/polyglot/metadata.go for why that matters.
func init() {
	m.Register(func(app core.App) error {
		datasources, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}

		tables := core.NewBaseCollection("tables")
		tables.Fields.Add(
			&core.RelationField{Name: "datasource", Required: true, CollectionId: datasources.Id, CascadeDelete: true},
			&core.TextField{Name: "name", Required: true},
			&core.TextField{Name: "description"},
			// query_guidance is table-level AI guidance distinct from
			// description - e.g. "partitioned by event_date, always
			// filter on it" - how to query well, not what the table means.
			&core.TextField{Name: "query_guidance"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		tables.AddIndex("idx_tables_datasource_name", true, "datasource, name", "")
		if err := app.Save(tables); err != nil {
			return err
		}

		columns := core.NewBaseCollection("columns")
		columns.Fields.Add(
			&core.RelationField{Name: "table", Required: true, CollectionId: tables.Id, CascadeDelete: true},
			&core.TextField{Name: "name", Required: true},
			&core.TextField{Name: "type"}, // introspected native type, e.g. sqlite's "TEXT"
			&core.TextField{Name: "description"},
			// Deliberately no query_guidance on columns - table-level
			// prose can name specific columns when relevant; a third
			// tier would be over-engineering.
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		columns.AddIndex("idx_columns_table_name", true, "table, name", "")
		return app.Save(columns)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("columns"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		c, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
