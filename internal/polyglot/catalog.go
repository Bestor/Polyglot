package polyglot

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
)

// reconcileCatalog calls inst.Catalog(ctx) for live ground truth, then
// upserts - never overwrites - the persisted tables/columns snapshot for
// datasourceName: inserts tables/columns not yet present, deletes ones no
// longer present upstream (cascade handles columns), refreshes a column's
// type, but never touches an existing row's hand-authored
// description/query_guidance. Safe to call repeatedly, including after a
// human has annotated things - re-running this must never clobber curated
// text.
func reconcileCatalog(ctx context.Context, app core.App, datasourceName string, inst dataprovider.Instance) error {
	dsRec, err := app.FindFirstRecordByFilter("datasources", "name = {:name}", dbx.Params{"name": datasourceName})
	if err != nil {
		return fmt.Errorf("catalog: looking up datasource %q: %w", datasourceName, err)
	}

	live, err := inst.Catalog(ctx)
	if err != nil {
		return fmt.Errorf("catalog: introspecting %q: %w", datasourceName, err)
	}

	tablesCol, err := app.FindCachedCollectionByNameOrId("tables")
	if err != nil {
		return err
	}
	columnsCol, err := app.FindCachedCollectionByNameOrId("columns")
	if err != nil {
		return err
	}

	existingTables, err := app.FindRecordsByFilter("tables", "datasource = {:ds}", "", 0, 0, dbx.Params{"ds": dsRec.Id})
	if err != nil {
		return fmt.Errorf("catalog: listing existing tables for %q: %w", datasourceName, err)
	}
	existingByName := make(map[string]*core.Record, len(existingTables))
	for _, t := range existingTables {
		existingByName[t.GetString("name")] = t
	}

	liveNames := make(map[string]bool, len(live))
	for _, t := range live {
		liveNames[t.Name] = true

		tableRec, ok := existingByName[t.Name]
		if !ok {
			tableRec = core.NewRecord(tablesCol)
			tableRec.Set("datasource", dsRec.Id)
			tableRec.Set("name", t.Name)
			if err := app.Save(tableRec); err != nil {
				return fmt.Errorf("catalog: creating table %q: %w", t.Name, err)
			}
		}

		if err := reconcileColumns(app, columnsCol, tableRec, t.Columns); err != nil {
			return fmt.Errorf("catalog: reconciling columns for %q: %w", t.Name, err)
		}
	}

	// Delete tables no longer present upstream - CascadeDelete on
	// columns.table handles their columns.
	for name, rec := range existingByName {
		if !liveNames[name] {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("catalog: deleting stale table %q: %w", name, err)
			}
		}
	}

	return nil
}

func reconcileColumns(app core.App, columnsCol *core.Collection, tableRec *core.Record, liveColumns []dataprovider.ColumnCatalog) error {
	existing, err := app.FindRecordsByFilter("columns", "table = {:table}", "", 0, 0, dbx.Params{"table": tableRec.Id})
	if err != nil {
		return err
	}
	existingByName := make(map[string]*core.Record, len(existing))
	for _, c := range existing {
		existingByName[c.GetString("name")] = c
	}

	liveNames := make(map[string]bool, len(liveColumns))
	for _, c := range liveColumns {
		liveNames[c.Name] = true

		colRec, ok := existingByName[c.Name]
		if !ok {
			colRec = core.NewRecord(columnsCol)
			colRec.Set("table", tableRec.Id)
			colRec.Set("name", c.Name)
			colRec.Set("type", c.Type)
			if err := app.Save(colRec); err != nil {
				return fmt.Errorf("creating column %q: %w", c.Name, err)
			}
			continue
		}

		// Only the introspected type is ever refreshed on an existing
		// row - description is hand-authored and must never be touched
		// here.
		if colRec.GetString("type") != c.Type {
			colRec.Set("type", c.Type)
			if err := app.Save(colRec); err != nil {
				return fmt.Errorf("updating column %q: %w", c.Name, err)
			}
		}
	}

	for name, rec := range existingByName {
		if !liveNames[name] {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("deleting stale column %q: %w", name, err)
			}
		}
	}

	return nil
}
