package discordbot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxToolIterations bounds the tool-use loop so a confused model can't
// spin forever (or run up unbounded cost) - mirrors the safety-cap pattern
// already used elsewhere in this codebase (e.g. maxSyncPages in
// internal/providers/valorant/ingest).
const maxToolIterations = 8

// Answer runs one question through Claude's tool-use loop, letting it call
// any of mcpserver's tools (via mcpSession) as many times as it needs, and
// returns the final plain-language answer.
func Answer(ctx context.Context, ai anthropic.Client, model string, mcpSession *mcp.ClientSession, tools []anthropic.ToolUnionParam, question string) (string, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(question)),
	}

	for i := 0; i < maxToolIterations; i++ {
		resp, err := ai.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 4096,
			Tools:     tools,
			Messages:  messages,
		})
		if err != nil {
			return "", fmt.Errorf("calling claude: %w", err)
		}
		messages = append(messages, resp.ToParam())

		if resp.StopReason != anthropic.StopReasonToolUse {
			return finalText(resp), nil
		}

		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}

			var args map[string]any
			if err := json.Unmarshal(toolUse.Input, &args); err != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("invalid tool arguments: %v", err), true))
				continue
			}

			result, err := mcpSession.CallTool(ctx, &mcp.CallToolParams{Name: toolUse.Name, Arguments: args})
			if err != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("tool call failed: %v", err), true))
				continue
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUse.ID, mcpResultText(result), result.IsError))
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	return "", fmt.Errorf("gave up after %d tool-call rounds without a final answer", maxToolIterations)
}

func finalText(resp *anthropic.Message) string {
	var out string
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			out += tb.Text
		}
	}
	return out
}

func mcpResultText(result *mcp.CallToolResult) string {
	var out string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out
}
