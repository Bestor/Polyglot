package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestClient_Call_QueryParams(t *testing.T) {
	var gotAuth, gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"rows":[],"row_count":0,"truncated":false}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	op := Operation{Method: "GET", Path: "/query", Params: []Param{{Name: "sql"}}}

	status, body, err := client.Call(context.Background(), op, map[string]any{"sql": "SELECT 1"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}
	if gotMethod != "GET" {
		t.Errorf("expected GET, got %s", gotMethod)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("expected bearer token header, got %q", gotAuth)
	}
	if gotQuery != "sql=SELECT+1" {
		t.Errorf("expected sql query param, got %q", gotQuery)
	}
	if string(body) == "" {
		t.Error("expected a non-empty response body")
	}
}

func TestClient_Call_JSONBody(t *testing.T) {
	var gotBody map[string]any
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		defer r.Body.Close()
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":400,"message":"unknown function","data":{}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	op := Operation{Method: "POST", Path: "/warm", HasBody: true}

	status, body, err := client.Call(context.Background(), op, map[string]any{"function": "nope", "args": map[string]any{}})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", status)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected application/json content-type, got %q", gotContentType)
	}
	if gotBody["function"] != "nope" {
		t.Errorf("expected function=nope in forwarded body, got %+v", gotBody)
	}
	if len(body) == 0 {
		t.Error("expected a non-empty response body")
	}
}

// TestClient_Call_PropagatesTraceparent proves the otelhttp-wrapped client
// actually injects a traceparent header, given an active span on the
// calling context - this is the mcpserver -> polyglot hop specifically.
func TestClient_Call_PropagatesTraceparent(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var gotTraceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		w.Write([]byte(`{"rows":[],"row_count":0,"truncated":false}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	op := Operation{Method: "GET", Path: "/query", Params: []Param{{Name: "sql"}}}

	ctx, span := otel.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	if _, _, err := client.Call(ctx, op, map[string]any{"sql": "SELECT 1"}); err != nil {
		t.Fatalf("Call: %v", err)
	}

	if gotTraceparent == "" {
		t.Error("expected the backend to receive a traceparent header, got none")
	}
}
