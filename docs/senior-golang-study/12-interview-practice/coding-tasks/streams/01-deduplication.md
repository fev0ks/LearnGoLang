# Задача 1: Stream Deduplication

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Точный TTL-deduper](#решение-1-точный-ttl-deduper-на-map)
- [Sharding](#решение-2-sharding-для-высокой-конкурентности)
- [Rotating Bloom filter](#решение-3-rotating-bloom-filter-approximate)
- [Redis](#решение-4-persistent-state-в-redis)
- [Effectively-once в БД](#effectively-once-эффект-в-бд)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Возможные расширения](#возможные-расширения)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Stream deduplication подавляет повторную обработку событий с одинаковым
идентификатором в течение заданного времени. Это не `singleflight`:
`singleflight` объединяет только одновременно выполняющиеся вызовы, а deduper
помнит уже завершённые события.

---

## Формулировка

> "Дан stream сообщений с уникальным ID. Поток может содержать дубли (например, at-least-once delivery из Kafka). Реализуй фильтр: пропускать только **первое** появление каждого ID, дубли отбрасывать."

Где встречается:
- Kafka consumer с at-least-once delivery → идемпотентный DB-effect
- Webhook retry deduplication
- Idempotent event processing
- Log deduplication

---

## Уточняющие вопросы

1. **Окно дедупа — какое?**
   "Forever (нужна persistent storage) или sliding (in-memory)? 1 минута / 1 час / 24 часа?"

2. **Точная дедупликация или approximate?**
   "Точно — `map`/Redis/БД. Approximate — Bloom filter с ограниченной памятью."

3. **Как дедуп связан с side effect и acknowledgement?**
   "Если отметить ID до DB commit, crash может потерять эффект. Если подтвердить
   offset до commit — сообщение тоже потеряется. Нужна одна транзакционная граница
   либо outbox/idempotency на внешней стороне."

4. **Persistent или volatile?**
   "Restart pod → начинаем с пустой dedup state → дубли проходят. Нужен Redis/DB?"

5. **Throughput?**
   "Выбор делают по benchmark: один mutex может быть достаточен, а при измеренном
   contention состояние shard'ят или переносят в partition-local consumer."

6. **Cardinality дубликатов — high или low?**
   "Если 99% уникальных — bloom filter эффективный. Если 50% дубли — отдельные расчёты."

---

## Решение 1: точный TTL-deduper на `map`

`map[ID]time.Time` хранит момент первого принятого события. Повтор подавляется в
течение `window` после этого момента; повтор сам окно не продлевает.

```go
package streamdedup

import (
    "errors"
    "sync"
    "time"
)

var ErrInvalidConfig = errors.New("deduper: invalid configuration")

type Deduper struct {
    mu     sync.Mutex
    seen   map[string]time.Time
    window time.Duration
    now    func() time.Time

    stop      chan struct{}
    done      chan struct{}
    closeOnce sync.Once
}

func New(window time.Duration) (*Deduper, error) {
    return newDeduper(window, time.Now)
}

func newDeduper(window time.Duration, now func() time.Time) (*Deduper, error) {
    if window <= 0 || now == nil {
        return nil, ErrInvalidConfig
    }
    d := &Deduper{
        seen:   make(map[string]time.Time),
        window: window,
        now:    now,
        stop:   make(chan struct{}),
        done:   make(chan struct{}),
    }
    go d.cleanupLoop()
    return d, nil
}

// Allow возвращает true если ID видится впервые в окне.
func (d *Deduper) Allow(id string) bool {
    d.mu.Lock()
    defer d.mu.Unlock()

    now := d.now()
    if firstSeen, ok := d.seen[id]; ok && now.Sub(firstSeen) < d.window {
        return false  // дубль
    }
    d.seen[id] = now
    return true
}

func (d *Deduper) cleanupLoop() {
    cleanupEvery := d.window / 4
    if cleanupEvery <= 0 {
        cleanupEvery = d.window
    }
    ticker := time.NewTicker(cleanupEvery)
    defer ticker.Stop()
    defer close(d.done)

    for {
        select {
        case <-d.stop:
            return
        case <-ticker.C:
            d.purge(d.now())
        }
    }
}

func (d *Deduper) purge(now time.Time) {
    d.mu.Lock()
    defer d.mu.Unlock()

    cutoff := now.Add(-d.window)
    for id, seen := range d.seen {
        if !seen.After(cutoff) {
            delete(d.seen, id)
        }
    }
}

// Close останавливает служебную goroutine. После Close deduper не используют.
func (d *Deduper) Close() {
    d.closeOnce.Do(func() { close(d.stop) })
    <-d.done
}
```

**Использование:**

```go
dedup, err := streamdedup.New(time.Hour)
if err != nil {
    return err
}
defer dedup.Close()

for msg := range stream {
    if !dedup.Allow(msg.ID) {
        continue  // дубль
    }
    process(msg)
}
```

**Trade-offs:**
- Память — `O(unique IDs in window)`, а не `O(history)`.
- Точность — false positive и false negative отсутствуют внутри TTL-контракта.
- Цена — один mutex сериализует `Allow`, а периодический полный scan создаёт
  latency spike.
- Capacity — размер записи зависит от длины ID, layout `map` и аллокатора. Его
  измеряют benchmark/heap profile, а не считают фиксированными «50 байтами».

---

## Решение 2: sharding для высокой конкурентности

Если profile показывает contention на общем mutex, состояние можно разделить на
независимые shards:

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
    stop   chan struct{}
    done   chan struct{}

    closeOnce sync.Once
}

func NewSharded(window time.Duration) (*ShardedDeduper, error) {
    if window <= 0 {
        return nil, ErrInvalidConfig
    }
    d := &ShardedDeduper{
        window: window,
        stop:   make(chan struct{}),
        done:   make(chan struct{}),
    }
    for i := range d.shards {
        d.shards[i] = &shard{seen: make(map[string]time.Time)}
    }
    go d.cleanupLoop()
    return d, nil
}

func (d *ShardedDeduper) shardFor(id string) *shard {
    h := fnv.New64a()
    _, _ = h.Write([]byte(id)) // hash.Hash.Write по контракту не возвращает error
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
    cleanupEvery := d.window / 4
    if cleanupEvery <= 0 {
        cleanupEvery = d.window
    }
    ticker := time.NewTicker(cleanupEvery)
    defer ticker.Stop()
    defer close(d.done)

    for {
        select {
        case <-d.stop:
            return
        case now := <-ticker.C:
            cutoff := now.Add(-d.window)
            for _, s := range d.shards {
                s.mu.Lock()
                for id, seen := range s.seen {
                    if !seen.After(cutoff) {
                        delete(s.seen, id)
                    }
                }
                s.mu.Unlock()
            }
        }
    }
}

func (d *ShardedDeduper) Close() {
    d.closeOnce.Do(func() { close(d.stop) })
    <-d.done
}
```

64 mutex уменьшают contention только при достаточно равномерном распределении
ключей. Hot key всё равно сериализуется внутри одного shard, а cleanup по-прежнему
сканирует все записи.

---

## Решение 3: rotating Bloom filter (approximate)

Когда memory критична — bloom filter (см. [data-structures/03-bloom-filter.md](../data-structures/03-bloom-filter.md)).

```go
import (
    "errors"
    "sync"
    "time"

    "github.com/bits-and-blooms/bloom/v3"
)

type BloomDeduper struct {
    filters        [2]*bloom.BloomFilter // current + previous
    currentIdx     int
    mu             sync.Mutex
    capacity       uint
    fpr            float64
    rotateAt       time.Time
    rotateInterval time.Duration
}

func NewBloomDeduper(capacity uint, fpr float64, rotateInterval time.Duration) (*BloomDeduper, error) {
    if capacity == 0 || fpr <= 0 || fpr >= 1 || rotateInterval <= 0 {
        return nil, errors.New("bloom deduper: invalid capacity, fpr, or interval")
    }
    d := &BloomDeduper{
        capacity:       capacity,
        fpr:            fpr,
        rotateInterval: rotateInterval,
        rotateAt:       time.Now().Add(rotateInterval),
    }
    d.filters[0] = bloom.NewWithEstimates(capacity, fpr)
    d.filters[1] = bloom.NewWithEstimates(capacity, fpr)
    return d, nil
}

func (d *BloomDeduper) Allow(id string) bool {
    d.mu.Lock()
    defer d.mu.Unlock()

    now := time.Now()
    if !now.Before(d.rotateAt) {
        if !now.Before(d.rotateAt.Add(d.rotateInterval)) {
            // Пропущено минимум два интервала: старых данных быть не должно.
            d.filters[0].ClearAll()
            d.filters[1].ClearAll()
            d.currentIdx = 0
        } else {
            d.currentIdx = (d.currentIdx + 1) % 2
            d.filters[d.currentIdx].ClearAll()
        }
        d.rotateAt = now.Add(d.rotateInterval)
    }

    data := []byte(id)
    // Проверить current и previous interval.
    if d.filters[0].Test(data) || d.filters[1].Test(data) {
        return false // дубль либо false positive
    }

    d.filters[d.currentIdx].Add(data)
    return true
}
```

**Trade-offs относительно `map`:**

- Память — фиксируется при создании и не зависит от фактического числа записей,
  но заданная capacity должна покрывать число уникальных IDs в одном
  `rotateInterval`; при переполнении фактический FPR растёт.
- Ошибка — false positive подавляет уникальное событие; false negative появляется
  на границе retention, когда старый filter очищается.
- Окно — запись хранится от одного до двух `rotateInterval` в зависимости от
  момента попадания, поэтому это не точное sliding window.
- Удаление — конкретный ID удалить нельзя; очищается целый filter.

Для `n = 10_000_000` и `p = 1%` один Bloom filter требует примерно
`-n·ln(p)/(ln 2)² ≈ 95.9 млн бит ≈ 12 MB`, а два — около `24 MB` без учёта
служебных данных. Проверка двух filters увеличивает совокупный FPR примерно до
`1 - (1-p)²`; для общего бюджета 1% каждый filter настраивают примерно на 0.5%.

Такой вариант подходит только там, где редкая потеря уникального события входит
в контракт, например для необязательной телеметрии. Для платежей, аудита и
security-событий false positive обычно неприемлем.

---

## Решение 4: persistent state в Redis

Если pod restart не должен сбросить state:

```go
type RedisDeduper struct {
    client *redis.Client
    ttl    time.Duration
}

func NewRedisDeduper(client *redis.Client, ttl time.Duration) (*RedisDeduper, error) {
    if client == nil || ttl <= 0 {
        return nil, ErrInvalidConfig
    }
    return &RedisDeduper{client: client, ttl: ttl}, nil
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

- Scope — состояние общее для нескольких pods, поэтому один ID атомарно выигрывает
  только один `SET NX`.
- Retention — ключ исчезает по TTL; «навсегда» этот вариант не дедуплицирует.
- Durability — переживание сбоя Redis зависит от AOF/RDB, репликации и выбранного
  режима failover, а не следует из самого `SET NX`.
- Latency — на каждое событие добавляется сетевой вызов; конкретное время нужно
  измерять в своей топологии.
- Failure policy — при недоступности Redis нужно явно выбрать fail-open
  (возможны дубли) или fail-closed (останавливается обработка).

Если ID содержит персональные данные или имеет неограниченную длину, в Redis key
кладут namespaced hash, а исходный ID сохраняют в payload/БД при необходимости.

---

## Effectively-once эффект в БД

At-least-once consumer может повторно получить сообщение после сбоя. Нельзя
сначала пометить ID в памяти, а затем менять БД: сбой между шагами оставит метку,
и retry ошибочно отбросит ещё не применённое событие.

Для эффекта внутри одной БД используют transactional inbox: регистрация
`event_id` и business update входят в одну транзакцию.

```go
type KafkaConsumer struct {
    consumer OffsetCommitter
    db       *sql.DB
}

// Типы адаптера не привязаны к конкретной Go-библиотеке Kafka.
type Message struct {
    EventID   string
    Payload   []byte
    Topic     string
    Partition int
    Offset    int64
}

type OffsetCommitter interface {
    Commit(ctx context.Context, msg Message) error
}

func (c *KafkaConsumer) Handle(ctx context.Context, msg Message) error {
    eventID := msg.EventID
    if eventID == "" {
        return errors.New("missing event-id")
    }

    tx, err := c.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    result, err := tx.ExecContext(ctx, `
        INSERT INTO processed_events (event_id, processed_at)
        VALUES ($1, now())
        ON CONFLICT (event_id) DO NOTHING
    `, eventID)
    if err != nil {
        return err
    }
    inserted, err := result.RowsAffected()
    if err != nil {
        return err
    }

    if inserted == 1 {
        if err := applyBusinessChange(ctx, tx, msg); err != nil {
            return err
        }
        // Для email/API здесь записывают outbox row, а не вызывают внешний
        // сервис внутри транзакции.
    }

    if err := tx.Commit(); err != nil {
        return err
    }
    return c.consumer.Commit(ctx, msg)
}
```

Если процесс падает после `tx.Commit`, но до commit offset, Kafka доставит
сообщение повторно. `UNIQUE(event_id)` превратит повтор в no-op, после чего offset
можно подтвердить ещё раз. Это даёт effectively-once для изменений в этой БД.

Такой constraint корректен, только если `event_id` глобально уникален в области
consumer. Иначе ключ делают составным, например `(source, event_id)`, где
`source` — стабильная часть event protocol, а не случайный process identifier.

Для внешнего email/API одной DB-транзакции недостаточно: outbox фиксируется
вместе с business update, а relay доставляет его идемпотентно. Локальный точный
cache разрешено использовать только как оптимизацию для уже committed IDs; он
не заменяет durable constraint. Bloom filter нельзя ставить на пути, где false
positive приводит к пропуску business effect.

См. также [05-idempotency-handler.md](../system-primitives/05-idempotency-handler.md) и [09-saga-and-outbox.md](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md).

---

## Тесты

```go
func TestDeduper_FirstSeen(t *testing.T) {
    now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    d, err := newDeduper(time.Minute, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    if !d.Allow("id-1") {
        t.Error("first should pass")
    }
    if d.Allow("id-1") {
        t.Error("duplicate should not pass")
    }
}

func TestDeduper_WindowExpiry(t *testing.T) {
    now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    d, err := newDeduper(time.Minute, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    d.Allow("id-1")
    now = now.Add(time.Minute)

    if !d.Allow("id-1") {
        t.Error("should pass after window expiry")
    }
}

func TestDeduper_Concurrent(t *testing.T) {
    fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    d, err := newDeduper(time.Hour, func() time.Time { return fixed })
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

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

func TestDeduper_PurgeExpired(t *testing.T) {
    now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    d, err := newDeduper(time.Minute, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    for i := 0; i < 10000; i++ {
        d.Allow(fmt.Sprintf("id-%d", i))
    }

    now = now.Add(time.Minute)
    d.purge(now)

    d.mu.Lock()
    count := len(d.seen)
    d.mu.Unlock()

    if count != 0 {
        t.Errorf("seen size %d after cleanup, want 0", count)
    }
}
```

---

## Подводные камни

### 1. Утечка памяти без cleanup

```go
seen := make(map[string]bool)
seen[id] = true  // никогда не удаляется
```

`map` растёт без ограничения и со временем приводит к OOM.

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

### 3. Перезапуск pod сбрасывает dedup state

```
restart → empty map → дубли пройдут как новые
```

Для критичных данных нужен durable state в Redis/БД. Для некритичных логов
потеря dedup state при restart может входить в контракт.

### 4. Wide-window with high cardinality

```go
New(24 * time.Hour)  // окно 24 часа
// 1M новых ID/час × 24 часа = 24M записей.
// При допущении 100 bytes/entry это около 2.4 GB.
```

`100 bytes/entry` — только допущение: реальный размер измеряют на своих ID и
версии Go через heap profile.

### 5. False positives в bloom filter

```go
bloom.NewWithEstimates(1_000_000, 0.01)
// После заполнения расчётной capacity вероятность false positive около 1%
// на одну membership-проверку; это не гарантия "ровно 1% потерь".
```

Для необязательной телеметрии это может быть допустимо. Для платежей — нет.

### 6. Race condition при чтении и обновлении

```go
// ❌ Two-step без atomic
if _, ok := d.seen[id]; !ok {
    d.seen[id] = now  // ← другой поток может вставить между этими
    return true
}
return false
```

Под одним mutex операция корректна. Без синхронизации возникает race.

### 7. Late arrivals

```
Event timestamp: 10:00:00
Arrived: 10:05:00 (5 minutes late)

Dedup window = 1 minute → event считается "новым"
Но он уже обрабатывался 5 минут назад!
```

TTL должен покрывать максимальный ожидаемый интервал между первой доставкой и
повтором. Event time и processing time — разные часы; один timestamp события не
решает задачу retention.

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

Простое «зарегистрировать после обработки» тоже недостаточно: два concurrent
дубля успеют выполнить side effect. Для DB-эффекта регистрация ID и изменение
данных входят в одну транзакцию; для внешнего эффекта используют outbox и
идемпотентность получателя.

### 10. Cleanup interval vs window

```go
window := time.Hour
go d.cleanupLoop(window)  // ← cleanup раз в час?

// За 1 час map может вырасти существенно перед cleanup
```

Частый cleanup уменьшает retained garbage, но чаще сканирует map. Интервал
выбирают из скорости уникальных ID, допустимой лишней памяти и latency scan;
`window/4` в примере — стартовая настройка, а не универсальное правило.

---

## Возможные расширения

### 1. Multi-level dedup

L1: точный cache уже committed IDs → L2: Redis → L3: DB `UNIQUE`.

Последний уровень остаётся источником гарантии. Bloom filter можно использовать
как подсказку, но не как основание пропустить критический side effect.

### 2. Probabilistic counter (HyperLogLog)

Не "видел ли я этот ID", а "сколько было уникальных". Меньше памяти. Cardinality estimation.

### 3. Time-windowed Top-K dedup

Не "уникальный или нет", а "часто или редко". Дедуп только spam'а (одинаковое сообщение N раз подряд).

### 4. Late event handling

Watermark — после "10:00:00 + 5 минут" считать события до 10:00:00 закрытыми. Поздно пришедшие — отбрасывать или в отдельную queue.

### 5. Partitioned TTL buckets

ID группируют по временному bucket и удаляют bucket целиком. Это уменьшает цену
cleanup по сравнению с полным scan, но делает границу окна грубее.

---

## Interview-ready answer

**1. Чем stream deduplication отличается от singleflight?**

- Stream deduplication — хранит уже обработанные ID в течение retention window.
- Singleflight — объединяет только одновременно выполняющиеся вызовы и ничего не
  помнит после их завершения.

**2. Как выбрать между `map`, Redis и Bloom filter?**

- `map` — точный локальный TTL-dedup с памятью `O(unique IDs in window)`.
- Redis — общий atomic state между процессами, но с сетевой ценой и отдельной
  failure policy.
- Bloom filter — bounded memory ценой false positives; для критических эффектов
  он не является источником истины.

**3. Почему `Allow(id)` перед side effect опасен?**

- Окно сбоя — процесс может записать ID и упасть до эффекта, после чего retry
  будет ошибочно отброшен.
- Решение — inbox ID и DB-effect фиксируются одной транзакцией; внешний эффект
  проходит через outbox/idempotent receiver.

**4. От чего зависит размер состояния?**

- Оценка — `unique IDs/sec × retention seconds × measured bytes/entry`.
- Cleanup — частота scan выбирается по допустимой лишней памяти и latency, а
  sharding помогает только при равномерном распределении ключей.

---

## Связанные материалы

- [Bloom Filter](../data-structures/03-bloom-filter.md) — основа approximate dedup
- [Sliding Window Counter](../data-structures/05-sliding-window-counter.md) — родственная структура
- [Idempotency Handler](../system-primitives/05-idempotency-handler.md) — request-level dedup
- [Kafka](../../../07-message-brokers-and-streaming/01-kafka.md) — at-least-once semantics
- [Redis `SET` options](https://redis.io/docs/latest/commands/set/)
- [Bloom filter Go package](https://pkg.go.dev/github.com/bits-and-blooms/bloom/v3)
