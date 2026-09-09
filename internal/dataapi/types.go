// Package dataapi is the generic serving harness shared by every standalone
// Data API binary (cmd/valorantapi, cmd/chesscomapi, ...): the GET /query,
// GET /schema, GET /functions, POST /warm, GET /warm handlers, and the
// Function shape those endpoints operate on. None of it knows anything
// about any particular domain - a binary's own package (internal/valorant,
// internal/chesscom, ...) supplies the actual Function list, migrations,
// and ingest logic; this package only hosts them over HTTP.
package dataapi

import "context"

// FunctionArg documents one argument a Function's Run accepts.
type FunctionArg struct {
	Name        string
	Type        string // a JSON-schema type, e.g. "string" | "integer" | "boolean"
	Description string
	Required    bool
}

// FunctionOutcome reports what a Function's Run call actually did, in a
// form suitable for feeding back to the caller (and logging).
type FunctionOutcome struct {
	Summary string
	Data    map[string]any
}

// FunctionRun runs one named data-fill action, using args the caller
// supplied (matching that Function's declared Args).
type FunctionRun func(ctx context.Context, args map[string]any) (FunctionOutcome, error)

// Function is a named, generically-invokable data-fill action, callable via
// POST /warm and self-described via GET /warm's function listing.
type Function struct {
	Name        string
	Description string
	Args        []FunctionArg
	Run         FunctionRun
}
