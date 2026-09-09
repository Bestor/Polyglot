// Package warmer implements the proactive-warming binaries' shared logic:
// periodically calling a standalone Data API's POST /warm for every
// identifier listed in a file, so caches stay fresh without waiting on a
// live question to trigger a sync. Domain-agnostic - which function to
// call and what arg shape an identifier maps to is supplied by the caller
// (cmd/cachewarmer, cmd/chesscomwarmer, ...), not this package.
package warmer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls a Data API's POST /warm, using the same static bearer-token
// scheme as internal/mcpserver's Client.
type Client struct {
	baseURL   string
	authToken string
	http      *http.Client
}

func NewClient(baseURL, authToken string) *Client {
	return &Client{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		authToken: authToken,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

type warmRequest struct {
	Function string         `json:"function"`
	Args     map[string]any `json:"args"`
}

type warmJob struct {
	ID string `json:"id"`
}

// Warm calls POST /warm and returns the started job's id. It does not wait
// for the job to finish - /warm is already async, and the whole point of a
// warmer binary is to fire-and-forget across a watchlist, not block on any
// one identifier's sync.
func (c *Client) Warm(ctx context.Context, function string, args map[string]any) (string, error) {
	body, err := json.Marshal(warmRequest{Function: function, Args: args})
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/warm", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling data api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var job warmJob
	if err := json.Unmarshal(respBody, &job); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return job.ID, nil
}
