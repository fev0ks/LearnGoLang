# Домен и доступ к данным

Три паттерна, которые **отделяют бизнес-логику от деталей хранения и реализации**. Все они про границу между "что делаем" (домен) и "как делаем" (infrastructure).

- **Strategy** — изменяемое поведение/алгоритм
- **Repository** — domain-language интерфейс к storage
- **Unit of Work** — транзакционная граница для нескольких операций

В Go они выглядят значительно проще чем в классическом OOP — благодаря интерфейсам, функциям и композиции.

## Содержание

- [Strategy](#strategy)
- [Repository](#repository)
- [Unit of Work](#unit-of-work)
- [Связки между паттернами](#связки-между-паттернами)
- [Anti-patterns](#anti-patterns)

---

## Strategy

Идея: вынести изменяемый алгоритм или поведение за интерфейс, чтобы менять его независимо от остального кода.

```go
// Через интерфейс (когда нужно состояние или много методов)
type PricingStrategy interface {
    Calculate(ctx context.Context, order Order) (Money, error)
}

type RegularPricing struct{}
func (RegularPricing) Calculate(ctx context.Context, order Order) (Money, error) {
    return Money(order.Subtotal), nil
}

type DiscountPricing struct{ discount float64 }
func (d DiscountPricing) Calculate(ctx context.Context, order Order) (Money, error) {
    return Money(order.Subtotal * (1 - d.discount)), nil
}

type PromoPricing struct{ promoCode string; repo PromoRepository }
func (p PromoPricing) Calculate(ctx context.Context, order Order) (Money, error) {
    promo, err := p.repo.GetByCode(ctx, p.promoCode)
    if err != nil {
        return 0, err
    }
    return promo.Apply(order), nil
}

// Через функцию (Go-style, когда нет состояния)
type PriceFunc func(order Order) Money

// Выбор стратегии
func selectPricing(user User, promoCode string) PriceFunc {
    if promoCode != "" {
        return promoPricing(promoCode)
    }
    if user.IsPremium {
        return premiumPricing
    }
    return regularPricing
}
```

### Strategy vs switch

```go
// Плохо: switch растёт с каждым новым типом
func calculatePrice(order Order, pricingType string) Money {
    switch pricingType {
    case "regular":  return regularPrice(order)
    case "discount": return discountPrice(order)
    case "promo":    return promoPrice(order)
    // добавится ещё 10 кейсов...
    }
}

// Хорошо: стратегия передаётся снаружи
func calculatePrice(order Order, strategy PriceFunc) Money {
    return strategy(order)
}
```

**Почему лучше:**
- Открыт для расширения — новая стратегия добавляется без правки `calculatePrice`
- Тестируемо — мокаешь стратегию в тесте
- Видимость намерения — `selectPricing(user, code)` чище чем switch

### Interface vs function

Когда стратегия — **функция**:
- Нет состояния
- Один метод
- Stateless преобразование

```go
type Comparator func(a, b Item) int
type Validator func(input string) error
type Hasher func(s string) string
```

Когда стратегия — **интерфейс**:
- Нужно состояние (например, repository как поле)
- Несколько методов
- Конструктор сложный

```go
type AuthStrategy interface {
    Authenticate(ctx context.Context, token string) (*User, error)
    Refresh(ctx context.Context, refreshToken string) (string, error)
}
```

В Go функция — почти всегда предпочтительнее когда возможно. Меньше boilerplate, легче тестировать.

### Когда Strategy уместен

- **Бизнес-правило с несколькими реализациями** — pricing, scoring, ranking
- **Алгоритм меняется в зависимости от context** — sort order, retry policy
- **Plugin architecture** — пользователь подключает свою стратегию

### Когда избыточен

- **Два варианта, не растут** — простой `if` или `switch` понятнее
- **Поведение не меняется** — стабильный алгоритм без вариантов

### Strategy в config-driven системах

```go
type RetryStrategy struct {
    MaxAttempts int
    BackoffFunc func(attempt int) time.Duration
}

// Linear backoff
linear := RetryStrategy{
    MaxAttempts: 3,
    BackoffFunc: func(n int) time.Duration { return time.Second * time.Duration(n) },
}

// Exponential
exp := RetryStrategy{
    MaxAttempts: 5,
    BackoffFunc: func(n int) time.Duration { return time.Second << n },
}
```

Стратегии могут быть data, передаваться из config, конфигурироваться runtime'ом.

---

## Repository

Идея: спрятать детали хранения за интерфейсом, который говорит на языке домена.

```go
// Плохо: SQL в service layer
func (s *OrderService) GetPendingOrders(ctx context.Context) ([]Order, error) {
    rows, err := s.db.QueryContext(ctx,
        "SELECT id, user_id, total FROM orders WHERE status = 'pending' AND created_at > NOW() - INTERVAL '1 hour'")
    // ...
}

// Хорошо: domain-language interface
type OrderRepository interface {
    FindPending(ctx context.Context, since time.Time) ([]Order, error)
    Save(ctx context.Context, order Order) error
    FindByID(ctx context.Context, id OrderID) (Order, error)
}
```

### Что делает Repository

**1. Хранит domain-объекты по их identity.**

```go
order, _ := repo.FindByID(ctx, orderID)
// repo вернёт Order — domain object, а не строку из БД
```

**2. Говорит языком домена.**

Не `GetAll(filter)` а `FindActiveOrdersForUser(userID)`. Не `Update(map[string]any)` а `MarkAsShipped(orderID)`.

**3. Скрывает storage detail.**

Service не знает что под капотом — Postgres, MongoDB, или комбинация (write в Postgres, read из Elasticsearch).

### Когда Repository полезен, а когда нет

| | Полезен | Вреден |
|---|---|---|
| Бизнес-логика | Есть сложные domain rules | Простой CRUD без правил |
| Тестирование | Нужны unit-тесты без БД | Достаточно integration тестов |
| Абстракция | Может смениться storage | PostgreSQL навсегда |
| Запросы | Стабильные domain-операции | Много специфичных queries |
| Mapping | Сложный domain model ↔ DB | 1:1 маппинг таблицы в структуру |

**Практичное правило:** Repository должен выражать операции домена (`FindPending`, `CompleteOrder`), а не быть оберткой над таблицей (`GetAll`, `UpdateById`).

### Хорошие имена

| Плохо (database-language) | Хорошо (domain-language) |
|---|---|
| `GetAll()` | `FindActive(ctx, since)` |
| `Update(id, fields)` | `MarkAsShipped(ctx, orderID)` |
| `GetByEmail(email)` | `FindByEmail(ctx, email)` |
| `Insert(order)` | `Save(ctx, order)` |
| `Count(filter)` | `CountPendingForUser(ctx, userID)` |

`Find` подразумевает что **результат может быть nil** (`(Order, error)` где error == NotFound). `Save` — upsert семантика, без разделения insert/update в API.

### Реализация для PostgreSQL

```go
type pgOrderRepo struct {
    db *pgxpool.Pool
}

func NewOrderRepository(db *pgxpool.Pool) OrderRepository {
    return &pgOrderRepo{db: db}
}

func (r *pgOrderRepo) FindByID(ctx context.Context, id OrderID) (Order, error) {
    var o orderRow
    err := r.db.QueryRow(ctx, `
        SELECT id, user_id, total, status, created_at
        FROM orders WHERE id = $1
    `, id).Scan(&o.ID, &o.UserID, &o.Total, &o.Status, &o.CreatedAt)

    if errors.Is(err, pgx.ErrNoRows) {
        return Order{}, fmt.Errorf("order %s: %w", id, domain.ErrNotFound)
    }
    if err != nil {
        return Order{}, fmt.Errorf("query order: %w", err)
    }
    return o.toDomain(), nil
}

// orderRow — internal DB struct, не выходит из package
type orderRow struct {
    ID        string
    UserID    string
    Total     int64
    Status    string
    CreatedAt time.Time
}

func (r orderRow) toDomain() Order {
    return Order{
        ID:        OrderID(r.ID),
        UserID:    UserID(r.UserID),
        Total:     Money(r.Total),
        Status:    OrderStatus(r.Status),
        CreatedAt: r.CreatedAt,
    }
}
```

**Ключевые моменты:**
- `orderRow` — internal mapping struct, не выходит из package
- `toDomain()` явный mapping в domain object
- `pgx.ErrNoRows` → `domain.ErrNotFound` (нормализация ошибок)

### Repository и тесты

С интерфейсом легко мокать в тестах:

```go
type fakeOrderRepo struct {
    orders map[OrderID]Order
}

func (r *fakeOrderRepo) FindByID(ctx context.Context, id OrderID) (Order, error) {
    o, ok := r.orders[id]
    if !ok {
        return Order{}, domain.ErrNotFound
    }
    return o, nil
}

// В тесте
repo := &fakeOrderRepo{orders: map[OrderID]Order{
    "1": {ID: "1", Status: "pending"},
}}
svc := NewOrderService(repo, ...)
// Test без БД
```

Подробнее про тестирование — [09-testing-and-quality/](../../../09-testing-and-quality/).

### Generic repository — анти-паттерн

```go
// ❌ Generic repository "на все случаи"
type Repository[T any] interface {
    GetAll(ctx context.Context) ([]T, error)
    GetByID(ctx context.Context, id any) (T, error)
    Create(ctx context.Context, entity T) error
    Update(ctx context.Context, entity T) error
    Delete(ctx context.Context, id any) error
}
```

Звучит "DRY", но проблемы:
- **Нет domain-методов** — `MarkAsShipped` некуда добавить
- **`any` вместо типизированных ID** — теряется type safety
- **Все entity получают одинаковый API** — но реально они разные

Это **попытка переиспользовать boilerplate**, но boilerplate — не то ради чего создан Repository. Лучше — отдельный repository per aggregate с уникальными domain-методами.

---

## Unit of Work

Идея: объединить несколько storage-операций в одну транзакционную границу.

```go
// Интерфейс
type UnitOfWork interface {
    WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Реализация для PostgreSQL
type pgUnitOfWork struct {
    db *pgxpool.Pool
}

func (u *pgUnitOfWork) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
    tx, err := u.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    ctx = context.WithValue(ctx, txKey{}, tx)  // передать tx через context

    if err := fn(ctx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

// Использование в use case
func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) error {
    return s.uow.WithinTx(ctx, func(ctx context.Context) error {
        order := NewOrder(cmd)
        if err := s.orders.Save(ctx, order); err != nil {
            return err
        }
        if err := s.inventory.Reserve(ctx, order.Items); err != nil {
            return err
        }
        return s.events.Publish(ctx, OrderCreatedEvent{OrderID: order.ID})
    })
}
```

### Как UoW узнаёт про tx

В примере выше — через `context`:

```go
type txKey struct{}

// Помещаем tx в context при начале
ctx = context.WithValue(ctx, txKey{}, tx)

// В repository — достаём
func (r *pgOrderRepo) Save(ctx context.Context, order Order) error {
    if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
        // Используем transactional connection
        _, err := tx.Exec(ctx, "INSERT INTO orders ...", ...)
        return err
    }
    // Иначе — обычный pool
    _, err := r.db.Exec(ctx, "INSERT INTO orders ...", ...)
    return err
}
```

**Это работает потому что:**
- Repository прозрачен для use case (не знает про UoW)
- UoW не знает про конкретные репозитории
- Transactional connection распространяется через context

### Альтернатива: transactional repository

```go
type OrderRepository interface {
    Save(ctx context.Context, order Order) error
}

type TxOrderRepository interface {
    OrderRepository
    WithTx(tx pgx.Tx) OrderRepository
}

// Использование:
err := db.Tx(ctx, func(tx pgx.Tx) error {
    txOrders := orders.WithTx(tx)
    txInventory := inventory.WithTx(tx)
    // ...
})
```

Более explicit, но больше boilerplate. Подходит когда нужен fine-grained контроль.

### Когда UoW нужен

- **Несколько операций должны быть атомарны** — `Save order + Reserve inventory + Save outbox event`
- **Откат при ошибке** — если step 3 fails, step 1 и 2 откатываются
- **Outbox pattern** — write to outbox в той же tx что и business write. См. [../09-saga-and-outbox.md](../09-saga-and-outbox.md).

### Когда UoW не нужен

- **Одна операция в use case** — не оборачивать ради семантики
- **Eventual consistency** — операции не должны быть atomic, асинхронные
- **Read-only flows** — нет writes, нет нужды в tx

### UoW и distributed transactions

UoW работает **в рамках одной БД**. Если нужна "транзакция" через несколько сервисов — это distributed transaction, нужна **Saga**. См. [../09-saga-and-outbox.md](../09-saga-and-outbox.md).

UoW решает локальную atomicity. Saga решает eventually consistent распределённую.

---

## Связки между паттернами

Эти три паттерна часто работают вместе.

### Repository + UoW

Стандартная связка для use case с несколькими entity:

```go
func (s *OrderService) CreateOrderWithPayment(ctx context.Context, cmd CreateOrderCommand) error {
    return s.uow.WithinTx(ctx, func(ctx context.Context) error {
        // Все repositories — внутри одной транзакции через context
        order, _ := s.orders.Save(ctx, NewOrder(cmd))
        payment, _ := s.payments.Save(ctx, NewPayment(order))
        return s.outbox.Save(ctx, OrderCreatedEvent{...})
    })
}
```

### Strategy + Repository

Pricing strategy может зависеть от данных из БД:

```go
type DiscountStrategy interface {
    Calculate(ctx context.Context, order Order) (Money, error)
}

// Стратегия зависит от promo-кода из БД
type PromoDiscountStrategy struct {
    promos PromoRepository
}

func (s *PromoDiscountStrategy) Calculate(ctx context.Context, order Order) (Money, error) {
    promo, err := s.promos.FindByCode(ctx, order.PromoCode)
    if err != nil { ... }
    return promo.Apply(order), nil
}
```

Strategy декларирует "что считать", Repository — "откуда брать данные". Чисто разделено.

### Decorator + Repository

Очень частая связка — навешивать decorator'ы (cache, retry, metrics) на repository. См. [02-behavior-wrapping.md](./02-behavior-wrapping.md).

```go
var orders OrderRepository = postgres.NewOrderRepository(db)
orders = NewCachedOrderRepo(orders, cache, 5*time.Minute)
orders = NewInstrumentedOrderRepo(orders, metrics)
```

---

## Anti-patterns

**1. CRUD Repository.**

```go
// ❌ Просто обёртка над таблицей
type OrderRepository interface {
    Insert(ctx context.Context, o Order) error
    Update(ctx context.Context, o Order) error
    Delete(ctx context.Context, id string) error
    GetByID(ctx context.Context, id string) (Order, error)
    GetAll(ctx context.Context) ([]Order, error)
}
```

Нет domain-логики, просто лишний слой. Если CRUD — лучше работать с БД напрямую через sqlc или query builder.

**2. Repository знает про SQL.**

```go
// ❌ Repository принимает SQL фрагменты
type OrderRepository interface {
    Find(ctx context.Context, whereClause string, args ...any) ([]Order, error)
}

repo.Find(ctx, "status = $1 AND created_at > $2", "pending", time.Now())
```

SQL протекает в use case. Если завтра меняешь на NoSQL — переписывать все вызовы.

**3. UoW с side effects вне БД.**

```go
// ❌ В транзакции зависание на external API
uow.WithinTx(ctx, func(ctx) error {
    // Долгий external API call внутри tx
    resp, _ := http.Post("https://payments.example.com/charge", ...)
    return repo.Save(...)
})
```

Транзакция держит lock на rows на всё время HTTP call'а. Database connection заблокирован на секунды. Под нагрузкой — exhausted pool.

**Правило:** в транзакции — **только БД-операции**. External calls — до или после.

**4. Strategy через interface для двух fixed вариантов.**

```go
// ❌ Overengineering
type GreetStrategy interface { Greet() string }
type FormalGreet struct{}
type InformalGreet struct{}

// Хотя достаточно
func greet(formal bool) string {
    if formal { return "Good morning" }
    return "Hi"
}
```

Strategy нужен когда **расширение реально ожидается** или **поведение конфигурируемое**.

**5. UoW на каждом запросе.**

```go
// ❌ Каждый use case оборачивает в tx
func (s *UserService) GetUser(ctx context.Context, id string) (User, error) {
    var user User
    err := s.uow.WithinTx(ctx, func(ctx context.Context) error {
        user, err := s.users.FindByID(ctx, id)
        return err
    })
    return user, err
}
```

Single read не нужна транзакция. Read transactions используют pool connections и могут блокировать write'ы. Только когда **реально нужна atomicity**.

**6. Repository возвращает internal БД типы.**

```go
// ❌ Утечка pgx типов в domain
func (r *pgOrderRepo) FindByID(ctx context.Context, id string) (*pgx.Row, error) { ... }
```

Domain должен получать domain-объекты, не database-row-объекты. Маппинг — внутри repository.

---

## Чек-лист

```
□ Repository выражает domain-операции (FindActive, MarkAsShipped), не SQL-операции?
□ Repository возвращает domain objects, не database rows?
□ Ошибки нормализованы (sql.ErrNoRows → domain.ErrNotFound)?
□ Strategy уместен — есть 3+ реализаций или конфигурируется data?
□ UoW используется только когда реально нужна atomicity нескольких операций?
□ Внутри UoW нет external API calls?
□ Read-only use cases НЕ оборачиваются в tx без необходимости?
□ Generic repository НЕ используется — каждый aggregate имеет свой interface?
```

---

См. также:
- [01-dependencies-and-composition.md](./01-dependencies-and-composition.md) — small interfaces (про которые здесь много говорим)
- [02-behavior-wrapping.md](./02-behavior-wrapping.md) — decorator (часто навешивается на repository)
- [04-context-and-errors.md](./04-context-and-errors.md) — context (передача tx) и error wrapping
- [../05-ddd-in-go.md](../05-ddd-in-go.md) — глубокий разбор Entity, Aggregate, Domain Service
- [../09-saga-and-outbox.md](../09-saga-and-outbox.md) — UoW для outbox pattern, distributed transactions
