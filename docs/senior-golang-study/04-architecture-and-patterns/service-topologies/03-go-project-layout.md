# Go Project Layout

Структура папок в Go — не стандарт, а набор устоявшихся практик. Главное правило: структура должна отражать архитектуру, а не повторять framework-шаблон.

## Содержание

- [Простой сервис (layered)](#простой-сервис-layered)
- [Hexagonal / Clean architecture](#hexagonal--clean-architecture)
- [Modular monolith](#modular-monolith)
- [Микросервис в монорепо](#микросервис-в-монорепо)
- [Типичные ошибки структуры](#типичные-ошибки-структуры)
- [Правила пакетов в Go](#правила-пакетов-в-go)
- [Interview-ready answer](#interview-ready-answer)

---

## Простой сервис (layered)

Для небольшого сервиса с понятными слоями. Не нужна hexagonal, нет сложной domain-логики.

```
myservice/
├── cmd/
│   └── server/
│       └── main.go          ← точка входа, wire dependencies
├── internal/
│   ├── handler/             ← HTTP/gRPC handlers (transport layer)
│   │   ├── order.go
│   │   └── order_test.go
│   ├── service/             ← бизнес-логика (use cases)
│   │   ├── order.go
│   │   └── order_test.go
│   ├── repository/          ← storage (PostgreSQL, Redis)
│   │   ├── order_postgres.go
│   │   └── order_postgres_test.go
│   └── model/               ← shared types (без логики)
│       └── order.go
├── migrations/              ← SQL миграции
│   ├── 001_create_orders.up.sql
│   └── 001_create_orders.down.sql
├── config/
│   └── config.go            ← чтение конфига из ENV/файла
├── go.mod
└── go.sum
```

**Ключевые решения:**
- `internal/` — код не экспортируется за пределы модуля (Go enforcement)
- `cmd/` — точка входа собирает зависимости
- Нет `pkg/` — это legacy-паттерн без смысла для приложений

---

## Hexagonal / Clean architecture

Для сервиса с важной domain-логикой, несколькими входами (HTTP + Worker + CLI).

```
myservice/
├── cmd/
│   ├── server/
│   │   └── main.go          ← HTTP server
│   └── worker/
│       └── main.go          ← background worker
│
├── internal/
│   ├── domain/              ← ядро: модели, интерфейсы, domain errors
│   │   ├── order.go         ← Order, OrderStatus, OrderID types
│   │   ├── errors.go        ← ErrNotFound, ErrInvalidState...
│   │   └── ports.go         ← интерфейсы (output ports)
│   │       OrderRepository
│   │       PaymentProvider
│   │       EventPublisher
│   │
│   ├── usecase/             ← use cases (application layer)
│   │   ├── create_order.go
│   │   ├── cancel_order.go
│   │   └── create_order_test.go  ← unit tests без инфраструктуры
│   │
│   ├── transport/           ← input adapters
│   │   ├── http/
│   │   │   ├── handler.go
│   │   │   ├── middleware.go
│   │   │   └── dto.go       ← request/response types
│   │   └── worker/
│   │       └── order_consumer.go
│   │
│   └── infra/               ← output adapters
│       ├── postgres/
│       │   ├── order_repo.go
│       │   └── order_repo_test.go  ← integration tests
│       ├── stripe/
│       │   └── payment.go
│       └── kafka/
│           └── publisher.go
│
├── migrations/
├── config/
├── go.mod
└── go.sum
```

**Направление зависимостей:**
```
transport/* ──► usecase ──► domain ◄── infra/*
```
`domain` ни от кого не зависит. `infra` реализует интерфейсы из `domain`.

---

## Modular monolith

Один бинарник, но код разбит на автономные модули с явными границами.

```
myapp/
├── cmd/
│   └── server/
│       └── main.go          ← собирает все модули
│
├── internal/
│   ├── orders/              ← Orders module
│   │   ├── module.go        ← публичный API модуля
│   │   ├── service.go       ← internal
│   │   ├── repository.go    ← internal
│   │   ├── handler.go       ← internal
│   │   └── model.go         ← internal
│   │
│   ├── payments/            ← Payments module
│   │   ├── module.go        ← публичный API
│   │   ├── service.go
│   │   └── ...
│   │
│   ├── users/               ← Users module
│   │   ├── module.go
│   │   └── ...
│   │
│   └── platform/            ← shared infrastructure (НЕ бизнес-логика)
│       ├── db/
│       │   └── postgres.go
│       ├── logger/
│       └── metrics/
│
├── migrations/
│   ├── orders/              ← миграции по модулям
│   ├── payments/
│   └── users/
│
└── go.mod
```

**module.go — граница модуля:**
```go
// internal/orders/module.go
package orders

// Module — единственная точка входа для других модулей
type Module struct {
    svc *orderService  // unexported
}

func New(db *pgxpool.Pool, payments PaymentChecker) *Module {
    repo := newPostgresRepository(db)
    svc  := newOrderService(repo, payments)
    return &Module{svc: svc}
}

// Только эти методы видны снаружи:
func (m *Module) CreateOrder(ctx context.Context, cmd CreateOrderCmd) (OrderID, error) {
    return m.svc.createOrder(ctx, cmd)
}

func (m *Module) GetOrder(ctx context.Context, id OrderID) (Order, error) {
    return m.svc.getOrder(ctx, id)
}

func (m *Module) HTTPRoutes(r *mux.Router) {
    h := newHandler(m.svc)
    r.Handle("/orders", h.Create).Methods("POST")
    r.Handle("/orders/{id}", h.Get).Methods("GET")
}
```

**Что НЕ должно быть в `platform/` (shared):**
- Бизнес-логика любого модуля
- Модели данных конкретного домена
- Бизнес-ошибки

---

## Микросервис в монорепо

```
monorepo/
├── services/
│   ├── order-service/       ← отдельный Go модуль
│   │   ├── cmd/server/main.go
│   │   ├── internal/
│   │   ├── go.mod           ← module github.com/myco/order-service
│   │   └── Dockerfile
│   │
│   ├── payment-service/
│   │   ├── cmd/server/main.go
│   │   ├── internal/
│   │   ├── go.mod
│   │   └── Dockerfile
│   │
│   └── notification-service/
│
├── libs/                    ← shared libraries
│   ├── observability/       ← logging, tracing, metrics
│   │   └── go.mod           ← module github.com/myco/libs/observability
│   ├── testutil/
│   └── proto/               ← shared Protobuf definitions
│
├── deployments/
│   ├── k8s/
│   └── helm/
│
└── go.work                  ← Go workspace для локальной разработки
```

**go.work для локальной разработки:**
```
// go.work
go 1.22

use (
    ./services/order-service
    ./services/payment-service
    ./libs/observability
)
```

Это позволяет редактировать `observability` и сразу видеть изменения в сервисах без публикации в registry.

---

## Типичные ошибки структуры

### 1. Пустые папки `pkg/` и `pkg/models/`

```
Плохо:
  pkg/
    models/
      user.go     ← User struct
      order.go    ← Order struct
    utils/
      strings.go  ← функции которые везде используются

Почему плохо:
  - pkg/models становится God package
  - utils превращается в свалку
  - Все пакеты зависят от pkg/models → circular deps
```

### 2. Domain layer зависит от infrastructure

```go
// Плохо: domain знает о PostgreSQL
package domain

import "github.com/jackc/pgx/v5"  // ❌

type OrderService struct {
    db *pgx.Conn  // ❌ infrastructure в domain
}

// Хорошо: domain только интерфейсы
type OrderRepository interface {
    FindByID(ctx context.Context, id OrderID) (Order, error)
    Save(ctx context.Context, order Order) error
}
```

### 3. Handler содержит бизнес-логику

```go
// Плохо
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    var req CreateOrderRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Бизнес-логика прямо в handler ❌
    if req.Amount <= 0 {
        http.Error(w, "invalid amount", 400)
        return
    }
    discount := calculateDiscount(req.UserID)   // ❌
    finalAmount := req.Amount * (1 - discount)  // ❌

    _, err := h.db.Exec("INSERT INTO orders ...")  // ❌ SQL в handler
    // ...
}

// Хорошо: handler — тонкий слой
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    var req CreateOrderRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", 400)
        return
    }

    order, err := h.svc.CreateOrder(r.Context(), CreateOrderCmd{
        UserID: req.UserID,
        Amount: req.Amount,
    })
    // маппинг ошибок → HTTP коды
}
```

### 4. Circular imports

```
Проблема:
  orders → payments  (orders создаёт платёж)
  payments → orders  (payments смотрит статус заказа)
  → import cycle!

Решение варианты:
  1. Общие типы в третьем пакете (domain/)
  2. Взаимодействие через events, не прямые вызовы
  3. Один из модулей знает о другом только через интерфейс
```

---

## Правила пакетов в Go

| Правило | Почему |
|---|---|
| Имя пакета = его функция (не `utils`, не `common`) | Понятно что внутри |
| Один пакет = одна ответственность | Меньше coupling |
| `internal/` запрещает внешние импорты (compiler enforcement) | Явные границы |
| Интерфейс объявляет потребитель, не поставщик | Decoupling |
| Минимальный exported API | Легче изменять без breaking changes |
| Тесты рядом с кодом (`foo_test.go`) | Не нужен отдельный test пакет |
| Integration тесты в `_test` пакете | Тестировать публичный API, не приватные детали |

---

## Interview-ready answer

> "Структура зависит от архитектуры. Для простого сервиса — layered: `cmd/`, `internal/handler`, `internal/service`, `internal/repository`. Для сложного домена — hexagonal: `domain/` ни от кого не зависит, `usecase/` использует domain interfaces, `transport/` и `infra/` реализуют их.
>
> Для modular monolith — каждый модуль в `internal/orders/`, `internal/payments/` с публичным `module.go` как единственной точкой входа. Другие модули видят только этот файл.
>
> Главные ошибки: God package `pkg/models/` куда тянут все типы, бизнес-логика в handlers, и infrastructure-зависимости в domain слое (SQL-пакеты в domain = сломанные тесты и coupling).
>
> `internal/` — это не просто конвенция, это compiler enforcement: Go не позволяет импортировать `internal/` пакеты снаружи модуля."
