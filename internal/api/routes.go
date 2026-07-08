package api

import (
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/ingest"
)

// RegisterRoutes wires the internal HTTP API onto PocketBase's own router.
// It's called from within an existing app.OnServe() handler (see
// cmd/server/main.go), rather than registering its own, so schema/executor
// setup that must happen after bootstrap stays in one place.
func RegisterRoutes(se *core.ServeEvent, ing *ingest.Service, schema []ai.TableDescription, query ai.QueryFunc, provider ai.Provider, authToken string) {
	group := se.Router.Group("/api")
	group.BindFunc(requireAuthToken(authToken))
	group.POST("/ask", handleAsk(ing, schema, query, provider))
	group.POST("/warm", handleWarm(ing))
}
