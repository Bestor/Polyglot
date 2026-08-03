package polyglot

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
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

		if err := reconcileTableStats(ctx, inst, tableRec, t.Name, t.Columns); err != nil {
			return fmt.Errorf("catalog: computing stats for %q: %w", t.Name, err)
		}
		if err := app.Save(tableRec); err != nil {
			return fmt.Errorf("catalog: saving stats for %q: %w", t.Name, err)
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

	if fr, ok := inst.(dataprovider.FunctionRunner); ok {
		liveFunctions, err := fr.Functions(ctx)
		if err != nil {
			return fmt.Errorf("catalog: introspecting functions for %q: %w", datasourceName, err)
		}
		functionsCol, err := app.FindCachedCollectionByNameOrId("functions")
		if err != nil {
			return err
		}
		if err := reconcileFunctions(app, functionsCol, dsRec, liveFunctions); err != nil {
			return fmt.Errorf("catalog: reconciling functions for %q: %w", datasourceName, err)
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
			colRec.Set("references_table", c.ReferencesTable)
			colRec.Set("references_column", c.ReferencesColumn)
			if err := app.Save(colRec); err != nil {
				return fmt.Errorf("creating column %q: %w", c.Name, err)
			}
			continue
		}

		// Only introspected fields (type, references_table/column) are
		// ever refreshed on an existing row - description is hand-authored
		// and must never be touched here.
		if colRec.GetString("type") != c.Type ||
			colRec.GetString("references_table") != c.ReferencesTable ||
			colRec.GetString("references_column") != c.ReferencesColumn {
			colRec.Set("type", c.Type)
			colRec.Set("references_table", c.ReferencesTable)
			colRec.Set("references_column", c.ReferencesColumn)
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

// reconcileTableStats computes row_count/sample_rows/last_updated fresh
// from inst's own Query method and sets them on tableRec (the caller
// saves) - always overwritten, the same "refresh, never curated" treatment
// reconcileColumns already gives the introspected type/references_*
// fields. Deliberately provider-agnostic: every dataprovider.Instance
// already implements Query, so no per-provider introspection is needed
// here, unlike references_table/references_column. tableName/liveColumns
// come straight from inst.Catalog()'s own just-returned result for this
// same instance, not caller input, so interpolating tableName into SQL
// text below carries the same trust level reconcileColumns already relies
// on for column names.
func reconcileTableStats(ctx context.Context, inst dataprovider.Instance, tableRec *core.Record, tableName string, liveColumns []dataprovider.ColumnCatalog) error {
	countResult, err := inst.Query(ctx, fmt.Sprintf(`SELECT COUNT(*) AS n FROM "%s"`, tableName))
	if err != nil {
		return fmt.Errorf("counting rows: %w", err)
	}
	tableRec.Set("row_count", firstCellInt(countResult))

	sampleResult, err := inst.Query(ctx, fmt.Sprintf(`SELECT * FROM "%s" LIMIT 5`, tableName))
	if err != nil {
		return fmt.Errorf("sampling rows: %w", err)
	}
	tableRec.Set("sample_rows", queryResultToRows(sampleResult))

	// Best-effort only: only tables with a column literally named
	// "updated" (PocketBase's own autodate convention, true for every
	// current Valorant table) get a computed freshness value - there's no
	// universal way to know which column represents "last modified" for an
	// arbitrary onboarded schema.
	if !hasColumn(liveColumns, "updated") {
		tableRec.Set("last_updated", "")
		return nil
	}
	freshResult, err := inst.Query(ctx, fmt.Sprintf(`SELECT MAX("updated") AS m FROM "%s"`, tableName))
	if err != nil {
		return fmt.Errorf("checking freshness: %w", err)
	}
	if len(freshResult.Rows) > 0 && len(freshResult.Rows[0]) > 0 {
		if v, ok := freshResult.Rows[0][0].(string); ok {
			tableRec.Set("last_updated", v)
		}
	}
	return nil
}

// firstCellInt reads a single-row single-column numeric result (e.g.
// SELECT COUNT(*)) as an int - the driver-returned type varies (int64 for
// a direct in-process sqlite connection, float64 once a value has round-
// tripped through JSON via internal/providers/httpsql), so both are
// handled rather than assuming one.
func firstCellInt(result ai.QueryResult) int {
	if len(result.Rows) == 0 || len(result.Rows[0]) == 0 {
		return 0
	}
	switch v := result.Rows[0][0].(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func hasColumn(columns []dataprovider.ColumnCatalog, name string) bool {
	for _, c := range columns {
		if c.Name == name {
			return true
		}
	}
	return false
}

// reconcileFunctions upserts the functions catalog for dsRec, mirroring
// reconcileColumns' upsert/delete-by-name structure, with one deliberate
// difference: description/args always mirror the live source
// unconditionally (no change-detection before Save, unlike
// reconcileColumns' scalar-type check) - diffing a nested args JSON blob
// isn't worth the complexity, and the only cost is functions.updated
// bumping on an otherwise-idle reconcile pass, which nothing depends on.
// query_guidance is never touched here, same protection tables/columns get.
func reconcileFunctions(app core.App, functionsCol *core.Collection, dsRec *core.Record, live []dataprovider.FunctionCatalog) error {
	existing, err := app.FindRecordsByFilter("functions", "datasource = {:ds}", "", 0, 0, dbx.Params{"ds": dsRec.Id})
	if err != nil {
		return err
	}
	existingByName := make(map[string]*core.Record, len(existing))
	for _, f := range existing {
		existingByName[f.GetString("name")] = f
	}

	liveNames := make(map[string]bool, len(live))
	for _, f := range live {
		liveNames[f.Name] = true

		rec, ok := existingByName[f.Name]
		if !ok {
			rec = core.NewRecord(functionsCol)
			rec.Set("datasource", dsRec.Id)
			rec.Set("name", f.Name)
		}
		rec.Set("description", f.Description)
		rec.Set("args", f.Args)
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("saving function %q: %w", f.Name, err)
		}
	}

	for name, rec := range existingByName {
		if !liveNames[name] {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("deleting stale function %q: %w", name, err)
			}
		}
	}

	return nil
}
