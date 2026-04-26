# Retries и Backoff

Повторные попытки — простейший механизм компенсации временных отказов. Без правильной стратегии retry превращается в DDoS на упавший сервис.

## Содержание

- [Когда retry уместен](#когда-retry-уместен)
- [Exponential backoff](#exponential-backoff)
- [Jitter: зачем и какой](#jitter-зачем-и-какой)
- [Retry budget](#retry-budget)
- [Retry amplification в цепочке сервисов](#retry-amplification-в-цепочке-сервисов)
- [Реализация в Go](#реализация-в-go)
- [Антипаттерны](#антипаттерны)

---

## Когда retry уместен

Retry имеет смысл только для **идемпотентных** операций и **retriable** ошибок.

**Retry безопасен:**
- `503 Service Unavailable` — сервис временно перегружен
- `429 Too Many Requests` — rate limit, отступить и попробовать позже
- Network timeout / connection reset — временный сетевой сбой
- `500 Internal Server Error` — **только если операция идемпотентна**

**Retry опасен / бессмысленен:**
- `400 Bad Request` — данные неверные, retry вернёт то же
- `401 Unauthorized` / `403 Forbidden` — права не изменятся сами
- `404 Not Found` — ресурс не появится
- `409 Conflict` — конфликт состояния, нужна бизнес-логика
- Не идемпотентные операции: `POST /payments` без idempotency key — повторный retry создаст дублирующий платёж

```go
func isRetriable(err error) bool {
    var statusErr interface{ StatusCode() int }
    if errors.As(err, &statusErr) {
        switch statusErr.StatusCode() {
        case http.StatusTooManyRequests,
            http.StatusServiceUnavailable,
            http.StatusBadGateway,
            http.StatusGatewayTimeout:
            return true
        }
    }
    // Сетевые ошибки
    var netErr net.Error
    if errors.As(err, &netErr) && netErr.Timeout() {
        return true
    }
    return false
}
```

---

## Exponential backoff

Линейный retry (попробовать немедленно 3 раза) при массовом сбое создаёт всплеск нагрузки точно в момент когда сервис пытается восстановиться.

Exponential backoff увеличивает паузу между попытками экспоненциально:

```
попытка 1: подождать base * 2^0 = 100ms
попытка 2: подождать base * 2^1 = 200ms
попытка 3: подождать base * 2^2 = 400ms
попытка 4: подождать base * 2^3 = 800ms
...
ограничить максимумом: cap = 30s
```

```go
func backoffDuration(attempt int, base, cap time.Duration) time.Duration {
    d := base * (1 << attempt)  // base * 2^attempt
    if d > cap {
        d = cap
    }
    return d
}
```

---

## Jitter: зачем и какой

Без jitter все клиенты которые получили ошибку одновременно будут делать retry одновременно — thundering herd. Jitter добавляет случайность.

### Full jitter (рекомендуется AWS)

```go
// sleep = random(0, min(cap, base * 2^attempt))
func fullJitter(attempt int, base, cap time.Duration) time.Duration {
    ceiling := base * (1 << attempt)
    if ceiling > cap {
        ceiling = cap
    }
    return time.Duration(rand.Int63n(int64(ceiling)))
}
```

Распределение равномерное от 0 до ceiling — максимально рассредоточивает запросы.

### Decorrelated jitter

```go
// sleep = random(base, prev_sleep * 3)
// Хорошо рассредоточивает при большом числе клиентов
func decorrelatedJitter(base, cap, prev time.Duration) time.Duration {
    if prev == 0 {
        prev = base
    }
    d := base + time.Duration(rand.Int63n(int64(prev*3-base)))
    if d > cap {
        d = cap
    }
    return d
}
```

### Equal jitter (компромисс)

```go
// sleep = ceiling/2 + random(0, ceiling/2)
// Гарантирует минимальную паузу, но не слишком большую
func equalJitter(attempt int, base, cap time.Duration) time.Duration {
    ceiling := base * (1 << attempt)
    if ceiling > cap {
        ceiling = cap
    }
    half := ceiling / 2
    return half + time.Duration(rand.Int63n(int64(half)))
}
```

---

## Retry budget

Retry budget ограничивает долю запросов которые могут быть retry — защита от retry storm.

Пример: retry budget = 10% — не более 10 retry на 100 оригинальных запросов. Если ошибок больше — останавливаем retry, сигнализируем caller об ошибке.

```go
type RetryBudget struct {
    mu        sync.Mutex
    requests  int64
    retries   int64
    ratio     float64  // допустимая доля retry (0.1 = 10%)
    windowDur time.Duration
    resetAt   time.Time
}

func (b *RetryBudget) Allow() bool {
    b.mu.Lock()
    defer b.mu.Unlock()

    now := time.Now()
    if now.After(b.resetAt) {
        b.requests = 0
        b.retries = 0
        b.resetAt = now.Add(b.windowDur)
    }

    b.requests++
    if b.requests == 0 {
        return true
    }
    if float64(b.retries)/float64(b.requests) >= b.ratio {
        return false  // бюджет исчерпан
    }
    b.retries++
    return true
}
```

---

## Retry amplification в цепочке сервисов

Самая опасная проблема: retry на каждом уровне перемножается.

```
Клиент → A (3 retry) → B (3 retry) → C (3 retry) → DB

При одном сбое в DB:
  A делает 3 попытки
  Каждая попытка A → B делает 3 попытки
  Каждая попытка B → C делает 3 попытки
  Итого на DB: 3 × 3 × 3 = 27 запросов вместо 1
```

Правила:
1. **Retry только на краях системы** — клиент делает retry к A, A к B — нет, B к C — нет
2. **Или retry только на нижнем уровне** — C retry к DB, остальные не retry
3. **Использовать `Retry-After` header** — сервис сам говорит когда можно снова

```go
// Проверить Retry-After header перед retry
func retryAfterDelay(resp *http.Response) time.Duration {
    if resp == nil {
        return 0
    }
    h := resp.Header.Get("Retry-After")
    if h == "" {
        return 0
    }
    // Retry-After может быть числом секунд или датой
    if secs, err := strconv.Atoi(h); err == nil {
        return time.Duration(secs) * time.Second
    }
    if t, err := http.ParseTime(h); err == nil {
        return time.Until(t)
    }
    return 0
}
```

---

## Реализация в Go

```go
type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
}

func WithRetry(ctx context.Context, cfg RetryConfig, fn func(ctx context.Context) error) error {
    var lastErr error
    for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
        if err := ctx.Err(); err != nil {
            return err  // контекст истёк — не продолжаем
        }

        lastErr = fn(ctx)
        if lastErr == nil {
            return nil
        }
        if !isRetriable(lastErr) {
            return lastErr  // не retry
        }

        if attempt == cfg.MaxAttempts-1 {
            break
        }

        delay := fullJitter(attempt, cfg.BaseDelay, cfg.MaxDelay)
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
        }
    }
    return fmt.Errorf("after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// Использование
err := WithRetry(ctx, RetryConfig{
    MaxAttempts: 3,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    5 * time.Second,
}, func(ctx context.Context) error {
    return client.Call(ctx, req)
})
```

Готовые библиотеки:
- `github.com/cenkalti/backoff/v4` — полнофункциональная библиотека, ExponentialBackOff
- `google.golang.org/grpc/backoff` — стандартный backoff для gRPC reconnect

---

## Антипаттерны

**Немедленный retry без паузы** — `for { err = call(); if err == nil { break } }` — DDoS на себя.

**Retry на неидемпотентных операциях** — `POST /payments` без idempotency key: пользователь заплатит дважды.

**Игнорировать контекст в цикле retry** — проверяй `ctx.Err()` перед каждой попыткой. Если клиент отключился — нет смысла retry.

**Безлимитный retry** — без `MaxAttempts` или retry budget: сервис может делать retry вечно, накапливая goroutines.

**Одинаковый delay без jitter** — при массовом сбое 1000 клиентов с паузой 1s синхронно ударят через секунду.
