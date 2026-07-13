package polyglot

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
)

type ColumnDescription struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type TableDescription struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Datasource  string              `json:"datasource"`
	Columns     []ColumnDescription `json:"columns"`
}

type FunctionArg struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type FunctionDescription struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Datasource  string        `json:"datasource"`
	Args        []FunctionArg `json:"args"`
}

type MetadataResponse struct {
	Tables    []TableDescription    `json:"tables"`
	Functions []FunctionDescription `json:"functions"`
}

// handleMetadata implements GET /metadata: describes every active
// datasource's tables and functions, merged into one response and each
// tagged with its owning datasource. Built fresh per request (not cached
// at boot) since the schema can change at runtime via POST /datasources.
func handleMetadata(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		slog.Info("polyglot: metadata requested")
		metadata, err := buildMetadata(e.App, reg.ActiveInstances())
		if err != nil {
			return e.InternalServerError("failed to build metadata", err)
		}
		return e.JSON(http.StatusOK, metadata)
	}
}

func buildMetadata(app core.App, active []ActiveInstance) (MetadataResponse, error) {
	var resp MetadataResponse
	for _, a := range active {
		described, err := buildTableDescriptions(app, a.Tables)
		if err != nil {
			return MetadataResponse{}, err
		}
		for _, t := range described {
			t.Datasource = a.Type
			resp.Tables = append(resp.Tables, t)
		}
		for _, f := range a.Functions {
			resp.Functions = append(resp.Functions, FunctionDescription{
				Name:        f.Name,
				Description: f.Description,
				Datasource:  a.Type,
				Args:        toFunctionArgs(f.Args),
			})
		}
	}
	return resp, nil
}

// buildTableDescriptions introspects the live PocketBase collections for
// specs (so structure/types can never drift from what's actually there)
// and merges in each field's hand-authored Description. Uses
// FindCachedCollectionByNameOrId since this now runs on every /metadata
// request rather than once at boot.
func buildTableDescriptions(app core.App, specs []dataprovider.TableSpec) ([]TableDescription, error) {
	result := make([]TableDescription, 0, len(specs))
	for _, spec := range specs {
		col, err := app.FindCachedCollectionByNameOrId(spec.Name)
		if err != nil {
			return nil, fmt.Errorf("table %q: %w", spec.Name, err)
		}

		notes := make(map[string]string, len(spec.Fields))
		for _, f := range spec.Fields {
			notes[f.Name] = f.Description
		}

		columns := make([]ColumnDescription, 0, len(col.Fields))
		for _, f := range col.Fields {
			columns = append(columns, ColumnDescription{Name: f.GetName(), Type: f.Type(), Description: notes[f.GetName()]})
		}

		result = append(result, TableDescription{Name: col.Name, Description: spec.Description, Columns: columns})
	}
	return result, nil
}

func toFunctionArgs(args []dataprovider.FunctionArg) []FunctionArg {
	out := make([]FunctionArg, len(args))
	for i, a := range args {
		out[i] = FunctionArg{Name: a.Name, Type: a.Type, Required: a.Required, Description: a.Description}
	}
	return out
}
