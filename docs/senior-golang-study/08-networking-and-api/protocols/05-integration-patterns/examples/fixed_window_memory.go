package ratelimit

import (
	"context"
	"sync"
	"time"
)

// InMemoryFixedWindowLimiter is the simplest reference implementation.
//
// Pros:
// - tiny and easy to reason about
// - cheap per request
// - good enough for MVPs and local tests
//
// Cons:
// - boundary burst problem: traffic can spike around window edges
// - process-local only, so replicas do not share state
// - not suitable as the final production choice in a multi-instance deployment
type InMemoryFixedWindowLimiter struct {
	mu     sync.Mutex
	limit  int64
	window time.Duration
	now    func() time.Time
	state  map[string]fixedWindowState
}

type fixedWindowState struct {
	bucket int64
	count  int64
}

// NewInMemoryFixedWindowLimiter creates an in-memory fixed-window limiter.
func NewInMemoryFixedWindowLimiter(limit int, window time.Duration) *InMemoryFixedWindowLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &InMemoryFixedWindowLimiter{
		limit:  int64(limit),
		window: window,
		now:    time.Now,
		state:  make(map[string]fixedWindowState),
	}
}

// Allow records one hit in the current fixed window.
func (l *InMemoryFixedWindowLimiter) Allow(_ context.Context, key string) (Decision, error) {
	now := l.now()
	windowNanos := l.window.Nanoseconds()
	bucket := now.UnixNano() / windowNanos
	retryAfter := l.window - time.Duration(now.UnixNano()%windowNanos)
	if retryAfter <= 0 {
		retryAfter = l.window
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	st := l.state[key]
	if st.bucket != bucket {
		st = fixedWindowState{bucket: bucket}
	}
	st.count++
	l.state[key] = st

	if st.count > l.limit {
		return Decision{Allowed: false, RetryAfter: retryAfter}, nil
	}
	return Decision{Allowed: true}, nil
}
