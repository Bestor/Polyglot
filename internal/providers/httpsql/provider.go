// Package httpsql is a dataprovider.Provider that connects to another
// service speaking polyglot's own small machine-to-machine query contract
// (GET /query returning ai.QueryResult's columnar shape, GET /schema
// returning a dataprovider.TableCatalog list) over HTTP. cmd/valorantapi is
// the first real implementation of that contract - Valorant is onboarded
// into core polyglot as an ordinary http_sql datasource, not a special
// case - but nothing here is Valorant-specific.
package httpsql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/jobstore"
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

	// otelhttp.NewTransport both creates a per-call child span and injects
	// the traceparent header - both come for free from the context already
	// carrying an active span by the time Query/Catalog's ctx reaches here
	// (see internal/tracing's own doc comment for the one thing that must
	// stay true for this to work: internal/tracing.Init must run before
	// this Provider is ever used).
	inst := &instance{baseURL: strings.TrimRight(baseURL, "/"), token: token,
		client: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}}
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

type functionsResponse struct {
	Functions []dataprovider.FunctionCatalog `json:"functions"`
}

func (i *instance) Functions(ctx context.Context) ([]dataprovider.FunctionCatalog, error) {
	var out functionsResponse
	if err := i.get(ctx, "/functions", nil, &out); err != nil {
		return nil, err
	}
	return out.Functions, nil
}

type warmRequest struct {
	Function string         `json:"function"`
	Args     map[string]any `json:"args"`
}

func (i *instance) RunFunction(ctx context.Context, name string, args map[string]any) (jobstore.Job, error) {
	var job jobstore.Job
	err := i.post(ctx, "/warm", warmRequest{Function: name, Args: args}, &job)
	return job, err
}

func (i *instance) JobStatus(ctx context.Context, id string) (jobstore.Job, error) {
	var job jobstore.Job
	err := i.get(ctx, "/warm", url.Values{"id": {id}}, &job)
	return job, err
}

var _ dataprovider.FunctionRunner = (*instance)(nil)

// remoteError builds a *dataprovider.RemoteError from a non-2xx response,
// preserving the remote's own status code so callers (e.g. polyglot's
// handlers) can decide how to react instead of every failure collapsing to
// a generic error. Best-effort decodes the remote's own PocketBase-shaped
// {"message": "..."} error envelope for the message text, falling back to
// a generic one if the body isn't shaped that way.
func remoteError(target string, resp *http.Response) error {
	var envelope struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&envelope)
	msg := envelope.Message
	if msg == "" {
		msg = fmt.Sprintf("%q returned status %d", target, resp.StatusCode)
	}
	return &dataprovider.RemoteError{StatusCode: resp.StatusCode, Message: msg}
}

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
		return remoteError(target, resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("http_sql: decoding response from %q: %w", target, err)
	}
	return nil
}

func (i *instance) post(ctx context.Context, path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("http_sql: encoding request body for %q: %w", path, err)
	}

	target := i.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("http_sql: building request to %q: %w", target, err)
	}
	req.Header.Set("Authorization", "Bearer "+i.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("http_sql: calling %q: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return remoteError(target, resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("http_sql: decoding response from %q: %w", target, err)
	}
	return nil
}
