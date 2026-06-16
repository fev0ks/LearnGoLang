package main

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

func main() {
	pool := New(Config{Workers: 3, QueueSize: 10, ErrBuf: 16})

	// Потребитель ошибок: панику отделяем от обычной ошибки через errors.As.
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for err := range pool.Errors() {
			var pe *PanicError
			switch {
			case errors.As(err, &pe):
				slog.Error("паника в задаче", "recovered", pe.Recovered)
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				slog.Warn("задача отменена", "err", err)
			default:
				slog.Error("задача упала", "err", err)
			}
		}
	}()

	submit := func(ctx context.Context, fn func(context.Context) error) {
		if err := pool.Submit(ctx, fn); err != nil {
			slog.Error("submit не прошёл", "err", err)
		}
	}

	// 1. Обычная задача.
	submit(context.Background(), func(_ context.Context) error {
		slog.Info("привет из задачи")
		return nil
	})

	// 2. Паника - приходит как *PanicError, остальные задачи живут.
	submit(context.Background(), func(_ context.Context) error {
		panic("boom")
	})

	// 3. Per-task ctx cancellation: задача уважает ctx.Done().
	taskCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	submit(taskCtx, func(ctx context.Context) error {
		select {
		case <-time.After(time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	// 4. Graceful shutdown с дедлайном на drain.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := pool.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown не прошёл", "err", err)
	}
	close(pool.errs) // в проде канал не закрывают; здесь - чтобы выйти из range в демо
	<-consumerDone

	slog.Info("готово", "dropped_errors", pool.DroppedErrors())
}
