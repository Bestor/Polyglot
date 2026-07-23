package polyglot

import (
	"context"
	"testing"

	"val-analyzer/internal/dataprovider"
)

var secretSchema = []dataprovider.ConfigField{
	{Name: "api_key", Secret: true},
	{Name: "base_url", Secret: false},
}

func TestPersistConfig_ReplacesSecretWithRef(t *testing.T) {
	vc := newFakeVault()
	out, err := PersistConfig(context.Background(), vc, "widgets", secretSchema, map[string]any{
		"api_key":  "shh",
		"base_url": "http://example.com",
	})
	if err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}

	ref, ok := out["api_key"].(SecretRef)
	if !ok {
		t.Fatalf("expected api_key to become a SecretRef, got %#v", out["api_key"])
	}
	if ref.VaultPath != "datasources/widgets/api_key" {
		t.Errorf("unexpected vault path %q", ref.VaultPath)
	}
	if out["base_url"] != "http://example.com" {
		t.Errorf("expected non-secret field to pass through unchanged, got %v", out["base_url"])
	}
	if got, err := vc.Read(context.Background(), ref.VaultPath); err != nil || got != "shh" {
		t.Errorf("expected the real value to be written to vault, got %q, err %v", got, err)
	}
}

func TestResolveConfig_ResolvesRef(t *testing.T) {
	vc := newFakeVault()
	vc.store["datasources/widgets/api_key"] = "shh"

	out, err := ResolveConfig(context.Background(), vc, secretSchema, map[string]any{
		"api_key":  map[string]any{"$vault": "datasources/widgets/api_key"},
		"base_url": "http://example.com",
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if out["api_key"] != "shh" {
		t.Errorf("expected api_key resolved to the real value, got %v", out["api_key"])
	}
}

// TestResolveConfig_PassesThroughLegacyPlaintext is the property that
// makes a separate migration pass unnecessary: a Secret-flagged field that
// isn't ref-shaped is already a real value, so ResolveConfig leaves it
// untouched rather than erroring or trying to interpret it as a path.
func TestResolveConfig_PassesThroughLegacyPlaintext(t *testing.T) {
	vc := newFakeVault()

	out, err := ResolveConfig(context.Background(), vc, secretSchema, map[string]any{
		"api_key": "plaintext-legacy-secret",
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if out["api_key"] != "plaintext-legacy-secret" {
		t.Errorf("expected legacy plaintext value to pass through unchanged, got %v", out["api_key"])
	}
}

func TestPersistResolve_RoundTrip(t *testing.T) {
	vc := newFakeVault()
	persisted, err := PersistConfig(context.Background(), vc, "widgets", secretSchema, map[string]any{"api_key": "shh"})
	if err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}

	// Simulate a JSON round-trip through PocketBase's JSONField, which
	// decodes into map[string]any rather than preserving the SecretRef
	// struct type.
	asJSONShape := map[string]any{"api_key": map[string]any{"$vault": persisted["api_key"].(SecretRef).VaultPath}}

	resolved, err := ResolveConfig(context.Background(), vc, secretSchema, asJSONShape)
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if resolved["api_key"] != "shh" {
		t.Errorf("expected round-trip to recover the real value, got %v", resolved["api_key"])
	}
}
