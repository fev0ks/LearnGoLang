# Зависимости и композиция

## Содержание

- [Small interfaces](#small-interfaces)
- [Constructor injection](#constructor-injection)
- [Functional options](#functional-options)
- [Связки и антипаттерны](#связки-и-антипаттерны)

Группа паттернов, отвечающих на один вопрос: как компоненты находят друг друга. Контейнера внедрения зависимостей в духе Spring в Go нет, сборка выполняется явными конструкторами и интерфейсами. Это требует дисциплины, зато граф зависимостей виден целиком в одном файле, а не собирается по аннотациям.

Три паттерна закрывают три части вопроса:

- **Small interfaces** — кто описывает контракт.
- **Constructor injection** — кто его получает.
- **Functional options** — как настраивать компонент, не раздувая сигнатуру конструктора.

---

## Small interfaces

Идея: интерфейс описывает минимальное поведение, нужное потребителю, а не полный набор возможностей поставщика.

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

Ориентиры по размеру:

- `io.Reader` — один метод.
- `io.ReadWriter` — два.
- Интерфейс хранилища внутри сервиса — обычно два-четыре.

**Почему это работает в Go.** Соответствие интерфейсу неявное: `PostgresUserStore` удовлетворяет `UserLoader`, `UserSaver` и `UserFinder` одновременно, не объявляя об этом ни строкой. Один тип может подходить под десятки узких интерфейсов из разных пакетов, и добавление нового интерфейса не требует его правки.

**Связь с SOLID.** Это ISP и DIP сразу: потребитель зависит от узкого контракта, который сам же и определяет. Подробнее — в [SOLID в Go](../06-solid-in-go.md).

**Когда интерфейс не нужен:**
- Есть только одна реализация и она никогда не менялась
- Компонент не тестируется в изоляции
- Это internal utility без внешних зависимостей

Распространённая ошибка — интерфейсы на будущее: объявить `UserStoreInterface` заранее, на случай если однажды понадобится подмена в тесте. Смысла в этом нет: благодаря неявному соответствию интерфейс добавляется в любой момент, не трогая реализацию. Поэтому его вводят тогда, когда появился второй потребитель или реально понадобилась подмена, а не раньше.

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

По сигнатуре `NewOrderService(orders, events, log)` видно, что именно нужно сервису. Без явной передачи зависимостей пришлось бы прочитать всю реализацию, чтобы узнать, к чему он обращается.

**3. Управление временем жизни.**

В `main.go` видно, что создаётся раньше и что закрывается позже. С глобальным состоянием этот порядок нигде не зафиксирован, и место для `defer close()` приходится угадывать — а порядок здесь важен: закрыть пул соединений раньше, чем остановлены его пользователи, значит получить ошибки на завершении.

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

// 4. Скрытая инициализация поля
type Service struct {
    db    *sql.DB // нормально: передан через конструктор
    cache *Cache  // плохо: создаётся при первом вызове через sync.Once —
                  // зависимость не видна снаружи и не подменяется в тесте
}
```

### DI-фреймворки в Go

Существуют [google/wire](https://github.com/google/wire), [uber-go/fx](https://github.com/uber-go/fx) и [dig](https://github.com/uber-go/dig). Они различаются моментом сборки графа: wire генерирует код на этапе компиляции, fx и dig разрешают зависимости во время выполнения через рефлексию.

Разница практическая. Ошибка в графе у wire — это ошибка сборки; у fx и dig — паника при старте, иногда только на определённой конфигурации.

Для большинства проектов ни один из них не нужен: ручная сборка в `main.go` занимает 50-100 строк, читается сверху вниз и отлаживается обычным отладчиком. Смысл появляется, когда компонентов десятки, а порядок запуска и остановки нетривиален. Иначе явная сборка выигрывает.

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

Опция может не только присваивать значение, но и возвращать ошибку:

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

Вариант полезен для библиотек: неверная конфигурация обнаруживается при создании объекта, а не при первом запросе, когда до причины уже далеко.

---

## Связки и антипаттерны

### Композиция через embedding

Наследования в Go нет, но встраивание (`embedding`) даёт композицию с прямым доступом к методам вложенного типа:

```go
type BaseHandler struct {
    log *slog.Logger
    db  *sql.DB
}

type OrderHandler struct {
    BaseHandler  // ← embed, методы BaseHandler становятся методами OrderHandler
    orderRepo OrderRepository
}

// h.log.Info(...) доступен напрямую, без h.BaseHandler.log
```

**Когда уместно:**
- Действительно общие поля для группы хэндлеров (log, metrics, db)
- HTTP middleware-подобные wrapper'ы

**Когда встраивание вредит:**
- Встраивание ради переиспользования одной функции — явный вызов понятнее.
- Встраивание ради отношения «является» — это имитация наследования, и она приносит его проблемы: методы внешнего типа неотличимы от методов вложенного, а изменение базового типа молча меняет публичный интерфейс всех, кто его встроил.

### Антипаттерны этого раздела

**1. Всеобъемлющий интерфейс.**
`UserService` на два десятка методов, из которых каждому потребителю нужны два. Лечится разбиением на узкие интерфейсы по ролям.

**2. Скрытые зависимости.**
Конструктор выглядит чистым, но внутри читает переменные окружения или обращается к глобальному состоянию. Тест такого компонента невозможно настроить, не меняя окружение процесса.

**3. Опции-функции для всего подряд.**
Структура с двумя параметрами не нуждается в обёртках `WithXxx`: `New(url, timeout)` короче и понятнее.

**4. Одиночка через `init()`.**
Глобальный объект, создаваемый при импорте пакета. Его нельзя пересоздать между тестами, и порядок инициализации между пакетами определяется компилятором, а не автором.

**5. Локатор служб.**
`container.Get("user_service")` переносит проверку связей в момент выполнения: отсутствующая зависимость обнаружится при первом вызове, а не при сборке. Явная передача через конструктор даёт ту же гибкость без этого риска.

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
