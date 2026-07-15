package polyglot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
)

type WarmRequest struct {
	Datasource string         `json:"datasource"`
	Function   string         `json:"function"`
	Args       map[string]any `json:"args"`
}

// warmJobTimeout bounds how long a single background Function.Run may
// take before it's forcibly canceled, now that nothing else (an HTTP
// request's own deadline) bounds it. internal/ratelimit.Limiter.Wait and
// the henrik HTTP client both already respect ctx, so this actually
// aborts an in-flight rate-limit backoff/HTTP call, not just the wait for
// it to return.
const warmJobTimeout = 15 * time.Minute

// handleWarm implements POST /warm: look up the named datasource's
// Function, validate its required args are present (synchronously - an
// unknown datasource/function or a missing required arg is still an
// immediate 400, no job created), then run the function in the
// background and return 202 with a job to poll via GET /warm?id=.
// Per-argument type/value validation beyond presence is left to each
// Function's Run.
func handleWarm(reg *Registry, jobs *jobStore) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req WarmRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}

		instance, ok := reg.Instance(req.Datasource)
		if !ok {
			slog.Warn("polyglot: warm called with unknown datasource", "datasource", req.Datasource)
			return e.BadRequestError(fmt.Sprintf("unknown datasource %q", req.Datasource), nil)
		}

		var fn dataprovider.Function
		var found bool
		for _, f := range instance.Functions() {
			if f.Name == req.Function {
				fn, found = f, true
				break
			}
		}
		if !found {
			slog.Warn("polyglot: warm called with unknown function", "datasource", req.Datasource, "function", req.Function)
			return e.BadRequestError(fmt.Sprintf("unknown function %q for datasource %q", req.Function, req.Datasource), nil)
		}

		if err := requireArgs(fn.Args, req.Args); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}

		job := jobs.create(req.Datasource, req.Function)
		slog.Info("polyglot: warm started", "job_id", job.ID, "datasource", req.Datasource, "function", req.Function)
		slog.Debug("polyglot: warm args", "job_id", job.ID, "args", req.Args)

		go runWarmJob(jobs, job.ID, fn, req.Datasource, req.Function, req.Args)

		return e.JSON(http.StatusAccepted, job)
	}
}

// runWarmJob runs fn in the background and records its outcome in jobs.
// Deliberately derived from context.Background(), not the originating
// e.Request.Context() - that context is canceled the instant handleWarm
// returns, which would kill the work before it had a chance to run.
// Bounded by warmJobTimeout so a stuck call can't run forever.
func runWarmJob(jobs *jobStore, id string, fn dataprovider.Function, datasource, function string, args map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), warmJobTimeout)
	defer cancel()

	start := time.Now()
	outcome, err := fn.Run(ctx, args)
	if err != nil {
		slog.Error("polyglot: warm failed", "job_id", id, "datasource", datasource, "function", function,
			"error", err, "duration_ms", time.Since(start).Milliseconds())
		jobs.fail(id, err.Error())
		return
	}

	slog.Info("polyglot: warm complete", "job_id", id, "datasource", datasource, "function", function,
		"summary", outcome.Summary, "duration_ms", time.Since(start).Milliseconds())
	jobs.complete(id, outcome.Summary, outcome.Data)
}

// handleWarmStatus implements GET /warm?id=: report a previously started
// job's current status/result, per openapi/polyglot.yaml.
func handleWarmStatus(jobs *jobStore) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.URL.Query().Get("id")
		if id == "" {
			return e.BadRequestError("id query parameter is required", nil)
		}

		job, ok := jobs.get(id)
		if !ok {
			return e.NotFoundError(fmt.Sprintf("no job with id %q (unknown, or its result was evicted after finishing more than %s ago)", id, jobTTL), nil)
		}

		return e.JSON(http.StatusOK, job)
	}
}

// requireArgs checks that every required arg (per the function's own
// declared args, the single source of truth also served via GET
// /metadata) is present in the caller-supplied args.
func requireArgs(declared []dataprovider.FunctionArg, provided map[string]any) error {
	for _, arg := range declared {
		if !arg.Required {
			continue
		}
		if _, ok := provided[arg.Name]; !ok {
			return fmt.Errorf("missing required argument %q", arg.Name)
		}
	}
	return nil
}
