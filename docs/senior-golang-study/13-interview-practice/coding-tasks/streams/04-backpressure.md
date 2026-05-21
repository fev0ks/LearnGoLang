# Задача 4: Backpressure Handling

Что делать когда **producer быстрее consumer'а**? Это фундаментальный вопрос всех streaming систем. Без правильного backpressure → OOM, dropped events, cascade failure.

## Формулировка

> "Producer генерирует события быстрее, чем consumer может обработать. Реализуй компонент-mediator, который безопасно обрабатывает overflow."

Use cases:
- Slow downstream (БД, API) — нельзя терять events
- Bursty traffic — peak в 10x от стабильного
- Multi-tenant — один tenant заваливает других
- Real-time analytics — events приходят с разной скоростью

---

## Уточняющие вопросы

1. **Можно ли терять события?**
   "Metrics — OK. Платежи — нет."

2. **Priority — все события equal или разные?**
   "Если разные — high-priority должны проходить first."

3. **Backpressure до producer'а?**
   "Можем ли мы замедлить producer'а или он external?"

4. **Persistent или volatile buffer?**
   "Restart → потеря — OK?"

5. **Throughput?**
   "1k/sec — простой буфер. 1M/sec — нужны sharding и lock-free."

6. **SLA — какой max latency приемлем?**
   "Чем больше buffer — тем больше latency."

---

## Стратегии (подробно)

### 1. Block producer (backpressure propagation)

Producer ждёт когда буфер освободится.

```go
ch := make(chan Event, 100)
ch <- event  // блокирует если буфер полон
```

**Плюсы:**
- ✅ Не теряем события
- ✅ Естественная backpressure до источника

**Минусы:**
- ❌ Producer может deadlock'нуть caller'а
- ❌ Один slow consumer тормозит весь pipeline
- ❌ Source может не справляться с backpressure (например, network packet drops)

**Когда:** internal pipelines, producer контролируем.

### 2. Drop newest (reject)

При full буфере — отказывать новым.

```go
select {
case ch <- event:
default:
    droppedNewest.Add(1)
}
```

**Плюсы:**
- ✅ Не блокирует producer'а
- ✅ Defends consumer от backlog

**Минусы:**
- ❌ Теряем **свежие** события (часто более важные)

**Когда:** real-time metrics where "first observation" важнее последующих.

### 3. Drop oldest

Удаляем старейшее, оставляем новое.

```go
select {
case ch <- event:
default:
    <-ch       // drop oldest
    ch <- event  // try again
}
```

**Плюсы:**
- ✅ Сохраняем недавнее (актуальное)

**Минусы:**
- ❌ Теряем старое
- ❌ Race condition (другой может проснуться между <-ch и ch<-)

**Когда:** sensor data, real-time updates где новое value заменяет старое.

### 4. Random drop

Drop какой-то random event при overflow.

**Плюсы:**
- ✅ Распределённая loss — каждый класс events теряет одинаково
- ✅ Хорошо для sampling (statistical correctness)

**Минусы:**
- ❌ Менее предсказуемо

### 5. Sampling

Брать только каждое N-е событие, остальные drop.

```go
if atomic.AddInt64(&counter, 1) % sampleRate != 0 {
    return
}
ch <- event
```

**Плюсы:**
- ✅ Throughput не растёт неограниченно
- ✅ Statistical sampling корректный

**Минусы:**
- ❌ Теряем 99% событий — для metrics OK, для events critical — нет

**Когда:** distributed tracing (sample only 1%).

### 6. Reservoir Sampling

Сохранить N **random samples** из stream. Если приходит больше — replace с probability `N/total`.

```go
type Reservoir struct {
    items []Event
    n     int
    count int  // total seen
}

func (r *Reservoir) Add(item Event) {
    r.count++
    if len(r.items) < r.n {
        r.items = append(r.items, item)
        return
    }
    j := rand.Intn(r.count)
    if j < r.n {
        r.items[j] = item
    }
}
```

**Плюсы:**
- ✅ Constant memory
- ✅ Uniform random sample (mathematical guarantee)

**Когда:** post-hoc analysis, не need real-time processing.

### 7. Adaptive load shedding

Динамически меняем стратегию по нагрузке:
- Buffer < 50% → process all
- 50-90% → drop low-priority
- > 90% → drop all but critical

Используется Netflix Hystrix, Linkerd.

### 8. Push back to producer with rate limit

Producer'у возвращаем сигнал "медленнее":
```go
if isOverloaded() {
    rateLimiter.SetLimit(rateLimiter.Limit() * 0.9)  // reduce 10%
}
```

Используется gRPC backpressure.

---

## Базовое решение: Bounded buffer + policy

```go
package backpressure

import (
    "context"
    "errors"
    "sync"
    "sync/atomic"
)

type Policy int

const (
    PolicyBlock Policy = iota
    PolicyDropNewest
    PolicyDropOldest
)

var ErrBufferFull = errors.New("buffer full")

type Buffer[T any] struct {
    ch     chan T
    policy Policy

    // Metrics
    accepted atomic.Int64
    dropped  atomic.Int64
}

func New[T any](capacity int, policy Policy) *Buffer[T] {
    return &Buffer[T]{
        ch:     make(chan T, capacity),
        policy: policy,
    }
}

// Add — non-blocking (для DropNewest/DropOldest) или blocking (для Block).
func (b *Buffer[T]) Add(ctx context.Context, item T) error {
    switch b.policy {
    case PolicyBlock:
        select {
        case b.ch <- item:
            b.accepted.Add(1)
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }

    case PolicyDropNewest:
        select {
        case b.ch <- item:
            b.accepted.Add(1)
            return nil
        default:
            b.dropped.Add(1)
            return ErrBufferFull
        }

    case PolicyDropOldest:
        for {
            select {
            case b.ch <- item:
                b.accepted.Add(1)
                return nil
            default:
                // Drop oldest, try again
                select {
                case <-b.ch:
                    b.dropped.Add(1)
                default:
                    // Race — someone took oldest. Retry.
                }
            }
        }
    }
    return nil
}

// Receive возвращает channel для consumer'а.
func (b *Buffer[T]) Receive() <-chan T {
    return b.ch
}

type Stats struct {
    Accepted int64
    Dropped  int64
    Buffered int
}

func (b *Buffer[T]) Stats() Stats {
    return Stats{
        Accepted: b.accepted.Load(),
        Dropped:  b.dropped.Load(),
        Buffered: len(b.ch),
    }
}
```

**Использование:**

```go
buf := backpressure.New[Event](1000, backpressure.PolicyDropOldest)

// Producer
go func() {
    for ev := range producerStream {
        buf.Add(ctx, ev)
    }
}()

// Consumer
for ev := range buf.Receive() {
    process(ev)
}
```

---

## Production-grade: Multi-level с priority

Real-world systems часто комбинируют стратегии:

```go
package backpressure

type Priority int

const (
    PriorityLow Priority = iota
    PriorityNormal
    PriorityHigh
)

type PrioritizedItem[T any] struct {
    Priority Priority
    Item     T
}

type PriorityBuffer[T any] struct {
    cfg    Config

    // 3 channel'а — по приоритетам
    highCh   chan T
    normalCh chan T
    lowCh    chan T

    // Backpressure thresholds
    mu          sync.Mutex
    pressure    Pressure

    // Metrics
    accepted [3]atomic.Int64  // [low, normal, high]
    dropped  [3]atomic.Int64
}

type Pressure int

const (
    PressureLow Pressure = iota  // < 50%
    PressureMed                   // 50-90%
    PressureHigh                  // > 90%
)

type Config struct {
    HighCapacity   int
    NormalCapacity int
    LowCapacity    int
}

func NewPrioritized[T any](cfg Config) *PriorityBuffer[T] {
    return &PriorityBuffer[T]{
        cfg:      cfg,
        highCh:   make(chan T, cfg.HighCapacity),
        normalCh: make(chan T, cfg.NormalCapacity),
        lowCh:    make(chan T, cfg.LowCapacity),
    }
}

func (b *PriorityBuffer[T]) Add(ctx context.Context, p Priority, item T) error {
    var ch chan T
    var capacity int

    switch p {
    case PriorityHigh:
        ch = b.highCh
        capacity = b.cfg.HighCapacity
    case PriorityNormal:
        ch = b.normalCh
        capacity = b.cfg.NormalCapacity
    case PriorityLow:
        ch = b.lowCh
        capacity = b.cfg.LowCapacity
    }

    // Compute pressure
    pressure := b.computePressure()

    // Adaptive policy based on pressure
    switch pressure {
    case PressureLow:
        // Accept всё — block если full
        select {
        case ch <- item:
            b.accepted[p].Add(1)
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }

    case PressureMed:
        // Drop low-priority при full, остальные block
        if p == PriorityLow {
            select {
            case ch <- item:
                b.accepted[p].Add(1)
                return nil
            default:
                b.dropped[p].Add(1)
                return ErrBufferFull
            }
        }
        select {
        case ch <- item:
            b.accepted[p].Add(1)
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }

    case PressureHigh:
        // Drop low + normal, only high blocks
        if p != PriorityHigh {
            select {
            case ch <- item:
                b.accepted[p].Add(1)
                return nil
            default:
                b.dropped[p].Add(1)
                return ErrBufferFull
            }
        }
        // High priority — block even на high pressure
        select {
        case ch <- item:
            b.accepted[p].Add(1)
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    _ = capacity  // unused warning
    return nil
}

func (b *PriorityBuffer[T]) computePressure() Pressure {
    total := len(b.highCh) + len(b.normalCh) + len(b.lowCh)
    capacity := b.cfg.HighCapacity + b.cfg.NormalCapacity + b.cfg.LowCapacity
    ratio := float64(total) / float64(capacity)

    if ratio < 0.5 {
        return PressureLow
    }
    if ratio < 0.9 {
        return PressureMed
    }
    return PressureHigh
}

// Receive возвращает items по приоритету — high first.
func (b *PriorityBuffer[T]) Receive(ctx context.Context) (T, bool) {
    var zero T

    // High priority — попробовать non-blocking first
    select {
    case item := <-b.highCh:
        return item, true
    default:
    }

    select {
    case item := <-b.highCh:
        return item, true
    case item := <-b.normalCh:
        return item, true
    case item := <-b.lowCh:
        return item, true
    case <-ctx.Done():
        return zero, false
    }
}
```

**Использование:**

```go
buf := backpressure.NewPrioritized[Event](Config{
    HighCapacity:   100,
    NormalCapacity: 1000,
    LowCapacity:    10000,
})

// Producer
buf.Add(ctx, PriorityHigh, payment_event)
buf.Add(ctx, PriorityLow, metrics_event)

// Consumer
for {
    ev, ok := buf.Receive(ctx)
    if !ok {
        return
    }
    process(ev)
}
```

**Что важно:**
- **3 channel'а** для priorities
- **`computePressure`** — какой режим сейчас
- **Adaptive policy** — при low всё OK, при high — только critical events
- **Receive с priority** — high checked first

---

## Тесты

```go
func TestBuffer_DropNewest(t *testing.T) {
    b := New[int](2, PolicyDropNewest)

    b.Add(context.Background(), 1)
    b.Add(context.Background(), 2)
    err := b.Add(context.Background(), 3)  // full

    if err != ErrBufferFull {
        t.Errorf("expected ErrBufferFull, got %v", err)
    }

    // First two should still be there
    if v := <-b.Receive(); v != 1 {
        t.Errorf("got %d, want 1", v)
    }
    if v := <-b.Receive(); v != 2 {
        t.Errorf("got %d, want 2", v)
    }
}

func TestBuffer_DropOldest(t *testing.T) {
    b := New[int](2, PolicyDropOldest)

    b.Add(context.Background(), 1)
    b.Add(context.Background(), 2)
    b.Add(context.Background(), 3)  // drops 1

    if v := <-b.Receive(); v != 2 {
        t.Errorf("expected 2 (oldest dropped), got %d", v)
    }
    if v := <-b.Receive(); v != 3 {
        t.Errorf("got %d, want 3", v)
    }
}

func TestBuffer_Block(t *testing.T) {
    b := New[int](1, PolicyBlock)

    b.Add(context.Background(), 1)

    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    start := time.Now()
    err := b.Add(ctx, 2)
    elapsed := time.Since(start)

    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected DeadlineExceeded, got %v", err)
    }
    if elapsed < 40*time.Millisecond {
        t.Errorf("Add returned too fast: %v", elapsed)
    }
}

func TestPriorityBuffer_HighFirst(t *testing.T) {
    b := NewPrioritized[string](Config{
        HighCapacity: 10,
        NormalCapacity: 10,
        LowCapacity: 10,
    })

    b.Add(context.Background(), PriorityLow, "low-1")
    b.Add(context.Background(), PriorityHigh, "high-1")
    b.Add(context.Background(), PriorityNormal, "normal-1")

    ctx := context.Background()
    if v, _ := b.Receive(ctx); v != "high-1" {
        t.Errorf("first should be high, got %s", v)
    }
}
```

---

## Подводные камни

### 1. Drop oldest race

```go
default:
    <-ch       // одна горутина drop'нула
    ch <- item // другая горутина уже добавила
}
```

Между `<-ch` и `ch <- item` может пройти время → может быть race. Решение: lock или accept что race возможен.

### 2. Buffer слишком большой

```go
make(chan T, 1_000_000)
```

OOM. Buffer должен быть **bounded по реальным потребностям**. 1k-10k обычно.

### 3. Buffer слишком маленький

```go
make(chan T, 1)  // unbuffered-like
```

Любой micro-burst → drops. Buffer = ~2-10x от RPS × max acceptable latency.

### 4. Backpressure cascade up

Block в local buffer → upstream block → его upstream block → cascade to user → timeout user'у.

Это **естественно**, но нужно понимать что блокировка propagate'ит.

### 5. Memory leak через goroutines

```go
for ev := range producerStream {
    go func() {
        buf.Add(ctx, ev)  // ← если block, goroutine утечёт
    }()
}
```

Не делай spawn горутин для add — это invalidates backpressure целиком. Используй direct call.

### 6. Slow consumer = backed up everything

```go
for ev := range buf.Receive() {
    time.Sleep(time.Second)  // ← consumer медленный
    process(ev)
}
```

Buffer fills up immediately. Стратегия должна знать "consumer медленный" → drop low-priority.

### 7. Priority starvation

```go
// Receive — high first, иногда normal/low не доходят
```

Если high всегда есть — normal и low никогда не обрабатываются. **Fair scheduling**: round-robin между priorities с weights.

### 8. Pressure based на len(ch) — точка во времени

```go
ratio := len(ch) / cap(ch)  // snapshot
```

Между computePressure и Add channel может опустошиться → mismatch. Acceptable для adaptive — не need to be perfect.

### 9. Drop в metrics counter без context

```go
dropped.Add(1)
// Что именно drop'нули — info lost
```

Лог first/last drop с context для debug.

### 10. Не учитывать ctx в Add

```go
ch <- item  // блокирует forever
```

С context — caller'а может отменить:
```go
select {
case ch <- item:
case <-ctx.Done():
    return ctx.Err()
}
```

---

## Возможные расширения

### 1. Multi-tier buffer

L1 (in-memory) → L2 (Redis) → L3 (Kafka). При overflow перетекает дальше.

### 2. Persistent backpressure

При overflow — события записываются на disk вместо drop. Resume после восстановления.

### 3. Token bucket для producer

Producer ограничивается через rate.Limiter — не позволять fill'ить buffer быстрее consumer'а.

### 4. Reactive feedback

Consumer публикует "current rate" через метрику → producer auto-throttle через token bucket с этим rate.

### 5. Push-pull conversion

Если producer не controllable (push) — конвертировать в pull (consumer dictates rate).

---

## Что важно показать на собеседовании

1. **Знать все стратегии**: block, drop newest/oldest, sampling, reservoir
2. **Trade-offs каждой** — когда что подходит
3. **Priority + adaptive** — нетривиальный case
4. **Estuary эффект** — backpressure propagate'ит upstream
5. **Buffer sizing** — не too big (OOM), не too small (drops)
6. **Metrics для observability** — accepted, dropped, buffered, pressure
7. **Context propagation** — context cancellation
8. **Reservoir sampling** для analytics — math correct

## Связки

- [Pipeline](../concurrency/04-pipeline.md) — backpressure в stages
- [Reliability: backpressure-and-shedding](../../../05-system-design/reliability-patterns/05-backpressure-and-shedding.md)
- [Worker Pool](../concurrency/01-worker-pool.md) — bounded parallelism
- [Pub/Sub In-Memory](../concurrency/05-pubsub.md) — slow subscriber handling
