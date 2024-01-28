package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Request struct {
	Payload string
}

type Client interface {
	SendRequest(ctx context.Context, request Request) error
}

type client struct {
}

func (c client) SendRequest(ctx context.Context, request Request) error {
	fmt.Println("send request", request.Payload)
	return nil
}

func main() {
	ctx := context.Background()
	c := client{}
	requests := make([]Request, 1000)
	for i := 0; i < 1000; i++ {
		requests[i] = Request{Payload: strconv.Itoa(i)}
	}
	log := logrus.New()
	makeBatchApiCalls(ctx, c, log, requests)
}

// TODO: rate limit api calls
// 1. In fly requests limit
// 2. Per second limit
func makeBatchApiCalls(ctx context.Context, c Client, log *logrus.Logger, requests []Request) {
	rateLimiter := newRateLimiter(100)
	wg := sync.WaitGroup{}
	for _, r := range requests {
		r := r
		wg.Add(1)
		go func() {
			defer wg.Done()
			rateLimiter.wait()
			err := c.SendRequest(ctx, r)
			if err != nil {
				log.WithError(err).Error("send request")
			}
		}()
	}
	wg.Wait()
}

type inFlyLimiter struct {
	pool chan []struct{}
}

func newInFlyLimiter(poolSize int) *inFlyLimiter {
	return &inFlyLimiter{pool: make(chan []struct{}, poolSize)}
}

func (ifl *inFlyLimiter) wait() {

}

type rateLimiter struct {
	ch *time.Ticker
}

func (rl *rateLimiter) wait() {
	<-rl.ch.C
}

func newRateLimiter(rps int) *rateLimiter {
	interval := time.Second / time.Duration(rps)
	tck := time.NewTicker(interval)
	return &rateLimiter{ch: tck}
}
