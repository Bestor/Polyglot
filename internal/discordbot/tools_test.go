package discordbot

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolsToAnthropic(t *testing.T) {
	tools := []*mcp.Tool{
		{
			Name:        "warm",
			Description: "Invoke a named data-fill function",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"datasource": map[string]any{"type": "string"},
					"function":   map[string]any{"type": "string"},
					"args":       map[string]any{"type": "object"},
				},
				"required": []any{"datasource", "function", "args"},
			},
		},
	}

	got := ToolsToAnthropic(tools)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(got))
	}

	tool := got[0].OfTool
	if tool == nil {
		t.Fatal("expected OfTool to be set")
	}
	if tool.Name != "warm" {
		t.Errorf("expected name %q, got %q", "warm", tool.Name)
	}
	if tool.Description.Value != "Invoke a named data-fill function" {
		t.Errorf("expected description to be set, got %q", tool.Description.Value)
	}

	wantRequired := []string{"datasource", "function", "args"}
	if !reflect.DeepEqual(tool.InputSchema.Required, wantRequired) {
		t.Errorf("expected required %v, got %v", wantRequired, tool.InputSchema.Required)
	}

	props, ok := tool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatalf("expected Properties to be a map[string]any, got %T", tool.InputSchema.Properties)
	}
	if _, ok := props["datasource"]; !ok {
		t.Error("expected properties to include 'datasource'")
	}
	if len(props) != 3 {
		t.Errorf("expected 3 properties, got %d", len(props))
	}
}

func TestToolsToAnthropic_Empty(t *testing.T) {
	got := ToolsToAnthropic(nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(got))
	}
}
