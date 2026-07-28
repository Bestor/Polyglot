package polyglot

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// recentQueriesTraceLimit bounds how many recent TRACES are fetched from
// Jaeger to flatten into individual query rows - deliberately a surplus
// over recentQueriesRowLimit, since a single trace can contain many query
// spans (one multi-turn conversation can call the query tool repeatedly).
const recentQueriesTraceLimit = 50

// recentQueriesRowLimit caps the flattened, sorted result GET /queries
// actually returns.
const recentQueriesRowLimit = 50

// QuerySummary is one row of GET /queries - one actual "query" MCP tool
// call, not one Jaeger trace (see jaegerTrace's doc comment on jaeger.go).
type QuerySummary struct {
	ID         string  `json:"id"` // "<traceID>:<spanID>"
	SQL        string  `json:"sql"`
	Question   *string `json:"question"` // the discordbot-originated free-text question, if any (see internal/tracing.SetBaggageAttributes) - nil for calls that didn't originate from discordbot's /ask
	Status     string  `json:"status"`   // "success" | "error"
	DurationMs int64   `json:"duration_ms"`
	Timestamp  string  `json:"timestamp"` // RFC3339
}

type ListQueriesResponse struct {
	Queries []QuerySummary `json:"queries"`
}

type SpanDetail struct {
	Service       string            `json:"service"`
	OperationName string            `json:"operation_name"`
	DurationMs    int64             `json:"duration_ms"`
	Tags          map[string]string `json:"tags"`
}

type QueryDetail struct {
	QuerySummary
	Response string       `json:"response"`
	Spans    []SpanDetail `json:"spans"`
}

// handleListQueries implements GET /queries: the recent-queries list, read
// straight from Jaeger's own trace storage rather than any persisted
// collection of polyglot's own - see CLAUDE.md/the plan doc for why
// (reuses the tracing pipeline already wired up end to end, at the cost of
// resetting whenever Jaeger's own deliberately-ephemeral storage restarts).
func handleListQueries(jc *JaegerClient) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if jc == nil {
			return e.JSON(http.StatusOK, ListQueriesResponse{Queries: []QuerySummary{}})
		}

		traces, err := jc.RecentQueryTraces(e.Request.Context(), recentQueriesTraceLimit)
		if err != nil {
			return e.InternalServerError("failed to load recent queries from Jaeger", err)
		}

		var summaries []QuerySummary
		for _, trace := range traces {
			for _, span := range trace.querySpans() {
				summaries = append(summaries, buildQuerySummary(trace, span))
			}
		}

		sort.Slice(summaries, func(i, j int) bool { return summaries[i].Timestamp > summaries[j].Timestamp })
		if len(summaries) > recentQueriesRowLimit {
			summaries = summaries[:recentQueriesRowLimit]
		}

		return e.JSON(http.StatusOK, ListQueriesResponse{Queries: summaries})
	}
}

// handleQueryDetail implements GET /queries/detail?id=<traceID>:<spanID>:
// the full response plus every hop's span in the same trace, for
// cross-service context (mcpserver -> polyglot -> valorantapi).
func handleQueryDetail(jc *JaegerClient) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.URL.Query().Get("id")
		if id == "" {
			return e.BadRequestError("id query parameter is required", nil)
		}
		traceID, spanID, ok := strings.Cut(id, ":")
		if !ok || traceID == "" || spanID == "" {
			return e.BadRequestError(`malformed id - expected "<traceID>:<spanID>"`, nil)
		}

		if jc == nil {
			return e.NotFoundError("query history is not available (Jaeger is not configured)", nil)
		}

		trace, err := jc.TraceByID(e.Request.Context(), traceID)
		if err != nil {
			return e.InternalServerError("failed to load query detail from Jaeger", err)
		}

		var querySpan jaegerSpan
		var found bool
		for _, s := range trace.querySpans() {
			if s.SpanID == spanID {
				querySpan, found = s, true
				break
			}
		}
		if !found {
			return e.NotFoundError("no such query", nil)
		}

		response, _ := querySpan.tag("tool.response")

		spans := make([]SpanDetail, 0, len(trace.Spans))
		for _, s := range trace.Spans {
			tags := make(map[string]string, len(s.Tags))
			for _, t := range s.Tags {
				if v, ok := s.tag(t.Key); ok {
					tags[t.Key] = v
				}
			}
			spans = append(spans, SpanDetail{
				Service:       trace.Processes[s.ProcessID].ServiceName,
				OperationName: s.OperationName,
				DurationMs:    s.Duration / 1000,
				Tags:          tags,
			})
		}

		return e.JSON(http.StatusOK, QueryDetail{
			QuerySummary: buildQuerySummary(trace, querySpan),
			Response:     response,
			Spans:        spans,
		})
	}
}

// buildQuerySummary extracts a QuerySummary from one "query" span plus its
// parent trace (for the trace-wide baggage.question lookup).
func buildQuerySummary(trace jaegerTrace, span jaegerSpan) QuerySummary {
	sql := ""
	if args, ok := span.tag("tool.arguments"); ok {
		var parsed struct {
			SQL string `json:"sql"`
		}
		if err := json.Unmarshal([]byte(args), &parsed); err == nil && parsed.SQL != "" {
			sql = parsed.SQL
		} else {
			sql = args // fall back to the raw args blob rather than showing nothing
		}
	}

	var question *string
	if q, ok := trace.findTag("baggage.question"); ok {
		question = &q
	}

	status := "success"
	if code, ok := span.tag("http.response.status_code"); ok {
		if n, err := strconv.Atoi(code); err == nil && n >= 400 {
			status = "error"
		}
	}

	return QuerySummary{
		ID:         trace.TraceID + ":" + span.SpanID,
		SQL:        sql,
		Question:   question,
		Status:     status,
		DurationMs: span.Duration / 1000,
		Timestamp:  time.UnixMicro(span.StartTime).UTC().Format(time.RFC3339),
	}
}
