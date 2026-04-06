package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type MutexCounter struct {
	counter int
	mu      sync.RWMutex
}

func (m *MutexCounter) Inc() {
	m.mu.Lock()
	m.counter++
	m.mu.Unlock()
}

func (m *MutexCounter) Get() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counter
}

type AtomicCounter struct {
	counter atomic.Int32
	_       [60]byte // alignment
}

func (c *AtomicCounter) Inc() {
	c.counter.Add(1)
}

func (c *AtomicCounter) Get() int32 {
	return c.counter.Load()
}

type ShardingCounter struct {
	shards []AtomicCounter
}

func (s *ShardingCounter) Inc(idx int) {
	s.shards[idx].Inc()
}

func (s *ShardingCounter) Get() int32 {
	var count int32
	for i := 0; i < len(s.shards); i++ {
		count += s.shards[i].Get()
	}
	return count
}

// 100
// MutexCounter     149998	      7806 ns/op
// AtomicCounter    204369	      5842 ns/op
// ShardingCounter  1000000	      1347 ns/op
// AlignmentCounter 43346426	  26.30 ns/op

// NumCPU 16
// MutexCounter      943411	       1262 ns/op
// AtomicCounter    1304827	       978.4 ns/op
// ShardingCounter  1452512	       999.2 ns/op
// AlignmentCounter 149124004	   7.831 ns/op

// 10
// MutexCounter     1509852	       790.0 ns/op
// AtomicCounter    3888363	       333.3 ns/op
// ShardingCounter  3679748	       311.8 ns/op
// AlignmentCounter 267500436	   4.983 ns/op

func BenchmarkCounter(b *testing.B) {
	wg := sync.WaitGroup{}
	n := 100
	//n := runtime.NumCPU()
	fmt.Println(n)
	wg.Add(n)

	//counter := &ShardingCounter{shards: make([]AtomicCounter, n)}
	counter := AtomicCounter{}
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < b.N; j++ {
				//counter.Inc(i)
				counter.Inc()
				if j%1000 == 0 {
					counter.Get()
				}
			}
		}()
	}
	wg.Wait()
}
