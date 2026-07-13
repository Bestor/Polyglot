// Package ai provides schema introspection and read-only SQL execution
// over the cached data, for consumption by the polyglot Data API
// (internal/polyglot) - reasoning about questions happens in an external
// MCP client, not in this package.
package ai

import "context"

// ColumnDescription and TableDescription are defined in schema.go, since
// BuildSchema (live PocketBase introspection) is their primary producer.

// QueryResult is a flat, driver-agnostic tabular result set.
type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	// Truncated is true if the executor's row safety cap cut off further
	// results - see maxQueryRows in query.go.
	Truncated bool `json:"truncated"`
}

// QueryFunc executes a single read-only SQL statement against the cached
// data. Implementations must reject anything other than SELECT/WITH
// statements.
type QueryFunc func(ctx context.Context, sql string) (QueryResult, error)

// UpdateArg documents one argument a polyglot Function's Run accepts - see
// internal/polyglot.Function.
type UpdateArg struct {
	Name        string
	Type        string // a JSON-schema type, e.g. "string" | "integer" | "boolean"
	Description string
	Required    bool
}
