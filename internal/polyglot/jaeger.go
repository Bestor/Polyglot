package polyglot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// JaegerClient is a minimal read-only client for Jaeger's own trace-query
// HTTP API (the same port Jaeger's UI itself is served on, e.g.
// http://jaeger:16686 internally) - just enough to power GET /queries/GET
// /queries/detail, not a general Jaeger client. See jaegerTrace's own doc
// comment for why "recent queries" means flattening spans out of traces,
// not listing traces directly.
type JaegerClient struct {
	baseURL string
	client  *http.Client
}

// NewJaegerClient returns nil when baseURL is empty, so every caller
// downstream (handleListQueries, handleQueryDetail) can treat "Jaeger not
// configured" as a nil-safe no-op - mirroring this codebase's existing
// "tracing is fully optional when unset" convention - rather than a
// special-cased branch at every call site.
func NewJaegerClient(baseURL string) *JaegerClient {
	if baseURL == "" {
		return nil
	}
	return &JaegerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

type jaegerTag struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type jaegerSpan struct {
	SpanID        string      `json:"spanID"`
	OperationName string      `json:"operationName"`
	StartTime     int64       `json:"startTime"` // microseconds since epoch
	Duration      int64       `json:"duration"`  // microseconds
	Tags          []jaegerTag `json:"tags"`
	ProcessID     string      `json:"processID"`
}

func (s jaegerSpan) tag(key string) (string, bool) {
	for _, t := range s.Tags {
		if t.Key != key {
			continue
		}
		if str, ok := t.Value.(string); ok {
			return str, true
		}
		return fmt.Sprint(t.Value), true
	}
	return "", false
}

type jaegerProcess struct {
	ServiceName string `json:"serviceName"`
}

// jaegerTrace is Jaeger's own unit of storage/retrieval, but not this
// feature's unit of "a query" - a single trace can (and, in practice,
// routinely does) contain many spans named "query", one per tool call a
// multi-turn conversation made. Flattening every trace's query spans out
// individually, rather than treating one trace as one query, is what makes
// GET /queries accurate - confirmed against a real live trace during
// planning that contained 7 separate query spans from one discordbot.ask
// conversation.
type jaegerTrace struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
}

// querySpans returns every span in the trace named "query" - mcpserver's
// own tool-call span name (internal/mcpserver/server.go's toolHandler
// names its span after the MCP tool/operation name), not to be confused
// with the "GET /query" spans polyglot/valorantapi's own tracing.Middleware
// produces one level deeper in the same trace.
func (t jaegerTrace) querySpans() []jaegerSpan {
	var out []jaegerSpan
	for _, s := range t.Spans {
		if s.OperationName == "query" {
			out = append(out, s)
		}
	}
	return out
}

// findTag searches every span in the trace for the first match of key -
// used for baggage.question, which (per internal/tracing.SetBaggageAttributes)
// is copied onto every hop's span that had it in scope, not just the query
// span itself, so a trace-wide search is the correct scope for it.
func (t jaegerTrace) findTag(key string) (string, bool) {
	for _, s := range t.Spans {
		if v, ok := s.tag(key); ok {
			return v, true
		}
	}
	return "", false
}

type jaegerTracesResponse struct {
	Data []jaegerTrace `json:"data"`
}

// RecentQueryTraces fetches the limit most recent traces containing at
// least one "query" span. Note this bounds the number of TRACES fetched,
// not the number of query spans returned - since traces can contain
// multiple query spans, callers should fetch a deliberate surplus and
// flatten/cap client-side (see handleListQueries).
func (c *JaegerClient) RecentQueryTraces(ctx context.Context, limit int) ([]jaegerTrace, error) {
	target := fmt.Sprintf("%s/api/traces?service=mcpserver&operation=query&limit=%d", c.baseURL, limit)

	var out jaegerTracesResponse
	if err := c.get(ctx, target, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// TraceByID fetches one full trace (every span, every hop) by id - used by
// handleQueryDetail to show a query's full cross-service context, not just
// the query span in isolation.
func (c *JaegerClient) TraceByID(ctx context.Context, id string) (jaegerTrace, error) {
	target := fmt.Sprintf("%s/api/traces/%s", c.baseURL, id)

	var out jaegerTracesResponse
	if err := c.get(ctx, target, &out); err != nil {
		return jaegerTrace{}, err
	}
	if len(out.Data) == 0 {
		return jaegerTrace{}, fmt.Errorf("jaeger: no trace with id %q", id)
	}
	return out.Data[0], nil
}

func (c *JaegerClient) get(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("jaeger: building request to %q: %w", target, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("jaeger: calling %q: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jaeger: %q returned status %d", target, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jaeger: decoding response from %q: %w", target, err)
	}
	return nil
}
