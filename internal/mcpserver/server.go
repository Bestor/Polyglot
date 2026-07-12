package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxLoggedPayload caps how much of a raw argument/response body ends up in
// a single Debug log line, so one large tool call doesn't flood stderr.
const maxLoggedPayload = 4096

// serverName/serverVersion identify this MCP server to connecting clients.
const (
	serverName    = "polyglot"
	serverVersion = "0.1.0"
)

// NewServer builds an MCP server exposing one tool per Operation, each
// proxying its call to polyglot via client.
func NewServer(ops []Operation, client *Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)

	for _, op := range ops {
		server.AddTool(&mcp.Tool{
			Name:        op.Name,
			Description: op.Description,
			InputSchema: op.InputSchema,
		}, toolHandler(op, client))
	}

	return server
}

// toolHandler proxies one tool call to polyglot. A non-nil error return is
// reserved for genuine transport failures (polyglot unreachable); an HTTP
// error response from polyglot itself is surfaced as a successful MCP
// result with IsError set, so the model can see it and self-correct - see
// the CallToolResult.IsError doc in the SDK.
func toolHandler(op Operation, client *Client) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				slog.Warn("mcpserver: invalid tool arguments", "tool", op.Name, "error", err,
					"raw_args", truncateForLog(string(req.Params.Arguments), maxLoggedPayload))
				return errorResult(fmt.Sprintf("invalid tool arguments: %v", err)), nil
			}
		}

		slog.Info("mcpserver: tool call", "tool", op.Name, "method", op.Method, "path", op.Path)
		slog.Debug("mcpserver: tool call args", "tool", op.Name, "args", args)
		start := time.Now()

		status, body, err := client.Call(ctx, op, args)
		if err != nil {
			slog.Error("mcpserver: tool call transport failure", "tool", op.Name, "error", err,
				"duration_ms", time.Since(start).Milliseconds())
			return nil, fmt.Errorf("calling polyglot %s %s: %w", op.Method, op.Path, err)
		}

		slog.Info("mcpserver: tool call complete", "tool", op.Name, "status", status,
			"duration_ms", time.Since(start).Milliseconds())
		slog.Debug("mcpserver: tool call response", "tool", op.Name,
			"body", truncateForLog(string(body), maxLoggedPayload))

		return &mcp.CallToolResult{
			IsError: status >= 400,
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}
}

func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// truncateForLog caps s at max bytes so a large tool argument or response
// body can't flood a single Debug log line.
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
