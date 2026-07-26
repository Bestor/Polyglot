package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestInit_EmptyEndpoint_NoopShutdown(t *testing.T) {
	shutdown, err := Init(context.Background(), "test-svc", "")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned an error: %v", err)
	}
}

func TestInit_NonEmptyEndpoint_RegistersRealProvider(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	shutdown, err := Init(context.Background(), "test-svc", "http://127.0.0.1:4318")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer shutdown(context.Background())

	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Errorf("expected a real *sdktrace.TracerProvider to be globally registered, got %T", otel.GetTracerProvider())
	}
}

// withTestProvider swaps in an in-memory exporter as the global tracer
// provider/propagator for the duration of a test, restoring whatever was
// there before on cleanup - Middleware operates on process-wide otel
// globals (the same ones Init sets), so tests exercising actual span
// content need this rather than going through Init itself, which talks to
// a real (possibly unreachable) OTLP endpoint.
func withTestProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	// Composite, matching what Init actually registers - TestMiddleware_
	// ExtractsIncomingBaggage needs Baggage{} present, not just TraceContext{}.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return exp
}

func newTestRequestEvent(req *http.Request) *core.RequestEvent {
	return &core.RequestEvent{Event: router.Event{Request: req}}
}

func TestMiddleware_StartsASpanNamedAfterTheRequest(t *testing.T) {
	exp := withTestProvider(t)
	mw := Middleware("test-svc")

	e := newTestRequestEvent(httptest.NewRequest("GET", "/query", nil))
	if err := mw(e); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d: %+v", len(spans), spans)
	}
	if spans[0].Name != "GET /query" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "GET /query")
	}
}

func TestMiddleware_ExtractsIncomingTraceparent(t *testing.T) {
	exp := withTestProvider(t)
	mw := Middleware("test-svc")

	req := httptest.NewRequest("GET", "/query", nil)
	// A W3C Trace Context spec example traceparent header.
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	e := newTestRequestEvent(req)

	if err := mw(e); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].SpanContext.TraceID().String(); got != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace id = %q, want the incoming traceparent's trace id", got)
	}
	if got := spans[0].Parent.SpanID().String(); got != "b7ad6b7169203331" {
		t.Errorf("parent span id = %q, want the incoming traceparent's span id", got)
	}
}

// TestMiddleware_ExtractsIncomingBaggage proves an inbound "baggage" header
// (set by an upstream hop via SetBaggageAttributes' companion, an
// otelhttp-wrapped client injecting whatever's on its caller's context)
// ends up as a "baggage.<key>" attribute on this hop's own span - the
// mechanism that lets e.g. discordbot's "question" baggage member surface
// on polyglot's/valorantapi's spans too, several hops downstream, with no
// per-hop code needing to know that specific key name in advance.
// TestSetBaggageAttributes_CopiesEveryMember proves the helper is generic
// over key names - it doesn't special-case "question" or any other key,
// just copies whatever baggage members are actually present.
func TestSetBaggageAttributes_CopiesEveryMember(t *testing.T) {
	exp := withTestProvider(t)

	m1, err := baggage.NewMember("question", url.PathEscape("how good is Sova"))
	if err != nil {
		t.Fatalf("NewMember: %v", err)
	}
	m2, err := baggage.NewMember("request_id", "abc123")
	if err != nil {
		t.Fatalf("NewMember: %v", err)
	}
	bag, err := baggage.New(m1, m2)
	if err != nil {
		t.Fatalf("baggage.New: %v", err)
	}
	ctx := baggage.ContextWithBaggage(context.Background(), bag)

	_, span := otel.Tracer("test").Start(ctx, "test-span")
	SetBaggageAttributes(ctx, span)
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	got := map[string]string{}
	for _, kv := range spans[0].Attributes {
		got[string(kv.Key)] = kv.Value.AsString()
	}
	if got["baggage.question"] != "how good is Sova" {
		t.Errorf("baggage.question = %q, want %q", got["baggage.question"], "how good is Sova")
	}
	if got["baggage.request_id"] != "abc123" {
		t.Errorf("baggage.request_id = %q, want %q", got["baggage.request_id"], "abc123")
	}
}

func TestMiddleware_ExtractsIncomingBaggage(t *testing.T) {
	exp := withTestProvider(t)
	mw := Middleware("test-svc")

	req := httptest.NewRequest("GET", "/query", nil)
	req.Header.Set("baggage", "question=how%20good%20is%20Sova")
	e := newTestRequestEvent(req)

	if err := mw(e); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	var got string
	for _, kv := range spans[0].Attributes {
		if string(kv.Key) == "baggage.question" {
			got = kv.Value.AsString()
		}
	}
	if got != "how good is Sova" {
		t.Errorf("baggage.question attribute = %q, want %q", got, "how good is Sova")
	}
}

func TestMiddleware_SwapsSpanContextIntoRequest(t *testing.T) {
	withTestProvider(t)
	mw := Middleware("test-svc")

	e := newTestRequestEvent(httptest.NewRequest("GET", "/query", nil))
	if err := mw(e); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	// e.Next() is a no-op on a bare, not-dispatched-through-the-router
	// event like this one (verified safe against PocketBase's own
	// hook.Event source), so there's no real downstream handler to
	// observe the swapped context through - check it directly instead.
	// The span itself has already ended by the time mw returns (deferred
	// span.End() inside Middleware), but SpanContext (trace/span id,
	// flags) stays valid after End - only "is recording" changes.
	sc := trace.SpanContextFromContext(e.Request.Context())
	if !sc.IsValid() {
		t.Error("expected e.Request.Context() to carry a valid span context after the middleware ran")
	}
}

func TestMiddleware_NoErrorPassesThroughCleanly(t *testing.T) {
	withTestProvider(t)
	mw := Middleware("test-svc")

	e := newTestRequestEvent(httptest.NewRequest("GET", "/query", nil))
	if err := mw(e); err != nil {
		t.Errorf("expected no error from a bare event whose Next() no-ops, got %v", err)
	}
}
