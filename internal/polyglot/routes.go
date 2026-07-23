// Package polyglot implements the polyglot Data API: GET /query (read-only
// ANSI SQL, optionally routed to a named onboarded datasource), GET
// /metadata (schema/guidance discovery from the persisted catalog), and
// GET/POST /datasources plus /datasources/reconcile and the three
// /*/annotate curation endpoints. It has no AI/reasoning logic of its own
// - that lives in a separate MCP server that calls these endpoints. It
// also has no domain knowledge of its own - every onboarded datasource
// comes from a dataprovider.Provider registered with the Registry (see
// cmd/polyglot/main.go). Async jobs (currently just catalog reconciliation
// - there is no /warm here anymore, see cmd/valorantapi for that) are
// tracked via internal/jobstore and polled through GET /jobs?id=.
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
	group.GET("/jobs", handleJobStatus(jobs))

	return nil
}

// handleJobStatus implements GET /jobs?id=: poll a previously started
// async job's current status/result (currently only catalog-reconcile
// jobs - see internal/polyglot/catalog.go).
func handleJobStatus(jobs *jobstore.Store) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.URL.Query().Get("id")
		if id == "" {
			return e.BadRequestError("id query parameter is required", nil)
		}

		job, ok := jobs.Get(id)
		if !ok {
			return e.NotFoundError("no job with that id (unknown, or its result was evicted after finishing)", nil)
		}

		return e.JSON(http.StatusOK, job)
	}
}
