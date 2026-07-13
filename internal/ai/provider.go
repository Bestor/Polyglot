// Package ai provides read-only SQL execution over the cached data, for
// consumption by the polyglot Data API (internal/polyglot). It has no
// domain or provider knowledge of its own - schema description lives in
// internal/dataprovider/internal/polyglot, and reasoning about questions
// happens in an external MCP client, not in this package.
package ai

import "context"

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
