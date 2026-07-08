package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// claudeModel is Haiku 4.5 - the cheapest current Claude model - since this
// provider mostly needs to run small, mechanical plan/interpret steps
// rather than deep reasoning.
const (
	claudeModel     = anthropic.ModelClaudeHaiku4_5
	claudeMaxTokens = 4096

	// planToolName is the single tool the model is forced to call for the
	// plan step - see buildPlanTool.
	planToolName = "submit_plan"
	// maxPlanAttempts bounds how many times we'll ask for a corrected plan
	// after an update or query failure, so a bad plan can't loop forever.
	maxPlanAttempts = 3
)

// ClaudeProvider implements Provider as a three-step pipeline: ask Claude
// for a plan (which table updates to run, plus a SQL query), execute that
// plan, then ask Claude to interpret the query results in plain language.
type ClaudeProvider struct {
	client anthropic.Client
}

func NewClaudeProvider(apiKey string) ClaudeProvider {
	return ClaudeProvider{client: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

var _ Provider = ClaudeProvider{}

// queryPlan is the structured output of the plan step.
type queryPlan struct {
	Updates []struct {
		Table string         `json:"table"`
		Args  map[string]any `json:"args"`
	} `json:"updates"`
	SQL string `json:"sql"`
}

func (p ClaudeProvider) Answer(ctx context.Context, req Request) (Response, error) {
	updatersByTable := make(map[string]TableUpdater, len(req.Tables))
	for _, t := range req.Tables {
		if t.Updater != nil {
			updatersByTable[t.Name] = *t.Updater
		}
	}

	system := buildPlanSystemPrompt(req.Tables)
	planTool := buildPlanTool()

	var plan queryPlan
	var result QueryResult
	var lastErr error

	for attempt := 0; attempt < maxPlanAttempts; attempt++ {
		userText := req.Question
		if lastErr != nil {
			userText = fmt.Sprintf("%s\n\n[Your previous plan failed: %v. Provide a corrected plan.]", req.Question, lastErr)
		}

		resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:      claudeModel,
			MaxTokens:  claudeMaxTokens,
			System:     []anthropic.TextBlockParam{{Text: system}},
			Tools:      []anthropic.ToolUnionParam{{OfTool: &planTool}},
			ToolChoice: anthropic.ToolChoiceParamOfTool(planToolName),
			Messages:   []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(userText))},
		})
		if err != nil {
			return Response{}, fmt.Errorf("claude: %w", err)
		}

		p2, err := parsePlan(resp)
		if err != nil {
			lastErr = err
			continue
		}
		plan = p2

		if err := p.runUpdates(ctx, plan, updatersByTable); err != nil {
			lastErr = err
			continue
		}

		result, err = req.Query(ctx, plan.SQL)
		if err != nil {
			lastErr = fmt.Errorf("query failed: %w (sql: %s)", err, plan.SQL)
			continue
		}

		return p.interpret(ctx, req.Question, plan.SQL, result)
	}

	return Response{}, fmt.Errorf("claude: failed to produce a working query plan after %d attempts: %w", maxPlanAttempts, lastErr)
}

func (p ClaudeProvider) runUpdates(ctx context.Context, plan queryPlan, updatersByTable map[string]TableUpdater) error {
	for _, u := range plan.Updates {
		updater, ok := updatersByTable[u.Table]
		if !ok {
			return fmt.Errorf("unknown table %q in update plan", u.Table)
		}
		if _, err := updater.Run(ctx, u.Args); err != nil {
			return fmt.Errorf("update %q failed: %w", u.Table, err)
		}
	}
	return nil
}

func (p ClaudeProvider) interpret(ctx context.Context, question, sql string, result QueryResult) (Response, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return Response{}, fmt.Errorf("claude: failed to encode query result: %w", err)
	}

	prompt := fmt.Sprintf(
		"Question: %s\n\nSQL query run:\n%s\n\nResults (JSON):\n%s\n\n"+
			"Answer the question in plain language based on these results. "+
			"If the results are empty or clearly insufficient to answer the question, say so plainly rather than guessing.",
		question, sql, resultJSON,
	)

	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     claudeModel,
		MaxTokens: claudeMaxTokens,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
	})
	if err != nil {
		return Response{}, fmt.Errorf("claude: %w", err)
	}

	return Response{Answer: extractText(resp)}, nil
}

func buildPlanTool() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: planToolName,
		Description: anthropic.String(
			"Submit your query plan: which table updates to run first (to ensure the data you need is cached and fresh), " +
				"then the read-only SQL query to run afterward to answer the question.",
		),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"updates": map[string]any{
					"type": "array",
					"description": "Table updates to run before the query, in the order they should run. " +
						"Empty if the cached data is already sufficient to answer the question.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"table": map[string]any{
								"type":        "string",
								"description": "The table to update, matching one of the table names below that has an update action.",
							},
							"args": map[string]any{
								"type":        "object",
								"description": "Arguments for this table's update action, matching its documented argument names.",
							},
						},
						"required": []string{"table", "args"},
					},
				},
				"sql": map[string]any{
					"type":        "string",
					"description": "A single read-only SELECT or WITH statement to run after the updates complete. Writes are rejected.",
				},
			},
			Required: []string{"sql"},
		},
	}
}

func parsePlan(resp *anthropic.Message) (queryPlan, error) {
	for _, block := range resp.Content {
		toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || toolUse.Name != planToolName {
			continue
		}
		var plan queryPlan
		if err := json.Unmarshal([]byte(toolUse.JSON.Input.Raw()), &plan); err != nil {
			return queryPlan{}, fmt.Errorf("invalid plan input: %w", err)
		}
		if strings.TrimSpace(plan.SQL) == "" {
			return queryPlan{}, fmt.Errorf("plan did not include a sql query")
		}
		return plan, nil
	}
	return queryPlan{}, fmt.Errorf("model did not call %s", planToolName)
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

func buildPlanSystemPrompt(tables []TableSpec) string {
	var b strings.Builder
	b.WriteString("Your job is to generate a plan to answer a statistical question about cached data stored in a SQLite database.\n\n")
	b.WriteString("Call the " + planToolName + " tool with:\n")
	b.WriteString("- \"updates\": a list of table updates to run first, in order, for any table whose data might be missing or stale for this question. ")
	b.WriteString("Each entry has \"table\" and \"args\" (matching that table's documented update arguments below). Leave empty if the cache is already sufficient.\n")
	b.WriteString("- \"sql\": a single read-only SELECT or WITH statement against the schema below, to run after the updates complete.\n\n")
	b.WriteString("Tables with no \"update\" section below cannot be refreshed directly - they are populated as a side effect of another table's update. ")
	b.WriteString("In particular, updating \"matches\" also refreshes match_teams, match_players, rounds, round_player_stats, damage_events, kills, kill_assists, and event_player_locations.\n\n")
	fmt.Fprintf(&b, "The current date is %s. Use it to resolve relative time references (e.g. \"this year\", \"in May\", \"last month\").\n\n", time.Now().Format("2006-01-02"))

	b.WriteString("Tables:\n")
	for _, table := range tables {
		fmt.Fprintf(&b, "- %s: %s\n", table.Name, table.Description)
		for _, col := range table.Columns {
			if col.Description != "" {
				fmt.Fprintf(&b, "    - %s (%s): %s\n", col.Name, col.Type, col.Description)
			} else {
				fmt.Fprintf(&b, "    - %s (%s)\n", col.Name, col.Type)
			}
		}
		if table.Updater != nil {
			fmt.Fprintf(&b, "    update: %s\n", table.Updater.Description)
			for _, arg := range table.Updater.Args {
				req := ""
				if arg.Required {
					req = ", required"
				}
				fmt.Fprintf(&b, "        - %s (%s%s): %s\n", arg.Name, arg.Type, req, arg.Description)
			}
		}
	}

	return b.String()
}
