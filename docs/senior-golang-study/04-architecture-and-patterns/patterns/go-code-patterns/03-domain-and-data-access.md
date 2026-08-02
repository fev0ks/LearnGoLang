# Домен и доступ к данным

## Содержание

- [Strategy](#strategy)
- [Repository](#repository)
- [Unit of Work](#unit-of-work)
- [Связки между паттернами](#связки-между-паттернами)
- [Антипаттерны](#антипаттерны)
- [Чек-лист](#чек-лист)

Три паттерна, отделяющих бизнес-логику от деталей хранения и реализации. Все они проводят одну и ту же границу: между тем, что система делает, и тем, как именно это выполняется.

- **Strategy** — сменное поведение или алгоритм.
- **Repository** — доступ к хранилищу на языке домена.
- **Unit of Work** — граница транзакции, охватывающая несколько операций.

В Go все три выглядят проще, чем в классическом объектном исполнении: роль абстрактного класса берёт на себя интерфейс, роль объекта-стратегии часто — обычная функция.

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
    return order.Subtotal, nil
}

// Скидка задаётся в процентах и считается в минорных единицах:
// дробная доля в деньгах приводит к расхождению копеек.
type DiscountPricing struct{ percent int64 }
func (d DiscountPricing) Calculate(ctx context.Context, order Order) (Money, error) {
    return order.Subtotal - order.Subtotal*Money(d.percent)/100, nil
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

**Что меняется:**
- Новая стратегия добавляется без правки `calculatePrice` — уже проверенный код остаётся нетронутым.
- В тесте стратегия подменяется, и проверять расчёт можно отдельно от выбора.
- Выбор стратегии собран в одном месте (`selectPricing`), а не размазан по ветвлению внутри расчёта.

### Interface vs function

Стратегия оформляется функцией, когда у неё нет состояния, нужен один метод, а само преобразование зависит только от аргументов:

```go
type Comparator func(a, b Item) int
type Validator func(input string) error
type Hasher func(s string) string
```

Стратегия оформляется интерфейсом, когда ей нужны собственные зависимости (например, репозиторий), методов больше одного или создание требует настройки:

```go
type AuthStrategy interface {
    Authenticate(ctx context.Context, token string) (*User, error)
    Refresh(ctx context.Context, refreshToken string) (string, error)
}
```

При прочих равных функция предпочтительнее: она не требует объявления типа и метода, а в тесте заменяется литералом прямо в месте вызова.

### Когда Strategy уместен

- **Бизнес-правило с несколькими реализациями** — расчёт цены, скоринг, ранжирование.
- **Алгоритм зависит от обстоятельств вызова** — порядок сортировки, политика повторов.
- **Точка расширения для пользователя библиотеки** — своя реализация подключается снаружи.

### Когда избыточен

- **Два варианта, которые не растут** — обычный `if` читается быстрее, чем интерфейс с двумя реализациями.
- **Поведение стабильно** — вариантов нет и не предвидится, а точка расширения требует сопровождения.

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

Имена задают ожидания. `Find` означает, что отсутствие результата — нормальный исход, и вызывающий код обязан обработать `ErrNotFound`. `Save` означает запись без разделения на вставку и обновление: решение о том, что именно выполнить, принимает реализация, а не вызывающий.

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

**Три решения в этом коде:**
- `orderRow` не покидает пакет. Это отражение строки таблицы, и если оно уйдёт наружу, схема хранения станет частью контракта.
- Преобразование выполняется явным методом `toDomain`, а не тегами: доменный тип может иметь закрытые поля и инварианты, которые автоматическое отображение обойдёт.
- `pgx.ErrNoRows` превращается в `domain.ErrNotFound`. Без этого вызывающий код обязан импортировать `pgx`, чтобы отличить «не найдено» от настоящей ошибки.

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

Выглядит как соблюдение DRY, но проблемы такие:

- **Доменным операциям негде жить.** `MarkAsShipped` в общий интерфейс не помещается, и появляется второй интерфейс рядом.
- **`any` вместо типизированного идентификатора.** Компилятор перестаёт ловить передачу `UserID` туда, где ожидался `OrderID`.
- **Одинаковый набор операций для всех сущностей.** У одних нет удаления, у других нет обновления, и лишние методы приходится реализовывать заглушками.

Такой интерфейс переиспользует однотипный код, но однотипный код — не то, ради чего вводится repository. Его смысл в выражении доменных операций, а он у каждого агрегата свой.

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

Вариант явнее: транзакция видна в сигнатурах и не прячется в контексте. Платой становится дополнительный метод `WithTx` у каждого репозитория. Вариант с контекстом короче, но у него есть скрытая опасность: если репозиторий забудет достать транзакцию из контекста, он выполнит запрос мимо неё — компилятор такую ошибку не заметит, а атомарность потеряется.

### Когда UoW нужен

- **Несколько записей обязаны примениться вместе** — сохранить заказ, зарезервировать товар, записать событие в outbox.
- **Нужен откат при ошибке** — сбой на третьем шаге отменяет первые два.
- **Реализуется outbox** — событие пишется той же транзакцией, что и бизнес-данные; разбор в [Saga и Outbox](../09-saga-and-outbox.md).

### Когда UoW не нужен

- **В сценарии одна операция** — оборачивать её ради единообразия не нужно.
- **Согласованность отложенная** — операции связаны событиями, а не общей транзакцией.
- **Сценарий только читает** — записи нет, транзакция ничего не защищает.

### UoW и distributed transactions

Unit of Work работает в границах одной базы. Как только в операции участвует второй сервис или второе хранилище, общей транзакции больше нет, и вместо неё нужна saga — с компенсациями вместо отката. Разбор — в [Saga и Outbox](../09-saga-and-outbox.md).

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

Разделение здесь чёткое: стратегия отвечает за то, что именно считается, репозиторий — за то, откуда берутся данные для расчёта. Стратегию можно протестировать с подменённым репозиторием, не заглядывая в базу.

### Decorator + Repository

Очень частая связка — навешивать decorator'ы (cache, retry, metrics) на repository. См. [02-behavior-wrapping.md](./02-behavior-wrapping.md).

```go
var orders OrderRepository = postgres.NewOrderRepository(db)
orders = NewCachedOrderRepo(orders, cache, 5*time.Minute)
orders = NewInstrumentedOrderRepo(orders, metrics)
```

---

## Антипаттерны

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

Такой интерфейс повторяет операции таблицы и не добавляет ни одного доменного понятия — он лишь удлиняет путь от сценария до SQL. Если логика действительно сводится к CRUD, честнее работать с базой напрямую через `sqlc` или построитель запросов.

**2. Repository знает про SQL.**

```go
// ❌ Repository принимает SQL фрагменты
type OrderRepository interface {
    Find(ctx context.Context, whereClause string, args ...any) ([]Order, error)
}

repo.Find(ctx, "status = $1 AND created_at > $2", "pending", time.Now())
```

SQL при этом оказывается в сценарии использования, то есть ровно там, откуда его убирал repository. Смена хранилища или даже переименование колонки требует правок во всех вызовах, а не в одной реализации.

**3. UoW с side effects вне БД.**

```go
// ❌ В транзакции зависание на external API
uow.WithinTx(ctx, func(ctx) error {
    // Долгий external API call внутри tx
    resp, _ := http.Post("https://payments.example.com/charge", ...)
    return repo.Save(...)
})
```

Транзакция удерживает блокировки строк всё время сетевого вызова, а соединение из пула не возвращается. Внешний сервис, отвечающий за три секунды вместо трёхсот миллисекунд, превращается в исчерпанный пул соединений и отказ всего сервиса — включая те запросы, которые к этому внешнему сервису отношения не имеют.

**Правило:** внутри транзакции — только операции с базой. Внешние вызовы выполняются до или после, а связь между ними обеспечивает outbox.

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

Strategy оправдан там, где расширение действительно ожидается или поведение настраивается снаружи. Для двух неизменных вариантов интерфейс с двумя реализациями требует больше чтения, чем одно ветвление.

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

Одиночному чтению транзакция не нужна: одиночный запрос и так атомарен. При этом явная транзакция занимает соединение из пула на всё время работы функции и в некоторых уровнях изоляции удерживает снимок данных, мешая очистке старых версий строк. Транзакция вводится тогда, когда атомарность действительно требуется.

**6. Repository возвращает internal БД типы.**

```go
// ❌ Утечка pgx типов в domain
func (r *pgOrderRepo) FindByID(ctx context.Context, id string) (*pgx.Row, error) { ... }
```

Домен обязан получать доменные объекты. Возврат `*pgx.Row` означает, что вызывающий код будет разбирать строку сам — то есть знать схему таблицы, порядок колонок и типы драйвера. Преобразование выполняется внутри репозитория, и это его основная работа, а не формальность.

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
