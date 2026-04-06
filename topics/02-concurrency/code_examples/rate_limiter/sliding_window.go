package main

import (
	"fmt"
	"sync"
	"time"
)

type SlidingWindowLimiter struct {
	mu        sync.Mutex
	interval  time.Duration // Размер окна
	limit     int           // Максимум запросов за окно
	timestamp []int64       // Храним временные метки запросов
}

func NewSlidingWindowLimiter(interval time.Duration, limit int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		interval:  interval,
		limit:     limit,
		timestamp: []int64{},
	}
}

func (r *SlidingWindowLimiter) Allow() bool {
	now := time.Now().UnixNano()
	windowStart := now - r.interval.Nanoseconds()

	// Очищаем старые записи
	var newTimestamps []int64

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ts := range r.timestamp {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	r.timestamp = newTimestamps

	// Проверяем лимит
	if len(r.timestamp) < r.limit {
		r.timestamp = append(r.timestamp, now)
		return true
	}

	return false
}

func main() {
	limiter := NewSlidingWindowLimiter(time.Second, 5) // 5 запросов в секунду
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 15; i++ {
			if limiter.Allow() {
				fmt.Println("1 Request", i+1, "allowed")
			} else {
				fmt.Println("1 Request", i+1, "blocked")
			}
			time.Sleep(100 * time.Millisecond) // Симуляция входящих запросов
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 15; i++ {
			if limiter.Allow() {
				fmt.Println("2 Request", i+1, "allowed")
			} else {
				fmt.Println("2 Request", i+1, "blocked")
			}
			time.Sleep(150 * time.Millisecond) // Симуляция входящих запросов
		}
	}()
	wg.Wait()
}
