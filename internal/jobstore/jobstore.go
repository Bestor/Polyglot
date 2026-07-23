// Package jobstore is a generic, in-memory, mutex-guarded tracker for
// short-lived background jobs (minutes at most) fired by an async HTTP
// endpoint that immediately returns 202 + a job id to poll. Shared by
// cmd/polyglot (catalog-reconcile jobs) and cmd/valorantapi (/warm jobs) -
// job tracking doesn't know or care what kind of work a Job's Function
// name actually names.
package jobstore

import (
	"sort"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/tools/security"
)

// Status is the lifecycle state of a Job.
type Status string

const (
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
)

// Job is the state of one async call: created in Running the instant the
// request is accepted, then updated exactly once (by the goroutine
// actually doing the work) to either Succeeded or Failed.
type Job struct {
	ID         string         `json:"id"`
	Datasource string         `json:"datasource"`
	Function   string         `json:"function"`
	Status     Status         `json:"status"`
	Summary    string         `json:"summary,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

const (
	// TTL bounds how long a finished job's result stays queryable before
	// being swept - long enough for a client to reasonably poll for it,
	// short enough that sustained traffic can't accumulate jobs forever.
	TTL = time.Hour
	// maxTrackedJobs is a hard safety cap independent of TTL, in case a
	// burst of job creation outpaces the TTL sweep. Running jobs are never
	// evicted regardless of this cap - only finished ones, oldest first.
	maxTrackedJobs = 500
)

// Store is an in-memory, mutex-guarded set of Jobs. Jobs are short-lived
// and losing job tracking on a process restart is acceptable, since the
// goroutine actually doing the work dies with the process too - so no
// persistence layer is needed here.
type Store struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func New() *Store {
	return &Store{jobs: map[string]*Job{}}
}

// Create registers a new running job and returns a snapshot of it (safe to
// hand straight to e.JSON - see Get's doc comment on why this is a value,
// not the internal pointer).
func (s *Store) Create(datasource, function string) Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	job := &Job{
		ID:         security.RandomString(15),
		Datasource: datasource,
		Function:   function,
		Status:     Running,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.jobs[job.ID] = job

	// Evict after inserting, not before: the new job is always Running at
	// this point, so it's exempt from eviction either way, but running
	// this after insertion is what lets the cap-based sweep below account
	// for the job we're about to hand back, instead of always leaving the
	// store one over maxTrackedJobs.
	s.evictLocked()

	return *job
}

// Get returns a snapshot of job id, if tracked. It returns a value copy
// rather than the internal pointer so a caller's read can never race with
// Complete/Fail mutating the same job concurrently from its goroutine.
func (s *Store) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *job, true
}

// Complete and Fail are each called exactly once per job, by the goroutine
// that ran its work. A missing id (already evicted) is a silent no-op, not
// an error - eviction only ever removes finished jobs (see evictLocked),
// so in practice a running job's own Complete/Fail call always finds
// itself still present; this is defensive only.
func (s *Store) Complete(id, summary string, data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status, job.Summary, job.Data, job.UpdatedAt = Succeeded, summary, data, time.Now()
	}
}

func (s *Store) Fail(id, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status, job.Error, job.UpdatedAt = Failed, errMsg, time.Now()
	}
}

// evictLocked bounds Store's growth under sustained job-creation traffic:
// first sweeps any finished job older than TTL, then - only if still over
// maxTrackedJobs - drops the oldest-updated finished jobs until under cap.
// Running jobs are never evicted by either path. Caller must hold s.mu.
func (s *Store) evictLocked() {
	now := time.Now()
	for id, job := range s.jobs {
		if job.Status != Running && now.Sub(job.UpdatedAt) > TTL {
			delete(s.jobs, id)
		}
	}
	if len(s.jobs) <= maxTrackedJobs {
		return
	}

	type finished struct {
		id        string
		updatedAt time.Time
	}
	var candidates []finished
	for id, job := range s.jobs {
		if job.Status != Running {
			candidates = append(candidates, finished{id, job.UpdatedAt})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].updatedAt.Before(candidates[j].updatedAt) })
	for _, c := range candidates {
		if len(s.jobs) <= maxTrackedJobs {
			return
		}
		delete(s.jobs, c.id)
	}
}
