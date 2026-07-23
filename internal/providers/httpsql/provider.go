// Package httpsql is a dataprovider.Provider that connects to another
// service speaking polyglot's own small machine-to-machine query contract
// (GET /query returning ai.QueryResult's columnar shape, GET /schema
// returning a dataprovider.TableCatalog list) over HTTP. cmd/valorantapi is
// the first real implementation of that contract - Valorant is onboarded
// into core polyglot as an ordinary http_sql datasource, not a special
// case - but nothing here is Valorant-specific.
package httpsql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
)

// Type is this provider's stable slug, used as the registry key.
const Type = "http_sql"

type Provider struct{}

var _ dataprovider.Provider = Provider{}

func (Provider) Type() string { return Type }

func (Provider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{
		{Name: "base_url", Type: "string", Required: true,
			Description: "Base URL of a service exposing GET /query and GET /schema, e.g. http://valorantapi:8093."},
		{Name: "auth_token", Type: "string", Required: true, Secret: true,
			Description: "Bearer token for the remote service."},
	}
}

func (Provider) New(ctx context.Context, config map[string]any) (dataprovider.Instance, error) {
	baseURL, _ := config["base_url"].(string)
	token, _ := config["auth_token"].(string)
	if baseURL == "" || token == "" {
		return nil, fmt.Errorf("http_sql: base_url and auth_token are required")
	}

	inst := &instance{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: &http.Client{}}
	// A real round trip at onboarding time, same rationale as sqlite's
	// PingContext - a bad base_url/auth_token fails onboarding, not the
	// first query.
	if _, err := inst.Catalog(ctx); err != nil {
		return nil, fmt.Errorf("http_sql: connecting to %q: %w", baseURL, err)
	}
	return inst, nil
}

type instance struct {
	baseURL, token string
	client         *http.Client
}

var _ dataprovider.Instance = (*instance)(nil)

type schemaResponse struct {
	Tables []dataprovider.TableCatalog `json:"tables"`
}

func (i *instance) Catalog(ctx context.Context) ([]dataprovider.TableCatalog, error) {
	var out schemaResponse
	if err := i.get(ctx, "/schema", nil, &out); err != nil {
		return nil, err
	}
	return out.Tables, nil
}

func (i *instance) Query(ctx context.Context, sqlText string) (ai.QueryResult, error) {
	var out ai.QueryResult
	err := i.get(ctx, "/query", url.Values{"sql": {sqlText}}, &out)
	return out, err
}

func (i *instance) Close() error { return nil } // stateless HTTP client, nothing to release

func (i *instance) get(ctx context.Context, path string, query url.Values, out any) error {
	target := i.baseURL + path
	if query != nil {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("http_sql: building request to %q: %w", target, err)
	}
	req.Header.Set("Authorization", "Bearer "+i.token)

	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("http_sql: calling %q: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http_sql: %q returned status %d", target, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("http_sql: decoding response from %q: %w", target, err)
	}
	return nil
}
