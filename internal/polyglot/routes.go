// Package polyglot implements the polyglot Data API: GET /query (read-only
// ANSI SQL, optionally routed to a named onboarded datasource), GET
// /metadata (schema/guidance discovery from the persisted catalog),
// GET/POST /datasources plus /datasources/reconcile and the four
// /*/annotate curation endpoints, and POST /warm (a thin, generic proxy to
// whichever onboarded datasource implements dataprovider.FunctionRunner -
// e.g. cmd/valorantapi's own /warm, reached via internal/providers/
// httpsql). It has no AI/reasoning logic of its own - that lives in a
// separate MCP server that calls these endpoints. It also has no domain
// knowledge of its own - every onboarded datasource comes from a
// dataprovider.Provider registered with the Registry (see
// cmd/polyglot/main.go). Async jobs (polyglot's own catalog-reconcile
// jobs, or a proxied POST /warm job) are tracked via internal/jobstore (or,
// for a proxied job, the remote datasource's own equivalent) and polled
// through GET /jobs?id=[&datasource=].
package polyglot

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/httpauth"
	"val-analyzer/internal/jobstore"
)

// RegisterRoutes wires polyglot's HTTP API onto PocketBase's own router.
// It must be called from within an app.OnServe() handler (see
// cmd/polyglot/main.go), after reg has been rehydrated/auto-onboarded.
func RegisterRoutes(se *core.ServeEvent, reg *Registry, jobs *jobstore.Store, query ai.QueryFunc, authToken string) error {
	group := se.Router.Group("")
	group.BindFunc(httpauth.RequireToken(authToken))
	group.GET("/query", handleQuery(query, reg))
	group.GET("/metadata", handleMetadata())
	group.GET("/datasources", handleListDatasources(reg))
	group.POST("/datasources", handleOnboardDatasource(reg))
	group.POST("/datasources/reconcile", handleReconcileDatasource(reg))
	group.POST("/datasources/annotate", handleAnnotateDatasource())
	group.POST("/tables/annotate", handleAnnotateTable())
	group.POST("/columns/annotate", handleAnnotateColumn())
	group.POST("/warm", handleWarm(reg))
	group.POST("/functions/annotate", handleAnnotateFunction())
	group.GET("/jobs", handleJobStatus(jobs, reg))

	return nil
}

// handleJobStatus implements GET /jobs?id=: poll a previously started
// async job's current status/result - either one of polyglot's own local
// jobs (catalog-reconcile), or, when ?datasource= is given (mirroring
// /query's own ?datasource= convention), a proxied poll to that
// datasource's own FunctionRunner.JobStatus (see internal/polyglot/
// registry.go's FunctionJobStatus) - e.g. a job POST /warm started.
func handleJobStatus(jobs *jobstore.Store, reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.URL.Query().Get("id")
		if id == "" {
			return e.BadRequestError("id query parameter is required", nil)
		}

		if dsName := e.Request.URL.Query().Get("datasource"); dsName != "" {
			job, err := reg.FunctionJobStatus(e.Request.Context(), dsName, id)
			if err != nil {
				return functionErrorResponse(e, err)
			}
			return e.JSON(http.StatusOK, job)
		}

		job, ok := jobs.Get(id)
		if !ok {
			return e.NotFoundError("no job with that id (unknown, or its result was evicted after finishing)", nil)
		}

		return e.JSON(http.StatusOK, job)
	}
}
