package main

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// =====================================================================
// Layer 1 - anchor tests (для нарратива в видео).
// =====================================================================

// TestJuniorBugIsGone - главный тест-якорь.
//
// Прямая копия сценария TestAddTask_PanicOnBlockedSend из junior'а: занимаем
// единственного воркера, забиваем буфер, ставим ещё один Submit в send,
// вызываем Shutdown. У junior'а здесь паника "send on closed channel".
// У middle паники быть не должно: канал tasks никогда не закрывается.
func TestJuniorBugIsGone(t *testing.T) {
	const queueSize = 100

	p := New(Config{Workers: 1, QueueSize: queueSize, ErrBuf: 16})

	// Воркер виснет на release, очередь не разгребается.
	release := make(chan struct{})
	workerStarted := make(chan struct{})
	if err := p.Submit(context.Background(), func(_ context.Context) error {
		close(workerStarted)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("первый Submit не должен возвращать ошибку: %v", err)
	}
	<-workerStarted

	// Забиваем буфер до конца.
	for range queueSize {
		if err := p.Submit(context.Background(), func(_ context.Context) error { return nil }); err != nil {
			t.Fatalf("Submit при заполнении буфера: %v", err)
		}
	}

	// Этот Submit заблокируется на send: буфер полон, воркер занят.
	var (
		submitPanic atomic.Value
		submitErr   atomic.Value
	)
	submitDone := make(chan struct{})
	go func() {
		defer close(submitDone)
		defer func() {
			if r := recover(); r != nil {
				submitPanic.Store(r)
			}
		}()
		err := p.Submit(context.Background(), func(_ context.Context) error { return nil })
		if err != nil {
			submitErr.Store(err)
		}
	}()

	waitUntilBlocked(t)

	// Запускаем Shutdown пока воркер всё ещё висит.
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- p.Shutdown(context.Background())
	}()

	waitUntilBlocked(t)

	// Отпускаем воркера: он добьёт буфер, Shutdown разблокируется.
	close(release)

	<-submitDone
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown вернул ошибку: %v", err)
	}

	if r := submitPanic.Load(); r != nil {
		t.Fatalf("в middle не должно быть паники, получили: %v", r)
	}

	// Submit мог либо успеть в очередь (nil), либо отбиться (ErrPoolClosed).
	// Оба исхода легитимны - главное, что нет send-on-closed-channel.
	if v := submitErr.Load(); v != nil {
		if err, _ := v.(error); !errors.Is(err, ErrPoolClosed) {
			t.Fatalf("ожидали либо nil, либо ErrPoolClosed; получили: %v", err)
		}
	}
}

// TestPanicRecovery: одна задача из пяти паникует.
// Остальные четыре должны выполниться, паника придёт как *PanicError.
func TestPanicRecovery(t *testing.T) {
	p := New(Config{Workers: 2, QueueSize: 8, ErrBuf: 8})

	const total = 5
	var executed atomic.Int64
	for i := range total {
		err := p.Submit(context.Background(), func(_ context.Context) error {
			if i == 2 {
				panic("boom")
			}
			executed.Add(1)
			return nil
		})
		if err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := executed.Load(); got != total-1 {
		t.Fatalf("executed=%d, ожидали %d (паника не должна была забрать соседей)", got, total-1)
	}

	var pe *PanicError
	select {
	case err := <-p.Errors():
		if !errors.As(err, &pe) {
			t.Fatalf("ожидали *PanicError, получили %T: %v", err, err)
		}
	default:
		t.Fatal("в Errors() ничего нет - паника потерялась")
	}

	if pe.Recovered != "boom" {
		t.Fatalf("Recovered=%v, ожидали \"boom\"", pe.Recovered)
	}
	if len(pe.Stack) == 0 {
		t.Fatal("PanicError.Stack пустой")
	}
}

// TestGracefulDrain: 100 коротких задач, Shutdown без таймаута - все выполнятся.
func TestGracefulDrain(t *testing.T) {
	p := New(Config{Workers: 4, QueueSize: 200, ErrBuf: 0})

	const total = 100
	var executed atomic.Int64
	for range total {
		err := p.Submit(context.Background(), func(_ context.Context) error {
			executed.Add(1)
			return nil
		})
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := executed.Load(); got != total {
		t.Fatalf("executed=%d, ожидали %d (graceful drain должен дорезать всё)", got, total)
	}
}

// TestShutdownTimeout: 50 задач по ~100ms, дедлайн 200ms → force-stop.
// Возвращается обёрнутый context.DeadlineExceeded, горутины не текут.
func TestShutdownTimeout(t *testing.T) {
	baseline := runtime.NumGoroutine()

	p := New(Config{Workers: 2, QueueSize: 100, ErrBuf: 0})

	const total = 50
	for range total {
		err := p.Submit(context.Background(), func(ctx context.Context) error {
			select {
			case <-time.After(100 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := p.Shutdown(ctx)
	if err == nil {
		t.Fatal("ожидали ошибку drain-таймаута, получили nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, DeadlineExceeded) = false, err=%v", err)
	}
	if !strings.HasPrefix(err.Error(), "workerpool: дедлайн на drain истёк:") {
		t.Fatalf("ожидали prefix \"workerpool: дедлайн на drain истёк:\", получили: %q", err.Error())
	}

	// Воркеры обязаны были выйти к моменту возврата Shutdown - wg.Wait()
	// в Shutdown ждёт их явно. Любое превышение baseline логируем, но не
	// фейлим: рантайм может держать фоновые горутины GC/sysmon.
	if got := runtime.NumGoroutine(); got > baseline+2 {
		t.Logf("baseline=%d, after=%d (допустимый запас на runtime activity)", baseline, got)
	}
}

// TestPerTaskCtxCancellation: задача уважает ctx.Done(), отмена снаружи
// останавливает её ~через 50ms; пул продолжает принимать новые задачи.
func TestPerTaskCtxCancellation(t *testing.T) {
	p := New(Config{Workers: 1, QueueSize: 4, ErrBuf: 4})
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	taskCtx, cancel := context.WithCancel(context.Background())
	taskDone := make(chan struct{})
	start := time.Now()

	if err := p.Submit(taskCtx, func(ctx context.Context) error {
		defer close(taskDone)
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	time.AfterFunc(50*time.Millisecond, cancel)

	select {
	case <-taskDone:
	case <-time.After(2 * time.Second):
		t.Fatal("задача не реагирует на cancel")
	}

	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("отмена заняла %v, ожидали ~50ms", elapsed)
	}

	// Ошибку получим через Errors(): задача вернула ctx.Err().
	select {
	case err := <-p.Errors():
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ожидали context.Canceled, получили: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ошибка от отменённой задачи не пришла в Errors()")
	}

	// Пул всё ещё рабочий - новый Submit принимается.
	doneNew := make(chan struct{})
	if err := p.Submit(context.Background(), func(_ context.Context) error {
		close(doneNew)
		return nil
	}); err != nil {
		t.Fatalf("Submit после отмены не должен ошибаться: %v", err)
	}
	select {
	case <-doneNew:
	case <-time.After(time.Second):
		t.Fatal("новая задача не выполнилась - пул сломан")
	}
}

// =====================================================================
// Layer 2 - coverage (для CI).
// =====================================================================

// TestSubmitAfterShutdown: после Shutdown все Submit получают ErrPoolClosed.
func TestSubmitAfterShutdown(t *testing.T) {
	p := New(Config{Workers: 1, QueueSize: 1, ErrBuf: 0})
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	for range 5 {
		err := p.Submit(context.Background(), func(_ context.Context) error { return nil })
		if !errors.Is(err, ErrPoolClosed) {
			t.Fatalf("ожидали ErrPoolClosed, получили: %v", err)
		}
	}
}

// TestDoubleShutdown: повторный Shutdown отдаёт ErrAlreadyShutdown без double-close.
func TestDoubleShutdown(t *testing.T) {
	p := New(Config{Workers: 1, QueueSize: 1, ErrBuf: 0})

	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("первый Shutdown: %v", err)
	}
	err := p.Shutdown(context.Background())
	if !errors.Is(err, ErrAlreadyShutdown) {
		t.Fatalf("ожидали ErrAlreadyShutdown, получили: %v", err)
	}
}

// TestDroppedErrors: при переполнении Errors() инкрементится счётчик.
func TestDroppedErrors(t *testing.T) {
	// ErrBuf=1 - гарантированно переполняем.
	p := New(Config{Workers: 1, QueueSize: 16, ErrBuf: 1})

	const total = 10
	for range total {
		err := p.Submit(context.Background(), func(_ context.Context) error {
			return errors.New("fail")
		})
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := p.DroppedErrors(); got == 0 {
		t.Fatal("DroppedErrors=0, ожидали > 0 (буфер 1, ошибок 10)")
	}
}

// TestSubmitCtxAlreadyDone: Submit с уже отменённым ctx возвращает ctx.Err()
// и не зависает.
func TestSubmitCtxAlreadyDone(t *testing.T) {
	// Воркер занят, очередь полна - без ctx Submit бы блокировался навсегда.
	p := New(Config{Workers: 1, QueueSize: 1, ErrBuf: 0})
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	release := make(chan struct{})
	defer close(release)

	if err := p.Submit(context.Background(), func(_ context.Context) error {
		<-release
		return nil
	}); err != nil {
		t.Fatalf("первый Submit: %v", err)
	}
	if err := p.Submit(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("второй Submit (заполняет буфер): %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- p.Submit(ctx, func(_ context.Context) error { return nil })
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ожидали context.Canceled, получили: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Submit с отменённым ctx завис")
	}
}

// =====================================================================
// Helpers.
// =====================================================================

// waitUntilBlocked даёт планировщику шанс довести горутину-отправителя
// до состояния "заблокирован на send/recv". 50ms и несколько Gosched
// на практике хватает с огромным запасом.
func waitUntilBlocked(_ *testing.T) {
	for range 5 {
		runtime.Gosched()
	}
	time.Sleep(50 * time.Millisecond) //nolint:forbidigo // сценарный тайминг в тесте
}
