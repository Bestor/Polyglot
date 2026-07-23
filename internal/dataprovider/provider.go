// Package dataprovider defines the generic plugin interface polyglot
// (internal/polyglot) uses to host multiple onboarded datasources at
// runtime, without knowing anything about any specific one. Every v1
// implementation (internal/providers/sqlite, internal/providers/httpsql) is
// purely connect-style: New dials/opens the real thing immediately and
// returns a live Instance. There is deliberately no PocketBase app handle
// or shared-collection-creation mechanism anywhere in this interface - the
// one domain that used to need that (Valorant) now runs as its own
// standalone service (cmd/valorantapi) with its own PocketBase, reached
// from here over the network like any other http_sql datasource.
package dataprovider

import (
	"context"

	"val-analyzer/internal/ai"
)

// ConfigField self-describes one entry a Provider's config map accepts,
// e.g. a base URL or an API key. Secret marks a field whose persisted
// value becomes a vault path reference, never the literal value.
type ConfigField struct {
	Name        string
	Type        string // a JSON-schema type, e.g. "string" | "integer" | "boolean"
	Description string
	Required    bool
	Secret      bool
}

// Provider is a compiled-in, self-describing connector type, e.g.
// "sqlite" or "http_sql". Its config shape (ConfigSchema) is static,
// independent of any particular config value, so it can be inspected
// before onboarding (e.g. GET /datasources's "available types" listing).
type Provider interface {
	// Type is this provider's stable slug, used as the registry key.
	Type() string
	ConfigSchema() []ConfigField
	// New validates config and returns a fully live, ready-to-use
	// Instance - real I/O (dialing/pinging/opening a file) is expected
	// here; this is onboarding's actual validation step. config always
	// contains real resolved values, never a vault ref - Registry
	// resolves references before calling New.
	New(ctx context.Context, config map[string]any) (Instance, error)
}

// ColumnCatalog describes one column's live, introspected shape.
type ColumnCatalog struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// TableCatalog describes one table's live, introspected shape.
type TableCatalog struct {
	Name    string          `json:"name"`
	Columns []ColumnCatalog `json:"columns"`
}

// Instance is a live, configured connection to one onboarded datasource.
type Instance interface {
	// Catalog returns live ground truth by introspecting the real source.
	// Only the async reconcile job (internal/polyglot/catalog.go) calls
	// this directly - never the GET /metadata hot path.
	Catalog(ctx context.Context) ([]TableCatalog, error)
	// Query runs one read-only ANSI SQL statement against this instance's
	// own connection.
	Query(ctx context.Context, sqlText string) (ai.QueryResult, error)
	// Close releases any resources (open file handles, connections) this
	// instance holds. Called when an onboarded datasource is replaced
	// (re-onboarded under the same name) or the process shuts down.
	Close() error
}

// RowSampler is an optional capability a Provider's Instance may implement
// for human curation UX (sampling a few rows to help write a description) -
// not required by Instance, since not every provider needs it.
type RowSampler interface {
	SampleRows(ctx context.Context, table string, n int) ([]map[string]any, error)
}
