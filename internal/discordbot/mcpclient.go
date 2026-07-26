// Package discordbot implements a Discord bot that answers questions by
// acting as an MCP client against a running mcpserver instance: it lists
// mcpserver's tools, hands them to Claude as tool definitions, and lets
// Claude's tool-use loop decide which ones to call to answer a question.
package discordbot

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewMCPSession connects to a running mcpserver instance over Streamable
// HTTP and returns a session good for the process's lifetime - mcpserver is
// stateless (see StreamableHTTPOptions{Stateless: true} in
// cmd/mcpserver/main.go), so one long-lived client session issuing
// concurrent tool calls from multiple Discord interactions at once is the
// expected usage, not a workaround.
func NewMCPSession(ctx context.Context, mcpURL string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "val-analyzer-discordbot", Version: "0.1.0"}, nil)
	// The wrapped HTTPClient is what makes each CallTool's outbound POST
	// carry a traceparent header (and get its own span) - see
	// internal/tracing's doc comment on why Init must run before this
	// session is ever created.
	transport := &mcp.StreamableClientTransport{
		Endpoint:   mcpURL,
		HTTPClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
	return client.Connect(ctx, transport, nil)
}
