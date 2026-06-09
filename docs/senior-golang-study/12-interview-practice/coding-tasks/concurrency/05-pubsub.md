# Задача 5: Pub/Sub In-Memory

In-memory publish-subscribe — простая, но богатая на детали задача. Тестирует понимание channels, locks, goroutine lifecycle, slow consumers.

## Формулировка

> "Реализуй in-memory Pub/Sub: Publisher отправляет события в topic, Subscriber'ы получают копии."

Вариации:
- "Event bus в приложении"
- "Notification dispatcher"
- "WebSocket connection manager"

---

## Уточняющие вопросы

1. **Один topic или много?**
   "Один — проще. Много — нужен `topic → subscribers` map."

2. **Slow consumer — что делать?**
   - Drop сообщения для медленного
   - Disconnect медленного
   - Buffer per-subscriber с лимитом
   - Backpressure на publisher'а (плохо — один тормозит всех)

3. **Сообщения сохраняются или fire-and-forget?**
   "Сохраняются — это уже message broker (replay, persistence). Здесь — fire-and-forget."

4. **Wildcard topics? (`order.*`)**
   "Усложняет implementation. Уточни нужно ли."

5. **Возможность unsubscribe?**
   "Обязательно — иначе goroutine leak когда subscriber больше не нужен."

6. **Ordering — strict per-subscriber или нет?**
   "Per-subscriber strict — да, естественно. Между subscriber'ами — нет, async."

---

## Базовое решение

Простейший вариант с одним topic и list subscriber'ов:

```go
package pubsub

import "sync"

type Subscriber[T any] chan T

type PubSub[T any] struct {
    mu          sync.RWMutex
    subscribers []Subscriber[T]
    bufferSize  int
}

func New[T any](bufferSize int) *PubSub[T] {
    return &PubSub[T]{bufferSize: bufferSize}
}

// Subscribe возвращает channel для получения событий.
// Caller должен вызвать Unsubscribe.
func (ps *PubSub[T]) Subscribe() Subscriber[T] {
    ch := make(Subscriber[T], ps.bufferSize)
    ps.mu.Lock()
    ps.subscribers = append(ps.subscribers, ch)
    ps.mu.Unlock()
    return ch
}

func (ps *PubSub[T]) Unsubscribe(ch Subscriber[T]) {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    for i, sub := range ps.subscribers {
        if sub == ch {
            ps.subscribers = append(ps.subscribers[:i], ps.subscribers[i+1:]...)
            close(ch)
            return
        }
    }
}

// Publish отправляет сообщение всем subscriber'ам.
// Slow subscribers получают drop (non-blocking send).
func (ps *PubSub[T]) Publish(msg T) {
    ps.mu.RLock()
    defer ps.mu.RUnlock()

    for _, sub := range ps.subscribers {
        select {
        case sub <- msg:
        default:
            // Slow subscriber — drop сообщение
        }
    }
}

// Close закрывает все subscriber channels.
func (ps *PubSub[T]) Close() {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    for _, sub := range ps.subscribers {
        close(sub)
    }
    ps.subscribers = nil
}
```

**Использование:**

```go
ps := pubsub.New[OrderEvent](100)
defer ps.Close()

// Подписчик 1: считает заказы
sub1 := ps.Subscribe()
go func() {
    var count int
    for evt := range sub1 {
        count++
        log.Printf("subscriber 1: order %s, total %d", evt.OrderID, count)
    }
}()

// Подписчик 2: отправляет email
sub2 := ps.Subscribe()
go func() {
    for evt := range sub2 {
        sendEmail(evt)
    }
}()

// Publisher
ps.Publish(OrderEvent{OrderID: "123"})
ps.Publish(OrderEvent{OrderID: "456"})

// Отписать sub1
ps.Unsubscribe(sub1)
```

**Ключевые моменты:**
- **`sync.RWMutex`** — Publish использует RLock (читает list), Subscribe/Unsubscribe — Lock (изменяет)
- **`bufferSize`** на каждого subscriber'а — небольшой буфер для сглаживания
- **`select { default }`** — non-blocking publish, slow subscriber теряет сообщения
- **Close subscriber's channel при Unsubscribe** — signal каноничный

---

## Production-grade с topics, slow consumer handling и metrics

```go
package pubsub

import (
    "context"
    "errors"
    "sync"
    "sync/atomic"
    "time"
)

var (
    ErrTopicNotFound      = errors.New("topic not found")
    ErrSubscriptionClosed = errors.New("subscription closed")
)

type Message[T any] struct {
    Topic string
    Data  T
    Time  time.Time
}

// Subscription представляет одного subscriber'а.
type Subscription[T any] struct {
    id      uint64
    topic   string
    ch      chan Message[T]
    closed  atomic.Bool
    dropped atomic.Int64 // счётчик потерянных сообщений
    bus     *PubSub[T]
}

func (s *Subscription[T]) C() <-chan Message[T] {
    return s.ch
}

// Dropped возвращает сколько сообщений было drop'нуто (slow consumer).
func (s *Subscription[T]) Dropped() int64 {
    return s.dropped.Load()
}

func (s *Subscription[T]) Unsubscribe() {
    s.bus.unsubscribe(s)
}

type PubSub[T any] struct {
    mu       sync.RWMutex
    topics   map[string]map[uint64]*Subscription[T]  // topic → id → subscription
    nextID   atomic.Uint64
    buffer   int

    // Метрики
    published atomic.Int64
    delivered atomic.Int64
    dropped   atomic.Int64
}

func New[T any](bufferSize int) *PubSub[T] {
    return &PubSub[T]{
        topics: make(map[string]map[uint64]*Subscription[T]),
        buffer: bufferSize,
    }
}

func (ps *PubSub[T]) Subscribe(topic string) *Subscription[T] {
    sub := &Subscription[T]{
        id:    ps.nextID.Add(1),
        topic: topic,
        ch:    make(chan Message[T], ps.buffer),
        bus:   ps,
    }

    ps.mu.Lock()
    if _, ok := ps.topics[topic]; !ok {
        ps.topics[topic] = make(map[uint64]*Subscription[T])
    }
    ps.topics[topic][sub.id] = sub
    ps.mu.Unlock()

    return sub
}

func (ps *PubSub[T]) unsubscribe(sub *Subscription[T]) {
    if !sub.closed.CompareAndSwap(false, true) {
        return  // уже отписан
    }

    ps.mu.Lock()
    if subs, ok := ps.topics[sub.topic]; ok {
        delete(subs, sub.id)
        if len(subs) == 0 {
            delete(ps.topics, sub.topic)
        }
    }
    ps.mu.Unlock()

    close(sub.ch)
}

// Publish отправляет сообщение всем subscriber'ам topic'а.
// Slow subscribers получают drop (counted via Dropped()).
func (ps *PubSub[T]) Publish(topic string, data T) {
    ps.published.Add(1)

    msg := Message[T]{
        Topic: topic,
        Data:  data,
        Time:  time.Now(),
    }

    ps.mu.RLock()
    subs := ps.topics[topic]
    // Копируем список чтобы не держать lock на send
    subList := make([]*Subscription[T], 0, len(subs))
    for _, s := range subs {
        subList = append(subList, s)
    }
    ps.mu.RUnlock()

    for _, s := range subList {
        select {
        case s.ch <- msg:
            ps.delivered.Add(1)
        default:
            // Slow subscriber — drop
            s.dropped.Add(1)
            ps.dropped.Add(1)
        }
    }
}

// PublishBlocking — то же, но блокируется если slow consumer (опционально).
func (ps *PubSub[T]) PublishBlocking(ctx context.Context, topic string, data T) error {
    msg := Message[T]{Topic: topic, Data: data, Time: time.Now()}

    ps.mu.RLock()
    subList := make([]*Subscription[T], 0, len(ps.topics[topic]))
    for _, s := range ps.topics[topic] {
        subList = append(subList, s)
    }
    ps.mu.RUnlock()

    for _, s := range subList {
        select {
        case s.ch <- msg:
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    return nil
}

// Stats возвращает метрики.
type Stats struct {
    Topics      int
    Subscribers int
    Published   int64
    Delivered   int64
    Dropped     int64
}

func (ps *PubSub[T]) Stats() Stats {
    ps.mu.RLock()
    defer ps.mu.RUnlock()

    totalSubs := 0
    for _, subs := range ps.topics {
        totalSubs += len(subs)
    }

    return Stats{
        Topics:      len(ps.topics),
        Subscribers: totalSubs,
        Published:   ps.published.Load(),
        Delivered:   ps.delivered.Load(),
        Dropped:     ps.dropped.Load(),
    }
}

func (ps *PubSub[T]) Close() {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    for _, subs := range ps.topics {
        for _, s := range subs {
            if s.closed.CompareAndSwap(false, true) {
                close(s.ch)
            }
        }
    }
    ps.topics = make(map[string]map[uint64]*Subscription[T])
}
```

**Использование:**

```go
ps := pubsub.New[OrderEvent](100)
defer ps.Close()

// Подписаться на topic
sub := ps.Subscribe("orders")
defer sub.Unsubscribe()

go func() {
    for msg := range sub.C() {
        process(msg.Data)
    }
}()

// Опубликовать
ps.Publish("orders", OrderEvent{...})

// Метрики
stats := ps.Stats()
log.Printf("dropped: %d", stats.Dropped)
```

**Что улучшено по сравнению с базовым:**
- **Topics** — несколько тем, не один список
- **`Subscription` struct** с метаданными — нет нужды искать по `chan` для unsubscribe
- **`atomic.Bool` для closed flag** — idempotent unsubscribe (можно вызвать несколько раз)
- **Per-subscriber drop counter** — диагностика slow consumer'ов
- **Copy list under lock, send без lock** — Publish не блокирует Subscribe
- **`PublishBlocking`** — для случаев когда нельзя терять сообщения

---

## Slow consumer — стратегии

### 1. Drop (по умолчанию)

```go
select {
case sub.ch <- msg:
default:
    droppedCount++
}
```

**Когда:** metrics, telemetry, real-time updates (старое не важно).

### 2. Block publisher

```go
sub.ch <- msg  // ждём пока консьюмер прочитает
```

**Проблема:** один slow consumer тормозит всех. Не используй в shared pub/sub.

### 3. Disconnect slow consumer

```go
select {
case sub.ch <- msg:
case <-time.After(100 * time.Millisecond):
    // Slow consumer — отключаем
    sub.Unsubscribe()
}
```

**Когда:** WebSocket клиенты — отключенный лучше чем заблокированный.

### 4. Per-subscriber unbounded queue

Каждый subscriber имеет linked list, расти бесконечно.

**Проблема:** memory leak если consumer навсегда медленный.

### 5. Per-subscriber circular buffer

Хранить N последних сообщений, старые перетираются.

**Когда:** "show me last N events" use case.

---

## Тесты

```go
func TestPubSub_Basic(t *testing.T) {
    ps := New[string](10)
    defer ps.Close()

    sub := ps.Subscribe("topic1")
    defer sub.Unsubscribe()

    ps.Publish("topic1", "hello")

    select {
    case msg := <-sub.C():
        if msg.Data != "hello" {
            t.Errorf("got %q, want %q", msg.Data, "hello")
        }
    case <-time.After(time.Second):
        t.Fatal("no message received")
    }
}

func TestPubSub_MultipleSubscribers(t *testing.T) {
    ps := New[int](10)
    defer ps.Close()

    sub1 := ps.Subscribe("nums")
    sub2 := ps.Subscribe("nums")
    defer sub1.Unsubscribe()
    defer sub2.Unsubscribe()

    ps.Publish("nums", 42)

    for _, sub := range []*Subscription[int]{sub1, sub2} {
        select {
        case msg := <-sub.C():
            if msg.Data != 42 {
                t.Errorf("got %d, want 42", msg.Data)
            }
        case <-time.After(time.Second):
            t.Fatal("subscriber didn't receive")
        }
    }
}

func TestPubSub_DifferentTopics(t *testing.T) {
    ps := New[string](10)
    defer ps.Close()

    sub1 := ps.Subscribe("topic1")
    sub2 := ps.Subscribe("topic2")
    defer sub1.Unsubscribe()
    defer sub2.Unsubscribe()

    ps.Publish("topic1", "for-topic1")

    select {
    case msg := <-sub1.C():
        if msg.Data != "for-topic1" {
            t.Errorf("got %q", msg.Data)
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("sub1 didn't receive")
    }

    select {
    case msg := <-sub2.C():
        t.Fatalf("sub2 shouldn't receive: %v", msg)
    case <-time.After(50 * time.Millisecond):
        // Expected
    }
}

func TestPubSub_SlowConsumerDrops(t *testing.T) {
    ps := New[int](2)  // small buffer
    defer ps.Close()

    sub := ps.Subscribe("nums")
    defer sub.Unsubscribe()

    // Publish 10 без чтения
    for i := 0; i < 10; i++ {
        ps.Publish("nums", i)
    }

    if sub.Dropped() == 0 {
        t.Error("expected some drops, got 0")
    }
}

func TestPubSub_ConcurrentPublishSubscribe(t *testing.T) {
    ps := New[int](100)
    defer ps.Close()

    var receivedCount atomic.Int32
    var wg sync.WaitGroup

    // 10 subscribers
    for i := 0; i < 10; i++ {
        sub := ps.Subscribe("topic")
        wg.Add(1)
        go func() {
            defer wg.Done()
            defer sub.Unsubscribe()
            for range sub.C() {
                receivedCount.Add(1)
            }
        }()
    }

    // 5 publishers
    var pubWg sync.WaitGroup
    for i := 0; i < 5; i++ {
        pubWg.Add(1)
        go func() {
            defer pubWg.Done()
            for j := 0; j < 100; j++ {
                ps.Publish("topic", j)
            }
        }()
    }
    pubWg.Wait()

    // Дать subscriber'ам время прочитать
    time.Sleep(100 * time.Millisecond)
    ps.Close()
    wg.Wait()

    // 5 * 100 publishes * 10 subscribers = 5000 delivers (если без drop'ов)
    // С buffer 100 на subscriber — drop'ы возможны но мало
    if receivedCount.Load() < 4000 {
        t.Errorf("received only %d, expected ~5000", receivedCount.Load())
    }
}

func TestPubSub_UnsubscribeStops(t *testing.T) {
    ps := New[int](10)
    defer ps.Close()

    sub := ps.Subscribe("topic")

    done := make(chan bool)
    go func() {
        for range sub.C() {
        }
        done <- true
    }()

    ps.Publish("topic", 1)
    sub.Unsubscribe()

    select {
    case <-done:
        // Good — channel closed
    case <-time.After(time.Second):
        t.Fatal("subscriber goroutine didn't exit")
    }
}
```

---

## Подводные камни

### 1. Goroutine leak при unsubscribe

```go
sub := ps.Subscribe("topic")
go func() {
    for msg := range sub.C() {
        process(msg)
    }
}()

// Где-то позже...
sub.Unsubscribe()
// Если не close(sub.ch), горутина зависнет навсегда на range
```

**Решение:** Unsubscribe всегда close channel. Subscriber должен видеть закрытие через `range` exit.

### 2. Двойной close (panic)

```go
ps.Close()
sub.Unsubscribe()  // ← close(sub.ch) второй раз → panic
```

**Решение:** atomic flag `closed`:
```go
if !sub.closed.CompareAndSwap(false, true) {
    return  // уже закрыт
}
close(sub.ch)
```

### 3. Lock contention под нагрузкой

```go
ps.mu.Lock()  // ← каждый Publish/Subscribe конкурирует
```

При тысячах публикаций в секунду и многих subscribers — bottleneck. Решения:
- **RWMutex** — несколько Publish'ов могут идти параллельно (RLock)
- **Sharded мapping** — N топиков на N маленьких pubsub instance'ов
- **Lock-free через atomic.Value** для immutable subscriber lists

### 4. Send под lock

```go
ps.mu.RLock()
for _, sub := range ps.topics[topic] {
    sub.ch <- msg  // ← блок если slow consumer
}
ps.mu.RUnlock()
```

Если subscriber медленный → publisher держит lock → другие Publish/Subscribe блокируются. **Скопируй список под lock, отправляй вне lock.**

### 5. Map итерация под Lock

```go
ps.mu.Lock()
for _, sub := range ps.topics[topic] {
    // Если внутри изменяем topics → undefined behavior
}
```

Не модифицируй map во время range. Скопируй keys, потом обрабатывай.

### 6. Race condition при Subscribe + Publish

```go
// Publisher: получил empty list (не было subscriber'ов)
// В тот же момент Subscribe добавил
// Race решается через mutex, но subscriber пропустит publish который был до его регистрации
```

Это **expected** — pub/sub не гарантирует delivery до subscribe. Если нужно — нужен replay (broker, не in-memory pubsub).

### 7. Subscribe slow path

Если Subscribe возвращает channel и сразу же Publisher шлёт — first message может быть lost если subscriber ещё не успел запустить goroutine для чтения.

В тестах это видно — нужен sleep or sync mechanism.

### 8. Buffer size choice

```go
ch := make(chan T, 1000000)  // ← memory eating
ch := make(chan T, 0)         // ← блокирует publisher
ch := make(chan T, 10)        // ← OK для большинства случаев
```

Default 10-100 для in-memory pub/sub. Подбирай эмпирически.

### 9. Type assertion в любых направлениях

```go
// ❌ без generics
type PubSub struct {
    subs []chan interface{}
}
// Caller: msg.(MyType)  ← runtime cost + ошибки
```

Generics (Go 1.18+) делают type-safe.

### 10. Нет dead letter queue

Drop'нутые сообщения исчезают. Если важно — лог в файл, метрику, или DLQ.

---

## Возможные расширения

### 1. Wildcard topics (`order.*`)

```go
sub := ps.Subscribe("orders.*")
// Receives: "orders.created", "orders.updated", "orders.deleted"
```

Реализация: при Publish — итерируешь по subscriber'ам с patterns, проверяешь match (через path.Match или custom).

### 2. Replay для new subscriber'ов

```go
sub := ps.Subscribe("orders")
// Сразу получает last N сообщений
```

Хранить ring buffer N сообщений per topic, при Subscribe — flush из buffer.

### 3. Persistent pubsub (broker)

Это уже Kafka, RabbitMQ, Redis Streams. См. [07-message-brokers-and-streaming/](../../../07-message-brokers-and-streaming/).

### 4. Distributed pubsub

Несколько pod'ов — нужен external broker (Redis Pub/Sub, NATS).

### 5. Filtered subscriptions

```go
sub := ps.SubscribeFiltered("orders", func(evt OrderEvent) bool {
    return evt.Status == "paid"
})
```

Predicate на стороне subscriber'а — но проще делать filter в goroutine консьюмере.

### 6. Priority

Высокоприоритетные сообщения доставляются first.

### 7. Backpressure to publisher

Если все subscribers медленные — publisher блокируется (вместо drop). Trade-off безопасности vs throughput.

### 8. Metrics через Prometheus

Counter published / delivered / dropped per topic.

---

## Что важно показать на собеседовании

1. **Closure semantics** — кто close channel, когда, как защита от double close (atomic flag)
2. **RWMutex для optimization** — больше Publisher'ов чем Subscribe'ов
3. **Slow consumer handling** — drop strategy объяснить
4. **Goroutine leak prevention** — Unsubscribe closes channel → consumer's range exits
5. **Lock-free hot path** — копируй subscriber list под lock, send вне
6. **Generics** — type-safe API
7. **Trade-offs** — drop vs block vs disconnect

## Связки

- [Channels и горутины](../../../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md)
- [Redis Pub/Sub](../../../07-message-brokers-and-streaming/04-redis-pubsub.md) — distributed alternative
- [Kafka](../../../07-message-brokers-and-streaming/01-kafka.md) — persistent event streaming
- [WebSocket](../../../08-networking-and-api/protocols/05-websocket.md) — типичный потребитель in-memory pubsub
- [SSE](../../../08-networking-and-api/protocols/12-sse-and-realtime.md) — тоже
