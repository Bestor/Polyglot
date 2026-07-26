// Package tracing configures OpenTelemetry distributed tracing shared by
// val-analyzer's binaries. Mirrors internal/logging's Init(debug bool)
// precedent: callers resolve their own env-driven config and pass in
// already-typed values, rather than this package reading os.Getenv itself
// (the only internal/* package that reads env vars directly today is
// internal/config; everything else, including internal/logging, takes
// resolved parameters).
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/pocketbase/pocketbase/core"
)

// Init wires up an OTLP/HTTP exporter, a TracerProvider, and the W3C
// TraceContext propagator, all registered as OpenTelemetry's process-wide
// globals. otlpEndpoint == "" disables tracing entirely: no otel globals
// are touched (the API package's own default no-op TracerProvider/
// TextMapPropagator make every tracer.Start(...)/otelhttp-wrapped client
// elsewhere in the codebase safe either way, with zero special-casing at
// any call site), and the returned shutdown is a no-op.
//
// Callers MUST call Init before constructing anything whose http.Client
// gets wrapped with otelhttp.NewTransport - that wrap captures whatever
// TextMapPropagator is globally registered at construction time, not
// lazily per-request, so constructing an otelhttp-wrapped client before
// Init runs silently and permanently disables trace propagation for that
// client's whole lifetime (no error, just a broken trace chain).
func Init(ctx context.Context, serviceName, otlpEndpoint string) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if otlpEndpoint == "" {
		return noop, nil
	}

	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(otlpEndpoint))
	if err != nil {
		return noop, fmt.Errorf("tracing: building OTLP exporter for %q: %w", otlpEndpoint, err)
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceNameKey.String(serviceName)))
	if err != nil {
		return noop, fmt.Errorf("tracing: building resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		// 100% sampling, deliberate: this is a low-volume personal project
		// where the point is inspecting every request in Jaeger, not
		// statistical sampling at production scale.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	// Baggage alongside TraceContext: TraceContext alone only carries the
	// trace/span id (which hop belongs to which trace), not any actual
	// content. Baggage is the W3C-standard sibling mechanism for
	// propagating arbitrary key/value context (e.g. the incoming Discord
	// question) across every hop's HTTP boundary via its own header,
	// riding the same otelhttp-wrapped clients/Extract calls already in
	// place for TraceContext - see SetBaggageAttributes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp.Shutdown, nil
}

// SetBaggageAttributes copies every W3C baggage member already present on
// ctx onto span as a "baggage.<key>" attribute. Generic over key names
// deliberately - a hop doesn't need to know in advance which keys an
// upstream caller chose to propagate (e.g. discordbot.ask sets "question"),
// it just surfaces whatever baggage actually arrived. Safe to call
// unconditionally: ctx with no baggage yields zero members, a no-op.
func SetBaggageAttributes(ctx context.Context, span trace.Span) {
	for _, m := range baggage.FromContext(ctx).Members() {
		span.SetAttributes(attribute.String("baggage."+m.Key(), m.Value()))
	}
}

// Middleware mirrors internal/httpauth.RequireToken's exact shape and
// installation convention (group.BindFunc(...)) for the two PocketBase-
// embedded binaries (cmd/valorantapi, cmd/polyglot). It extracts any
// incoming trace context from the request's headers, starts a span for
// the whole request, and swaps the span-carrying context into e.Request -
// every handler downstream already calls e.Request.Context() (that's the
// established pattern throughout this codebase), so this requires zero
// changes to any individual handler body.
func Middleware(serviceName string) func(e *core.RequestEvent) error {
	tracer := otel.Tracer(serviceName)
	return func(e *core.RequestEvent) error {
		ctx := otel.GetTextMapPropagator().Extract(e.Request.Context(), propagation.HeaderCarrier(e.Request.Header))
		ctx, span := tracer.Start(ctx, e.Request.Method+" "+e.Request.URL.Path)
		defer span.End()
		SetBaggageAttributes(ctx, span)

		e.Request = e.Request.WithContext(ctx)

		err := e.Next()
		if err != nil {
			span.RecordError(err)
		}
		return err
	}
}
