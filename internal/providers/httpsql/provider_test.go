package httpsql

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
)

func newTestServer(t *testing.T, wantToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+wantToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/schema":
			json.NewEncoder(w).Encode(schemaResponse{Tables: []dataprovider.TableCatalog{
				{Name: "widgets", Columns: []dataprovider.ColumnCatalog{{Name: "sku", Type: "TEXT"}}},
			}})
		case "/query":
			json.NewEncoder(w).Encode(ai.QueryResult{Columns: []string{"sku"}, Rows: [][]any{{"abc123"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestProvider_New_Succeeds(t *testing.T) {
	srv := newTestServer(t, "secret-token")
	defer srv.Close()

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"base_url": srv.URL, "auth_token": "secret-token"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer inst.Close()
}

func TestProvider_New_RequiresBaseURLAndToken(t *testing.T) {
	p := Provider{}
	if _, err := p.New(context.Background(), map[string]any{"auth_token": "x"}); err == nil {
		t.Error("expected an error when base_url is missing")
	}
	if _, err := p.New(context.Background(), map[string]any{"base_url": "http://x"}); err == nil {
		t.Error("expected an error when auth_token is missing")
	}
}

func TestProvider_New_FailsOnBadAuth(t *testing.T) {
	srv := newTestServer(t, "secret-token")
	defer srv.Close()

	p := Provider{}
	if _, err := p.New(context.Background(), map[string]any{"base_url": srv.URL, "auth_token": "wrong-token"}); err == nil {
		t.Error("expected New to fail its real onboarding round trip against a bad token")
	}
}

func TestProvider_New_FailsOnUnreachableServer(t *testing.T) {
	p := Provider{}
	if _, err := p.New(context.Background(), map[string]any{"base_url": "http://127.0.0.1:1", "auth_token": "x"}); err == nil {
		t.Error("expected New to fail when the remote is unreachable")
	}
}

func TestInstance_Query(t *testing.T) {
	srv := newTestServer(t, "secret-token")
	defer srv.Close()

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"base_url": srv.URL, "auth_token": "secret-token"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer inst.Close()

	result, err := inst.Query(context.Background(), "SELECT sku FROM widgets")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "abc123" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestInstance_Catalog(t *testing.T) {
	srv := newTestServer(t, "secret-token")
	defer srv.Close()

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"base_url": srv.URL, "auth_token": "secret-token"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer inst.Close()

	catalog, err := inst.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog) != 1 || catalog[0].Name != "widgets" {
		t.Fatalf("expected one widgets table, got %+v", catalog)
	}
}
