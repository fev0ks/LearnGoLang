# Circuit Breaker

Circuit breaker — паттерн который останавливает вызовы к упавшей зависимости вместо того, чтобы тратить goroutines и таймаут на заведомо неудачные запросы.

## Содержание

- [Три состояния](#три-состояния)
- [gobreaker](#gobreaker)
- [Cascade failure: зачем нужен circuit breaker](#cascade-failure-зачем-нужен-circuit-breaker)
- [Тюнинг порогов](#тюнинг-порогов)
- [Fallback стратегии](#fallback-стратегии)
- [Антипаттерны](#антипаттерны)

---

## Три состояния

```
         ошибок много              пробный запрос
CLOSED ──────────────► OPEN ─────────────────────► HALF-OPEN
  ▲                                                    │
  │   пробный запрос успешен                           │
  └────────────────────────────────────────────────────┘
                                       │ пробный запрос упал
                                       └──────────────────► OPEN
```

**CLOSED** (нормальная работа): запросы проходят. Считаем ошибки. Если превысили порог — переходим в OPEN.

**OPEN** (circuit "разомкнут"): запросы блокируются немедленно, без вызова зависимости. Возвращаем ошибку сразу. Через `timeout` переходим в HALF-OPEN.

**HALF-OPEN** (проверка): пропускаем ограниченное число запросов. Если они успешны — переходим в CLOSED. Если нет — обратно в OPEN.

---

## gobreaker

`github.com/sony/gobreaker` — простая и надёжная реализация.

```go
import "github.com/sony/gobreaker"

cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name: "payment-service",

    // Сколько последних запросов держать в окне
    MaxRequests: 5,     // в HALF-OPEN: пропустить 5 запросов для проверки

    // Как долго ждать в OPEN перед переходом в HALF-OPEN
    Timeout: 10 * time.Second,

    // Условие перехода в OPEN
    // counts.ConsecutiveFailures — n подряд провалов
    // counts.TotalFailures / counts.Requests — процент ошибок
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        // Открыть если: минимум 5 запросов И процент ошибок > 60%
        if counts.Requests < 5 {
            return false
        }
        failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
        return failureRatio >= 0.6
    },

    // Вызывается при смене состояния — для метрик
    OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
        slog.Warn("circuit breaker state changed",
            "name", name,
            "from", from,
            "to",   to,
        )
        // Отправить метрику
        metrics.CircuitBreakerState.WithLabelValues(name).Set(float64(to))
    },
})

// Использование
result, err := cb.Execute(func() (interface{}, error) {
    return paymentClient.Charge(ctx, req)
})
if err != nil {
    if err == gobreaker.ErrOpenState {
        // Circuit открыт — зависимость недоступна
        return nil, ErrPaymentServiceUnavailable
    }
    return nil, err
}
return result.(*ChargeResponse), nil
```

### Circuit breaker per dependency

Один circuit breaker на весь сервис — плохо: ошибки платёжного сервиса закроют вызовы к нотификационному.

```go
type CircuitBreakers struct {
    Payment      *gobreaker.CircuitBreaker
    Notification *gobreaker.CircuitBreaker
    UserService  *gobreaker.CircuitBreaker
}

func NewCircuitBreakers() *CircuitBreakers {
    settings := func(name string) gobreaker.Settings {
        return gobreaker.Settings{
            Name:        name,
            MaxRequests: 5,
            Timeout:     10 * time.Second,
            ReadyToTrip: func(counts gobreaker.Counts) bool {
                return counts.Requests >= 10 &&
                    float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
            },
        }
    }
    return &CircuitBreakers{
        Payment:      gobreaker.NewCircuitBreaker(settings("payment")),
        Notification: gobreaker.NewCircuitBreaker(settings("notification")),
        UserService:  gobreaker.NewCircuitBreaker(settings("user-service")),
    }
}
```

---

## Cascade failure: зачем нужен circuit breaker

Без circuit breaker один медленный downstream убивает весь сервис:

```
PaymentService отвечает за 5s (вместо 200ms)

1. Запросы к PaymentService накапливаются, goroutines ждут
2. Пул goroutines/connections исчерпан
3. Входящие запросы начинают timeout на стороне нашего сервиса
4. Клиент видит ошибки и делает retry — ещё больше goroutines
5. Весь сервис деградирует хотя сам по себе исправен
```

С circuit breaker:
```
PaymentService отвечает за 5s

1. 10 запросов провалились — ReadyToTrip вернул true
2. Circuit переходит в OPEN — запросы к PaymentService блокируются немедленно
3. Goroutines немедленно получают ErrOpenState, не ждут 5s
4. Основной сервис продолжает работать, платёжные запросы отклоняются с понятной ошибкой
5. Через 10s circuit переходит в HALF-OPEN, пробует recovery
```

---

## Тюнинг порогов

**Слишком чувствительный** (маленький порог): circuit открывается от нескольких нормальных ошибок. Ложные срабатывания мешают работе.

**Слишком толерантный** (большой порог): circuit открывается когда уже всё плохо. Упускаем время для защиты.

Практическое правило:
- Минимальный объём (Requests >= N) — чтобы не срабатывать на холодный старт
- Процент ошибок 50-60% как порог (не 100% — частичная деградация тоже опасна)
- Timeout перехода в HALF-OPEN — чуть больше ожидаемого времени recovery (restart сервиса ~30s → timeout 60s)

```go
ReadyToTrip: func(counts gobreaker.Counts) bool {
    minRequests := uint32(10)
    failureThreshold := 0.5

    if counts.Requests < minRequests {
        return false
    }
    return float64(counts.TotalFailures)/float64(counts.Requests) >= failureThreshold
},
Timeout: 60 * time.Second,
```

---

## Fallback стратегии

Когда circuit открыт — что вернуть клиенту:

**Cached response** — вернуть последний известный ответ:
```go
result, err := cb.Execute(func() (interface{}, error) {
    return userService.GetProfile(ctx, userID)
})
if errors.Is(err, gobreaker.ErrOpenState) {
    // Вернуть из кэша
    if cached, ok := profileCache.Get(userID); ok {
        return cached, nil
    }
}
```

**Degraded response** — вернуть ответ с меньшим набором данных:
```go
if errors.Is(err, gobreaker.ErrOpenState) {
    // Вернуть базовый профиль без обогащённых данных
    return &UserProfile{ID: userID, Name: "Unknown"}, nil
}
```

**Fast fail с понятной ошибкой** — не маскировать, дать клиенту знать:
```go
if errors.Is(err, gobreaker.ErrOpenState) {
    return nil, status.Error(codes.Unavailable, "payment service temporarily unavailable")
}
```

---

## Антипаттерны

**Один circuit breaker на весь сервис** — слишком грубо. Нужен отдельный на каждую downstream зависимость.

**Не логировать смены состояний** — без `OnStateChange` невозможно диагностировать почему запросы начали падать.

**Открывать circuit от любой ошибки** — `400 Bad Request` от downstream это не повод открывать circuit. Только ошибки доступности (timeout, 503, connection refused).

```go
// Различать типы ошибок
ReadyToTrip: func(counts gobreaker.Counts) bool {
    // counts.TotalFailures считает всё что вернула Execute
    // Нужно самому решать что считать "failure"
    return counts.ConsecutiveFailures >= 5
},

// В Execute — не возвращать error для "нормальных" бизнес-ошибок
cb.Execute(func() (interface{}, error) {
    resp, err := client.Call(ctx, req)
    if err != nil {
        return nil, err  // сетевая ошибка — circuit считает
    }
    if resp.StatusCode == 404 {
        return resp, nil  // бизнес-ошибка — circuit НЕ считает
    }
    if resp.StatusCode >= 500 {
        return nil, fmt.Errorf("server error: %d", resp.StatusCode)  // circuit считает
    }
    return resp, nil
})
```

**Не добавлять метрики** — circuit breaker без Prometheus счётчика по состояниям — непрозрачный чёрный ящик в production.
