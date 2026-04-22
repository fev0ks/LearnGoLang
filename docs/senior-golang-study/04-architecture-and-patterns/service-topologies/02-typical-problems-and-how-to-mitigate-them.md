# Typical Problems And How To Mitigate Them

У каждой topology свои типичные проблемы. Важно уметь их не только назвать, но и предложить практические меры.

---

## Проблемы монолита

### Таблица симптомов и решений

| Симптом | Корневая причина | Mitigation |
|---|---|---|
| "Всё зависит от всего" | Нет модульных границ, прямые импорты | Выделить internal boundaries, запретить cross-layer imports |
| Медленные тесты | Большой код, инициализация всего при запуске | Разбить на пакеты, мокировать зависимости |
| Deployment blast radius | Один артефакт = все или ничего | Feature flags, canary deploy, rollback стратегия |
| Команды блокируют друг друга | Нет ownership по частям | Разграничить по модулям, потом по сервисам |
| Сложно масштабировать hotspot | Весь монолит = один scaling unit | Выделить горячую часть в отдельный сервис |
| "Страшно менять" | Нет тестов, высокий coupling | Добавить integration тесты, потом рефакторить |

### Что помогает практически

```go
// Architectural linting: запретить импорт infrastructure из domain
// с помощью go-cleanarch или custom analyzer

// Пример разбивки на внутренние пакеты:
internal/
  orders/          ← module boundary
    service.go     ← бизнес-логика
    repository.go  ← интерфейс
    handler.go     ← HTTP
    postgres/      ← реализация репозитория
  payments/        ← отдельный module
  users/

// Запрет: orders не должен импортировать payments напрямую
// Взаимодействие только через публичный API или events
```

---

## Проблемы modular monolith

### Таблица симптомов и решений

| Симптом | Корневая причина | Mitigation |
|---|---|---|
| Модули "на бумаге" | Нет enforcement границ | go-cleanarch, архитектурные тесты |
| Shared `utils/` пакет — свалка | Нет ownership | Явный owner, разбить по доменам |
| Общая БД превращает модули в coupling | Shared schema | Prefix таблиц по модулю, отдельные схемы, или views |
| Один медленный модуль тормозит всё | Нет изоляции по goroutine/cpu | Worker pool isolation, context timeout |
| Модули незаметно ломают друг друга | Нет contract | Interface-based integration, модульные тесты на границах |

### Enforcement границ в Go

```go
// Метод 1: Теги сборки (build constraints) — не пускать в prod
// Метод 2: Интеграционный тест проверяет публичный API

// Метод 3: go-cleanarch
// Запустить в CI: go-cleanarch -domain=internal/domain

// Метод 4: Кастомный analyzer
// Проверяет что orders не импортирует payments.internal.*

// Метод 5: Отдельные go.mod на модуль (workaround)
// modules/orders/go.mod — не может использовать внутренние payments
```

### Database isolation в modular monolith

```sql
-- Вариант 1: Prefix таблиц
orders_items, orders_status
payments_transactions, payments_refunds
users_profiles, users_sessions

-- Вариант 2: PostgreSQL schemas (рекомендуется)
CREATE SCHEMA orders;
CREATE SCHEMA payments;
CREATE TABLE orders.items (...);
CREATE TABLE payments.transactions (...);

-- Cross-schema через views (controlled access)
CREATE VIEW orders.payment_status AS
  SELECT order_id, status FROM payments.transactions WHERE ...;
```

---

## Проблемы микросервисов

### Таблица симптомов и решений

| Симптом | Корневая причина | Mitigation |
|---|---|---|
| Cascade failures | Зависимый сервис падает, тянет за собой | Circuit breaker + fallback |
| Latency grows with hops | N синхронных вызовов по цепочке | Async где возможно, cache, denormalize |
| Data consistency нарушена | Distributed state без координации | Outbox + saga, eventual consistency |
| "Кто сломал API?" | Нет contract versioning | Consumer-driven contract tests (Pact) |
| Локальная разработка невозможна | Нужно поднять 10+ сервисов | docker-compose, service virtualization, dev environment |
| Impossible to trace a request | Нет distributed tracing | OpenTelemetry, trace propagation через headers |
| Operational overload | Каждый сервис = свой мониторинг | Platform team, shared tooling, service mesh |

### Circuit breaker в Go

```go
// github.com/sony/gobreaker
cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "payment-service",
    MaxRequests: 5,                 // запросов в half-open state
    Interval:    30 * time.Second,  // сброс счётчиков
    Timeout:     10 * time.Second,  // время в open state
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
        return counts.Requests >= 10 && failureRatio >= 0.5
    },
    OnStateChange: func(name string, from, to gobreaker.State) {
        log.Printf("circuit breaker %s: %s → %s", name, from, to)
        metrics.GaugeSet("circuit_breaker.state", float64(to), name)
    },
})

result, err := cb.Execute(func() (interface{}, error) {
    return paymentClient.Charge(ctx, req)
})
if errors.Is(err, gobreaker.ErrOpenState) {
    // fallback: вернуть cached result или отложить
}
```

### Distributed tracing

```go
// Trace propagation через context (OpenTelemetry)
import "go.opentelemetry.io/otel"

// В каждом HTTP handler/gRPC interceptor:
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    ctx, span := otel.Tracer("order-service").Start(r.Context(), "CreateOrder")
    defer span.End()

    // При вызове другого сервиса — trace ID передаётся автоматически
    // через HTTP headers: traceparent: 00-{trace_id}-{span_id}-01
    result, err := h.paymentClient.Charge(ctx, req)
    if err != nil {
        span.RecordError(err)
    }
}
```

### Retry и timeout strategy

```
Без стратегии:                  С правильной стратегией:
  Request → Timeout (30s)         Request → Timeout (500ms)
  Retry immediately                         │
  Request → Timeout (30s)         Wait (exponential backoff + jitter)
  Retry immediately                         │
  ...                             Retry → Success (или DLQ)
  60 секунд потеряно

Правила:
  - Timeout < SLA вызывающего сервиса
  - Exponential backoff: 100ms, 200ms, 400ms, 800ms
  - Jitter: ±20% чтобы избежать thundering herd
  - Max retries: 3-5 для transient errors
  - Circuit breaker: не retry при open state
```

---

## Общий anti-pattern: распределённый монолит

```
Симптомы:
  □ Сервисы нельзя деплоить независимо
  □ Общая база данных между сервисами
  □ Синхронные вызовы по цепочке > 3 hop'ов
  □ При падении одного сервиса падают все
  □ Изменение в одном сервисе требует изменений в других

Это хуже монолита: все минусы обоих подходов.

Лечение:
  1. Сначала навести порядок в одном сервисе
  2. Ввести data ownership (каждый сервис = своя БД)
  3. Перейти на async там где возможно
  4. Добавить circuit breaker и fallback
  5. Только потом думать об архитектуре
```

---

## Как выбирать митигацию

```
Проблема                         Первый шаг
──────────────────────────────   ─────────────────────────────────
Cascade failures                 Circuit breaker + timeout
Data inconsistency               Outbox + idempotent consumers
API breaking changes             Semantic versioning + deprecation period
Slow integration tests           Мокировать внешние зависимости
Distributed tracing отсутствует  OpenTelemetry + Jaeger/Tempo
Локальная разработка сложная     docker-compose + wiremock/mockserver
Команды блокируют друг друга     Чёткий ownership + contract tests
```

---

## Interview-ready answer

> "Каждая topology несёт предсказуемые проблемы. В монолите — coupling и общий blast radius, лечится модульными границами и feature flags. В modular monolith — нужна дисциплина: без enforcement границы ломаются, общая схема БД создаёт неявный coupling, лечится архитектурными тестами и PostgreSQL schemas по модулям.
>
> В микросервисах самые опасные проблемы — cascade failures и data consistency. Для cascade: circuit breaker с fallback и правильные timeout'ы (не 30 секунд, а 500ms). Для consistency: outbox pattern и idempotent consumers, потому что exactly-once в distributed системе практически недостижимо.
>
> Главный anti-pattern — распределённый монолит: сервисы с shared DB, синхронные цепочки вызовов и coupled deployment. Это все минусы обоих подходов без плюсов."
