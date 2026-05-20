# Зависимости и композиция

Группа паттернов отвечающая на главный вопрос: **как компоненты находят друг друга**. В Go нет DI-контейнера типа Spring, всё делается через явные конструкторы и интерфейсы. Это требует дисциплины, но даёт прозрачную сборку и тестируемость.

Три ключевых паттерна:
- **Small interfaces** — кто описывает контракт
- **Constructor injection** — кто его получает
- **Functional options** — как настраивать без раздутого API

## Содержание

- [Small interfaces](#small-interfaces)
- [Constructor injection](#constructor-injection)
- [Functional options](#functional-options)
- [Связки и anti-patterns](#связки-и-anti-patterns)

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

**Почему это работает в Go:** интерфейсы satisfy implicitly. `PostgresUserStore` автоматически удовлетворяет `UserLoader`, `UserSaver`, `UserFinder` — без `implements` объявлений. Один тип может удовлетворять десятки маленьких интерфейсов из разных пакетов.

**Связь с SOLID:** это ISP (Interface Segregation Principle) + DIP (Dependency Inversion Principle) одновременно. Потребитель зависит от **узкого** контракта, который он сам и определяет. Подробнее: [../06-solid-in-go.md](../06-solid-in-go.md).

**Когда интерфейс не нужен:**
- Есть только одна реализация и она никогда не менялась
- Компонент не тестируется в изоляции
- Это internal utility без внешних зависимостей

В Go распространённая ошибка — **превентивные интерфейсы**. Заранее объявить `UserStoreInterface` "на случай если когда-то понадобится мокать". Не надо. Объяви интерфейс **когда появится второй пользователь** или **когда понадобится мок в тесте**.

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

### Почему это важно

**1. Тестируемость.**

```go
// В тесте
fakeRepo := &FakeOrderRepository{}
fakeEvents := &FakeEventPublisher{}
fakeLog := &TestLogger{}

svc := service.NewOrderService(fakeRepo, fakeEvents, fakeLog)
// Проверяем что svc.CreateOrder делает правильные вызовы
```

Без DI пришлось бы делать integration test с реальной БД и Kafka.

**2. Видимость зависимостей.**

Прочитав сигнатуру `NewOrderService(orders, events, log)`, ты знаешь **что нужно** этому сервису. Без DI приходится читать весь implementation чтобы понять что он использует внутри.

**3. Lifecycle management.**

В `main.go` ясно когда что создаётся и в каком порядке закрывается. С глобальным состоянием — не понятно когда `defer close()` ставить.

### Типичные ошибки

```go
// 1. os.Getenv внутри NewXxx
func NewClient() *Client {
    return &Client{
        url: os.Getenv("API_URL"),  // ❌ — нельзя тестировать с другим URL
    }
}

// 2. Глобальные переменные
var globalDB *sql.DB
func init() { globalDB = ... }  // ❌

// 3. Слишком много зависимостей (10+)
func NewService(a, b, c, d, e, f, g, h, i, j) *Service { ... }
// Если 10+ зависимостей — скорее всего сервис делает слишком много (нарушение SRP).
// Раздели или сгруппируй (например, "infra struct" с несколькими client'ами).

// 4. Скрытая инициализация в struct field
type Service struct {
    db *sql.DB  // ok, если передан через конструктор
    cache *Cache  // BAD if инициализируется в методе через sync.Once
}
```

### DI-фреймворки в Go

Есть [google/wire](https://github.com/google/wire), [uber-go/fx](https://github.com/uber-go/fx), [dig](https://github.com/uber-go/dig). Они **генерируют** или **resolve в runtime** dependency graph.

**Honest opinion:** для большинства Go-проектов **они не нужны**. Ручная сборка в `main.go` — 50-100 строк, понятна, отлаживается. DI-фреймворки имеет смысл когда:
- 50+ компонентов
- Сложные lifecycles (start/stop ordering)
- Команда привыкла к Spring/Dagger

Иначе — простой `main.go` лучше.

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

### Functional options vs Config struct

| | Functional options | Config struct |
|---|---|---|
| Добавить новый параметр | Добавить функцию, без breaking change | Добавить поле — backward compatible |
| Читаемость | Хорошая для библиотек | Хорошая для приложений |
| Defaults | Внутри конструктора | Надо явно задавать |
| Валидация | В каждой Option функции | В одном месте |
| Когда выбирать | Публичный API библиотеки | Config сервиса из YAML/ENV |

### Когда functional options оправданы

- **Библиотеки** (gRPC, HTTP-клиенты, БД-драйверы) — пользователь не должен заполнять все поля
- **5+ необязательных параметров** — иначе сигнатура `New` нечитаемая
- **Sensible defaults** — большинству пользователей подходят defaults

### Когда Config struct лучше

- **Приложение читает config из YAML/ENV** — естественно maps в struct
- **Все параметры обязательны и валидируются вместе**
- **2-3 параметра** — `New(url, timeout)` понятнее `New(url, WithTimeout(timeout))`

### Validation в functional options

Опции могут не только устанавливать значения, но и **возвращать ошибку**:

```go
type Option func(*Client) error

func WithTimeout(d time.Duration) Option {
    return func(c *Client) error {
        if d < 0 {
            return errors.New("timeout must be positive")
        }
        c.timeout = d
        return nil
    }
}

func NewClient(baseURL string, opts ...Option) (*Client, error) {
    c := &Client{...}
    for _, opt := range opts {
        if err := opt(c); err != nil {
            return nil, err
        }
    }
    return c, nil
}
```

Это удобно для библиотек где invalid configuration должен быть ошибкой compile-time-ish (early failure при инициализации).

---

## Связки и anti-patterns

### Композиция через embedding

Go не имеет наследования, но **embedding** — мощный механизм композиции:

```go
type BaseHandler struct {
    log *slog.Logger
    db  *sql.DB
}

type OrderHandler struct {
    BaseHandler  // ← embed, методы BaseHandler становятся методами OrderHandler
    orderRepo OrderRepository
}

// Можно вызвать h.log.Info(...) напрямую
// Но: не злоупотребляй, это легко превращается в наследование
```

**Когда уместно:**
- Действительно общие поля для группы хэндлеров (log, metrics, db)
- HTTP middleware-подобные wrapper'ы

**Когда плохо:**
- "Embed чтобы переиспользовать функцию" — лучше явный вызов
- "Embed чтобы получить is-a отношения" — это наследование, в Go не идиоматично

### Anti-patterns в этом разделе

**1. God interface.**
Один большой `IUserService` с 20 методами. Никто реально не нуждается во всех 20. Разбей на маленькие role-based интерфейсы.

**2. Скрытые зависимости.**
Конструктор внешне выглядит чистым, но внутри читает `os.Getenv` или `init()`-ит глобальные переменные. Невозможно тестировать.

**3. Functional options для всего.**
Простая структура с 2 параметрами не нуждается в `WithXxx()` обёртках. Перебор.

**4. Singleton через `init()`.**
Один глобальный объект, заинициализированный при импорте пакета. Невозможно reset в тестах, тяжело mock'ать.

**5. Service locator pattern.**
`Container.Get("user_service")` — runtime resolution, ошибки только при первом вызове. В Go идиоматичнее explicit DI через конструктор.

### Чек-лист

```
□ Интерфейс объявлен в пакете потребителя, не поставщика?
□ Размер интерфейса минимален (1-4 метода)?
□ Интерфейсы появляются только когда есть 2+ потребителя или нужен мок?
□ Зависимости передаются явно через конструктор?
□ В конструкторе нет os.Getenv, sql.Open, глобальных переменных?
□ Functional options использованы только в библиотеках с 5+ опциональными параметрами?
□ Defaults sensible — большинство пользователей не передают опций?
```

---

См. также:
- [02-behavior-wrapping.md](./02-behavior-wrapping.md) — следующий раздел про middleware/adapter/decorator
- [../06-solid-in-go.md](../06-solid-in-go.md) — SOLID принципы в Go контексте
- [../08-graceful-shutdown.md](../08-graceful-shutdown.md) — пример того как DI помогает корректно остановить компоненты
