package polyglot

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"val-analyzer/internal/dataprovider"
	_ "val-analyzer/internal/migrations"
)

// fakeProvider is a minimal dataprovider.Provider for testing the
// Registry's onboarding engine without any real upstream dependency.
type fakeProvider struct {
	typ    string
	tables []dataprovider.TableSpec
}

func (p fakeProvider) Type() string { return p.typ }
func (p fakeProvider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{{Name: "api_key", Type: "string", Required: true, Secret: true}}
}
func (p fakeProvider) Tables() []dataprovider.TableSpec { return p.tables }
func (p fakeProvider) New(config map[string]any) (dataprovider.Instance, error) {
	if _, ok := config["api_key"].(string); !ok {
		return nil, errors.New("api_key is required")
	}
	return &fakeInstance{}, nil
}

type fakeInstance struct{ bound bool }

func (i *fakeInstance) Bind(app core.App) error { i.bound = true; return nil }
func (i *fakeInstance) Functions() []dataprovider.Function {
	return []dataprovider.Function{{Name: "sync"}}
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

func TestRegistryOnboard(t *testing.T) {
	app := newTestApp(t)

	widgets := fakeProvider{
		typ: "widgets",
		tables: []dataprovider.TableSpec{
			{Name: "widgets", Fields: []dataprovider.FieldSpec{{Name: "sku", Type: dataprovider.FieldText}}},
		},
	}
	reg := NewRegistry(map[string]dataprovider.Provider{"widgets": widgets})

	resp, err := reg.Onboard(app, "widgets", map[string]any{"api_key": "secret"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if resp.Type != "widgets" {
		t.Errorf("expected type %q, got %q", "widgets", resp.Type)
	}
	if len(resp.Tables) != 1 || resp.Tables[0] != "widgets" {
		t.Errorf("expected tables [widgets], got %v", resp.Tables)
	}
	if len(resp.Functions) != 1 || resp.Functions[0] != "sync" {
		t.Errorf("expected functions [sync], got %v", resp.Functions)
	}
	if resp.Config["api_key"] != "***" {
		t.Errorf("expected secret config field masked, got %v", resp.Config["api_key"])
	}

	if _, err := app.FindCollectionByNameOrId("widgets"); err != nil {
		t.Errorf("expected widgets collection to be dynamically created: %v", err)
	}

	inst, ok := reg.Instance("widgets")
	if !ok {
		t.Fatal("expected widgets instance to be active after Onboard")
	}
	if !inst.(*fakeInstance).bound {
		t.Error("expected instance to be Bind()ed")
	}

	// Re-onboarding is an idempotent upsert, not a duplicate/error.
	if _, err := reg.Onboard(app, "widgets", map[string]any{"api_key": "rotated"}); err != nil {
		t.Fatalf("re-onboarding: %v", err)
	}
}

func TestRegistryOnboardUnknownType(t *testing.T) {
	app := newTestApp(t)
	reg := NewRegistry(map[string]dataprovider.Provider{})

	_, err := reg.Onboard(app, "nope", nil)
	if !errors.Is(err, errUnknownProviderType) {
		t.Errorf("expected errUnknownProviderType, got %v", err)
	}
}

func TestRegistryOnboardInvalidConfig(t *testing.T) {
	app := newTestApp(t)
	reg := NewRegistry(map[string]dataprovider.Provider{"widgets": fakeProvider{typ: "widgets"}})

	_, err := reg.Onboard(app, "widgets", map[string]any{})
	if !errors.Is(err, errInvalidConfig) {
		t.Errorf("expected errInvalidConfig, got %v", err)
	}
}

func TestRegistryOnboardTableCollision(t *testing.T) {
	app := newTestApp(t)

	a := fakeProvider{typ: "a", tables: []dataprovider.TableSpec{
		{Name: "shared_table", Fields: []dataprovider.FieldSpec{{Name: "x", Type: dataprovider.FieldText}}},
	}}
	b := fakeProvider{typ: "b", tables: []dataprovider.TableSpec{
		{Name: "shared_table", Fields: []dataprovider.FieldSpec{{Name: "y", Type: dataprovider.FieldText}}},
	}}
	reg := NewRegistry(map[string]dataprovider.Provider{"a": a, "b": b})

	if _, err := reg.Onboard(app, "a", map[string]any{"api_key": "k"}); err != nil {
		t.Fatalf("onboarding a: %v", err)
	}
	_, err := reg.Onboard(app, "b", map[string]any{"api_key": "k"})
	if !errors.Is(err, errTableCollision) {
		t.Errorf("expected errTableCollision, got %v", err)
	}
}

func TestRegistryOnboardReservedDatasourcesName(t *testing.T) {
	app := newTestApp(t)

	evil := fakeProvider{typ: "evil", tables: []dataprovider.TableSpec{
		{Name: "datasources", Fields: []dataprovider.FieldSpec{{Name: "x", Type: dataprovider.FieldText}}},
	}}
	reg := NewRegistry(map[string]dataprovider.Provider{"evil": evil})

	_, err := reg.Onboard(app, "evil", map[string]any{"api_key": "k"})
	if !errors.Is(err, errTableCollision) {
		t.Errorf("expected errTableCollision when a provider claims the reserved %q table, got %v", datasourcesCollection, err)
	}
}
