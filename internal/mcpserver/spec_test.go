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

	if len(ops) != 6 {
		t.Fatalf("expected 6 operations, got %d: %+v", len(ops), byName)
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
	if len(query.Params) != 1 || query.Params[0].Name != "sql" {
		t.Errorf("query: expected a single 'sql' param, got %+v", query.Params)
	}
	requireStringSliceContains(t, "query", requiredOf(query.InputSchema), "sql")

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
	required := requiredOf(warm.InputSchema)
	requireStringSliceContains(t, "warm", required, "function")
	requireStringSliceContains(t, "warm", required, "args")

	getWarmJob, ok := byName["getWarmJob"]
	if !ok {
		t.Fatal("missing getWarmJob operation")
	}
	if getWarmJob.Method != "GET" || getWarmJob.Path != "/warm" {
		t.Errorf("getWarmJob: got method=%s path=%s", getWarmJob.Method, getWarmJob.Path)
	}
	if getWarmJob.HasBody {
		t.Error("getWarmJob: expected HasBody false")
	}
	if len(getWarmJob.Params) != 1 || getWarmJob.Params[0].Name != "id" {
		t.Errorf("getWarmJob: expected a single 'id' param, got %+v", getWarmJob.Params)
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

func requireStringSliceContains(t *testing.T, op string, haystack []string, want string) {
	t.Helper()
	for _, s := range haystack {
		if s == want {
			return
		}
	}
	t.Errorf("%s: expected required %q, got %v", op, want, haystack)
}
