package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// functions is polyglot's persisted catalog of each onboarded datasource's
// warm-triggerable actions (see internal/dataprovider.FunctionRunner),
// reconciled the same async pass as tables/columns (internal/polyglot/
// catalog.go's reconcileCatalog). Unlike tables/columns' description
// (always empty from introspection, purely hand-curated), description/args
// here always mirror the live source - a function's description already
// comes meaningfully from the provider's own code (e.g. valorant.Function's
// own rich Description strings) - only query_guidance is ever human-authored,
// same never-overwrite-on-reconcile protection tables/columns get.
func init() {
	m.Register(func(app core.App) error {
		datasources, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}

		functions := core.NewBaseCollection("functions")
		functions.Fields.Add(
			&core.RelationField{Name: "datasource", Required: true, CollectionId: datasources.Id, CascadeDelete: true},
			&core.TextField{Name: "name", Required: true},
			&core.TextField{Name: "description"},
			&core.JSONField{Name: "args", MaxSize: 1 << 20},
			// query_guidance is the one field a human can add without a
			// code change - operational guidance beyond what description
			// already covers.
			&core.TextField{Name: "query_guidance"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		functions.AddIndex("idx_functions_datasource_name", true, "datasource, name", "")
		return app.Save(functions)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("functions")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
