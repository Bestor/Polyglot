// Package ai is a provider-agnostic abstraction for answering statistical
// questions about cached Valorant data. Rather than pre-curating the
// specific fields a question might need, it hands the AI the real database
// schema and a safe read-only SQL query tool, and lets the model decide
// what data to fetch and how to interpret it. This stays generic across
// arbitrary questions without us hand-modeling each shape of question.
package ai

import "context"

// QueryResult is a flat, driver-agnostic tabular result set.
type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// QueryFunc executes a single read-only SQL statement against the cached
// data. Implementations must reject anything other than SELECT/WITH
// statements.
type QueryFunc func(ctx context.Context, sql string) (QueryResult, error)

// SyncOutcome reports what a SyncFunc call actually did.
type SyncOutcome struct {
	Fetched int `json:"fetched"`
	Skipped int `json:"skipped"`
}

// SyncFunc fetches and caches up to count additional matches for the
// player identified by puuid. It lets a Provider decide, after inspecting
// what's already cached via Query, that it needs more data before it can
// answer - e.g. a question about "the last 10 matches" when only 5 are
// cached. May be nil if the caller didn't wire up sync capability.
type SyncFunc func(ctx context.Context, puuid string, count int) (SyncOutcome, error)

// Request bundles everything a Provider needs to answer a question: the
// question itself, the full schema it can query against, free-form hints
// about the request (e.g. resolved player identities), the query tool to
// fetch data with, and an optional tool to sync more data on demand.
type Request struct {
	Question string
	Schema   []TableDescription
	Hints    []string
	Query    QueryFunc
	SyncMore SyncFunc
}

type Response struct {
	Answer string
}

// Provider is implemented by whichever AI backend is eventually chosen. A
// real implementation would typically use Schema and Query in a
// tool-calling loop: propose SQL, run it via Query, feed the rows back, and
// either issue another query or return a final Response - the interface
// supports single-shot or multi-round use without changing shape.
type Provider interface {
	Answer(ctx context.Context, req Request) (Response, error)
}
