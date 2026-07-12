package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client calls the polyglot Data API over HTTP, using the same static
// bearer-token scheme as internal/polyglot's own auth middleware.
type Client struct {
	baseURL   string
	authToken string
	http      *http.Client
}

func NewClient(baseURL, authToken string) *Client {
	return &Client{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		authToken: authToken,
		http:      &http.Client{},
	}
}

// Call turns a tool call's arguments into an HTTP request for op (query
// parameters for a body-less GET, or the whole args map as a JSON body),
// and returns the raw response status/body for the caller to interpret.
func (c *Client) Call(ctx context.Context, op Operation, args map[string]any) (int, []byte, error) {
	req, err := c.buildRequest(ctx, op, args)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("reading response body: %w", err)
	}

	return resp.StatusCode, respBody, nil
}

func (c *Client) buildRequest(ctx context.Context, op Operation, args map[string]any) (*http.Request, error) {
	if op.HasBody {
		encoded, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, op.Method, c.baseURL+op.Path, bytes.NewReader(encoded))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	u, err := url.Parse(c.baseURL + op.Path)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for _, p := range op.Params {
		if v, ok := args[p.Name]; ok {
			q.Set(p.Name, fmt.Sprint(v))
		}
	}
	u.RawQuery = q.Encode()

	return http.NewRequestWithContext(ctx, op.Method, u.String(), nil)
}
