package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestServer_EndToEnd drives the real MCP protocol (server and client
// connected over an in-memory transport) against a fake polyglot HTTP
// backend, proving tools generated from openapi/polyglot.yaml are
// actually callable, not just structurally present.
func TestServer_EndToEnd(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("backend saw auth header %q", got)
		}
		switch {
		case r.URL.Path == "/query" && r.URL.Query().Get("sql") == "SELECT 1":
			w.Write([]byte(`{"rows":[{"one":1}],"row_count":1,"truncated":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer backend.Close()

	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("LoadOperations: %v", err)
	}

	server := NewServer(ops, NewClient(backend.URL, "test-token"))

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 13 {
		t.Fatalf("expected 13 tools, got %d", len(tools.Tools))
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"sql": "SELECT 1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected a successful result, got error content: %+v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, `"one":1`) {
		t.Errorf("expected query result in response, got %q", text.Text)
	}
}

// TestServer_ToolHandler_CreatesASpan proves toolHandler creates a span
// named after the tool for every call - including over the in-memory
// transport this package's own tests use, where req.Extra can be nil (the
// nil-guard must not prevent span creation, just header extraction).
func TestServer_ToolHandler_CreatesASpan(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	exp := tracetest.NewInMemoryExporter()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)))

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"rows":[],"row_count":0,"truncated":false}`))
	}))
	defer backend.Close()

	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("LoadOperations: %v", err)
	}
	server := NewServer(ops, NewClient(backend.URL, "test-token"))

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer session.Close()

	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "query", Arguments: map[string]any{"sql": "SELECT 1"}}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	// Two spans are expected here, not one: toolHandler's own "query" span,
	// plus a child "HTTP GET" span from client.go's own otelhttp-wrapped
	// http.Client (NewClient) instrumenting the outbound call to the fake
	// polyglot backend - both are correct, this just asserts the former
	// exists among them.
	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "query" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a span named %q among %+v", "query", spans)
	}
}
