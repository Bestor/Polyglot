// Package polyglot implements the polyglot Data API described by
// openapi/polyglot.yaml: GET /query (read-only ANSI SQL), POST /warm
// (named data-fill functions), GET /metadata (schema + function
// discovery), and GET/POST /datasources (onboard and describe
// DataProvider instances). It has no AI/reasoning logic of its own - that
// lives in a separate MCP server that calls these endpoints. It also has
// no domain knowledge of its own - every table/function comes from a
// dataprovider.Provider registered with the Registry (see
// cmd/polyglot/main.go).
package polyglot

import (
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
)

// RegisterRoutes wires polyglot's HTTP API onto PocketBase's own router.
// It must be called from within an app.OnServe() handler (see
// cmd/polyglot/main.go), after reg has been rehydrated/auto-onboarded,
// since building metadata requires collections that only exist once
// migrations (and any dynamic table creation from onboarding) have run.
func RegisterRoutes(se *core.ServeEvent, reg *Registry, query ai.QueryFunc, authToken string) error {
	group := se.Router.Group("")
	group.BindFunc(requireAuthToken(authToken))
	group.GET("/query", handleQuery(query))
	group.POST("/warm", handleWarm(reg))
	group.GET("/metadata", handleMetadata(reg))
	group.GET("/datasources", handleListDatasources(reg))
	group.POST("/datasources", handleOnboardDatasource(reg))

	return nil
}
