// Package dataprovider defines the generic plugin interface polyglot
// (internal/polyglot) uses to host multiple data sources (Valorant, a
// hypothetical NFL provider, chess.com, ...) at runtime, without knowing
// anything about any specific domain. Concrete providers live in their own
// internal/providers/<name> package and are registered by cmd/polyglot at
// compile time (see internal/polyglot.Registry).
package dataprovider

import "github.com/pocketbase/pocketbase/core"

// ConfigField self-describes one entry a Provider's config map accepts,
// e.g. an API key. Secret marks a field that must never be echoed back in
// an onboarding API response or logged.
type ConfigField struct {
	Name        string
	Type        string // a JSON-schema type, e.g. "string" | "integer" | "boolean"
	Description string
	Required    bool
	Secret      bool
}

// Provider is a compiled-in, self-describing data source type. Its schema
// (Tables) and config shape (ConfigSchema) are static, independent of any
// particular config value, so both can be inspected before onboarding
// (e.g. GET /datasources's "available types" listing).
type Provider interface {
	// Type is this provider's stable slug (e.g. "valorant"), used as the
	// registry key and, for now, the datasource id - one active instance
	// per provider type.
	Type() string
	ConfigSchema() []ConfigField
	// Tables describes every PocketBase collection this provider needs.
	// Order matters: a table with a FieldSpec of Type FieldRelation must
	// appear after the table(s) it relates to, since dynamic creation
	// resolves RelationTable to a live CollectionId while creating tables
	// in this order (mirrors how internal/migrations is numbered today).
	Tables() []TableSpec
	// New validates config and constructs a configured-but-not-yet-bound
	// Instance. New must be side-effect-free: no PocketBase access, no
	// network I/O - it only builds in-memory clients/config. polyglot
	// ensures every TableSpec in Tables() exists as a collection next,
	// then calls Instance.Bind.
	New(config map[string]any) (Instance, error)
}

// Instance is a configured, live provider bound to a running PocketBase
// app. polyglot never reuses an Instance across re-onboarding: each
// onboarding call constructs and binds a fresh one via Provider.New+Bind.
type Instance interface {
	// Bind constructs whatever store/service layer this instance needs
	// against app. Called exactly once, immediately after New, once every
	// TableSpec in the owning Provider's Tables() exists as a collection.
	Bind(app core.App) error
	// Functions returns this instance's warm actions. Only valid after Bind.
	Functions() []Function
}
