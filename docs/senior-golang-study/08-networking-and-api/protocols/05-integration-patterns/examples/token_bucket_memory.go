package ratelimit

import (
	"context"
	"sync"
	"time"
)

// InMemoryTokenBucketLimiter models burst + refill behaviour.
//
// Pros:
// - best fit when you want small bursts but smooth sustained traffic
// - intuitive production model for APIs and edge throttling
// - Retry-After maps naturally to refill time
//
// Cons:
// - more stateful and math-heavy than fixed window
// - still process-local only in this in-memory form
// - distributed strictness requires Lua/scripts or another atomic backend
type InMemoryTokenBucketLimiter struct {
	mu         sync.Mutex
	capacity   float64
	refillRate float64
	now        func() time.Time
	buckets    map[string]tokenBucketState
}

type tokenBucketState struct {
	tokens float64
	last   time.Time
}

// NewInMemoryTokenBucketLimiter creates an in-memory token bucket.
//
// refillRate is tokens per second.
func NewInMemoryTokenBucketLimiter(capacity int, refillRate float64) *InMemoryTokenBucketLimiter {
	if capacity <= 0 {
		capacity = 1
	}
	if refillRate <= 0 {
		refillRate = 1
	}
	return &InMemoryTokenBucketLimiter{
		capacity:   float64(capacity),
		refillRate: refillRate,
		now:        time.Now,
		buckets:    make(map[string]tokenBucketState),
	}
}

// Allow consumes one token if available, refilling the bucket over time.
func (l *InMemoryTokenBucketLimiter) Allow(_ context.Context, key string) (Decision, error) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	st, ok := l.buckets[key]
	if !ok {
		st = tokenBucketState{
			tokens: l.capacity,
			last:   now,
		}
	}

	elapsed := now.Sub(st.last).Seconds()
	if elapsed > 0 {
		st.tokens += elapsed * l.refillRate
		if st.tokens > l.capacity {
			st.tokens = l.capacity
		}
		st.last = now
	}

	if st.tokens >= 1 {
		st.tokens--
		l.buckets[key] = st
		return Decision{Allowed: true}, nil
	}

	missing := 1 - st.tokens
	retryAfter := time.Duration((missing / l.refillRate) * float64(time.Second))
	if retryAfter < 0 {
		retryAfter = 0
	}
	l.buckets[key] = st
	return Decision{Allowed: false, RetryAfter: retryAfter}, nil
}
