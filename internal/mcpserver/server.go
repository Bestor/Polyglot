package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
				return errorResult(fmt.Sprintf("invalid tool arguments: %v", err)), nil
			}
		}

		status, body, err := client.Call(ctx, op, args)
		if err != nil {
			return nil, fmt.Errorf("calling polyglot %s %s: %w", op.Method, op.Path, err)
		}

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
