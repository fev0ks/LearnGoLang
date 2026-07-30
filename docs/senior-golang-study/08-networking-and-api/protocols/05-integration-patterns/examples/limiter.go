// Package ratelimit contains small reference rate-limiter implementations
// used in study notes for comparing trade-offs between algorithms.
package ratelimit

import (
	"context"
	"time"
)

// Decision is the result of one limiter check.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Limiter is a narrow contract that keeps middleware decoupled from the
// concrete algorithm or backend.
type Limiter interface {
	Allow(ctx context.Context, key string) (Decision, error)
}
