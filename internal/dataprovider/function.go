package dataprovider

import "context"

// FunctionArg documents one argument a Function's Run accepts.
type FunctionArg struct {
	Name        string
	Type        string // a JSON-schema type, e.g. "string" | "integer" | "boolean"
	Description string
	Required    bool
}

// FunctionOutcome reports what a FunctionFunc call actually did, in a form
// suitable for feeding back to the caller (and logging).
type FunctionOutcome struct {
	Summary string
	Data    map[string]any
}

// FunctionFunc runs one named data-fill action, using args the caller
// supplied (matching that Function's declared Args).
type FunctionFunc func(ctx context.Context, args map[string]any) (FunctionOutcome, error)

// Function is a named, generically-invokable data-fill action a provider
// exposes, callable via POST /warm and self-described via GET /metadata.
type Function struct {
	Name        string
	Description string
	Args        []FunctionArg
	Run         FunctionFunc
}
