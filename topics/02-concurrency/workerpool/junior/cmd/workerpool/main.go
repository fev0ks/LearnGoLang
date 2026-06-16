package main

import (
	"log/slog"
	"time"
)

func main() {
	pool := New(3)

	for i := range 5 {
		pool.AddTask(func() {
			slog.Info("задача запущена", "id", i)
			<-time.After(200 * time.Millisecond)
			slog.Info("задача готова", "id", i)
		})
	}

	pool.Close()
	slog.Info("все задачи завершены")
}
