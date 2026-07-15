package polyglot

import (
	"testing"
	"time"
)

func TestJobStore_CreateGet(t *testing.T) {
	s := newJobStore()

	job := s.create("valorant", "sync_matches")
	if job.Status != JobRunning {
		t.Errorf("expected new job to be running, got %s", job.Status)
	}
	if job.ID == "" {
		t.Error("expected a non-empty job id")
	}

	got, ok := s.get(job.ID)
	if !ok {
		t.Fatal("expected to find the job just created")
	}
	if got.Datasource != "valorant" || got.Function != "sync_matches" {
		t.Errorf("unexpected job fields: %+v", got)
	}
}

func TestJobStore_Get_Unknown(t *testing.T) {
	s := newJobStore()
	if _, ok := s.get("does-not-exist"); ok {
		t.Error("expected ok=false for an unknown job id")
	}
}

func TestJobStore_Complete(t *testing.T) {
	s := newJobStore()
	job := s.create("valorant", "sync_matches")

	s.complete(job.ID, "synced 5 matches", map[string]any{"fetched": 5})

	got, ok := s.get(job.ID)
	if !ok {
		t.Fatal("expected job to still be tracked")
	}
	if got.Status != JobSucceeded {
		t.Errorf("expected status succeeded, got %s", got.Status)
	}
	if got.Summary != "synced 5 matches" {
		t.Errorf("expected summary to be set, got %q", got.Summary)
	}
	if got.Data["fetched"] != 5 {
		t.Errorf("expected data to be set, got %+v", got.Data)
	}
}

func TestJobStore_Fail(t *testing.T) {
	s := newJobStore()
	job := s.create("valorant", "sync_matches")

	s.fail(job.ID, "upstream error")

	got, ok := s.get(job.ID)
	if !ok {
		t.Fatal("expected job to still be tracked")
	}
	if got.Status != JobFailed {
		t.Errorf("expected status failed, got %s", got.Status)
	}
	if got.Error != "upstream error" {
		t.Errorf("expected error to be set, got %q", got.Error)
	}
}

func TestJobStore_CompleteFail_UnknownID(t *testing.T) {
	s := newJobStore()
	// Neither should panic on an id that was never created.
	s.complete("unknown", "summary", nil)
	s.fail("unknown", "error")
}

func TestJobStore_EvictsExpiredFinishedJobs(t *testing.T) {
	s := newJobStore()

	finished := s.create("valorant", "sync_seasons")
	s.complete(finished.ID, "done", nil)
	// Backdate past jobTTL directly on the stored job.
	s.mu.Lock()
	s.jobs[finished.ID].UpdatedAt = time.Now().Add(-jobTTL - time.Minute)
	s.mu.Unlock()

	// A fresh create() sweeps expired finished jobs as a side effect.
	s.create("valorant", "sync_seasons")

	if _, ok := s.get(finished.ID); ok {
		t.Error("expected the expired finished job to have been evicted")
	}
}

func TestJobStore_NeverEvictsRunningJobs(t *testing.T) {
	s := newJobStore()

	running := s.create("valorant", "sync_matches")
	s.mu.Lock()
	s.jobs[running.ID].UpdatedAt = time.Now().Add(-jobTTL - time.Hour)
	s.mu.Unlock()

	s.create("valorant", "sync_seasons")

	if _, ok := s.get(running.ID); !ok {
		t.Error("expected a running job to survive eviction regardless of age")
	}
}

func TestJobStore_EvictsOldestFinishedOverCap(t *testing.T) {
	s := newJobStore()

	// Fill past maxTrackedJobs with already-finished jobs, oldest first.
	var ids []string
	for i := 0; i < maxTrackedJobs+10; i++ {
		job := s.create("valorant", "sync_seasons")
		s.complete(job.ID, "done", nil)
		s.mu.Lock()
		s.jobs[job.ID].UpdatedAt = time.Now().Add(time.Duration(i) * time.Millisecond)
		s.mu.Unlock()
		ids = append(ids, job.ID)
	}

	// One more create() to trigger the cap-based sweep.
	s.create("valorant", "sync_seasons")

	s.mu.Lock()
	count := len(s.jobs)
	s.mu.Unlock()
	if count > maxTrackedJobs {
		t.Errorf("expected at most %d tracked jobs, got %d", maxTrackedJobs, count)
	}

	if _, ok := s.get(ids[0]); ok {
		t.Error("expected the oldest-updated finished job to have been evicted first")
	}
}
