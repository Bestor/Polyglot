package cachewarmer

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

// Client calls polyglot's POST /warm, using the same static bearer-token
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

// Warm calls POST /warm (against cmd/valorantapi - the only service with
// a /warm endpoint now, see internal/polyglot/routes.go's doc comment) and
// returns the started job's id. It does not wait for the job to finish -
// /warm is already async, and cachewarmer's whole point is to
// fire-and-forget across a player list, not block on any one player's
// sync.
func (c *Client) Warm(ctx context.Context, function, playerTag string) (string, error) {
	body, err := json.Marshal(warmRequest{
		Function: function,
		Args:     map[string]any{"player_tag": playerTag},
	})
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
		return "", fmt.Errorf("calling polyglot: %w", err)
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
