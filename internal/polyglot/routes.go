package polyglot

import (
	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/ingest"
)

// RegisterRoutes wires polyglot's HTTP API (GET /query, POST /warm, GET
// /metadata - see openapi/polyglot.yaml) onto PocketBase's own router. It
// must be called from within an app.OnServe() handler (see
// cmd/polyglot/main.go), since building metadata requires collections
// that only exist once migrations have run during bootstrap.
func RegisterRoutes(se *core.ServeEvent, ing *ingest.Service, query ai.QueryFunc, authToken string) error {
	functions := []Function{resolvePlayerFunction(ing), syncMatchesFunction(ing)}

	metadata, err := buildMetadata(se.App, functions)
	if err != nil {
		return err
	}

	functionsByName := make(map[string]Function, len(functions))
	for _, f := range functions {
		functionsByName[f.Name] = f
	}

	group := se.Router.Group("")
	group.BindFunc(requireAuthToken(authToken))
	group.GET("/query", handleQuery(query))
	group.POST("/warm", handleWarm(functionsByName))
	group.GET("/metadata", handleMetadata(metadata))

	return nil
}
