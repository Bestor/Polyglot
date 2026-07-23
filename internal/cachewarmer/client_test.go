package cachewarmer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Warm(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody warmRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"id":"job123","datasource":"valorant","function":"sync_matches","status":"running"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	jobID, err := client.Warm(context.Background(), "sync_matches", "OrBest#NA1")
	if err != nil {
		t.Fatalf("Warm: %v", err)
	}
	if jobID != "job123" {
		t.Errorf("expected job id %q, got %q", "job123", jobID)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/warm" {
		t.Errorf("expected path /warm, got %s", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("expected bearer token header, got %q", gotAuth)
	}
	if gotBody.Function != "sync_matches" {
		t.Errorf("unexpected request body: %+v", gotBody)
	}
	if gotBody.Args["player_tag"] != "OrBest#NA1" {
		t.Errorf("expected player_tag arg, got %+v", gotBody.Args)
	}
}

func TestClient_Warm_NonAcceptedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":400,"message":"unknown function"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")
	if _, err := client.Warm(context.Background(), "bogus", "OrBest#NA1"); err == nil {
		t.Error("expected an error for a non-202 status")
	}
}
