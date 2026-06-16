// Package main - production-ready worker pool уровня senior.
// Полное обоснование решений и сравнение с middle-версией - в README.md.
package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Pool - worker pool с graceful shutdown, panic recovery, per-task context,
// неблокирующим возвратом ошибок, выбираемым backpressure (Config.OnFull)
// и динамическим resize'ом числа воркеров (Resize).
type Pool struct {
	cfg     Config
	tasks   chan task
	errs    chan error
	stop    chan struct{} // закрывается в начале Shutdown
	done    chan struct{} // закрывается на force-stop
	quitOne chan struct{} // shrink в Resize: K send'ов = K воркеров выйдут (close нельзя - broadcast)

	wg            sync.WaitGroup
	once          sync.Once
	droppedErrors atomic.Uint64

	droppedTasks   atomic.Uint64
	workersRunning atomic.Int64
	nextWorkerID   atomic.Int64
	resizeMu       sync.Mutex
}

// workerIDKey - приватный тип-ключ для context.Value, чтобы worker_id
// нельзя было перезаписать снаружи (idiomatic Go).
type workerIDKey struct{}

// WorkerID достаёт worker_id из ctx внутри задачи.
// Полезно для structured-логов: slog.Info("...", "worker_id", id).
func WorkerID(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(workerIDKey{}).(int64)
	return v, ok
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
		cfg:     cfg,
		tasks:   make(chan task, cfg.QueueSize),
		errs:    make(chan error, cfg.ErrBuf),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		quitOne: make(chan struct{}),
	}

	// workersRunning инкрементим ДО старта горутин, чтобы concurrent
	// Resize видел корректное значение, а не stale (см. README, секция
	// «Resize: реализация и race-семантика»).
	p.wg.Add(cfg.Workers)
	p.workersRunning.Add(int64(cfg.Workers))
	for range cfg.Workers {
		go p.worker()
	}
	return p
}

// Submit отправляет задачу в очередь.
//
// Поведение при полной очереди определяется Config.OnFull:
//   - OnFullBlock      - блокирует до слота / ctx.Done() / Shutdown (default);
//   - OnFullFailFast   - вернуть ErrQueueFull сразу, без ожидания;
//   - OnFullDropNewest - молча дропнуть, инкрементить DroppedTasks, вернуть nil.
//
// Возвращает:
//   - nil           - задача принята (либо тихо дропнута при OnFullDropNewest);
//   - ctx.Err()     - ctx отменён до того, как задача попала в очередь;
//   - ErrPoolClosed - Shutdown уже вызван;
//   - ErrNilTask    - fn == nil;
//   - ErrQueueFull  - OnFullFailFast и очередь полная.
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

	switch p.cfg.OnFull {
	case OnFullFailFast:
		select {
		case p.tasks <- t:
			return nil
		default:
			return ErrQueueFull
		}

	case OnFullDropNewest:
		select {
		case p.tasks <- t:
			return nil
		default:
			p.droppedTasks.Add(1)
			return nil
		}
	}

	// OnFullBlock (default) - ждём слот / ctx / stop.
	select {
	case p.tasks <- t:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.stop:
		return ErrPoolClosed
	}
}

// Resize меняет число воркеров. Синхронный, под resizeMu.
//
// Возвращает (фактическое число воркеров после изменения, ошибка):
//   - (n, nil)                  - изменение применено;
//   - (current, ErrPoolClosed)  - Shutdown уже был вызван (или пошёл во время Resize);
//   - (0, ErrResizeNonPositive) - n <= 0.
//
// При shrink - лишние воркеры выходят кооперативно после текущей задачи.
// Какие именно из живых воркеров выйдут - undefined (Go random select).
// Send в quitOne блокирующий: Resize ждёт пока все N воркеров примут сигнал.
// Это «сигнал отправлен», не «воркер уже завершился» - текущая задача
// может ещё доисполняться.
func (p *Pool) Resize(n int) (int, error) {
	if n <= 0 {
		return 0, ErrResizeNonPositive
	}

	p.resizeMu.Lock()
	defer p.resizeMu.Unlock()

	select {
	case <-p.stop:
		return int(p.workersRunning.Load()), ErrPoolClosed
	default:
	}

	current := int(p.workersRunning.Load())
	delta := n - current

	if delta == 0 {
		return current, nil
	}

	if delta > 0 {
		p.wg.Add(delta)
		p.workersRunning.Add(int64(delta))
		for range delta {
			go p.worker()
		}
		return n, nil
	}

	// Shrink: посылаем -delta сигналов и СРАЗУ после успешного send'а
	// декрементируем workersRunning. Иначе два concurrent Resize'а читают
	// одинаковое stale значение, оба считают одинаковую delta - и пул
	// усыхает вдвое сильнее, чем нужно.
	//
	// Параллельно следим за p.stop - если Shutdown пошёл, воркеры могут
	// выйти через drain-ветку, и наш send в unbuffered quitOne повис бы
	// навсегда без этой страховки.
	sent := 0
	for sent < -delta {
		select {
		case p.quitOne <- struct{}{}:
			p.workersRunning.Add(-1)
			sent++
		case <-p.stop:
			return int(p.workersRunning.Load()), ErrPoolClosed
		}
	}
	return n, nil
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

// DroppedTasks - счётчик задач, дропнутых стратегией OnFullDropNewest.
// Намёк на observability - в production это превращается в Prometheus
// counter (см. Q&A #2 в senior-design.md).
func (p *Pool) DroppedTasks() uint64 {
	return p.droppedTasks.Load()
}

func (p *Pool) worker() {
	// workersRunning менеджится Resize'ом эаgerно: +N в grow, -N сразу
	// после каждого успешного send'а в quitOne. Воркер сам этот счётчик
	// НЕ трогает - иначе concurrent Resize читал бы stale значение пока
	// defer не успел отработать.
	//
	// При выходе через done/stop (Shutdown) workersRunning остаётся
	// «зависшим», но Shutdown сам отбивает все последующие Resize'ы
	// через ErrPoolClosed, так что stale-значение никого не путает.
	wid := p.nextWorkerID.Add(1)
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			return
		case <-p.quitOne:
			return
		case t := <-p.tasks:
			p.runTask(t, wid)
		case <-p.stop:
			p.drainAndExit(wid)
			return
		}
	}
}

func (p *Pool) drainAndExit(wid int64) {
	for {
		select {
		case <-p.done:
			return
		case t := <-p.tasks:
			p.runTask(t, wid)
		default:
			return
		}
	}
}

// runTask - отдельная функция, чтобы defer recover
// срабатывал на каждой задаче, а не один раз на всю жизнь воркера.
func (p *Pool) runTask(t task, wid int64) {
	defer func() {
		if r := recover(); r != nil {
			p.sendErr(&PanicError{Recovered: r, Stack: debug.Stack()})
		}
	}()

	if err := t.ctx.Err(); err != nil {
		p.sendErr(fmt.Errorf("workerpool: ctx задачи отменён до запуска: %w", err))
		return
	}

	ctx := context.WithValue(t.ctx, workerIDKey{}, wid)
	if err := t.fn(ctx); err != nil {
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
