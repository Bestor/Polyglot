package polyglot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// wantAPIErrorStatus asserts err is a *router.ApiError with the given
// status - handleListQueries/handleQueryDetail's error paths (Bad
// RequestError/NotFoundError/InternalServerError) return the error
// directly rather than writing to the response themselves (the actual HTTP
// response gets written later, by PocketBase's own router error handling)
// - confirmed against this package's existing functions_test.go, which
// asserts the same way.
func wantAPIErrorStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected a *router.ApiError, got %T: %v", err, err)
	}
	if apiErr.Status != wantStatus {
		t.Errorf("Status = %d, want %d", apiErr.Status, wantStatus)
	}
}

// newTestQueryEvent builds a *core.RequestEvent good enough to call
// handleListQueries/handleQueryDetail directly: e.JSON needs both a real
// Response (http.ResponseWriter) and a non-nil Request (it reads
// e.Request.URL.Query() even on a 200, for the ?fields= picker) - verified
// against the vendored pocketbase module's router.Event.JSON.
func newTestQueryEvent(t *testing.T, target string) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return &core.RequestEvent{Event: router.Event{Response: rec, Request: req}}, rec
}

// fakeJaegerServer returns an httptest.Server serving fixed JSON for both
// /api/traces (list) and /api/traces/{id} (detail) - tracesJSON/detailJSON
// are the raw response bodies to serve, letting each test control exactly
// what "Jaeger" returns.
func fakeJaegerServer(t *testing.T, tracesJSON, detailJSON string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/traces":
			w.Write([]byte(tracesJSON))
		default: // /api/traces/{id}
			w.Write([]byte(detailJSON))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleListQueries_NilClient_ReturnsEmptyList(t *testing.T) {
	e, rec := newTestQueryEvent(t, "/queries")
	if err := handleListQueries(nil)(e); err != nil {
		t.Fatalf("handleListQueries: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got ListQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got.Queries) != 0 {
		t.Errorf("expected an empty list, got %+v", got.Queries)
	}
}

// TestHandleListQueries_FlattensMultiQueryTrace reproduces the exact
// scenario found live during planning: one trace (one discordbot.ask
// conversation) containing multiple "query" spans, which must become
// multiple separate rows, not one - and sorted most-recent first.
func TestHandleListQueries_FlattensMultiQueryTrace(t *testing.T) {
	const tracesJSON = `{"data":[{
		"traceID": "trace1",
		"spans": [
			{"spanID":"span1","operationName":"query","startTime":1000000,"duration":5000,
			 "tags":[{"key":"tool.arguments","value":"{\"datasource\":\"valorant\",\"sql\":\"SELECT 1\"}"},{"key":"http.response.status_code","value":200}],
			 "processID":"p1"},
			{"spanID":"span2","operationName":"query","startTime":2000000,"duration":7000,
			 "tags":[{"key":"tool.arguments","value":"{\"datasource\":\"valorant\",\"sql\":\"SELECT 2\"}"},{"key":"http.response.status_code","value":500}],
			 "processID":"p1"},
			{"spanID":"span3","operationName":"getMetadata","startTime":3000000,"duration":1000,
			 "tags":[],"processID":"p1"}
		],
		"processes": {"p1": {"serviceName":"mcpserver"}}
	}]}`

	jc := NewJaegerClient(fakeJaegerServer(t, tracesJSON, "").URL)
	e, rec := newTestQueryEvent(t, "/queries")
	if err := handleListQueries(jc)(e); err != nil {
		t.Fatalf("handleListQueries: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var got ListQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got.Queries) != 2 {
		t.Fatalf("expected 2 query rows (getMetadata excluded), got %d: %+v", len(got.Queries), got.Queries)
	}
	// Most recent (span2) first.
	if got.Queries[0].ID != "trace1:span2" || got.Queries[0].SQL != "SELECT 2" || got.Queries[0].Status != "error" {
		t.Errorf("unexpected first row: %+v", got.Queries[0])
	}
	if got.Queries[1].ID != "trace1:span1" || got.Queries[1].SQL != "SELECT 1" || got.Queries[1].Status != "success" {
		t.Errorf("unexpected second row: %+v", got.Queries[1])
	}
}

func TestHandleListQueries_NoBaggageQuestion_QuestionIsNil(t *testing.T) {
	const tracesJSON = `{"data":[{
		"traceID": "trace1",
		"spans": [
			{"spanID":"span1","operationName":"query","startTime":1000000,"duration":5000,
			 "tags":[{"key":"tool.arguments","value":"{\"sql\":\"SELECT 1\"}"}],"processID":"p1"}
		],
		"processes": {"p1": {"serviceName":"mcpserver"}}
	}]}`

	jc := NewJaegerClient(fakeJaegerServer(t, tracesJSON, "").URL)
	e, rec := newTestQueryEvent(t, "/queries")
	if err := handleListQueries(jc)(e); err != nil {
		t.Fatalf("handleListQueries: %v", err)
	}

	var got ListQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got.Queries) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got.Queries))
	}
	if got.Queries[0].Question != nil {
		t.Errorf("expected a nil Question for a trace with no baggage.question tag, got %q", *got.Queries[0].Question)
	}
}

func TestHandleQueryDetail_NilClient_ReturnsNotFound(t *testing.T) {
	e, _ := newTestQueryEvent(t, "/queries/detail?id=trace1:span1")
	wantAPIErrorStatus(t, handleQueryDetail(nil)(e), http.StatusNotFound)
}

func TestHandleQueryDetail_MalformedID_ReturnsBadRequest(t *testing.T) {
	for _, id := range []string{"", "no-colon-here"} {
		e, _ := newTestQueryEvent(t, "/queries/detail?id="+id)
		wantAPIErrorStatus(t, handleQueryDetail(nil)(e), http.StatusBadRequest)
	}
}

func TestHandleQueryDetail_ReturnsFullSpanContext(t *testing.T) {
	const detailJSON = `{"data":[{
		"traceID": "trace1",
		"spans": [
			{"spanID":"span1","operationName":"query","startTime":1000000,"duration":5000,
			 "tags":[{"key":"tool.arguments","value":"{\"sql\":\"SELECT 1\"}"},{"key":"tool.response","value":"{\"rows\":[]}"},{"key":"baggage.question","value":"how good is Sova"},{"key":"http.response.status_code","value":200}],
			 "processID":"p1"},
			{"spanID":"span2","operationName":"GET /query","startTime":1000500,"duration":2000,
			 "tags":[{"key":"db.statement","value":"SELECT 1"}],"processID":"p2"}
		],
		"processes": {"p1": {"serviceName":"mcpserver"}, "p2": {"serviceName":"polyglot"}}
	}]}`

	jc := NewJaegerClient(fakeJaegerServer(t, "", detailJSON).URL)
	e, rec := newTestQueryEvent(t, "/queries/detail?id=trace1:span1")
	if err := handleQueryDetail(jc)(e); err != nil {
		t.Fatalf("handleQueryDetail: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var got QueryDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.SQL != "SELECT 1" || got.Response != `{"rows":[]}` {
		t.Errorf("unexpected summary/response fields: %+v", got)
	}
	if got.Question == nil || *got.Question != "how good is Sova" {
		t.Errorf("expected Question = \"how good is Sova\", got %v", got.Question)
	}
	if len(got.Spans) != 2 {
		t.Fatalf("expected both spans in the trace, got %d", len(got.Spans))
	}
	if got.Spans[1].Service != "polyglot" || got.Spans[1].Tags["db.statement"] != "SELECT 1" {
		t.Errorf("unexpected second span: %+v", got.Spans[1])
	}
}

func TestHandleQueryDetail_SpanNotInTrace_ReturnsNotFound(t *testing.T) {
	const detailJSON = `{"data":[{
		"traceID": "trace1",
		"spans": [{"spanID":"span1","operationName":"query","startTime":1000000,"duration":5000,"tags":[],"processID":"p1"}],
		"processes": {"p1": {"serviceName":"mcpserver"}}
	}]}`

	jc := NewJaegerClient(fakeJaegerServer(t, "", detailJSON).URL)
	e, _ := newTestQueryEvent(t, "/queries/detail?id=trace1:nonexistent")
	wantAPIErrorStatus(t, handleQueryDetail(jc)(e), http.StatusNotFound)
}
