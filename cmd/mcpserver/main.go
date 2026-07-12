// Command mcpserver exposes the polyglot Data API (openapi/polyglot.yaml)
// as an MCP server: one tool per operation in the spec, each proxying its
// call to a running polyglot instance over HTTP. Tool schemas are derived
// from the spec at startup, not hand-maintained, so they can't drift from
// polyglot's actual REST contract.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"val-analyzer/internal/mcpserver"
)

func main() {
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
	// session open for.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	log.Printf("mcpserver: exposing %d tool(s) from %s, proxying to %s, listening on :%s/mcp", len(ops), specPath, polyglotURL, port)
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
