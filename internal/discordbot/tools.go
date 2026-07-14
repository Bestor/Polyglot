package discordbot

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsToAnthropic converts every MCP tool mcpserver advertises into an
// Anthropic tool definition, so Claude can pick from the live set (query,
// warm, getMetadata, listDatasources, onboardDatasource) without any
// hand-maintained duplicate list - matches the val-analyzer convention
// already established in internal/mcpserver/spec.go, where tool schemas
// are derived from the spec rather than hardcoded. MCP's InputSchema is
// already JSON Schema, so this is a field reshape, not a schema rewrite.
func ToolsToAnthropic(tools []*mcp.Tool) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema, _ := t.InputSchema.(map[string]any)
		props, _ := schema["properties"].(map[string]any)

		var required []string
		if raw, ok := schema["required"].([]any); ok {
			for _, r := range raw {
				if s, ok := r.(string); ok {
					required = append(required, s)
				}
			}
		}

		out = append(out, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: props, Required: required},
		}})
	}
	return out
}
