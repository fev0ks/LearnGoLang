package main

import "sync"

type ProtectedMap[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

func NewProtectedMap[K comparable, V any]() *ProtectedMap[K, V] {
	return &ProtectedMap[K, V]{
		m:  make(map[K]V),
		mu: sync.RWMutex{},
	}
}

func (p *ProtectedMap[K, V]) Put(k K, v V) {
	p.mu.Lock()
	p.m[k] = v
	p.mu.Unlock()
}

func (p *ProtectedMap[K, V]) Get(k K) (v V, ok bool) {
	p.mu.RLock()
	v, ok = p.m[k]
	p.mu.RUnlock()
	return v, ok
}

func main() {
	pMap := NewProtectedMap[int, int]()
}
