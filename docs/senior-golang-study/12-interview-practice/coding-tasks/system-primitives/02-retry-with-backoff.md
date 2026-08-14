# Задача 2: Retry с Exponential Backoff

## Содержание

- [Контракт задачи](#контракт-задачи)
- [Backoff и jitter](#backoff-и-jitter)
- [Корректная реализация](#корректная-реализация)
- [HTTP и Retry-After](#http-и-retry-after)
- [Retry budget и композиция](#retry-budget-и-композиция)
- [Тестирование и метрики](#тестирование-и-метрики)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Retry полезен для кратковременной ошибки, но каждую попытку оплачивают latency
и дополнительной нагрузкой на уже нездоровую зависимость. Поэтому корректный
retry всегда bounded, cancelable и разрешён только для безопасной операции.

---

## Контракт задачи

До кода нужно определить:

1. `MaxAttempts` включает первый вызов или только повторы?
2. Какие ошибки retryable и кто их классифицирует?
3. Безопасно ли повторить side effect после неизвестного результата?
4. Есть ли timeout одной попытки и общий deadline всей операции?
5. Как учитывать server hint `Retry-After`?
6. Где расположен retry, чтобы несколько слоёв не умножали attempts?

Ниже `MaxAttempts` включает первый вызов. При `MaxAttempts=3` callback
выполнится не более трёх раз, а ожиданий между ними будет не более двух.

---

## Backoff и jitter

Exponential backoff задаёт верхнюю границу ожидания перед повтором:

```text
cap(retry) = min(MaxDelay, BaseDelay * Multiplier^retry)
```

`retry=0` — ожидание после первой неуспешной попытки. При `BaseDelay=100ms`,
`Multiplier=2`, `MaxDelay=1s` caps равны `100ms, 200ms, 400ms, 800ms, 1s`.

Full Jitter выбирает случайную задержку равномерно из `[0, cap)`:

```text
delay = random(0, cap)
```

Он разносит клиентов во времени. Выражение `cap/2 + random(0, cap/2)` — это не
Full Jitter, а другой диапазон. Конкретная стратегия — trade-off: случайная
задержка уменьшает синхронные пики, но делает latency отдельного запроса менее
предсказуемой.

---

## Корректная реализация

Generic `Do` сделан функцией, потому что Go не разрешает методам объявлять свои
type parameters. RNG защищён mutex-ом: `*rand.Rand` не безопасен для concurrent
использования.

```go
package retry

import (
    "context"
    "errors"
    "fmt"
    "math"
    "math/rand"
    "sync"
    "time"
)

type Config struct {
    MaxAttempts      int
    BaseDelay        time.Duration
    MaxDelay         time.Duration
    Multiplier       float64
    AttemptTimeout   time.Duration
    Retryable        func(error) bool
}

type Retrier struct {
    cfg Config

    randomMu sync.Mutex
    random   *rand.Rand
    wait     func(context.Context, time.Duration) error
}

func New(cfg Config, source rand.Source) (*Retrier, error) {
    if cfg.MaxAttempts < 1 {
        return nil, fmt.Errorf("max attempts must be positive")
    }
    if cfg.MaxAttempts > 1 && cfg.BaseDelay <= 0 {
        return nil, fmt.Errorf("base delay must be positive")
    }
    if cfg.MaxDelay < cfg.BaseDelay {
        return nil, fmt.Errorf("max delay must be >= base delay")
    }
    if cfg.Multiplier < 1 || math.IsNaN(cfg.Multiplier) ||
        math.IsInf(cfg.Multiplier, 0) {
        return nil, fmt.Errorf("multiplier must be finite and >= 1")
    }
    if cfg.AttemptTimeout < 0 {
        return nil, fmt.Errorf("attempt timeout must not be negative")
    }
    if cfg.Retryable == nil {
        return nil, fmt.Errorf("retryable classifier is required")
    }
    if source == nil {
        source = rand.NewSource(time.Now().UnixNano())
    }

    return &Retrier{
        cfg:    cfg,
        random: rand.New(source),
        wait:   waitContext,
    }, nil
}

func Do[T any](
    ctx context.Context,
    r *Retrier,
    fn func(context.Context) (T, error),
) (T, error) {
    var zero T
    var lastErr error

    for attempt := 1; attempt <= r.cfg.MaxAttempts; attempt++ {
        if err := ctx.Err(); err != nil {
            return zero, err
        }

        value, err := callAttempt(ctx, r.cfg.AttemptTimeout, fn)
        if err == nil {
            return value, nil
        }
        lastErr = err

        if !r.cfg.Retryable(err) {
            return zero, err
        }
        if attempt == r.cfg.MaxAttempts {
            break
        }

        delay := r.delay(attempt - 1)
        if hinted, ok := retryDelay(err); ok {
            delay = hinted
            if delay > r.cfg.MaxDelay {
                delay = r.cfg.MaxDelay
            }
            if delay < 0 {
                delay = 0
            }
        }

        if err := r.wait(ctx, delay); err != nil {
            return zero, err
        }
    }

    return zero, fmt.Errorf(
        "retry exhausted after %d attempts: %w",
        r.cfg.MaxAttempts,
        lastErr,
    )
}

func callAttempt[T any](
    parent context.Context,
    timeout time.Duration,
    fn func(context.Context) (T, error),
) (T, error) {
    if timeout <= 0 {
        return fn(parent)
    }
    ctx, cancel := context.WithTimeout(parent, timeout)
    defer cancel()
    return fn(ctx)
}

func (r *Retrier) delay(retry int) time.Duration {
    capDelay := float64(r.cfg.BaseDelay)
    maxDelay := float64(r.cfg.MaxDelay)

    for i := 0; i < retry; i++ {
        if capDelay >= maxDelay/r.cfg.Multiplier {
            capDelay = maxDelay
            break
        }
        capDelay *= r.cfg.Multiplier
    }
    if capDelay > maxDelay {
        capDelay = maxDelay
    }

    r.randomMu.Lock()
    sample := r.random.Float64()
    r.randomMu.Unlock()
    return time.Duration(sample * capDelay)
}

func waitContext(ctx context.Context, delay time.Duration) error {
    timer := time.NewTimer(delay)
    defer timer.Stop()

    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-timer.C:
        return nil
    }
}

type delayHint interface {
    RetryDelay() time.Duration
}

func retryDelay(err error) (time.Duration, bool) {
    var hint delayHint
    if !errors.As(err, &hint) {
        return 0, false
    }
    return hint.RetryDelay(), true
}
```

Инъекция `rand.Source` и приватной `wait` позволяет тестировать последовательность
без реального времени. В production observer/hook вызывают вне внутренних
locks; его panic или блокировка не должны ломать retry protocol.

Вычисление caps насыщается до `MaxDelay` до умножения и не конвертирует огромное
`math.Pow` напрямую в `time.Duration`.

---

## HTTP и Retry-After

RFC 9110 разрешает `Retry-After` как целое число секунд или HTTP-date. Parser
должен отвергать отрицательные и переполняющие значения и использовать
инъецированный `now` для теста:

```go
func ParseRetryAfter(value string, now time.Time) (time.Duration, error) {
    if seconds, err := strconv.ParseInt(value, 10, 32); err == nil {
        if seconds < 0 {
            return 0, fmt.Errorf("negative Retry-After")
        }
        return time.Duration(seconds) * time.Second, nil
    }

    deadline, err := http.ParseTime(value)
    if err != nil {
        return 0, fmt.Errorf("parse Retry-After: %w", err)
    }
    if !deadline.After(now) {
        return 0, nil
    }
    return deadline.Sub(now), nil
}
```

Hint следует вернуть из attempt как часть typed error. Нельзя сначала сделать
`time.Sleep(Retry-After)` внутри HTTP callback, а затем ещё раз ждать backoff во
внешнем retrier: это удваивает delay и игнорирует cancellation первого sleep.

HTTP retry имеет дополнительные условия:

- `http.Client.Do` не считает non-2xx ошибкой — status классифицирует caller;
- response body закрывают; для reuse HTTP/1 connection его обычно дочитывают до
  EOF в разумной bounded policy;
- request body для новой попытки создают заново через request factory или
  `GetBody`;
- transport error не доказывает, что server не применил mutation;
- `PUT` и `DELETE` идемпотентны по HTTP semantics, а `POST` и `PATCH` — не
  гарантированно; фактический business effect всё равно нужно проверить;
- `429`, `503` и некоторые другие ответы могут нести `Retry-After`, но policy
  зависит от API.

Безопасная форма callback создаёт новый request на каждой попытке:

```go
result, err := retry.Do(ctx, retrier, func(attemptCtx context.Context) (*http.Response, error) {
    req, err := newRequest(attemptCtx)
    if err != nil {
        return nil, err // classifier должен считать build error permanent
    }
    return client.Do(req)
})
```

Не следует слепо повторять все `4xx` или все `5xx`: классификатор должен знать
семантику endpoint и конкретного ответа.

---

## Retry budget и композиция

Общий deadline ограничивает сумму:

```text
attempts latency + backoff waits + queueing
```

Например, общий budget `800ms`, timeout попытки `250ms` и максимум три attempts
не гарантируют выполнение всех трёх: две попытки по `250ms` плюс waits могут
израсходовать deadline раньше. Это корректно — deadline важнее счётчика.

Retries усиливают нагрузку. Если три слоя независимо делают до трёх attempts,
один входной запрос способен породить:

```text
3 * 3 * 3 = 27 downstream attempts
```

Поэтому retry обычно принадлежит одному слою, который понимает операцию и её
budget. Дополнительно применяют:

- retry budget как долю от обычного трафика;
- circuit breaker для fast fail нездоровой dependency;
- bulkhead/concurrency limit;
- rate-limit headers и server hints;
- idempotency key для mutations.

Hedged request отличается от retry: следующая копия стартует до завершения
первой и расходует concurrency. Его применяют только к безопасным операциям и
по измеренному latency tail.

---

## Тестирование и метрики

Подменив `r.wait`, можно записывать delays без `Sleep`. Проверяются:

1. `MaxAttempts=1` вызывает callback один раз и не ждёт;
2. permanent error возвращается сразу;
3. successful attempt возвращает value;
4. exhaustion сохраняет `errors.Is` для исходной ошибки;
5. cancel во время wait немедленно завершает операцию;
6. timeout попытки не превышает parent deadline;
7. cap не превышает `MaxDelay` при большом retry;
8. deterministic source даёт ожидаемый Full Jitter;
9. `Retry-After` не добавляется к ещё одному backoff;
10. concurrent вызовы проходят под `go test -race`.

Метрики: operations, attempts per operation, retry delay, terminal reason,
exhausted, canceled и budget remaining. Логировать каждую попытку как `error`
обычно шумно: промежуточный retry — событие, terminal failure — итог операции.

---

## Типичные ошибки

- Считать retries отдельно от первого вызова и получить off-by-one.
- Не валидировать zero/negative durations и multiplier.
- Называть Full Jitter другой формулой.
- Использовать `time.Sleep`, который нельзя отменить context-ом.
- Повторно отправлять уже прочитанный request body.
- Retry-ить mutation после transport error без idempotency protocol.
- Ждать и `Retry-After`, и собственный backoff последовательно.
- Разрешить retries на каждом слое и получить multiplicative amplification.
- Использовать глобальный RNG или user callback конкурентно без оговорённой
  thread safety.
- Выбирать «пять попыток» без общего deadline и capacity calculation.

---

## Interview-ready answer

1. **Из чего состоит корректный retry?**
   - **Bounded attempts —** первый вызов входит в `MaxAttempts`.
   - **Backoff + jitter —** caps растут до `MaxDelay`, Full Jitter разносит
     клиентов.
   - **Cancellation —** attempt и wait подчиняются общему context.

2. **Какие ошибки можно повторять?**
   - **Transient —** timeout/connect failure или явно retryable response.
   - **Operation-aware —** transport error мог скрывать уже выполненный side
     effect.
   - **Idempotency —** mutation повторяют только с доказанной защитой от
     дубля.

3. **Как не устроить retry storm?**
   - **One owner —** policy находится на одном подходящем слое.
   - **Budget —** общий deadline и retry quota ограничивают amplification.
   - **Coordination —** учитывать `Retry-After`, circuit breaker и bulkhead.

---

## Связанные материалы

- [RFC 9110: Retry-After](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3)
- [RFC 9110: Idempotent Methods](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)
- [Circuit Breaker](./03-circuit-breaker.md)
- [Idempotency handler](./05-idempotency-handler.md)
