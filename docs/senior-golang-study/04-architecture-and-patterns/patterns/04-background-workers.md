# Background Workers и Job Orchestration

Фоновые воркеры — отдельный класс задач в Go: graceful shutdown, worker pools, распределённые lease'ы, защита от дублирования. Часто спрашивают на интервью именно детали реализации.

## Содержание

- [Типы фоновых задач](#типы-фоновых-задач)
- [Worker pool](#worker-pool)
- [Graceful shutdown](#graceful-shutdown) → [подробнее](./08-graceful-shutdown.md)
- [Periodic jobs (cron-style)](#periodic-jobs-cron-style)
- [Distributed lease: один воркер в кластере](#distributed-lease-один-воркер-в-кластере)
- [Idempotent workers](#idempotent-workers)
- [Backpressure и bounded concurrency](#backpressure-и-bounded-concurrency)
- [Observability воркеров](#observability-воркеров)
- [Interview-ready answer](#interview-ready-answer)

---

## Типы фоновых задач

| Тип | Описание | Пример |
|---|---|---|
| **One-shot** | Выполнить один раз и завершить | Миграция данных |
| **Periodic** | Запускаться по расписанию | Отчёт раз в сутки |
| **Queue consumer** | Читать задачи из очереди | Обработка заказов |
| **Reconciler** | Периодически сверять состояние | Sync с внешним API |
| **Event listener** | Реагировать на события из брокера | Kafka consumer |
| **Scheduled at-time** | Запустить в определённое время | Напоминание пользователю |

---

## Worker pool

Ограниченный пул воркеров защищает от self-DoS под нагрузкой.

```go
type WorkerPool struct {
    workers  int
    jobs     chan Job
    wg       sync.WaitGroup
    ctx      context.Context
    cancel   context.CancelFunc
}

func NewWorkerPool(workers int) *WorkerPool {
    ctx, cancel := context.WithCancel(context.Background())
    return &WorkerPool{
        workers: workers,
        jobs:    make(chan Job, workers*2),  // буфер = 2x количество воркеров
        ctx:     ctx,
        cancel:  cancel,
    }
}

func (p *WorkerPool) Start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        go func(id int) {
            defer p.wg.Done()
            for {
                select {
                case job, ok := <-p.jobs:
                    if !ok {
                        return  // канал закрыт
                    }
                    p.process(job)
                case <-p.ctx.Done():
                    return
                }
            }
        }(i)
    }
}

func (p *WorkerPool) Submit(ctx context.Context, job Job) error {
    select {
    case p.jobs <- job:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (p *WorkerPool) Shutdown() {
    p.cancel()
    close(p.jobs)  // сигнал воркерам что задач больше не будет
    p.wg.Wait()    // ждём завершения текущих задач
}
```

**Выбор размера пула:**

| Тип задачи | Рекомендация |
|---|---|
| CPU-bound | `runtime.NumCPU()` или `runtime.NumCPU() + 1` |
| IO-bound (DB, HTTP) | `50–500`, зависит от upstream capacity |
| Mixed | Профилировать, начать с `runtime.NumCPU() * 4` |

---

## Graceful shutdown

Воркер слушает `ctx.Done()` — при отмене заканчивает текущую задачу и выходит. `sync.WaitGroup` фиксирует полное завершение. `signal.NotifyContext` ловит SIGTERM/SIGINT.

Подробно: паттерны, оркестрация нескольких компонентов (HTTP + gRPC + workers), таймауты, частые ошибки — в [08. Graceful Shutdown](./08-graceful-shutdown.md).

---

## Periodic jobs (cron-style)

```go
// Простой ticker-based job
func RunPeriodic(ctx context.Context, interval time.Duration, fn func(ctx context.Context) error, log *slog.Logger) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    // Запустить сразу при старте, не ждать первого тика
    if err := fn(ctx); err != nil {
        log.Error("job error", "err", err)
    }

    for {
        select {
        case <-ticker.C:
            if err := fn(ctx); err != nil {
                log.Error("job error", "err", err)
                // не прекращать — продолжить по расписанию
            }
        case <-ctx.Done():
            return
        }
    }
}

// Использование
go RunPeriodic(ctx, 5*time.Minute, reconciler.Reconcile, logger)
go RunPeriodic(ctx, 1*time.Hour, reporter.GenerateHourly, logger)
```

**Cron-библиотека для сложных расписаний:**

```go
// github.com/robfig/cron/v3
c := cron.New(cron.WithSeconds())

c.AddFunc("0 * * * *", func() {  // каждый час
    if err := generateReport(ctx); err != nil {
        log.Error("report failed", "err", err)
    }
})

c.AddFunc("*/30 * * * * *", func() {  // каждые 30 секунд
    reconcile(ctx)
})

c.Start()
defer c.Stop()
```

---

## Distributed lease: один воркер в кластере

**Проблема:** есть 3 реплики сервиса. Periodic job должна запускаться только на одной из них (иначе дублирование).

```go
// Distributed lock через Redis
type DistributedLock struct {
    redis  *redis.Client
    key    string
    ttl    time.Duration
    nodeID string
}

func (l *DistributedLock) TryAcquire(ctx context.Context) (bool, error) {
    // SET key nodeID NX EX ttl — атомарная операция
    ok, err := l.redis.SetNX(ctx, l.key, l.nodeID, l.ttl).Result()
    return ok, err
}

func (l *DistributedLock) Renew(ctx context.Context) error {
    // Продлить TTL если мы ещё держим lock
    script := `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("EXPIRE", KEYS[1], ARGV[2])
        end
        return 0
    `
    result, err := l.redis.Eval(ctx, script, []string{l.key}, l.nodeID, int(l.ttl.Seconds())).Int()
    if result == 0 {
        return errors.New("lock lost")
    }
    return err
}

// Использование в periodic job
func RunWithLease(ctx context.Context, lock *DistributedLock, interval time.Duration, fn func(ctx context.Context) error) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    renewTicker := time.NewTicker(l.ttl / 3)  // продлевать каждые TTL/3
    defer renewTicker.Stop()

    for {
        select {
        case <-ticker.C:
            acquired, err := lock.TryAcquire(ctx)
            if err != nil || !acquired {
                continue  // другой инстанс держит lock
            }
            if err := fn(ctx); err != nil {
                log.Error("job error", "err", err)
            }

        case <-renewTicker.C:
            if err := lock.Renew(ctx); err != nil {
                log.Warn("lock lost, skipping renewal")
            }

        case <-ctx.Done():
            return
        }
    }
}
```

**Альтернатива через PostgreSQL Advisory Locks:**

```go
func withAdvisoryLock(ctx context.Context, db *pgxpool.Pool, lockID int64, fn func() error) error {
    conn, err := db.Acquire(ctx)
    if err != nil {
        return err
    }
    defer conn.Release()

    // Попытаться взять lock (non-blocking)
    var acquired bool
    if err := conn.QueryRow(ctx,
        "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
        return err
    }
    if !acquired {
        return nil  // другой инстанс держит lock, пропустить
    }
    defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)

    return fn()
}

// Использование
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        withAdvisoryLock(ctx, db, 42, func() error {
            return reconcile(ctx)
        })
    }
}()
```

---

## Idempotent workers

Воркер должен быть idempotent: повторная обработка одного сообщения не ломает систему.

```go
func (w *Worker) processOrder(ctx context.Context, msg Message) error {
    var event OrderCreatedEvent
    if err := json.Unmarshal(msg.Body, &event); err != nil {
        return fmt.Errorf("unmarshal: %w", err)
    }

    // Idempotency check: уже обработали это событие?
    processed, err := w.db.ExecContext(ctx, `
        INSERT INTO processed_messages (message_id, processed_at)
        VALUES ($1, NOW())
        ON CONFLICT (message_id) DO NOTHING
    `, msg.ID)
    if err != nil {
        return fmt.Errorf("idempotency check: %w", err)
    }
    if processed.RowsAffected() == 0 {
        // Уже обработано — idempotent skip
        return nil
    }

    // Основная обработка
    return w.fulfillOrder(ctx, event)
}
```

---

## Backpressure и bounded concurrency

```go
// Semaphore для ограничения параллельных задач
type Semaphore struct {
    ch chan struct{}
}

func NewSemaphore(n int) *Semaphore {
    return &Semaphore{ch: make(chan struct{}, n)}
}

func (s *Semaphore) Acquire(ctx context.Context) error {
    select {
    case s.ch <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Semaphore) Release() {
    <-s.ch
}

// Использование: не более 10 параллельных внешних API вызовов
sem := NewSemaphore(10)

for _, item := range items {
    item := item
    if err := sem.Acquire(ctx); err != nil {
        break
    }
    go func() {
        defer sem.Release()
        if err := externalAPI.Process(ctx, item); err != nil {
            log.Error("process error", "err", err)
        }
    }()
}
```

**Почему это важно:**

```
Без bounded concurrency:
  10000 задач → 10000 goroutines → 10000 параллельных DB запросов
  → DB connection pool exhausted
  → Все запросы fail
  → Service unavailable

С bounded concurrency:
  10000 задач → 50 параллельных → DB нормально
  → Остальные ждут в канале
  → Throughput стабилен
```

---

## Observability воркеров

```go
// Метрики которые обязательны для воркера
type WorkerMetrics struct {
    processed    prometheus.Counter      // сколько задач обработано
    failed       prometheus.Counter      // сколько упало
    duration     prometheus.Histogram    // время обработки
    queueDepth   prometheus.Gauge        // глубина очереди
    inFlight     prometheus.Gauge        // сколько сейчас обрабатывается
}

func (w *Worker) process(ctx context.Context, job Job) {
    w.metrics.inFlight.Inc()
    defer w.metrics.inFlight.Dec()

    start := time.Now()

    if err := w.handle(ctx, job); err != nil {
        w.metrics.failed.Inc()
        w.log.Error("job failed", "err", err, "job_id", job.ID, "type", job.Type)
    } else {
        w.metrics.processed.Inc()
    }

    w.metrics.duration.Observe(time.Since(start).Seconds())
}
```

**Алерты которые должны быть:**

| Метрика | Условие алерта |
|---|---|
| `queue_depth` | > 1000 (воркеры не успевают) |
| `job_failure_rate` | > 5% за 5 минут |
| `job_duration_p99` | > 30 сек (задачи зависают) |
| `in_flight` | = 0 при непустой очереди (воркер завис) |
| Worker uptime | restart > 3 раз за час |

---

## Interview-ready answer

> "Фоновые воркеры в Go — это прежде всего правильный graceful shutdown и bounded concurrency.
>
> Graceful shutdown: `signal.NotifyContext` ловит SIGTERM, передаёт ctx в воркеры, они заканчивают текущую задачу и выходят. `sync.WaitGroup` + timeout контекст дают уверенность что всё завершилось до kill-9.
>
> Worker pool: bounded channel как семафор. Без ограничения параллельности под нагрузкой можно получить 10K goroutines и исчерпанный DB connection pool.
>
> Для distributed periodic jobs (только один инстанс в кластере): Redis SETNX или PostgreSQL advisory lock. Простой, надёжный, не нужен Zookeeper.
>
> Idempotency: таблица `processed_messages` с `ON CONFLICT DO NOTHING`. At-least-once delivery из брокера + идемпотентный воркер = exactly-once семантика на уровне бизнес-логики."
