package polyglot

import (
	"log/slog"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type ColumnDescription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type TableDescription struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Datasource    string              `json:"datasource"`
	QueryGuidance string              `json:"query_guidance"`
	Columns       []ColumnDescription `json:"columns"`
}

type DatasourceGuidance struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	QueryGuidance string `json:"query_guidance"`
}

type MetadataResponse struct {
	Datasources []DatasourceGuidance `json:"datasources"`
	Tables      []TableDescription   `json:"tables"`
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

func buildMetadata(app core.App) (MetadataResponse, error) {
	dsRecords, err := app.FindAllRecords(datasourcesCollection)
	if err != nil {
		return MetadataResponse{}, err
	}

	var resp MetadataResponse
	dsNameByID := make(map[string]string, len(dsRecords))
	for _, ds := range dsRecords {
		dsNameByID[ds.Id] = ds.GetString("name")
		resp.Datasources = append(resp.Datasources, DatasourceGuidance{
			Name:          ds.GetString("name"),
			Description:   ds.GetString("description"),
			QueryGuidance: ds.GetString("query_guidance"),
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
				ID:          c.Id,
				Name:        c.GetString("name"),
				Type:        c.GetString("type"),
				Description: c.GetString("description"),
			})
		}

		resp.Tables = append(resp.Tables, TableDescription{
			ID:            t.Id,
			Name:          t.GetString("name"),
			Description:   t.GetString("description"),
			Datasource:    dsNameByID[t.GetString("datasource")],
			QueryGuidance: t.GetString("query_guidance"),
			Columns:       columns,
		})
	}

	return resp, nil
}
