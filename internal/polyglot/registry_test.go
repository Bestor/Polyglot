package polyglot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/jobstore"
	_ "val-analyzer/internal/migrations"
)

// fakeProvider is a minimal dataprovider.Provider for testing the
// Registry's onboarding engine without any real connection.
type fakeProvider struct {
	typ          string
	secretConfig bool
	catalog      []dataprovider.TableCatalog
	newErr       error
	// functionsCapable, when true, makes New return a *fakeFunctionInstance
	// (implementing dataprovider.FunctionRunner) instead of a plain
	// *fakeInstance - existing fakeProvider{...} call sites that don't set
	// this are unaffected.
	functionsCapable bool
	functions        []dataprovider.FunctionCatalog
	runFunctionJob   jobstore.Job
	runFunctionErr   error
	jobStatusJob     jobstore.Job
	jobStatusErr     error
}

func (p fakeProvider) Type() string { return p.typ }
func (p fakeProvider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{{Name: "api_key", Type: "string", Required: true, Secret: p.secretConfig}}
}
func (p fakeProvider) New(ctx context.Context, config map[string]any) (dataprovider.Instance, error) {
	if p.newErr != nil {
		return nil, p.newErr
	}
	if _, ok := config["api_key"].(string); !ok {
		return nil, errors.New("api_key is required")
	}
	base := &fakeInstance{catalog: p.catalog}
	if p.functionsCapable {
		return &fakeFunctionInstance{
			fakeInstance:   base,
			functions:      p.functions,
			runFunctionJob: p.runFunctionJob,
			runFunctionErr: p.runFunctionErr,
			jobStatusJob:   p.jobStatusJob,
			jobStatusErr:   p.jobStatusErr,
		}, nil
	}
	return base, nil
}

type fakeInstance struct {
	closed  bool
	catalog []dataprovider.TableCatalog
}

func (i *fakeInstance) Catalog(ctx context.Context) ([]dataprovider.TableCatalog, error) {
	return i.catalog, nil
}
func (i *fakeInstance) Query(ctx context.Context, sqlText string) (ai.QueryResult, error) {
	return ai.QueryResult{}, nil
}
func (i *fakeInstance) Close() error { i.closed = true; return nil }

// fakeFunctionInstance layers dataprovider.FunctionRunner onto a plain
// *fakeInstance via embedding, so tests that don't care about functions
// keep using fakeInstance directly and unmodified.
type fakeFunctionInstance struct {
	*fakeInstance
	functions      []dataprovider.FunctionCatalog
	runFunctionJob jobstore.Job
	runFunctionErr error
	jobStatusJob   jobstore.Job
	jobStatusErr   error
}

func (i *fakeFunctionInstance) Functions(ctx context.Context) ([]dataprovider.FunctionCatalog, error) {
	return i.functions, nil
}
func (i *fakeFunctionInstance) RunFunction(ctx context.Context, name string, args map[string]any) (jobstore.Job, error) {
	return i.runFunctionJob, i.runFunctionErr
}
func (i *fakeFunctionInstance) JobStatus(ctx context.Context, id string) (jobstore.Job, error) {
	return i.jobStatusJob, i.jobStatusErr
}

var _ dataprovider.FunctionRunner = (*fakeFunctionInstance)(nil)

// fakeVault is a hermetic, in-memory vaultClient for tests - no real
// OpenBao needed.
type fakeVault struct{ store map[string]string }

func newFakeVault() *fakeVault { return &fakeVault{store: map[string]string{}} }

func (v *fakeVault) Write(ctx context.Context, path, value string) error {
	v.store[path] = value
	return nil
}

func (v *fakeVault) Read(ctx context.Context, path string) (string, error) {
	val, ok := v.store[path]
	if !ok {
		return "", errors.New("vault: not found")
	}
	return val, nil
}

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)
	if _, err := core.NewMigrationsRunner(app, core.AppMigrations).Up(); err != nil {
		t.Fatalf("running app migrations: %v", err)
	}
	return app
}

func newTestRegistry(providers map[string]dataprovider.Provider) (*Registry, *jobstore.Store) {
	jobs := jobstore.New()
	return NewRegistry(providers, newFakeVault(), jobs), jobs
}

// waitForJob polls until id's job is no longer running, so a test doesn't
// race the async reconcile goroutine Onboard/Reconcile always starts
// against the same app.Cleanup-scoped test app.
func waitForJob(t *testing.T, jobs *jobstore.Store, id string) jobstore.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := jobs.Get(id)
		if ok && job.Status != jobstore.Running {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("job %q did not finish within the test timeout", id)
	return jobstore.Job{}
}

func TestRegistryOnboard(t *testing.T) {
	app := newTestApp(t)
	widgets := fakeProvider{typ: "widgets", secretConfig: true}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": widgets})

	resp, err := reg.Onboard(context.Background(), app, "my_widgets", "widgets", map[string]any{"api_key": "secret"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if resp.Name != "my_widgets" || resp.Type != "widgets" {
		t.Errorf("unexpected response identity: %+v", resp)
	}
	ref, ok := resp.Config["api_key"].(SecretRef)
	if !ok {
		t.Fatalf("expected secret config field to be a SecretRef, got %#v", resp.Config["api_key"])
	}
	if ref.VaultPath != "datasources/my_widgets/api_key" {
		t.Errorf("unexpected vault path %q", ref.VaultPath)
	}

	inst, ok := reg.Instance("my_widgets")
	if !ok {
		t.Fatal("expected my_widgets instance to be active after Onboard")
	}
	if inst.(*fakeInstance).closed {
		t.Error("expected the newly onboarded instance not to be closed")
	}

	waitForJob(t, jobs, resp.ReconcileJobID)

	rec, err := app.FindFirstRecordByFilter("datasources", "name = 'my_widgets'")
	if err != nil {
		t.Fatalf("expected a persisted datasources row: %v", err)
	}
	if rec.GetString("type") != "widgets" {
		t.Errorf("expected persisted type %q, got %q", "widgets", rec.GetString("type"))
	}
}

func TestRegistryOnboardUnknownType(t *testing.T) {
	app := newTestApp(t)
	reg, _ := newTestRegistry(map[string]dataprovider.Provider{})

	_, err := reg.Onboard(context.Background(), app, "x", "nope", nil)
	if !errors.Is(err, errUnknownProviderType) {
		t.Errorf("expected errUnknownProviderType, got %v", err)
	}
}

func TestRegistryOnboardInvalidConfig(t *testing.T) {
	app := newTestApp(t)
	reg, _ := newTestRegistry(map[string]dataprovider.Provider{"widgets": fakeProvider{typ: "widgets"}})

	_, err := reg.Onboard(context.Background(), app, "x", "widgets", map[string]any{})
	if !errors.Is(err, errInvalidConfig) {
		t.Errorf("expected errInvalidConfig, got %v", err)
	}
}

func TestRegistryOnboardReservedName(t *testing.T) {
	app := newTestApp(t)
	reg, _ := newTestRegistry(map[string]dataprovider.Provider{"widgets": fakeProvider{typ: "widgets"}})

	for _, name := range []string{"datasources", "tables", "columns"} {
		_, err := reg.Onboard(context.Background(), app, name, "widgets", map[string]any{"api_key": "k"})
		if !errors.Is(err, errReservedName) {
			t.Errorf("expected errReservedName for name %q, got %v", name, err)
		}
	}
}

// TestRegistryTwoDatasourcesSharingOneProviderType is the key identity
// test for this design: a datasource's identity is its name, not its
// provider type, so two names can both be "sqlite"-typed without
// colliding - unlike the pre-unification design where the provider type
// itself doubled as the datasource id.
func TestRegistryTwoDatasourcesSharingOneProviderType(t *testing.T) {
	app := newTestApp(t)
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": fakeProvider{typ: "widgets"}})

	respA, err := reg.Onboard(context.Background(), app, "widgets_a", "widgets", map[string]any{"api_key": "a"})
	if err != nil {
		t.Fatalf("onboarding widgets_a: %v", err)
	}
	respB, err := reg.Onboard(context.Background(), app, "widgets_b", "widgets", map[string]any{"api_key": "b"})
	if err != nil {
		t.Fatalf("onboarding widgets_b: %v", err)
	}
	waitForJob(t, jobs, respA.ReconcileJobID)
	waitForJob(t, jobs, respB.ReconcileJobID)

	if _, ok := reg.Instance("widgets_a"); !ok {
		t.Error("expected widgets_a to be active")
	}
	if _, ok := reg.Instance("widgets_b"); !ok {
		t.Error("expected widgets_b to be active")
	}
}

func TestRegistryReonboardClosesOldInstance(t *testing.T) {
	app := newTestApp(t)
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": fakeProvider{typ: "widgets"}})

	resp1, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "a"})
	if err != nil {
		t.Fatalf("first onboard: %v", err)
	}
	waitForJob(t, jobs, resp1.ReconcileJobID)
	firstInst, _ := reg.Instance("widgets")

	resp2, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "b"})
	if err != nil {
		t.Fatalf("second onboard: %v", err)
	}
	waitForJob(t, jobs, resp2.ReconcileJobID)

	if !firstInst.(*fakeInstance).closed {
		t.Error("expected the first instance to be closed after re-onboarding under the same name")
	}
	secondInst, _ := reg.Instance("widgets")
	if secondInst.(*fakeInstance).closed {
		t.Error("expected the new instance to still be open")
	}
}

func TestRegistryRehydrate(t *testing.T) {
	app := newTestApp(t)
	widgets := fakeProvider{typ: "widgets", secretConfig: true}

	// Onboard once with one registry (persists a vault-ref-shaped config row).
	reg1, jobs1 := newTestRegistry(map[string]dataprovider.Provider{"widgets": widgets})
	resp, err := reg1.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "secret"})
	if err != nil {
		t.Fatalf("onboard: %v", err)
	}
	waitForJob(t, jobs1, resp.ReconcileJobID)

	// A fresh registry, sharing the SAME underlying vault store (so the
	// ref written above can actually resolve), rehydrates from the
	// persisted row.
	jobs2 := jobstore.New()
	reg2 := NewRegistry(map[string]dataprovider.Provider{"widgets": widgets}, reg1.vc, jobs2)
	if err := reg2.Rehydrate(context.Background(), app); err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}

	if _, ok := reg2.Instance("widgets"); !ok {
		t.Fatal("expected widgets to be rehydrated and active")
	}

	// Rehydrate's internal Onboard call fires its own async reconcile
	// goroutine (see startReconcile) but doesn't expose that job's id back
	// to the caller, so there's no id to waitForJob on here - a short,
	// generous wait avoids the test's own t.Cleanup(app.Cleanup) closing
	// the app out from under that still-running goroutine (observed as a
	// real, if intermittent, SIGSEGV panic - the reconcile work itself is
	// in-memory-only and near-instant, so this is overwhelmingly reliable
	// in practice, not a source of new flakiness).
	waitForNoInFlightWork()
}

// waitForNoInFlightWork gives a fire-and-forget reconcile goroutine
// (started by Rehydrate, which doesn't expose its job id) time to finish
// before a test's own app.Cleanup runs - see the callers' comments.
func waitForNoInFlightWork() { time.Sleep(50 * time.Millisecond) }

// TestRegistryRehydrate_LegacyPlaintextSecretSelfMigrates proves the
// simplification over an earlier design pass: no separate migration pass
// is needed for a pre-vault plaintext secret - ResolveConfig passes it
// through unchanged (already a real value), and the very next Onboard's
// PersistConfig step converts it into a vault ref, exactly like any other
// onboard.
func TestRegistryRehydrate_LegacyPlaintextSecretSelfMigrates(t *testing.T) {
	app := newTestApp(t)
	widgets := fakeProvider{typ: "widgets", secretConfig: true}

	col, err := app.FindCollectionByNameOrId("datasources")
	if err != nil {
		t.Fatalf("finding datasources collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", "widgets")
	rec.Set("type", "widgets")
	rec.Set("config", map[string]any{"api_key": "plaintext-legacy-secret"})
	if err := app.Save(rec); err != nil {
		t.Fatalf("seeding legacy row: %v", err)
	}

	vc := newFakeVault()
	jobs := jobstore.New()
	reg := NewRegistry(map[string]dataprovider.Provider{"widgets": widgets}, vc, jobs)
	if err := reg.Rehydrate(context.Background(), app); err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	waitForNoInFlightWork()

	if _, ok := reg.Instance("widgets"); !ok {
		t.Fatal("expected widgets to be rehydrated despite the legacy plaintext secret")
	}

	updated, err := app.FindFirstRecordByFilter("datasources", "name = 'widgets'")
	if err != nil {
		t.Fatalf("re-reading datasources row: %v", err)
	}
	var storedConfig map[string]any
	if err := updated.UnmarshalJSONField("config", &storedConfig); err != nil {
		t.Fatalf("unmarshaling config: %v", err)
	}
	refShaped, ok := storedConfig["api_key"].(map[string]any)
	if !ok {
		t.Fatalf("expected api_key to be re-persisted as a vault ref, got %#v", storedConfig["api_key"])
	}
	if refShaped["$vault"] != "datasources/widgets/api_key" {
		t.Errorf("unexpected vault path %v", refShaped["$vault"])
	}
	if got, err := vc.Read(context.Background(), "datasources/widgets/api_key"); err != nil || got != "plaintext-legacy-secret" {
		t.Errorf("expected the legacy secret's real value to have been written to vault, got %q, err %v", got, err)
	}
}

func TestRegistryRunFunction(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ:              "widgets",
		functionsCapable: true,
		runFunctionJob:   jobstore.Job{ID: "job1", Datasource: "widgets", Function: "sync", Status: jobstore.Running},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "my_widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	job, err := reg.RunFunction(context.Background(), "my_widgets", "sync", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("RunFunction: %v", err)
	}
	if job.ID != "job1" || job.Status != jobstore.Running {
		t.Errorf("unexpected job: %+v", job)
	}
}

// TestRegistryRunFunction_OverwritesJobDatasource proves the remote's own
// self-labeled Job.Datasource never leaks through unchanged - it must
// always reflect the name this instance was actually onboarded under here,
// not whatever the remote itself happened to stamp.
func TestRegistryRunFunction_OverwritesJobDatasource(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ:              "widgets",
		functionsCapable: true,
		runFunctionJob:   jobstore.Job{ID: "job1", Datasource: "some-other-remote-self-label", Function: "sync"},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "my_widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	job, err := reg.RunFunction(context.Background(), "my_widgets", "sync", nil)
	if err != nil {
		t.Fatalf("RunFunction: %v", err)
	}
	if job.Datasource != "my_widgets" {
		t.Errorf("job.Datasource = %q, want the onboarded name %q", job.Datasource, "my_widgets")
	}
}

func TestRegistryRunFunction_UnknownDatasource(t *testing.T) {
	reg, _ := newTestRegistry(map[string]dataprovider.Provider{})

	_, err := reg.RunFunction(context.Background(), "nope", "sync", nil)
	if !errors.Is(err, errUnknownDatasource) {
		t.Errorf("expected errUnknownDatasource, got %v", err)
	}
}

func TestRegistryRunFunction_DatasourceNotFunctionCapable(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{typ: "widgets"} // functionsCapable: false
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	_, err = reg.RunFunction(context.Background(), "widgets", "sync", nil)
	if !errors.Is(err, errFunctionsNotSupported) {
		t.Errorf("expected errFunctionsNotSupported, got %v", err)
	}
}

func TestRegistryFunctionJobStatus(t *testing.T) {
	app := newTestApp(t)
	provider := fakeProvider{
		typ:              "widgets",
		functionsCapable: true,
		jobStatusJob:     jobstore.Job{ID: "job1", Datasource: "irrelevant", Status: jobstore.Succeeded, Summary: "done"},
	}
	reg, jobs := newTestRegistry(map[string]dataprovider.Provider{"widgets": provider})

	resp, err := reg.Onboard(context.Background(), app, "my_widgets", "widgets", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	waitForJob(t, jobs, resp.ReconcileJobID)

	job, err := reg.FunctionJobStatus(context.Background(), "my_widgets", "job1")
	if err != nil {
		t.Fatalf("FunctionJobStatus: %v", err)
	}
	if job.Status != jobstore.Succeeded || job.Summary != "done" {
		t.Errorf("unexpected job: %+v", job)
	}
	if job.Datasource != "my_widgets" {
		t.Errorf("job.Datasource = %q, want the onboarded name %q", job.Datasource, "my_widgets")
	}
}

func TestRegistryFunctionJobStatus_UnknownDatasource(t *testing.T) {
	reg, _ := newTestRegistry(map[string]dataprovider.Provider{})

	_, err := reg.FunctionJobStatus(context.Background(), "nope", "job1")
	if !errors.Is(err, errUnknownDatasource) {
		t.Errorf("expected errUnknownDatasource, got %v", err)
	}
}
