package try

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

type Pool struct {
	log *slog.Logger
	wg  sync.WaitGroup
	m   sync.Mutex

	runningWorkers atomic.Int64

	gracefulShutdown chan struct{}
	forcedShutdown   chan struct{}

	shutdownOnce sync.Once

	tasks  chan Task
	errors chan error

	errTasks     atomic.Uint64
	droppedTasks atomic.Uint64

	nextWorkerId atomic.Int64

	workerStop chan struct{}
}

type Task struct {
	ctx context.Context
	f   func(ctx context.Context)
}

func NewPool(ctx context.Context, workersCount int, bufferSize int) *Pool {
	pool := &Pool{
		log:    slog.Default(),
		tasks:  make(chan Task, bufferSize),
		errors: make(chan error, bufferSize),

		forcedShutdown:   make(chan struct{}),
		gracefulShutdown: make(chan struct{}),
		workerStop:       make(chan struct{}),
	}

	pool.wg.Add(workersCount)
	for i := 0; i < workersCount; i++ {
		go pool.worker(ctx)
	}
	return pool
}

func (p *Pool) Submit(ctx context.Context, fn func(ctx context.Context)) error {
	task := Task{
		ctx: ctx,
		f:   fn,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.gracefulShutdown:
		return errors.New("workerPool is closed")
	case p.tasks <- task:
		return nil
	default:
		p.droppedTasks.Add(1)
		return errors.New("workerPool is full")
	}
}

func (p *Pool) Resize(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("workerPool must be > 0")
	}

	p.m.Lock()
	defer p.m.Unlock()

	select {
	case <-p.gracefulShutdown:
		return 0, errors.New("workerPool is closed")
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	currentN := int(p.runningWorkers.Load())
	if n == currentN {
		return currentN, nil
	}

	delta := n - currentN

	if delta == 0 {
		return currentN, nil
	}

	if delta > 0 {
		p.wg.Add(delta)
		for i := 0; i < delta; i++ {
			go p.worker(ctx)
		}
		return n, nil
	}

	if delta < 0 {
		for i := 0; i < -delta; i++ {
			select {
			case p.workerStop <- struct{}{}:
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-p.gracefulShutdown:
				return 0, errors.New("workerPool is closed")
			}
		}
	}

	newN := int(p.runningWorkers.Load())
	return newN, nil
}

func (p *Pool) GetErrorsChan() <-chan error {
	return p.errors
}

func (p *Pool) GetFailedTasks() uint64 {
	return p.errTasks.Load()
}

func (p *Pool) GetDroppedTasks() uint64 {
	return p.droppedTasks.Load()
}

func (p *Pool) Shutdown(ctx context.Context) error {
	closed := false
	p.shutdownOnce.Do(func() {
		close(p.gracefulShutdown)
		closed = true
	})
	if !closed {
		return errors.New("pool is shut down")
	}

	finished := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(finished)
	}()

	select {
	case <-ctx.Done():
		close(p.forcedShutdown)
		<-finished
		return ctx.Err()
	case <-finished:
		return nil
	}

}

func (p *Pool) worker(ctx context.Context) {
	defer p.wg.Done()
	currentWorkerId := p.nextWorkerId.Add(1)
	p.runningWorkers.Add(1)
	defer p.runningWorkers.Add(-1)

	for {
		select {
		case <-ctx.Done():
			p.log.Info("context canceled, worker stopped: %d", currentWorkerId)
			return
		case <-p.gracefulShutdown:
			p.log.Info("workerPool shutting down, worker is stopping: %d", currentWorkerId)
			p.graceShutdown(currentWorkerId)
			return
		case <-p.workerStop:
			p.log.Info("worker stopped: %d", currentWorkerId)
			return
		case task := <-p.tasks:
			p.runTask(task, currentWorkerId)
		}
	}
}

func (p *Pool) graceShutdown(currentWorkerId int64) {
	for {
		select {
		case <-p.forcedShutdown:
			return
		case t := <-p.tasks:
			p.runTask(t, currentWorkerId)
		default:
			return
		}
	}
}

func (p *Pool) runTask(t Task, wid int64) {
	defer func() {
		if err := recover(); err != nil {
			p.log.Warn("worker %d task panic: %s", wid, err)
			p.ErrorHandler(&PanicError{Recovered: err, Stack: debug.Stack()})
		}
	}()
	if err := t.ctx.Err(); err != nil {
		p.log.Warn("worker %d task context error: %s", wid, err)
		p.ErrorHandler(err)
	}
}

type PanicError struct {
	Recovered any
	Stack     []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("workerPool task panic: %v", e.Recovered)
}

func (p *Pool) ErrorHandler(err error) {
	select {
	case p.errors <- err:
	default:
		p.errTasks.Add(1)
	}
}
