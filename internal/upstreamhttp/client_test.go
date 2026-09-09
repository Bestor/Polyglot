package upstreamhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"val-analyzer/internal/ratelimit"
)

// fastClient builds a Client whose retry backoff/cap are tiny, so tests
// that exercise the retry loop don't actually sleep for real-world
// durations - the loop's counting/decision logic is what's under test, not
// wall-clock timing.
func fastClient(source string, opts ...Option) *Client {
	base := []Option{WithBackoff(time.Millisecond, time.Millisecond)}
	return NewClient(source, ratelimit.NewLimiter(1000, 1000), time.Second, append(base, opts...)...)
}

type wireBody struct {
	Value string `json:"value"`
}

func TestClient_Get_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test"); got != "hello" {
			t.Errorf("expected X-Test header %q, got %q", "hello", got)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value":"ok"}`))
	}))
	defer srv.Close()

	c := fastClient("test")
	var out wireBody
	raw, err := c.Get(context.Background(), srv.URL, map[string]string{"X-Test": "hello"}, &out)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Value != "ok" {
		t.Errorf("decoded value = %q, want %q", out.Value, "ok")
	}
	if string(raw) != `{"value":"ok"}` {
		t.Errorf("raw body = %q", raw)
	}
}

func TestClient_Get_RetriesOn429ThenSucceeds_WithRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value":"ok"}`))
	}))
	defer srv.Close()

	c := fastClient("test")
	var out wireBody
	if _, err := c.Get(context.Background(), srv.URL, nil, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Value != "ok" {
		t.Errorf("decoded value = %q, want %q", out.Value, "ok")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
}

func TestClient_Get_RetriesOn429ThenSucceeds_WithoutRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value":"ok"}`))
	}))
	defer srv.Close()

	c := fastClient("test")
	var out wireBody
	if _, err := c.Get(context.Background(), srv.URL, nil, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 requests, got %d", got)
	}
}

func TestClient_Get_GivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := fastClient("test", WithMaxRetries(1))
	_, err := c.Get(context.Background(), srv.URL, nil, nil)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
	if !apiErr.IsRateLimited() {
		t.Error("expected IsRateLimited() to be true")
	}
	// maxRetries=1 means: initial attempt + 1 retry = 2 requests total.
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
}

func TestClient_Get_NonRetryableError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	c := fastClient("test")
	_, err := c.Get(context.Background(), srv.URL, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if !apiErr.IsNotFound() {
		t.Error("expected IsNotFound() to be true")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected exactly 1 request (no retry for a non-429 error), got %d", got)
	}
}
