# Задача 2: Retry с Exponential Backoff

Retry с правильным backoff — must для любого надёжного клиента. Спрашивают на собеседованиях для понимания: что retry'ить, как retry'ить, и **что НЕ retry'ить**.

## Формулировка

> "Реализуй функцию которая retry'ит вызов при transient ошибке. Используй exponential backoff с jitter."

Вариации:
- "HTTP client с retry"
- "Идемпотентная retry-обёртка"
- "Resilient API client"

---

## Уточняющие вопросы

1. **Что считать retryable?**
   "Transient: network timeout, 502/503/504, rate limit (429), DB connection lost. Non-retryable: 400/401/403, validation errors, ctx cancelled."

2. **Сколько попыток?**
   "5 — типичный максимум. Больше — только усугубляет проблему."

3. **Backoff strategy — exponential? fixed?**
   "Exponential обычно. Fixed — для предсказуемых задержек."

4. **Jitter обязательно?**
   "Да — иначе thundering herd при общем failure."

5. **Идемпотентность гарантируется caller'ом?**
   "Должен — retry неидемпотентного создаёт дубли."

6. **Total timeout?**
   "Через context.WithTimeout. Retry не должен превышать."

---

## Базовое решение

```go
package retry

import (
    "context"
    "errors"
    "math/rand"
    "time"
)

type Config struct {
    MaxAttempts int           // максимум попыток (включая первую)
    BaseDelay   time.Duration // 100ms типично
    MaxDelay    time.Duration // 30s типично — ограничивает рост exponential
    Multiplier  float64       // 2.0 обычно
    Jitter      float64       // 0.0-1.0, доля randomness
}

func DefaultConfig() Config {
    return Config{
        MaxAttempts: 5,
        BaseDelay:   100 * time.Millisecond,
        MaxDelay:    30 * time.Second,
        Multiplier:  2.0,
        Jitter:      0.5,
    }
}

// RetryableError помечает что ошибка retryable.
type RetryableError struct {
    Err error
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// Retryable оборачивает error в RetryableError.
func Retryable(err error) error {
    if err == nil {
        return nil
    }
    return &RetryableError{Err: err}
}

// Do выполняет fn с retry при retryable ошибках.
func Do(ctx context.Context, cfg Config, fn func(ctx context.Context) error) error {
    var lastErr error
    delay := cfg.BaseDelay

    for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
        // Первая попытка без задержки
        if attempt > 0 {
            // Apply jitter
            jitteredDelay := applyJitter(delay, cfg.Jitter)

            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(jitteredDelay):
            }

            // Increase delay для следующего раза (capped)
            delay = time.Duration(float64(delay) * cfg.Multiplier)
            if delay > cfg.MaxDelay {
                delay = cfg.MaxDelay
            }
        }

        err := fn(ctx)
        if err == nil {
            return nil
        }

        // Non-retryable error — выходим сразу
        var retryable *RetryableError
        if !errors.As(err, &retryable) {
            return err
        }

        lastErr = err
    }

    return lastErr
}

// applyJitter добавляет случайность к delay (full jitter — самый безопасный).
func applyJitter(delay time.Duration, jitter float64) time.Duration {
    if jitter <= 0 {
        return delay
    }
    // Full jitter: [0, delay*jitter] добавляется к (delay * (1-jitter))
    base := float64(delay) * (1 - jitter)
    random := rand.Float64() * float64(delay) * jitter
    return time.Duration(base + random)
}
```

**Использование:**

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
defer cancel()

err := retry.Do(ctx, retry.DefaultConfig(), func(ctx context.Context) error {
    resp, err := httpClient.Do(req)
    if err != nil {
        // Network error — retry
        return retry.Retryable(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 500 {
        return retry.Retryable(fmt.Errorf("server error: %d", resp.StatusCode))
    }
    if resp.StatusCode == 429 {
        return retry.Retryable(errors.New("rate limited"))
    }
    if resp.StatusCode >= 400 {
        // 4xx (кроме 429) — не retry, это ошибка клиента
        return fmt.Errorf("client error: %d", resp.StatusCode)
    }

    // Success
    return nil
})
```

**Что важно:**
- **Marker error type** (`RetryableError`) — caller явно говорит "это retryable"
- **`time.After`** + `ctx.Done()` — interruptible sleep
- **Jitter** — обязательно, иначе все клиенты ретраят одновременно (thundering herd)
- **MaxDelay cap** — exponential без ограничения уйдёт в секунды/минуты

---

## Backoff стратегии

### Fixed delay

```
attempt: 1  2  3  4  5
delay:   1s 1s 1s 1s 1s
```

Просто, предсказуемо. Не подходит когда зависимость нагружена — все retry одновременно.

### Linear

```
delay = base * attempt
attempt: 1  2  3  4  5
delay:   1s 2s 3s 4s 5s
```

Полусерьёзно используется. Лучше fixed, хуже exponential.

### Exponential

```
delay = base * multiplier^(attempt-1)
attempt: 1   2   3   4    5
delay:   1s  2s  4s  8s   16s
```

**Стандарт для retry.** Быстро даёт зависимости время восстановиться.

### Exponential with jitter (Full Jitter)

```
delay = random(0, base * multiplier^(attempt-1))
```

Используется AWS, Google. **Безопаснее всего** — равномерно распределяет retry'и во времени, никаких "коллективных всплесков" после общего failure.

### Decorrelated Jitter

```
delay = random(base, prev_delay * 3)
```

Иногда лучше Full Jitter — баланс между равномерностью и быстрым backoff. См. [AWS blog post](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/).

---

## Production-grade с metrics

```go
package retry

import (
    "context"
    "errors"
    "math"
    "math/rand"
    "time"
)

type Metrics interface {
    RecordAttempt(attempt int)
    RecordRetry(attempt int)
    RecordSuccess(attempt int)
    RecordFailure(err error, attempt int)
    RecordDuration(d time.Duration)
}

type Retrier struct {
    cfg     Config
    metrics Metrics
}

func New(cfg Config, m Metrics) *Retrier {
    return &Retrier{cfg: cfg, metrics: m}
}

// Do — generic version. Возвращает result типа T.
func Do[T any](
    ctx context.Context,
    r *Retrier,
    fn func(ctx context.Context) (T, error),
) (T, error) {
    var (
        zero    T
        lastErr error
    )
    start := time.Now()
    defer func() {
        if r.metrics != nil {
            r.metrics.RecordDuration(time.Since(start))
        }
    }()

    for attempt := 1; attempt <= r.cfg.MaxAttempts; attempt++ {
        if r.metrics != nil {
            r.metrics.RecordAttempt(attempt)
        }

        if attempt > 1 {
            delay := r.computeDelay(attempt)
            if r.metrics != nil {
                r.metrics.RecordRetry(attempt)
            }
            select {
            case <-ctx.Done():
                return zero, ctx.Err()
            case <-time.After(delay):
            }
        }

        result, err := fn(ctx)
        if err == nil {
            if r.metrics != nil {
                r.metrics.RecordSuccess(attempt)
            }
            return result, nil
        }

        // Context cancelled — выходим сразу, не retry
        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
            return zero, err
        }

        var retryable *RetryableError
        if !errors.As(err, &retryable) {
            if r.metrics != nil {
                r.metrics.RecordFailure(err, attempt)
            }
            return zero, err
        }

        lastErr = err
    }

    if r.metrics != nil {
        r.metrics.RecordFailure(lastErr, r.cfg.MaxAttempts)
    }
    return zero, lastErr
}

func (r *Retrier) computeDelay(attempt int) time.Duration {
    // attempt = 2 (first retry), 3, 4, 5...
    backoff := float64(r.cfg.BaseDelay) * math.Pow(r.cfg.Multiplier, float64(attempt-2))
    if backoff > float64(r.cfg.MaxDelay) {
        backoff = float64(r.cfg.MaxDelay)
    }

    // Full jitter
    return time.Duration(rand.Float64() * backoff)
}
```

**Использование с generics:**

```go
type APIResponse struct {
    Data []byte
}

resp, err := retry.Do(ctx, retrier, func(ctx context.Context) (APIResponse, error) {
    return callAPI(ctx)
})
```

---

## Smart retry: учитывать Retry-After header

HTTP 429/503 часто содержат `Retry-After` header — сервер говорит когда retry.

```go
func parseRetryAfter(resp *http.Response) (time.Duration, bool) {
    val := resp.Header.Get("Retry-After")
    if val == "" {
        return 0, false
    }

    // Может быть число секунд: "120"
    if seconds, err := strconv.Atoi(val); err == nil {
        return time.Duration(seconds) * time.Second, true
    }

    // Или HTTP date: "Wed, 21 Oct 2025 07:28:00 GMT"
    if t, err := http.ParseTime(val); err == nil {
        return time.Until(t), true
    }

    return 0, false
}

// В retry — если сервер дал hint, использовать
func httpDo(ctx context.Context, req *http.Request) error {
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return retry.Retryable(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode == 429 || resp.StatusCode == 503 {
        if delay, ok := parseRetryAfter(resp); ok {
            // Сервер сказал когда retry — wait и retry
            time.Sleep(delay)
            return retry.Retryable(fmt.Errorf("server overloaded: %d", resp.StatusCode))
        }
        return retry.Retryable(fmt.Errorf("server error: %d", resp.StatusCode))
    }
    // ...
}
```

---

## Тесты

```go
func TestRetry_SucceedsAfterFailures(t *testing.T) {
    var attempts atomic.Int32
    cfg := Config{
        MaxAttempts: 5,
        BaseDelay:   time.Millisecond,
        Multiplier:  2,
        MaxDelay:    100 * time.Millisecond,
    }

    err := Do(context.Background(), cfg, func(ctx context.Context) error {
        n := attempts.Add(1)
        if n < 3 {
            return Retryable(errors.New("fail"))
        }
        return nil
    })

    if err != nil {
        t.Fatal(err)
    }
    if attempts.Load() != 3 {
        t.Errorf("attempts %d, want 3", attempts.Load())
    }
}

func TestRetry_GivesUpAfterMax(t *testing.T) {
    var attempts atomic.Int32
    cfg := Config{
        MaxAttempts: 3,
        BaseDelay:   time.Millisecond,
    }

    err := Do(context.Background(), cfg, func(ctx context.Context) error {
        attempts.Add(1)
        return Retryable(errors.New("fail"))
    })

    if err == nil {
        t.Fatal("expected error")
    }
    if attempts.Load() != 3 {
        t.Errorf("attempts %d, want 3", attempts.Load())
    }
}

func TestRetry_NonRetryable(t *testing.T) {
    var attempts atomic.Int32
    cfg := DefaultConfig()
    cfg.BaseDelay = time.Millisecond

    nonRetryable := errors.New("validation failed")
    err := Do(context.Background(), cfg, func(ctx context.Context) error {
        attempts.Add(1)
        return nonRetryable  // NOT wrapped в Retryable
    })

    if !errors.Is(err, nonRetryable) {
        t.Errorf("got %v, want %v", err, nonRetryable)
    }
    if attempts.Load() != 1 {
        t.Errorf("attempts %d, want 1", attempts.Load())
    }
}

func TestRetry_ContextCancelled(t *testing.T) {
    cfg := Config{
        MaxAttempts: 100,
        BaseDelay:   time.Second,
    }

    ctx, cancel := context.WithCancel(context.Background())
    go func() {
        time.Sleep(50 * time.Millisecond)
        cancel()
    }()

    err := Do(ctx, cfg, func(ctx context.Context) error {
        return Retryable(errors.New("fail"))
    })

    if !errors.Is(err, context.Canceled) {
        t.Errorf("got %v, want Canceled", err)
    }
}

func TestRetry_ExponentialBackoff(t *testing.T) {
    cfg := Config{
        MaxAttempts: 4,
        BaseDelay:   10 * time.Millisecond,
        Multiplier:  2,
        Jitter:      0,  // detect predictable
    }

    var times []time.Time
    Do(context.Background(), cfg, func(ctx context.Context) error {
        times = append(times, time.Now())
        return Retryable(errors.New("fail"))
    })

    // delays между attempts должны расти: ~10ms, 20ms, 40ms
    for i := 1; i < len(times); i++ {
        d := times[i].Sub(times[i-1])
        expected := time.Duration(10*math.Pow(2, float64(i-1))) * time.Millisecond
        if d < expected*8/10 || d > expected*15/10 {
            t.Errorf("attempt %d delay %v, expected ~%v", i, d, expected)
        }
    }
}
```

---

## Подводные камни

### 1. Retry для non-idempotent операций

```go
// ❌ POST /orders — создание заказа
retry.Do(ctx, cfg, func(ctx context.Context) error {
    return createOrder(ctx)  // ← дубли при retry!
})
```

POST/PATCH/DELETE — обычно non-idempotent. Retry без idempotency-key создаёт дубликаты. См. [05-idempotency-handler.md](./05-idempotency-handler.md).

### 2. Retry на 4xx

```go
// ❌ 401, 403, 400 — НЕ retry
if resp.StatusCode >= 400 {
    return retry.Retryable(err)
}
```

4xx — ошибка **клиента**. Retry не поможет. Только 429 (rate limit) — retryable из 4xx.

### 3. Retry без context

```go
// ❌ Бесконечный wait
time.Sleep(delay)
```

Должно быть `select { case <-ctx.Done(): ... case <-time.After(delay): }`.

### 4. Retry без jitter — thundering herd

```go
// ❌ 1000 клиентов retry'ят одновременно
time.Sleep(exponential(attempt))
```

Зависимость только-только начала восстанавливаться → бомбардировка → опять падает. Jitter обязателен.

### 5. Retry амплифицирует нагрузку

Зависимость в стрессе → клиенты массово retry → ещё больше нагрузка → больше failures → больше retry.

**Решение:** circuit breaker (см. [03-circuit-breaker.md](./03-circuit-breaker.md)) или token bucket для retry'ев.

### 6. Не учитывать ctx.Done в самом fn

```go
fn := func(ctx context.Context) error {
    resp, err := http.Get(url)  // ← ctx не передан!
    // ...
}
```

Длинная operation продолжается после ctx cancelled.

### 7. Логировать каждую попытку как error

```
ERROR: retry attempt 1 failed
ERROR: retry attempt 2 failed
ERROR: retry attempt 3 failed
ERROR: retry attempt 4 failed
ERROR: retry attempt 5 failed
```

5 error логов на одну операцию. Лучше WARN на промежуточные и ERROR на final.

### 8. Слишком много попыток

```go
MaxAttempts: 50  // ← на сценарии "запрос упал" будет минуты ждать
```

5-7 — практичный max. После — alert / circuit breaker.

### 9. Не сохранять оригинальную ошибку

```go
// ❌ Только last attempt info
return errors.New("retry exhausted")
```

Caller хочет видеть оригинал → wrap последнюю: `fmt.Errorf("retry exhausted: %w", lastErr)`.

### 10. Retry внутри retry

Service A retries 5 раз → каждый запрос вызывает Service B который retries 5 → 25 retries per logical request. Multiplicative explosion.

**Решение:** retry **только на крайнем слое** (entry point), внутренние сервисы не retry.

---

## Возможные расширения

### 1. Retry budget

Глобальный лимит "сколько retry в секунду" — защита от retry storm.

### 2. Custom retryable predicate

```go
func(err error) bool {
    var httpErr *HTTPError
    return errors.As(err, &httpErr) && httpErr.StatusCode >= 500
}
```

Гибче чем marker error.

### 3. Hedged requests

Для latency-critical — отправить N запросов параллельно, использовать первый успешный. См. Tail at Scale paper Google.

### 4. Retry с different endpoints

При фейле на server-1 — retry на server-2 (round robin).

### 5. Persistent retry queue

Если retry exhausted — положить в DB queue, retry asynchronously позже.

### 6. Adaptive retry

Снизить MaxAttempts когда circuit breaker open / error rate высокий.

---

## Что важно показать на собеседовании

1. **Что retryable, что нет** — 5xx/429/network yes, 4xx no
2. **Exponential backoff с jitter** — почему jitter обязателен (thundering herd)
3. **MaxDelay cap** — не уходить в minutes
4. **Context propagation** — interruptible sleep
5. **Идемпотентность** — caller responsibility
6. **Retry budget / circuit breaker** — защита от amplification
7. **Retry-After header** — respect сервера
8. **Стандартные библиотеки** — cenkalti/backoff, avast/retry-go

## Связки

- [Retries и backoff (reliability)](../../../05-system-design/reliability-patterns/02-retries-and-backoff.md)
- [Circuit Breaker](./03-circuit-breaker.md) — защита от retry storm
- [Idempotency](./05-idempotency-handler.md) — обязательно для retry'ев на mutations
- [AWS blog: Exponential Backoff and Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
