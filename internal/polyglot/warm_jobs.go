package polyglot

import (
	"sort"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/tools/security"
)

// JobStatus is the lifecycle state of a WarmJob.
type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

// WarmJob is the state of one POST /warm call: created in JobRunning the
// instant the request is accepted, then updated exactly once (by the
// goroutine actually running the Function) to either JobSucceeded or
// JobFailed. Mirrors the WarmJob schema in openapi/polyglot.yaml.
type WarmJob struct {
	ID         string         `json:"id"`
	Datasource string         `json:"datasource"`
	Function   string         `json:"function"`
	Status     JobStatus      `json:"status"`
	Summary    string         `json:"summary,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

const (
	// jobTTL bounds how long a finished job's result stays queryable via
	// GET /warm?id= before being swept - long enough for a client to
	// reasonably poll for it, short enough that an hourly cachewarmer
	// cycle can't accumulate jobs across cycles.
	jobTTL = time.Hour
	// maxTrackedJobs is a hard safety cap independent of jobTTL, in case a
	// burst of job creation outpaces the TTL sweep. Running jobs are never
	// evicted regardless of this cap - only finished ones, oldest first.
	maxTrackedJobs = 500
)

// jobStore is an in-memory, mutex-guarded set of WarmJobs. Jobs are
// short-lived (minutes at most) and losing job tracking on a process
// restart is acceptable, since the goroutine actually doing the work dies
// with the process too - so no persistence layer is needed here.
type jobStore struct {
	mu   sync.Mutex
	jobs map[string]*WarmJob
}

func newJobStore() *jobStore {
	return &jobStore{jobs: map[string]*WarmJob{}}
}

// create registers a new running job and returns a snapshot of it (safe
// to hand straight to e.JSON - see get's doc comment on why this is a
// value, not the internal pointer).
func (s *jobStore) create(datasource, function string) WarmJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	job := &WarmJob{
		ID:         security.RandomString(15),
		Datasource: datasource,
		Function:   function,
		Status:     JobRunning,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.jobs[job.ID] = job

	// Evict after inserting, not before: the new job is always JobRunning
	// at this point, so it's exempt from eviction either way, but running
	// this after insertion is what lets the cap-based sweep below account
	// for the job we're about to hand back, instead of always leaving the
	// store one over maxTrackedJobs.
	s.evictLocked()

	return *job
}

// get returns a snapshot of job id, if tracked. It returns a value copy
// rather than the internal pointer so a caller's read can never race with
// complete/fail mutating the same job concurrently from its goroutine.
func (s *jobStore) get(id string) (WarmJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return WarmJob{}, false
	}
	return *job, true
}

// complete and fail are each called exactly once per job, by the
// goroutine that ran its Function. A missing id (already evicted) is a
// silent no-op, not an error - eviction only ever removes finished jobs
// (see evictLocked), so in practice a running job's own complete/fail
// call always finds itself still present; this is defensive only.
func (s *jobStore) complete(id, summary string, data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status, job.Summary, job.Data, job.UpdatedAt = JobSucceeded, summary, data, time.Now()
	}
}

func (s *jobStore) fail(id, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status, job.Error, job.UpdatedAt = JobFailed, errMsg, time.Now()
	}
}

// evictLocked bounds jobStore's growth under sustained hourly warm
// traffic: first sweeps any finished job older than jobTTL, then - only
// if still over maxTrackedJobs - drops the oldest-updated finished jobs
// until under cap. Running jobs are never evicted by either path.
// Caller must hold s.mu.
func (s *jobStore) evictLocked() {
	now := time.Now()
	for id, job := range s.jobs {
		if job.Status != JobRunning && now.Sub(job.UpdatedAt) > jobTTL {
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
		if job.Status != JobRunning {
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
