package discordbot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"val-analyzer/internal/tracing"
)

// maxToolIterations bounds the tool-use loop so a confused model can't
// spin forever (or run up unbounded cost) - mirrors the safety-cap pattern
// already used elsewhere in this codebase (e.g. maxSyncPages in
// internal/providers/valorant/ingest).
const maxToolIterations = 8

// answerMaxTokens is deliberately generous for a broad, multi-player
// question (e.g. a table across several players' recent matches) - a
// request that hit the previous, tighter budget mid-generation is exactly
// what produced an empty final answer in production once, not just a
// truncated one (see bot.go's finalizeAnswer, which now also guards
// against a too-long answer for Discord's own 2000-char limit regardless
// of this budget).
const answerMaxTokens = 8192

// systemPrompt reinforces, at the whole-conversation level, the same
// warm-tool restriction openapi/polyglot.yaml's "warm" operation
// description states - a single tool's description only gets weighed at
// the moment Claude is considering that specific call, which wasn't a
// strong enough signal on its own to stop it reaching for warm to plug a
// cache gap mid-question. That reflex is also the common way a question
// burns through maxToolIterations without ever reaching a final answer,
// since warm is async and never returns the data inline.
const systemPrompt = `You are answering Valorant statistics questions using tools backed by a local cache (query, getMetadata) and, separately, a tool that reaches out to a rate-limited upstream API (warm, polled via getJob).

Only call warm if the user has explicitly asked you to refresh, update, or sync the cache. Never call it just because a query came back empty or incomplete - warm runs in the background and will not return usable data in time to help answer the current question. If the data you need isn't cached, say so plainly instead of trying to warm it yourself.

Your answer is posted directly into a Discord message, and Discord's markdown renderer does not support pipe tables (` + "`| col | col |`" + ` style) - it just shows the raw pipe/dash characters as plain text. Never use a markdown table. For any tabular or side-by-side comparison data, use a fenced code block (triple backticks) with columns aligned using spaces, or fall back to a simple bulleted/numbered list - both render correctly in Discord.`

// Answer runs one question through Claude's tool-use loop, letting it call
// any of mcpserver's tools (via mcpSession) as many times as it needs, and
// returns the final plain-language answer.
func Answer(ctx context.Context, ai anthropic.Client, model string, mcpSession *mcp.ClientSession, tools []anthropic.ToolUnionParam, question string) (string, error) {
	tracer := otel.Tracer("discordbot")
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(question)),
	}

	for i := 0; i < maxToolIterations; i++ {
		callCtx, aiSpan := tracer.Start(ctx, "anthropic.messages.create")
		tracing.SetBaggageAttributes(callCtx, aiSpan)
		resp, err := ai.Messages.New(callCtx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: answerMaxTokens,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Tools:     tools,
			Messages:  messages,
		})
		if err != nil {
			aiSpan.RecordError(err)
			aiSpan.End()
			return "", fmt.Errorf("calling claude: %w", err)
		}
		aiSpan.SetAttributes(
			attribute.String("stop_reason", string(resp.StopReason)),
			attribute.Int64("usage.input_tokens", resp.Usage.InputTokens),
			attribute.Int64("usage.output_tokens", resp.Usage.OutputTokens),
		)
		aiSpan.End()
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

			toolCtx, toolSpan := tracer.Start(ctx, "mcp.tool."+toolUse.Name)
			tracing.SetBaggageAttributes(toolCtx, toolSpan)
			// toolUse.Input is already the raw JSON args Claude generated for
			// this call (e.g. {"sql":"..."} for query, {"function":...,
			// "args":...} for warm) - exactly "what did the AI send to
			// polyglot", attached as-is rather than re-marshaled from the
			// already-decoded args map above.
			toolSpan.SetAttributes(attribute.String("tool.arguments", string(toolUse.Input)))
			result, err := mcpSession.CallTool(toolCtx, &mcp.CallToolParams{Name: toolUse.Name, Arguments: args})
			if err != nil {
				toolSpan.RecordError(err)
				toolSpan.End()
				toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("tool call failed: %v", err), true))
				continue
			}
			toolSpan.SetAttributes(attribute.Bool("tool.is_error", result.IsError))
			toolSpan.End()
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
