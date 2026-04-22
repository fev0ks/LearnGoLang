# Go Code Patterns

Go не поощряет тяжёлую объектную иерархию. Паттерны здесь выглядят проще: маленькие интерфейсы, функции, композиция, явные зависимости и тонкие адаптеры вокруг внешнего мира.

## Содержание

- [Обзор паттернов](#обзор-паттернов)
- [Small interfaces](#small-interfaces)
- [Constructor injection](#constructor-injection)
- [Functional options](#functional-options)
- [Middleware](#middleware)
- [Adapter](#adapter)
- [Decorator](#decorator)
- [Strategy](#strategy)
- [Repository](#repository)
- [Unit of Work](#unit-of-work)
- [Context boundaries](#context-boundaries)
- [Error wrapping and mapping](#error-wrapping-and-mapping)
- [Checklist](#checklist)
- [Interview-ready answer](#interview-ready-answer)

---

## Обзор паттернов

| Паттерн | Проблема которую решает | Когда не нужен |
|---|---|---|
| Small interface | Coupling к конкретной реализации | Интерфейс используется в одном месте |
| Constructor injection | Скрытые зависимости, global state | Нет внешних зависимостей |
| Functional options | Много необязательных параметров | Параметров ≤ 3, все обязательные |
| Middleware | Дублирование cross-cutting concerns | Одноразовый handler |
| Adapter | Внешний SDK протекает в домен | Простая интеграция без изменений |
| Decorator | Нужно расширить поведение без изменения кода | Добавляешь в одно место |
| Strategy | Большой `switch` по типу поведения | 2 варианта, не меняются |
| Repository | SQL/Redis детали в бизнес-логике | Простой CRUD без domain rules |
| Unit of Work | Несколько операций в одной транзакции | Одиночные операции |

---

## Small interfaces

Идея: интерфейс описывает минимальное поведение, которое нужно **потребителю**, а не поставщику.

```go
// Плохо: интерфейс объявлен рядом с реализацией
// postgres/user_store.go
type UserStoreInterface interface {
    GetByID(ctx context.Context, id int64) (User, error)
    Save(ctx context.Context, user User) error
    Delete(ctx context.Context, id int64) error
    List(ctx context.Context, filter Filter) ([]User, error)
    // ... 10 методов
}

// Хорошо: интерфейс объявлен в том пакете, который его использует
// service/notification.go
type UserLoader interface {
    GetByID(ctx context.Context, id int64) (User, error)
}

type NotificationService struct {
    users UserLoader  // нужен только GetByID
}
```

**Правило интерфейсов в Go:**

```
Поставщик           Потребитель
PostgresUserStore → объявляет UserLoader (1 метод)
                  → объявляет UserSaver  (1 метод)
                  → объявляет UserFinder (3 метода)
```

Размер интерфейса:
- `io.Reader` — 1 метод
- `io.ReadWriter` — 2 метода
- Твой `UserStore` в сервисе — обычно 2-4 метода

**Когда интерфейс не нужен:**
- Есть только одна реализация и она никогда не менялась
- Компонент не тестируется в изоляции
- Это internal utility без внешних зависимостей

---

## Constructor injection

Идея: зависимости передаются явно через конструктор, видны в сигнатуре.

```go
// Плохо: скрытые зависимости
func NewOrderService() *OrderService {
    db, _ := sql.Open("postgres", os.Getenv("DATABASE_URL"))  // скрыто!
    redis := redis.NewClient(&redis.Options{Addr: "localhost:6379"})  // скрыто!
    return &OrderService{db: db, cache: redis}
}

// Хорошо: явные зависимости
type OrderService struct {
    orders  OrderRepository
    events  EventPublisher
    log     Logger
}

func NewOrderService(orders OrderRepository, events EventPublisher, log Logger) *OrderService {
    return &OrderService{orders: orders, events: events, log: log}
}
```

**Dependency graph становится видимым в `main.go`:**

```go
func main() {
    db      := postgres.New(cfg.DatabaseURL)
    broker  := kafka.New(cfg.KafkaAddr)
    logger  := slog.New(...)

    orders  := postgres.NewOrderRepository(db)
    events  := kafka.NewEventPublisher(broker)

    svc := service.NewOrderService(orders, events, logger)
    // ...
}
```

**Типичная ошибка:** инициализация зависимостей внутри `New*` через `os.Getenv`, `sql.Open` или глобальные переменные. Это делает unit-тесты невозможными.

---

## Functional options

Идея: опциональные настройки передаются через функции, defaults устанавливаются внутри.

```go
type Client struct {
    baseURL    string
    timeout    time.Duration
    retries    int
    maxConns   int
}

type Option func(*Client)

func WithTimeout(d time.Duration) Option {
    return func(c *Client) { c.timeout = d }
}

func WithRetries(n int) Option {
    return func(c *Client) { c.retries = n }
}

func NewClient(baseURL string, opts ...Option) *Client {
    c := &Client{
        baseURL:  baseURL,
        timeout:  5 * time.Second,  // defaults
        retries:  3,
        maxConns: 100,
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}

// Использование
client := NewClient("https://api.example.com",
    WithTimeout(10*time.Second),
    WithRetries(5),
)
```

**Functional options vs Config struct:**

| | Functional options | Config struct |
|---|---|---|
| Добавить новый параметр | Добавить функцию, без breaking change | Добавить поле — backward compatible |
| Читаемость | Хорошая для библиотек | Хорошая для приложений |
| Defaults | Внутри конструктора | Надо явно задавать |
| Валидация | В каждой Option функции | В одном месте |
| Когда выбирать | Публичный API библиотеки | Config сервиса из YAML/ENV |

---

## Middleware

Идея: обернуть обработчик общей технической логикой без изменения кода handler'а.

```go
// Тип middleware
type Middleware func(http.Handler) http.Handler

// Logging
func Logging(log *slog.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            rw := &responseWriter{ResponseWriter: w}
            next.ServeHTTP(rw, r)
            log.Info("request",
                "method", r.Method,
                "path", r.URL.Path,
                "status", rw.status,
                "duration", time.Since(start),
            )
        })
    }
}

// Auth
func Auth(verifier TokenVerifier) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.Header.Get("Authorization")
            claims, err := verifier.Verify(token)
            if err != nil {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            ctx := context.WithValue(r.Context(), claimsKey, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Цепочка middleware:**

```
Request
  │
  ▼
┌─────────────┐
│  Recovery   │  ← паника не роняет сервер
└──────┬──────┘
       ▼
┌─────────────┐
│   Tracing   │  ← открыть span
└──────┬──────┘
       ▼
┌─────────────┐
│   Logging   │  ← залогировать запрос
└──────┬──────┘
       ▼
┌─────────────┐
│    Auth     │  ← проверить токен
└──────┬──────┘
       ▼
┌─────────────┐
│  RateLimit  │  ← ограничить частоту
└──────┬──────┘
       ▼
┌─────────────┐
│   Handler   │  ← бизнес-логика
└─────────────┘
```

Применение цепочки:
```go
chain := Alice(Recovery(), Tracing(), Logging(log), Auth(verifier), RateLimit(limiter))
http.Handle("/api/orders", chain(ordersHandler))
```

**Типичная ошибка:** класть бизнес-логику в middleware. Middleware — для технических cross-cutting concerns: логирование, auth, metrics, tracing, rate limiting.

---

## Adapter

Идея: привести внешний API к внутреннему интерфейсу, чтобы внешний SDK не проникал в домен.

```
Внешний мир          Adapter               Домен
─────────────────    ──────────────────    ─────────────────
stripe.Client   →    StripeAdapter    →    PaymentProvider
sendgrid.Client →    SendGridAdapter  →    EmailSender
s3.Client       →    S3Adapter        →    FileStorage
```

```go
// Интерфейс домена (не знает о Stripe)
type PaymentProvider interface {
    Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}

// Адаптер (знает о Stripe, изолирует детали)
type StripeAdapter struct {
    client *stripe.Client
}

func (a *StripeAdapter) Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error) {
    params := &stripe.ChargeParams{
        Amount:   stripe.Int64(req.Amount.Cents()),
        Currency: stripe.String(string(req.Currency)),
        Source:   &stripe.SourceParams{Token: stripe.String(req.Token)},
    }
    ch, err := a.client.Charges.New(params)
    if err != nil {
        return ChargeResult{}, mapStripeError(err)  // нормализация ошибок
    }
    return ChargeResult{ID: ch.ID, Status: mapStripeStatus(ch.Status)}, nil
}
```

**Что адаптер делает обязательно:**
1. Конвертирует типы (domain model ↔ external model)
2. Нормализует ошибки (stripe.Error → domain.PaymentError)
3. Изолирует изменения внешнего API в одном месте

---

## Decorator

Идея: добавить поведение к существующей реализации без изменения её кода, оборачивая через тот же интерфейс.

```
         ┌──────────────────────────────────────┐
         │   MetricsUserStore                   │
         │   ┌──────────────────────────────┐   │
         │   │   CachedUserStore            │   │
         │   │   ┌──────────────────────┐   │   │
         │   │   │   PostgresUserStore   │   │   │
         │   │   └──────────────────────┘   │   │
         │   └──────────────────────────────┘   │
         └──────────────────────────────────────┘
```

```go
// Кеш-декоратор
type CachedUserStore struct {
    next  UserStore
    cache Cache
    ttl   time.Duration
}

func (s *CachedUserStore) GetByID(ctx context.Context, id int64) (User, error) {
    key := fmt.Sprintf("user:%d", id)
    if user, ok := s.cache.Get(key); ok {
        return user.(User), nil
    }
    user, err := s.next.GetByID(ctx, id)
    if err != nil {
        return User{}, err
    }
    s.cache.Set(key, user, s.ttl)
    return user, nil
}

// Метрики-декоратор
type InstrumentedUserStore struct {
    next    UserStore
    metrics Metrics
}

func (s *InstrumentedUserStore) GetByID(ctx context.Context, id int64) (User, error) {
    start := time.Now()
    user, err := s.next.GetByID(ctx, id)
    s.metrics.RecordDuration("user_store.get_by_id", time.Since(start), err != nil)
    return user, err
}

// Сборка в main.go
var store UserStore = postgres.NewUserStore(db)
store = &CachedUserStore{next: store, cache: redisCache, ttl: 5 * time.Minute}
store = &InstrumentedUserStore{next: store, metrics: metrics}
```

**Типичные применения decorator:**

| Поведение | Описание |
|---|---|
| Cache | Кешировать результат в Redis/memory |
| Retry | Повторить при transient ошибке |
| Circuit breaker | Не вызывать при высоком error rate |
| Tracing | Добавить span вокруг вызова |
| Metrics | Замерить latency и error rate |
| Logging | Залогировать входные/выходные данные |

---

## Strategy

Идея: вынести изменяемый алгоритм или поведение за интерфейс, чтобы менять его независимо от остального кода.

```go
// Через интерфейс (когда нужно состояние или много методов)
type PricingStrategy interface {
    Calculate(ctx context.Context, order Order) (Money, error)
}

type RegularPricing struct{}
type DiscountPricing struct{ discount float64 }
type PromoPricing struct{ promoCode string }

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

**Strategy vs switch:**

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

**Когда Repository полезен, а когда нет:**

| | Полезен | Вреден |
|---|---|---|
| Бизнес-логика | Есть сложные domain rules | Простой CRUD без правил |
| Тестирование | Нужны unit-тесты без БД | Достаточно integration тестов |
| Абстракция | Может смениться storage | PostgreSQL навсегда |
| Запросы | Стабильные domain-операции | Много специфичных queries |
| Mapping | Сложный domain model ↔ DB | 1:1 маппинг таблицы в структуру |

**Практичное правило:** Repository должен выражать операции домена (`FindPending`, `CompleteOrder`), а не быть оберткой над таблицей (`GetAll`, `UpdateById`).

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

---

## Context boundaries

`context.Context` — не просто параметр для отмены. Это граница времени жизни запроса.

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
rows, err := db.QueryContext(ctx, query)     // ✓
resp, err := http.NewRequestWithContext(ctx, ...) // ✓
```

**Что уместно хранить в context:**
- Request ID / Trace ID (для logging/tracing)
- Authenticated user claims
- Transaction handle (для UoW паттерна)

---

## Error wrapping and mapping

**Три уровня работы с ошибками:**

```
Storage layer          Service layer          Transport layer
─────────────────      ─────────────────      ─────────────────
*pgconn.PgError    →   domain.NotFoundError → HTTP 404
*pgconn.PgError    →   domain.ConflictError → HTTP 409
context.DeadlineExceeded → (пробросить)    → HTTP 504
```

```go
// Storage layer: оборачивать с контекстом
func (r *pgOrderRepo) FindByID(ctx context.Context, id OrderID) (Order, error) {
    var o Order
    err := r.db.QueryRowContext(ctx, query, id).Scan(&o.ID, &o.Status)
    if errors.Is(err, sql.ErrNoRows) {
        return Order{}, fmt.Errorf("order %s: %w", id, domain.ErrNotFound)
    }
    if err != nil {
        return Order{}, fmt.Errorf("find order %s: %w", id, err)
    }
    return o, nil
}

// Transport layer: маппить на protocol коды
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
    order, err := h.svc.GetOrder(r.Context(), id)
    if err != nil {
        switch {
        case errors.Is(err, domain.ErrNotFound):
            http.Error(w, "not found", http.StatusNotFound)
        case errors.Is(err, domain.ErrForbidden):
            http.Error(w, "forbidden", http.StatusForbidden)
        default:
            http.Error(w, "internal error", http.StatusInternalServerError)
        }
        return
    }
    // ...
}
```

**Типичные ошибки:**

| Ошибка | Проблема |
|---|---|
| `return fmt.Errorf("failed")` | Потеряна причина, нельзя `errors.Is` |
| Возвращать `*pgconn.PgError` из service layer | Протечка storage деталей |
| Логировать одну ошибку на каждом уровне | Дублирование в логах |
| `log.Fatal` внутри library кода | Убивает программу, caller не может обработать |

---

## Checklist

```
□ Интерфейс объявлен в пакете потребителя, не поставщика?
□ Размер интерфейса минимален (1-4 метода)?
□ Зависимости передаются явно через конструктор?
□ В конструкторе нет os.Getenv, sql.Open, глобальных переменных?
□ Middleware занимается только технической логикой?
□ Adapter нормализует ошибки внешнего API?
□ Repository говорит языком домена, не SQL?
□ Transaction boundary явная и контролируемая?
□ Ошибки оборачиваются с %w и маппятся на границе transport?
□ context.Context первый аргумент, не хранится в struct?
```

---

## Interview-ready answer

> "В Go я чаще всего использую small interfaces, constructor injection, middleware, adapter, decorator и repository там где есть реальная domain-логика. Но я не переношу GoF один-в-один — из-за интерфейсов, функций и композиции Go требует значительно меньше слоёв.
>
> Хороший Go-паттерн делает зависимости явными, упрощает тесты и изолирует внешний мир за интерфейсом. Плохой паттерн добавляет абстракции которые ничего не защищают. Например, Repository поверх простого CRUD без domain-правил — это просто лишний слой. А вот Decorator для кеширования или метрик — честная изоляция без изменения основной логики.
>
> Главный индикатор: можно ли протестировать use case без поднятия настоящей БД, брокера и внешних сервисов? Если нет — скорее всего не хватает правильных интерфейсов или зависимости скрыты."
