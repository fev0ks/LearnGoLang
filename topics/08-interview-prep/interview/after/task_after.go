package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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

//утечки памяти //нет eviction //нет TTL //рост latency // OOM

var errSome = errors.New("some error")

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	f := NewFetcher()
	var ids []int
	for i := 0; i < 100; i++ {
		ids = append(ids, i)
	}
	start := time.Now()
	defer func() {
		fmt.Println("duration", time.Since(start))
	}()
	defer fmt.Println("duration2", time.Since(start))

	out, errCh := f.FetchAll(ctx, ids)

	for r := range out {
		fmt.Println("result:", r)
	}

	// Optional: check if there was a non-context error.
	if err := <-errCh; err != nil {
		fmt.Println("error:", err)
	}

}

func NewFetcher() *Fetcher {
	return &Fetcher{cache: make(map[int]Result)}
}

//Как ты обычно реализуешь ретраи при сетевых запросах и на что обращаешь внимание?
//“Ретраи делаю только для transient ошибок, с экспоненциальным backoff и jitter,
//ограничиваю количеством попыток и обязательно учитываю deadline контекста. Для небезопасных операций — только с идемпотентностью.”
//“Можно ли ретраить любой запрос?”

func (f *Fetcher) fetch(ctx context.Context, id int) (Result, error) {
	// Pretend it is IO with a cancellable wait.
	select {
	case <-time.After(50 * time.Millisecond):
		if rand.Intn(10) > 5 {
			return Result{}, errSome
		}
		return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}, nil
	case <-ctx.Done():
		fmt.Println("ctx done")
		return Result{}, ctx.Err()
	}
}

func (f *Fetcher) FetchAll(ctx context.Context, ids []int) (<-chan Result, <-chan error) {
	out := make(chan Result) // unbuffered
	//? Буфер len(ids) здесь — не оптимизация, а средство безопасности и упрощения конкурентной модели.
	//Он убирает жёсткую синхронизацию между воркерами и consumer и снижает риск дедлоков.
	//var out chan Result // unbuffered
	jobs := make(chan int)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	// Producer
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

	//singleflight — про дедупликацию одинаковой работы,
	//errgroup — про параллельное выполнение и сбор ошибок.
	// Workers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()

			for id := range jobs {
				// Fast path: read cache
				f.mu.RLock()
				r, ok := f.cache[id]
				f.mu.RUnlock()
				if ok {
					select {
					case out <- r:
					case <-ctx.Done():
						fmt.Println("ctx done")
						return
					}
					continue
				}

				r, err := f.fetch(ctx, id)
				if err != nil {
					// If ctx cancelled, just exit quietly.
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return
					}
					if errors.Is(err, errSome) {
						cancel()
					}
					// Report first error non-blocking.
					select {
					case errCh <- err:
					default:
					}
					return
				}

				// Save cache
				f.mu.Lock()
				f.cache[id] = r
				f.mu.Unlock()

				select {
				case out <- r:
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}

	// Closer
	go func() {
		wg.Wait()
		cancel()
		close(out)
		close(errCh)
	}()

	return out, errCh
}
