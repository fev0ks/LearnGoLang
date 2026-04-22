# Distributed Task Queue

Разбор задачи "Спроектируй распределённую очередь задач" (job queue, background processing system). Аналоги: Celery, Sidekiq, BullMQ, Temporal. Проверяет понимание at-least-once delivery, idempotency, dead letter queues, scheduling.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Кандидат: Уточняю — task queue может означать разные вещи.

Вопросы:
  - Это просто очередь задач (fire-and-forget) или нужен workflow (цепочки задач)?
    → Пока одиночные задачи, без DAG
  - Нужно ли расписание (cron-like: "запускать каждый час")?
    → Да, delayed tasks ("запустить через 30 мин") и recurring ("каждые 5 мин")
  - Приоритеты у задач?
    → Да, три уровня: high/normal/low
  - Retry при ошибке?
    → Да, с конфигурируемой стратегией (max attempts, backoff)
  - Нужен UI для мониторинга задач?
    → Статусы через API, UI — out of scope
  - Нужна ли гарантия exactly-once?
    → at-least-once + idempotent workers (пользователь обеспечивает идемпотентность)
```

**Договорились (scope):**
- Enqueue task → worker picks up → executes → reports result
- Delayed tasks (schedule at time T)
- Recurring tasks (cron expression)
- Priority: high/normal/low
- Retry с exponential backoff + max attempts
- Dead Letter Queue для необработанных задач
- Task status tracking (queued/running/done/failed)
- At-least-once delivery

**Out of scope:** workflow/DAG, task dependencies, distributed tracing (интеграция — ок, design — нет), UI dashboard.

### Нефункциональные требования

```
- Throughput: 10K tasks/sec enqueue; 5K tasks/sec execution
- Latency: задача начинается в пределах 1 сек после enqueue (для high priority)
- Durability: задача не теряется при падении любого компонента
- Availability: 99.9%
- Scale: воркеры масштабируются горизонтально
- Task isolation: один медленный/крашащий worker не влияет на остальных
```

---

## Фаза 2: Оценка нагрузки

```
Enqueue:
  10K tasks/sec = 864M tasks/day
  Peak = 3x = 30K tasks/sec

Storage для задач (pending/active):
  Одна задача: ~5KB (payload + metadata)
  In-flight в любой момент: 5K tasks × 60 sec avg execution = 300K задач
  300K × 5KB = 1.5GB — умещается в Redis

История выполненных задач:
  864M/day × 7 дней хранения × 5KB = 30 TB
  → Нужна отдельная хранилище для history (PostgreSQL или ClickHouse)
  → В Redis хранить только active/pending задачи

Воркеры:
  5K tasks/sec, avg 1 сек на задачу → 5K параллельных воркеров
  При 50 goroutines на под → 100 Pod'ов
```

---

## Фаза 3: Высокоуровневый дизайн

```
  Producer                     ┌──────────────────────────────────┐
  (API/Service)                │        Task Queue System         │
      │                        │                                  │
      │  enqueue(task)          │  ┌────────────┐                 │
      ├────────────────────────►│  │   Queue    │  API            │
      │                        │  │   API      │  (REST/gRPC)    │
      │  task_status(id)        │  └─────┬──────┘                 │
      ├────────────────────────►│        │                        │
      │                        │  ┌─────▼──────┐                 │
                               │  │  Broker    │                 │
                               │  │  (Redis /  │                 │
                               │  │  Kafka)    │                 │
                               │  └─────┬──────┘                 │
                               │        │                        │
                               │  ┌─────▼──────────────────┐     │
                               │  │   Worker Pool          │     │
                               │  │ ┌──────┐ ┌──────┐ ...  │     │
                               │  │ │  W1  │ │  W2  │      │     │
                               │  │ └──────┘ └──────┘      │     │
                               │  └────────────────────────┘     │
                               │                                  │
                               │  ┌────────────┐ ┌────────────┐  │
                               │  │ Task Store │ │  Scheduler │  │
                               │  │(PostgreSQL)│ │  Service   │  │
                               │  └────────────┘ └────────────┘  │
                               └──────────────────────────────────┘
```

---

## Фаза 4: Deep Dive

### Broker: Redis vs Kafka

```
Redis (с BLPOP / Streams):
  + Latency < 1ms (in-memory)
  + Встроенная поддержка sorted sets для priority queues и delayed tasks
  + XACK для at-least-once с acknowledgment
  - Ограниченная retention (память дорогая)
  - Не подходит для очень высокого throughput (>100K/sec)

Kafka:
  + Высокий throughput (миллионы/sec)
  + Retention 7+ дней
  + Consumer groups для масштабирования
  - Нет нативных delayed tasks (нужен обходной путь)
  - Нет приоритетов (нужно несколько топиков)
  - Задача нельзя "взять" атомарно → нужен external locking

Выбор: Redis Streams + Sorted Sets
  - 10K tasks/sec — Redis справится
  - Нативные delayed tasks через ZSET
  - XREADGROUP + XACK = at-least-once с tracking
  - Проще для operator'а
```

---

### Схема данных в Redis

**Очереди по приоритету:**
```
Redis List/Stream:
  queue:high   → XADD / XREADGROUP
  queue:normal → XADD / XREADGROUP
  queue:low    → XADD / XREADGROUP

Worker читает: сначала queue:high, если пусто → queue:normal, если пусто → queue:low
```

**Delayed tasks (sorted set по timestamp):**
```
Redis ZSET: delayed_tasks
  Score = execute_at (unix timestamp)
  Member = task_id

Scheduler job (каждые 500ms):
  ZRANGEBYSCORE delayed_tasks 0 {now}
  → перенести в соответствующую очередь
  → ZREM delayed_tasks {task_id}
```

**Pending ACK (at-least-once):**
```
Redis Streams автоматически ведут PEL (Pending Entry List):
  XREADGROUP → задача в PEL до XACK
  При падении worker → задача остаётся в PEL
  
Redelivery job (каждые 30 сек):
  XPENDING queue:high workers 0 + {idle_30sec}
  → XCLAIM → вернуть задачу в обработку
```

---

### Task Schema

```go
type Task struct {
    ID             string            `json:"id"`              // UUID v7 (time-ordered)
    Type           string            `json:"type"`            // "send_email", "resize_image"
    Payload        json.RawMessage   `json:"payload"`         // task-specific data
    Priority       Priority          `json:"priority"`        // high/normal/low
    Status         Status            `json:"status"`          // queued/running/done/failed
    MaxAttempts    int               `json:"max_attempts"`
    Attempt        int               `json:"attempt"`
    LastError      string            `json:"last_error,omitempty"`
    CreatedAt      time.Time         `json:"created_at"`
    ScheduledAt    time.Time         `json:"scheduled_at"`    // для delayed
    StartedAt      *time.Time        `json:"started_at,omitempty"`
    CompletedAt    *time.Time        `json:"completed_at,omitempty"`
    WorkerID       string            `json:"worker_id,omitempty"`
    IdempotencyKey string            `json:"idempotency_key,omitempty"`
}
```

---

### Worker: at-least-once и idempotency

**Worker lifecycle:**
```go
func (w *Worker) Run(ctx context.Context) {
    for {
        // 1. Claim задачу из Redis Stream
        task, err := w.broker.Claim(ctx, w.queues, timeout=30*time.Second)
        if err != nil { /* handle */ continue }

        // 2. Обновить статус: running + worker_id
        w.store.UpdateStatus(ctx, task.ID, StatusRunning, w.id)

        // 3. Выполнить с timeout
        taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
        err = w.handlers[task.Type].Handle(taskCtx, task.Payload)
        cancel()

        if err != nil {
            w.handleFailure(ctx, task, err)  // retry or DLQ
        } else {
            // 4. ACK + обновить статус: done
            w.broker.Ack(ctx, task.ID)
            w.store.UpdateStatus(ctx, task.ID, StatusDone, "")
        }
    }
}
```

**Idempotency на стороне worker:**
```go
func (h *SendEmailHandler) Handle(ctx context.Context, payload []byte) error {
    var p SendEmailPayload
    json.Unmarshal(payload, &p)

    // Idempotency check: не отправлять email если уже отправили
    sent, _ := h.cache.Get(ctx, "email_sent:" + p.IdempotencyKey)
    if sent != "" {
        return nil  // уже отправили, успех
    }

    if err := h.emailClient.Send(ctx, p); err != nil {
        return err
    }

    h.cache.Set(ctx, "email_sent:" + p.IdempotencyKey, "1", 24*time.Hour)
    return nil
}
```

---

### Retry Strategy

```go
func (w *Worker) handleFailure(ctx context.Context, task *Task, err error) {
    task.Attempt++
    task.LastError = err.Error()

    if task.Attempt >= task.MaxAttempts {
        // Исчерпаны попытки → Dead Letter Queue
        w.broker.MoveToDLQ(ctx, task)
        w.store.UpdateStatus(ctx, task.ID, StatusFailed, "")
        w.metrics.Inc("tasks.dlq", "type", task.Type)
        return
    }

    // Exponential backoff: 30s, 2min, 15min, 2h
    delay := time.Duration(math.Pow(8, float64(task.Attempt))) * time.Second
    delay = min(delay, 2*time.Hour)

    // Добавить обратно с задержкой
    task.ScheduledAt = time.Now().Add(delay)
    w.broker.Schedule(ctx, task)
    w.store.UpdateStatus(ctx, task.ID, StatusRetrying, "")
}
```

**Почему exponential backoff?**
- Transient errors (network hiccup, DB overload) обычно проходят за секунды
- Немедленный retry при постоянной ошибке = DDoS собственного сервиса
- Jitter (±20% к delay) предотвращает синхронный retry storm

---

### Persistent Storage (PostgreSQL)

**Для истории и мониторинга:**
```sql
CREATE TABLE tasks (
  id              VARCHAR(36) PRIMARY KEY,
  type            VARCHAR(100) NOT NULL,
  payload         JSONB NOT NULL,
  priority        SMALLINT NOT NULL DEFAULT 1,
  status          VARCHAR(20) NOT NULL DEFAULT 'queued',
  max_attempts    SMALLINT NOT NULL DEFAULT 3,
  attempt         SMALLINT NOT NULL DEFAULT 0,
  last_error      TEXT,
  idempotency_key VARCHAR(255) UNIQUE,
  worker_id       VARCHAR(100),
  scheduled_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at      TIMESTAMPTZ,
  completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_tasks_status_priority ON tasks(status, priority, scheduled_at);
CREATE INDEX idx_tasks_type_status ON tasks(type, status, created_at);
```

**Двойная запись:**
```
Enqueue:
  1. INSERT INTO tasks (PostgreSQL) — для durability и истории
  2. XADD queue:{priority} (Redis) — для fast dispatch

При падении Redis → scheduler может восстановить из PostgreSQL:
  SELECT * FROM tasks WHERE status = 'queued' AND scheduled_at <= NOW()
  → Re-enqueue в Redis
```

---

### Scheduler Service (cron и delayed)

```go
// Каждые 500ms: перенести delayed tasks в очередь
func (s *Scheduler) Run(ctx context.Context) {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // ZRANGEBYSCORE delayed_tasks 0 now LIMIT 1000
            tasks, _ := s.redis.GetReadyTasks(ctx, time.Now(), 1000)
            for _, task := range tasks {
                s.redis.Enqueue(ctx, task)
                s.redis.RemoveFromDelayed(ctx, task.ID)
            }
        }
    }
}

// Cron: "отправить digest каждый день в 9:00"
func (s *Scheduler) ScheduleCron(rule CronRule) {
    // Хранить cron rules в PostgreSQL
    // При наступлении времени → создать task + enqueue
    next := rule.CronExpr.Next(time.Now())
    s.redis.ZAdd("delayed_tasks", next.Unix(), rule.TaskTemplate)
}
```

**Leader election для scheduler:**
```
Несколько нод Scheduler — нужен один активный (иначе duplicate tasks).

Решение: Redis distributed lock
  SETNX scheduler:leader {node_id} EX 10
  Продлевать каждые 5 сек: EXPIRE scheduler:leader 10
  При потере лидера → другая нода захватит через 10 сек
```

---

### Мониторинг и операции

```
Метрики:
  queue_depth{priority=high}     — длина очереди
  task_processing_time{type}     — время выполнения
  task_failure_rate{type}        — % ошибок
  dlq_size                       — размер DLQ (алерт при росте)
  worker_concurrency             — активные воркеры

API:
  GET  /tasks/{id}               — статус задачи
  POST /tasks/{id}/retry         — ручной retry из DLQ
  GET  /tasks?status=failed&type=send_email  — поиск задач
  GET  /queues/stats             — глубина очередей

Алерты:
  queue_depth{priority=high} > 1000  — воркеров не хватает
  dlq_size > 100                     — много необработанных ошибок
  task_failure_rate > 5%             — проблема с конкретным типом задач
```

---

### Worker Autoscaling

```
Kubernetes HPA:
  Metric: queue_depth / worker_count (custom metric через KEDA)

KEDA (Kubernetes Event-Driven Autoscaling):
  scaleObject:
    triggers:
    - type: redis
      metadata:
        address: redis:6379
        listName: queue:high
        listLength: "10"  # 1 worker на 10 задач в очереди

  → При росте очереди → добавить Pod'ы
  → При пустой очереди → scale to zero (для экономии)
```

---

## Трейдоффы

| Решение | Принятое | Альтернатива | Когда менять |
|---|---|---|---|
| Broker | Redis Streams | Kafka | При > 100K tasks/sec или retention > 7 дней |
| Delivery | at-least-once | exactly-once | Если idempotency у worker невозможна |
| Scheduling | Redis ZSET + Cron Service | DB polling | При > 1M scheduled tasks |
| Worker | Stateless goroutines | Actor model | При complex state в workflow |
| Persistence | PostgreSQL | Cassandra | При > 10M tasks/day с retention > 30 дней |

---

## Что если Redis падает?

```
Сценарий: Redis недоступен 5 минут

1. Enqueue: новые задачи пишутся только в PostgreSQL, статус = queued
2. Воркеры: не могут читать из Redis → idle
3. Восстановление:
   - Redis поднялся
   - Recovery job: SELECT * FROM tasks WHERE status='queued' ORDER BY priority, scheduled_at LIMIT 1000
   - Re-enqueue в Redis
   - Задержка: 5 мин простоя + время recovery

Более resilient: Redis Sentinel или Redis Cluster для HA
  Master+Replica → автоматический failover < 30 сек
```

---

## Interview-ready ответ (2 минуты)

> "Task queue — это три основных challenge: надёжная доставка (at-least-once), эффективная диспетчеризация с приоритетами и delayed tasks, плюс масштабируемые воркеры.
>
> Broker: Redis Streams с consumer groups. Три очереди по приоритетам — воркер берёт сначала из high. Delayed tasks через Sorted Set по timestamp, Scheduler service раз в 500ms перекладывает готовые задачи в основную очередь.
>
> At-least-once: Redis XREADGROUP + XACK. Задача остаётся в Pending Entry List до явного ACK. При падении воркера — redelivery через XCLAIM после timeout.
>
> Retry: exponential backoff с jitter. После max attempts — DLQ с алертингом.
>
> PostgreSQL — долгосрочное хранилище для истории и мониторинга. Двойная запись: сначала PostgreSQL (durability), потом Redis (dispatch). При потере Redis — recovery из PostgreSQL.
>
> Масштабирование воркеров: stateless pods, KEDA autoscaling по глубине очереди."
