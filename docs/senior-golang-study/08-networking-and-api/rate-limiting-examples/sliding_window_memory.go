package ratelimit

import (
	"context"
	"sync"
	"time"
)

// InMemorySlidingWindowLimiter keeps one timestamp per accepted hit.
//
// Pros:
// - much fairer than fixed window
// - avoids the sharp burst at window boundaries
// - easy to understand and useful as a reference algorithm
//
// Cons:
// - more memory and CPU than fixed window because old hits must be pruned
// - still process-local only
// - naive timestamp lists do not scale well for very high throughput
type InMemorySlidingWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
}

// NewInMemorySlidingWindowLimiter creates an in-memory sliding-window limiter.
func NewInMemorySlidingWindowLimiter(limit int, window time.Duration) *InMemorySlidingWindowLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &InMemorySlidingWindowLimiter{
		limit:  limit,
		window: window,
		now:    time.Now,
		hits:   make(map[string][]time.Time),
	}
}

// Allow records one hit if the number of hits inside the window stays <= limit.
func (l *InMemorySlidingWindowLimiter) Allow(_ context.Context, key string) (Decision, error) {
	now := l.now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.hits[key]
	keep := hits[:0]
	for _, ts := range hits {
		if ts.After(cutoff) {
			keep = append(keep, ts)
		}
	}
	hits = keep

	if len(hits) >= l.limit {
		retryAfter := hits[0].Add(l.window).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		l.hits[key] = hits
		return Decision{Allowed: false, RetryAfter: retryAfter}, nil
	}

	hits = append(hits, now)
	l.hits[key] = hits
	return Decision{Allowed: true}, nil
}
