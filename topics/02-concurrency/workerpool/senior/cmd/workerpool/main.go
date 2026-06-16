package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

func main() {
	demoBackpressure()
	demoResize()
}

// demoBackpressure показывает три стратегии Config.OnFull:
// Block (default), FailFast, DropNewest. Один и тот же сценарий
// (1 воркер + полная очередь) - разные итоги.
func demoBackpressure() {
	slog.Info("=== demo: backpressure (Config.OnFull) ===")

	policies := []struct {
		name   string
		policy OnFullPolicy
	}{
		{"OnFullBlock", OnFullBlock},
		{"OnFullFailFast", OnFullFailFast},
		{"OnFullDropNewest", OnFullDropNewest},
	}

	for _, p := range policies {
		pool := New(Config{Workers: 1, QueueSize: 1, ErrBuf: 0, OnFull: p.policy})

		// Занимаем единственного воркера: он висит на release.
		release := make(chan struct{})
		started := make(chan struct{})
		if err := pool.Submit(context.Background(), func(_ context.Context) error {
			close(started)
			<-release
			return nil
		}); err != nil {
			slog.Error("submit#1", "policy", p.name, "err", err)
		}
		<-started

		// Забиваем буфер.
		if err := pool.Submit(context.Background(), func(_ context.Context) error { return nil }); err != nil {
			slog.Error("submit#2", "policy", p.name, "err", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		err := pool.Submit(ctx, func(_ context.Context) error { return nil })
		cancel()

		slog.Info("Submit при полной очереди",
			"policy", p.name,
			"err", err,
			"dropped_tasks", pool.DroppedTasks(),
		)

		close(release)
		if err = pool.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown", "policy", p.name, "err", err)
		}
	}
}

// demoResize показывает динамический resize: пул растёт под пиковую
// нагрузку, потом ужимается, лишние воркеры выходят кооперативно.
func demoResize() {
	slog.Info("=== demo: dynamic resize ===")

	pool := New(Config{Workers: 2, QueueSize: 32, ErrBuf: 0})
	slog.Info("стартуем пул", "воркеров", 2)

	slog.Info("пик нагрузки - увеличиваем пул", "с", 2, "до", 8)
	if got, err := pool.Resize(8); err != nil {
		slog.Error("Resize", "err", err)
	} else {
		slog.Info("пул вырос", "воркеров_сейчас", got)
	}

	slog.Info("кидаем 8 задач, все должны стартовать параллельно")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		err := pool.Submit(context.Background(), func(ctx context.Context) error {
			defer wg.Done()
			wid, _ := WorkerID(ctx)
			slog.Info(">>> стартанула", "task_id", i, "worker_id", wid)
			<-time.After(50 * time.Millisecond)
			slog.Info("<<< закончилась", "task_id", i, "worker_id", wid)
			return nil
		})
		if err != nil {
			wg.Done()
			slog.Error("submit", "id", i, "err", err)
		}
	}
	wg.Wait()

	slog.Info("нагрузка спала - ужимаем пул", "с", 8, "до", 3)
	if got, err := pool.Resize(3); err != nil {
		slog.Error("Resize", "err", err)
	} else {
		slog.Info("пул ужался, лишние воркеры вышли кооперативно", "воркеров_сейчас", got)
	}

	// Дать воркерам время разойтись (для красоты лога).
	<-time.After(50 * time.Millisecond)

	// Демонстрация что после Resize(3) работают РОВНО 3 воркера:
	// кидаем 9 задач - пойдут волнами по 3 (по числу воркеров).
	// worker_id достаём из ctx через WorkerID(ctx) - видно кто на ком.
	slog.Info("кидаем 9 задач - пойдут волнами по 3 (потому что воркеров = 3)")
	var wg2 sync.WaitGroup
	for i := range 9 {
		wg2.Add(1)
		err := pool.Submit(context.Background(), func(ctx context.Context) error {
			defer wg2.Done()
			wid, _ := WorkerID(ctx)
			slog.Info(">>> стартанула", "task_id", i, "worker_id", wid)
			<-time.After(400 * time.Millisecond)
			slog.Info("<<< закончилась", "task_id", i, "worker_id", wid)
			return nil
		})
		if err != nil {
			wg2.Done()
			slog.Error("submit", "id", i, "err", err)
		}
	}
	wg2.Wait()

	// Shutdown с дедлайном на drain: graceful (дочерпать буфер), но не дольше
	// таймаута. По истечении ctx Pool делает force-stop (close(done)). Таймаут -
	// ответственность вызывающего: своего time.After внутри Pool нет.
	slog.Info("graceful shutdown с дедлайном на drain (5s)")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := pool.Shutdown(shutdownCtx)
	cancel()
	if err != nil {
		slog.Error("shutdown", "err", err)
	}

	slog.Info("готово",
		"dropped_tasks", pool.DroppedTasks(),
		"dropped_errors", pool.DroppedErrors(),
	)

	slog.Info("проверка: после Shutdown Resize должен отбиться")
	if _, err := pool.Resize(5); !errors.Is(err, ErrPoolClosed) {
		slog.Warn("Resize после Shutdown не вернул ErrPoolClosed", "err", err)
	} else {
		slog.Info("Resize после Shutdown отбился ErrPoolClosed - корректно")
	}
}
