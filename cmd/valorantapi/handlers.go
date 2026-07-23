package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/jobstore"
	"val-analyzer/internal/valorant"
)

// handleQuery implements GET /query: run a caller-supplied read-only ANSI
// SQL statement against this binary's own PocketBase data and return the
// raw ai.QueryResult shape. This is a machine-to-machine contract consumed
// by core polyglot's internal/providers/httpsql, not by mcpserver/
// discordbot directly - so unlike core polyglot's own /query, there's no
// row-object reshaping here, just ai.QueryResult's own columnar JSON.
func handleQuery(query ai.QueryFunc) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		sqlText := e.Request.URL.Query().Get("sql")
		if sqlText == "" {
			return e.BadRequestError("sql query parameter is required", nil)
		}

		result, err := query(e.Request.Context(), sqlText)
		if err != nil {
			if errors.Is(err, ai.ErrNotReadOnly) {
				return e.BadRequestError(err.Error(), nil)
			}
			return e.InternalServerError("query failed", err)
		}

		return e.JSON(http.StatusOK, result)
	}
}

type schemaColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type schemaTable struct {
	Name    string         `json:"name"`
	Columns []schemaColumn `json:"columns"`
}

type schemaResponse struct {
	Tables []schemaTable `json:"tables"`
}

// reservedCollectionNames mirrors internal/polyglot's own reservedNames -
// core polyglot's onboarding/catalog bookkeeping collections. This
// binary's own migration set (internal/valorant/migrations) never creates
// them, but a data directory that started life as the old, pre-split
// combined single-binary polyglot database can still physically contain
// them as leftover collections; excluding them here keeps /schema (and
// thus core polyglot's httpsql catalog reconciliation) reporting only
// real Valorant domain tables regardless of a data directory's history.
var reservedCollectionNames = map[string]bool{"datasources": true, "tables": true, "columns": true}

// handleSchema implements GET /schema: introspects this binary's own live
// PocketBase collections (the 15 Valorant domain tables created by
// internal/valorant/migrations). This is what core polyglot's
// httpsql.Instance.Catalog calls to build its tables/columns catalog -
// curated descriptions live entirely on the core polyglot side, so this is
// structure only, no Description text.
func handleSchema(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		collections, err := app.FindAllCollections(core.CollectionTypeBase)
		if err != nil {
			return e.InternalServerError("failed to list collections", err)
		}

		resp := schemaResponse{Tables: make([]schemaTable, 0, len(collections))}
		for _, col := range collections {
			// core.CollectionTypeBase alone isn't enough to exclude
			// PocketBase's own system collections - several of them
			// (_mfas, _otps, _externalAuths, ...) are "base" type too,
			// not "auth". PocketBase's own convention is a leading
			// underscore for every system collection; none of Valorant's
			// real domain tables are named that way.
			if strings.HasPrefix(col.Name, "_") || reservedCollectionNames[col.Name] {
				continue
			}
			table := schemaTable{Name: col.Name, Columns: make([]schemaColumn, 0, len(col.Fields))}
			for _, f := range col.Fields {
				table.Columns = append(table.Columns, schemaColumn{Name: f.GetName(), Type: f.Type()})
			}
			resp.Tables = append(resp.Tables, table)
		}

		return e.JSON(http.StatusOK, resp)
	}
}

type warmRequest struct {
	Function string         `json:"function"`
	Args     map[string]any `json:"args"`
}

// warmJobTimeout bounds how long a single background Function.Run may
// take before it's forcibly canceled. Generous because sync_matches's
// full_history option can page up to the upstream API's actual history -
// a prolific player's entire history can take a long time under upstream
// rate-limit backoff, and since /warm is async this doesn't block anything
// else while it runs.
const warmJobTimeout = 2 * time.Hour

// handleWarm implements POST /warm: look up the named Function, validate
// its required args are present (synchronously - an unknown function or a
// missing required arg is still an immediate 400, no job created), then
// run it in the background and return 202 with a job to poll via
// GET /warm?id=.
func handleWarm(functions []valorant.Function, jobs *jobstore.Store) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req warmRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}

		var fn valorant.Function
		var found bool
		for _, f := range functions {
			if f.Name == req.Function {
				fn, found = f, true
				break
			}
		}
		if !found {
			slog.Warn("valorantapi: warm called with unknown function", "function", req.Function)
			return e.BadRequestError(fmt.Sprintf("unknown function %q", req.Function), nil)
		}

		if err := requireArgs(fn.Args, req.Args); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}

		job := jobs.Create("valorant", req.Function)
		slog.Info("valorantapi: warm started", "job_id", job.ID, "function", req.Function)
		slog.Debug("valorantapi: warm args", "job_id", job.ID, "args", req.Args)

		go runWarmJob(jobs, job.ID, fn, req.Args)

		return e.JSON(http.StatusAccepted, job)
	}
}

// runWarmJob runs fn in the background and records its outcome in jobs.
// Deliberately derived from context.Background(), not the originating
// e.Request.Context() - that context is canceled the instant handleWarm
// returns, which would kill the work before it had a chance to run.
func runWarmJob(jobs *jobstore.Store, id string, fn valorant.Function, args map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), warmJobTimeout)
	defer cancel()

	start := time.Now()
	outcome, err := fn.Run(ctx, args)
	if err != nil {
		slog.Error("valorantapi: warm failed", "job_id", id, "function", fn.Name,
			"error", err, "duration_ms", time.Since(start).Milliseconds())
		jobs.Fail(id, err.Error())
		return
	}

	slog.Info("valorantapi: warm complete", "job_id", id, "function", fn.Name,
		"summary", outcome.Summary, "duration_ms", time.Since(start).Milliseconds())
	jobs.Complete(id, outcome.Summary, outcome.Data)
}

// handleWarmStatus implements GET /warm?id=: report a previously started
// job's current status/result.
func handleWarmStatus(jobs *jobstore.Store) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.URL.Query().Get("id")
		if id == "" {
			return e.BadRequestError("id query parameter is required", nil)
		}

		job, ok := jobs.Get(id)
		if !ok {
			return e.NotFoundError(fmt.Sprintf("no job with id %q (unknown, or its result was evicted after finishing more than %s ago)", id, jobstore.TTL), nil)
		}

		return e.JSON(http.StatusOK, job)
	}
}

// requireArgs checks that every required arg (per the function's own
// declared args) is present in the caller-supplied args.
func requireArgs(declared []valorant.FunctionArg, provided map[string]any) error {
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
