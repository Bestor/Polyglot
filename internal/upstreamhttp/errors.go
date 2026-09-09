package upstreamhttp

import "fmt"

// APIError reports a non-2xx HTTP response from an upstream data source
// API, preserving its status code and raw body.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("upstream api: status %d: %s", e.StatusCode, e.Message)
}

func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

func (e *APIError) IsRateLimited() bool {
	return e.StatusCode == 429
}
