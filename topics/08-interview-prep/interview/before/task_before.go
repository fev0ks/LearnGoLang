package main

import (
	"context"
	"fmt"
	"time"
)

type Result struct {
	ID   int
	Data string
}

type Fetcher struct {
	cache map[int]Result
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()

	f := NewFetcher()

	ids := []int{1, 2, 3, 2, 4, 1, 5, 6, 7, 8, 9}

	start := time.Now()
	defer fmt.Println("duration", time.Since(start))

	for r := range f.FetchAll(ids) {
		fmt.Println(r)
	}
}

func NewFetcher() *Fetcher {
	return &Fetcher{}
}

func (f *Fetcher) doRequest(id int) Result {
	time.Sleep(50 * time.Millisecond)
	return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}
}

func (f *Fetcher) FetchAll(ids []int) chan Result {
	var out chan Result
	var jobs chan int

	go func() {
		defer close(jobs)
		for _, id := range ids {
			jobs <- id
		}
	}()

	for i := 0; i < 4; i++ {
		go func(worker int) {

			for id := range jobs {
				r, ok := f.cache[id]
				if ok {
					out <- r
					continue
				}

				r = f.doRequest(id)

				f.cache[id] = r

				out <- r
			}
		}(i)
	}

	return out
}
