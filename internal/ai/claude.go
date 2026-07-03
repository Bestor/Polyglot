package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// claudeModel is Haiku 4.5 - the cheapest current Claude model - since this
// provider mostly needs to run small, mechanical SQL-fetch-and-summarize
// loops rather than deep reasoning.
const (
	claudeModel        = anthropic.ModelClaudeHaiku4_5
	claudeMaxTokens    = 4096
	claudeMaxToolTurns = 8

	// defaultSyncMoreCount is used when the model omits or gives an
	// out-of-range count on a sync_more_matches call.
	defaultSyncMoreCount = 20
	// maxSyncMoreCount bounds a single sync_more_matches call so the model
	// can't trigger an unbounded number of upstream API requests.
	maxSyncMoreCount = 50
)

// ClaudeProvider implements Provider by driving Claude through a manual
// tool-calling loop: Claude proposes SQL via the run_sql_query tool, we
// execute it through req.Query, and feed the rows back until Claude returns
// a final text answer.
type ClaudeProvider struct {
	client anthropic.Client
}

func NewClaudeProvider(apiKey string) ClaudeProvider {
	return ClaudeProvider{client: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

var _ Provider = ClaudeProvider{}

func (p ClaudeProvider) Answer(ctx context.Context, req Request) (Response, error) {
	sqlTool := anthropic.ToolParam{
		Name:        "run_sql_query",
		Description: anthropic.String("Run a single read-only SQL SELECT (or WITH) statement against the cached Valorant data and return the resulting rows."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "A single SELECT or WITH statement. Writes are rejected.",
				},
			},
		},
	}
	tools := []anthropic.ToolUnionParam{{OfTool: &sqlTool}}

	if req.SyncMore != nil {
		syncTool := anthropic.ToolParam{
			Name:        "sync_more_matches",
			Description: anthropic.String("Fetch and cache more recent matches for a player from the upstream Valorant API. Use this when run_sql_query shows fewer cached matches than the question needs (e.g. the question asks about the last 10 matches but only 5 are cached). Only call this after checking the cache first - it makes a real, rate-limited external API call."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"puuid": map[string]any{
						"type":        "string",
						"description": "The players.riot_puuid of the player to sync, from the hints below.",
					},
					"count": map[string]any{
						"type":        "integer",
						"description": "How many additional matches to fetch, e.g. 10. Defaults to a reasonable value if omitted.",
					},
				},
				Required: []string{"puuid"},
			},
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &syncTool})
	}

	system := []anthropic.TextBlockParam{{Text: buildSystemPrompt(req.Schema, req.Hints, req.SyncMore != nil)}}
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.Question)),
	}

	for turn := 0; turn < claudeMaxToolTurns; turn++ {
		resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     claudeModel,
			MaxTokens: claudeMaxTokens,
			System:    system,
			Tools:     tools,
			Messages:  messages,
		})
		if err != nil {
			return Response{}, fmt.Errorf("claude: %w", err)
		}

		messages = append(messages, resp.ToParam())

		if resp.StopReason != anthropic.StopReasonToolUse {
			return Response{Answer: extractText(resp)}, nil
		}

		toolResults := make([]anthropic.ContentBlockParamUnion, 0, len(resp.Content))
		for _, block := range resp.Content {
			toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			toolResults = append(toolResults, p.dispatchTool(ctx, req, toolUse))
		}

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	return Response{}, fmt.Errorf("claude: exceeded %d tool-calling turns without a final answer", claudeMaxToolTurns)
}

func (p ClaudeProvider) dispatchTool(ctx context.Context, req Request, toolUse anthropic.ToolUseBlock) anthropic.ContentBlockParamUnion {
	switch toolUse.Name {
	case "run_sql_query":
		return p.runSQLQuery(ctx, req, toolUse)
	case "sync_more_matches":
		return p.runSyncMore(ctx, req, toolUse)
	default:
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("unknown tool %q", toolUse.Name), true)
	}
}

func (p ClaudeProvider) runSQLQuery(ctx context.Context, req Request, toolUse anthropic.ToolUseBlock) anthropic.ContentBlockParamUnion {
	var input struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal([]byte(toolUse.JSON.Input.Raw()), &input); err != nil {
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("invalid tool input: %v", err), true)
	}

	result, err := req.Query(ctx, input.SQL)
	if err != nil {
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("query failed: %v", err), true)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("failed to encode result: %v", err), true)
	}

	return anthropic.NewToolResultBlock(toolUse.ID, string(resultJSON), false)
}

func (p ClaudeProvider) runSyncMore(ctx context.Context, req Request, toolUse anthropic.ToolUseBlock) anthropic.ContentBlockParamUnion {
	if req.SyncMore == nil {
		return anthropic.NewToolResultBlock(toolUse.ID, "sync_more_matches is not available for this request", true)
	}

	var input struct {
		PUUID string `json:"puuid"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(toolUse.JSON.Input.Raw()), &input); err != nil {
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("invalid tool input: %v", err), true)
	}
	if input.Count <= 0 || input.Count > maxSyncMoreCount {
		input.Count = defaultSyncMoreCount
	}

	outcome, err := req.SyncMore(ctx, input.PUUID, input.Count)
	if err != nil {
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("sync failed: %v", err), true)
	}

	resultJSON, err := json.Marshal(outcome)
	if err != nil {
		return anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("failed to encode result: %v", err), true)
	}

	return anthropic.NewToolResultBlock(toolUse.ID, string(resultJSON), false)
}

func extractText(resp *anthropic.Message) string {
	var b strings.Builder
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func buildSystemPrompt(schema []TableDescription, hints []string, canSyncMore bool) string {
	var b strings.Builder
	b.WriteString("You answer statistical questions about Valorant players using cached match data stored in a SQLite database. ")
	b.WriteString("Use the run_sql_query tool to run read-only SELECT/WITH queries against the schema below to gather the data you need, ")
	b.WriteString("then answer the user's question in plain language based on the results. Only query the tables described below.\n\n")

	if canSyncMore {
		b.WriteString("Cached data may be incomplete. If run_sql_query shows fewer matches than the question needs ")
		b.WriteString("(e.g. the question asks about the last 10 matches but you only find 5 cached rows for that player), ")
		b.WriteString("call sync_more_matches with that player's puuid from the hints below to fetch more before answering. ")
		b.WriteString("Check the cache first with run_sql_query rather than syncing speculatively.\n\n")
	}

	b.WriteString("Schema:\n")
	for _, table := range schema {
		fmt.Fprintf(&b, "- %s: %s\n", table.Name, table.Description)
		for _, col := range table.Columns {
			if col.Description != "" {
				fmt.Fprintf(&b, "    - %s (%s): %s\n", col.Name, col.Type, col.Description)
			} else {
				fmt.Fprintf(&b, "    - %s (%s)\n", col.Name, col.Type)
			}
		}
	}

	if len(hints) > 0 {
		b.WriteString("\nHints:\n")
		for _, h := range hints {
			fmt.Fprintf(&b, "- %s\n", h)
		}
	}

	return b.String()
}
