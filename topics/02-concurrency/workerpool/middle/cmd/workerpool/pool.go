// Package main - production-ready worker pool уровня middle.
// Полное обоснование решений и сравнение с junior-версией - в README.md.
package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Pool - worker pool с graceful shutdown, panic recovery, per-task context
// и неблокирующим возвратом ошибок.
type Pool struct {
	tasks chan task
	errs  chan error
	stop  chan struct{} // закрывается в начале Shutdown
	done  chan struct{} // закрывается на force-stop

	wg            sync.WaitGroup
	once          sync.Once
	droppedErrors atomic.Uint64
}

type task struct {
	ctx context.Context //nolint:containedctx // ctx живёт ровно одну задачу, как у http.Request.ctx
	fn  func(ctx context.Context) error
}

// New валидирует cfg и стартует cfg.Workers горутин.
// Паника на невалидном cfg - программерская ошибка.
func New(cfg Config) *Pool {
	cfg.validate()

	p := &Pool{
		tasks: make(chan task, cfg.QueueSize),
		errs:  make(chan error, cfg.ErrBuf),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}

	p.wg.Add(cfg.Workers)
	for range cfg.Workers {
		go p.worker()
	}
	return p
}

// Submit отправляет задачу в очередь.
//
// Возвращает:
//   - nil           - задача принята;
//   - ctx.Err()     - ctx отменён до того, как задача попала в очередь;
//   - ErrPoolClosed - Shutdown уже вызван;
//   - ErrNilTask    - fn == nil.
func (p *Pool) Submit(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return ErrNilTask
	}

	select {
	case <-p.stop:
		return ErrPoolClosed
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	t := task{ctx: ctx, fn: fn}
	select {
	case p.tasks <- t:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.stop:
		return ErrPoolClosed
	}
}

// Shutdown - graceful drain с force-stop fallback'ом.
//
// Возвращает:
//   - nil                 - drain успел до дедлайна;
//   - обёрнутый ctx.Err() - ctx истёк (errors.Is с DeadlineExceeded);
//   - ErrAlreadyShutdown  - повторный вызов.
func (p *Pool) Shutdown(ctx context.Context) error {
	called := false
	p.once.Do(func() {
		called = true
		close(p.stop)
	})
	if !called {
		return ErrAlreadyShutdown
	}

	drained := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		close(p.done)
		<-drained
		return fmt.Errorf("workerpool: дедлайн на drain истёк: %w", ctx.Err())
	}
}

// Errors - канал ошибок и паник от воркеров. НЕ закрывается на Shutdown:
// in-flight задачи могут писать после возврата Shutdown. Паники приходят
// как *PanicError.
func (p *Pool) Errors() <-chan error {
	return p.errs
}

// DroppedErrors - счётчик ошибок, не вместившихся в Errors().
func (p *Pool) DroppedErrors() uint64 {
	return p.droppedErrors.Load()
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			return
		case t := <-p.tasks:
			p.runTask(t)
		case <-p.stop:
			p.drainAndExit()
			return
		}
	}
}

func (p *Pool) drainAndExit() {
	for {
		select {
		case <-p.done:
			return
		case t := <-p.tasks:
			p.runTask(t)
		default:
			return
		}
	}
}

// runTask - отдельная функция, чтобы defer recover
// срабатывал на каждой задаче, а не один раз на всю жизнь воркера.
func (p *Pool) runTask(t task) {
	defer func() {
		if r := recover(); r != nil {
			p.sendErr(&PanicError{Recovered: r, Stack: debug.Stack()})
		}
	}()

	if err := t.ctx.Err(); err != nil {
		p.sendErr(fmt.Errorf("workerpool: ctx задачи отменён до запуска: %w", err))
		return
	}

	if err := t.fn(t.ctx); err != nil {
		p.sendErr(err)
	}
}

func (p *Pool) sendErr(err error) {
	select {
	case p.errs <- err:
	default:
		p.droppedErrors.Add(1)
	}
}
