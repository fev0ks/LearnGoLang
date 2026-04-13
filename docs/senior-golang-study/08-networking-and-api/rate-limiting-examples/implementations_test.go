package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryFixedWindowLimiter(t *testing.T) {
	start := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	l := NewInMemoryFixedWindowLimiter(2, time.Minute)
	l.now = func() time.Time { return start }

	ctx := context.Background()
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("first hit should pass")
	}
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("second hit should pass")
	}
	if d, _ := l.Allow(ctx, "ip-1"); d.Allowed {
		t.Fatal("third hit should be blocked")
	}
}

func TestInMemorySlidingWindowLimiter(t *testing.T) {
	start := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	now := start
	l := NewInMemorySlidingWindowLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	ctx := context.Background()
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("first hit should pass")
	}
	now = now.Add(10 * time.Second)
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("second hit should pass")
	}
	now = now.Add(10 * time.Second)
	if d, _ := l.Allow(ctx, "ip-1"); d.Allowed {
		t.Fatal("third hit inside the window should be blocked")
	}
	now = start.Add(61 * time.Second)
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("hit should pass once the oldest event leaves the window")
	}
}

func TestInMemoryTokenBucketLimiter(t *testing.T) {
	start := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	now := start
	l := NewInMemoryTokenBucketLimiter(2, 1)
	l.now = func() time.Time { return now }

	ctx := context.Background()
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("first token should be available")
	}
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("second token should be available")
	}
	if d, _ := l.Allow(ctx, "ip-1"); d.Allowed {
		t.Fatal("third hit should be blocked with empty bucket")
	}
	now = now.Add(1100 * time.Millisecond)
	if d, _ := l.Allow(ctx, "ip-1"); !d.Allowed {
		t.Fatal("bucket should refill after one second")
	}
}
