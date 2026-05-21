# Задача 5: Sliding Window Counter

Структура для подсчёта событий за **скользящее окно** времени: "сколько запросов за последние 60 секунд", "сколько ошибок за час". Используется для rate limiting, real-time аналитики, anomaly detection.

## Формулировка

> "Реализуй структуру: `Inc()` записывает событие, `Count()` возвращает количество событий за последние N секунд."

Вариации:
- "Rate limiter с sliding window"
- "Топ запросов за последние 5 минут"
- "Real-time metrics: requests per minute"

---

## Уточняющие вопросы

1. **Точное окно или approximate?**
   "Точно — нужно хранить timestamps всех. Approximate — bucket'ы по N секунд."

2. **Размер окна — фиксированный или dynamic?**
   "Обычно фиксированный, e.g., 60 секунд."

3. **Throughput — сколько событий в секунду?**
   "1k/s — log можно хранить. 1M/s — нужны bucket'ы или approximate."

4. **Concurrent — несколько writers?**
   "Almost always. Нужна синхронизация."

5. **Single key или per-key (как rate limiter)?**
   "Per-key — структура содержит map ключ → counter."

6. **Persistent или volatile?**
   "Stateful — Redis. Stateless — in-memory."

---

## Решение 1: Точное — log of timestamps

Простой, точный, O(N) memory.

```go
package slidingwindow

import (
    "sync"
    "time"
)

// ExactCounter хранит точные timestamp'ы всех событий.
// Memory: O(events in window). Подходит для малых rates.
type ExactCounter struct {
    mu       sync.Mutex
    window   time.Duration
    events   []time.Time
}

func NewExactCounter(window time.Duration) *ExactCounter {
    return &ExactCounter{window: window}
}

func (c *ExactCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.events = append(c.events, time.Now())
    c.evictExpired()
}

func (c *ExactCounter) Count() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.evictExpired()
    return len(c.events)
}

func (c *ExactCounter) evictExpired() {
    cutoff := time.Now().Add(-c.window)
    // Найти первый non-expired (binary search для скорости)
    i := 0
    for i < len(c.events) && c.events[i].Before(cutoff) {
        i++
    }
    c.events = c.events[i:]
}
```

**Использование:**

```go
c := NewExactCounter(time.Minute)

for i := 0; i < 100; i++ {
    c.Inc()
}

fmt.Println(c.Count())  // 100 (если меньше минуты прошло)
```

**Trade-offs:**
- ✅ Точный — никакой approximation
- ❌ Memory O(events in window) — для 1M req/min = 60M timestamps × 8 байт = 500 MB
- ❌ append → может cause copy → slow для high throughput

Подходит для low-rate scenarios (logs, audit events).

---

## Решение 2: Bucket-based (approximate, fast)

Разбиваем окно на N bucket'ов. Каждый bucket — count за свой период. Sliding = выбросить старые bucket'ы.

```
Window = 60s, buckets = 6 (each 10s):
[10s][10s][10s][10s][10s][10s]
 b0    b1   b2   b3   b4   b5

Now 50s. Buckets b0-b4 = "older 50s". При Inc — добавляем в b5.
Через 10s — drop b0, добавим новый b6.
```

```go
package slidingwindow

import (
    "sync"
    "sync/atomic"
    "time"
)

type bucket struct {
    timestamp int64        // unix timestamp начала bucket'а
    count     atomic.Int64
}

// BucketCounter — sliding window через ring buffer bucket'ов.
type BucketCounter struct {
    mu          sync.RWMutex
    window      time.Duration
    bucketSize  time.Duration
    bucketCount int
    buckets     []*bucket
}

func NewBucketCounter(window, bucketSize time.Duration) *BucketCounter {
    n := int(window / bucketSize)
    buckets := make([]*bucket, n)
    for i := range buckets {
        buckets[i] = &bucket{}
    }
    return &BucketCounter{
        window:      window,
        bucketSize:  bucketSize,
        bucketCount: n,
        buckets:     buckets,
    }
}

func (c *BucketCounter) Inc() {
    now := time.Now().UnixNano()
    bucketStart := now - (now % int64(c.bucketSize))

    c.mu.RLock()
    idx := (bucketStart / int64(c.bucketSize)) % int64(c.bucketCount)
    b := c.buckets[idx]
    c.mu.RUnlock()

    // Проверить bucket — если timestamp устарел, reset
    if b.timestamp != bucketStart {
        c.mu.Lock()
        if b.timestamp != bucketStart {
            b.timestamp = bucketStart
            b.count.Store(0)
        }
        c.mu.Unlock()
    }

    b.count.Add(1)
}

func (c *BucketCounter) Count() int64 {
    now := time.Now().UnixNano()
    cutoff := now - int64(c.window)

    c.mu.RLock()
    defer c.mu.RUnlock()

    var total int64
    for _, b := range c.buckets {
        // Bucket считается только если timestamp в окне
        if b.timestamp >= cutoff {
            total += b.count.Load()
        }
    }
    return total
}
```

**Использование:**

```go
c := NewBucketCounter(time.Minute, time.Second)  // 60 buckets of 1 second

for i := 0; i < 1000; i++ {
    c.Inc()
}

fmt.Println(c.Count())  // 1000
```

**Trade-offs:**
- ✅ Constant memory O(N buckets) — обычно 10-60
- ✅ Atomic operations — fast concurrent
- ❌ Approximate — bucket boundaries создают +/- bucketSize error
- ❌ Точность зависит от bucket size — мелкие более точные, но больше bucket'ов

**Tip:** для rate limiter оптимально 10 bucket'ов на окно. Точность ±10%, memory minimal.

---

## Решение 3: Sliding Window Counter (weighted approximation)

Гибрид: один counter per "current" + один per "previous" window. Вес `previous` зависит от позиции внутри текущего.

```
Now: 75 секунд в текущей минуте (start = 60).
previous window: 0-60s  (count_prev = 100)
current window:  60-?   (count_curr = 50)

Sliding count = count_curr + count_prev * (1 - 75/60) = 50 + 100 * 0.25 = 75
                                                    ^
                                                    fraction of prev still inside sliding window
```

```go
type SlidingWindowApprox struct {
    mu             sync.Mutex
    windowSize     time.Duration
    currentCount   int64
    previousCount  int64
    windowStart    time.Time
}

func NewSlidingWindowApprox(window time.Duration) *SlidingWindowApprox {
    return &SlidingWindowApprox{
        windowSize:  window,
        windowStart: time.Now().Truncate(window),
    }
}

func (s *SlidingWindowApprox) Inc() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.rotate()
    s.currentCount++
}

func (s *SlidingWindowApprox) Count() float64 {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.rotate()

    // Сколько прошло в текущем окне
    elapsed := time.Since(s.windowStart)
    weight := float64(s.windowSize-elapsed) / float64(s.windowSize)

    return float64(s.currentCount) + float64(s.previousCount)*weight
}

func (s *SlidingWindowApprox) rotate() {
    elapsed := time.Since(s.windowStart)
    if elapsed >= 2*s.windowSize {
        // Прошло больше двух окон — оба обнулить
        s.previousCount = 0
        s.currentCount = 0
        s.windowStart = time.Now().Truncate(s.windowSize)
    } else if elapsed >= s.windowSize {
        // Прошло одно окно — current стал previous
        s.previousCount = s.currentCount
        s.currentCount = 0
        s.windowStart = s.windowStart.Add(s.windowSize)
    }
}
```

**Trade-off vs bucket-based:**
- ✅ Constant memory — только 2 counter'а
- ✅ Очень быстро
- ❌ Менее точный — assume равномерное распределение в previous window
- ⚠️ Для большинства rate limit'ов — достаточно точный (±5%)

Используется в Cloudflare rate limiter, Stripe API rate limiting.

---

## Решение 4: Per-key sliding window (rate limit)

Для rate limiting per-user / per-IP — map ключей на counter'ы:

```go
type PerKeyCounter struct {
    mu        sync.Mutex
    window    time.Duration
    counters  map[string]*SlidingWindowApprox

    // Cleanup для idle keys
    lastSeen  map[string]time.Time
}

func NewPerKeyCounter(window time.Duration) *PerKeyCounter {
    c := &PerKeyCounter{
        window:   window,
        counters: make(map[string]*SlidingWindowApprox),
        lastSeen: make(map[string]time.Time),
    }
    go c.cleanup()
    return c
}

func (c *PerKeyCounter) Inc(key string) {
    c.mu.Lock()
    cnt, ok := c.counters[key]
    if !ok {
        cnt = NewSlidingWindowApprox(c.window)
        c.counters[key] = cnt
    }
    c.lastSeen[key] = time.Now()
    c.mu.Unlock()

    cnt.Inc()
}

func (c *PerKeyCounter) Count(key string) float64 {
    c.mu.Lock()
    cnt, ok := c.counters[key]
    c.mu.Unlock()
    if !ok {
        return 0
    }
    return cnt.Count()
}

func (c *PerKeyCounter) cleanup() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        c.mu.Lock()
        cutoff := time.Now().Add(-10 * c.window)
        for key, last := range c.lastSeen {
            if last.Before(cutoff) {
                delete(c.counters, key)
                delete(c.lastSeen, key)
            }
        }
        c.mu.Unlock()
    }
}
```

**Critical:** cleanup goroutine для idle keys — без него map растёт бесконечно.

---

## Тесты

```go
func TestExactCounter(t *testing.T) {
    c := NewExactCounter(100 * time.Millisecond)

    for i := 0; i < 10; i++ {
        c.Inc()
    }
    if c.Count() != 10 {
        t.Errorf("got %d, want 10", c.Count())
    }

    time.Sleep(150 * time.Millisecond)

    if c.Count() != 0 {
        t.Errorf("after expiry got %d, want 0", c.Count())
    }
}

func TestBucketCounter_Concurrent(t *testing.T) {
    c := NewBucketCounter(time.Second, 100*time.Millisecond)

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                c.Inc()
            }
        }()
    }
    wg.Wait()

    count := c.Count()
    if count != 10000 {
        t.Errorf("got %d, want 10000", count)
    }
}

func TestSlidingWindowApprox(t *testing.T) {
    c := NewSlidingWindowApprox(200 * time.Millisecond)

    for i := 0; i < 100; i++ {
        c.Inc()
    }

    // Сразу: ~100 (previous=0, current=100)
    if c.Count() < 95 {
        t.Errorf("initial count %f, expected ~100", c.Count())
    }

    // Через 200ms — previous стал 100, current=0
    time.Sleep(220 * time.Millisecond)
    count := c.Count()
    // weight = 200/200 - elapsed/200, для первых ms после rotate ~ 0.9
    if count < 80 || count > 100 {
        t.Errorf("after rotate count %f, expected 80-100", count)
    }

    // Через ещё 200ms — оба нули
    time.Sleep(220 * time.Millisecond)
    if c.Count() > 5 {
        t.Errorf("after both windows expired count %f", c.Count())
    }
}
```

---

## Подводные камни

### 1. Boundary problem в fixed window

```
window = 1 second, limit = 100

09:00:59.999 — 99 requests sent → OK
09:01:00.000 — counter reset
09:01:00.001 — 99 requests sent → OK
Total: 198 requests in 2 milliseconds
```

Это "Boundary problem" — fixed window не точен на границах. Sliding window (любой подход выше) решает это.

### 2. Memory growth in ExactCounter

Для high throughput списком timestamp'ов — OOM. Использовать bucket-based.

### 3. Slice mutation issue

```go
c.events = c.events[i:]  // ← underlying array всё ещё держит старые timestamps
```

Memory не освобождается. Periodic полная re-allocate:
```go
if cap(c.events) > 4*len(c.events) {
    new := make([]time.Time, len(c.events))
    copy(new, c.events)
    c.events = new
}
```

### 4. Race condition в bucket reset

Между чтением bucket'а и reset'ом может прийти Inc другой goroutine. Атомарно:
```go
// Atomic CAS for reset
oldTs := b.timestamp.Load()
if oldTs != bucketStart {
    if b.timestamp.CompareAndSwap(oldTs, bucketStart) {
        b.count.Store(0)
    }
}
b.count.Add(1)
```

### 5. Time skew

System clock can move backwards (NTP adjustment). Тогда old bucket может стать "current". Защита — monotonic clock через `time.Since(start)` или ignore mismatched timestamps.

### 6. Cleanup goroutine leak

```go
go c.cleanup()
// Не выйдет даже если counter уже не нужен
```

Pass context или Close method:
```go
func NewCounter(ctx context.Context, window time.Duration) *Counter {
    c := &Counter{...}
    go c.cleanup(ctx)
    return c
}

func (c *Counter) cleanup(ctx context.Context) {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.evictIdle()
        }
    }
}
```

### 7. Lock contention в hot key

10k QPS на одном key через global mutex — bottleneck.

Решения:
- Sharded counter (один counter не разделять между ядрами)
- Atomic operations в bucket-based (см. выше)
- Per-goroutine counter с periodic flush в shared

### 8. Approximate ≠ wrong

Sliding window approximation даёт ±5-10% точность. Для rate limit'а это **нормально** — никто не считает миллисекунды. Для биллинга — нужна точность, другой подход.

---

## Возможные расширения

### 1. Distributed sliding window через Redis

```redis
# Через Sorted Set с timestamp как score
ZADD requests:user-42 <timestamp> <unique-id>
ZREMRANGEBYSCORE requests:user-42 0 <cutoff>
ZCARD requests:user-42  # текущий count
```

Точный, но Redis нагружается. Альтернатива — Redis HyperLogLog (approximate cardinality).

### 2. Multiple time scales

Sliding 1m + 5m + 15m + 1h одновременно — для multi-tier rate limiting:
- 100 req/sec
- 1000 req/min
- 10000 req/hour

Несколько counter'ов проверяются вместе.

### 3. Histogram in window

Не просто count, а **distribution** — p50, p99 latency за последнюю минуту. Для metrics dashboards.

### 4. Top-K в sliding window

См. также [02-top-k.md](./02-top-k.md). Hot URLs за последние 5 минут — top-K + sliding window combined.

### 5. Decay function

Вместо hard expiration — exponential decay:
```go
count = count * exp(-elapsed/halflife) + 1
```

Старые события постепенно теряют вес. Используется в Hacker News ranking, EWMA metrics.

### 6. Per-shard counter

Разделить counter на N shards по hash(key). Меньше lock contention.

---

## Реальные применения

- **Rate limiting** — все основные web frameworks
- **Real-time metrics** — Prometheus rate(), Grafana queries
- **Anomaly detection** — "вдруг 10x больше запросов за минуту"
- **Top-K analytics** — самые популярные URL'ы за last hour
- **DDoS protection** — Cloudflare, AWS WAF
- **Auto-scaling triggers** — Kubernetes HPA based on requests/sec

---

## Что важно показать на собеседовании

1. **Trade-offs алгоритмов:**
   - Exact log — память O(N), точно
   - Bucket-based — память O(buckets), approximate ±bucketSize
   - Weighted sliding — память O(1), approximate но достаточно точно
2. **Boundary problem** в fixed window — почему sliding нужен
3. **Concurrent через atomic per bucket** — fast lock-less
4. **Cleanup для idle keys** — без него memory leak
5. **Distributed через Redis sorted set** — production случай
6. **Per-key + global** — hierarchical rate limit

## Связки

- [Rate Limiter](../concurrency/02-rate-limiter.md) — использует sliding window
- [Top-K](./02-top-k.md) — combined с window для "hot in last X minutes"
- [Prometheus metrics](../../../11-devops-and-observability/prometheus-and-metrics/) — `rate()` функция
- [DDoS protection](../../../12-security/perimeter-and-traffic-protection/01-ddos-protection.md) — production sliding window
