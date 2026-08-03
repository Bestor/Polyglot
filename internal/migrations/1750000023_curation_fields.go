package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Curated additions to datasources/tables - hand- or AI-authored via
// POST /datasources/annotate and POST /tables/annotate (internal/polyglot/
// datasources.go), same never-overwritten-on-reconcile protection
// description/query_guidance already get.
//
// glossary lives at the datasource level, not per-table or per-column:
// terms like "ACS"/"ADR"/"KAST" span many tables, so one glossary per
// datasource avoids repeating the same definitions everywhere the term is
// used. A column-specific quirk still fits naturally in that column's own
// existing description - no new column-level field is added here.
//
// example_queries exists at both levels: datasource-level for cross-table
// "cookbook" entries (e.g. a player's full match history, joining
// players/match_players/matches), table-level for queries naturally
// scoped to one or two tables.
func init() {
	m.Register(func(app core.App) error {
		datasources, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}
		datasources.Fields.Add(
			// []{term, definition}
			&core.JSONField{Name: "glossary", MaxSize: 1 << 20},
			// []{question, sql}
			&core.JSONField{Name: "example_queries", MaxSize: 1 << 20},
		)
		if err := app.Save(datasources); err != nil {
			return err
		}

		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		tables.Fields.Add(
			// Two independently-patchable fields, not one - mirrors
			// description/query_guidance already being separate fields
			// rather than one blob.
			&core.TextField{Name: "good_for"},
			&core.TextField{Name: "bad_for"},
			&core.TextField{Name: "known_gaps"},
			// []{question, sql}
			&core.JSONField{Name: "example_queries", MaxSize: 1 << 20},
		)
		return app.Save(tables)
	}, func(app core.App) error {
		datasources, err := app.FindCollectionByNameOrId("datasources")
		if err != nil {
			return err
		}
		datasources.Fields.RemoveByName("glossary")
		datasources.Fields.RemoveByName("example_queries")
		if err := app.Save(datasources); err != nil {
			return err
		}

		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		tables.Fields.RemoveByName("good_for")
		tables.Fields.RemoveByName("bad_for")
		tables.Fields.RemoveByName("known_gaps")
		tables.Fields.RemoveByName("example_queries")
		return app.Save(tables)
	})
}
