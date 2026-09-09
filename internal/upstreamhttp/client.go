// Package upstreamhttp is the shared rate-limited-GET-with-429-retry
// mechanics every upstream data-source client (HenrikDev, chess.com, ...)
// needs: apply internal/ratelimit's token bucket, retry a 429 in place
// (honoring a Retry-After header when the upstream sends one, falling back
// to exponential backoff otherwise), and map any other non-2xx response to
// a typed APIError. Deliberately knows nothing about any specific
// upstream's URL shape, auth scheme, or response bodies - those stay in
// each concrete client (e.g. internal/valorant/data_sources/henrik),
// passed in here as a plain URL and header map.
package upstreamhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"val-analyzer/internal/ratelimit"
)

// Client performs rate-limited GETs against one upstream API, retrying a
// 429 in place rather than failing the whole call immediately.
type Client struct {
	http        *http.Client
	limiter     *ratelimit.Limiter
	source      string // log-line prefix only, e.g. "henrik"/"chesscom" - no other behavior depends on it
	maxRetries  int
	backoffBase time.Duration
	backoffCap  time.Duration
}

// Option configures a Client beyond its required constructor args.
type Option func(*Client)

// WithMaxRetries overrides how many times a 429 is retried before giving up
// and returning an APIError. Default 5.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithBackoff overrides the exponential-backoff tuning used when a 429
// response carries no usable Retry-After header, and the cap applied to
// any wait duration regardless of source. Default base 2s, cap 60s.
func WithBackoff(base, cap time.Duration) Option {
	return func(c *Client) { c.backoffBase = base; c.backoffCap = cap }
}

// NewClient builds a Client. source identifies the upstream purely for log
// lines (e.g. "henrik"/"chesscom"). timeout bounds a single HTTP round
// trip, independent of the overall retry loop.
func NewClient(source string, limiter *ratelimit.Limiter, timeout time.Duration, opts ...Option) *Client {
	c := &Client{
		http:        &http.Client{Timeout: timeout},
		limiter:     limiter,
		source:      source,
		maxRetries:  5,
		backoffBase: 2 * time.Second,
		backoffCap:  60 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get performs a rate-limited GET against url, applying headers verbatim
// (a caller supplies its own Authorization/User-Agent/Accept as needed),
// and returns the raw response body - so a caller that wants to persist
// the full payload (e.g. matches.raw_json) can do so without a second
// round trip. If out is non-nil, the body is also JSON-decoded into it. A
// 429 response is retried in place (up to maxRetries times, honoring a
// Retry-After header when present, exponential backoff otherwise) rather
// than immediately failing the call.
func (c *Client) Get(ctx context.Context, url string, headers map[string]string, out any) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		start := time.Now()
		slog.Info(c.source+": request", "url", url)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			slog.Error(c.source+": request failed", "url", url, "error", err, "duration_ms", time.Since(start).Milliseconds())
			return nil, err
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			slog.Error(c.source+": reading response body failed", "url", url, "error", readErr, "duration_ms", time.Since(start).Milliseconds())
			return nil, readErr
		}

		duration := time.Since(start)

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			wait := c.retryAfterDuration(resp.Header, attempt)
			slog.Warn(c.source+": rate limited, pausing before retry", "url", url, "wait", wait, "attempt", attempt+1, "duration_ms", duration.Milliseconds())
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if resp.StatusCode >= 400 {
			slog.Warn(c.source+": request returned error status", "url", url, "status", resp.StatusCode, "duration_ms", duration.Milliseconds())
			return nil, &APIError{StatusCode: resp.StatusCode, Message: string(body)}
		}

		slog.Info(c.source+": request complete", "url", url, "status", resp.StatusCode, "bytes", len(body), "duration_ms", duration.Milliseconds())

		if out != nil {
			if err := json.Unmarshal(body, out); err != nil {
				return nil, fmt.Errorf("decoding %s response: %w", c.source, err)
			}
		}

		return body, nil
	}
}

// retryAfterDuration decides how long to pause before retrying a 429,
// honoring a Retry-After header (seconds or an HTTP date) when present and
// falling back to exponential backoff based on attempt otherwise. Always
// capped at c.backoffCap.
func (c *Client) retryAfterDuration(header http.Header, attempt int) time.Duration {
	if v := header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return c.capDuration(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			if d := time.Until(t); d > 0 {
				return c.capDuration(d)
			}
		}
	}

	return c.capDuration(c.backoffBase * time.Duration(1<<attempt))
}

func (c *Client) capDuration(d time.Duration) time.Duration {
	if d > c.backoffCap {
		return c.backoffCap
	}
	return d
}
