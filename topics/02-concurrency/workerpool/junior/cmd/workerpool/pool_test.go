package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAddTask_PanicOnBlockedSend детерминированно воспроизводит главный баг
// junior-реализации (см. BUG #4 в README.md): AddTask пишет в канал, который
// Close может закрыть параллельно. Любая такая комбинация = "panic: send on
// closed channel".
//
// Здесь мы загоняем гонку в гарантированный сценарий:
//  1. Занимаем единственного воркера долгой задачей, он не разгребает очередь.
//  2. Забиваем буфер канала под завязку.
//  3. Запускаем ещё один AddTask, он блокируется на send (буфер полон).
//  4. Вызываем Close, который делает close(p.tasks) пока есть заблокированный отправитель.
//  5. Заблокированный send получает "send on closed channel", это паника.
//
// Это та же первопричина, что и TOCTOU между Unlock и send: AddTask держит
// в руках канал, который Close имеет право закрыть. На собесе достаточно
// показать любой из этих сценариев.
func TestAddTask_PanicOnBlockedSend(t *testing.T) {
	const bufferSize = 100 // должен совпадать с константой в New (BUG #2)

	p := New(1)

	// Воркер виснет на release, очередь не разгребается.
	release := make(chan struct{})
	workerStarted := make(chan struct{})
	p.AddTask(func() {
		close(workerStarted)
		<-release
	})
	<-workerStarted

	// Забиваем буфер до конца: воркер занят, эти задачи лягут в очередь.
	for range bufferSize {
		p.AddTask(func() {})
	}

	// Этот AddTask заблокируется на send: буфер полон, воркер занят.
	var panicMsg atomic.Value
	addDone := make(chan struct{})
	go func() {
		defer close(addDone)
		defer func() {
			if r := recover(); r != nil {
				panicMsg.Store(r)
			}
		}()
		p.AddTask(func() {})
	}()

	// Дожидаемся, пока горутина действительно встанет в send.
	// Без этого Close может выиграть гонку и сценарий не сработает.
	waitUntilBlocked(t)

	// Запускаем Close, пока воркер всё ещё висит на release.
	// Close под мьютексом сделает close(p.tasks), и заблокированный
	// отправитель получит панику. После этого Close виснет на wg.Wait,
	// потому что воркер ещё держит таску.
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		p.Close()
	}()

	// Даём Close время реально вызвать close(p.tasks) и тем самым
	// уронить заблокированного отправителя.
	waitUntilBlocked(t)

	// Теперь отпускаем воркера, чтобы он добил буфер, увидел закрытый
	// канал и завершился. Это разблокирует wg.Wait внутри Close.
	close(release)

	<-addDone
	<-closeDone

	r := panicMsg.Load()
	if r == nil {
		t.Fatal("ожидали 'panic: send on closed channel' от заблокированного " +
			"AddTask, но паники не было: баг исчез или сценарий сломался")
	}
	t.Logf("баг воспроизведён: %v", r)
}

// TestAddTask_RaceUnderRaceDetector это дополнительный сценарий для запуска
// с `go test -race`. Он не гарантирует панику, но гарантированно показывает
// data race на самом канале: close в Close и send в AddTask выполняются
// параллельно без общей синхронизации, и детектор гонок это видит.
func TestAddTask_RaceUnderRaceDetector(t *testing.T) {
	const iterations = 500
	var sent atomic.Int64

	for range iterations {
		p := New(2)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			defer func() { _ = recover() }()
			p.AddTask(func() {})
			sent.Add(1)
		}()

		go func() {
			defer wg.Done()
			p.Close()
		}()

		wg.Wait()
	}

	t.Logf("итераций: %d, успешных AddTask: %d (остальные либо словили "+
		"панику, либо вышли по p.closed=true)", iterations, sent.Load())
}

// waitUntilBlocked даёт планировщику шанс довести горутину-отправителя
// до состояния "заблокирован на send". 50ms и несколько Gosched на практике
// хватает с огромным запасом.
func waitUntilBlocked(_ *testing.T) {
	for range 5 {
		runtime.Gosched()
	}
	time.Sleep(50 * time.Millisecond) //nolint:forbidigo // сценарный тайминг в тесте
}
