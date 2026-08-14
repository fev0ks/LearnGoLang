# Задача 5: Event Time, Watermarks и Late Events

## Содержание

- [Формулировка](#формулировка)
- [Три вида времени](#event-time-processing-time-и-ingestion-time)
- [Watermark](#что-означает-watermark)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Реализация](#tumbling-window-aggregator)
- [Несколько partitions](#несколько-partitions)
- [Allowed lateness](#allowed-lateness-и-обновление-результатов)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Event-time aggregation нужна, когда порядок доставки не совпадает с порядком
возникновения событий. Например, мобильное устройство создало событие в 12:00,
было offline и отправило его в 12:05. Processing-time окно отнесёт событие к
12:05, event-time окно — к 12:00.

---

## Формулировка

> События содержат `OccurredAt`, приходят не по порядку и группируются в
> минутные tumbling windows `[start, end)`. Закрывай окно по watermark, возвращай
> агрегат и отдельно отмечай late events. Память должна освобождаться после
> закрытия окна.

В этой задаче важнее сначала определить семантику, а уже потом писать `map` и
mutex. Без явной late-event policy две корректно синхронизированные реализации
могут давать разные business results.

---

## Event time, processing time и ingestion time

| Время | Что означает | Плюсы | Риски |
|---|---|---|---|
| Event time | когда событие произошло у источника | корректные исторические окна | clock skew, out-of-order, late events |
| Ingestion time | когда платформа приняла событие | удобно измерять transport delay | зависит от точки входа |
| Processing time | когда оператор обработал событие | просто, низкая задержка | результат зависит от lag/retry/restart |

Эти timestamps нельзя безоговорочно заменять друг другом. Полезно хранить как
минимум `occurred_at` и `received_at`: их разница показывает lateness и помогает
обнаружить сломанные часы producer.

---

## Что означает watermark

Watermark — монотонная оценка прогресса event time. В упрощённой модели bounded
out-of-orderness:

```text
watermark = max_observed_event_time - max_out_of_order
```

При `max_observed = 12:01:15` и `max_out_of_order = 10s` watermark равен
`12:01:05`. Tumbling window `[12:00:00, 12:01:00)` можно закрыть, потому что его
`end <= watermark`.

Watermark — не доказательство, что старое событие физически невозможно. Это
контракт/эвристика: пришедшее после закрытия событие обрабатывается по late-event
policy. Больший lag watermark принимает больше out-of-order events, но позже
выдаёт результаты и дольше хранит state.

Важно: событие с timestamp меньше watermark не обязательно late. Например,
watermark `12:01:05`, а событие `12:01:02` принадлежит окну
`[12:01:00, 12:02:00)`, которое ещё открыто. Проверять нужно закрытие его окна.

---

## Уточняющие вопросы

1. **Как назначается event timestamp?**
   Кто владеет часами, возможны ли исправления и в какой timezone он приходит?
2. **Какой out-of-order lag допустим?**
   Его выбирают по измеренному lateness distribution, а не произвольным `5m`.
3. **Что делать с late event?**
   Drop, side output/DLQ, reopen window или emit correction/retraction?
4. **Результат final или updateable?**
   Downstream должен уметь idempotent upsert, если окно переиздаётся.
5. **Сколько partitions/sources?**
   Один idle partition может остановить общий watermark.
6. **Как checkpoint связан с offsets?**
   После restart нельзя потерять state или дважды неидемпотентно выдать window.

---

## Tumbling-window aggregator

Пример считает `count/sum` по `key` и минутному окну. Он поддерживает один
логический input partition. `Add` возвращает только что закрытые окна и флаг
`late`; caller сам решает, куда отправить late event.

```go
package eventwindow

import (
    "errors"
    "math"
    "sort"
    "sync"
    "time"
)

var (
    ErrInvalidConfig = errors.New("event window: invalid configuration")
    ErrInvalidEvent  = errors.New("event window: invalid event")
    ErrFutureEvent   = errors.New("event window: event too far in the future")
    ErrClosed        = errors.New("event window: closed")
)

type Config struct {
    WindowSize    time.Duration
    MaxOutOfOrder time.Duration
    MaxFutureSkew time.Duration
}

type Event struct {
    Key        string
    Value      float64
    OccurredAt time.Time
}

type windowKey struct {
    key   string
    start time.Time
}

type windowState struct {
    count int64
    sum   float64
}

type Result struct {
    Key   string
    Start time.Time // inclusive
    End   time.Time // exclusive
    Count int64
    Sum   float64
}

type Aggregator struct {
    cfg Config

    mu           sync.Mutex
    windows      map[windowKey]windowState
    initialized  bool
    maxEventTime time.Time
    watermark    time.Time
    closed       bool
}

func New(cfg Config) (*Aggregator, error) {
    if cfg.WindowSize <= 0 || cfg.MaxOutOfOrder < 0 || cfg.MaxFutureSkew <= 0 {
        return nil, ErrInvalidConfig
    }
    return &Aggregator{
        cfg:     cfg,
        windows: make(map[windowKey]windowState),
    }, nil
}

// Add использует receivedAt как ingestion time для защиты от ошибочного
// timestamp далеко в будущем.
func (a *Aggregator) Add(
    event Event,
    receivedAt time.Time,
) (closed []Result, late bool, err error) {
    if event.Key == "" || event.OccurredAt.IsZero() || receivedAt.IsZero() ||
        math.IsNaN(event.Value) || math.IsInf(event.Value, 0) {
        return nil, false, ErrInvalidEvent
    }

    eventTime := event.OccurredAt.UTC()
    if eventTime.After(receivedAt.UTC().Add(a.cfg.MaxFutureSkew)) {
        return nil, false, ErrFutureEvent
    }

    a.mu.Lock()
    defer a.mu.Unlock()
    if a.closed {
        return nil, false, ErrClosed
    }

    if !a.initialized || eventTime.After(a.maxEventTime) {
        a.initialized = true
        a.maxEventTime = eventTime
        candidate := a.maxEventTime.Add(-a.cfg.MaxOutOfOrder)
        if a.watermark.IsZero() || candidate.After(a.watermark) {
            a.watermark = candidate
        }
    }

    // Сначала материализовать уже закрываемые окна. Даже late event может
    // одновременно принести caller новые готовые результаты.
    closed = a.closeReadyLocked()

    start := eventTime.Truncate(a.cfg.WindowSize)
    end := start.Add(a.cfg.WindowSize)
    if !end.After(a.watermark) {
        return closed, true, nil
    }

    key := windowKey{key: event.Key, start: start}
    state := a.windows[key]
    if state.count == math.MaxInt64 {
        return closed, false, ErrInvalidEvent
    }
    nextSum := state.sum + event.Value
    if math.IsNaN(nextSum) || math.IsInf(nextSum, 0) {
        return closed, false, ErrInvalidEvent
    }
    state.count++
    state.sum = nextSum
    a.windows[key] = state
    return closed, false, nil
}

func (a *Aggregator) Watermark() (time.Time, bool) {
    a.mu.Lock()
    defer a.mu.Unlock()
    return a.watermark, a.initialized
}

// Close завершает bounded input и выдаёт все оставшиеся непустые окна.
func (a *Aggregator) Close() []Result {
    a.mu.Lock()
    defer a.mu.Unlock()
    if a.closed {
        return nil
    }
    a.closed = true

    results := make([]Result, 0, len(a.windows))
    for key, state := range a.windows {
        results = append(results, resultFrom(key, state, a.cfg.WindowSize))
        delete(a.windows, key)
    }
    sortResults(results)
    return results
}

func (a *Aggregator) closeReadyLocked() []Result {
    results := make([]Result, 0)
    for key, state := range a.windows {
        if key.start.Add(a.cfg.WindowSize).After(a.watermark) {
            continue
        }
        results = append(results, resultFrom(key, state, a.cfg.WindowSize))
        delete(a.windows, key)
    }
    sortResults(results)
    return results
}

func resultFrom(key windowKey, state windowState, size time.Duration) Result {
    return Result{
        Key:   key.key,
        Start: key.start,
        End:   key.start.Add(size),
        Count: state.count,
        Sum:   state.sum,
    }
}

func sortResults(results []Result) {
    sort.Slice(results, func(i, j int) bool {
        if results[i].Start.Equal(results[j].Start) {
            return results[i].Key < results[j].Key
        }
        return results[i].Start.Before(results[j].Start)
    })
}
```

Сложность `Add` здесь не строго `O(1)`: при продвижении watermark метод сканирует
все открытые windows. Для большого state используют min-heap/timing wheel по
`window end`, чтобы находить готовые окна без полного scan. `map` остаётся
полезным interview baseline, потому что явно показывает семантику.

Память пропорциональна числу `(key, open window)`. Даже при исправном watermark
неограниченная cardinality ключей создаёт OOM, поэтому production-варианту нужны
лимит/admission policy и метрика размера state.

Результат, возвращённый из `Add`, ещё нужно надёжно записать. Если процесс
закоммитит input offset до sink write, окно может потеряться; если запишет sink и
упадёт до offset commit, возможна повторная выдача. Нужны checkpoint/transaction
или idempotent upsert с ключом `(stream, key, window_start)`.

---

## Несколько partitions

Watermark считают отдельно для каждого partition/split. Безопасный общий прогресс
равен минимуму watermarks активных inputs:

```text
global_watermark = min(partition_watermark[active partitions])
```

Использовать `max` нельзя: быстрый partition тогда преждевременно закроет окна
медленного. Обратная проблема — idle partition бесконечно удерживает минимум.
Его можно пометить idle после timeout и временно исключить, но при возобновлении
его старые события окажутся late. Это осознанный trade-off latency против
полноты.

Если один partition сильно опережает другие, его state может расти. Watermark
alignment или pause/resume быстрых inputs ограничивает drift, но добавляет
backpressure источнику.

---

## Allowed lateness и обновление результатов

`MaxOutOfOrder` в примере задерживает watermark. Allowed lateness — отдельная
policy после первого закрытия окна:

- **Drop:** результат final, late event учитывается только в метрике.
- **Side output:** late event отправляется в отдельный topic/DLQ для анализа или
  backfill.
- **Grace period:** state хранится до `watermark >= end + allowedLateness`, а
  позднее событие вызывает updated result.
- **Retraction/correction:** downstream получает новую версию или разницу со
  старой.

Для updates нужен стабильный ключ и version, например
`(tenant, window_start, revision)`. Обычный `INSERT` без idempotency создаст
дубликаты при retry.

Чем больше allowed lateness, тем дольше state retention и позже действительно
final result. Бесконечное исправление истории несовместимо с bounded memory.

---

## Тесты

```go
func testAggregator(t *testing.T) *Aggregator {
    t.Helper()
    agg, err := New(Config{
        WindowSize:    time.Minute,
        MaxOutOfOrder: 10 * time.Second,
        MaxFutureSkew: 5 * time.Minute,
    })
    if err != nil {
        t.Fatal(err)
    }
    return agg
}

func TestAggregator_OutOfOrderAndLateEvent(t *testing.T) {
    agg := testAggregator(t)
    receivedAt := time.Date(2026, 1, 1, 12, 2, 0, 0, time.UTC)

    closed, late, err := agg.Add(Event{
        Key: "checkout", Value: 10,
        OccurredAt: time.Date(2026, 1, 1, 12, 0, 50, 0, time.UTC),
    }, receivedAt)
    if err != nil || late || len(closed) != 0 {
        t.Fatalf("first Add: closed=%v late=%v err=%v", closed, late, err)
    }

    // max event time=12:01:15, watermark=12:01:05: первое окно закрывается.
    closed, late, err = agg.Add(Event{
        Key: "checkout", Value: 20,
        OccurredAt: time.Date(2026, 1, 1, 12, 1, 15, 0, time.UTC),
    }, receivedAt)
    if err != nil || late || len(closed) != 1 {
        t.Fatalf("second Add: closed=%v late=%v err=%v", closed, late, err)
    }
    if closed[0].Count != 1 || closed[0].Sum != 10 {
        t.Fatalf("closed result = %+v", closed[0])
    }

    // Окно [12:00, 12:01) уже закрыто.
    _, late, err = agg.Add(Event{
        Key: "checkout", Value: 30,
        OccurredAt: time.Date(2026, 1, 1, 12, 0, 59, 0, time.UTC),
    }, receivedAt)
    if err != nil || !late {
        t.Fatalf("late Add: late=%v err=%v", late, err)
    }
}

func TestAggregator_TimestampBeforeWatermarkCanBelongToOpenWindow(t *testing.T) {
    agg := testAggregator(t)
    receivedAt := time.Date(2026, 1, 1, 12, 2, 0, 0, time.UTC)

    _, _, _ = agg.Add(Event{
        Key: "k", Value: 1,
        OccurredAt: time.Date(2026, 1, 1, 12, 1, 15, 0, time.UTC),
    }, receivedAt)

    // watermark=12:01:05, eventTime=12:01:02, но окно заканчивается в 12:02.
    _, late, err := agg.Add(Event{
        Key: "k", Value: 2,
        OccurredAt: time.Date(2026, 1, 1, 12, 1, 2, 0, time.UTC),
    }, receivedAt)
    if err != nil || late {
        t.Fatalf("event in open window: late=%v err=%v", late, err)
    }

    remaining := agg.Close()
    if len(remaining) != 1 || remaining[0].Count != 2 {
        t.Fatalf("remaining = %+v", remaining)
    }
}

func TestAggregator_RejectsFutureTimestampWithoutAdvancingWatermark(t *testing.T) {
    agg := testAggregator(t)
    receivedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
    _, _, err := agg.Add(Event{
        Key: "k", Value: 1,
        OccurredAt: receivedAt.Add(10 * time.Minute),
    }, receivedAt)
    if !errors.Is(err, ErrFutureEvent) {
        t.Fatalf("Add error = %v", err)
    }
    if _, initialized := agg.Watermark(); initialized {
        t.Fatal("rejected event advanced watermark")
    }
}
```

---

## Подводные камни

### 1. Считать `time.Now` event time

Такой код группирует по времени обработки и меняет результат при lag/replay.
Event timestamp должен приходить из определённого поля/metadata.

### 2. Закрывать окно по максимальному timestamp

Самый новый event не доказывает, что более старых уже не будет. Нужен
out-of-order budget или explicit watermark от source.

### 3. Смешивать watermark lag и allowed lateness

Первое задерживает первоначальное закрытие, второе разрешает обновлять уже
выданный результат. Это разные state/latency contracts.

### 4. Брать максимум partition watermarks

Это закрывает окна быстрее самого медленного input и превращает нормальные
события этого input в late. Для совместного оператора нужен минимум активных.

### 5. Не обрабатывать idle input

Watermark, вычисляемый только из events, не продвигается во время тишины; в
multi-input операторе минимум также удерживает idle partition. Idle timeout или
explicit source watermark решает stall ценой того, что возобновившийся старый
поток может стать late.

### 6. Timestamp далеко в будущем

Ошибка часов одного producer может резко продвинуть watermark и закрыть все
окна. Нужен max future skew, quarantine и метрика clock skew.

### 7. Забыть границы `[start, end)`

Event ровно в `end` относится к следующему окну. Inclusive обеих границ либо
дублирует событие, либо создаёт неоднозначность.

### 8. Emit без checkpoint

State, sink result и input offset образуют согласованный протокол. Иначе crash
создаёт потерю или дубль результата.

### 9. Watermark идёт назад

Watermark должен быть monotonic. Correction clock/event не должна уменьшать уже
объявленный прогресс.

### 10. Бесконечно хранить закрытые окна

Allowed lateness без конечного cleanup превращает bounded streaming state в
полную историю.

---

## Interview-ready answer

**1. Чем event time отличается от processing time?**

- Event time — время возникновения у источника; processing time — время работы
  оператора.
- Стабильность — replay/lag меняет processing-time результат, но не должен менять
  event-time window.
- Цена — event time требует работы с out-of-order и late events.

**2. Что такое watermark?**

- Суть — монотонная оценка прогресса event time.
- Закрытие — окно `[start,end)` можно первоначально закрыть при `watermark >= end`.
- Trade-off — чем больше допустимый out-of-order lag, тем выше полнота, но больше latency и
  state.

**3. Как объединять watermarks partitions?**

- Объединение — брать минимум активных inputs.
- Idleness — исключать input только по выбранной timeout policy.
- Alignment — быстрые inputs при необходимости замедлять backpressure.

**4. Что делать с late events?**

- Варианты — drop с метрикой, side output, grace period или correction/retraction.
- Downstream — update должен быть идемпотентным и versioned.
- Retention — state удаляется после конечной allowed-lateness границы.

---

## Связанные материалы

- [Streaming Aggregation](./03-streaming-aggregation.md)
- [Deduplication](./01-deduplication.md)
- [Backpressure](./04-backpressure.md)
- [Apache Flink: event-time windows](https://nightlies.apache.org/flink/flink-docs-stable/docs/dev/datastream/operators/windows/)
