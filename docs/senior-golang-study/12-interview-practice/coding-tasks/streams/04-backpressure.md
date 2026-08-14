# Задача 4: Backpressure Handling

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Стратегии](#стратегии)
- [Bounded buffer](#bounded-buffer-с-атомарным-drop-oldest)
- [Priority и adaptive shedding](#priority-и-adaptive-load-shedding)
- [Sampling](#sampling-и-reservoir-sampling)
- [Capacity planning](#как-оценить-capacity)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Возможные расширения](#возможные-расширения)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Backpressure — это реакция системы на ситуацию, когда входной поток временно или
постоянно быстрее обработки. Неограниченная очередь маскирует проблему до OOM;
bounded queue заставляет заранее выбрать: замедлить, отклонить, отбросить,
сэмплировать или надёжно вынести backlog во внешнее хранилище.

---

## Формулировка

> Producer создаёт события быстрее consumer. Реализуй bounded mediator с
> context-aware `Add`, явной overflow policy, безопасным concurrent lifecycle и
> метриками потерь.

Типичные случаи:

- медленная БД или внешний API;
- кратковременный traffic burst;
- noisy tenant, влияющий на соседей;
- telemetry, где часть данных разрешено sampling/drop;
- worker pool с фиксированной параллельностью.

---

## Уточняющие вопросы

1. **Разрешена ли потеря?**
   Для telemetry — иногда да, для payment command — обычно нет.
2. **Может ли producer замедлиться?**
   Внутренняя goroutine может ждать; внешний webhook-клиент может только получить
   `429/503` и решить, повторять ли запрос.
3. **Нужно ли пережить process crash?**
   In-memory buffer этого не умеет — нужен broker/WAL/durable queue.
4. **Что важнее при потере: свежесть или полнота?**
   От этого зависит `drop oldest` против `drop newest`.
5. **Есть ли priorities/tenants?**
   Один общий FIFO позволяет noisy neighbor занять всю capacity.
6. **Какой допустим queueing delay?**
   Большой buffer уменьшает кратковременные rejects, но увеличивает stale work и
   latency.

---

## Стратегии

| Стратегия | Что происходит при full | Где уместна | Главный риск |
|---|---|---|---|
| Block | producer ждёт место | контролируемый pipeline без потерь | timeout/deadlock/cascade |
| Reject / drop newest | новый item не принимается | admission control, свежий backlog не нужен | потеря новых данных |
| Drop oldest | старый waiting item заменяется новым | gauges, latest-state telemetry | потеря уже принятого item |
| Sample | принимается подмножество | traces/analytics | sampling bias |
| Spill to durable queue | overflow пишется на disk/broker | данные должны пережить сбой | operational complexity |
| Scale consumers | растёт service rate | параллелизуемая обработка | ordering, downstream limit |

Ни одна policy не исправляет постоянное неравенство `arrival rate > service
rate`. Она лишь определяет поведение до масштабирования, снижения входа или
деградации качества.

### Block

Block сохраняет данные только пока жив процесс и caller не отменил ожидание. Он
распространяет давление upstream, но это не означает автоматическую end-to-end
гарантию: protocol/client должны уметь замедляться или retry.

### Drop newest и drop oldest

`drop newest` сохраняет уже накопленную FIFO-очередь. `drop oldest` сохраняет
самые свежие waiting items. Для command/event log оба обычно неприемлемы без
явного business contract.

### Durable queue

Если успешный приём должен пережить crash, acknowledgement дают только после
durable write. Redis/Kafka/локальный WAL — не взаимозаменяемые слова: нужно
отдельно определить replication, fsync, retention, replay и deduplication.

---

## Bounded buffer с атомарным `drop oldest`

Два channel operations — «прочитать старое, затем записать новое» — не являются
одной атомарной заменой. Между ними другой consumer может изменить channel.
Кольцевой buffer под одним mutex делает eviction и insertion одной critical
section.

```go
package backpressure

import (
    "context"
    "errors"
    "sync"
)

var (
    ErrClosed        = errors.New("buffer: closed")
    ErrBufferFull    = errors.New("buffer: full")
    ErrInvalidConfig = errors.New("buffer: invalid configuration")
)

type Policy int

const (
    PolicyBlock Policy = iota
    PolicyDropNewest
    PolicyDropOldest
)

type Buffer[T any] struct {
    mu      sync.Mutex
    storage []T
    head    int
    size    int
    policy  Policy
    closed  bool

    cond *sync.Cond

    accepted int64
    taken    int64
    rejected int64 // новый item не был принят
    evicted  int64 // ранее принятый waiting item был удалён
}

func New[T any](capacity int, policy Policy) (*Buffer[T], error) {
    validPolicy := policy >= PolicyBlock && policy <= PolicyDropOldest
    if capacity <= 0 || !validPolicy {
        return nil, ErrInvalidConfig
    }
    b := &Buffer[T]{
        storage: make([]T, capacity),
        policy:  policy,
    }
    b.cond = sync.NewCond(&b.mu)
    return b, nil
}

func (b *Buffer[T]) Add(ctx context.Context, item T) error {
    if ctx == nil {
        return ErrInvalidConfig
    }
    if err := ctx.Err(); err != nil {
        return err
    }

    b.mu.Lock()
    defer b.mu.Unlock()

    for {
        if b.closed {
            return ErrClosed
        }
        if err := ctx.Err(); err != nil {
            return err
        }

        if b.size < len(b.storage) {
            b.pushLocked(item)
            b.accepted++
            b.cond.Broadcast()
            return nil
        }

        switch b.policy {
        case PolicyDropNewest:
            b.rejected++
            return ErrBufferFull

        case PolicyDropOldest:
            _, _ = b.popLocked()
            b.evicted++
            b.pushLocked(item)
            b.accepted++
            b.cond.Broadcast()
            return nil

        case PolicyBlock:
            stopWakeup := context.AfterFunc(ctx, func() {
                b.mu.Lock()
                b.cond.Broadcast()
                b.mu.Unlock()
            })
            b.cond.Wait()
            stopWakeup()
        }
    }
}

// Take возвращает ok=false после Close, когда уже drained весь buffer.
func (b *Buffer[T]) Take(ctx context.Context) (item T, ok bool, err error) {
    if ctx == nil {
        return item, false, ErrInvalidConfig
    }
    if err := ctx.Err(); err != nil {
        return item, false, err
    }

    b.mu.Lock()
    defer b.mu.Unlock()

    for {
        if err := ctx.Err(); err != nil {
            return item, false, err
        }
        if b.size > 0 {
            item, _ = b.popLocked()
            b.taken++
            b.cond.Broadcast()
            return item, true, nil
        }
        if b.closed {
            return item, false, nil
        }

        stopWakeup := context.AfterFunc(ctx, func() {
            b.mu.Lock()
            b.cond.Broadcast()
            b.mu.Unlock()
        })
        b.cond.Wait()
        stopWakeup()
    }
}

func (b *Buffer[T]) TryTake() (item T, ok bool) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if b.size == 0 {
        return item, false
    }
    item, _ = b.popLocked()
    b.taken++
    b.cond.Broadcast()
    return item, true
}

func (b *Buffer[T]) Close() {
    b.mu.Lock()
    defer b.mu.Unlock()

    if b.closed {
        return
    }
    b.closed = true
    b.cond.Broadcast() // разбудить всех blocked Add/Take
}

func (b *Buffer[T]) pushLocked(item T) {
    tail := (b.head + b.size) % len(b.storage)
    b.storage[tail] = item
    b.size++
}

func (b *Buffer[T]) popLocked() (item T, ok bool) {
    if b.size == 0 {
        return item, false
    }
    item = b.storage[b.head]
    var zero T
    b.storage[b.head] = zero // не удерживать ссылку после dequeue
    b.head = (b.head + 1) % len(b.storage)
    b.size--
    return item, true
}

type Stats struct {
    Accepted int64
    Taken    int64
    Rejected int64
    Evicted  int64
    Buffered int
    Capacity int
    Closed   bool
}

func (b *Buffer[T]) Stats() Stats {
    b.mu.Lock()
    defer b.mu.Unlock()
    return Stats{
        Accepted: b.accepted,
        Taken:    b.taken,
        Rejected: b.rejected,
        Evicted:  b.evicted,
        Buffered: b.size,
        Capacity: len(b.storage),
        Closed:   b.closed,
    }
}
```

`sync.Cond` не умеет ждать context напрямую, поэтому `context.AfterFunc`
пробуждает waiters при cancellation. После каждого wake условие проверяется
заново под mutex: пробуждение нескольких goroutines безопасно. Цена подхода —
thundering herd при большом числе waiters; оптимизировать его стоит после profile.

---

## Priority и adaptive load shedding

Три отдельных channel и «проверить high, затем `select` по всем» не дают strict
priority: если несколько cases готовы, `select` выбирает одну из них
псевдослучайно. Strict high-first, напротив, может навсегда оставить low queue без
обработки.

Практичный вариант:

- отдельная capacity на priority/tenant, чтобы low не занял место critical;
- weighted round-robin, например `high:normal:low = 8:4:1`;
- aging: долго ожидающий item постепенно повышает эффективный priority;
- reserve для critical и общий global limit;
- admission policy проверяет как global pressure, так и заполнение конкретной
  queue.

Пример политики деградации:

| Состояние | Low | Normal | High |
|---|---:|---:|---:|
| Healthy | accept | accept | accept |
| Normal queue saturated | accept в своём лимите | reject | accept |
| Global overload | drop/sample | reject | accept из reserve или block |

Это именно policy, а не универсальные thresholds `50%/90%`. Thresholds выбирают
по queueing delay, downstream latency и данным load test. Snapshot `len/cap` уже
устаревает к моменту решения, поэтому его используют как приблизительный сигнал,
а hard capacity остаётся окончательной защитой.

---

## Sampling и reservoir sampling

«Каждое N-е событие» — systematic sampling. Оно может быть сильно biased, если
у входа есть периодичность, совпадающая с `N`. Для независимого probabilistic
sampling каждое событие принимают с заданной вероятностью, а решение и effective
sample rate записывают в metadata.

Reservoir sampling решает другую задачу: после потока неизвестной длины оставить
равномерную выборку фиксированного размера. Algorithm R:

```go
package backpressure

import (
    "errors"
    "math"
    "math/rand"
    "sync"
)

type Reservoir[T any] struct {
    mu    sync.Mutex
    rng   *rand.Rand // принадлежит Reservoir и защищён тем же mutex
    items []T
    seen  int64
}

func NewReservoir[T any](size int, rng *rand.Rand) (*Reservoir[T], error) {
    if size <= 0 || rng == nil {
        return nil, ErrInvalidConfig
    }
    return &Reservoir[T]{
        rng:   rng,
        items: make([]T, 0, size),
    }, nil
}

func (r *Reservoir[T]) Add(item T) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if r.seen == math.MaxInt64 {
        return errors.New("reservoir: sample counter overflow")
    }
    r.seen++
    if len(r.items) < cap(r.items) {
        r.items = append(r.items, item)
        return nil
    }

    index := r.rng.Int63n(r.seen)
    if index < int64(len(r.items)) {
        r.items[index] = item
    }
    return nil
}

func (r *Reservoir[T]) Snapshot() []T {
    r.mu.Lock()
    defer r.mu.Unlock()
    return append([]T(nil), r.items...)
}
```

Reservoir не является очередью для последующей обработки: он намеренно забывает
большинство событий и нужен для анализа выборки.

---

## Как оценить capacity

Для краткого burst backlog растёт примерно так:

```text
backlog = max(0, arrival_rate - service_rate) × burst_duration
```

Допущение: burst даёт `5_000 events/s`, consumer обрабатывает `3_000 events/s`,
burst длится `2s`. Тогда нужно место примерно для
`(5_000 - 3_000) × 2 = 4_000` events. Если средний payload равен `2 KB`, только
payload займёт около `8 MB`; сверху будут Go objects, pointers и allocator
overhead, которые измеряют heap profile.

Queueing delay можно приблизить законом Литтла:

```text
queued_items ≈ throughput × average_queueing_time
```

Например, backlog `4_000` при service rate `3_000/s` добавляет примерно `1.33s`
ожидания последнему item, если новый вход прекратился. Если вход продолжает
стабильно превышать обработку, никакая конечная capacity не решает проблему.

---

## Тесты

```go
func mustBuffer[T any](t *testing.T, capacity int, policy Policy) *Buffer[T] {
    t.Helper()
    b, err := New[T](capacity, policy)
    if err != nil {
        t.Fatal(err)
    }
    return b
}

func TestBuffer_DropNewest(t *testing.T) {
    b := mustBuffer[int](t, 2, PolicyDropNewest)
    if err := b.Add(context.Background(), 1); err != nil {
        t.Fatal(err)
    }
    if err := b.Add(context.Background(), 2); err != nil {
        t.Fatal(err)
    }
    if err := b.Add(context.Background(), 3); !errors.Is(err, ErrBufferFull) {
        t.Fatalf("third Add error = %v", err)
    }

    first, _, _ := b.Take(context.Background())
    second, _, _ := b.Take(context.Background())
    if first != 1 || second != 2 {
        t.Fatalf("got %d, %d", first, second)
    }
}

func TestBuffer_DropOldest(t *testing.T) {
    b := mustBuffer[int](t, 2, PolicyDropOldest)
    for _, item := range []int{1, 2, 3} {
        if err := b.Add(context.Background(), item); err != nil {
            t.Fatal(err)
        }
    }

    first, _, _ := b.Take(context.Background())
    second, _, _ := b.Take(context.Background())
    if first != 2 || second != 3 {
        t.Fatalf("got %d, %d", first, second)
    }
    if got := b.Stats().Evicted; got != 1 {
        t.Fatalf("Evicted = %d", got)
    }
}

func TestBuffer_BlockHonorsCancellation(t *testing.T) {
    b := mustBuffer[int](t, 1, PolicyBlock)
    if err := b.Add(context.Background(), 1); err != nil {
        t.Fatal(err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    if err := b.Add(ctx, 2); !errors.Is(err, context.Canceled) {
        t.Fatalf("Add error = %v", err)
    }
}

func TestBuffer_BlockedAddWakesAfterTake(t *testing.T) {
    b := mustBuffer[int](t, 1, PolicyBlock)
    if err := b.Add(context.Background(), 1); err != nil {
        t.Fatal(err)
    }

    result := make(chan error, 1)
    go func() { result <- b.Add(context.Background(), 2) }()

    first, ok, err := b.Take(context.Background())
    if err != nil || !ok || first != 1 {
        t.Fatalf("Take = %d, %v, %v", first, ok, err)
    }
    if err := <-result; err != nil {
        t.Fatal(err)
    }
}

func TestBuffer_CloseDrainsAndWakesWaiters(t *testing.T) {
    b := mustBuffer[int](t, 1, PolicyBlock)
    if err := b.Add(context.Background(), 1); err != nil {
        t.Fatal(err)
    }
    b.Close()
    b.Close() // idempotent

    item, ok, err := b.Take(context.Background())
    if err != nil || !ok || item != 1 {
        t.Fatalf("first Take = %d, %v, %v", item, ok, err)
    }
    _, ok, err = b.Take(context.Background())
    if err != nil || ok {
        t.Fatalf("drained Take = ok:%v err:%v", ok, err)
    }
    if err := b.Add(context.Background(), 2); !errors.Is(err, ErrClosed) {
        t.Fatalf("Add after Close error = %v", err)
    }
}
```

Concurrent stress-test нужно запускать с `go test -race` и проверять invariant:
`Accepted - Evicted = Taken + Buffered` после остановки producers.

---

## Подводные камни

### 1. Неатомарный `drop oldest` на channel

`<-ch`, затем `ch <- item` — две операции. При нескольких producers/consumers
результат зависит от interleaving, а при capacity `0` retry loop может стать
бесконечным.

### 2. Goroutine на каждый blocked `Add`

```go
go buffer.Add(ctx, item)
```

Это превращает goroutines в неограниченную скрытую очередь и уничтожает смысл
backpressure.

### 3. Считать большой buffer решением

Большая очередь добавляет latency и память. Она полезна только для измеренного
burst, после которого service rate способен догнать вход.

### 4. Смешивать loss и rejection

При `drop newest` caller получает ошибку и ещё может retry. При `drop oldest`
успешный `Add` одновременно означает потерю ранее принятого item. Эти outcomes
нужно отражать разными метриками и API-контрактом.

### 5. Не учитывать shutdown

Consumer, ожидающий пустую очередь, должен проснуться после `Close`; buffered
items при этом обычно сначала дренируются. Producer после `Close` получает
`ErrClosed`, а не panic.

### 6. Strict priority без fairness

Постоянный high traffic вызывает starvation lower priorities. Weighted scheduling
и aging должны быть частью контракта, а не случайным поведением `select`.

### 7. Global occupancy вместо per-queue saturation

Пустой high reserve может скрыть полностью заполненную normal queue. Проверяют и
общую нагрузку, и конкретный partition/priority/tenant.

### 8. Sampling без metadata

Без effective sample rate downstream не сможет восстановить оценки. Для adaptive
sampling вероятность может меняться во времени, поэтому её записывают рядом с
sample.

### 9. Retry storm после reject

Если все producers сразу повторяют запрос, overload усиливается. Нужны bounded
retries, exponential backoff с jitter и серверный `Retry-After`, где protocol это
поддерживает.

### 10. Нет overload-метрик

Минимум нужны queue depth/capacity, queueing time, accepted/rejected/dropped по
причине и priority, processing latency и service rate. Только occupancy без
скорости роста backlog запаздывает.

---

## Возможные расширения

- Per-tenant queues с fair scheduling и отдельными quotas.
- Coalescing по business key: хранить только последнее состояние устройства,
  вместо drop произвольного события.
- Spillover в durable queue с отдельным replay rate limit.
- Feedback controller, который меняет admission rate по latency/error budget, с
  защитой от oscillation.
- Circuit breaker перед заведомо failing downstream, чтобы не накапливать работу,
  которую сейчас невозможно выполнить.

---

## Interview-ready answer

**1. Что делать, если producer быстрее consumer?**

- Диагностика — понять, burst это или постоянный overload.
- Policy — выбрать block, reject/drop, sample, durable spill или scale.
- Защита — ограничить память и наблюдать queueing delay/service rate.

**2. Когда использовать block, а когда drop?**

- Block — контролируемый upstream и запрет потерь, но с context/deadline.
- Drop — только когда потеря входит в контракт; newest/oldest выбирают по
  ценности свежести.
- Durable queue — если успешный приём обязан пережить crash.

**3. Как корректно реализовать `drop oldest`?**

- Атомарность — eviction и insertion входят в одну critical section.
- Структура — ring buffer под mutex проще двух channel operations.
- Метрики — нужны отдельные counters для accepted и evicted.

**4. Как оценить buffer?**

- Burst — `(arrival - service) × duration`.
- Latency — сверить `depth/service rate` с допустимым queueing delay.
- Память — bytes per item и runtime overhead измерить, а sustained overload устранять не
  увеличением очереди.

---

## Связанные материалы

- [Pipeline](../concurrency/04-pipeline.md)
- [Reliability: backpressure and shedding](../../../05-system-design/reliability-patterns/05-backpressure-and-shedding.md)
- [Worker Pool](../concurrency/01-worker-pool.md)
- [Pub/Sub In-Memory](../concurrency/05-pubsub.md)
