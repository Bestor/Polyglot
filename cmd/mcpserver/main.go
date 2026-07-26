// Command mcpserver exposes the polyglot Data API (openapi/polyglot.yaml)
// as an MCP server: one tool per operation in the spec, each proxying its
// call to a running polyglot instance over HTTP. Tool schemas are derived
// from the spec at startup, not hand-maintained, so they can't drift from
// polyglot's actual REST contract.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"val-analyzer/internal/logging"
	"val-analyzer/internal/mcpserver"
	"val-analyzer/internal/tracing"
)

func main() {
	debug := getEnvBool("DEBUG", false)
	logging.Init(debug)

	// Must run before mcpserver.NewClient(...) below, which constructs an
	// otelhttp-wrapped http.Client - see internal/tracing's doc comment on
	// why that ordering is load-bearing, not stylistic.
	shutdownTracing, err := tracing.Init(context.Background(), "mcpserver", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatalf("tracing: %v", err)
	}

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

	// A real *http.Server (not a bare http.ListenAndServe call) plus
	// signal handling, mirroring cmd/discordbot/main.go's existing
	// pattern - needed regardless of tracing (this binary previously had
	// no graceful shutdown at all), but specifically required here so
	// shutdownTracing has a real place to flush batched spans before exit.
	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcpserver: error during shutdown", "error", err)
	}
	shutdownTracing(shutdownCtx)
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
