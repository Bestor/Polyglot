package mcpserver

import (
	"testing"
)

// specPath points at the repo's real openapi/polyglot.yaml (this test
// package lives at internal/mcpserver, two directories below repo root),
// so this test doubles as a guard against the spec and parser drifting
// apart.
const specPath = "../../openapi/polyglot.yaml"

func TestLoadOperations(t *testing.T) {
	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("LoadOperations: %v", err)
	}

	byName := make(map[string]Operation, len(ops))
	for _, op := range ops {
		byName[op.Name] = op
	}

	if len(ops) != 11 {
		t.Fatalf("expected 11 operations, got %d: %+v", len(ops), byName)
	}

	query, ok := byName["query"]
	if !ok {
		t.Fatal("missing query operation")
	}
	if query.Method != "GET" || query.Path != "/query" {
		t.Errorf("query: got method=%s path=%s", query.Method, query.Path)
	}
	if query.HasBody {
		t.Error("query: expected HasBody false")
	}
	if len(query.Params) != 2 {
		t.Errorf("query: expected 'sql' and 'datasource' params, got %+v", query.Params)
	}
	requireStringSliceContains(t, "query", requiredOf(query.InputSchema), "sql")

	onboard, ok := byName["onboardDatasource"]
	if !ok {
		t.Fatal("missing onboardDatasource operation")
	}
	if onboard.Method != "POST" || onboard.Path != "/datasources" {
		t.Errorf("onboardDatasource: got method=%s path=%s", onboard.Method, onboard.Path)
	}
	if !onboard.HasBody {
		t.Error("onboardDatasource: expected HasBody true")
	}
	required := requiredOf(onboard.InputSchema)
	requireStringSliceContains(t, "onboardDatasource", required, "name")
	requireStringSliceContains(t, "onboardDatasource", required, "type")

	getJob, ok := byName["getJob"]
	if !ok {
		t.Fatal("missing getJob operation")
	}
	if getJob.Method != "GET" || getJob.Path != "/jobs" {
		t.Errorf("getJob: got method=%s path=%s", getJob.Method, getJob.Path)
	}
	if getJob.HasBody {
		t.Error("getJob: expected HasBody false")
	}
	if len(getJob.Params) != 2 {
		t.Errorf("getJob: expected 'id' and 'datasource' params, got %+v", getJob.Params)
	}
	requireStringSliceContains(t, "getJob", paramNames(getJob), "id")
	requireStringSliceContains(t, "getJob", paramNames(getJob), "datasource")

	warm, ok := byName["warm"]
	if !ok {
		t.Fatal("missing warm operation")
	}
	if warm.Method != "POST" || warm.Path != "/warm" {
		t.Errorf("warm: got method=%s path=%s", warm.Method, warm.Path)
	}
	if !warm.HasBody {
		t.Error("warm: expected HasBody true")
	}
	warmRequired := requiredOf(warm.InputSchema)
	requireStringSliceContains(t, "warm", warmRequired, "datasource")
	requireStringSliceContains(t, "warm", warmRequired, "function")

	for _, name := range []string{"reconcileDatasource", "annotateDatasource", "annotateTable", "annotateColumn", "annotateFunction", "listDatasources"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing %s operation", name)
		}
	}

	metadata, ok := byName["getMetadata"]
	if !ok {
		t.Fatal("missing getMetadata operation")
	}
	if metadata.Method != "GET" || metadata.Path != "/metadata" {
		t.Errorf("getMetadata: got method=%s path=%s", metadata.Method, metadata.Path)
	}
	if metadata.InputSchema["type"] != "object" {
		t.Errorf("getMetadata: expected an object input schema, got %+v", metadata.InputSchema)
	}
	if _, hasRequired := metadata.InputSchema["required"]; hasRequired {
		t.Errorf("getMetadata: expected no required fields, got %+v", metadata.InputSchema["required"])
	}
}

func requiredOf(schema map[string]any) []string {
	raw, _ := schema["required"].([]string)
	return raw
}

func paramNames(op Operation) []string {
	names := make([]string, 0, len(op.Params))
	for _, p := range op.Params {
		names = append(names, p.Name)
	}
	return names
}

func requireStringSliceContains(t *testing.T, op string, haystack []string, want string) {
	t.Helper()
	for _, s := range haystack {
		if s == want {
			return
		}
	}
	t.Errorf("%s: expected required %q, got %v", op, want, haystack)
}
