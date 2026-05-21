# Задача 3: Streaming Aggregation

Подсчёт **агрегатных метрик** (sum/avg/percentiles/count) над sliding window поток событий. Отличается от [sliding window counter](../data-structures/05-sliding-window-counter.md) — там просто count, здесь — full aggregation (значения, не события).

## Формулировка

> "Дан поток numeric events (например, latency requests). Возвращай статистики (avg, p50, p99) за последние N секунд по требованию `Stats()`. Память O(window), не O(history)."

Use cases:
- Real-time metrics (avg latency last minute)
- Anomaly detection (p99 latency правда выше normal?)
- Live dashboards (active users per minute)
- Trading systems (rolling moving average)
- Per-user activity stats

---

## Уточняющие вопросы

1. **Какие агрегаты — sum/avg/min/max простые или percentiles?**
   "Простые — O(1) per event. Percentiles — O(N) (если хранить values) или approximate (HDR Histogram)."

2. **Точность важна или approximate OK?**
   "Точно — хранить все values. Approximate — t-digest или HDR Histogram."

3. **Размер окна и granularity bucket'а?**
   "60s window with 1s buckets — sliding с 60 bucket'ов. Trade-off accuracy vs memory."

4. **Latency requirements?**
   "Read latency обычно жёсткий (метрики чтаем часто). Write — bursty."

5. **Multi-dimensional (per-user, per-route)?**
   "Map of aggregators. Cleanup для idle keys."

6. **Concurrent Add + Stats?**
   "Обязательно. Гонка частая."

---

## Решение 1: Bucket-based aggregation (точно для простых статов)

Разбиваем окно на N bucket'ов. В каждом — sum, count, min, max. Скользящий sum'ой по bucket'ам.

```go
package streamagg

import (
    "math"
    "sync"
    "sync/atomic"
    "time"
)

type bucket struct {
    timestamp int64    // unix nano, начало bucket'а
    sum       float64
    count     int64
    min       float64
    max       float64
}

type SimpleAggregator struct {
    window      time.Duration
    bucketSize  time.Duration
    bucketCount int

    mu      sync.Mutex
    buckets []*bucket
}

func New(window, bucketSize time.Duration) *SimpleAggregator {
    n := int(window / bucketSize)
    buckets := make([]*bucket, n)
    for i := range buckets {
        buckets[i] = &bucket{min: math.Inf(1), max: math.Inf(-1)}
    }
    return &SimpleAggregator{
        window:      window,
        bucketSize:  bucketSize,
        bucketCount: n,
        buckets:     buckets,
    }
}

func (a *SimpleAggregator) Add(value float64) {
    now := time.Now().UnixNano()
    bucketStart := now - (now % int64(a.bucketSize))
    idx := (bucketStart / int64(a.bucketSize)) % int64(a.bucketCount)

    a.mu.Lock()
    defer a.mu.Unlock()

    b := a.buckets[idx]
    if b.timestamp != bucketStart {
        // Reset stale bucket
        b.timestamp = bucketStart
        b.sum = 0
        b.count = 0
        b.min = math.Inf(1)
        b.max = math.Inf(-1)
    }

    b.sum += value
    b.count++
    if value < b.min {
        b.min = value
    }
    if value > b.max {
        b.max = value
    }
}

type Stats struct {
    Count int64
    Sum   float64
    Avg   float64
    Min   float64
    Max   float64
}

func (a *SimpleAggregator) Stats() Stats {
    now := time.Now().UnixNano()
    cutoff := now - int64(a.window)

    a.mu.Lock()
    defer a.mu.Unlock()

    var s Stats
    s.Min = math.Inf(1)
    s.Max = math.Inf(-1)

    for _, b := range a.buckets {
        if b.timestamp < cutoff || b.count == 0 {
            continue
        }
        s.Sum += b.sum
        s.Count += b.count
        if b.min < s.Min {
            s.Min = b.min
        }
        if b.max > s.Max {
            s.Max = b.max
        }
    }

    if s.Count > 0 {
        s.Avg = s.Sum / float64(s.Count)
    }
    return s
}
```

**Использование:**

```go
agg := streamagg.New(time.Minute, time.Second)

// Записать события
for latency := range latencyStream {
    agg.Add(float64(latency.Milliseconds()))
}

// Запросить statы (можно вызывать часто)
s := agg.Stats()
fmt.Printf("avg=%.2fms p99=N/A count=%d\n", s.Avg, s.Count)
```

**Trade-offs:**
- ✅ O(buckets) memory — типично 60-360 bucket'ов
- ✅ O(1) Add
- ✅ O(buckets) Stats — fast read
- ❌ **Не поддерживает percentiles** — для них нужны actual values

---

## Решение 2: Percentiles через HDR Histogram

Percentiles нельзя посчитать только из sum/count. Нужно хранить **distribution** значений.

**Naive approach** — хранить все values: O(events in window) memory. Не масштабируется.

**Better:** HDR Histogram — bucket'ы для каждого "bucket of magnitude" (logarithmic). Approximate но **constant memory**.

```go
import "github.com/HdrHistogram/hdrhistogram-go"

type HDRAggregator struct {
    window      time.Duration
    bucketSize  time.Duration
    bucketCount int

    mu      sync.Mutex
    buckets []*hdrBucket
}

type hdrBucket struct {
    timestamp int64
    hist      *hdrhistogram.Histogram
}

func NewHDR(window, bucketSize time.Duration, min, max int64) *HDRAggregator {
    n := int(window / bucketSize)
    buckets := make([]*hdrBucket, n)
    for i := range buckets {
        buckets[i] = &hdrBucket{
            hist: hdrhistogram.New(min, max, 3),  // 3 = significant digits
        }
    }
    return &HDRAggregator{
        window:      window,
        bucketSize:  bucketSize,
        bucketCount: n,
        buckets:     buckets,
    }
}

func (a *HDRAggregator) Add(value int64) {
    now := time.Now().UnixNano()
    bucketStart := now - (now % int64(a.bucketSize))
    idx := (bucketStart / int64(a.bucketSize)) % int64(a.bucketCount)

    a.mu.Lock()
    defer a.mu.Unlock()

    b := a.buckets[idx]
    if b.timestamp != bucketStart {
        b.timestamp = bucketStart
        b.hist.Reset()
    }
    b.hist.RecordValue(value)
}

type Percentiles struct {
    Count int64
    P50   int64
    P95   int64
    P99   int64
    P999  int64
}

func (a *HDRAggregator) Percentiles() Percentiles {
    now := time.Now().UnixNano()
    cutoff := now - int64(a.window)

    a.mu.Lock()
    defer a.mu.Unlock()

    // Merge все валидные histogram'ы
    merged := hdrhistogram.New(1, 1_000_000_000, 3)
    for _, b := range a.buckets {
        if b.timestamp >= cutoff {
            merged.Merge(b.hist)
        }
    }

    return Percentiles{
        Count: merged.TotalCount(),
        P50:   merged.ValueAtQuantile(50),
        P95:   merged.ValueAtQuantile(95),
        P99:   merged.ValueAtQuantile(99),
        P999:  merged.ValueAtQuantile(99.9),
    }
}
```

**Trade-offs:**
- ✅ Constant memory — ~10 KB на histogram (≪ хранение всех values)
- ✅ Approximate но accurate ~0.1% для percentiles
- ✅ Merge'абельный — distribute из multi-instance
- ❌ Только positive integers (для floats — multiply by 1000)
- ❌ Pre-defined range (min, max)

В Prometheus internals — что-то похожее (histograms).

---

## Решение 3: Per-key aggregation (для multi-dimensional)

Per-user, per-route, per-tenant — map с aggregator на каждый key + cleanup для idle.

```go
type PerKeyAggregator struct {
    mu         sync.Mutex
    aggs       map[string]*SimpleAggregator
    lastSeen   map[string]time.Time
    cleanupAge time.Duration

    window     time.Duration
    bucketSize time.Duration
}

func NewPerKey(window, bucketSize, cleanupAge time.Duration) *PerKeyAggregator {
    p := &PerKeyAggregator{
        aggs:       make(map[string]*SimpleAggregator),
        lastSeen:   make(map[string]time.Time),
        cleanupAge: cleanupAge,
        window:     window,
        bucketSize: bucketSize,
    }
    go p.cleanupLoop()
    return p
}

func (p *PerKeyAggregator) Add(key string, value float64) {
    p.mu.Lock()
    agg, ok := p.aggs[key]
    if !ok {
        agg = New(p.window, p.bucketSize)
        p.aggs[key] = agg
    }
    p.lastSeen[key] = time.Now()
    p.mu.Unlock()

    agg.Add(value)
}

func (p *PerKeyAggregator) Stats(key string) (Stats, bool) {
    p.mu.Lock()
    agg, ok := p.aggs[key]
    p.mu.Unlock()
    if !ok {
        return Stats{}, false
    }
    return agg.Stats(), true
}

func (p *PerKeyAggregator) cleanupLoop() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        p.mu.Lock()
        cutoff := time.Now().Add(-p.cleanupAge)
        for key, last := range p.lastSeen {
            if last.Before(cutoff) {
                delete(p.aggs, key)
                delete(p.lastSeen, key)
            }
        }
        p.mu.Unlock()
    }
}
```

Каждый key — независимый aggregator. Cleanup для idle (typical: `cleanupAge = window * 2`).

---

## Тесты

```go
func TestAggregator_BasicStats(t *testing.T) {
    agg := New(time.Minute, time.Second)

    values := []float64{10, 20, 30, 40, 50}
    for _, v := range values {
        agg.Add(v)
    }

    s := agg.Stats()
    if s.Count != 5 {
        t.Errorf("count %d, want 5", s.Count)
    }
    if s.Avg != 30 {
        t.Errorf("avg %f, want 30", s.Avg)
    }
    if s.Min != 10 || s.Max != 50 {
        t.Errorf("min %f max %f", s.Min, s.Max)
    }
}

func TestAggregator_WindowExpiry(t *testing.T) {
    agg := New(100*time.Millisecond, 10*time.Millisecond)

    agg.Add(100)
    time.Sleep(150 * time.Millisecond)

    s := agg.Stats()
    if s.Count != 0 {
        t.Errorf("after window count %d, want 0", s.Count)
    }
}

func TestAggregator_Concurrent(t *testing.T) {
    agg := New(time.Second, 100*time.Millisecond)

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 1000; j++ {
                agg.Add(float64(j))
            }
        }()
    }
    wg.Wait()

    s := agg.Stats()
    if s.Count != 10000 {
        t.Errorf("count %d, want 10000", s.Count)
    }
}
```

---

## Подводные камни

### 1. Очень большое окно с малыми bucket'ами

```go
New(24*time.Hour, time.Second)  // 86400 bucket'ов
```

86400 buckets × 100 bytes = 8.6 MB на один aggregator. С per-key map'ом и 10k user'ов — 86 GB. Reduce buckets.

### 2. Bucket reset race

```go
// Two goroutines:
b.timestamp = newStart  // ← один пишет
sum := b.sum            // ← второй ещё читает старый

// Под одним lock — fine. Atomic — нет.
```

Mutex обязателен.

### 3. Percentiles не складываются как sum

```go
p99_total = (p99_bucket1 + p99_bucket2) / 2  // ← ЭТО НЕПРАВИЛЬНО
```

Avg(percentiles) != percentile(merged). Нужно либо HDR Histogram (merge'able), либо хранить все samples.

### 4. Clock skew / drift

`time.Now()` может прыгать назад (NTP). Старый bucket "оживёт". Использовать monotonic — `time.Now().Round(0)` или `time.Since(start)`.

### 5. Float precision

```go
sum += value
// После 10M маленьких add'ов float64 может потерять точность
```

Для money — int (cents), не float. Или Kahan summation.

### 6. Per-key explosion

```go
PerKeyAggregator with key = user_id  // если миллионы user'ов
```

Map огромный. Cleanup критичен. Или sharding.

### 7. Read latency растёт с window'ом

`Stats()` итерирует все bucket'ы. С 86400 bucket'ов = ms latency. Если Stats вызывается часто — cache result.

### 8. Hot bucket contention

Все add'ы в **текущий** bucket идут под одним mutex'е. На 100k events/sec — bottleneck.

Sharding по time (один lock per N buckets) или atomic-based bucket update.

### 9. Missing data при reset

```go
if b.timestamp != bucketStart {
    b.sum = 0  // ← reset до записи
    b.count = 0
}
b.sum += value
```

Если несколько workers одновременно — кто-то записал в "новый" bucket, ктo-то ещё в "старом". Под lock — OK.

### 10. HDR Histogram pre-defined range

```go
hdr := hdrhistogram.New(1, 1_000_000, 3)
hdr.RecordValue(2_000_000)  // ← вне range, ошибка
```

Outliers handle separately.

---

## Возможные расширения

### 1. Distributed aggregation

Несколько сервисов суммируют → coordinator merge'ит. Через Prometheus pull или Kafka consumer aggregator.

### 2. Multi-resolution (Carbonara/Whisper-style)

Recent — fine granularity (1s buckets, last hour). Older — coarse (1min buckets, last day). Tier'ed retention.

### 3. T-digest вместо HDR

T-digest — другой approximate percentile algorithm, лучше для skewed distributions.

### 4. Stream Joins

Aggregate events from **multiple streams** (e.g., orders + payments). Time-windowed join.

### 5. Reservoir Sampling

Если хочешь сохранить **N random samples** для post-hoc analysis. Используется в Datadog, NewRelic.

### 6. Sketch-based aggregation

Apache DataSketches: HyperLogLog (cardinality), CountMinSketch (frequency), KLL (quantiles). Production-grade approximate aggregations.

---

## Что важно показать на собеседовании

1. **Bucket-based** для constant memory
2. **Percentiles требуют distribution** — sum/count недостаточно
3. **HDR Histogram** для approximate percentiles
4. **Trade-off accuracy vs memory** — больше bucket'ов = точнее
5. **Sharding** для high concurrency Add'ов
6. **Per-key с cleanup** — для multi-dimensional
7. **Avg of percentiles ≠ percentile of merged** — частая ошибка
8. **Prometheus histograms** — production reference

## Связки

- [Sliding Window Counter](../data-structures/05-sliding-window-counter.md) — родственный — count only
- [Prometheus metrics](../../../11-devops-and-observability/prometheus-and-metrics/) — production
- [Bloom filter](../data-structures/03-bloom-filter.md) — другая probabilistic structure
- [HDR Histogram](https://github.com/HdrHistogram/hdrhistogram-go)
