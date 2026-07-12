// Package ai is a provider-agnostic abstraction for answering statistical
// questions about cached data. It has no knowledge of what the data
// actually is (Valorant matches, chess.com games, or anything else) -
// callers describe each table's schema and, optionally, a generic "update"
// action that can refresh that table from upstream before it's queried.
// The AI is asked once for a plan (a SQL query plus which updates to run
// first), the caller executes that plan, and an optional second AI call
// interprets the raw results into a plain-language answer.
package ai

import "context"

// ColumnDescription and TableDescription are defined in schema.go, since
// BuildSchema (live PocketBase introspection) is their primary producer
// today. Nothing here assumes that - TableSpec just embeds whatever
// []TableDescription a caller supplies, so a future data-exploration step
// could produce the same shape without touching this file.

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

// UpdateArg documents one argument a TableUpdater's Run accepts.
type UpdateArg struct {
	Name        string
	Type        string // a JSON-schema type, e.g. "string" | "integer" | "boolean"
	Description string
	Required    bool
}

// UpdateOutcome reports what an UpdateFunc call actually did, in a form
// suitable for feeding back to the model (and logging) - not returned to
// the original HTTP caller.
type UpdateOutcome struct {
	Summary string
}

// UpdateFunc refreshes a table's data from upstream, using args the model
// supplied (matching that table's declared UpdateArgs). What it actually
// does is entirely up to the caller that constructed it - the ai package
// never inspects args itself.
type UpdateFunc func(ctx context.Context, args map[string]any) (UpdateOutcome, error)

// TableUpdater is a named, generic "refresh this table" action attached to
// a table. The ai package treats it as an opaque callable described by
// Description/Args for the model's benefit.
type TableUpdater struct {
	// Table must match a TableDescription.Name.
	Table       string
	Description string
	Args        []UpdateArg
	Run         UpdateFunc
}

// TableSpec is what the AI actually sees for one table: its full schema
// description plus an optional updater.
type TableSpec struct {
	TableDescription
	Updater *TableUpdater // nil if this table has no way to refresh itself
}

// BuildTableSpecs merges table descriptions with updaters by table name.
// Tables with no matching updater are left with a nil Updater - the AI can
// still query them, just not request a refresh.
func BuildTableSpecs(tables []TableDescription, updaters []TableUpdater) []TableSpec {
	byTable := make(map[string]TableUpdater, len(updaters))
	for _, u := range updaters {
		byTable[u.Table] = u
	}

	specs := make([]TableSpec, len(tables))
	for i, t := range tables {
		specs[i] = TableSpec{TableDescription: t}
		if u, ok := byTable[t.Name]; ok {
			uu := u
			specs[i].Updater = &uu
		}
	}
	return specs
}

// Request bundles everything a Provider needs to answer a question: the
// question itself, the tables it can query (and optionally refresh), and
// the query tool to fetch data with.
type Request struct {
	Question string
	Tables   []TableSpec
	Query    QueryFunc
}

type Response struct {
	Answer string
}

// Provider is implemented by whichever AI backend is eventually chosen.
type Provider interface {
	Answer(ctx context.Context, req Request) (Response, error)
}
