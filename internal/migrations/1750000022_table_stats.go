package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Introspected additions to tables/columns - refreshed on every reconcile
// pass (internal/polyglot/catalog.go), never hand-edited, the same
// protection the existing introspected `type` field already gets: computed
// fresh from a live source and silently overwritten, unlike
// description/query_guidance, which reconcile never touches.
//
// row_count/sample_rows/last_updated are computed centrally in catalog.go
// via the datasource's own Instance.Query (every provider already has one)
// - not sourced from any provider-specific introspection.
// references_table/references_column ARE provider-specific (a PocketBase
// relation field for httpsql, a declared SQLite foreign key for sqlite) -
// see internal/dataprovider.ColumnCatalog.
func init() {
	m.Register(func(app core.App) error {
		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		tables.Fields.Add(
			&core.NumberField{Name: "row_count", OnlyInt: true},
			// Up to 5 example rows, row-object shape ({"col": val, ...}) -
			// the same shape GET /query already returns.
			&core.JSONField{Name: "sample_rows", MaxSize: 1 << 20},
			// Best-effort only: set from MAX(updated) when the table has a
			// column literally named "updated" (true for every current
			// Valorant table, PocketBase's own autodate convention), left
			// empty otherwise - not a universal guarantee.
			&core.DateField{Name: "last_updated"},
		)
		if err := app.Save(tables); err != nil {
			return err
		}

		columns, err := app.FindCollectionByNameOrId("columns")
		if err != nil {
			return err
		}
		columns.Fields.Add(
			// The target table/column of a foreign-key-like relation, e.g.
			// references_table="players", references_column="id" - empty
			// when none is mechanically known. Plain text, not a relation
			// to the tables collection: catalog.go's reconcile loop
			// upserts one table (and its columns) at a time in
			// Instance.Catalog()'s own order, so a relation's target
			// table's own row isn't guaranteed to exist yet within the
			// same pass if this were itself a PocketBase relation field.
			&core.TextField{Name: "references_table"},
			&core.TextField{Name: "references_column"},
		)
		return app.Save(columns)
	}, func(app core.App) error {
		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		tables.Fields.RemoveByName("row_count")
		tables.Fields.RemoveByName("sample_rows")
		tables.Fields.RemoveByName("last_updated")
		if err := app.Save(tables); err != nil {
			return err
		}

		columns, err := app.FindCollectionByNameOrId("columns")
		if err != nil {
			return err
		}
		columns.Fields.RemoveByName("references_table")
		columns.Fields.RemoveByName("references_column")
		return app.Save(columns)
	})
}
