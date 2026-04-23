# DDD в Go

Domain-Driven Design — это подход к проектированию ПО, при котором модель предметной области является центром архитектуры. В Go DDD хорошо ложится на пакеты, интерфейсы и struct'ы — без сложных иерархий классов.

## Содержание

- [Стратегический DDD](#стратегический-ddd)
- [Тактический DDD: обзор](#тактический-ddd-обзор)
- [Entity](#entity)
- [Value Object](#value-object)
- [Aggregate](#aggregate)
- [Domain Events](#domain-events)
- [Repository](#repository)
- [Domain Service](#domain-service)
- [Application Service](#application-service)
- [Структура пакетов](#структура-пакетов)
- [Когда DDD нужен, а когда нет](#когда-ddd-нужен-а-когда-нет)
- [Типичные ошибки в Go](#типичные-ошибки-в-go)
- [Interview-ready answer](#interview-ready-answer)

---

## Стратегический DDD

Стратегический уровень — о том, *как разбить сложную систему на части* и *как части взаимодействуют*.

### Ubiquitous Language

Единый язык между разработчиками и экспертами предметной области. Используется в коде: имена типов, методов, переменных, пакетов — из домена, а не из технических слоёв.

```go
// Плохо — технические имена
type UserRecord struct { ... }
func (r *UserRecord) UpdateStatus(s int) {}

// Хорошо — язык домена
type Order struct { ... }
func (o *Order) Cancel(reason CancellationReason) error {}
func (o *Order) Confirm() error {}
```

### Bounded Context

Граница, внутри которой термины и модели имеют строго определённый смысл.

Одно слово может означать разное в разных контекстах:

| Контекст | `User` | `Product` |
|---|---|---|
| Accounts | Учётная запись, роли, auth | — |
| Orders | Покупатель с адресом доставки | Товар в позиции заказа |
| Inventory | — | Единица склада, остатки |
| Billing | Плательщик, реквизиты | Услуга для выставления счёта |

Каждый bounded context — отдельная модель, отдельный код. **Не надо создавать единый God-объект `User` на все случаи жизни.**

### Context Map — отношения между контекстами

```
 ┌─────────────┐         ┌─────────────────┐
 │   Orders    │ ──ACL── │  External        │
 │   Context   │         │  Payment Gateway │
 └──────┬──────┘         └─────────────────┘
        │ Customer-Supplier
        ▼
 ┌─────────────┐
 │  Inventory  │
 │   Context   │
 └─────────────┘
```

| Паттерн интеграции | Смысл |
|---|---|
| **Shared Kernel** | Общий код (типы, логика) — только для тесно связанных контекстов одной команды |
| **Customer-Supplier** | Один контекст (supplier) публикует API, другой (customer) его использует |
| **ACL (Anti-Corruption Layer)** | Адаптер-переводчик: защищает свою модель от чужой |
| **Open Host Service** | Публичный API для множества потребителей (не меняется под каждого) |
| **Conformist** | Просто копируем модель поставщика — когда нет сил бороться |

---

## Тактический DDD: обзор

Тактический уровень — строительные блоки модели внутри bounded context.

```
┌─────────────────────────────────────────────────────────────┐
│                      Bounded Context                         │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                    Aggregate                        │    │
│  │                                                     │    │
│  │   ┌─────────────┐      ┌─────────────────────┐     │    │
│  │   │ Aggregate   │      │  Child Entity /      │     │    │
│  │   │    Root     │─────▶│  Value Object        │     │    │
│  │   │  (Entity)   │      │                     │     │    │
│  │   └─────────────┘      └─────────────────────┘     │    │
│  │          │                                          │    │
│  │          │ raises                                   │    │
│  │          ▼                                          │    │
│  │   ┌─────────────┐                                   │    │
│  │   │Domain Event │                                   │    │
│  │   └─────────────┘                                   │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐   │
│  │ Repository  │   │   Domain    │   │   Application   │   │
│  │ (interface) │   │   Service   │   │    Service      │   │
│  └─────────────┘   └─────────────┘   └─────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Entity

**Entity** — объект с уникальной идентичностью. Два объекта с одинаковым ID — один и тот же, даже если все поля разные (изменились со временем).

```go
package domain

import "time"

// OrderID — типизированный идентификатор (не просто string/int)
type OrderID string

// Order — Entity: тождество по OrderID
type Order struct {
    id        OrderID     // unexported — инвариант защищён
    customerID CustomerID
    status    OrderStatus
    items     []OrderItem
    createdAt time.Time
    updatedAt time.Time

    events []DomainEvent // накапливаем события, не публикуем сразу
}

// ID — единственный способ получить идентификатор снаружи
func (o *Order) ID() OrderID { return o.id }

// Equality по ID, а не по значению полей
func (o *Order) Equal(other *Order) bool {
    return o.id == other.id
}

// Поведение — методы, защищающие инварианты
func (o *Order) Cancel(reason CancellationReason) error {
    if o.status == OrderStatusCancelled {
        return ErrAlreadyCancelled
    }
    if o.status == OrderStatusShipped {
        return ErrCannotCancelShipped
    }

    o.status = OrderStatusCancelled
    o.updatedAt = time.Now()
    o.events = append(o.events, OrderCancelledEvent{
        OrderID:   o.id,
        Reason:    reason,
        CancelledAt: o.updatedAt,
    })
    return nil
}

func (o *Order) PopEvents() []DomainEvent {
    evts := o.events
    o.events = nil
    return evts
}
```

**Ключевые свойства Entity в Go:**
- Поля закрыты (unexported) — внешний код не может нарушить инварианты
- Конструктор валидирует и возвращает ошибку
- Поведение (методы) — на указателе, изменяют состояние

---

## Value Object

**Value Object** — объект без идентичности. Тождество по значению всех полей. Immutable.

```go
package domain

import (
    "errors"
    "fmt"
)

// Money — Value Object: 100 USD == 100 USD, независимо от "экземпляра"
type Money struct {
    amount   int64  // в копейках/центах — никогда float64!
    currency string // ISO 4217: "USD", "EUR", "RUB"
}

func NewMoney(amount int64, currency string) (Money, error) {
    if amount < 0 {
        return Money{}, errors.New("amount cannot be negative")
    }
    if len(currency) != 3 {
        return Money{}, fmt.Errorf("invalid currency: %s", currency)
    }
    return Money{amount: amount, currency: currency}, nil
}

// Immutable операции — возвращают новый объект
func (m Money) Add(other Money) (Money, error) {
    if m.currency != other.currency {
        return Money{}, fmt.Errorf("currency mismatch: %s vs %s", m.currency, other.currency)
    }
    return Money{amount: m.amount + other.amount, currency: m.currency}, nil
}

func (m Money) Multiply(factor int64) Money {
    return Money{amount: m.amount * factor, currency: m.currency}
}

// Equality по значению (встроенная в Go для struct без указателей)
func (m Money) Equal(other Money) bool {
    return m.amount == other.amount && m.currency == other.currency
}

func (m Money) IsZero() bool  { return m.amount == 0 }
func (m Money) Amount() int64  { return m.amount }
func (m Money) Currency() string { return m.currency }
func (m Money) String() string {
    return fmt.Sprintf("%d %s", m.amount, m.currency)
}
```

```go
// Email — Value Object с валидацией
type Email struct {
    value string
}

func NewEmail(raw string) (Email, error) {
    if !strings.Contains(raw, "@") {
        return Email{}, fmt.Errorf("invalid email: %s", raw)
    }
    return Email{value: strings.ToLower(strings.TrimSpace(raw))}, nil
}

func (e Email) String() string { return e.value }
```

**Когда Value Object, а когда Entity:**

| | Value Object | Entity |
|---|---|---|
| Тождество | По значению всех полей | По ID |
| Мутабельность | Immutable | Mutable |
| Примеры | Money, Email, Address, DateRange | Order, User, Product |
| Хранение | Встраивается в Entity | Отдельная запись в БД |

---

## Aggregate

**Aggregate** — кластер Entity и Value Object, объединённых единой границей транзакционной согласованности. Один из объектов — **Aggregate Root** — единственная точка входа.

**Главное правило:** одна транзакция изменяет один aggregate.

```go
package domain

import (
    "errors"
    "time"
)

// Order — Aggregate Root
type Order struct {
    id         OrderID
    customerID CustomerID
    status     OrderStatus
    items      []OrderItem  // дочерние Entity — доступны только через Order
    total      Money
    address    ShippingAddress // Value Object
    createdAt  time.Time
    events     []DomainEvent
}

// OrderItem — дочерняя Entity внутри Aggregate
// Не имеет смысла вне заказа; доступна только через Order
type OrderItem struct {
    id        OrderItemID
    productID ProductID
    quantity  int
    price     Money
}

// Конструктор — единственный способ создать валидный Order
func NewOrder(id OrderID, customerID CustomerID, address ShippingAddress) (*Order, error) {
    if id == "" {
        return nil, errors.New("order id is required")
    }
    if customerID == "" {
        return nil, errors.New("customer id is required")
    }
    o := &Order{
        id:         id,
        customerID: customerID,
        status:     OrderStatusDraft,
        address:    address,
        createdAt:  time.Now(),
    }
    return o, nil
}

// AddItem — изменение состояния через метод, не прямой append
func (o *Order) AddItem(productID ProductID, qty int, price Money) error {
    if o.status != OrderStatusDraft {
        return ErrOrderNotDraft
    }
    if qty <= 0 {
        return errors.New("quantity must be positive")
    }

    // Если товар уже в заказе — увеличиваем количество
    for i, item := range o.items {
        if item.productID == productID {
            o.items[i].quantity += qty
            o.recalcTotal()
            return nil
        }
    }

    o.items = append(o.items, OrderItem{
        id:        newOrderItemID(),
        productID: productID,
        quantity:  qty,
        price:     price,
    })
    o.recalcTotal()
    return nil
}

// Place — перевод в статус "размещён", проверка инвариантов
func (o *Order) Place() error {
    if o.status != OrderStatusDraft {
        return ErrOrderNotDraft
    }
    if len(o.items) == 0 {
        return ErrEmptyOrder
    }

    o.status = OrderStatusPlaced
    o.events = append(o.events, OrderPlacedEvent{
        OrderID:    o.id,
        CustomerID: o.customerID,
        Total:      o.total,
        PlacedAt:   time.Now(),
    })
    return nil
}

func (o *Order) recalcTotal() {
    total, _ := NewMoney(0, "USD")
    for _, item := range o.items {
        lineTotal := item.price.Multiply(int64(item.quantity))
        total, _ = total.Add(lineTotal)
    }
    o.total = total
}

// Getters — только нужные данные наружу
func (o *Order) ID() OrderID         { return o.id }
func (o *Order) Status() OrderStatus  { return o.status }
func (o *Order) Total() Money         { return o.total }
func (o *Order) Items() []OrderItem   { return append([]OrderItem{}, o.items...) } // копия!
func (o *Order) PopEvents() []DomainEvent {
    evts := o.events
    o.events = nil
    return evts
}
```

**Правила aggregate:**
1. Внешний код работает только с Aggregate Root — не с дочерними объектами напрямую
2. Дочерние объекты (OrderItem) не передаются наружу для мутации — только копии
3. Все инварианты aggregate защищены внутри методов root
4. Один aggregate — одна транзакция

---

## Domain Events

**Domain Event** — факт, произошедший в домене. Всегда в прошедшем времени.

```go
package domain

import "time"

// DomainEvent — маркерный интерфейс
type DomainEvent interface {
    EventName() string
    OccurredAt() time.Time
}

// OrderPlacedEvent — конкретное событие
type OrderPlacedEvent struct {
    OrderID    OrderID
    CustomerID CustomerID
    Total      Money
    PlacedAt   time.Time
}

func (e OrderPlacedEvent) EventName() string   { return "order.placed" }
func (e OrderPlacedEvent) OccurredAt() time.Time { return e.PlacedAt }

// OrderCancelledEvent
type OrderCancelledEvent struct {
    OrderID     OrderID
    Reason      CancellationReason
    CancelledAt time.Time
}

func (e OrderCancelledEvent) EventName() string   { return "order.cancelled" }
func (e OrderCancelledEvent) OccurredAt() time.Time { return e.CancelledAt }
```

**Два подхода к публикации событий:**

```go
// Подход 1: Outbox через Application Service
// Events накапливаются в aggregate, Application Service публикует после сохранения
func (s *OrderService) PlaceOrder(ctx context.Context, cmd PlaceOrderCmd) error {
    order, err := s.repo.FindByID(ctx, cmd.OrderID)
    if err != nil {
        return err
    }

    if err := order.Place(); err != nil {
        return err
    }

    // Сохраняем + публикуем в одной транзакции (Outbox паттерн)
    return s.repo.SaveWithEvents(ctx, order, order.PopEvents())
}

// Подход 2: In-process EventBus (для modular monolith)
type EventBus interface {
    Publish(ctx context.Context, events ...DomainEvent) error
}

func (s *OrderService) PlaceOrder(ctx context.Context, cmd PlaceOrderCmd) error {
    order, err := s.repo.FindByID(ctx, cmd.OrderID)
    if err != nil {
        return err
    }
    if err := order.Place(); err != nil {
        return err
    }
    if err := s.repo.Save(ctx, order); err != nil {
        return err
    }
    // После успешного сохранения — публикуем (at-most-once в памяти)
    return s.eventBus.Publish(ctx, order.PopEvents()...)
}
```

---

## Repository

**Repository** — абстракция хранилища для aggregate. Интерфейс объявляется в **domain**, реализация — в **infra**.

```go
// domain/ports.go — интерфейс: domain ничего не знает о PostgreSQL
package domain

import "context"

type OrderRepository interface {
    FindByID(ctx context.Context, id OrderID) (*Order, error)
    FindByCustomer(ctx context.Context, customerID CustomerID) ([]*Order, error)
    Save(ctx context.Context, order *Order) error
    Delete(ctx context.Context, id OrderID) error
}

// Доменные ошибки — тоже в domain
var (
    ErrOrderNotFound  = errors.New("order not found")
    ErrOrderConflict  = errors.New("order already exists")
)
```

```go
// infra/postgres/order_repo.go — реализация
package postgres

import (
    "context"
    "errors"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/myapp/internal/domain"
)

type orderRepository struct {
    db *pgxpool.Pool
}

func NewOrderRepository(db *pgxpool.Pool) domain.OrderRepository {
    return &orderRepository{db: db}
}

func (r *orderRepository) FindByID(ctx context.Context, id domain.OrderID) (*domain.Order, error) {
    // 1. Запрос к БД — получаем "сырые" данные
    row := r.db.QueryRow(ctx, `
        SELECT id, customer_id, status, total_amount, total_currency, created_at
        FROM orders WHERE id = $1
    `, string(id))

    var rec orderRecord
    if err := row.Scan(
        &rec.ID, &rec.CustomerID, &rec.Status,
        &rec.TotalAmount, &rec.TotalCurrency, &rec.CreatedAt,
    ); err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, domain.ErrOrderNotFound
        }
        return nil, err
    }

    // 2. Загружаем items
    items, err := r.findItems(ctx, id)
    if err != nil {
        return nil, err
    }

    // 3. Маппинг record → domain object (reconstitute)
    return reconstitute(rec, items), nil
}

func (r *orderRepository) Save(ctx context.Context, order *domain.Order) error {
    // Upsert через ON CONFLICT
    _, err := r.db.Exec(ctx, `
        INSERT INTO orders (id, customer_id, status, total_amount, total_currency, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW())
        ON CONFLICT (id) DO UPDATE SET
            status = EXCLUDED.status,
            total_amount = EXCLUDED.total_amount,
            updated_at = EXCLUDED.updated_at
    `, string(order.ID()), /* ... */ )
    return err
}
```

**Важно:** Repository работает с aggregate целиком — нет `UpdateStatus(id, status)`. Загружаем → меняем → сохраняем.

---

## Domain Service

**Domain Service** — логика, которая не принадлежит ни одному конкретному aggregate, но является частью домена.

```go
// domain/pricing_service.go
package domain

// PricingService — domain service для расчёта цены
// Не является Entity/VO — это операция над несколькими объектами
type PricingService struct {
    discountRepo DiscountRepository
}

func NewPricingService(discountRepo DiscountRepository) *PricingService {
    return &PricingService{discountRepo: discountRepo}
}

// CalculateFinalPrice — бизнес-логика принадлежит домену, не application layer
func (s *PricingService) CalculateFinalPrice(
    ctx context.Context,
    order *Order,
    customer *Customer,
) (Money, error) {
    discounts, err := s.discountRepo.FindActive(ctx, customer.Tier())
    if err != nil {
        return Money{}, err
    }

    total := order.Total()
    for _, d := range discounts {
        if d.Applies(order) {
            total = d.Apply(total)
        }
    }
    return total, nil
}
```

**Когда нужен Domain Service:**
- Операция затрагивает несколько aggregates
- Операция требует доступа к внешним данным (репозиториям), но логика — доменная
- Нельзя естественно поместить в один aggregate без нарушения его границ

---

## Application Service

**Application Service** — оркестрирует use case: загружает aggregates, вызывает domain logic, сохраняет. Не содержит бизнес-логики.

```go
// usecase/place_order.go
package usecase

import (
    "context"
    "github.com/myapp/internal/domain"
)

type PlaceOrderCmd struct {
    OrderID    domain.OrderID
    CustomerID domain.CustomerID
    Items      []PlaceOrderItemCmd
    Address    domain.ShippingAddress
}

type PlaceOrderItemCmd struct {
    ProductID domain.ProductID
    Quantity  int
}

type PlaceOrderUseCase struct {
    orders    domain.OrderRepository
    products  domain.ProductRepository
    customers domain.CustomerRepository
    pricing   *domain.PricingService
    events    domain.EventPublisher
}

func NewPlaceOrderUseCase(
    orders domain.OrderRepository,
    products domain.ProductRepository,
    customers domain.CustomerRepository,
    pricing *domain.PricingService,
    events domain.EventPublisher,
) *PlaceOrderUseCase {
    return &PlaceOrderUseCase{
        orders: orders, products: products,
        customers: customers, pricing: pricing, events: events,
    }
}

func (uc *PlaceOrderUseCase) Execute(ctx context.Context, cmd PlaceOrderCmd) error {
    // 1. Загрузить нужные aggregates
    customer, err := uc.customers.FindByID(ctx, cmd.CustomerID)
    if err != nil {
        return err
    }

    // 2. Создать новый aggregate
    order, err := domain.NewOrder(cmd.OrderID, cmd.CustomerID, cmd.Address)
    if err != nil {
        return err
    }

    // 3. Добавить товары — domain logic защищает инварианты
    for _, item := range cmd.Items {
        product, err := uc.products.FindByID(ctx, item.ProductID)
        if err != nil {
            return err
        }
        if err := order.AddItem(product.ID(), item.Quantity, product.Price()); err != nil {
            return err
        }
    }

    // 4. Domain service — пересчёт с учётом скидок
    finalTotal, err := uc.pricing.CalculateFinalPrice(ctx, order, customer)
    if err != nil {
        return err
    }
    order.ApplyDiscount(finalTotal)

    // 5. Разместить заказ (domain logic)
    if err := order.Place(); err != nil {
        return err
    }

    // 6. Сохранить
    if err := uc.orders.Save(ctx, order); err != nil {
        return err
    }

    // 7. Опубликовать domain events
    return uc.events.Publish(ctx, order.PopEvents()...)
}
```

**Правило:** если вы пишете `if` с бизнес-условием в Application Service — это сигнал, что логика принадлежит домену.

---

## Структура пакетов

```
internal/orders/           ← Orders Bounded Context
├── domain/                ← ядро: Entity, VO, Aggregate, Domain Events, interfaces
│   ├── order.go           ← Order aggregate root
│   ├── order_item.go      ← OrderItem entity
│   ├── money.go           ← Money value object
│   ├── events.go          ← OrderPlacedEvent, OrderCancelledEvent
│   ├── errors.go          ← ErrOrderNotFound, ErrEmptyOrder, ...
│   └── ports.go           ← OrderRepository, EventPublisher (interfaces)
│
├── usecase/               ← Application Services (оркестрация)
│   ├── place_order.go
│   ├── cancel_order.go
│   └── place_order_test.go  ← unit tests с mock-репозиториями
│
├── infra/                 ← реализации interfaces из domain
│   ├── postgres/
│   │   └── order_repo.go
│   └── kafka/
│       └── event_publisher.go
│
└── transport/             ← HTTP/gRPC handlers
    └── http/
        └── handler.go
```

**Направление зависимостей:**
```
transport → usecase → domain ← infra
```

`domain` не зависит ни от кого. `infra` реализует интерфейсы из `domain`.

---

## Когда DDD нужен, а когда нет

| Признак | DDD полезен | DDD избыточен |
|---|---|---|
| Сложность | Сложные бизнес-правила, много инвариантов | CRUD с валидацией |
| Изменяемость | Правила меняются часто | Стабильная схема |
| Команда | Работа с domain experts | Только технические требования |
| Размер | Большой bounded context | Маленький сервис на 3-4 таблицы |
| Примеры | Страхование, банкинг, логистика | Блог, панель метрик, CMS |

**Практическое правило:** если можно описать всю логику как "валидировать и сохранить" — нужен CRUD, не DDD. Если есть состояния, переходы между ними, инварианты "заказ нельзя отменить после отгрузки" — DDD оправдан.

---

## Типичные ошибки в Go

### 1. Anemic Domain Model

```go
// Плохо — Entity без поведения (просто struct с полями)
type Order struct {
    ID     string
    Status string  // открытые поля, кто угодно меняет
    Items  []Item
}

// Логика вынесена в сервис (нарушение принципа)
func (s *OrderService) CancelOrder(o *Order) {
    if o.Status == "placed" {      // бизнес-правило снаружи aggregate
        o.Status = "cancelled"
    }
}

// Хорошо — поведение в агрегате
func (o *Order) Cancel(reason CancellationReason) error {
    // логика здесь, поля закрыты
}
```

### 2. Репозиторий знает о domain events

```go
// Плохо — репозиторий публикует события
func (r *orderRepo) Save(ctx context.Context, o *domain.Order) error {
    // сохранить...
    r.kafka.Publish(o.PopEvents()) // ❌ инфраструктура решает когда публиковать
    return nil
}

// Хорошо — Application Service управляет порядком
func (uc *PlaceOrderUseCase) Execute(ctx context.Context, cmd PlaceOrderCmd) error {
    // ...
    if err := uc.orders.Save(ctx, order); err != nil { return err }
    return uc.events.Publish(ctx, order.PopEvents()...) // ✓
}
```

### 3. Один aggregate ссылается на другой не через ID

```go
// Плохо — Order держит ссылку на Customer
type Order struct {
    customer *Customer  // ❌ нарушение границ aggregate
}

// Хорошо — ссылка только через ID
type Order struct {
    customerID CustomerID  // ✓ загружаем Customer отдельно когда нужно
}
```

### 4. Сохранять отдельные поля вместо aggregate целиком

```go
// Плохо — bypass aggregate, нарушение инвариантов
func (r *repo) UpdateStatus(ctx context.Context, id OrderID, status OrderStatus) error {
    _, err := r.db.Exec(`UPDATE orders SET status=$1 WHERE id=$2`, status, id)
    return err
}
// Любой может выставить статус напрямую, обойдя Cancel/Place

// Хорошо — загружаем → меняем через методы → сохраняем целиком
order, _ := repo.FindByID(ctx, id)
order.Cancel(reason)
repo.Save(ctx, order)
```

### 5. God aggregate

```go
// Плохо — один aggregate для всего
type Order struct {
    // заказ
    items    []OrderItem
    // доставка (должна быть отдельным aggregate)
    shipments []Shipment
    // платёж (тоже отдельный aggregate)
    payments  []Payment
    // рефанды
    refunds   []Refund
}
// При любом изменении любого аспекта — блокируем весь Order

// Хорошо — отдельные aggregates: Order, Shipment, Payment
// связаны через OrderID
```

---

## Interview-ready answer

> "DDD я воспринимаю на двух уровнях. Стратегический — это Bounded Context и Ubiquitous Language: разбить систему на части так, чтобы внутри каждой термины имели однозначный смысл, а между ними — явные контракты через ACL или Open Host Service. В Go это маппируется на пакеты с `internal/` границами.
>
> Тактический — это строительные блоки: Entity (тождество по ID), Value Object (тождество по значению, immutable — Money, Email), Aggregate (граница транзакционной согласованности, один root на aggregate), Domain Events (факты в прошедшем времени — OrderPlaced), Repository (интерфейс в domain, реализация в infra), Application Service (оркестрация без бизнес-логики).
>
> В Go это хорошо ложится: unexported поля защищают инварианты aggregate, интерфейсы из domain пакета инвертируют зависимости, domain events накапливаются в slice и публикуются Application Service после сохранения.
>
> Когда не нужен DDD: простой CRUD, нет сложных инвариантов, нет состояний с переходами. DDD окупается когда есть реальные бизнес-правила которые меняются, несколько команд с разными моделями, и когда ошибка в логике стоит дорого."