package polyglot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/jobstore"
)

// datasourcesCollection is polyglot's own bookkeeping collection (see
// internal/migrations/1750000018_datasources.go/1750000020_datasources_name.go).
// reservedNames pre-claims it (and the catalog collections it links to)
// so no onboarded datasource can ever be given one of these names.
const datasourcesCollection = "datasources"

var reservedNames = map[string]bool{"datasources": true, "tables": true, "columns": true}

var (
	errUnknownProviderType = errors.New("unknown provider type")
	errInvalidConfig       = errors.New("invalid config")
	errReservedName        = errors.New("name is reserved")
	errUnknownDatasource   = errors.New("unknown datasource")
)

// reconcileTimeout bounds how long a single background catalog-reconcile
// pass may take. Generous since Instance.Catalog can mean a real network
// round trip (internal/providers/httpsql) to a service that itself has to
// introspect PocketBase collections.
const reconcileTimeout = 5 * time.Minute

// Registry tracks compiled-in provider types and the currently active
// datasource instances built from them - keyed by a user-chosen name
// (§10), not Provider.Type(), since multiple datasources can share one
// provider type (e.g. two onboarded SQLite files).
type Registry struct {
	mu        sync.RWMutex
	providers map[string]dataprovider.Provider // keyed by Type() - immutable after NewRegistry, safe to read unlocked
	instances map[string]dataprovider.Instance // keyed by name
	types     map[string]string                // keyed by name -> provider type, for GET /datasources
	configs   map[string]map[string]any        // keyed by name -> real (resolved) config
	vc        vaultClient
	jobs      *jobstore.Store
}

func NewRegistry(providers map[string]dataprovider.Provider, vc vaultClient, jobs *jobstore.Store) *Registry {
	return &Registry{
		providers: providers,
		instances: map[string]dataprovider.Instance{},
		types:     map[string]string{},
		configs:   map[string]map[string]any{},
		vc:        vc,
		jobs:      jobs,
	}
}

// Rehydrate re-onboards every datasource persisted in the datasources
// collection (e.g. after a process restart). Must run after migrations
// and before RegisterRoutes. A single bad row (unknown provider type,
// unreadable config, or a vault resolve failure - e.g. OpenBao still
// sealed) is logged and skipped rather than failing the whole boot, so
// polyglot still serves every other datasource plus GET /metadata/
// GET /query against the persisted catalog.
func (r *Registry) Rehydrate(ctx context.Context, app core.App) error {
	records, err := app.FindAllRecords(datasourcesCollection)
	if err != nil {
		return err
	}
	for _, rec := range records {
		name := rec.GetString("name")
		dsType := rec.GetString("type")

		provider, ok := r.providers[dsType]
		if !ok {
			slog.Error("polyglot: skipping datasource with unknown provider type", "name", name, "type", dsType)
			continue
		}

		var storedConfig map[string]any
		if err := rec.UnmarshalJSONField("config", &storedConfig); err != nil {
			slog.Error("polyglot: skipping datasource with unreadable config", "name", name, "error", err)
			continue
		}

		resolvedConfig, err := ResolveConfig(ctx, r.vc, provider.ConfigSchema(), storedConfig)
		if err != nil {
			slog.Error("polyglot: failed to resolve secrets for datasource, skipping", "name", name, "error", err)
			continue
		}

		if _, err := r.Onboard(ctx, app, name, dsType, resolvedConfig); err != nil {
			slog.Error("polyglot: failed to rehydrate datasource", "name", name, "error", err)
			continue
		}
		slog.Info("polyglot: rehydrated datasource", "name", name)
	}
	return nil
}

// Onboard validates config, constructs a fresh Instance, persists the
// registration (secrets replaced with vault references - see
// PersistConfig), activates it under name - replacing and closing any
// previously active instance of the same name (an idempotent upsert) -
// and kicks off an async catalog-reconcile job.
func (r *Registry) Onboard(ctx context.Context, app core.App, name, providerType string, config map[string]any) (OnboardResponse, error) {
	if reservedNames[name] {
		return OnboardResponse{}, fmt.Errorf("%w: %q", errReservedName, name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	provider, ok := r.providers[providerType]
	if !ok {
		return OnboardResponse{}, fmt.Errorf("%w: %q", errUnknownProviderType, providerType)
	}
	if err := validateConfig(provider.ConfigSchema(), config); err != nil {
		return OnboardResponse{}, fmt.Errorf("%w: %v", errInvalidConfig, err)
	}

	instance, err := provider.New(ctx, config)
	if err != nil {
		return OnboardResponse{}, fmt.Errorf("%w: %v", errInvalidConfig, err)
	}

	refConfig, err := PersistConfig(ctx, r.vc, name, provider.ConfigSchema(), config)
	if err != nil {
		instance.Close()
		return OnboardResponse{}, fmt.Errorf("persisting secrets for %q: %w", name, err)
	}
	if err := persistDatasource(app, name, providerType, refConfig); err != nil {
		instance.Close()
		return OnboardResponse{}, fmt.Errorf("persisting datasource %q: %w", name, err)
	}

	if old, exists := r.instances[name]; exists {
		if err := old.Close(); err != nil {
			slog.Warn("polyglot: error closing previous instance during re-onboard", "name", name, "error", err)
		}
	}
	r.instances[name] = instance
	r.types[name] = providerType
	r.configs[name] = config

	job := r.startReconcile(app, name, instance)

	return OnboardResponse{
		Name:           name,
		Type:           providerType,
		Config:         refConfig,
		ReconcileJobID: job.ID,
	}, nil
}

// Reconcile re-runs catalog reconciliation for an already-onboarded
// datasource on demand (POST /datasources/reconcile).
func (r *Registry) Reconcile(app core.App, name string) (jobstore.Job, error) {
	r.mu.RLock()
	inst, ok := r.instances[name]
	r.mu.RUnlock()
	if !ok {
		return jobstore.Job{}, fmt.Errorf("%w: %q", errUnknownDatasource, name)
	}
	return r.startReconcile(app, name, inst), nil
}

// startReconcile launches an async reconcile pass and returns immediately
// with the created job. Only touches r.jobs (its own internal locking),
// never r.mu - safe to call both while already holding Registry's lock
// (from Onboard) and without it (from Reconcile).
func (r *Registry) startReconcile(app core.App, name string, inst dataprovider.Instance) jobstore.Job {
	job := r.jobs.Create(name, "reconcile_catalog")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
		defer cancel()

		if err := reconcileCatalog(ctx, app, name, inst); err != nil {
			slog.Error("polyglot: catalog reconcile failed", "datasource", name, "error", err)
			r.jobs.Fail(job.ID, err.Error())
			return
		}
		slog.Info("polyglot: catalog reconcile complete", "datasource", name)
		r.jobs.Complete(job.ID, fmt.Sprintf("reconciled catalog for %q", name), nil)
	}()

	return job
}

// Instance returns the active Instance for name, if any.
func (r *Registry) Instance(name string) (dataprovider.Instance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[name]
	return inst, ok
}

// List describes every compiled-in provider type plus every active
// datasource, for GET /datasources.
func (r *Registry) List() DatasourcesResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerTypes := make([]string, 0, len(r.providers))
	for t := range r.providers {
		providerTypes = append(providerTypes, t)
	}
	sort.Strings(providerTypes)

	available := make([]ProviderTypeDescription, 0, len(providerTypes))
	for _, t := range providerTypes {
		p := r.providers[t]
		cfg := make([]ConfigFieldDescription, 0, len(p.ConfigSchema()))
		for _, f := range p.ConfigSchema() {
			cfg = append(cfg, ConfigFieldDescription{Name: f.Name, Type: f.Type, Description: f.Description, Required: f.Required, Secret: f.Secret})
		}
		available = append(available, ProviderTypeDescription{Type: t, Config: cfg})
	}

	names := make([]string, 0, len(r.instances))
	for name := range r.instances {
		names = append(names, name)
	}
	sort.Strings(names)

	active := make([]ActiveDatasourceDescription, 0, len(names))
	for _, name := range names {
		active = append(active, ActiveDatasourceDescription{Name: name, Type: r.types[name]})
	}

	return DatasourcesResponse{AvailableTypes: available, Active: active}
}

func persistDatasource(app core.App, name, providerType string, config map[string]any) error {
	col, err := app.FindCollectionByNameOrId(datasourcesCollection)
	if err != nil {
		return err
	}
	rec, err := app.FindFirstRecordByFilter(datasourcesCollection, "name = {:name}", dbx.Params{"name": name})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("name", name)
	}
	// description/query_guidance deliberately never set here - re-onboarding
	// must not clobber a human's prior curation, same principle as
	// reconcileCatalog never overwriting a table/column's curated fields.
	rec.Set("type", providerType)
	rec.Set("config", config)
	return app.Save(rec)
}

func validateConfig(schema []dataprovider.ConfigField, config map[string]any) error {
	for _, f := range schema {
		if f.Required {
			if v, ok := config[f.Name]; !ok || v == "" {
				return fmt.Errorf("missing required config field %q", f.Name)
			}
		}
	}
	return nil
}
