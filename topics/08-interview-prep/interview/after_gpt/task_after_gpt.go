package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Result struct {
	ID   int
	Data string
}

type Fetcher struct {
	mu    sync.RWMutex
	cache map[int]Result
}

func NewFetcher() *Fetcher {
	return &Fetcher{cache: make(map[int]Result)}
}

// fetch simulates a slow IO call and respects context cancellation.
func (f *Fetcher) fetch(ctx context.Context, id int) (Result, error) {
	// Pretend it is IO with a cancellable wait.
	select {
	case <-time.After(50 * time.Millisecond):
		return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// FetchAll is a classic worker-pool implementation:
// - no goroutine leaks (all goroutines exit on ctx cancel or jobs close)
// - safe cache access
// - cancellable send to out
// - closes out when all workers are done
func (f *Fetcher) FetchAll(ctx context.Context, ids []int, workers int) (<-chan Result, <-chan error) {
	if workers <= 0 {
		workers = 1
	}

	out := make(chan Result, workers) // small buffer to reduce coupling
	errCh := make(chan error, 1)      // first error wins (you can change policy)

	jobs := make(chan int)

	// Producer: feed jobs or stop on ctx cancel.
	go func() {
		defer close(jobs)
		for _, id := range ids {
			select {
			case jobs <- id:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(workers)

	// Worker function.
	workerFn := func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case id, ok := <-jobs:
				if !ok {
					return
				}

				// Cache read (fast path).
				f.mu.RLock()
				r, ok := f.cache[id]
				f.mu.RUnlock()
				if ok {
					// Cancellable send (never block forever).
					select {
					case out <- r:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Slow path.
				r, err := f.fetch(ctx, id)
				if err != nil {
					// If ctx cancelled, just exit quietly.
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return
					}
					// Report first error non-blocking.
					select {
					case errCh <- err:
					default:
					}
					return
				}

				// Cache write.
				f.mu.Lock()
				f.cache[id] = r
				f.mu.Unlock()

				select {
				case out <- r:
				case <-ctx.Done():
					return
				}
			}
		}
	}

	for i := 0; i < workers; i++ {
		go workerFn()
	}

	// Closer: close out + errCh when workers are finished.
	go func() {
		wg.Wait()
		close(out)
		close(errCh)
	}()

	return out, errCh
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	f := NewFetcher()
	ids := []int{1, 2, 3, 2, 4, 5, 6}

	out, errCh := f.FetchAll(ctx, ids, 4)

	for r := range out {
		fmt.Println("result:", r)
	}

	// Optional: check if there was a non-context error.
	if err := <-errCh; err != nil {
		fmt.Println("error:", err)
	}
}
