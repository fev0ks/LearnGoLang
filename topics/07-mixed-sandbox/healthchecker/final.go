package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type ResponseStatus string

const (
	StatusOk       ResponseStatus = "ok"
	StatusNotOk    ResponseStatus = "not_ok"
	StatusCanceled ResponseStatus = "canceled"
)

type Checker struct {
	counter    atomic.Int64
	doer       Doer
	maxWorkers int
	stopAfter  int64
}

func NewChecker(doer Doer) *Checker {
	return &Checker{
		doer:       doer,
		maxWorkers: 4,
		stopAfter:  2,
	}
}

type Response struct {
	Url    string
	Status ResponseStatus
	Err    error
}

func (c *Checker) Check(ctx context.Context, urls <-chan string) <-chan Response {
	checkerCtx, cancel := context.WithCancel(ctx)
	wg := sync.WaitGroup{}
	wg.Add(c.maxWorkers)

	out := make(chan Response)
	for range c.maxWorkers {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-checkerCtx.Done():
					return
				case url, ok := <-urls:
					if !ok {
						return
					}
					resp := c.fetchStatus(checkerCtx, url)
					if resp.Status == StatusOk && c.counter.Add(1) == c.stopAfter {
						cancel()
					}
					out <- resp
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		cancel()
		close(out)
	}()

	return out
}

func (c *Checker) fetchStatus(ctx context.Context, url string) Response {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Response{Url: url, Status: StatusNotOk, Err: err}
	}
	resp, err := c.doer.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, context.Canceled) {
			return Response{Url: url, Status: StatusCanceled, Err: ctx.Err()}
		}
		return Response{Url: url, Status: StatusNotOk, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Response{Url: url, Status: StatusNotOk}
	}

	return Response{Url: url, Status: StatusOk}
}
