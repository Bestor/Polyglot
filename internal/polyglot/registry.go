package polyglot

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
)

// datasourcesCollection is polyglot's own bookkeeping collection (see
// internal/migrations/1750000018_datasources.go). reservedOwner pre-claims
// it in tableOwner so no provider can ever declare a TableSpec with this
// name and shadow it - reuses the same collision-rejection path as a
// genuine cross-provider table-name collision.
const (
	datasourcesCollection = "datasources"
	reservedOwner         = "__polyglot__"
)

var (
	errUnknownProviderType = errors.New("unknown provider type")
	errInvalidConfig       = errors.New("invalid config")
	errTableCollision      = errors.New("table already owned by another datasource")
)

// ActiveInstance bundles one active datasource's identity with its
// provider-declared schema and instance-declared functions.
type ActiveInstance struct {
	Type      string
	Tables    []dataprovider.TableSpec
	Functions []dataprovider.Function
}

// Registry tracks compiled-in provider types and the currently active
// datasource instances built from them - one active instance per provider
// type (datasource id == provider Type()).
type Registry struct {
	mu         sync.RWMutex
	providers  map[string]dataprovider.Provider
	instances  map[string]dataprovider.Instance
	configs    map[string]map[string]any
	tableOwner map[string]string
}

func NewRegistry(providers map[string]dataprovider.Provider) *Registry {
	return &Registry{
		providers:  providers,
		instances:  map[string]dataprovider.Instance{},
		configs:    map[string]map[string]any{},
		tableOwner: map[string]string{datasourcesCollection: reservedOwner},
	}
}

// Rehydrate re-onboards every datasource persisted in the datasources
// collection (e.g. after a process restart). Must run after migrations
// and before RegisterRoutes. A single bad row (e.g. its provider type was
// removed from the binary) is logged and skipped rather than failing the
// whole boot.
func (r *Registry) Rehydrate(app core.App) error {
	records, err := app.FindAllRecords(datasourcesCollection)
	if err != nil {
		return err
	}
	for _, rec := range records {
		dsType := rec.GetString("type")
		var config map[string]any
		if err := rec.UnmarshalJSONField("config", &config); err != nil {
			slog.Error("polyglot: skipping datasource with unreadable config", "type", dsType, "error", err)
			continue
		}
		if _, err := r.Onboard(app, dsType, config); err != nil {
			slog.Error("polyglot: failed to rehydrate datasource", "type", dsType, "error", err)
			continue
		}
		slog.Info("polyglot: rehydrated datasource", "type", dsType)
	}
	return nil
}

// Onboard validates config, ensures the provider's tables exist (creating
// any that are missing), binds a fresh Instance, persists the
// registration, and activates it - replacing any previously active
// instance of the same type (an idempotent upsert: re-onboarding an
// already-active type updates its stored config and rebinds).
func (r *Registry) Onboard(app core.App, dsType string, config map[string]any) (OnboardResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	provider, ok := r.providers[dsType]
	if !ok {
		return OnboardResponse{}, fmt.Errorf("%w: %q", errUnknownProviderType, dsType)
	}
	if err := validateConfig(provider.ConfigSchema(), config); err != nil {
		return OnboardResponse{}, fmt.Errorf("%w: %v", errInvalidConfig, err)
	}
	instance, err := provider.New(config)
	if err != nil {
		return OnboardResponse{}, fmt.Errorf("%w: %v", errInvalidConfig, err)
	}

	tables := provider.Tables()
	for _, t := range tables {
		if owner, claimed := r.tableOwner[t.Name]; claimed && owner != dsType {
			return OnboardResponse{}, fmt.Errorf("%w: table %q is owned by datasource %q", errTableCollision, t.Name, owner)
		}
	}
	for _, t := range tables {
		if err := ensureTable(app, t); err != nil {
			return OnboardResponse{}, fmt.Errorf("ensuring table %q: %w", t.Name, err)
		}
		r.tableOwner[t.Name] = dsType
	}

	if err := instance.Bind(app); err != nil {
		return OnboardResponse{}, fmt.Errorf("binding datasource %q: %w", dsType, err)
	}
	if err := persistDatasource(app, dsType, config); err != nil {
		return OnboardResponse{}, fmt.Errorf("persisting datasource %q: %w", dsType, err)
	}

	r.instances[dsType] = instance
	r.configs[dsType] = config

	return OnboardResponse{
		Type:      dsType,
		Tables:    tableNames(tables),
		Functions: functionNames(instance.Functions()),
		Config:    maskSecrets(provider.ConfigSchema(), config),
	}, nil
}

func (r *Registry) Instance(dsType string) (dataprovider.Instance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[dsType]
	return inst, ok
}

// ActiveInstances returns every active datasource, sorted by Type for
// deterministic output.
func (r *Registry) ActiveInstances() []ActiveInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ActiveInstance, 0, len(r.instances))
	for dsType, inst := range r.instances {
		out = append(out, ActiveInstance{Type: dsType, Tables: r.providers[dsType].Tables(), Functions: inst.Functions()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// List describes every compiled-in provider type plus every active
// datasource, for GET /datasources.
func (r *Registry) List() DatasourcesResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	sort.Strings(types)

	available := make([]ProviderTypeDescription, 0, len(types))
	for _, t := range types {
		p := r.providers[t]
		cfg := make([]ConfigFieldDescription, 0, len(p.ConfigSchema()))
		for _, f := range p.ConfigSchema() {
			cfg = append(cfg, ConfigFieldDescription{Name: f.Name, Type: f.Type, Description: f.Description, Required: f.Required, Secret: f.Secret})
		}
		available = append(available, ProviderTypeDescription{Type: t, Config: cfg})
	}

	active := make([]ActiveDatasourceDescription, 0, len(r.instances))
	for dsType, inst := range r.instances {
		active = append(active, ActiveDatasourceDescription{
			Type:      dsType,
			Tables:    tableNames(r.providers[dsType].Tables()),
			Functions: functionNames(inst.Functions()),
		})
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Type < active[j].Type })

	return DatasourcesResponse{AvailableTypes: available, Active: active}
}

func ensureTable(app core.App, spec dataprovider.TableSpec) error {
	if _, err := app.FindCollectionByNameOrId(spec.Name); err == nil {
		return nil // already exists (e.g. every Valorant table, via its real migration)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	c := core.NewBaseCollection(spec.Name)
	fields := make([]core.Field, 0, len(spec.Fields))
	for _, f := range spec.Fields {
		field, err := f.ToCoreField(app)
		if err != nil {
			return err
		}
		fields = append(fields, field)
	}
	c.Fields.Add(fields...)
	for _, idx := range spec.Indexes {
		c.AddIndex(idx.Name, idx.Unique, strings.Join(idx.Columns, ", "), "")
	}
	return app.Save(c)
}

func persistDatasource(app core.App, dsType string, config map[string]any) error {
	col, err := app.FindCollectionByNameOrId(datasourcesCollection)
	if err != nil {
		return err
	}
	rec, err := app.FindFirstRecordByFilter(datasourcesCollection, "type = {:type}", dbx.Params{"type": dsType})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("type", dsType)
	}
	rec.Set("config", config)
	return app.Save(rec)
}

func validateConfig(schema []dataprovider.ConfigField, config map[string]any) error {
	for _, f := range schema {
		if f.Required {
			if _, ok := config[f.Name]; !ok {
				return fmt.Errorf("missing required config field %q", f.Name)
			}
		}
	}
	return nil
}

func maskSecrets(schema []dataprovider.ConfigField, config map[string]any) map[string]any {
	secret := make(map[string]bool, len(schema))
	for _, f := range schema {
		secret[f.Name] = f.Secret
	}
	masked := make(map[string]any, len(config))
	for k, v := range config {
		if secret[k] {
			masked[k] = "***"
		} else {
			masked[k] = v
		}
	}
	return masked
}

func tableNames(tables []dataprovider.TableSpec) []string {
	out := make([]string, len(tables))
	for i, t := range tables {
		out[i] = t.Name
	}
	return out
}

func functionNames(fns []dataprovider.Function) []string {
	out := make([]string, len(fns))
	for i, f := range fns {
		out[i] = f.Name
	}
	return out
}
