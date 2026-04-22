# Modular Monolith: глубокий разбор

Один deployable artifact с архитектурными границами между модулями. Часто лучший выбор между "большим монолитом" и преждевременными микросервисами.

## Содержание

- [Что делает монолит модульным](#что-делает-монолит-модульным)
- [Структура модуля](#структура-модуля)
- [Cross-module communication](#cross-module-communication)
- [Database: самая сложная часть](#database-самая-сложная-часть)
- [Enforcement границ](#enforcement-границ)
- [Тестирование модульных границ](#тестирование-модульных-границ)
- [Эволюция: выделение в микросервис](#эволюция-выделение-в-микросервис)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

---

## Что делает монолит модульным

Разница не в структуре папок, а в **enforcement границ**.

```
Большой монолит                  Modular monolith
────────────────────             ────────────────────────────────
/internal/orders/                /internal/orders/
  service.go                       module.go     ← публичный API
  handler.go                       service.go    ← unexported
/internal/payments/              /internal/payments/
  service.go                       module.go
  handler.go                       service.go

Payments может импортировать     Payments видит только
orders.Service напрямую          orders.Module (публичный API)
→ hidden coupling                → явный контракт
```

**Три признака настоящего модульного монолита:**

| Признак | Как проверить |
|---|---|
| Явный публичный API модуля | `module.go` экспортирует только то что нужно снаружи |
| Нельзя импортировать internal пакеты | Go `internal/` enforcement или кастомный linter |
| Модуль можно удалить с понятным списком последствий | Нет неявных зависимостей через shared типы |

---

## Структура модуля

```
internal/
  orders/
    module.go          ← единственная точка входа
    service.go         ← бизнес-логика (unexported)
    repository.go      ← интерфейс хранилища (unexported)
    handler.go         ← HTTP handlers (unexported)
    model.go           ← внутренние типы (unexported)
    errors.go          ← domain errors
    postgres/
      repository.go    ← реализация (unexported)
    module_test.go     ← тесты публичного API
```

**module.go — граница модуля:**

```go
// internal/orders/module.go
package orders

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

// Зависимости которые Orders требует от других модулей — только интерфейсы
type PaymentChecker interface {
    IsPaymentConfirmed(ctx context.Context, orderID string) (bool, error)
}

type UserGetter interface {
    GetUser(ctx context.Context, userID string) (User, error)
}

// Module — единственная точка входа для остального приложения
type Module struct {
    svc *orderService
    h   *handler
}

func New(db *pgxpool.Pool, payments PaymentChecker, users UserGetter) *Module {
    repo := newPostgresRepository(db)
    svc  := newOrderService(repo, payments, users)
    h    := newHandler(svc)
    return &Module{svc: svc, h: h}
}

// Публичный API: только то что реально нужно снаружи

func (m *Module) CreateOrder(ctx context.Context, cmd CreateOrderCmd) (OrderID, error) {
    return m.svc.create(ctx, cmd)
}

func (m *Module) GetOrder(ctx context.Context, id OrderID) (Order, error) {
    return m.svc.get(ctx, id)
}

func (m *Module) CancelOrder(ctx context.Context, id OrderID, reason string) error {
    return m.svc.cancel(ctx, id, reason)
}

// HTTP роуты — регистрируются в main.go
func (m *Module) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /orders", m.h.create)
    mux.HandleFunc("GET /orders/{id}", m.h.get)
    mux.HandleFunc("DELETE /orders/{id}", m.h.cancel)
}
```

**Сборка в main.go:**

```go
func main() {
    db := mustConnectDB(cfg)

    // Modules
    users    := users.New(db)
    payments := payments.New(db, stripeClient)
    orders   := orders.New(db, payments, users)
    // orders.New принимает интерфейсы — не конкретные модули!
    // payments удовлетворяет orders.PaymentChecker
    // users удовлетворяет orders.UserGetter

    mux := http.NewServeMux()
    users.RegisterRoutes(mux)
    payments.RegisterRoutes(mux)
    orders.RegisterRoutes(mux)

    http.ListenAndServe(":8080", mux)
}
```

---

## Cross-module communication

Три способа — с разными trade-offs.

### 1. Прямой вызов через интерфейс (синхронный)

```
Orders Module ──────────────────────► Payments Module
              PaymentChecker interface   (реализует интерфейс)
```

```go
// Orders определяет что ему нужно — минимальный интерфейс
type PaymentChecker interface {
    IsPaymentConfirmed(ctx context.Context, orderID string) (bool, error)
}

// Payments реализует этот интерфейс — не зная об Orders
func (m *PaymentsModule) IsPaymentConfirmed(ctx context.Context, orderID string) (bool, error) {
    // ...
}
```

**Когда:** нужен немедленный ответ (синхронный flow), простая зависимость.

**Минус:** coupling существует, хотя и через интерфейс. При выделении в микросервис — станет HTTP/gRPC вызовом.

---

### 2. Domain events (асинхронный, in-process)

```
Orders Module ──event──► In-Process Event Bus ──► Payments Module
                                                 ──► Notifications Module
                                                 ──► Analytics Module
```

```go
// Общий event bus (в platform/)
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventType string, handler EventHandler)
}

// Orders публикует событие — не знает кто слушает
func (s *orderService) create(ctx context.Context, cmd CreateOrderCmd) (OrderID, error) {
    order := newOrder(cmd)
    if err := s.repo.save(ctx, order); err != nil {
        return "", err
    }
    s.bus.Publish(ctx, OrderCreatedEvent{
        OrderID:    order.ID,
        UserID:     order.UserID,
        TotalCents: order.Total.Cents(),
        OccurredAt: time.Now(),
    })
    return order.ID, nil
}

// Payments подписывается — не знает об Orders
func (m *PaymentsModule) registerHandlers(bus EventBus) {
    bus.Subscribe("order.created", m.onOrderCreated)
}

func (m *PaymentsModule) onOrderCreated(ctx context.Context, e Event) error {
    event := e.(OrderCreatedEvent)
    return m.initiatePayment(ctx, event.OrderID, event.TotalCents)
}
```

**Когда:** несколько модулей реагируют на одно событие, нет необходимости в немедленном ответе.

**Минус:** сложнее отлаживать, порядок не гарантирован, нужна осторожность с транзакциями.

---

### 3. Shared read model (денормализованные данные)

```
Orders Module ──writes──► orders.orders (table)
                                │
                         ──sync─┘
                                ▼
                         shared.order_summaries (view/table)
                                │
Payments Module ──reads─────────┘
Analytics Module ──reads────────┘
```

```go
// Payments читает из shared view — не дёргает Orders Module
type orderSummary struct {
    OrderID    string
    UserID     string
    TotalCents int64
    Status     string
}

func (r *pgPaymentRepo) getOrderSummary(ctx context.Context, orderID string) (orderSummary, error) {
    // Читает из orders schema через view, не через Orders Module API
    var s orderSummary
    err := r.db.QueryRow(ctx,
        "SELECT order_id, user_id, total_cents, status FROM orders.order_summaries WHERE order_id = $1",
        orderID).Scan(&s.OrderID, &s.UserID, &s.TotalCents, &s.Status)
    return s, err
}
```

**Когда:** read-heavy, нет необходимости в актуальных данных в реальном времени.

**Минус:** денормализация, нужно поддерживать view актуальным.

---

### Сравнение подходов

| | Прямой вызов | Domain events | Shared read model |
|---|---|---|---|
| Coupling | Через интерфейс | Минимальный | Через схему БД |
| Транзакционность | Да (один процесс) | Нет (eventual) | Нет |
| Отладка | Просто | Сложнее | Просто |
| Latency | Нет | Нет (in-process) | Нет |
| Путь к микросервисам | → HTTP/gRPC | → Kafka/брокер | → API endpoint |

---

## Database: самая сложная часть

Общая БД при плохой дисциплине превращает все модули в неявно связанную систему.

### PostgreSQL schemas (рекомендуется)

```sql
-- Каждый модуль — своя schema
CREATE SCHEMA orders;
CREATE SCHEMA payments;
CREATE SCHEMA users;

-- Таблицы модуля
CREATE TABLE orders.orders (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL,
    status     VARCHAR(20) NOT NULL,
    total_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payments.transactions (
    id         UUID PRIMARY KEY,
    order_id   UUID NOT NULL,
    amount_cents BIGINT NOT NULL,
    status     VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Payments НЕ делает JOIN с orders.orders напрямую в production коде
-- Если нужны данные из другого модуля — через API или shared view
```

**Правила для БД:**

```
✓ Каждый модуль пишет только в свою schema
✓ Читать из своей schema без ограничений
✓ Cross-schema JOIN только в явно обозначенных read models / views
✗ Прямой UPDATE/INSERT в чужую schema
✗ Foreign keys между схемами (создаёт deployment coupling)
```

### Миграции по модулям

```
migrations/
  orders/
    001_create_orders.up.sql
    002_add_index_user_id.up.sql
  payments/
    001_create_transactions.up.sql
  users/
    001_create_users.up.sql
```

```go
// Применение миграций по модулю
func migrateOrders(db *sql.DB) error {
    return goose.Up(db, "migrations/orders")
}
```

---

## Enforcement границ

Без автоматической проверки границы нарушаются "случайно".

### Go `internal/` как enforcement

```
internal/orders/
  module.go          ← виден всем внутри модуля orders
  service.go         ← НЕ виден снаружи orders (Go enforcement)

Попытка импорта снаружи:
import "myapp/internal/orders/service"  // ← compile error!
// use of internal package not allowed
```

Но `internal/orders/` и `internal/payments/` могут импортировать друг друга напрямую — `internal/` защищает только от внешних модулей.

### Глубокий `internal/` для полной защиты

```
internal/
  orders/
    internal/        ← второй уровень internal!
      service.go     ← недоступен из payments
      repository.go
    module.go        ← доступен из payments
```

Теперь `payments` может импортировать `orders.Module`, но не `orders/internal/service`.

### Архитектурные тесты

```go
// architecture_test.go — CI проверяет нарушения
func TestModuleBoundaries(t *testing.T) {
    // orders не должен импортировать payments напрямую
    pkgs, _ := packages.Load(&packages.Config{Mode: packages.NeedImports},
        "myapp/internal/orders/...")

    for _, pkg := range pkgs {
        for imp := range pkg.Imports {
            if strings.HasPrefix(imp, "myapp/internal/payments/") &&
               !strings.HasSuffix(imp, "/module") {
                t.Errorf("orders imports payments internal: %s → %s", pkg.PkgPath, imp)
            }
        }
    }
}
```

### go-cleanarch / depguard

```yaml
# .golangci.yml
linters-settings:
  depguard:
    rules:
      orders-no-payments-internals:
        files:
          - "**/orders/**/*.go"
        deny:
          - pkg: "myapp/internal/payments"
            desc: "orders must not import payments internals, use interface"
```

---

## Тестирование модульных границ

### Unit тесты внутри модуля

```go
// internal/orders/service_test.go
// Тестируем внутреннюю логику с моками

type mockPaymentChecker struct {
    confirmed bool
}

func (m *mockPaymentChecker) IsPaymentConfirmed(_ context.Context, _ string) (bool, error) {
    return m.confirmed, nil
}

func TestOrderService_Cancel(t *testing.T) {
    svc := newOrderService(
        newInMemoryRepo(),
        &mockPaymentChecker{confirmed: false},
        &mockUserGetter{},
    )
    // ...
}
```

### Тесты публичного API модуля

```go
// internal/orders/module_test.go
// Тестируем через публичный API — как другой модуль

func TestOrdersModule_CreateAndGet(t *testing.T) {
    db := testutil.NewTestDB(t)  // реальная test DB
    payments := &stubPaymentChecker{}
    users    := &stubUserGetter{}

    m := orders.New(db, payments, users)

    id, err := m.CreateOrder(ctx, orders.CreateOrderCmd{
        UserID:     "user-1",
        TotalCents: 5000,
    })
    require.NoError(t, err)

    order, err := m.GetOrder(ctx, id)
    require.NoError(t, err)
    assert.Equal(t, "pending", order.Status)
}
```

### Integration тест cross-module flow

```go
// integration/order_payment_flow_test.go
func TestOrderPaymentFlow(t *testing.T) {
    db := testutil.NewTestDB(t)

    usersModule    := users.New(db)
    paymentsModule := payments.New(db, fakeStripe)
    ordersModule   := orders.New(db, paymentsModule, usersModule)
    // paymentsModule реализует orders.PaymentChecker — Go проверит при компиляции

    userID, _ := usersModule.CreateUser(ctx, ...)
    orderID, _ := ordersModule.CreateOrder(ctx, orders.CreateOrderCmd{UserID: userID, ...})

    // Симулировать подтверждение оплаты
    paymentsModule.ConfirmPayment(ctx, orderID)

    order, _ := ordersModule.GetOrder(ctx, orderID)
    assert.Equal(t, "paid", order.Status)
}
```

---

## Эволюция: выделение в микросервис

Модульный монолит — промежуточный шаг. Правильно организованный модуль легко выделяется.

```
До выделения:                    После выделения:
─────────────────                ─────────────────────────────
monolith                         monolith          payments-service
  orders.Module  ─────────►        orders.Module ──HTTP──► PaymentsClient
  payments.Module                                           │
    (реализует                                    (реализует та же
    PaymentChecker)                               PaymentChecker interface)
```

**Шаги выделения:**

```
1. Убедиться что граница чистая:
   orders взаимодействует с payments только через PaymentChecker интерфейс ✓

2. Создать HTTP/gRPC клиент:
   type paymentsHTTPClient struct { baseURL string }
   func (c *paymentsHTTPClient) IsPaymentConfirmed(...) (bool, error) { ... }
   // реализует orders.PaymentChecker

3. Заменить в main.go:
   // было:
   payments := payments.New(db, stripeClient)
   orders   := orders.New(db, payments, users)
   
   // стало:
   paymentsClient := paymentshttp.NewClient(cfg.PaymentsServiceURL)
   orders         := orders.New(db, paymentsClient, users)

4. Задеплоить payments как отдельный сервис

5. Удалить payments.Module из монолита
```

**Почему это работает:** orders никогда не знал о конкретном `payments.Module`, только об интерфейсе. Замена реализации = одна строка в `main.go`.

**Когда выделять:**
- Команда payments хочет деплоиться независимо
- Payments требует значительно другого scaling
- SLA payments отличается от остального монолита

**Когда НЕ выделять:**
- "Так правильнее архитектурно"
- Команда маленькая и нет реальной причины
- Граница ещё не устоялась

---

## Типичные ошибки

### 1. Модули только в README

```
Симптом: в коде нет module.go, любой пакет может импортировать любой
Следствие: через месяц — всё зависит от всего, те же проблемы что у монолита
Решение: внедрить module.go + architectural tests в CI с первого дня
```

### 2. Shared `domain/` пакет для всех моделей

```go
// Плохо: один пакет с моделями всех модулей
package domain  // ← все зависят от него

type Order struct { ... }
type Payment struct { ... }
type User struct { ... }
type Invoice struct { ... }

// Любое изменение в domain ломает всех
// circular imports при добавлении логики
```

```go
// Хорошо: каждый модуль — свои типы
// internal/orders/model.go
type Order struct { ... }
type OrderID string

// internal/payments/model.go
type Transaction struct { ... }
type TransactionID string

// Если нужны общие типы (Money, UserID) — отдельный minimal пакет
// internal/types/money.go — только primitive value types, без логики
```

### 3. Cross-schema JOIN в production коде

```go
// Плохо: payments делает JOIN с orders напрямую
func (r *pgPaymentRepo) getOrderWithPayment(ctx context.Context, orderID string) error {
    r.db.QueryRow(ctx, `
        SELECT o.user_id, p.amount
        FROM orders.orders o              -- ← чужая schema!
        JOIN payments.transactions p ON p.order_id = o.id
        WHERE o.id = $1
    `, orderID)
}
// При выделении payments в сервис — этот query нужно переписать полностью
```

### 4. Транзакции через модули

```go
// Плохо: транзакция захватывает данные двух модулей
func (s *orderService) createWithPayment(ctx context.Context, ...) error {
    tx, _ := s.db.Begin(ctx)
    s.orderRepo.saveWithTx(ctx, tx, order)
    s.paymentRepo.createWithTx(ctx, tx, payment)  // ← знает о payment repo!
    tx.Commit(ctx)
}
// Жёсткий coupling, нельзя выделить модули
```

```go
// Хорошо: каждый модуль — своя транзакционная граница
// Координация через saga/events если нужна
func (s *orderService) create(ctx context.Context, ...) error {
    if err := s.orderRepo.save(ctx, order); err != nil {
        return err
    }
    // Публикуем событие — payments реагирует асинхронно
    s.bus.Publish(ctx, OrderCreatedEvent{...})
    return nil
}
```

---

## Interview-ready answer

> "Modular monolith — это не просто папки с названиями модулей, а архитектурные границы с enforcement. Ключевой элемент — `module.go`: единственная точка входа, всё остальное в `internal/` и недоступно снаружи модуля. Go сам это проверяет на уровне компилятора.
>
> Cross-module communication — три варианта: прямой вызов через интерфейс (синхронно, простота), domain events через in-process bus (decoupling, eventual consistency), shared read model через PostgreSQL view (для read-heavy). Главное правило: модуль знает только об интерфейсе, не о конкретной реализации другого модуля.
>
> База данных — самое сложное. PostgreSQL schemas по модулю дают физическую изоляцию без отдельных БД. Нет foreign keys между схемами — это deployment coupling. Нет прямых JOIN в production коде — это binding к чужой структуре.
>
> Главная ценность: когда понадобится выделить модуль в сервис, замена — это один `main.go` изменение: заменить конкретный модуль на HTTP-клиент, реализующий тот же интерфейс."
