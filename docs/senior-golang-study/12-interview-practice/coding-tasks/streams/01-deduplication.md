# Задача 1: Stream Deduplication

Дедупликация сообщений в потоке. **Не путать с singleflight** (концурентный дедуп). Здесь — поток событий идёт во времени, нужно отбрасывать **повторы**, видя дубль через секунды или минуты.

## Формулировка

> "Дан stream сообщений с уникальным ID. Поток может содержать дубли (например, at-least-once delivery из Kafka). Реализуй фильтр: пропускать только **первое** появление каждого ID, дубли отбрасывать."

Use cases:
- Kafka consumer с at-least-once → exactly-once семантика
- Webhook retry deduplication
- Idempotent event processing
- Log deduplication

---

## Уточняющие вопросы

1. **Окно дедупа — какое?**
   "Forever (нужна persistent storage) или sliding (in-memory)? 1 минута / 1 час / 24 часа?"

2. **Точная дедупликация или approximate?**
   "Точно — map. Approximate — bloom filter (память константа)."

3. **At-most-once или at-least-once для downstream?**
   "Если мы fail после dedup, но до commit — пропустим event (at-most-once). Нужно ack'ать."

4. **Persistent или volatile?**
   "Restart pod → начинаем с пустой dedup state → дубли проходят. Нужен Redis/DB?"

5. **Throughput?**
   "10/sec — простой map. 100k/sec — нужны оптимизации."

6. **Cardinality дубликатов — high или low?**
   "Если 99% уникальных — bloom filter эффективный. Если 50% дубли — отдельные расчёты."

---

## Решение 1: Sliding window map

Простейший вариант — `map[ID]time.Time`, периодический cleanup.

```go
package streamdedup

import (
    "sync"
    "time"
)

type Deduper struct {
    mu     sync.Mutex
    seen   map[string]time.Time
    window time.Duration
}

func New(window time.Duration) *Deduper {
    d := &Deduper{
        seen:   make(map[string]time.Time),
        window: window,
    }
    go d.cleanupLoop()
    return d
}

// Allow возвращает true если ID видится впервые в окне.
func (d *Deduper) Allow(id string) bool {
    d.mu.Lock()
    defer d.mu.Unlock()

    now := time.Now()
    if firstSeen, ok := d.seen[id]; ok && now.Sub(firstSeen) < d.window {
        return false  // дубль
    }
    d.seen[id] = now
    return true
}

func (d *Deduper) cleanupLoop() {
    ticker := time.NewTicker(d.window / 4)
    defer ticker.Stop()

    for range ticker.C {
        d.mu.Lock()
        cutoff := time.Now().Add(-d.window)
        for id, seen := range d.seen {
            if seen.Before(cutoff) {
                delete(d.seen, id)
            }
        }
        d.mu.Unlock()
    }
}
```

**Использование:**

```go
dedup := streamdedup.New(time.Hour)

for msg := range stream {
    if !dedup.Allow(msg.ID) {
        continue  // дубль
    }
    process(msg)
}
```

**Trade-offs:**
- ✅ Простой, точный
- ✅ TTL-based cleanup
- ❌ Memory O(unique IDs in window) — миллионы IDs × 50 байт = 50 MB на 1M IDs
- ❌ Global mutex — contention под нагрузкой

---

## Решение 2: Sharded для high throughput

При 100k+ msg/sec — global mutex bottleneck. Sharded map:

```go
package streamdedup

import (
    "hash/fnv"
    "sync"
    "time"
)

const shardCount = 64

type shard struct {
    mu   sync.Mutex
    seen map[string]time.Time
}

type ShardedDeduper struct {
    shards [shardCount]*shard
    window time.Duration
}

func NewSharded(window time.Duration) *ShardedDeduper {
    d := &ShardedDeduper{window: window}
    for i := range d.shards {
        d.shards[i] = &shard{seen: make(map[string]time.Time)}
    }
    go d.cleanupLoop()
    return d
}

func (d *ShardedDeduper) shardFor(id string) *shard {
    h := fnv.New64a()
    h.Write([]byte(id))
    return d.shards[h.Sum64()%shardCount]
}

func (d *ShardedDeduper) Allow(id string) bool {
    s := d.shardFor(id)
    s.mu.Lock()
    defer s.mu.Unlock()

    now := time.Now()
    if firstSeen, ok := s.seen[id]; ok && now.Sub(firstSeen) < d.window {
        return false
    }
    s.seen[id] = now
    return true
}

func (d *ShardedDeduper) cleanupLoop() {
    ticker := time.NewTicker(d.window / 4)
    defer ticker.Stop()

    for range ticker.C {
        cutoff := time.Now().Add(-d.window)
        for _, s := range d.shards {
            s.mu.Lock()
            for id, seen := range s.seen {
                if seen.Before(cutoff) {
                    delete(s.seen, id)
                }
            }
            s.mu.Unlock()
        }
    }
}
```

**Преимущества:** 64 независимых mutex'ов → contention падает в 64 раза.

---

## Решение 3: Bloom filter (approximate)

Когда memory критична — bloom filter (см. [data-structures/03-bloom-filter.md](../data-structures/03-bloom-filter.md)).

```go
import "github.com/bits-and-blooms/bloom/v3"

type BloomDeduper struct {
    filters [2]*bloom.BloomFilter  // current + previous (rotating)
    currentIdx int
    mu         sync.Mutex
    capacity   uint
    fpr        float64
    rotateAt   time.Time
    rotateInterval time.Duration
}

func NewBloomDeduper(capacity uint, fpr float64, rotateInterval time.Duration) *BloomDeduper {
    d := &BloomDeduper{
        capacity:       capacity,
        fpr:            fpr,
        rotateInterval: rotateInterval,
        rotateAt:       time.Now().Add(rotateInterval),
    }
    d.filters[0] = bloom.NewWithEstimates(capacity, fpr)
    d.filters[1] = bloom.NewWithEstimates(capacity, fpr)
    return d
}

func (d *BloomDeduper) Allow(id string) bool {
    d.mu.Lock()
    defer d.mu.Unlock()

    // Rotate filters если окно истекло
    if time.Now().After(d.rotateAt) {
        d.currentIdx = (d.currentIdx + 1) % 2
        d.filters[d.currentIdx].ClearAll()
        d.rotateAt = time.Now().Add(d.rotateInterval)
    }

    data := []byte(id)
    // Проверить в обоих filter'ах (current + previous = sliding window)
    if d.filters[0].Test(data) || d.filters[1].Test(data) {
        return false  // возможный дубль (false positive ≤ fpr)
    }

    d.filters[d.currentIdx].Add(data)
    return true
}
```

**Trade-offs vs map:**
- ✅ Constant memory — 10M IDs в 1.2 MB @ 1% FPR
- ✅ Fast lookup — O(K hash) ~50 нс
- ❌ False positives — изредка пропускаем уникальный (loss)
- ❌ Не удалить конкретный ID
- ❌ "Two-filter rotation" approximate sliding window

Подходит для: log dedup где можно терять 1% событий, security event dedup.
Не подходит для: платежей, biling — где терять events недопустимо.

---

## Решение 4: Persistent (Redis SET)

Если pod restart не должен сбросить state:

```go
type RedisDeduper struct {
    client *redis.Client
    ttl    time.Duration
}

func (d *RedisDeduper) Allow(ctx context.Context, id string) (bool, error) {
    // SET key value NX EX ttl — атомарно установит если key not exists
    ok, err := d.client.SetNX(ctx, "dedup:"+id, "1", d.ttl).Result()
    if err != nil {
        return false, err
    }
    return ok, nil  // true если был добавлен (= новый), false если уже был (= дубль)
}
```

**Trade-offs:**
- ✅ Persistent через restart
- ✅ Shared между pod'ами (несколько consumers)
- ❌ Network round-trip ~1ms per message
- ❌ Cost — Redis нужно поддерживать
- ❌ Если Redis down — что делать? (degrade — пропускать всё / fail-close — отказывать)

---

## At-least-once → exactly-once в Kafka

Реальный сценарий: Kafka даёт at-least-once, мы хотим exactly-once для downstream effect (DB write, email).

```go
type KafkaConsumer struct {
    consumer kafka.Consumer
    dedup    *Deduper
    db       *sql.DB
}

func (c *KafkaConsumer) Consume(ctx context.Context) error {
    for {
        msg, err := c.consumer.Read(ctx)
        if err != nil {
            return err
        }

        // Извлечь idempotency key (часто = message ID или business key)
        eventID := msg.Headers.Get("event-id")
        if eventID == "" {
            // No idempotency — пропустить или log
            continue
        }

        // Dedup
        if !c.dedup.Allow(eventID) {
            // Already processed — просто commit offset, не делать work
            c.consumer.Commit(msg)
            continue
        }

        // Process с idempotency на уровне БД (additional defense)
        err = c.db.Exec(`
            INSERT INTO processed_events (event_id, ...)
            VALUES ($1, ...)
            ON CONFLICT (event_id) DO NOTHING
        `, eventID, ...)
        if err != nil {
            return err
        }

        c.consumer.Commit(msg)
    }
}
```

**Defense in depth:**
1. **In-memory dedup** — fast filter ~99% дублей
2. **BD UNIQUE constraint** — last line, гарантирует exactly-once на уровне DB

См. также [05-idempotency-handler.md](../system-primitives/05-idempotency-handler.md) и [09-saga-and-outbox.md](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md).

---

## Тесты

```go
func TestDeduper_FirstSeen(t *testing.T) {
    d := New(time.Minute)

    if !d.Allow("id-1") {
        t.Error("first should pass")
    }
    if d.Allow("id-1") {
        t.Error("duplicate should not pass")
    }
}

func TestDeduper_WindowExpiry(t *testing.T) {
    d := New(50 * time.Millisecond)

    d.Allow("id-1")

    time.Sleep(100 * time.Millisecond)

    if !d.Allow("id-1") {
        t.Error("should pass after window expiry")
    }
}

func TestDeduper_Concurrent(t *testing.T) {
    d := New(time.Minute)

    var passed atomic.Int32
    var wg sync.WaitGroup

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                if d.Allow(fmt.Sprintf("id-%d", j%10)) {
                    passed.Add(1)
                }
            }
        }()
    }
    wg.Wait()

    // Только 10 уникальных → 10 passes
    if passed.Load() != 10 {
        t.Errorf("passed %d, want 10", passed.Load())
    }
}

func TestDeduper_NoLeakAfterCleanup(t *testing.T) {
    d := New(50 * time.Millisecond)

    for i := 0; i < 10000; i++ {
        d.Allow(fmt.Sprintf("id-%d", i))
    }

    // Подождать cleanup'а
    time.Sleep(200 * time.Millisecond)

    d.mu.Lock()
    count := len(d.seen)
    d.mu.Unlock()

    if count > 100 {
        t.Errorf("seen size %d after cleanup, expected near 0", count)
    }
}
```

---

## Подводные камни

### 1. Memory leak без cleanup

```go
seen := make(map[string]bool)
seen[id] = true  // никогда не удаляется
```

Map растёт бесконечно. Через сутки — OOM.

### 2. Cleanup goroutine блокирует Allow

```go
d.mu.Lock()  // в cleanup
for id := range d.seen { ... }  // длинная операция
// Allow'ы блокируются всё это время
```

Решения:
- Sharding (cleanup один shard за раз)
- Lazy cleanup (на Allow, если случайно)
- Snapshot + swap

### 3. Restart pod = потеря dedup state

```
Restart → empty map → дубли пройдут как новые
```

Для critical workloads — Redis/DB. Для logs — OK.

### 4. Wide-window with high cardinality

```go
New(24 * time.Hour)  // окно 24 часа
// 1M new IDs per hour = 24M IDs in map = ~2.4 GB
```

Mind capacity planning.

### 5. False positives в bloom filter

```go
bloom.NewWithEstimates(1_000_000, 0.01)
// 1% уникальных будут потеряны как "дубли"
```

Для метрик — OK. Для платежей — НЕТ.

### 6. Race condition при чтении и обновлении

```go
// ❌ Two-step без atomic
if _, ok := d.seen[id]; !ok {
    d.seen[id] = now  // ← другой поток может вставить между этими
    return true
}
return false
```

Под одним lock'ом — OK. Без lock — race.

### 7. Late arrivals

```
Event timestamp: 10:00:00
Arrived: 10:05:00 (5 minutes late)

Dedup window = 1 minute → event считается "новым"
Но он уже обрабатывался 5 минут назад!
```

Window должно быть **> max expected late arrival**.

### 8. Dedup key выбран плохо

```go
// ❌ Dedup by timestamp
if !d.Allow(fmt.Sprintf("%d", event.Time.UnixMicro())) {
    // Каждое событие уникально по времени — dedup бесполезен
}

// ✓ Dedup by business key
if !d.Allow(event.PaymentID) {
    // Дублирующие payment events отброшены
}
```

### 9. Atomic check + side effect

```go
if !d.Allow(id) { return }
processInDB(id)  // ← если упало здесь, потеряли event навсегда
                  // (в map уже зарегистрирован)
```

Решение: register **после** успешной обработки. Или DB-level idempotency.

### 10. Cleanup interval vs window

```go
window := time.Hour
go d.cleanupLoop(window)  // ← cleanup раз в час?

// За 1 час map может вырасти существенно перед cleanup
```

Cleanup interval должен быть **доли** window — типично `window/4`.

---

## Возможные расширения

### 1. Multi-level dedup

L1: in-memory bloom (fast) → L2: Redis (persistent) → L3: DB UNIQUE.

Каждый level отлавливает большинство, последний — обязательная гарантия.

### 2. Probabilistic counter (HyperLogLog)

Не "видел ли я этот ID", а "сколько было уникальных". Меньше памяти. Cardinality estimation.

### 3. Time-windowed Top-K dedup

Не "уникальный или нет", а "часто или редко". Дедуп только spam'а (одинаковое сообщение N раз подряд).

### 4. Late event handling

Watermark — после "10:00:00 + 5 минут" считать события до 10:00:00 закрытыми. Поздно пришедшие — отбрасывать или в отдельную queue.

### 5. Compact storage через trie

Если IDs имеют общий prefix (UUID v7 с timestamp) — trie вместо map экономит память.

---

## Что важно показать на собеседовании

1. **Stream dedup ≠ singleflight** — разные паттерны
2. **Trade-off map vs bloom** — точность vs память
3. **Sharding для high throughput** — global mutex bottleneck
4. **Persistent vs volatile** — критичность через restart
5. **Defense in depth** — in-memory dedup + DB UNIQUE
6. **Window > max late arrival**
7. **Cleanup interval = window/4** rule of thumb
8. **At-least-once → exactly-once** — типичный сценарий из Kafka

## Связки

- [Bloom Filter](../data-structures/03-bloom-filter.md) — basis для approximate dedup
- [Sliding Window Counter](../data-structures/05-sliding-window-counter.md) — родственная структура
- [Idempotency Handler](../system-primitives/05-idempotency-handler.md) — request-level dedup
- [Kafka](../../../07-message-brokers-and-streaming/01-kafka.md) — at-least-once semantics
