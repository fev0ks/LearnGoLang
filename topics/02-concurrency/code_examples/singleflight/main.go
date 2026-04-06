package main

import (
	"context"
	"errors"
	"sync"
)

func main() {

}

type Call struct {
	val  any
	err  error
	done chan struct{}
}

type Singleflight struct {
	calls map[string]*Call
	mu    sync.Mutex
}

func NewSingleflight() *Singleflight {
	return &Singleflight{
		calls: make(map[string]*Call),
	}
}

func (s *Singleflight) Do(ctx context.Context, key string, task func(ctx context.Context) (any, error)) (any, error) {
	s.mu.Lock()

	if cl, ok := s.calls[key]; ok {
		s.mu.Unlock()
		return s.Wait(ctx, cl)
	}
	call := &Call{
		done: make(chan struct{}),
	}

	s.calls[key] = call
	s.mu.Unlock()

	go func() {
		defer func() {
			if v := recover(); v != nil {
				call.err = errors.New("error from single flight")
			}
			close(call.done)

			s.mu.Lock()
			delete(s.calls, key)
			s.mu.Unlock()
		}()
		call.val, call.err = task(ctx)
	}()

	return s.Wait(ctx, call)
}

func (s *Singleflight) Wait(ctx context.Context, call *Call) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		return call.val, call.err
	}
}
