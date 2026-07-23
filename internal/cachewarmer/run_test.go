package cachewarmer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestRunPass(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"id":"job123","datasource":"valorant","function":"sync_matches","status":"running"}`))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "players.txt")
	if err := os.WriteFile(path, []byte("OrBest#NA1\ngoatninja01#NA1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	client := NewClient(srv.URL, "secret-token")
	RunPass(context.Background(), client, path, "sync_matches")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 warm calls, got %d", got)
	}
}

func TestRunPass_EmptyFile(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "does-not-exist.txt")
	client := NewClient(srv.URL, "secret-token")
	RunPass(context.Background(), client, path, "sync_matches")

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("expected 0 warm calls for a missing file, got %d", got)
	}
}
