package polyglot

import (
	"testing"

	"val-analyzer/internal/dataprovider"
)

func TestRequireArgs(t *testing.T) {
	declared := []dataprovider.FunctionArg{
		{Name: "player_tag", Required: true},
		{Name: "count", Required: false},
	}

	if err := requireArgs(declared, map[string]any{"player_tag": "OrBest#NA1"}); err != nil {
		t.Errorf("expected no error when required arg present, got %v", err)
	}

	if err := requireArgs(declared, map[string]any{"count": float64(10)}); err == nil {
		t.Error("expected an error when required arg missing")
	}

	if err := requireArgs(declared, nil); err == nil {
		t.Error("expected an error when args map is nil")
	}
}
