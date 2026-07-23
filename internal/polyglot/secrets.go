package polyglot

import (
	"context"
	"fmt"

	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/vault"
)

// SecretRef is what a Secret-flagged dataprovider.ConfigField's value
// becomes once persisted - a pointer into vault, never the literal secret.
type SecretRef struct {
	VaultPath string `json:"$vault"`
}

// vaultClient is the slice of *vault.Client's API PersistConfig/
// ResolveConfig actually need - narrowed to an interface so tests can
// exercise both against a fake, hermetically.
type vaultClient interface {
	Write(ctx context.Context, path, value string) error
	Read(ctx context.Context, path string) (string, error)
}

// PersistConfig replaces every Secret-flagged field's real value with a
// SecretRef, writing the real value to vault first. Called unconditionally
// inside Registry.Onboard's persist step - including every time Rehydrate
// re-onboards a datasource on boot, which is also how a legacy
// plaintext-stored secret (e.g. datasources.config still holding a plain
// henrik_api_key string from before this existed) migrates into vault: no
// separate migration pass needed, the very next successful onboard for
// that row runs it through here regardless of where the real value came
// from.
func PersistConfig(ctx context.Context, vc vaultClient, datasourceName string, schema []dataprovider.ConfigField, config map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(config))
	for k, v := range config {
		out[k] = v
	}

	for _, f := range schema {
		if !f.Secret {
			continue
		}
		s, ok := config[f.Name].(string)
		if !ok || s == "" {
			continue
		}
		path := vault.PathFor(datasourceName, f.Name)
		if err := vc.Write(ctx, path, s); err != nil {
			return nil, fmt.Errorf("persisting secret %q: %w", f.Name, err)
		}
		out[f.Name] = SecretRef{VaultPath: path}
	}

	return out, nil
}

// ResolveConfig is the inverse, used only by Registry.Rehydrate (the only
// caller starting from a persisted, possibly ref-shaped row). A
// Secret-flagged field already holding a plain string is left as-is - it's
// already a real value - so this handles both "resolve a vault ref" and
// "pass through a not-yet-migrated plaintext value" with no special case.
func ResolveConfig(ctx context.Context, vc vaultClient, schema []dataprovider.ConfigField, config map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(config))
	for k, v := range config {
		out[k] = v
	}

	for _, f := range schema {
		if !f.Secret {
			continue
		}
		m, ok := config[f.Name].(map[string]any)
		if !ok {
			continue // not a ref shape - already a real value, pass through unchanged
		}
		path, _ := m["$vault"].(string)
		if path == "" {
			continue
		}
		val, err := vc.Read(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("resolving secret %q: %w", f.Name, err)
		}
		out[f.Name] = val
	}

	return out, nil
}
