package polyglot

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// GlossaryEntry/ExampleQuery back both the curated datasource/table
// glossary and example_queries fields - see internal/migrations'
// curation-fields migration and POST /datasources/annotate,
// POST /tables/annotate.
type GlossaryEntry struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

type ExampleQuery struct {
	Question string `json:"question"`
	SQL      string `json:"sql"`
}

type ColumnDescription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	// ReferencesTable/ReferencesColumn are introspected (refreshed every
	// reconcile, see catalog.go's reconcileColumns), not curated - the
	// target of a foreign-key-like relation, empty when none is
	// mechanically known.
	ReferencesTable  string `json:"references_table,omitempty"`
	ReferencesColumn string `json:"references_column,omitempty"`
}

type TableDescription struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Datasource    string `json:"datasource"`
	QueryGuidance string `json:"query_guidance"`
	// GoodFor/BadFor/KnownGaps/ExampleQueries are curated - only ever
	// change via POST /tables/annotate, reconcile never touches them.
	GoodFor        string         `json:"good_for"`
	BadFor         string         `json:"bad_for"`
	KnownGaps      string         `json:"known_gaps"`
	ExampleQueries []ExampleQuery `json:"example_queries"`
	// RowCount/SampleRows/LastUpdated are introspected - computed fresh by
	// catalog.go's reconcileTableStats on every reconcile pass, from the
	// datasource's own live data, not hand-authored. LastUpdated is
	// best-effort (empty if the table has no "updated" column) - see
	// reconcileTableStats's own doc comment.
	RowCount    int                 `json:"row_count"`
	SampleRows  []map[string]any    `json:"sample_rows"`
	LastUpdated string              `json:"last_updated,omitempty"`
	Columns     []ColumnDescription `json:"columns"`
}

type DatasourceGuidance struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	QueryGuidance string `json:"query_guidance"`
	// Glossary/ExampleQueries are curated, connection-level - glossary
	// terms (e.g. "ACS", "ADR") and cookbook queries that span many tables
	// live here rather than being repeated per-table.
	Glossary       []GlossaryEntry `json:"glossary"`
	ExampleQueries []ExampleQuery  `json:"example_queries"`
}

type FunctionArgDescription struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type FunctionDescription struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Datasource    string                   `json:"datasource"`
	QueryGuidance string                   `json:"query_guidance"`
	Args          []FunctionArgDescription `json:"args"`
}

type MetadataResponse struct {
	Datasources []DatasourceGuidance  `json:"datasources"`
	Tables      []TableDescription    `json:"tables"`
	Functions   []FunctionDescription `json:"functions"`
}

// handleMetadata implements GET /metadata: describes every onboarded
// datasource plus its tables/columns, merged into one response. Built
// fresh per request from the persisted tables/columns/datasources
// snapshot (internal/polyglot/catalog.go's reconcileCatalog is what keeps
// that snapshot current) - deliberately never a live Instance.Catalog()
// call, so this endpoint's latency stays independent of any one
// datasource's health/speed, even a slow or temporarily-unreachable
// network one.
func handleMetadata() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		slog.Info("polyglot: metadata requested")
		metadata, err := buildMetadata(e.App)
		if err != nil {
			return e.InternalServerError("failed to build metadata", err)
		}
		return e.JSON(http.StatusOK, metadata)
	}
}

// unmarshalJSONArray reads a JSONField into a slice, normalizing a stored
// JSON null (or a field that was never Set at all, e.g. a fresh record's
// glossary/example_queries before its first annotation) to a non-nil empty
// slice. encoding/json sets a slice to nil on a null input regardless of
// its prior value, and a nil slice re-serializes as JSON null rather than
// [] - every consumer of GET /metadata (webui, any MCP client) reasonably
// expects an array-typed field to always be an array.
func unmarshalJSONArray[T any](rec *core.Record, field string) ([]T, error) {
	var v []T
	if err := rec.UnmarshalJSONField(field, &v); err != nil {
		return nil, err
	}
	if v == nil {
		v = []T{}
	}
	return v, nil
}

func buildMetadata(app core.App) (MetadataResponse, error) {
	dsRecords, err := app.FindAllRecords(datasourcesCollection)
	if err != nil {
		return MetadataResponse{}, err
	}

	var resp MetadataResponse
	dsNameByID := make(map[string]string, len(dsRecords))
	for _, ds := range dsRecords {
		dsNameByID[ds.Id] = ds.GetString("name")

		glossary, err := unmarshalJSONArray[GlossaryEntry](ds, "glossary")
		if err != nil {
			return MetadataResponse{}, err
		}
		dsExampleQueries, err := unmarshalJSONArray[ExampleQuery](ds, "example_queries")
		if err != nil {
			return MetadataResponse{}, err
		}

		resp.Datasources = append(resp.Datasources, DatasourceGuidance{
			Name:           ds.GetString("name"),
			Description:    ds.GetString("description"),
			QueryGuidance:  ds.GetString("query_guidance"),
			Glossary:       glossary,
			ExampleQueries: dsExampleQueries,
		})
	}

	tableRecords, err := app.FindAllRecords("tables")
	if err != nil {
		return MetadataResponse{}, err
	}

	for _, t := range tableRecords {
		columnRecords, err := app.FindRecordsByFilter("columns", "table = {:table}", "name", 0, 0, dbx.Params{"table": t.Id})
		if err != nil {
			return MetadataResponse{}, err
		}
		columns := make([]ColumnDescription, 0, len(columnRecords))
		for _, c := range columnRecords {
			columns = append(columns, ColumnDescription{
				ID:               c.Id,
				Name:             c.GetString("name"),
				Type:             c.GetString("type"),
				Description:      c.GetString("description"),
				ReferencesTable:  c.GetString("references_table"),
				ReferencesColumn: c.GetString("references_column"),
			})
		}

		tableExampleQueries, err := unmarshalJSONArray[ExampleQuery](t, "example_queries")
		if err != nil {
			return MetadataResponse{}, err
		}
		sampleRows, err := unmarshalJSONArray[map[string]any](t, "sample_rows")
		if err != nil {
			return MetadataResponse{}, err
		}

		lastUpdated := ""
		if dt := t.GetDateTime("last_updated").Time(); !dt.IsZero() {
			lastUpdated = dt.UTC().Format(time.RFC3339)
		}

		resp.Tables = append(resp.Tables, TableDescription{
			ID:             t.Id,
			Name:           t.GetString("name"),
			Description:    t.GetString("description"),
			Datasource:     dsNameByID[t.GetString("datasource")],
			QueryGuidance:  t.GetString("query_guidance"),
			GoodFor:        t.GetString("good_for"),
			BadFor:         t.GetString("bad_for"),
			KnownGaps:      t.GetString("known_gaps"),
			ExampleQueries: tableExampleQueries,
			RowCount:       t.GetInt("row_count"),
			SampleRows:     sampleRows,
			LastUpdated:    lastUpdated,
			Columns:        columns,
		})
	}

	functionRecords, err := app.FindAllRecords("functions")
	if err != nil {
		return MetadataResponse{}, err
	}
	for _, f := range functionRecords {
		args, err := unmarshalJSONArray[FunctionArgDescription](f, "args")
		if err != nil {
			return MetadataResponse{}, err
		}
		resp.Functions = append(resp.Functions, FunctionDescription{
			ID:            f.Id,
			Name:          f.GetString("name"),
			Description:   f.GetString("description"),
			Datasource:    dsNameByID[f.GetString("datasource")],
			QueryGuidance: f.GetString("query_guidance"),
			Args:          args,
		})
	}

	return resp, nil
}
