# Задача 1: Connection Pool

Connection pool — переиспользование дорогих ресурсов (TCP connections, БД handles). Знание устройства pool'а — критично для senior, потому что под капотом `database/sql`, `http.Transport`, `redis.Client` все используют один и тот же паттерн.

## Формулировка

> "Реализуй generic resource pool: Acquire/Release. Лимит на максимальный размер, переиспользование, health check на release."

Вариации:
- "Что делает database/sql.DB?"
- "Реализуй HTTP client pool"
- "Сделай TCP connection pool с lifecycle"

---

## Уточняющие вопросы

1. **Какой максимальный размер?**
   "Max active connections. Limit на total — иначе можно exhaust resources."

2. **Что делать когда лимит достигнут — ждать или error?**
   "Обычно ждать (с timeout через ctx). Reject — если приоритет latency over success."

3. **Idle timeout — закрывать неиспользуемые?**
   "Да. Иначе connections живут вечно, БД может их разорвать со своей стороны."

4. **Max lifetime — закрывать старые?**
   "Да. Защита от cached state (e.g., DNS TTL, SSL session refresh)."

5. **Health check при Acquire?**
   "Опционально. Стоит ping/validate. Но добавляет latency."

6. **Thread safe? Обязательно.**

---

## Базовое решение: buffered channel

Самый простой и идиоматичный Go-way — channel как pool.

```go
package pool

import (
    "context"
    "errors"
    "sync"
)

type Conn interface {
    Close() error
}

// Pool — простой connection pool через buffered channel.
type Pool[T Conn] struct {
    mu       sync.Mutex
    factory  func(context.Context) (T, error)
    closed   bool
    pool     chan T
    capacity int
}

func New[T Conn](capacity int, factory func(context.Context) (T, error)) *Pool[T] {
    return &Pool[T]{
        factory:  factory,
        pool:     make(chan T, capacity),
        capacity: capacity,
    }
}

// Acquire берёт connection из pool. Если pool пустой — создаёт новый.
// Блокируется ничего нет и достигнут лимит — ждёт пока кто-то Release.
// Не реализовано в простой версии — добавим в production.
func (p *Pool[T]) Acquire(ctx context.Context) (T, error) {
    var zero T

    if p.closed {
        return zero, errors.New("pool closed")
    }

    select {
    case c := <-p.pool:
        return c, nil
    default:
        // pool пуст — создать новое
        return p.factory(ctx)
    }
}

// Release возвращает connection в pool.
// Если pool полон — закрывает соединение.
func (p *Pool[T]) Release(c T) {
    if p.closed {
        c.Close()
        return
    }

    select {
    case p.pool <- c:
        // OK
    default:
        // pool полон — закрываем (соединение лишнее)
        c.Close()
    }
}

func (p *Pool[T]) Close() error {
    p.mu.Lock()
    if p.closed {
        p.mu.Unlock()
        return nil
    }
    p.closed = true
    close(p.pool)
    p.mu.Unlock()

    for c := range p.pool {
        c.Close()
    }
    return nil
}
```

**Использование:**

```go
pool := pool.New(10, func(ctx context.Context) (*MyConn, error) {
    return dialConn(ctx, "host:5432")
})
defer pool.Close()

conn, err := pool.Acquire(ctx)
if err != nil {
    return err
}
defer pool.Release(conn)

// ... use conn
```

**Что важно объяснить:**
- `select { default }` — non-blocking, если pool пустой — создаём новый
- `Release` non-blocking — если pool полон, лишний conn закрывается
- Channel сам обеспечивает thread safety (no manual mutex для acquire/release)

**Проблемы базового:**
- Нет лимита **общего** числа соединений (только pool buffer'а)
- Нет idle timeout
- Нет health check
- Нет ожидания при exhaustion

---

## Production-grade: лимит total, idle/lifetime, wait

```go
package pool

import (
    "context"
    "errors"
    "sync"
    "sync/atomic"
    "time"
)

type pooledConn[T Conn] struct {
    conn       T
    createdAt  time.Time
    lastUsedAt time.Time
}

type Pool[T Conn] struct {
    factory     func(context.Context) (T, error)
    healthCheck func(T) bool

    maxOpen     int           // лимит total active
    maxIdle     int           // лимит idle в pool'е
    maxIdleTime time.Duration // idle > этого = закрыть
    maxLifetime time.Duration // age > этого = закрыть

    mu       sync.Mutex
    idle     []*pooledConn[T]
    open     int          // active = idle + in-use
    waiters  []chan T     // ждут когда освободится
    closed   bool

    // Метрики
    acquired atomic.Int64
    waited   atomic.Int64
    created  atomic.Int64
    closed_  atomic.Int64
}

type Config[T Conn] struct {
    Factory     func(context.Context) (T, error)
    HealthCheck func(T) bool
    MaxOpen     int
    MaxIdle     int
    MaxIdleTime time.Duration
    MaxLifetime time.Duration
}

func New[T Conn](cfg Config[T]) *Pool[T] {
    p := &Pool[T]{
        factory:     cfg.Factory,
        healthCheck: cfg.HealthCheck,
        maxOpen:     cfg.MaxOpen,
        maxIdle:     cfg.MaxIdle,
        maxIdleTime: cfg.MaxIdleTime,
        maxLifetime: cfg.MaxLifetime,
    }
    if p.healthCheck == nil {
        p.healthCheck = func(T) bool { return true }
    }
    go p.cleanup()
    return p
}

func (p *Pool[T]) Acquire(ctx context.Context) (T, error) {
    p.acquired.Add(1)

    var zero T
    p.mu.Lock()

    if p.closed {
        p.mu.Unlock()
        return zero, errors.New("pool closed")
    }

    // Найти валидный idle connection
    for len(p.idle) > 0 {
        n := len(p.idle) - 1
        pc := p.idle[n]
        p.idle = p.idle[:n]

        // Проверки lifecycle
        if p.isExpired(pc) || !p.healthCheck(pc.conn) {
            pc.conn.Close()
            p.open--
            p.closed_.Add(1)
            continue
        }

        pc.lastUsedAt = time.Now()
        p.mu.Unlock()
        return pc.conn, nil
    }

    // Idle пуст. Можно создать новое?
    if p.open < p.maxOpen {
        p.open++
        p.mu.Unlock()

        conn, err := p.factory(ctx)
        if err != nil {
            p.mu.Lock()
            p.open--
            p.mu.Unlock()
            return zero, err
        }
        p.created.Add(1)
        return conn, nil
    }

    // Лимит достигнут — встаём в очередь
    p.waited.Add(1)
    ch := make(chan T, 1)
    p.waiters = append(p.waiters, ch)
    p.mu.Unlock()

    select {
    case conn := <-ch:
        return conn, nil
    case <-ctx.Done():
        // Удалить себя из waiters
        p.mu.Lock()
        for i, w := range p.waiters {
            if w == ch {
                p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
                break
            }
        }
        p.mu.Unlock()
        return zero, ctx.Err()
    }
}

func (p *Pool[T]) Release(conn T) {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.closed {
        conn.Close()
        p.open--
        return
    }

    // Если кто-то ждёт — отдать ему напрямую
    if len(p.waiters) > 0 {
        w := p.waiters[0]
        p.waiters = p.waiters[1:]
        w <- conn
        return
    }

    // Иначе — в idle pool
    if len(p.idle) >= p.maxIdle {
        conn.Close()
        p.open--
        p.closed_.Add(1)
        return
    }

    p.idle = append(p.idle, &pooledConn[T]{
        conn:       conn,
        createdAt:  time.Now(),  // simplified — реально надо сохранить
        lastUsedAt: time.Now(),
    })
}

// cleanup периодически удаляет expired idle connections.
func (p *Pool[T]) cleanup() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        p.mu.Lock()
        if p.closed {
            p.mu.Unlock()
            return
        }
        var kept []*pooledConn[T]
        for _, pc := range p.idle {
            if p.isExpired(pc) {
                pc.conn.Close()
                p.open--
            } else {
                kept = append(kept, pc)
            }
        }
        p.idle = kept
        p.mu.Unlock()
    }
}

func (p *Pool[T]) isExpired(pc *pooledConn[T]) bool {
    now := time.Now()
    if p.maxIdleTime > 0 && now.Sub(pc.lastUsedAt) > p.maxIdleTime {
        return true
    }
    if p.maxLifetime > 0 && now.Sub(pc.createdAt) > p.maxLifetime {
        return true
    }
    return false
}

type Stats struct {
    Acquired int64
    Waited   int64
    Created  int64
    Closed   int64
}

func (p *Pool[T]) Stats() Stats {
    return Stats{
        Acquired: p.acquired.Load(),
        Waited:   p.waited.Load(),
        Created:  p.created.Load(),
        Closed:   p.closed_.Load(),
    }
}
```

**Что добавлено:**
- **maxOpen** vs **maxIdle** — разные лимиты на active vs cached
- **idle timeout / max lifetime** — лечит "застрявшие" соединения
- **Waiter queue** — если лимит достигнут, новый caller ждёт через channel
- **Health check** на acquire
- **Metrics** — acquired/waited/created/closed
- **Cleanup goroutine** — фоновое удаление expired

---

## Тесты

```go
func TestPool_BasicAcquireRelease(t *testing.T) {
    var created atomic.Int32

    p := New(Config[*fakeConn]{
        MaxOpen: 5,
        MaxIdle: 3,
        Factory: func(ctx context.Context) (*fakeConn, error) {
            created.Add(1)
            return &fakeConn{}, nil
        },
    })
    defer p.Close()

    // Take 3, release 3 → должно переиспользовать
    for i := 0; i < 3; i++ {
        c, err := p.Acquire(context.Background())
        if err != nil {
            t.Fatal(err)
        }
        p.Release(c)
    }

    if created.Load() > 3 {
        t.Errorf("created %d, expected <= 3", created.Load())
    }
}

func TestPool_LimitsTotal(t *testing.T) {
    p := New(Config[*fakeConn]{
        MaxOpen: 2,
        MaxIdle: 2,
        Factory: func(ctx context.Context) (*fakeConn, error) {
            return &fakeConn{}, nil
        },
    })
    defer p.Close()

    c1, _ := p.Acquire(context.Background())
    c2, _ := p.Acquire(context.Background())

    // Третий — должен ждать
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()
    _, err := p.Acquire(ctx)
    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected timeout, got %v", err)
    }

    p.Release(c1)
    p.Release(c2)
}

func TestPool_WaitsForRelease(t *testing.T) {
    p := New(Config[*fakeConn]{
        MaxOpen: 1,
        MaxIdle: 1,
        Factory: func(ctx context.Context) (*fakeConn, error) {
            return &fakeConn{}, nil
        },
    })
    defer p.Close()

    c1, _ := p.Acquire(context.Background())

    // Третий ждёт
    done := make(chan bool)
    go func() {
        c2, _ := p.Acquire(context.Background())
        _ = c2
        done <- true
    }()

    time.Sleep(20 * time.Millisecond)
    p.Release(c1)

    select {
    case <-done:
        // OK
    case <-time.After(100 * time.Millisecond):
        t.Error("waiter didn't get connection after release")
    }
}
```

---

## Подводные камни

### 1. Counter `open` без mutex

Acquire `p.open < maxOpen` и Acquire `p.open++` — race condition между двумя horizutines. Всегда под mutex'ом.

### 2. Waiter не получил уведомление при close

```go
p.Close()
// waiters висят навсегда
```

При Close — нужно закрыть все waiter channels (или послать err).

### 3. Health check блокирует acquire

Если health check включает ping (network), Acquire становится медленнее. Trade-off: проверять только перед использованием, не на release.

### 4. Pooled connection остался unused при ctx.Done

```go
conn, _ := pool.Acquire(ctx)
// ... ctx cancelled ...
// Forgot to Release(conn) — leak
```

Обычно решается через `defer pool.Release(conn)`. Если caller забыл — пулом нельзя ничего сделать.

### 5. Resource leak при panic

```go
conn, _ := pool.Acquire(ctx)
defer pool.Release(conn)
panic("oops")  // ← defer вызовется, OK
```

`defer pool.Release()` спасает.

### 6. Stale connections — server side close

БД закрыла соединение со своей стороны (idle timeout на server). Pool не знает. При следующем acquire — connection даёт ошибку.

**Решение:** retry once при ошибке от соединения из pool'а — если pool'ed conn invalid, открыть новое. database/sql делает это автоматически.

### 7. Buffer size = MaxOpen

```go
pool := make(chan Conn, maxOpen)  // ← это buffer pool'а, не максимальный open
```

Buffer'a channel'а — сколько **idle** можно хранить. Не то же самое что лимит на total.

### 8. Goroutine leak в cleanup

```go
go p.cleanup()
// Никогда не выйдет если pool не Close'нут
```

`cleanup` должна реагировать на `p.closed` или receive из `ctx.Done()`.

### 9. Health check возвращает false для всех

Все idle отбрасываются → создаются новые → factory bombs. Если бэкенд down — нужно тоже понимать (через circuit breaker наверху).

### 10. Pool без max — DoS на себя

```go
p := New(0, factory)  // ← unlimited!
```

Каждый запрос создаёт connection → OOM или exhausting downstream. Всегда maxOpen > 0.

---

## Возможные расширения

### 1. Lazy pooling

Не создавать min connections заранее, только по запросу.

### 2. Eager warm-up

Наоборот — создать min connections на старте чтобы первые запросы не ждали.

### 3. Per-target pool

Один pool struct, внутри map[host] → pool. Используется в `http.Transport`.

### 4. Stats для Prometheus

Expose metrics — acquire latency histogram, in-use gauge, wait time.

### 5. Graceful close

Подождать пока все active соединения вернутся в pool, потом закрыть.

### 6. Connection-level retry

Если pool'ed соединение возвращает ошибку — retry с новым.

---

## Что важно показать на собеседовании

1. **Channel как pool** — идиоматичный Go-way
2. **MaxOpen vs MaxIdle** — два разных лимита
3. **Waiter queue с context** — правильное ожидание с возможностью отмены
4. **Lifecycle (idle/max lifetime)** — почему нужно
5. **Health check** — на acquire вместо при release
6. **Metrics** — для production observability
7. **database/sql.DB** — стандартный пример pool'а в Go

## Связки

- [Database connection pooling](../../../06-databases/database-systems-catalog/postgresql/09-connection-pooling.md)
- [Bulkhead](../../../05-system-design/reliability-patterns/07-bulkhead.md) — connection pool per dependency
- [database/sql docs](https://pkg.go.dev/database/sql) — реальный example pool'а
