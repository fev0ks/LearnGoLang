# Context и ошибки

## Содержание

- [Context boundaries](#context-boundaries)
- [Обёртывание и преобразование ошибок](#обёртывание-и-преобразование-ошибок)
- [Sentinel errors vs custom types](#sentinel-errors-vs-custom-types)
- [Логирование ошибок](#логирование-ошибок)
- [Чек-лист](#чек-лист)
- [Interview-ready answer](#interview-ready-answer)
- [Что почитать](#что-почитать)

Сквозные аспекты — то, что проходит через все слои и не принадлежит ни одному из них. Их два: управление временем жизни операции через `context.Context` и обработка ошибок при переходе между слоями.

Паттернами в строгом смысле они не являются — это скорее правила, нарушение которых видно не в коде, а в эксплуатации: по зависшим запросам к базе, исчерпанному пулу соединений и логам, где одна ошибка записана пять раз.

---

## Context boundaries

`context.Context` — не просто параметр для отмены, а граница времени жизни операции и носитель данных, привязанных к запросу.

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

### Что означает время жизни запроса

```
HTTP request:
  ↓
  ctx = r.Context()  ← создан Go runtime для этого request'а
  ↓
  Handler → Service → Repository → DB driver
                                       ↓
  При cancel/timeout — все эти слои узнают
```

Когда клиент закрывает соединение или истекает предельный срок, `ctx.Done()` срабатывает на всех уровнях стека сразу. Драйвер базы видит отмену, отправляет PostgreSQL запрос на прерывание выполнения и возвращает соединение в пул.

**Если контекст не пробрасывать:**
- База продолжает выполнять запрос, результат которого уже никому не нужен.
- Пул соединений заполняется такими запросами, и новые ждут очереди.
- Процессорное время тратится на формирование ответов, которые некому получить.

Особенно заметно это при всплеске нагрузки: клиенты уходят по таймауту и повторяют запрос, а сервер продолжает выполнять и старые, и новые — нагрузка растёт быстрее, чем поток запросов.

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

**Почему `defer cancel()` обязателен.** `context.WithTimeout` запускает таймер и регистрирует дочерний контекст у родителя. Без вызова `cancel` эта связь сохраняется до срабатывания таймера, даже если операция завершилась за миллисекунду. При долгоживущем родительском контексте и потоке запросов такие незакрытые дочерние контексты накапливаются — это утечка, которую `go vet` находит через проверку `lostcancel`.

### Что уместно хранить в context

**Уместно** — то, что относится к запросу целиком и проходит через все слои, не участвуя в бизнес-логике:
- Идентификатор запроса и трассировки — для логов и связывания вызовов.
- Данные аутентификации, положенные middleware и читаемые обработчиком.
- Дескриптор транзакции для Unit of Work.
- Идентификатор арендатора в многоарендных системах.

**Неуместно** — то, что является входными данными операции:
- Параметры функции (`userID`, `orderID`). Спрятанные в контексте, они исчезают из сигнатуры, и компилятор перестаёт проверять их наличие: забытое значение превращается в панику или тихий нулевой результат вместо ошибки сборки.
- Конфигурация. Это зависимость, и передаётся она через конструктор.
- Состояние компонента. Это поля структуры.

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

**Зачем неэкспортируемый тип ключа.** Ключи в контексте сравниваются по значению и типу. Если ключом сделать строку `"user"`, любой другой пакет — включая стороннюю библиотеку — может положить своё значение по тому же ключу и перезаписать чужое. Неэкспортируемый тип делает коллизию невозможной: снаружи пакета такое значение просто не создать.

### Передача контекста в горутины

```go
// context.Background() теряет всё: и отмену, и значения —
// вместе с идентификатором трассировки.
go func() {
    doBackgroundWork(context.Background())
}()

// context.WithoutCancel (Go 1.21+) отбрасывает только отмену:
// идентификатор запроса и трассировка остаются на месте.
bgCtx := context.WithoutCancel(ctx)
go doBackgroundWork(bgCtx)
```

Горутина, продолжающая работу после ответа клиенту, не может пользоваться `r.Context()`: он отменяется сразу после записи ответа, и фоновая работа прекратится, не начавшись. Из двух вариантов замены `context.WithoutCancel` предпочтительнее — он сохраняет значения, поэтому логи фоновой задачи остаются связанными с породившим её запросом.

Отдельно стоит помнить, что такая горутина не учитывается сервером при остановке: её нужно считать самостоятельно, иначе она будет прервана при завершении процесса. Разбор — в [Graceful Shutdown](../08-graceful-shutdown.md).

См. [01-go-core/concurrency-and-performance/04-context-patterns.md](../../../01-go-core/concurrency-and-performance/04-context-patterns.md) — детально про context patterns в concurrency.

---

## Обёртывание и преобразование ошибок

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

**Зачем `%w`, а не `%v`.** Обе записи дают одинаковый текст, но `%w` сохраняет исходную ошибку внутри новой, и `errors.Is` с `errors.As` могут пройти по цепочке до неё. С `%v` от исходной ошибки остаётся только строка, и проверить её тип уже нечем — остаётся сравнение подстрок, которое ломается при первом изменении формулировки.

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

**Преобразование — в одном месте.** Если проверки `errors.Is` разойдутся по обработчикам, одна и та же доменная ошибка в разных эндпоинтах превратится в разные коды ответа, и клиент столкнётся с несогласованным API. Общий помощник решает это:

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

**Плюсы:** объявляется одной строкой, проверяется через `errors.Is`, не требует дополнительных типов.

**Минусы:** несёт только сам факт — «не найдено», без указания, что именно и по какому идентификатору.

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

**Плюсы:** внутри ошибки есть структурированные данные, пригодные для логов и для ответа клиенту без разбора строки.

**Минусы:** на каждый вид ошибки нужен тип с методом `Error`, а при использовании — `errors.As` вместо более короткого `errors.Is`.

### Когда что

- **Sentinel** — для большинства случаев: вызывающему коду нужно принять решение, а не разобрать подробности.
- **Собственный тип** — когда данные из ошибки действительно используются: подставляются в ответ API, попадают в поля структурированного лога, определяют текст для пользователя.

Практический критерий — что делает вызывающий код. Если он только выбирает ветку обработки, sentinel достаточно. Если он извлекает из ошибки значения, нужен тип. В реальной кодовой базе обычно встречается и то, и другое.

---

## Логирование ошибок

Самая частая ошибка при работе с ошибками — записывать одну и ту же ошибку в лог на каждом уровне стека.

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

Одна ошибка превращается в три записи. Вреда здесь больше, чем кажется: объём логов растёт втрое, счётчик ошибок в мониторинге завышается втрое, а при разборе инцидента приходится определять, три это разных сбоя или один.

### Хорошо

**Правило:** ошибку записывает в лог тот, кто её обрабатывает. Тот, кто возвращает ошибку выше, вместо записи в лог добавляет контекст через `%w`.

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
        // Единственное место записи — граница, где ошибка обрабатывается
        h.log.Error("get order", "err", err, "request_id", ...)
        writeError(w, err)
    }
}
```

Ещё лучше — вынести запись в middleware, тогда ни один обработчик о логировании не думает:

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

`log.Fatal` вызывает `os.Exit(1)`, а это значит, что отложенные вызовы не выполнятся: соединения не закроются, буферы не сбросятся, начатая транзакция останется висеть до таймаута на сервере базы. Решение о завершении процесса принимает тот, кто им владеет, — вызывающий код получает ошибку и решает сам.

Уместное место для `log.Fatal` — только `main.go`, и только для ошибок запуска, после которых работать всё равно нечем.

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

Каждый уровень добавил своё имя, и в результате сообщение описывает путь по коду, а не проблему. Полезный контекст — это то, чего нет в самой ошибке: идентификатор сущности, имя операции с точки зрения предметной области. Имена функций в цепочку добавлять не нужно — они и так восстанавливаются по тексту.

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

- Роль — граница времени жизни операции: первым аргументом, не полем структуры, не хранилищем бизнес-данных.
- Обязательная передача — в запросы к базе, HTTP-вызовы и операции с брокером; иначе отмена не доходит до места, где выполняется работа.
- Цена нарушения — при таймауте клиента запрос к базе продолжает выполняться и занимать соединение, а нагрузка растёт быстрее потока запросов.
- Ключи значений — неэкспортируемый тип, иначе возможна коллизия с чужим пакетом.
- Фоновая работа — `context.WithoutCancel`: отмена отбрасывается, идентификатор трассировки сохраняется.

**2. Как устроена работа с ошибками по слоям?**

- Хранилище — оборачивает с контекстом через `%w` и переводит ошибки драйвера в доменные (`sql.ErrNoRows` в `ErrNotFound`).
- Сервис — пробрасывает доменную ошибку как есть, остальные оборачивает с указанием операции.
- Транспорт — переводит доменные ошибки в коды протокола, в одном помощнике или middleware.
- Зачем `%w`, а не `%v` — сохраняется исходная ошибка, поэтому `errors.Is` и `errors.As` продолжают работать выше по стеку.

**3. Кто и где логирует ошибку?**

- Правило — логирует тот, кто обрабатывает; тот, кто возвращает выше, только добавляет контекст.
- Цена нарушения — одна ошибка даёт три записи, втрое завышенный счётчик в мониторинге и неясность при разборе инцидента.
- Лучшее место — middleware на границе: обработчики о логировании не думают вовсе.
- Чего не делать — `log.Fatal` вне `main.go`: `os.Exit` пропускает отложенные вызовы, соединения и транзакции остаются незакрытыми.

**4. Sentinel errors или собственные типы?**

- Sentinel (`domain.ErrNotFound`, `domain.ErrForbidden`) — когда вызывающему коду нужно выбрать ветку обработки.
- Собственный тип — когда из ошибки извлекаются данные: для ответа API, для полей структурированного лога.
- Критерий выбора — что делает вызывающий код: сравнивает или разбирает.
- На практике — встречается и то, и другое в одной кодовой базе.

---

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
