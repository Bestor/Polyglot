// Command mcpserver exposes the polyglot Data API (openapi/polyglot.yaml)
// as an MCP server: one tool per operation in the spec, each proxying its
// call to a running polyglot instance over HTTP. Tool schemas are derived
// from the spec at startup, not hand-maintained, so they can't drift from
// polyglot's actual REST contract.
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"val-analyzer/internal/logging"
	"val-analyzer/internal/mcpserver"
)

func main() {
	debug := getEnvBool("DEBUG", false)
	logging.Init(debug)

	specPath := getEnvDefault("OPENAPI_SPEC_PATH", "openapi/polyglot.yaml")
	port := getEnvDefault("PORT", "8092")

	polyglotURL := os.Getenv("POLYGLOT_URL")
	if polyglotURL == "" {
		log.Fatal("POLYGLOT_URL is required")
	}
	authToken := os.Getenv("POLYGLOT_AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("POLYGLOT_AUTH_TOKEN is required")
	}

	ops, err := mcpserver.LoadOperations(specPath)
	if err != nil {
		log.Fatalf("loading %s: %v", specPath, err)
	}

	client := mcpserver.NewClient(polyglotURL, authToken)
	server := mcpserver.NewServer(ops, client)

	// Stateless: true since every tool call is an independent, one-shot
	// proxy to polyglot - there's no server->client interaction to keep a
	// session open for. JSONResponse: true returns a single application/json
	// response per call instead of text/event-stream framing, since callers
	// here don't need streaming or server-initiated messages either.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	slog.Info("mcpserver: starting", "tools", len(ops), "spec", specPath, "polyglot_url", polyglotURL, "port", port, "debug", debug)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}
