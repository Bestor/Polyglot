package jobstore

import (
	"testing"
	"time"
)

func TestStore_CreateGet(t *testing.T) {
	s := New()

	job := s.Create("valorant", "sync_matches")
	if job.Status != Running {
		t.Errorf("expected new job to be running, got %s", job.Status)
	}
	if job.ID == "" {
		t.Error("expected a non-empty job id")
	}

	got, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("expected to find the job just created")
	}
	if got.Datasource != "valorant" || got.Function != "sync_matches" {
		t.Errorf("unexpected job fields: %+v", got)
	}
}

func TestStore_Get_Unknown(t *testing.T) {
	s := New()
	if _, ok := s.Get("does-not-exist"); ok {
		t.Error("expected ok=false for an unknown job id")
	}
}

func TestStore_Complete(t *testing.T) {
	s := New()
	job := s.Create("valorant", "sync_matches")

	s.Complete(job.ID, "synced 5 matches", map[string]any{"fetched": 5})

	got, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("expected job to still be tracked")
	}
	if got.Status != Succeeded {
		t.Errorf("expected status succeeded, got %s", got.Status)
	}
	if got.Summary != "synced 5 matches" {
		t.Errorf("expected summary to be set, got %q", got.Summary)
	}
	if got.Data["fetched"] != 5 {
		t.Errorf("expected data to be set, got %+v", got.Data)
	}
}

func TestStore_Fail(t *testing.T) {
	s := New()
	job := s.Create("valorant", "sync_matches")

	s.Fail(job.ID, "upstream error")

	got, ok := s.Get(job.ID)
	if !ok {
		t.Fatal("expected job to still be tracked")
	}
	if got.Status != Failed {
		t.Errorf("expected status failed, got %s", got.Status)
	}
	if got.Error != "upstream error" {
		t.Errorf("expected error to be set, got %q", got.Error)
	}
}

func TestStore_CompleteFail_UnknownID(t *testing.T) {
	s := New()
	// Neither should panic on an id that was never created.
	s.Complete("unknown", "summary", nil)
	s.Fail("unknown", "error")
}

func TestStore_EvictsExpiredFinishedJobs(t *testing.T) {
	s := New()

	finished := s.Create("valorant", "sync_seasons")
	s.Complete(finished.ID, "done", nil)
	// Backdate past TTL directly on the stored job.
	s.mu.Lock()
	s.jobs[finished.ID].UpdatedAt = time.Now().Add(-TTL - time.Minute)
	s.mu.Unlock()

	// A fresh Create() sweeps expired finished jobs as a side effect.
	s.Create("valorant", "sync_seasons")

	if _, ok := s.Get(finished.ID); ok {
		t.Error("expected the expired finished job to have been evicted")
	}
}

func TestStore_NeverEvictsRunningJobs(t *testing.T) {
	s := New()

	running := s.Create("valorant", "sync_matches")
	s.mu.Lock()
	s.jobs[running.ID].UpdatedAt = time.Now().Add(-TTL - time.Hour)
	s.mu.Unlock()

	s.Create("valorant", "sync_seasons")

	if _, ok := s.Get(running.ID); !ok {
		t.Error("expected a running job to survive eviction regardless of age")
	}
}

func TestStore_EvictsOldestFinishedOverCap(t *testing.T) {
	s := New()

	// Fill past maxTrackedJobs with already-finished jobs, oldest first.
	var ids []string
	for i := 0; i < maxTrackedJobs+10; i++ {
		job := s.Create("valorant", "sync_seasons")
		s.Complete(job.ID, "done", nil)
		s.mu.Lock()
		s.jobs[job.ID].UpdatedAt = time.Now().Add(time.Duration(i) * time.Millisecond)
		s.mu.Unlock()
		ids = append(ids, job.ID)
	}

	// One more Create() to trigger the cap-based sweep.
	s.Create("valorant", "sync_seasons")

	s.mu.Lock()
	count := len(s.jobs)
	s.mu.Unlock()
	if count > maxTrackedJobs {
		t.Errorf("expected at most %d tracked jobs, got %d", maxTrackedJobs, count)
	}

	if _, ok := s.Get(ids[0]); ok {
		t.Error("expected the oldest-updated finished job to have been evicted first")
	}
}
