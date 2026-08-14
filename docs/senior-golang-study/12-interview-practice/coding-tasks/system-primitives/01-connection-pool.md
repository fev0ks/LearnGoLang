# Задача 1: Connection Pool

## Содержание

- [Контракт задачи](#контракт-задачи)
- [Инварианты](#инварианты)
- [Реализация с lease](#реализация-с-lease)
- [Acquire, Release и Close](#acquire-release-и-close)
- [Lifetime и health check](#lifetime-и-health-check)
- [Настройка capacity](#настройка-capacity)
- [Тестирование и метрики](#тестирование-и-метрики)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Pool переиспользует дорогие ресурсы и одновременно ограничивает их число.
Буферизированный канал только хранит idle objects; сам по себе он не ограничивает
число созданных ресурсов и не решает гонку `Close` с `Release`.

---

## Контракт задачи

Перед кодом нужно уточнить:

1. `MaxOpen` ограничивает idle или все созданные ресурсы?
2. Что делает `Acquire`, когда limit достигнут: ждёт, отклоняет или создаёт ещё?
3. Как caller возвращает broken resource?
4. Ждёт ли `Close` занятые ресурсы и можно ли ограничить ожидание context-ом?
5. Нужны ли max lifetime, max idle time и health check?
6. Как защищаться от double release?

Ниже `MaxOpen` — строгий предел `idle + in-use + factory in-flight`.
`Acquire` ждёт с `context`, а `Close` запрещает новые acquisitions, закрывает
idle и ждёт возврата in-use до deadline.

---

## Инварианты

Для любого состояния pool:

```text
0 <= idle <= MaxIdle <= MaxOpen
open = idle + in-use + factory-in-flight
```

Каждый успешно созданный resource ровно один раз либо находится в pool/lease,
либо закрыт. Нельзя закрывать channel, в который concurrent `Release` ещё может
отправить: это приводит к panic. В реализации ниже ожидатели просыпаются через
versioned notification channel, а сами resources хранятся под mutex.

---

## Реализация с lease

Lease хранит метаданные момента создания и не позволяет вернуть один resource
дважды. Это надёжнее API `Acquire() T` + `Release(T)`, где pool не отличает
легитимный возврат от double release.

```go
package pool

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"
)

var (
    ErrClosed        = errors.New("pool is closed")
    ErrLeaseReleased = errors.New("lease already released")
)

type Config struct {
    MaxOpen        int
    MaxIdle        int
    MaxLifetime    time.Duration
    MaxIdleTime    time.Duration
}

type entry[T any] struct {
    value      T
    createdAt  time.Time
    lastUsedAt time.Time
}

type Pool[T any] struct {
    cfg     Config
    factory func(context.Context) (T, error)
    closeFn func(T) error
    healthy func(context.Context, T) error
    now     func() time.Time

    mu      sync.Mutex
    idle    []entry[T]
    open    int
    closed  bool
    changed chan struct{}
}

type Lease[T any] struct {
    pool  *Pool[T]
    entry entry[T]

    mu       sync.Mutex
    released bool
}

func New[T any](
    cfg Config,
    factory func(context.Context) (T, error),
    closeFn func(T) error,
    healthy func(context.Context, T) error,
) (*Pool[T], error) {
    if cfg.MaxOpen < 1 {
        return nil, fmt.Errorf("max open must be positive")
    }
    if cfg.MaxIdle < 0 || cfg.MaxIdle > cfg.MaxOpen {
        return nil, fmt.Errorf("max idle must be in [0, max open]")
    }
    if cfg.MaxLifetime < 0 || cfg.MaxIdleTime < 0 {
        return nil, fmt.Errorf("lifetimes must not be negative")
    }
    if factory == nil || closeFn == nil {
        return nil, fmt.Errorf("factory and close function are required")
    }

    return &Pool[T]{
        cfg:     cfg,
        factory: factory,
        closeFn: closeFn,
        healthy: healthy,
        now:     time.Now,
        changed: make(chan struct{}),
    }, nil
}

func (p *Pool[T]) signalLocked() {
    close(p.changed)
    p.changed = make(chan struct{})
}

func (p *Pool[T]) expired(e entry[T], now time.Time) bool {
    lifetimeExpired := p.cfg.MaxLifetime > 0 &&
        now.Sub(e.createdAt) >= p.cfg.MaxLifetime
    idleExpired := p.cfg.MaxIdleTime > 0 &&
        now.Sub(e.lastUsedAt) >= p.cfg.MaxIdleTime
    return lifetimeExpired || idleExpired
}
```

Метаданные принадлежат pool, поэтому `createdAt` не сбрасывается при каждом
`Release`. Иначе `MaxLifetime` незаметно превращается в ещё один idle timeout.

---

## Acquire, Release и Close

Factory, health check и resource `Close` могут блокироваться, поэтому они не
вызываются под mutex pool-а.

```go
func (p *Pool[T]) Acquire(ctx context.Context) (*Lease[T], error) {
    for {
        if err := ctx.Err(); err != nil {
            return nil, err
        }

        p.mu.Lock()
        if p.closed {
            p.mu.Unlock()
            return nil, ErrClosed
        }

        if n := len(p.idle); n > 0 {
            e := p.idle[n-1]
            p.idle = p.idle[:n-1]
            p.mu.Unlock()

            if p.expired(e, p.now()) {
                _ = p.retire(e.value)
                continue
            }
            if p.healthy != nil {
                if err := p.healthy(ctx, e.value); err != nil {
                    if ctx.Err() != nil {
                        _ = p.put(e, false)
                        return nil, ctx.Err()
                    }
                    _ = p.retire(e.value)
                    continue
                }
            }
            return &Lease[T]{pool: p, entry: e}, nil
        }

        if p.open < p.cfg.MaxOpen {
            p.open++
            p.signalLocked()
            p.mu.Unlock()

            value, err := p.factory(ctx)
            if err != nil {
                p.decrementOpen()
                return nil, fmt.Errorf("create resource: %w", err)
            }

            now := p.now()
            e := entry[T]{value: value, createdAt: now, lastUsedAt: now}

            p.mu.Lock()
            closed := p.closed
            p.mu.Unlock()
            if closed || ctx.Err() != nil {
                _ = p.retire(value)
                if ctx.Err() != nil {
                    return nil, ctx.Err()
                }
                return nil, ErrClosed
            }
            return &Lease[T]{pool: p, entry: e}, nil
        }

        changed := p.changed
        p.mu.Unlock()

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-changed:
        }
    }
}

func (l *Lease[T]) Value() T {
    return l.entry.value
}

func (l *Lease[T]) Release(discard bool) error {
    l.mu.Lock()
    if l.released {
        l.mu.Unlock()
        return ErrLeaseReleased
    }
    l.released = true
    l.mu.Unlock()

    return l.pool.put(l.entry, discard)
}

func (p *Pool[T]) put(e entry[T], discard bool) error {
    now := p.now()

    p.mu.Lock()
    shouldClose := discard || p.closed || p.expired(e, now) ||
        len(p.idle) >= p.cfg.MaxIdle
    if !shouldClose {
        e.lastUsedAt = now
        p.idle = append(p.idle, e)
        p.signalLocked()
        p.mu.Unlock()
        return nil
    }
    p.mu.Unlock()

    return p.retire(e.value)
}

func (p *Pool[T]) retire(value T) error {
    err := p.closeFn(value)
    p.decrementOpen()
    return err
}

func (p *Pool[T]) decrementOpen() {
    p.mu.Lock()
    p.open--
    p.signalLocked()
    p.mu.Unlock()
}

func (p *Pool[T]) Close(ctx context.Context) error {
    p.mu.Lock()
    if !p.closed {
        p.closed = true
        idle := p.idle
        p.idle = nil
        p.open -= len(idle)
        p.signalLocked()
        p.mu.Unlock()

        for _, e := range idle {
            _ = p.closeFn(e.value)
        }
    } else {
        p.mu.Unlock()
    }

    for {
        p.mu.Lock()
        if p.open == 0 {
            p.mu.Unlock()
            return nil
        }
        changed := p.changed
        p.mu.Unlock()

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-changed:
        }
    }
}
```

`Close` не отбирает in-use resources: он ждёт их возврата. Если deadline
истёк, pool остаётся закрытым, а последующие `Release` всё равно закроют свои
resources. Ошибки закрытия idle resources в компактном примере не агрегируются;
production API должен решить, возвращать ли `errors.Join` и как метрифицировать
такие ошибки.

Успешная проверка `closed` под mutex является linearization point выдачи нового
lease. Если `Close` захватит mutex сразу после неё, acquisition логически уже
произошёл: `open` включает этот resource, поэтому `Close` дождётся его возврата.
Если `Close` успел раньше, factory result закрывается через `retire` и caller
получает `ErrClosed`.

---

## Lifetime и health check

Пример делает lazy eviction перед выдачей и при возврате. Поэтому idle resource
может физически лежать дольше `MaxIdleTime`, пока нет следующей операции. Если
нужно освобождать capacity по времени без трафика, добавляют cleanup goroutine с
явными `stop`/`done`; expired entries отсоединяют под mutex, а закрывают снаружи.

Health check перед каждым acquire добавляет latency и нагрузку. Для DB connection
часто лучше:

- доверять ошибке реальной операции и discard broken connection;
- проверять только давно idle connection;
- ограничивать ping отдельным timeout;
- не считать caller cancellation признаком broken connection автоматически.

Для `database/sql` собственный pool обычно не нужен: `*sql.DB` уже является
concurrent-safe pool. `sql.Open` часто не устанавливает соединение сразу;
доступность проверяет `PingContext`. Настраивают `SetMaxOpenConns`,
`SetMaxIdleConns`, `SetConnMaxLifetime`, `SetConnMaxIdleTime` и наблюдают
`DBStats`.

---

## Настройка capacity

Оценка через закон Литтла:

```text
in-flight = throughput * average service time
```

При 200 DB operations/s и среднем времени 25 ms ожидаемый concurrency:

```text
200 * 0.025 = 5 connections
```

Это среднее, не готовый `MaxOpen`. Нужны headroom для variance/peak и общий
лимит БД. Если 20 pods имеют `MaxOpen=30`, потенциально это 600 connections.
Этот предел должен помещаться в capacity PostgreSQL с резервом для migrations,
admin и background jobs.

Слишком маленький pool увеличивает wait duration; слишком большой переносит
очередь в БД, повышает memory/context switching и может ухудшить latency.

---

## Тестирование и метрики

Нужны детерминированные barriers, а не `Sleep`:

1. одновременно создаётся не больше `MaxOpen`;
2. waiter просыпается после `Release`;
3. canceled waiter не забирает и не теряет resource;
4. factory error освобождает capacity;
5. broken/expired resource закрывается и заменяется;
6. double release отклоняется;
7. `Close` гоняется с factory и `Release` без panic/race;
8. deadline `Close` не переоткрывает pool;
9. каждый созданный resource закрывается ровно один раз.

Метрики: open, in-use, idle, factory-in-flight, wait count/duration, acquire
timeout, created/closed по причинам и health-check latency. Для `database/sql`
часть уже доступна в `DBStats`, включая `WaitCount`, `WaitDuration` и причины
закрытия.

---

## Типичные ошибки

- Использовать capacity канала как `MaxOpen`, создавая resources вне лимита.
- Закрыть channel в `Close`, пока `Release` может в него отправить.
- Получить zero value из закрытого channel и вернуть его как успешный acquire.
- Выполнять factory, ping или network close под общим mutex.
- Сбрасывать `createdAt` при возврате и тем самым отключать max lifetime.
- Потерять resource в гонке между `ctx.Done()` и handoff waiter-у.
- Не защищаться от double release.
- Забыть суммарный connection budget всех replicas.

---

## Interview-ready answer

1. **Какие инварианты у connection pool?**
   - **Strict limit —** `idle + in-use + creating <= MaxOpen`.
   - **Ownership —** resource находится ровно в одном месте и закрывается один
     раз.
   - **Cancelable wait —** достижение limit не блокирует caller навсегда.

2. **Почему недостаточно buffered channel?**
   - **Idle only —** capacity ограничивает сохранённые значения, не число
     созданных.
   - **Shutdown race —** concurrent send в закрытый channel вызывает panic.
   - **Metadata —** lifetime, health и double release требуют отдельного
     ownership state.

3. **Как выбирать размер?**
   - **Demand —** начать с throughput × service time и измерений ожидания.
   - **Global budget —** умножить per-instance limit на число replicas.
   - **Trade-off —** малый pool создаёт локальную очередь, большой перегружает
     downstream.

---

## Связанные материалы

- [`database/sql`](https://pkg.go.dev/database/sql)
- [Connection Pooling](../../../06-databases/database-systems-catalog/postgresql/09-connection-pooling.md)
- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md)
