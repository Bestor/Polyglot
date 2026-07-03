// Package ratelimit provides a process-wide rate limiter for outbound
// data-source API calls, since providers like HenrikDev cap requests per
// minute regardless of which internal caller triggered the request.
package ratelimit

import (
	"context"

	"golang.org/x/time/rate"
)

type Limiter struct {
	rl *rate.Limiter
}

// NewLimiter creates a limiter allowing requestsPerMinute tokens per minute,
// with up to burst tokens available immediately.
func NewLimiter(requestsPerMinute, burst int) *Limiter {
	perSecond := rate.Limit(float64(requestsPerMinute) / 60.0)
	return &Limiter{rl: rate.NewLimiter(perSecond, burst)}
}

// Wait blocks until a token is available or ctx is done.
func (l *Limiter) Wait(ctx context.Context) error {
	return l.rl.Wait(ctx)
}
