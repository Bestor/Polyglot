// Package vault is a thin wrapper around the OpenBao (open-source Vault
// fork) KV v2 secrets engine, used by internal/polyglot to store every
// datasource secret out of PocketBase's own persisted config - only a
// vault path reference is ever persisted there (see
// internal/polyglot/secrets.go).
package vault

import (
	"context"
	"fmt"
	"time"

	vaultapi "github.com/openbao/openbao/api/v2"
)

// mountPath is the KV v2 secrets engine's mount point - enabled once by
// hand (`bao secrets enable -path=secret kv-v2`, see terraform/README.md).
// "secret" is the ecosystem-standard default name; there's no benefit to a
// bespoke name here since this OpenBao instance is single-purpose.
const mountPath = "secret"

// secretDataKey is the one field every secret is stored under, generic
// across whichever dataprovider.ConfigField.Name it's for.
const secretDataKey = "value"

// unsealMaxAttempts/unsealRetryDelay bound how long New waits for
// OpenBao's listener to come up before giving up - docker-compose's
// depends_on only waits for container start, not app readiness, so right
// after `docker compose up` a first attempt can easily race OpenBao's own
// startup. Package-level vars, not consts, so tests can shrink the retry
// delay instead of a real test run paying unsealMaxAttempts*unsealRetryDelay
// on the all-attempts-fail path.
var (
	unsealMaxAttempts = 10
	unsealRetryDelay  = time.Second
)

type Client struct{ kv *vaultapi.KVv2 }

// New builds a client and, if unsealKey is non-empty, unseals OpenBao if
// it's currently sealed - required on every real restart, since the file
// storage backend has no persisted seal state. A single-key-share/
// single-threshold init (see terraform/README.md) makes one key
// sufficient here. Storing that key alongside other deploy secrets
// (.env/GitHub Secrets/Terraform state) so this can run fully
// automatically - rather than requiring a manual `bao operator unseal`
// after every restart - is a deliberate, explicit choice, not an
// oversight: anyone who can already reach those secrets could unseal
// OpenBao by hand anyway.
func New(ctx context.Context, addr, token, unsealKey string) (*Client, error) {
	cfg := vaultapi.DefaultConfig()
	if addr != "" {
		cfg.Address = addr
	}
	cli, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("vault: building client: %w", err)
	}
	cli.SetToken(token)

	if unsealKey != "" {
		if err := unsealIfNeeded(ctx, cli, unsealKey); err != nil {
			return nil, fmt.Errorf("vault: unsealing: %w", err)
		}
	}

	return &Client{kv: cli.KVv2(mountPath)}, nil
}

// sealChecker is the slice of *vaultapi.Sys's API unsealIfNeeded actually
// needs - narrowed to an interface so tests can exercise the retry/branch
// logic below against a fake, without a real OpenBao server in CI.
type sealChecker interface {
	SealStatusWithContext(ctx context.Context) (*vaultapi.SealStatusResponse, error)
	UnsealWithContext(ctx context.Context, key string) (*vaultapi.SealStatusResponse, error)
}

func unsealIfNeeded(ctx context.Context, cli *vaultapi.Client, unsealKey string) error {
	return unsealSysIfNeeded(ctx, cli.Sys(), unsealKey)
}

func unsealSysIfNeeded(ctx context.Context, sys sealChecker, unsealKey string) error {
	var status *vaultapi.SealStatusResponse
	var err error
	for attempt := 0; attempt < unsealMaxAttempts; attempt++ {
		status, err = sys.SealStatusWithContext(ctx)
		if err == nil {
			break
		}
		time.Sleep(unsealRetryDelay)
	}
	if err != nil {
		return fmt.Errorf("checking seal status after %d attempts: %w", unsealMaxAttempts, err)
	}
	if !status.Sealed {
		return nil
	}

	_, err = sys.UnsealWithContext(ctx, unsealKey)
	return err
}

// Write stores value at path, creating a new KV v2 version.
func (c *Client) Write(ctx context.Context, path, value string) error {
	_, err := c.kv.Put(ctx, path, map[string]any{secretDataKey: value})
	if err != nil {
		return fmt.Errorf("vault: writing %q: %w", path, err)
	}
	return nil
}

// Read fetches the current value stored at path.
func (c *Client) Read(ctx context.Context, path string) (string, error) {
	secret, err := c.kv.Get(ctx, path)
	if err != nil {
		return "", fmt.Errorf("vault: reading %q: %w", path, err)
	}
	v, ok := secret.Data[secretDataKey].(string)
	if !ok {
		return "", fmt.Errorf("vault: %q has no %q field", path, secretDataKey)
	}
	return v, nil
}

// PathFor is keyed by datasource *name*, not provider type - two
// datasources sharing one provider type (e.g. two onboarded SQLite files)
// must never collide on the same vault path.
func PathFor(datasourceName, fieldName string) string {
	return fmt.Sprintf("datasources/%s/%s", datasourceName, fieldName)
}
