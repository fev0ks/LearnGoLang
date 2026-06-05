# Задача 3: Circuit Breaker

Circuit breaker — паттерн для защиты от **cascade failure**. Когда зависимость падает, не продолжать слать ей запросы — она и так не справляется. Дать ей время восстановиться.

Аналогия — электрический предохранитель: при коротком замыкании размыкает цепь, не даёт сгореть всему.

## Формулировка

> "Реализуй circuit breaker — защиту от cascade failure. Состояния: closed (normal), open (failing), half-open (probing recovery)."

Вариации:
- "Resilient HTTP client с CB"
- "Микросервисная resilience"
- "Что делать при downstream failure?"

---

## Уточняющие вопросы

1. **Что считать failure?**
   "Errors от fn. Иногда — timeout, slow responses (latency > threshold)."

2. **Когда открывать circuit?**
   "Threshold: 50% errors за last 10 requests или 5 errors подряд."

3. **Когда пробовать recovery?**
   "Half-open после N секунд в open."

4. **Что в half-open — один запрос или несколько?**
   "Обычно один. Если успех — close. Если fail — back to open."

5. **Per-target или global?**
   "Per-target (per-host или per-service). Не один глобальный."

6. **Sliding window для error rate?**
   "Да — иначе старые failures навечно держат circuit open."

---

## Теория: три состояния

```
                     fail rate > threshold
        ┌───────────────────────────────────┐
        ▼                                   │
   ┌─────────┐    timeout    ┌──────────────┐
   │  CLOSED │ ──────────────│   OPEN       │
   │         │               │              │
   │ normal  │               │  reject all  │
   │ traffic │               │  immediately │
   └─────────┘               └──────────────┘
        ▲                            │
        │ success                    │ retry timer expires
        │                            ▼
        │                  ┌─────────────────┐
        │                  │   HALF-OPEN     │
        │ ◄────────────────│                 │
        │                  │ probe request   │
        │  fail            │                 │
        └──────────────────┘─────────────────┘
                              ▲
                              │ fail → back to OPEN
```

**Closed (normal):**
- Все запросы проходят
- Считается success/failure rate
- Если rate ошибок превышает threshold → переход в Open

**Open (failing):**
- Все запросы **сразу** возвращают error (fast fail)
- Не нагружаем downstream
- Через `recoveryTimeout` → Half-open

**Half-open (probing):**
- Пропустить один (или несколько) запросов "на пробу"
- Если успех → Closed
- Если fail → обратно Open

---

## Базовое решение

```go
package circuitbreaker

import (
    "context"
    "errors"
    "sync"
    "time"
)

type State int

const (
    StateClosed State = iota
    StateOpen
    StateHalfOpen
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type Config struct {
    MaxFailures      int           // failures для перехода в Open
    RecoveryTimeout  time.Duration // как долго ждать в Open перед Half-open
    SuccessThreshold int           // успехов в Half-open для перехода в Closed
}

type Breaker struct {
    cfg Config

    mu                 sync.Mutex
    state              State
    failures           int
    halfOpenSuccesses  int
    lastFailureTime    time.Time
}

func New(cfg Config) *Breaker {
    if cfg.MaxFailures <= 0 {
        cfg.MaxFailures = 5
    }
    if cfg.RecoveryTimeout <= 0 {
        cfg.RecoveryTimeout = 30 * time.Second
    }
    if cfg.SuccessThreshold <= 0 {
        cfg.SuccessThreshold = 1
    }
    return &Breaker{cfg: cfg, state: StateClosed}
}

// Do выполняет fn если circuit разрешает, иначе ErrCircuitOpen.
func (b *Breaker) Do(ctx context.Context, fn func(ctx context.Context) error) error {
    if err := b.checkState(); err != nil {
        return err
    }

    err := fn(ctx)
    b.recordResult(err)
    return err
}

func (b *Breaker) checkState() error {
    b.mu.Lock()
    defer b.mu.Unlock()

    switch b.state {
    case StateClosed:
        return nil  // OK to proceed
    case StateOpen:
        // Проверить, не время ли probe'нуть
        if time.Since(b.lastFailureTime) > b.cfg.RecoveryTimeout {
            b.state = StateHalfOpen
            b.halfOpenSuccesses = 0
            return nil
        }
        return ErrCircuitOpen
    case StateHalfOpen:
        return nil  // probe — let it through
    }
    return nil
}

func (b *Breaker) recordResult(err error) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if err != nil {
        // Failure
        b.failures++
        b.lastFailureTime = time.Now()

        switch b.state {
        case StateClosed:
            if b.failures >= b.cfg.MaxFailures {
                b.state = StateOpen
            }
        case StateHalfOpen:
            // Fail в half-open → обратно в Open
            b.state = StateOpen
            b.halfOpenSuccesses = 0
        }
    } else {
        // Success
        switch b.state {
        case StateClosed:
            b.failures = 0  // reset на success
        case StateHalfOpen:
            b.halfOpenSuccesses++
            if b.halfOpenSuccesses >= b.cfg.SuccessThreshold {
                b.state = StateClosed
                b.failures = 0
            }
        }
    }
}

func (b *Breaker) State() State {
    b.mu.Lock()
    defer b.mu.Unlock()
    return b.state
}
```

**Использование:**

```go
cb := circuitbreaker.New(circuitbreaker.Config{
    MaxFailures:      5,
    RecoveryTimeout:  30 * time.Second,
    SuccessThreshold: 2,
})

err := cb.Do(ctx, func(ctx context.Context) error {
    return callExternalAPI(ctx)
})
if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
    // Fast fail — circuit open, не дёргать API
    return fallbackResponse()
}
```

**Что важно:**
- **Fast fail в Open** — не делать запрос вообще, сразу error
- **Single mutex** — простая синхронизация (для high QPS подумай о sharding)
- **Recovery через time** — после `RecoveryTimeout` дать шанс
- **Half-open для probe** — не возвращаемся в Closed без подтверждения

---

## Production-grade: sliding window + concurrency control

Базовое решение считает **подряд идущие** failures. В production обычно используют **error rate за окно времени**.

```go
package circuitbreaker

import (
    "context"
    "errors"
    "sync"
    "sync/atomic"
    "time"
)

type bucket struct {
    successes atomic.Int64
    failures  atomic.Int64
}

type Config struct {
    // Sliding window
    WindowSize     time.Duration // окно для подсчёта error rate (e.g., 10s)
    BucketCount    int           // bucket'ов в окне (e.g., 10)

    // Thresholds
    MinRequests        int     // минимум запросов в окне для решения
    FailureRateThreshold float64 // 0.5 = 50% — после порога открывается

    // Recovery
    RecoveryTimeout   time.Duration // как долго в Open
    HalfOpenMaxCalls  int           // сколько concurrent в half-open
}

type Breaker struct {
    cfg     Config

    state           atomic.Int32  // State as int32 for atomic
    mu              sync.Mutex
    buckets         []*bucket
    currentBucket   int
    lastBucketTime  time.Time
    openedAt        time.Time
    halfOpenCalls   atomic.Int32
}

func New(cfg Config) *Breaker {
    if cfg.BucketCount <= 0 {
        cfg.BucketCount = 10
    }
    if cfg.MinRequests <= 0 {
        cfg.MinRequests = 10
    }
    if cfg.FailureRateThreshold <= 0 {
        cfg.FailureRateThreshold = 0.5
    }

    buckets := make([]*bucket, cfg.BucketCount)
    for i := range buckets {
        buckets[i] = &bucket{}
    }

    return &Breaker{
        cfg:            cfg,
        buckets:        buckets,
        lastBucketTime: time.Now(),
    }
}

func (b *Breaker) Do(ctx context.Context, fn func(ctx context.Context) error) error {
    state := State(b.state.Load())

    switch state {
    case StateOpen:
        // Проверить — не время ли пробовать recovery
        b.mu.Lock()
        if time.Since(b.openedAt) > b.cfg.RecoveryTimeout {
            b.state.Store(int32(StateHalfOpen))
            state = StateHalfOpen
        }
        b.mu.Unlock()

        if state == StateOpen {
            return ErrCircuitOpen
        }
        fallthrough

    case StateHalfOpen:
        // Limited concurrent calls в half-open
        if b.halfOpenCalls.Add(1) > int32(b.cfg.HalfOpenMaxCalls) {
            b.halfOpenCalls.Add(-1)
            return ErrCircuitOpen
        }
        defer b.halfOpenCalls.Add(-1)
    }

    err := fn(ctx)
    b.recordResult(err == nil)
    return err
}

func (b *Breaker) recordResult(success bool) {
    bucket := b.currentBucketAtomic()
    if success {
        bucket.successes.Add(1)
    } else {
        bucket.failures.Add(1)
    }

    // Решение об изменении state
    state := State(b.state.Load())
    successes, failures := b.windowStats()
    total := successes + failures

    if total < int64(b.cfg.MinRequests) {
        return  // мало данных для решения
    }

    failureRate := float64(failures) / float64(total)

    b.mu.Lock()
    defer b.mu.Unlock()

    switch state {
    case StateClosed:
        if failureRate >= b.cfg.FailureRateThreshold {
            b.state.Store(int32(StateOpen))
            b.openedAt = time.Now()
        }
    case StateHalfOpen:
        if !success {
            // Любая ошибка в half-open → обратно в Open
            b.state.Store(int32(StateOpen))
            b.openedAt = time.Now()
        } else if failureRate < b.cfg.FailureRateThreshold {
            // Recovery подтверждено
            b.state.Store(int32(StateClosed))
        }
    }
}

func (b *Breaker) currentBucketAtomic() *bucket {
    b.mu.Lock()
    defer b.mu.Unlock()

    now := time.Now()
    bucketDur := b.cfg.WindowSize / time.Duration(b.cfg.BucketCount)
    elapsed := now.Sub(b.lastBucketTime)

    if elapsed >= bucketDur {
        // Перейти к новому bucket'у (возможно несколько)
        shift := int(elapsed / bucketDur)
        if shift > b.cfg.BucketCount {
            shift = b.cfg.BucketCount
        }
        for i := 0; i < shift; i++ {
            b.currentBucket = (b.currentBucket + 1) % b.cfg.BucketCount
            b.buckets[b.currentBucket].successes.Store(0)
            b.buckets[b.currentBucket].failures.Store(0)
        }
        b.lastBucketTime = b.lastBucketTime.Add(time.Duration(shift) * bucketDur)
    }

    return b.buckets[b.currentBucket]
}

func (b *Breaker) windowStats() (successes, failures int64) {
    for _, bucket := range b.buckets {
        successes += bucket.successes.Load()
        failures += bucket.failures.Load()
    }
    return
}
```

**Что улучшено:**
- **Sliding window** через rotating buckets — старые failures expire'ятся
- **Error rate** вместо подряд идущих failures
- **MinRequests** — не решать при малом sample'е
- **HalfOpenMaxCalls** — не позволять flood'ить в half-open
- **Atomic operations** на hot path — меньше lock contention

---

## Использование готовых библиотек

Не пиши свой в production — используй:
- **[sony/gobreaker](https://github.com/sony/gobreaker)** — простой и популярный
- **[afex/hystrix-go](https://github.com/afex/hystrix-go)** — Netflix Hystrix port (legacy но full-featured)
- **[mercari/go-circuitbreaker](https://github.com/mercari/go-circuitbreaker)** — современная альтернатива

```go
import "github.com/sony/gobreaker"

cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "api-call",
    MaxRequests: 5,
    Interval:    10 * time.Second,
    Timeout:     30 * time.Second,
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
        return counts.Requests >= 3 && failureRatio >= 0.6
    },
    OnStateChange: func(name string, from, to gobreaker.State) {
        log.Printf("circuit %s: %s -> %s", name, from, to)
    },
})

result, err := cb.Execute(func() (any, error) {
    return callAPI()
})
```

---

## Тесты

```go
func TestBreaker_OpensAfterFailures(t *testing.T) {
    cb := New(Config{MaxFailures: 3, RecoveryTimeout: time.Second})

    for i := 0; i < 3; i++ {
        cb.Do(context.Background(), func(ctx context.Context) error {
            return errors.New("fail")
        })
    }

    if cb.State() != StateOpen {
        t.Errorf("state %v, want Open", cb.State())
    }

    // Следующий call должен сразу fail (без вызова fn)
    var called bool
    err := cb.Do(context.Background(), func(ctx context.Context) error {
        called = true
        return nil
    })
    if !errors.Is(err, ErrCircuitOpen) {
        t.Errorf("got %v, want ErrCircuitOpen", err)
    }
    if called {
        t.Error("fn was called when circuit open")
    }
}

func TestBreaker_RecoveryAfterTimeout(t *testing.T) {
    cb := New(Config{
        MaxFailures:      2,
        RecoveryTimeout:  50 * time.Millisecond,
        SuccessThreshold: 1,
    })

    // Open the circuit
    cb.Do(context.Background(), func(ctx context.Context) error {
        return errors.New("fail")
    })
    cb.Do(context.Background(), func(ctx context.Context) error {
        return errors.New("fail")
    })

    time.Sleep(60 * time.Millisecond)

    // Should be half-open now — let probe through
    err := cb.Do(context.Background(), func(ctx context.Context) error {
        return nil  // success
    })
    if err != nil {
        t.Fatal(err)
    }
    if cb.State() != StateClosed {
        t.Errorf("state %v, want Closed", cb.State())
    }
}

func TestBreaker_HalfOpenFailRevertsToOpen(t *testing.T) {
    cb := New(Config{
        MaxFailures:     2,
        RecoveryTimeout: 50 * time.Millisecond,
    })

    cb.Do(context.Background(), func(ctx context.Context) error {
        return errors.New("fail")
    })
    cb.Do(context.Background(), func(ctx context.Context) error {
        return errors.New("fail")
    })

    time.Sleep(60 * time.Millisecond)

    // Probe fails → back to Open
    cb.Do(context.Background(), func(ctx context.Context) error {
        return errors.New("still failing")
    })

    if cb.State() != StateOpen {
        t.Errorf("state %v, want Open", cb.State())
    }
}
```

---

## Подводные камни

### 1. CB на 4xx ошибки

```go
// ❌ 404 трактуется как failure
if resp.StatusCode >= 400 {
    return errors.New("error")
}
```

4xx — обычно проблема клиента, не downstream. Открывать circuit на 404 — bug. **Считать failure только 5xx, timeout, network error.**

### 2. Один CB на множество targets

```go
// ❌ Один CB для всех downstream сервисов
cb := New(...)
cb.Do(ctx, callServiceA)
cb.Do(ctx, callServiceB)  // ← если A падает, B тоже fail
```

CB **per-target**. ServiceA, ServiceB — два независимых CB.

### 3. Recovery timeout слишком короткий

```go
RecoveryTimeout: 100 * time.Millisecond
```

Зависимость не успела восстановиться → probe fail → opened again. Эффективно — почти всегда open.

Разумный диапазон — 10-60 секунд.

### 4. Recovery timeout слишком длинный

```go
RecoveryTimeout: 10 * time.Minute
```

Зависимость восстановилась через минуту, но мы её 10 минут не используем. Деградированный сервис.

### 5. No metrics

Без метрик нет видимости когда CB open. Скрытые failures.

Метрики: state changes, failure rate, requests rejected.

### 6. Strict mode в half-open

```go
HalfOpenMaxCalls: 1
```

Один probe, и если он немного медленный — другие запросы fast-fail. На high QPS это значит почти все запросы fail. Лучше 5-10.

### 7. No timeout в Do()

```go
err := cb.Do(ctx, slowOperation)
```

Если `slowOperation` зависнет — CB не знает. Должен быть timeout в самом fn или ctx.

### 8. Reset failures на success в Closed

```go
case StateClosed:
    failures = 0  // reset
```

Sliding window лучше — кратковременный всплеск 5 errors не должен **немедленно** открыть CB если в общем 5 из 1000.

### 9. Synchronous state change под нагрузкой

При 100k RPS все запросы делают mutex.Lock в Do. Bottleneck. Используй atomic state + lock только для transition.

### 10. CB без fallback

```go
err := cb.Do(ctx, fn)
if errors.Is(err, ErrCircuitOpen) {
    return err  // ← а что делать-то?
}
```

При circuit open нужна **fallback strategy**: cached value, default response, degraded UI, queue для async. Просто error → плохой UX.

---

## Возможные расширения

### 1. Slow call detection

Не только errors, но и **medium latency** считать как failure. Если p99 > threshold → circuit open.

### 2. Multiple thresholds

- Soft threshold: 30% failures → warning, mark as "degraded"
- Hard threshold: 60% failures → open

### 3. Per-method CB

В одном сервисе разные endpoints — разная health. POST /payment может failing, GET /list — fine. CB per method.

### 4. CB + retry coordination

Retry должен учитывать CB state. Если open — не retry внутри Do (waste). Если recently transitioned to half-open — give it a shot.

### 5. Hystrix-style thread isolation

Отдельный thread pool на каждый downstream. Если pool exhausted — fail fast. Альтернатива CB.

### 6. Distributed CB

Несколько pod'ов делятся state через Redis. Решение об open принимается глобально на основе агрегированных метрик. Хайповая тема, не часто нужна.

### 7. Manual override

Operator может вручную поставить CB в open (maintenance mode).

---

## Что важно показать на собеседовании

1. **3 состояния** — Closed/Open/Half-open, диаграмма переходов
2. **Sliding window** для error rate, не consecutive failures
3. **Min requests** — не решать при малом sample'е
4. **Fast fail в Open** — не вызывать fn вообще
5. **Half-open для probe** — single или few requests
6. **Per-target, не global** — независимые CB на каждый downstream
7. **Metrics и observability** — критично для production
8. **Fallback strategy** — что делать при open
9. **sony/gobreaker** — production library

## Связки

- [Circuit Breaker (reliability)](../../../05-system-design/reliability-patterns/03-circuit-breaker.md) — теория и use cases
- [Retry с Backoff](./02-retry-with-backoff.md) — CB + retry coordination
- [Bulkhead](../../../05-system-design/reliability-patterns/07-bulkhead.md) — alternative isolation
- [sony/gobreaker](https://github.com/sony/gobreaker) — production library
