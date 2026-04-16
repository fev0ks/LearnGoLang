# Go Code Patterns

Go не поощряет тяжелую объектную иерархию. В Go паттерны обычно выглядят проще: маленькие интерфейсы, функции, композиция, явные зависимости и тонкие адаптеры вокруг внешнего мира.

## Содержание

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

## Small interfaces

Идея: интерфейс описывает минимальное поведение, которое нужно потребителю.

```go
type UserStore interface {
	GetByID(ctx context.Context, id int64) (User, error)
}
```

Где использовать:
- в сервисном слое, если нужно отделить бизнес-логику от storage;
- в тестах, чтобы заменить внешнюю зависимость;
- на границах модулей.

Сильные стороны:
- меньше coupling;
- проще тестировать;
- проще заменить реализацию.

Слабые стороны:
- слишком много интерфейсов создает шум;
- интерфейс "на всякий случай" часто устаревает раньше, чем становится полезным.

Правило: интерфейс обычно объявляет потребитель, а не поставщик. Если есть `PostgresUserStore`, ему не обязательно рядом иметь `PostgresUserStoreInterface`.

## Constructor injection

Идея: зависимости передаются явно через конструктор.

```go
type Service struct {
	users UserStore
	log   Logger
}

func NewService(users UserStore, log Logger) *Service {
	return &Service{users: users, log: log}
}
```

Где использовать:
- почти во всех сервисах с внешними зависимостями;
- когда компонент должен быть тестируемым;
- когда важно видеть dependency graph.

Сильные стороны:
- зависимости видны сразу;
- тесты не завязаны на global state;
- lifecycle зависимостей контролируется снаружи.

Типичная ошибка: прятать зависимости внутри конструктора через `sql.Open`, `redis.NewClient` или чтение env. Это усложняет тестирование и делает компонент менее предсказуемым.

## Functional options

Идея: опциональные настройки передаются через функции.

```go
type Client struct {
	timeout time.Duration
	retries int
}

type Option func(*Client)

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
	}
}

func NewClient(opts ...Option) *Client {
	c := &Client{
		timeout: 3 * time.Second,
		retries: 1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
```

Когда выбирать:
- много необязательных параметров;
- нужны defaults;
- API конструктора должен оставаться стабильным.

Когда не выбирать:
- параметров мало;
- все параметры обязательные;
- обычная config-структура читается проще.

Альтернатива:

```go
type Config struct {
	Timeout time.Duration
	Retries int
}

func NewClient(cfg Config) *Client { /* ... */ return &Client{} }
```

Config проще для приложений, functional options удобнее для библиотечного API.

## Middleware

Идея: обернуть обработчик общей логикой: logging, auth, metrics, tracing, rate limit.

```go
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("method=%s path=%s duration=%s", r.Method, r.URL.Path, time.Since(start))
	})
}
```

Где использовать:
- HTTP/gRPC interceptors;
- observability;
- auth и authorization boundaries;
- retries/timeouts на client-side.

Сильные стороны:
- cross-cutting concerns не размазываются по handlers;
- порядок применения явно контролируется;
- легко переиспользовать.

Типичная ошибка: класть в middleware бизнес-логику. Middleware должен заниматься технической или boundary-логикой, а не решать domain use cases.

## Adapter

Идея: привести внешний API к внутреннему интерфейсу приложения.

```go
type PaymentProvider interface {
	Charge(ctx context.Context, amount Money) (PaymentID, error)
}

type StripeAdapter struct {
	client *stripe.Client
}

func (a *StripeAdapter) Charge(ctx context.Context, amount Money) (PaymentID, error) {
	// Convert internal model to Stripe request and map Stripe errors back.
	return PaymentID("..."), nil
}
```

Где использовать:
- внешние API;
- брокеры сообщений;
- базы данных;
- SDK, которые не хочется протаскивать в domain/service layer.

Сильные стороны:
- внешний SDK не заражает бизнес-логику;
- проще заменить provider;
- проще нормализовать ошибки.

Trade-off: адаптер добавляет слой. Он оправдан, когда внешний контракт нестабилен, сложен или нежелателен внутри ядра приложения.

## Decorator

Идея: добавить поведение к существующей реализации без изменения ее кода.

```go
type CachedUserStore struct {
	next  UserStore
	cache Cache
}

func (s *CachedUserStore) GetByID(ctx context.Context, id int64) (User, error) {
	if user, ok := s.cache.Get(id); ok {
		return user, nil
	}
	user, err := s.next.GetByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	s.cache.Set(id, user)
	return user, nil
}
```

Где использовать:
- cache layer;
- metrics wrapper;
- tracing wrapper;
- retry wrapper;
- circuit breaker wrapper.

Сильные стороны:
- можно комбинировать поведение;
- основная реализация остается простой;
- удобно тестировать.

Типичная ошибка: сделать цепочку wrapper-ов такой длинной, что становится сложно понять, где реально происходит работа.

## Strategy

Идея: выбирать алгоритм или поведение через интерфейс или функцию.

```go
type PricingStrategy interface {
	Price(order Order) Money
}
```

В Go часто достаточно функции:

```go
type PriceFunc func(order Order) Money
```

Где использовать:
- разные pricing rules;
- разные алгоритмы сортировки, matching, scoring;
- разные delivery providers;
- feature-specific поведение без большого `switch`.

Когда не выбирать:
- вариантов один или два и они не меняются;
- простой `switch` по enum читается лучше.

## Repository

Идея: спрятать детали хранения за интерфейсом, который говорит языком приложения.

```go
type OrderRepository interface {
	Save(ctx context.Context, order Order) error
	FindByID(ctx context.Context, id OrderID) (Order, error)
}
```

Где полезен:
- domain/service layer не должен знать SQL, Redis или Mongo details;
- есть сложный mapping между storage model и domain model;
- нужны тесты use cases без настоящей базы.

Где вреден:
- CRUD простой, а repository просто повторяет таблицу;
- repository скрывает важные query patterns и мешает оптимизации;
- поверх `sqlc` или `pgx` добавляется пустой слой без новой ответственности.

Практичное правило: repository должен выражать операции домена, а не быть универсальным `Get/Save/Delete` для каждой таблицы.

## Unit of Work

Идея: объединить несколько storage-операций в одну транзакционную границу.

```go
type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}
```

Где использовать:
- несколько repository-операций должны commit/rollback вместе;
- use case создает несколько записей;
- нужно явно управлять transaction boundary.

Trade-off: abstraction над транзакциями легко становится слишком общей. В Go часто достаточно конкретного `WithTx` рядом с database layer.

## Context boundaries

`context.Context` в Go - не просто параметр для отмены. Это boundary для request lifetime.

Правила:
- `context.Context` почти всегда первый аргумент;
- не хранить `context.Context` в struct;
- не использовать context как generic map для бизнес-данных;
- обязательно пробрасывать его в DB, HTTP clients, broker clients;
- timeout лучше задавать на границе use case или external call.

## Error wrapping and mapping

Паттерн: низкоуровневая ошибка оборачивается, а на boundary маппится в понятный ответ.

```go
if err != nil {
	return fmt.Errorf("load user %d: %w", id, err)
}
```

Где boundary:
- HTTP handler маппит domain errors в status codes;
- gRPC handler маппит domain errors в gRPC codes;
- worker решает, retry или dead-letter.

Типичная ошибка: возвращать наружу raw SQL/SDK errors или, наоборот, терять причину через `fmt.Errorf("failed")` без `%w`.

## Checklist

- Интерфейс объявлен там, где он потребляется?
- Зависимости передаются явно?
- Есть ли реальная причина для adapter/decorator/repository?
- Не спрятали ли бизнес-логику в middleware?
- Понятно ли, где transaction boundary?
- Ошибки сохраняют причину и маппятся на внешней границе?
- Можно ли протестировать use case без настоящего внешнего сервиса?

## Interview-ready answer

В Go я чаще всего использую small interfaces, constructor injection, middleware, adapter, decorator, strategy и иногда repository/unit of work. Но я стараюсь не переносить GoF один-в-один: из-за интерфейсов, функций и композиции Go обычно требует меньше классов и слоев. Хороший Go-паттерн делает зависимости явными, упрощает тесты и изолирует внешний мир. Плохой паттерн добавляет абстракции, которые не защищают ни от изменений, ни от сложности.
