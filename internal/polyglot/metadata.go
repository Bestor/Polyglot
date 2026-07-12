package polyglot

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
)

type ColumnDescription struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type TableDescription struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
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
	Args        []FunctionArg `json:"args"`
}

type MetadataResponse struct {
	Tables    []TableDescription    `json:"tables"`
	Functions []FunctionDescription `json:"functions"`
}

// buildMetadata introspects the live database schema (via ai.BuildSchema,
// so structure/types can never drift from the migrations) and describes
// the given functions, matching GET /metadata's contract in
// openapi/polyglot.yaml.
func buildMetadata(app core.App, functions []Function) (MetadataResponse, error) {
	schema, err := ai.BuildSchema(app)
	if err != nil {
		return MetadataResponse{}, err
	}

	tables := make([]TableDescription, len(schema))
	for i, t := range schema {
		columns := make([]ColumnDescription, len(t.Columns))
		for j, c := range t.Columns {
			columns[j] = ColumnDescription{Name: c.Name, Type: c.Type, Description: c.Description}
		}
		tables[i] = TableDescription{Name: t.Name, Description: t.Description, Columns: columns}
	}

	functionDescriptions := make([]FunctionDescription, len(functions))
	for i, f := range functions {
		args := make([]FunctionArg, len(f.Args))
		for j, a := range f.Args {
			args[j] = FunctionArg{Name: a.Name, Type: a.Type, Required: a.Required, Description: a.Description}
		}
		functionDescriptions[i] = FunctionDescription{Name: f.Name, Description: f.Description, Args: args}
	}

	return MetadataResponse{Tables: tables, Functions: functionDescriptions}, nil
}

// handleMetadata serves the metadata built once at route-registration time
// (post-migration) - the schema only changes when collections change
// during a boot's migrations, not per-request.
func handleMetadata(metadata MetadataResponse) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, metadata)
	}
}
