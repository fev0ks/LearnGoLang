# Graceful Shutdown в Go

Graceful shutdown — корректное завершение процесса без потери данных и прерывания активных запросов. Один из самых частых вопросов про Go-сервисы на собеседовании.

---

## Зачем это важно

```
Без graceful shutdown:
  SIGTERM → процесс убит мгновенно
  → Запросы в flight обрываются с 502
  → Воркеры бросают задачу на полпути
  → Транзакция в БД не откатилась (или не закомитилась)
  → Клиент retry → дубли → нарушение идемпотентности

С graceful shutdown:
  SIGTERM → принят сигнал
  → Перестать принимать новые запросы
  → Дождаться завершения текущих
  → Закрыть соединения и flush буферы
  → Выйти с кодом 0
```

**Когда приходит SIGTERM:**
- `kubectl rollout` — K8s посылает SIGTERM перед заменой пода
- `docker stop` — 10-секундный grace period перед SIGKILL
- Деплой на VM через systemd
- `Ctrl+C` в терминале (это SIGINT)

---

## Перехват сигналов OS

### `signal.NotifyContext` (Go 1.16+, рекомендуемый способ)

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGTERM, // K8s, systemd
        syscall.SIGINT,  // Ctrl+C
    )
    defer stop() // освобождает ресурсы NotifyContext

    // Передаём ctx в весь сервис
    if err := run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

`ctx.Done()` закрывается при получении сигнала. `stop()` отменяет подписку (вторая `stop()` повторно отправит следующий сигнал напрямую процессу — полезно для force-kill).

### Старый способ через канал (для понимания)

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit
// начать shutdown...
signal.Stop(quit) // перестать получать сигналы
```

---

## HTTP-сервер: `srv.Shutdown`

`http.Server.Shutdown(ctx)` — закрывает listener (новые соединения не принимаются), ждёт завершения активных запросов, закрывает idle keep-alive соединения.

```go
func run(ctx context.Context) error {
    srv := &http.Server{
        Addr:         ":8080",
        Handler:      router,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    // Запускаем сервер в отдельной горутине
    errCh := make(chan error, 1)
    go func() {
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
    }()

    // Ждём сигнала или ошибки
    select {
    case err := <-errCh:
        return fmt.Errorf("server error: %w", err)
    case <-ctx.Done():
        // Сигнал получен, начинаем shutdown
    }

    // Даём активным запросам время завершиться
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("shutdown error: %w", err)
    }
    return nil
}
```

**Важно:** `srv.Shutdown` не прерывает активные соединения — запросы должны сами отслеживать `r.Context().Done()` для длинных операций.

---

## Background workers

```go
type Worker struct {
    db     *pgxpool.Pool
    broker MessageBroker
    log    *slog.Logger
    wg     sync.WaitGroup
}

func (w *Worker) Run(ctx context.Context) {
    for i := 0; i < 5; i++ {
        w.wg.Add(1)
        go func() {
            defer w.wg.Done()
            w.processLoop(ctx)
        }()
    }
}

func (w *Worker) processLoop(ctx context.Context) {
    for {
        // Проверить отмену перед следующей итерацией
        select {
        case <-ctx.Done():
            w.log.Info("worker stopping")
            return
        default:
        }

        msg, err := w.broker.Receive(ctx) // blocking с timeout
        if err != nil {
            if errors.Is(err, context.Canceled) {
                return
            }
            w.log.Error("receive error", "err", err)
            time.Sleep(time.Second)
            continue
        }

        if err := w.handle(ctx, msg); err != nil {
            w.broker.Nack(msg)
        } else {
            w.broker.Ack(msg)
        }
    }
}

func (w *Worker) Wait() { w.wg.Wait() }
```

---

## Оркестрация нескольких компонентов

Реальный сервис: HTTP-сервер + воркеры + gRPC-сервер + фоновые задачи. Все должны остановиться корректно.

```go
func run(ctx context.Context) error {
    // --- Инициализация ---
    db, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    defer db.Close()

    worker := NewWorker(db, broker, logger)
    httpSrv := newHTTPServer(router)
    grpcSrv := newGRPCServer()

    // --- Запуск компонентов ---
    g, gCtx := errgroup.WithContext(ctx)

    // HTTP-сервер
    g.Go(func() error {
        errCh := make(chan error, 1)
        go func() {
            if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
                errCh <- err
            }
        }()
        select {
        case err := <-errCh:
            return err
        case <-gCtx.Done():
            sdCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            return httpSrv.Shutdown(sdCtx)
        }
    })

    // gRPC-сервер
    g.Go(func() error {
        go grpcSrv.Serve(lis)
        <-gCtx.Done()
        grpcSrv.GracefulStop() // ждёт завершения RPC в flight
        return nil
    })

    // Воркеры
    g.Go(func() error {
        worker.Run(gCtx)
        <-gCtx.Done()

        // Даём воркерам 45 секунд завершить текущие задачи
        done := make(chan struct{})
        go func() {
            worker.Wait()
            close(done)
        }()

        select {
        case <-done:
            logger.Info("workers stopped gracefully")
        case <-time.After(45 * time.Second):
            logger.Warn("workers shutdown timeout")
        }
        return nil
    })

    return g.Wait()
}
```

`errgroup.WithContext` удобен тем, что при ошибке любого компонента `gCtx` отменяется — остальные тоже начинают shutdown.

---

## Таймауты shutdown

Структура таймаутов должна соответствовать окружению:

```
K8s terminationGracePeriodSeconds: 60s
  └── SIGTERM → grace period начался
      ├── HTTP запросы: 30s (WriteTimeout сервера)
      ├── Воркеры: 45s (завершить текущую задачу)
      └── Итого: нужно уложиться в 60s до SIGKILL

Рекомендация:
  terminationGracePeriodSeconds = max(worker_timeout, http_timeout) + 10s buffer
```

```go
// Глобальный таймаут на весь процесс shutdown
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := run(ctx); err != nil {
        logger.Error("run failed", "err", err)
        os.Exit(1)
    }
}

// НЕ делать так:
func main() {
    // ...
    <-ctx.Done()
    os.Exit(0) // ← горутины не успели завершиться
}
```

---

## Частые ошибки

| Ошибка | Последствие | Правило |
|---|---|---|
| `os.Exit(0)` после сигнала | defer'ы не выполняются, горутины брошены | Дождаться WaitGroup |
| Нет `defer cancel()` на shutdown context | Timer goroutine утекает | Всегда `defer cancel()` |
| Shutdown context без таймаута | Зависает если воркер застрял | Всегда `WithTimeout` |
| Закрыть канал до WaitGroup.Wait | Panic в воркерах | Сначала cancel, потом close |
| WaitGroup.Add внутри горутины | Race: Wait может вернуться раньше | Add перед go |
| Игнорировать SIGTERM, ловить только SIGINT | Сервис не деплоится корректно в K8s | Ловить оба |

---

## Проверка корректности в тестах

```go
func TestGracefulShutdown(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    worker := NewWorker(...)
    started := make(chan struct{})

    go func() {
        close(started)
        worker.Run(ctx)
    }()

    <-started // убедились что воркер запущен

    cancel() // эмулируем SIGTERM

    done := make(chan struct{})
    go func() {
        worker.Wait()
        close(done)
    }()

    select {
    case <-done:
        // OK
    case <-time.After(5 * time.Second):
        t.Fatal("worker did not stop within timeout")
    }
}
```

---

## Graceful shutdown vs Force kill

```
signal.NotifyContext ловит второй SIGTERM/SIGINT:
  Первый Ctrl+C → ctx отменён, stop() вызван
  Второй Ctrl+C → сигнал доставлен напрямую процессу → immediate exit

Это стандартное поведение:
  - Один раз: graceful (ждём)
  - Два раза: force (убиваем немедленно)
```

---

## Interview-ready answer

**Q: Как реализовать graceful shutdown Go-сервиса в K8s?**

`signal.NotifyContext` ловит SIGTERM (K8s посылает при rollout). Контекст передаётся в HTTP-сервер, воркеры и все фоновые горутины. При отмене:
- `srv.Shutdown(ctx)` — listener закрыт, активные HTTP-запросы дожидаются ответа
- `grpcSrv.GracefulStop()` — ждёт in-flight RPC
- Воркеры читают `ctx.Done()` и выходят после текущей задачи, `sync.WaitGroup` фиксирует полное завершение

Важно: таймаут shutdown < `terminationGracePeriodSeconds` в K8s (buffer ~10s), иначе SIGKILL убьёт то, что не успело.

Типичные ошибки: `os.Exit` без WaitGroup, отсутствие таймаута на shutdown context, игнорирование SIGTERM (только SIGINT).
