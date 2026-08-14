# Задача 3: Streaming Aggregation

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Семантика окна](#семантика-окна)
- [Простые агрегаты](#решение-1-bucket-based-aggregation)
- [Перцентили](#решение-2-percentiles-через-hdr-histogram)
- [Per-key aggregation](#per-key-aggregation)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Возможные расширения](#возможные-расширения)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Streaming aggregation считает `count/sum/avg/min/max/percentiles` по недавней
части потока, не сохраняя всю историю. В отличие от простого
[sliding window counter](../data-structures/05-sliding-window-counter.md), здесь
у события есть измеряемое значение, например latency в микросекундах.

---

## Формулировка

> Дан поток числовых значений. По запросу верни статистику за последние
> `window`, сохраняя ограниченное число time buckets вместо всех samples.

Типичные применения:

- latency/error-size на live dashboard;
- rolling average для anomaly detection;
- per-route или per-tenant operational metrics;
- локальная предварительная агрегация перед отправкой в metrics backend.

---

## Уточняющие вопросы

1. **Processing time или event time?**
   Пример ниже использует время приёма процессом. Для event time нужны timestamp,
   watermark и политика late events.
2. **Нужны точные `sum/count` или approximate percentiles?**
   Перцентили нельзя восстановить из `sum` и `count`.
3. **Какова допустимая погрешность границы окна?**
   Time buckets включают пограничный bucket целиком.
4. **Какие диапазон и единица значения?**
   Для latency удобно хранить целые микросекунды, а не `float64` nanoseconds.
5. **Какова cardinality ключей?**
   `route` и `status_class` обычно безопаснее, чем raw `user_id` или URL.
6. **Соотношение writes и reads?**
   Если `Stats` вызывается очень часто, merge всех buckets может стать дороже
   записи.

---

## Семантика окна

Пусть `window = 60s`, `bucketSize = 10s`. Запрос в `12:01:05` должен видеть
интервал примерно `(12:00:05, 12:01:05]`. Но первый пересекающийся bucket
`[12:00:00, 12:00:10)` содержит и пять лишних секунд.

```text
desired:       (12:00:05 -------------------- 12:01:05]
included: [12:00:00) ... whole buckets ... [12:01:00, 12:01:10)
error:     < 10s at the left boundary
```

Следовательно:

- `count/sum/min/max` точны для выбранных целых buckets;
- относительно идеального sliding window возможна примесь данных не старше
  одного `bucketSize` за левой границей;
- для покрытия окна нужно `ceil(window / bucketSize) + 1` slots, а не
  `floor(window / bucketSize)`;
- меньший bucket уменьшает временную погрешность, но увеличивает память и цену
  `Stats`.

Реализация использует elapsed time от момента создания. В обычном запуске
`time.Sub` использует monotonic component значений `time.Now`, поэтому изменение
wall clock не «оживляет» старые buckets. Это processing-time семантика, а не
решение для распределённого event time.

---

## Решение 1: bucket-based aggregation

```go
package streamagg

import (
    "errors"
    "math"
    "sync"
    "time"
)

var (
    ErrInvalidConfig = errors.New("stream aggregator: invalid configuration")
    ErrInvalidValue  = errors.New("stream aggregator: NaN and Inf are not supported")
)

type bucket struct {
    start int64 // elapsed nanoseconds from origin
    used  bool
    sum   float64
    count int64
    min   float64
    max   float64
}

type SimpleAggregator struct {
    windowNS     int64
    bucketSizeNS int64
    origin       time.Time
    now          func() time.Time

    mu      sync.RWMutex
    buckets []bucket
}

func New(window, bucketSize time.Duration) (*SimpleAggregator, error) {
    return newSimpleAggregator(window, bucketSize, time.Now)
}

func newSimpleAggregator(
    window, bucketSize time.Duration,
    now func() time.Time,
) (*SimpleAggregator, error) {
    slots, err := bucketSlots(window, bucketSize)
    if err != nil || now == nil {
        return nil, ErrInvalidConfig
    }
    return &SimpleAggregator{
        windowNS:     int64(window),
        bucketSizeNS: int64(bucketSize),
        origin:       now(),
        now:          now,
        buckets:      make([]bucket, slots),
    }, nil
}

func bucketSlots(window, bucketSize time.Duration) (int, error) {
    const maxBucketSlots = 1_000_000 // safety limit; подбирается по memory budget

    if window <= 0 || bucketSize <= 0 {
        return 0, ErrInvalidConfig
    }
    slots := int64(window / bucketSize)
    if window%bucketSize != 0 {
        slots++
    }
    if slots >= maxBucketSlots {
        return 0, ErrInvalidConfig
    }
    slots++ // bucket, пересекающий левую границу
    return int(slots), nil
}

func (a *SimpleAggregator) Add(value float64) error {
    if math.IsNaN(value) || math.IsInf(value, 0) {
        return ErrInvalidValue
    }

    elapsed := a.now().Sub(a.origin).Nanoseconds()
    if elapsed < 0 {
        return errors.New("stream aggregator: clock moved before origin")
    }
    bucketStart := elapsed - elapsed%a.bucketSizeNS
    bucketID := bucketStart / a.bucketSizeNS
    idx := int(bucketID % int64(len(a.buckets)))

    a.mu.Lock()
    defer a.mu.Unlock()

    b := &a.buckets[idx]
    if !b.used || b.start != bucketStart {
        *b = bucket{
            start: bucketStart,
            used:  true,
            min:   value,
            max:   value,
        }
    }
    if b.count == math.MaxInt64 {
        return ErrInvalidValue
    }
    nextSum := b.sum + value
    if math.IsNaN(nextSum) || math.IsInf(nextSum, 0) {
        return ErrInvalidValue
    }
    b.sum = nextSum
    b.count++
    if value < b.min {
        b.min = value
    }
    if value > b.max {
        b.max = value
    }
    return nil
}

type Stats struct {
    HasData bool
    Count   int64
    Sum     float64
    Avg     float64
    Min     float64
    Max     float64
}

func (a *SimpleAggregator) Stats() Stats {
    elapsed := a.now().Sub(a.origin).Nanoseconds()
    cutoff := elapsed - a.windowNS

    a.mu.RLock()
    defer a.mu.RUnlock()

    var result Stats
    for i := range a.buckets {
        b := &a.buckets[i]
        bucketEnd := b.start + a.bucketSizeNS
        if !b.used || b.count == 0 || b.start > elapsed || bucketEnd <= cutoff {
            continue
        }

        if !result.HasData {
            result.HasData = true
            result.Min = b.min
            result.Max = b.max
        } else {
            result.Min = min(result.Min, b.min)
            result.Max = max(result.Max, b.max)
        }
        result.Sum += b.sum
        result.Count += b.count
    }
    if result.Count > 0 {
        result.Avg = result.Sum / float64(result.Count)
    }
    return result
}
```

На пустом окне `HasData=false`, а остальные поля имеют zero values. Это лучше,
чем возвращать `Min=+Inf` и `Max=-Inf`, которые легко случайно сериализовать в
API response.

Сложность:

- `Add` — `O(1)` под одним mutex;
- `Stats` — `O(number of buckets)`;
- память — `O(ceil(window/bucketSize))`, независимо от числа samples.

Лимит `1_000_000` slots в примере защищает конструктор от очевидно опасной
аллокации. В production его заменяют конфигурационным пределом, рассчитанным из
memory budget и измеренного размера bucket.

---

## Решение 2: percentiles через HDR Histogram

`p99` нельзя вычислить из `sum/count`, а среднее арифметическое нескольких `p99`
не равно `p99` объединённого потока. Для bounded-memory approximation каждый
time bucket может хранить HDR Histogram.

Ниже показана существенная часть реализации; расчёт `bucketSlots` и monotonic
origin такой же, как в предыдущем примере.

```go
import (
    "errors"
    "fmt"
    "sync"
    "time"

    hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
)

type hdrBucket struct {
    start int64
    used  bool
    hist  *hdrhistogram.Histogram
}

type HDRAggregator struct {
    windowNS     int64
    bucketSizeNS int64
    lowest       int64
    highest      int64
    sigfigs      int
    origin       time.Time
    now          func() time.Time

    mu      sync.RWMutex
    buckets []hdrBucket
}

func NewHDR(
    window, bucketSize time.Duration,
    lowest, highest int64,
    sigfigs int,
) (*HDRAggregator, error) {
    slots, err := bucketSlots(window, bucketSize)
    if err != nil || lowest < 1 || highest < lowest || sigfigs < 1 || sigfigs > 5 {
        return nil, ErrInvalidConfig
    }

    now := time.Now
    a := &HDRAggregator{
        windowNS:     int64(window),
        bucketSizeNS: int64(bucketSize),
        lowest:       lowest,
        highest:      highest,
        sigfigs:      sigfigs,
        origin:       now(),
        now:          now,
        buckets:      make([]hdrBucket, slots),
    }
    for i := range a.buckets {
        a.buckets[i].hist = hdrhistogram.New(lowest, highest, sigfigs)
    }
    return a, nil
}

func (a *HDRAggregator) Add(value int64) error {
    elapsed := a.now().Sub(a.origin).Nanoseconds()
    if elapsed < 0 {
        return errors.New("stream aggregator: clock moved before origin")
    }
    bucketStart := elapsed - elapsed%a.bucketSizeNS
    idx := int((bucketStart / a.bucketSizeNS) % int64(len(a.buckets)))

    a.mu.Lock()
    defer a.mu.Unlock()

    b := &a.buckets[idx]
    if !b.used || b.start != bucketStart {
        b.start = bucketStart
        b.used = true
        b.hist.Reset()
    }
    // Значение вне [lowest, highest] возвращает error и не должно теряться тихо.
    return b.hist.RecordValue(value)
}

type Percentiles struct {
    HasData bool
    Count   int64
    P50     int64
    P95     int64
    P99     int64
    P999    int64
}

func (a *HDRAggregator) Percentiles() (Percentiles, error) {
    elapsed := a.now().Sub(a.origin).Nanoseconds()
    cutoff := elapsed - a.windowNS

    a.mu.RLock()
    defer a.mu.RUnlock()

    merged := hdrhistogram.New(a.lowest, a.highest, a.sigfigs)
    for i := range a.buckets {
        b := &a.buckets[i]
        if !b.used || b.start > elapsed || b.start+a.bucketSizeNS <= cutoff {
            continue
        }
        if dropped := merged.Merge(b.hist); dropped != 0 {
            return Percentiles{}, fmt.Errorf("HDR merge dropped %d samples", dropped)
        }
    }

    count := merged.TotalCount()
    if count == 0 {
        return Percentiles{}, nil
    }
    return Percentiles{
        HasData: true,
        Count:   count,
        P50:     merged.ValueAtQuantile(50),
        P95:     merged.ValueAtQuantile(95),
        P99:     merged.ValueAtQuantile(99),
        P999:    merged.ValueAtQuantile(99.9),
    }, nil
}
```

Важные trade-offs:

- память bounded, но не фиксированные «10 KB»: она зависит от range, числа
  significant figures и числа time buckets; библиотека предоставляет
  `ByteSize()` для оценки одного histogram;
- `RecordValue` возвращает ошибку вне trackable range — её нельзя игнорировать;
- `Merge` возвращает число отброшенных samples — его тоже нужно проверять;
- HDR хранит целые значения. Для latency обычно выбирают микросекунды; для
  decimal величин сначала задают явный scale;
- histogram approximation относится к значениям, а временная bucket-погрешность
  остаётся отдельной;
- fixed-bucket histogram Prometheus и HDR Histogram — разные структуры, хотя обе
  позволяют агрегировать распределение.

---

## Per-key aggregation

Для `route`, `tenant` или другой dimension обычно используют
`map[key]*Aggregator`. Здесь появляются дополнительные задачи:

- ограничить число keys и удалять idle entries;
- запретить unbounded labels вроде raw URL, request ID или произвольного user ID;
- останавливать cleanup goroutine через идемпотентный `Close`;
- не держать map mutex во время полного `Stats`, иначе один большой aggregator
  задержит все keys;
- определить, считается ли чтение активностью или cleanupAge обновляется только
  при `Add`.

Безопасный lifecycle cleanup выглядит так:

```go
type PerKeyAggregator struct {
    mu       sync.RWMutex
    entries  map[string]*entry
    closed   bool
    stop     chan struct{}
    done     chan struct{}
    closeOnce sync.Once
}

type entry struct {
    agg      *SimpleAggregator
    lastSeen time.Time
}

func (p *PerKeyAggregator) Close() {
    p.closeOnce.Do(func() {
        p.mu.Lock()
        p.closed = true
        p.mu.Unlock()
        close(p.stop)
    })
    <-p.done
}

func (p *PerKeyAggregator) cleanupLoop(
    cleanupEvery, cleanupAge time.Duration,
    now func() time.Time,
) {
    ticker := time.NewTicker(cleanupEvery)
    defer ticker.Stop()
    defer close(p.done)

    for {
        select {
        case <-p.stop:
            return
        case <-ticker.C:
            cutoff := now().Add(-cleanupAge)
            p.mu.Lock()
            for key, item := range p.entries {
                if !item.lastSeen.After(cutoff) {
                    delete(p.entries, key)
                }
            }
            p.mu.Unlock()
        }
    }
}
```

Полная реализация должна валидировать `window`, `bucketSize`, `cleanupAge` до
старта goroutine. Для очень высокой cardinality локальный cleanup не заменяет
лимит keys, eviction policy или агрегацию на стороне metrics backend.

---

## Тесты

Clock передаётся зависимостью, поэтому expiry проверяется без `Sleep`.

```go
func TestAggregator_BasicStats(t *testing.T) {
    now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    agg, err := newSimpleAggregator(time.Minute, time.Second, func() time.Time {
        return now
    })
    if err != nil {
        t.Fatal(err)
    }

    for _, value := range []float64{10, 20, 30, 40, 50} {
        if err := agg.Add(value); err != nil {
            t.Fatal(err)
        }
    }

    got := agg.Stats()
    if !got.HasData || got.Count != 5 || got.Sum != 150 || got.Avg != 30 {
        t.Fatalf("Stats() = %+v", got)
    }
    if got.Min != 10 || got.Max != 50 {
        t.Fatalf("min=%v max=%v", got.Min, got.Max)
    }
}

func TestAggregator_ExpiresAfterBoundaryBucket(t *testing.T) {
    now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    agg, err := newSimpleAggregator(100*time.Millisecond, 30*time.Millisecond, func() time.Time {
        return now
    })
    if err != nil {
        t.Fatal(err)
    }
    if err := agg.Add(42); err != nil {
        t.Fatal(err)
    }

    // Bucket [0, 30ms) полностью левее cutoff=31ms.
    now = now.Add(131 * time.Millisecond)
    if got := agg.Stats(); got.HasData || got.Count != 0 {
        t.Fatalf("expired Stats() = %+v", got)
    }
}

func TestAggregator_Concurrent(t *testing.T) {
    fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    agg, err := newSimpleAggregator(time.Second, 100*time.Millisecond, func() time.Time {
        return fixed
    })
    if err != nil {
        t.Fatal(err)
    }

    var wg sync.WaitGroup
    for range 10 {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for range 1000 {
                if err := agg.Add(1); err != nil {
                    t.Error(err)
                    return
                }
            }
        }()
    }
    wg.Wait()

    if got := agg.Stats(); got.Count != 10_000 || got.Sum != 10_000 {
        t.Fatalf("Stats() = %+v", got)
    }
}
```

---

## Подводные камни

### 1. `floor(window/bucketSize)` slots

Если окно не делится на bucket или текущий момент не совпадает с границей,
`floor` не покрывает весь интервал. Нужны ceiling и дополнительный boundary slot.

### 2. Обещание «точного sliding window»

Bucket хранит агрегат без индивидуальных timestamps, поэтому часть самого
старого bucket нельзя вычесть точно. Либо принимают погрешность, либо хранят
samples/более мелкие buckets.

### 3. Пустой результат как `±Inf`

`Min=+Inf` и `Max=-Inf` удобны как внутренние sentinels, но плохи как внешний
контракт. Нужен `HasData`, `ok` или nullable fields.

### 4. Среднее перцентилей

```text
p99(all) != average(p99(bucket_1), p99(bucket_2), ...)
```

Нужно merge распределений, а не готовых percentile values.

### 5. Игнорирование range error HDR

Outlier вне `highest` не записывается. Ошибку нужно считать метрикой и либо
расширять range, либо применять явную clamp/drop policy.

### 6. Ошибочная работа со временем

`UnixNano` использует wall clock. `Round(0)` не «включает monotonic» — наоборот,
он удаляет monotonic reading. Для process-local processing time можно считать
elapsed duration между значениями `time.Now`; для event time нужны watermarks.

### 7. Float для денег и накопление ошибки

Для денег используют integer minor units/decimal, а не `float64`. Для огромного
числа очень разных по масштабу samples простой `sum += value` также может
накапливать floating-point error; при необходимости применяют compensated sum.

### 8. Hot-key contention

Разделение locks «по времени» не помогает: почти все writers попадают в текущий
bucket. Рабочий вариант — несколько stripe aggregators по hash producer/key и
merge их результатов либо single-owner goroutine.

### 9. Per-key explosion

При `24h / 1s` получается `86_401` time slots на один key. Умножение на тысячи
keys быстро становится неприемлемым даже без точной оценки bytes per bucket.
Размер нужно считать как `keys × slots × measured bytes/slot` и подтверждать heap
profile.

### 10. Read amplification

Каждый `Stats` сканирует buckets, а HDR-вариант ещё и строит merged histogram.
При частом чтении нужны cached snapshots, incremental totals или более грубая
granularity.

---

## Возможные расширения

- Multi-resolution retention: недавние данные в мелких buckets, старые — в
  крупных.
- KLL/t-digest для другого trade-off точности quantiles и merge.
- HyperLogLog для cardinality и Count-Min Sketch для приблизительных частот.
- Windowed joins двух потоков — отдельная задача с event time, state retention и
  late-event policy.

---

## Interview-ready answer

**1. Почему buckets дают bounded memory?**

- Модель — хранится фиксированное число интервалов, а не каждый sample.
- Память — примерно пропорциональна `ceil(window/bucketSize)`.
- Цена — погрешность до одного bucket на левой границе.

**2. Почему из `sum/count` нельзя получить `p99`?**

- Ограничение — `sum/count` не сохраняют форму распределения.
- Структура — нужны samples или mergeable approximation: HDR, t-digest, KLL.
- Merge — усреднять `p99` отдельных buckets математически неверно.

**3. Как работать со временем?**

- Выбор — сначала определить processing time или event time.
- Processing time — использовать monotonic component `time.Now` для elapsed time.
- Event time — определить watermark, allowed lateness и обработку late events.

**4. Где bottleneck?**

- Writes — writers конкурируют за текущий bucket.
- Reads — readers сканируют все buckets и могут дорого merge'ить histograms.
- Решение — выбирают по profile: stripes/single owner для writes, snapshots для
  reads, лимит cardinality для per-key state.

---

## Связанные материалы

- [Sliding Window Counter](../data-structures/05-sliding-window-counter.md)
- [Prometheus metrics](../../../10-devops-and-observability/prometheus-and-metrics/)
- [Bloom Filter](../data-structures/03-bloom-filter.md)
- [HDR Histogram: Go package](https://pkg.go.dev/github.com/HdrHistogram/hdrhistogram-go)
- [Go monotonic clocks](https://pkg.go.dev/time#hdr-Monotonic_Clocks)
