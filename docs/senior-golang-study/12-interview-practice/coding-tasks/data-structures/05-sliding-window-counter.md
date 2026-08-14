# Задача 5: Sliding Window Counter

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Точный журнал событий](#решение-1-точное--log-of-timestamps)
- [Временные бакеты](#решение-2-bucket-based-approximate-fast)
- [Взвешенное приближение](#решение-3-sliding-window-counter-weighted-approximation)
- [Счётчики по ключам](#решение-4-per-key-sliding-window-rate-limit)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Расширения](#возможные-расширения)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связки)

Sliding window counter отвечает на вопрос «сколько событий произошло за
последний интервал времени». Точный вариант хранит моменты событий, а
приближённый объединяет их во временные бакеты и ограничивает память.

---

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
    "errors"
    "sync"
    "time"
)

var ErrInvalidWindow = errors.New("slidingwindow: window must be positive")

// ExactCounter хранит точные timestamp'ы всех событий.
// Memory: O(events in window). Подходит для малых rates.
type ExactCounter struct {
    mu     sync.Mutex
    window time.Duration
    events []time.Time
    now    func() time.Time
}

func NewExactCounter(window time.Duration) (*ExactCounter, error) {
    return newExactCounter(window, time.Now)
}

func newExactCounter(window time.Duration, now func() time.Time) (*ExactCounter, error) {
    if window <= 0 || now == nil {
        return nil, ErrInvalidWindow
    }
    return &ExactCounter{window: window, now: now}, nil
}

func (c *ExactCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    now := c.now()
    c.events = append(c.events, now)
    c.evictExpired(now)
}

func (c *ExactCounter) Count() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.evictExpired(c.now())
    return len(c.events)
}

func (c *ExactCounter) evictExpired(now time.Time) {
    cutoff := now.Add(-c.window)
    i := 0
    for i < len(c.events) && !c.events[i].After(cutoff) {
        i++
    }
    c.events = c.events[i:]

    if cap(c.events) > 4*len(c.events) {
        compact := append([]time.Time(nil), c.events...)
        c.events = compact
    }
}
```

**Использование:**

```go
c, err := NewExactCounter(time.Minute)
if err != nil {
    log.Fatal(err)
}

for i := 0; i < 100; i++ {
    c.Inc()
}

fmt.Println(c.Count())  // 100 (если меньше минуты прошло)
```

**Trade-offs:**
- ✅ Точный — никакой approximation
- ❌ Память `O(events in window)` — при одном миллионе событий в минутном окне
  хранится один миллион `time.Time`; размер нужно измерять для конкретного типа и
  учитывать capacity backing array
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
    "errors"
    "sync"
    "time"
)

var ErrInvalidBucketConfig = errors.New("slidingwindow: window must be divisible by positive bucket size")

type bucket struct {
    timestamp int64
    count     int64
}

// BucketCounter — sliding window через ring buffer bucket'ов.
type BucketCounter struct {
    mu          sync.Mutex
    window      time.Duration
    bucketSize  time.Duration
    bucketCount int
    buckets     []bucket
}

func NewBucketCounter(window, bucketSize time.Duration) (*BucketCounter, error) {
    if window <= 0 || bucketSize <= 0 || window%bucketSize != 0 {
        return nil, ErrInvalidBucketConfig
    }
    n := int(window / bucketSize)
    return &BucketCounter{
        window:      window,
        bucketSize:  bucketSize,
        bucketCount: n,
        buckets:     make([]bucket, n),
    }, nil
}

func (c *BucketCounter) Inc() {
    now := time.Now().UnixNano()
    bucketStart := now - (now % int64(c.bucketSize))

    c.mu.Lock()
    defer c.mu.Unlock()

    idx := (bucketStart / int64(c.bucketSize)) % int64(c.bucketCount)
    b := &c.buckets[idx]

    if b.timestamp != bucketStart {
        b.timestamp = bucketStart
        b.count = 0
    }
    b.count++
}

func (c *BucketCounter) Count() int64 {
    now := time.Now().UnixNano()
    cutoff := now - int64(c.window)

    c.mu.Lock()
    defer c.mu.Unlock()

    var total int64
    for _, b := range c.buckets {
        // Bucket считается только если timestamp в окне
        if b.timestamp >= cutoff {
            total += b.count
        }
    }
    return total
}
```

**Использование:**

```go
c, err := NewBucketCounter(time.Minute, time.Second)
if err != nil {
    log.Fatal(err)
}

for i := 0; i < 1000; i++ {
    c.Inc()
}

fmt.Println(c.Count())  // 1000
```

**Trade-offs:**

- Память — `O(B)`, где `B = window / bucketSize`.
- Синхронизация — один mutex делает reset и increment одной атомарной логической
  операцией, но сериализует обращения.
- Погрешность — реализация отбрасывает частично попадающий в окно старейший
  бакет и может недосчитать события из интервала короче `bucketSize`.
- Детализация — меньший бакет снижает временную погрешность, но увеличивает
  память и стоимость суммирования.

Число бакетов выбирается по допустимой погрешности, памяти и цене `Count`.
Универсального оптимального значения нет.

---

## Решение 3: Sliding Window Counter (weighted approximation)

Гибрид: один counter per "current" + один per "previous" window. Вес `previous` зависит от позиции внутри текущего.

```
Now: отметка 75 секунд, текущее фиксированное окно началось на 60-й секунде.
previous window: 0-60s  (count_prev = 100)
current window:  60-?   (count_curr = 50)

elapsed = 75 - 60 = 15s
weight = 1 - 15/60 = 0.75
Sliding count = 50 + 100 * 0.75 = 125
```

```go
type SlidingWindowApprox struct {
    mu            sync.Mutex
    windowSize    time.Duration
    currentCount  int64
    previousCount int64
    windowStart   time.Time
    now           func() time.Time
}

func NewSlidingWindowApprox(window time.Duration) (*SlidingWindowApprox, error) {
    if window <= 0 {
        return nil, ErrInvalidWindow
    }
    return newSlidingWindowApprox(window, time.Now), nil
}

func newSlidingWindowApprox(window time.Duration, now func() time.Time) *SlidingWindowApprox {
    current := now()
    return &SlidingWindowApprox{
        windowSize:  window,
        windowStart: current.Truncate(window),
        now:         now,
    }
}

func (s *SlidingWindowApprox) Inc() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.rotate(s.now())
    s.currentCount++
}

func (s *SlidingWindowApprox) Count() float64 {
    s.mu.Lock()
    defer s.mu.Unlock()
    now := s.now()
    s.rotate(now)

    // Сколько прошло в текущем окне
    elapsed := now.Sub(s.windowStart)
    weight := float64(s.windowSize-elapsed) / float64(s.windowSize)

    return float64(s.currentCount) + float64(s.previousCount)*weight
}

func (s *SlidingWindowApprox) rotate(now time.Time) {
    elapsed := now.Sub(s.windowStart)
    windowsPassed := int64(elapsed / s.windowSize)

    if windowsPassed >= 2 {
        // Прошло больше двух окон — оба обнулить
        s.previousCount = 0
        s.currentCount = 0
        s.windowStart = s.windowStart.Add(time.Duration(windowsPassed) * s.windowSize)
    } else if windowsPassed == 1 {
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
- ⚠️ Погрешность зависит от распределения событий внутри предыдущего окна и не
  имеет универсальной границы в несколько процентов

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

func NewPerKeyCounter(ctx context.Context, window time.Duration) (*PerKeyCounter, error) {
    if window <= 0 {
        return nil, ErrInvalidWindow
    }
    c := &PerKeyCounter{
        window:   window,
        counters: make(map[string]*SlidingWindowApprox),
        lastSeen: make(map[string]time.Time),
    }
    go c.cleanup(ctx)
    return c, nil
}

func (c *PerKeyCounter) Inc(key string) {
    c.mu.Lock()
    cnt, ok := c.counters[key]
    if !ok {
        cnt = newSlidingWindowApprox(c.window, time.Now)
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

func (c *PerKeyCounter) cleanup(ctx context.Context) {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case now := <-ticker.C:
            c.mu.Lock()
            cutoff := now.Add(-10 * c.window)
            for key, last := range c.lastSeen {
                if last.Before(cutoff) {
                    delete(c.counters, key)
                    delete(c.lastSeen, key)
                }
            }
            c.mu.Unlock()
        }
    }
}
```

**Critical:** cleanup goroutine для idle keys — без него map растёт бесконечно.

---

## Тесты

```go
func TestExactCounter(t *testing.T) {
    now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    c, err := newExactCounter(time.Minute, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }

    for i := 0; i < 10; i++ {
        c.Inc()
    }
    if c.Count() != 10 {
        t.Errorf("got %d, want 10", c.Count())
    }

    now = now.Add(time.Minute + time.Nanosecond)

    if c.Count() != 0 {
        t.Errorf("after expiry got %d, want 0", c.Count())
    }
}

func TestBucketCounter_Concurrent(t *testing.T) {
    c, err := NewBucketCounter(time.Second, 100*time.Millisecond)
    if err != nil {
        t.Fatal(err)
    }

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
    now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    c := newSlidingWindowApprox(time.Minute, func() time.Time { return now })

    for i := 0; i < 100; i++ {
        c.Inc()
    }

    // Сразу: ~100 (previous=0, current=100)
    if c.Count() < 95 {
        t.Errorf("initial count %f, expected ~100", c.Count())
    }

    now = now.Add(time.Minute)
    if count := c.Count(); count != 100 {
        t.Fatalf("at next boundary count = %f, want 100", count)
    }

    now = now.Add(30 * time.Second)
    if count := c.Count(); count != 50 {
        t.Fatalf("halfway through next window count = %f, want 50", count)
    }

    now = now.Add(31 * time.Second)
    if count := c.Count(); count != 0 {
        t.Fatalf("after previous window expired count = %f, want 0", count)
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

Fixed window допускает почти двойной лимит около границы. Точный sliding window
устраняет этот скачок, а приближённые варианты сглаживают его с погрешностью,
зависящей от своей модели.

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

Между сменой timestamp и обнулением счётчика может прийти другая goroutine.
Наивная последовательность из двух атомарных операций проблему не решает:
```go
if b.timestamp.CompareAndSwap(oldTimestamp, bucketStart) {
    b.count.Store(0) // чужой Add мог выполниться после CAS и будет потерян здесь
}
```

Нужен mutex вокруг `timestamp + count`, упакованное состояние с CAS или другой
алгоритм, который доказывает линейную точку операции.

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

При большом числе обращений к одному ключу общий mutex может стать bottleneck.

Решения:
- Sharded counter (один counter не разделять между ядрами)
- Отдельный mutex или упакованное атомарное состояние на shard
- Per-goroutine counter с periodic flush в shared

### 8. Approximate ≠ wrong

Приближение является частью контракта, а не синонимом ошибки реализации. Его
допустимость зависит от задачи: rate limiter может принять известный запас, а
биллинг требует точной и воспроизводимой модели. Процент погрешности нельзя
назвать без размера бакета и распределения событий.

---

## Возможные расширения

### 1. Distributed sliding window через Redis

```redis
# Через Sorted Set с timestamp как score
ZADD requests:user-42 <timestamp> <unique-id>
ZREMRANGEBYSCORE requests:user-42 0 <cutoff>
ZCARD requests:user-42  # текущий count
```

Точный вариант требует атомарно удалить старые элементы, добавить новое событие
и проверить `ZCARD`, обычно через Lua script или server-side transaction.
HyperLogLog здесь не замена: он оценивает число уникальных элементов, а не число
событий в окне.

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

Старые события постепенно теряют вес. Это уже не строгое sliding window, а
отдельная модель decay, подходящая для рейтингов и сглаженных метрик.

### 6. Per-shard counter

Разделить counter на N shards по hash(key). Меньше lock contention.

---

## Реальные применения

- **Rate limiting —** ограничение числа действий пользователя или клиента.
- **Метрики —** число запросов или ошибок за недавний интервал.
- **Обнаружение аномалий —** сравнение текущей интенсивности с baseline.
- **Top-K analytics —** популярные ключи за ограниченный интервал.
- **Защита от всплесков —** локальные и распределённые лимиты трафика.
- **Автомасштабирование —** производная метрика запросов в секунду для внешнего
  контроллера.

---

## Interview-ready answer

**1. Какие варианты sliding window counter существуют?**

- Точный журнал — хранит timestamp каждого события и использует память,
  пропорциональную числу событий в окне.
- Бакеты — хранят счётчик на интервал и ограничивают память, но погрешность
  зависит от ширины бакета.
- Взвешенные окна — используют два счётчика и предполагают равномерность событий
  в предыдущем окне.

**2. Где возникает ошибка на границе?**

- Fixed window — допускает почти двойной лимит около границы двух интервалов.
- Bucket-based sliding — сглаживает границу, но эта реализация отбрасывает
  частично попадающий старейший бакет и потому может недосчитать события.
- Точный журнал — сравнивает timestamp каждого события с cutoff.

**3. Что важно для конкурентной реализации?**

- Reset — timestamp бакета и его count образуют одно логическое состояние.
- Синхронизация — отдельные atomic-поля не гарантируют атомарный переход и могут
  потерять increment.
- Время — тестируемые часы устраняют `Sleep`, а clock rollback требует явной
  политики.
- Lifecycle — per-key cleanup должен останавливаться через context или `Close`.

---

## Связки

- [Rate Limiter](../concurrency/02-rate-limiter.md) — использует sliding window
- [Top-K](./02-top-k.md) — combined с window для "hot in last X minutes"
- [Prometheus metrics](../../../10-devops-and-observability/prometheus-and-metrics/) — `rate()` функция
- [DDoS protection](../../../11-security/perimeter-and-traffic-protection/01-ddos-protection.md) — production sliding window
