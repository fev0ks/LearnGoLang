# Context и ошибки

Cross-cutting concerns — то что пронизывает весь код, независимо от слоя. Два главных: **управление lifetime** через context.Context и **обработка ошибок** через wrap/map.

Эти "паттерны" не такие явные как Repository или Decorator. Это скорее **правила гигиены кода**, которые отличают качественный Go-сервис от любительского.

## Содержание

- [Context boundaries](#context-boundaries)
- [Error wrapping and mapping](#error-wrapping-and-mapping)
- [Sentinel errors vs custom types](#sentinel-errors-vs-custom-types)
- [Логирование ошибок](#логирование-ошибок)
- [Чек-лист](#чек-лист)
- [Interview-ready answer](#interview-ready-answer)

---

## Context boundaries

`context.Context` — не просто параметр для отмены. Это **граница времени жизни запроса** и носитель request-scoped данных.

### Базовые правила

```go
// Правила:

// 1. context.Context — всегда первый аргумент
func (s *Service) Process(ctx context.Context, req Request) error { ... }

// 2. Не хранить в struct
type BadService struct {
    ctx context.Context  // ❌ — lifetime непредсказуем
}

// 3. Не использовать как generic map для бизнес-данных
ctx = context.WithValue(ctx, "userID", 123)  // ❌ — используй явные параметры

// 4. Timeout на границе use case или external call
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()
    err := h.svc.CreateOrder(ctx, ...)
}

// 5. Обязательно пробрасывать в DB, HTTP, broker clients
rows, err := db.QueryContext(ctx, query)               // ✓
resp, err := http.NewRequestWithContext(ctx, ...)      // ✓
```

### Что значит "lifetime запроса"

```
HTTP request:
  ↓
  ctx = r.Context()  ← создан Go runtime для этого request'а
  ↓
  Handler → Service → Repository → DB driver
                                       ↓
  При cancel/timeout — все эти слои узнают
```

Когда клиент закроет соединение (или timeout наступит) — `ctx.Done()` срабатывает на **всех уровнях стека**. БД-driver видит cancellation, посылает Postgres'у `CancelQuery`, освобождает connection.

**Без proper propagation:**
- БД продолжает выполнять query который никому не нужен
- Connection pool exhausted "висящими" запросами
- Затраты CPU на ответы которые никому не нужны

### Cancel propagation

```go
// Базовый запрос с timeout
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel()  // ← обязательно, даже если успешно

// Cancellation propagation
go func() {
    select {
    case <-ctx.Done():
        // Дочерние operations узнают о cancellation
        return
    case <-time.After(10*time.Second):
        // Слишком долго
    }
}()
```

**Почему `defer cancel()`:** даже если ctx истёк по timeout, нужно явно вызвать cancel — это освобождает ресурсы tied к context. Без cancel — небольшая memory leak.

### Что уместно хранить в context

**Yes (request-scoped, не бизнес-данные):**
- Request ID / Trace ID — для logging/tracing
- Authenticated user claims — auth middleware кладёт, handler читает
- Transaction handle — для UoW паттерна
- Tenant ID в multi-tenant системах

**No (бизнес-данные):**
- Параметры функции — `userID`, `orderID` — это аргументы, не context
- Конфигурация — это структура, передавай через DI
- Состояние — это поля, не context

### Type-safe context values

`context.WithValue` принимает `any`. Чтобы избежать конфликтов и сделать type-safe:

```go
// Использовать unexported type как ключ
type contextKey int

const (
    claimsContextKey contextKey = iota
    requestIDContextKey
)

// Setter и getter
func WithClaims(ctx context.Context, c *Claims) context.Context {
    return context.WithValue(ctx, claimsContextKey, c)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
    c, ok := ctx.Value(claimsContextKey).(*Claims)
    return c, ok
}

// Использование
claims, ok := ClaimsFromContext(ctx)
if !ok {
    return nil, errors.New("no claims in context")
}
```

**Зачем unexported type:** другой пакет не может случайно создать ключ с тем же значением — типы разные. Защита от ключевых коллизий.

### Context propagation across goroutines

```go
// Если запускаешь goroutine для async work
go func() {
    // ОСТОРОЖНО: переданный ctx может быть отменён
    // если request кончился — это работа в фоне без context'а
    bgCtx := context.Background()  // или WithoutCancel в Go 1.21+
    doBackgroundWork(bgCtx)
}()

// Go 1.21+ — оторвать cancellation сохраняя values
bgCtx := context.WithoutCancel(ctx)  // values сохранены, cancellation нет
go doBackgroundWork(bgCtx)
```

**Тонкий момент:** если goroutine продолжается после завершения request — пользоваться `r.Context()` нельзя, он отменится. Создавай отдельный context (Background или WithoutCancel).

См. [01-go-core/concurrency-and-performance/04-context-patterns.md](../../../01-go-core/concurrency-and-performance/04-context-patterns.md) — детально про context patterns в concurrency.

---

## Error wrapping and mapping

Ошибки в Go — это значения, и работа с ними имеет три уровня:

```
Storage layer          Service layer          Transport layer
─────────────────      ─────────────────      ─────────────────
*pgconn.PgError    →   domain.NotFoundError → HTTP 404
*pgconn.PgError    →   domain.ConflictError → HTTP 409
context.DeadlineExceeded → (пробросить)    → HTTP 504
```

### Storage layer: оборачивать с контекстом

```go
func (r *pgOrderRepo) FindByID(ctx context.Context, id OrderID) (Order, error) {
    var o Order
    err := r.db.QueryRowContext(ctx, query, id).Scan(&o.ID, &o.Status)
    if errors.Is(err, sql.ErrNoRows) {
        // Маппим на domain error, оборачиваем с контекстом
        return Order{}, fmt.Errorf("order %s: %w", id, domain.ErrNotFound)
    }
    if err != nil {
        // Любая другая ошибка — wrap с контекстом ("где это произошло")
        return Order{}, fmt.Errorf("find order %s: %w", id, err)
    }
    return o, nil
}
```

**Зачем `%w`:** позволяет `errors.Is` и `errors.As` "размотать" цепочку и проверить original error.

```go
err := repo.FindByID(ctx, "123")
// err = "find order 123: scan: connection closed"
//                              ^ inner error

if errors.Is(err, domain.ErrNotFound) {
    // ...
}

// Можно достать конкретный тип
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) {
    // ...
}
```

### Service layer: пробрасывать или мапить

```go
func (s *OrderService) ShipOrder(ctx context.Context, id OrderID) error {
    order, err := s.repo.FindByID(ctx, id)
    if err != nil {
        // Доменная ошибка — прокидываем как есть
        if errors.Is(err, domain.ErrNotFound) {
            return err
        }
        // Storage ошибка — оборачиваем
        return fmt.Errorf("ship order %s: %w", id, err)
    }

    if order.Status != "paid" {
        // Бизнес-правило — domain error
        return fmt.Errorf("order %s: %w", id, domain.ErrInvalidState)
    }

    // ... etc
}
```

### Transport layer: маппить на protocol коды

```go
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
    order, err := h.svc.GetOrder(r.Context(), id)
    if err != nil {
        switch {
        case errors.Is(err, domain.ErrNotFound):
            http.Error(w, "not found", http.StatusNotFound)
        case errors.Is(err, domain.ErrForbidden):
            http.Error(w, "forbidden", http.StatusForbidden)
        case errors.Is(err, context.DeadlineExceeded):
            http.Error(w, "timeout", http.StatusGatewayTimeout)
        default:
            // Не логируем здесь — это уже сделано в middleware
            http.Error(w, "internal error", http.StatusInternalServerError)
        }
        return
    }
    // ...
}
```

**Mapping в одном месте.** Если разбросать `if errors.Is` по handler'ам — каждый handler делает свой mapping, легко получить inconsistency. Используй helper:

```go
func writeError(w http.ResponseWriter, err error) {
    switch {
    case errors.Is(err, domain.ErrNotFound):
        http.Error(w, "not found", http.StatusNotFound)
    case errors.Is(err, domain.ErrForbidden):
        http.Error(w, "forbidden", http.StatusForbidden)
    case errors.Is(err, domain.ErrInvalidInput):
        http.Error(w, "bad request", http.StatusBadRequest)
    case errors.Is(err, context.DeadlineExceeded):
        http.Error(w, "timeout", http.StatusGatewayTimeout)
    default:
        http.Error(w, "internal error", http.StatusInternalServerError)
    }
}
```

---

## Sentinel errors vs custom types

Два способа определять domain errors:

### Sentinel errors (плоские)

```go
package domain

import "errors"

var (
    ErrNotFound       = errors.New("not found")
    ErrForbidden      = errors.New("forbidden")
    ErrInvalidInput   = errors.New("invalid input")
    ErrConflict       = errors.New("conflict")
    ErrInvalidState   = errors.New("invalid state")
)
```

**Использование:**
```go
return fmt.Errorf("order %s: %w", id, domain.ErrNotFound)

// Проверка
if errors.Is(err, domain.ErrNotFound) { ... }
```

**Плюсы:** простота, легко добавить, легко проверить.
**Минусы:** нельзя передать дополнительные данные.

### Custom error types (с данными)

```go
type NotFoundError struct {
    Resource string
    ID       string
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s %s not found", e.Resource, e.ID)
}

// Sentinel check работает с указателем
func (e *NotFoundError) Is(target error) bool {
    _, ok := target.(*NotFoundError)
    return ok
}
```

**Использование:**
```go
return &NotFoundError{Resource: "order", ID: id}

// Проверка с дополнительной инфо
var nfe *NotFoundError
if errors.As(err, &nfe) {
    log.Info("missing", "resource", nfe.Resource, "id", nfe.ID)
}
```

**Плюсы:** rich context для logging и handling.
**Минусы:** больше boilerplate.

### Когда что

- **Sentinel** — большинство случаев. Простой и идиоматичный.
- **Custom types** — когда нужны structured данные в ошибке (для logging, для UI с details, для API responses).

В реальной кодбазе — обычно смесь. Sentinel для common cases, custom для специфичных.

---

## Логирование ошибок

Главная ошибка новичков — **логировать одну ошибку много раз** на каждом уровне стека.

### Плохо

```go
func (r *pgOrderRepo) FindByID(ctx, id) (Order, error) {
    err := r.db.QueryRow(...)
    if err != nil {
        log.Error("query failed", "err", err)  // ← 1
        return Order{}, err
    }
}

func (s *OrderService) GetOrder(ctx, id) (Order, error) {
    order, err := s.repo.FindByID(ctx, id)
    if err != nil {
        log.Error("repo failed", "err", err)   // ← 2
        return Order{}, err
    }
}

func (h *Handler) GetOrder(w, r) {
    order, err := h.svc.GetOrder(...)
    if err != nil {
        log.Error("service failed", "err", err) // ← 3
        http.Error(...)
    }
}
```

Одна ошибка → три log entry. Шум в логах, дублирование, поиск инцидента превращается в кошмар.

### Хорошо

**Правило:** ошибку логирует **тот кто её обрабатывает**. Если возвращаешь — не логируешь.

```go
func (r *pgOrderRepo) FindByID(ctx, id) (Order, error) {
    err := r.db.QueryRow(...)
    if err != nil {
        return Order{}, fmt.Errorf("find order %s: %w", id, err)  // wrap, no log
    }
}

func (s *OrderService) GetOrder(ctx, id) (Order, error) {
    order, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return Order{}, err  // pass through
    }
}

func (h *Handler) GetOrder(w, r) {
    order, err := h.svc.GetOrder(...)
    if err != nil {
        // Логируем ТОЛЬКО здесь — на boundary
        h.log.Error("get order", "err", err, "request_id", ...)
        writeError(w, err)
    }
}
```

Или ещё лучше — **только в middleware**:

```go
func ErrorLoggingMiddleware(log *slog.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // ResponseWriter wrapper, который записывает err
            rw := &errorCapturingWriter{ResponseWriter: w}
            next.ServeHTTP(rw, r)
            if rw.err != nil {
                log.Error("request failed", "err", rw.err, ...)
            }
        })
    }
}
```

### Что включать в лог

```go
log.Error("get order",
    "err", err,                          // wrapped chain
    "request_id", requestID,              // trace
    "user_id", userID,                    // who
    "resource_id", orderID,               // what
    "duration_ms", time.Since(start).Milliseconds(),  // when
)
```

См. [10-devops-and-observability/logging-and-log-shipping/](../../../10-devops-and-observability/logging-and-log-shipping/) — про structured logging в Go.

### Что не делать

**1. `log.Fatal` в библиотечном коде.**

```go
// ❌ В пакете-библиотеке
func ParseConfig(path string) Config {
    data, err := os.ReadFile(path)
    if err != nil {
        log.Fatal("can't read config")  // ← убивает программу пользователя!
    }
}
```

Library не должна сама принимать решение убить процесс. Возвращай error, пусть caller решает.

`log.Fatal` уместен только в `main.go` для critical startup errors.

**2. Логирование sensitive данных.**

```go
log.Info("user login", "password", req.Password)  // ❌ пароль в логах
log.Info("payment", "credit_card", card.Number)   // ❌ PCI compliance violation
```

См. [11-security/authentication/04-auth-audit-logging.md](../../../11-security/authentication/04-auth-audit-logging.md) — что нельзя логировать.

**3. Слишком verbose error chain.**

```go
return fmt.Errorf("OrderService.CreateOrder: validation: input.Items.Length: invalid value: less than 1: %w", err)
```

Слишком длинно. Wrap должен добавлять полезный контекст, не повторять stack trace.

---

## Чек-лист

```
□ context.Context первый аргумент в публичных функциях?
□ context не хранится в struct'ах?
□ context не используется для бизнес-данных (через WithValue)?
□ Используется unexported type как ключ для WithValue?
□ defer cancel() после WithTimeout/WithCancel?
□ context пробрасывается в DB queries, HTTP calls, broker operations?
□ Background goroutines имеют свой context (WithoutCancel или Background)?

□ Ошибки оборачиваются с %w для error chain?
□ Storage errors мапятся на domain errors (sql.ErrNoRows → ErrNotFound)?
□ Transport layer маппит domain errors на HTTP/gRPC status codes?
□ Mapping в одном месте (helper или middleware), не разбросан?
□ Одна ошибка логируется ОДИН раз — на boundary?
□ Sensitive данные не попадают в логи?
□ log.Fatal не используется в библиотечном коде?
```

---

## Interview-ready answer

**1. Как правильно работать с context?**

- Context — граница времени жизни запроса: первым аргументом, не хранить в struct, не использовать для бизнес-данных. Критично пробрасывать в DB queries, HTTP calls и broker operations — иначе cancellation не работает, и при timeout запросы продолжают висеть, забивая connection pool.

**2. Как устроена слоистая работа с ошибками?**

- Storage: оборачивание с контекстом `fmt.Errorf("...: %w", err)` + маппинг на domain errors (`ErrNotFound`). Service: передать или обернуть по необходимости. Transport: маппинг на HTTP-статусы в одном helper/middleware. Главное правило: одна ошибка логируется один раз — обычно в middleware на границе, а не в каждом слое.

**3. Sentinel errors или custom types?**

- Sentinel (`domain.ErrNotFound`, `domain.ErrForbidden`) — для типовых случаев; custom error types — когда нужны structured-данные для логов или API-ответа. `log.Fatal` — только в `main.go` на критичных startup-ошибках, никогда в библиотечном коде.

## Что почитать

- [Go blog: error wrapping](https://go.dev/blog/go1.13-errors) — оригинальный пост про %w
- [01-go-core/concurrency-and-performance/04-context-patterns.md](../../../01-go-core/concurrency-and-performance/04-context-patterns.md) — context в concurrency контексте
- [01-go-core/05-error-handling.md](../../../01-go-core/05-error-handling.md) — глубоко про error handling в Go
- [10-devops-and-observability/logging-and-log-shipping/02-logging-in-go-and-why-wrap-logger.md](../../../10-devops-and-observability/logging-and-log-shipping/02-logging-in-go-and-why-wrap-logger.md) — про structured logging

---

См. также:
- [01-dependencies-and-composition.md](./01-dependencies-and-composition.md)
- [02-behavior-wrapping.md](./02-behavior-wrapping.md)
- [03-domain-and-data-access.md](./03-domain-and-data-access.md)
