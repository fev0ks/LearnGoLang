package main

import "sync"

type WorkerPool struct {
	tasks  chan func()
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

func New(workers int) *WorkerPool {
	p := &WorkerPool{
		tasks: make(chan func(), 100),
	}
	for range workers {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		task()
	}
}

func (p *WorkerPool) AddTask(f func()) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	p.tasks <- f
}

func (p *WorkerPool) Close() {
	p.mu.Lock()
	p.closed = true
	close(p.tasks)
	p.mu.Unlock()
	p.wg.Wait()
}
