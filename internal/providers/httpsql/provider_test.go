package httpsql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/jobstore"
)

func newTestServer(t *testing.T, wantToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+wantToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/schema":
			json.NewEncoder(w).Encode(schemaResponse{Tables: []dataprovider.TableCatalog{
				{Name: "widgets", Columns: []dataprovider.ColumnCatalog{{Name: "sku", Type: "TEXT"}}},
			}})
		case r.URL.Path == "/query" && r.URL.Query().Get("sql") == "boom":
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"message": "sql logic error"})
		case r.URL.Path == "/query":
			json.NewEncoder(w).Encode(ai.QueryResult{Columns: []string{"sku"}, Rows: [][]any{{"abc123"}}})
		case r.URL.Path == "/functions":
			json.NewEncoder(w).Encode(functionsResponse{Functions: []dataprovider.FunctionCatalog{
				{Name: "sync_matches", Description: "syncs matches", Args: []dataprovider.FunctionArg{
					{Name: "player_tag", Type: "string", Description: "the player", Required: true},
				}},
			}})
		case r.URL.Path == "/warm" && r.Method == http.MethodPost:
			var req warmRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Function == "unknown_function" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"message": `unknown function "unknown_function"`})
				return
			}
			json.NewEncoder(w).Encode(jobstore.Job{ID: "job1", Datasource: "valorant", Function: req.Function, Status: jobstore.Running})
		case r.URL.Path == "/warm" && r.Method == http.MethodGet:
			id := r.URL.Query().Get("id")
			if id == "missing" {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"message": "no job with that id"})
				return
			}
			json.NewEncoder(w).Encode(jobstore.Job{ID: id, Datasource: "valorant", Function: "sync_matches", Status: jobstore.Succeeded, Summary: "done"})
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

func newTestInstance(t *testing.T) dataprovider.Instance {
	t.Helper()
	srv := newTestServer(t, "secret-token")
	t.Cleanup(srv.Close)

	p := Provider{}
	inst, err := p.New(context.Background(), map[string]any{"base_url": srv.URL, "auth_token": "secret-token"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { inst.Close() })
	return inst
}

func TestInstance_Functions(t *testing.T) {
	inst := newTestInstance(t)
	fr := inst.(dataprovider.FunctionRunner)

	functions, err := fr.Functions(context.Background())
	if err != nil {
		t.Fatalf("Functions: %v", err)
	}
	if len(functions) != 1 || functions[0].Name != "sync_matches" {
		t.Fatalf("expected one sync_matches function, got %+v", functions)
	}
	if len(functions[0].Args) != 1 || functions[0].Args[0].Name != "player_tag" || !functions[0].Args[0].Required {
		t.Fatalf("unexpected args: %+v", functions[0].Args)
	}
}

func TestInstance_RunFunction(t *testing.T) {
	inst := newTestInstance(t)
	fr := inst.(dataprovider.FunctionRunner)

	job, err := fr.RunFunction(context.Background(), "sync_matches", map[string]any{"player_tag": "OrBest#NA1"})
	if err != nil {
		t.Fatalf("RunFunction: %v", err)
	}
	if job.ID != "job1" || job.Status != jobstore.Running {
		t.Fatalf("unexpected job: %+v", job)
	}
}

func TestInstance_RunFunction_RemoteRejectsUnknownFunction(t *testing.T) {
	inst := newTestInstance(t)
	fr := inst.(dataprovider.FunctionRunner)

	_, err := fr.RunFunction(context.Background(), "unknown_function", nil)
	if err == nil {
		t.Fatal("expected an error for an unknown function")
	}
	var remoteErr *dataprovider.RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("expected a *dataprovider.RemoteError, got %T: %v", err, err)
	}
	if remoteErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", remoteErr.StatusCode)
	}
}

func TestInstance_JobStatus(t *testing.T) {
	inst := newTestInstance(t)
	fr := inst.(dataprovider.FunctionRunner)

	job, err := fr.JobStatus(context.Background(), "job1")
	if err != nil {
		t.Fatalf("JobStatus: %v", err)
	}
	if job.Status != jobstore.Succeeded || job.Summary != "done" {
		t.Fatalf("unexpected job: %+v", job)
	}
}

func TestInstance_JobStatus_UnknownIDReturns404RemoteError(t *testing.T) {
	inst := newTestInstance(t)
	fr := inst.(dataprovider.FunctionRunner)

	_, err := fr.JobStatus(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for an unknown job id")
	}
	var remoteErr *dataprovider.RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("expected a *dataprovider.RemoteError, got %T: %v", err, err)
	}
	if remoteErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", remoteErr.StatusCode)
	}
}

func TestInstance_Query_NonOKStatusReturnsRemoteError(t *testing.T) {
	inst := newTestInstance(t)

	_, err := inst.Query(context.Background(), "boom")
	if err == nil {
		t.Fatal("expected an error")
	}
	var remoteErr *dataprovider.RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("expected a *dataprovider.RemoteError, got %T: %v", err, err)
	}
	if remoteErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", remoteErr.StatusCode)
	}
	if remoteErr.Message != "sql logic error" {
		t.Errorf("Message = %q, want the remote's own decoded message", remoteErr.Message)
	}
}
