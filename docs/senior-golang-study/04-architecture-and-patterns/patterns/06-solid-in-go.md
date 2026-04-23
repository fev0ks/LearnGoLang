# SOLID в Go

SOLID — пять принципов проектирования, которые снижают coupling и упрощают изменения. В Go они реализуются иначе, чем в Java/C#: нет классов и наследования, интерфейсы удовлетворяются неявно, composition — основной инструмент. Это делает некоторые принципы проще, а некоторые требуют переосмысления.

## Содержание

- [Обзор](#обзор)
- [S — Single Responsibility Principle](#s--single-responsibility-principle)
- [O — Open/Closed Principle](#o--openclosed-principle)
- [L — Liskov Substitution Principle](#l--liskov-substitution-principle)
- [I — Interface Segregation Principle](#i--interface-segregation-principle)
- [D — Dependency Inversion Principle](#d--dependency-inversion-principle)
- [Как принципы связаны между собой](#как-принципы-связаны-между-собой)
- [Типичные нарушения в Go](#типичные-нарушения-в-go)
- [Interview-ready answer](#interview-ready-answer)

---

## Обзор

| Принцип | Суть | Главный инструмент в Go |
|---|---|---|
| **S**RP — Single Responsibility | Один повод для изменения | Маленькие пакеты, разделение слоёв |
| **O**CP — Open/Closed | Расширяем без изменения | Интерфейсы + strategy/middleware |
| **L**SP — Liskov Substitution | Реализация заменяема без сюрпризов | Корректная реализация интерфейса |
| **I**SP — Interface Segregation | Маленькие интерфейсы по назначению | `io.Reader`, `io.Writer`, узкие interfaces |
| **D**IP — Dependency Inversion | Зависеть от абстракций | Consumer declares interface |

---

## S — Single Responsibility Principle

> Модуль должен иметь **одну причину для изменения**.

Не "делает одну вещь" (слишком буквально), а "меняется по одной причине". Handler меняется когда меняется API. Service — когда меняется бизнес-логика. Repository — когда меняется схема хранения.

### Нарушение

```go
// Плохо — один handler делает всё
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    // 1. Парсинг запроса (HTTP concern)
    var req struct {
        UserID string  `json:"user_id"`
        Items  []Item  `json:"items"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    // 2. Бизнес-логика (domain concern)
    if len(req.Items) == 0 {
        http.Error(w, "empty order", 400)
        return
    }
    discount := 0.0
    if req.UserID == "vip" { discount = 0.1 }  // ❌ логика в handler

    // 3. SQL (storage concern)
    _, err := h.db.ExecContext(r.Context(),  // ❌ db в handler
        "INSERT INTO orders ...", req.UserID)

    // 4. Ответ (HTTP concern)
    w.WriteHeader(201)
}
// Причин изменения: API, бизнес-правила, схема БД — всё три
```

### Соответствие

```go
// Handler — только HTTP concern
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    var req CreateOrderRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }
    orderID, err := h.svc.CreateOrder(r.Context(), service.CreateOrderCmd{
        UserID: req.UserID,
        Items:  mapItems(req.Items),
    })
    if err != nil {
        mapError(w, err)
        return
    }
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"id": string(orderID)})
}

// Service — только business concern
func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCmd) (OrderID, error) {
    if len(cmd.Items) == 0 {
        return "", ErrEmptyOrder
    }
    discount := s.discountPolicy.Calculate(cmd.UserID)
    order := newOrder(cmd, discount)
    return s.repo.Save(ctx, order)
}

// Repository — только storage concern
func (r *orderRepo) Save(ctx context.Context, o *Order) (OrderID, error) {
    id := newOrderID()
    _, err := r.db.ExecContext(ctx, "INSERT INTO orders ...", id, o.UserID)
    return id, err
}
```

**В Go SRP на уровне пакетов:** пакет `handler` меняется от изменений API, `service` — от изменений бизнес-правил, `repository` — от изменений схемы БД. Три разных причины — три разных пакета.

---

## O — Open/Closed Principle

> Программный объект должен быть **открыт для расширения**, но **закрыт для изменения**.

В Java это достигается наследованием. В Go — интерфейсами и composition.

### Нарушение: switch по типам

```go
// Плохо — каждый новый тип нотификации требует изменения существующего кода
func SendNotification(n Notification) error {
    switch n.Type {                    // ❌ Open/Closed нарушен
    case "email":
        return sendEmail(n)
    case "sms":
        return sendSMS(n)
    case "push":
        return sendPush(n)
    // Добавить Telegram → редактировать эту функцию
    }
    return fmt.Errorf("unknown type: %s", n.Type)
}
```

### Соответствие: интерфейс + новые реализации

```go
// Хорошо — расширение без изменения существующего кода
type Sender interface {
    Send(ctx context.Context, msg Message) error
}

// Существующие реализации — не меняются
type EmailSender struct { client *smtp.Client }
func (s *EmailSender) Send(ctx context.Context, msg Message) error { ... }

type SMSSender struct { client *twilio.Client }
func (s *SMSSender) Send(ctx context.Context, msg Message) error { ... }

// Новый тип: просто создаём новую реализацию
type TelegramSender struct { bot *tgbotapi.BotAPI }
func (s *TelegramSender) Send(ctx context.Context, msg Message) error { ... }

// NotificationService — не меняется при добавлении нового типа
type NotificationService struct {
    senders map[string]Sender
}

func (s *NotificationService) Notify(ctx context.Context, userID string, msg Message) error {
    prefs := s.loadPreferences(userID)
    for _, channelType := range prefs.Channels {
        sender, ok := s.senders[channelType]
        if !ok {
            continue
        }
        if err := sender.Send(ctx, msg); err != nil {
            return err
        }
    }
    return nil
}
```

### OCP в middleware

Middleware — классический пример OCP: добавляем поведение (логирование, авторизация, трейсинг) без изменения существующих хендлеров.

```go
type Middleware func(http.Handler) http.Handler

// Существующий handler не знает о новом поведении
func NewRouter(h *Handler, middlewares ...Middleware) http.Handler {
    var router http.Handler = h
    for i := len(middlewares) - 1; i >= 0; i-- {
        router = middlewares[i](router)
    }
    return router
}

// Добавить rate limiting → не трогаем handler
mux := NewRouter(handler,
    LoggingMiddleware(logger),
    AuthMiddleware(authSvc),
    RateLimitMiddleware(limiter),  // новое поведение
)
```

---

## L — Liskov Substitution Principle

> Объекты подтипа должны быть **заменяемы** объектами базового типа без изменения корректности программы.

В Go: любая реализация интерфейса должна вести себя так, как ожидает потребитель интерфейса. Нарушение — когда реализация делает меньше (паникует, возвращает неожиданные ошибки, нарушает контракт).

### Нарушение

```go
type Cache interface {
    Get(key string) ([]byte, bool)
    Set(key string, value []byte, ttl time.Duration)
}

// RedisCache — корректная реализация
type RedisCache struct { client *redis.Client }
func (c *RedisCache) Get(key string) ([]byte, bool) { ... }
func (c *RedisCache) Set(key string, value []byte, ttl time.Duration) { ... }

// ReadOnlyCache — нарушение LSP: Set паникует
type ReadOnlyCache struct { inner Cache }
func (c *ReadOnlyCache) Get(key string) ([]byte, bool) { return c.inner.Get(key) }
func (c *ReadOnlyCache) Set(key string, value []byte, ttl time.Duration) {
    panic("read-only cache")  // ❌ потребитель не ожидает паники от Set
}
```

```go
// Нарушение через неожиданные ошибки
type LoggingReader struct { inner io.Reader }
func (r *LoggingReader) Read(p []byte) (int, error) {
    n, err := r.inner.Read(p)
    if n == 0 {
        return 0, io.ErrUnexpectedEOF  // ❌ контракт io.Reader нарушен:
    }                                    // нулевое чтение не означает неожиданный EOF
    return n, err
}
```

### Соответствие

```go
// Правильно: разделить интерфейсы (см. ISP)
type ReadableCache interface {
    Get(key string) ([]byte, bool)
}

type WritableCache interface {
    ReadableCache
    Set(key string, value []byte, ttl time.Duration)
}

// ReadOnlyCache честно реализует только ReadableCache
type ReadOnlyCache struct { inner ReadableCache }
func (c *ReadOnlyCache) Get(key string) ([]byte, bool) { return c.inner.Get(key) }
```

### LSP и `io.Reader`

Стандартная библиотека Go — пример правильного LSP. Любой `io.Reader` (файл, HTTP body, bytes.Buffer, strings.Reader, gzip.Reader) ведёт себя предсказуемо: `Read` либо возвращает данные, либо `io.EOF`, либо ошибку. Никаких сюрпризов.

```go
// Эта функция работает с любым io.Reader — LSP в действии
func processStream(r io.Reader) error {
    buf := make([]byte, 4096)
    for {
        n, err := r.Read(buf)
        if n > 0 {
            process(buf[:n])
        }
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return err
        }
    }
}

// Работает с любой реализацией без изменения кода:
processStream(os.Stdin)
processStream(bytes.NewReader(data))
processStream(resp.Body)
processStream(gzip.NewReader(file))
```

---

## I — Interface Segregation Principle

> Клиент не должен зависеть от методов, которые он не использует.

В Go это реализуется **естественно**: интерфейс объявляет **потребитель**, только с нужными методами. Не нужно наследовать "жирный" интерфейс целиком.

### Нарушение: жирный интерфейс

```go
// Плохо — один большой интерфейс
type UserRepository interface {
    FindByID(ctx context.Context, id UserID) (*User, error)
    FindByEmail(ctx context.Context, email string) (*User, error)
    Save(ctx context.Context, user *User) error
    Delete(ctx context.Context, id UserID) error
    FindAllActive(ctx context.Context) ([]*User, error)
    CountByRegion(ctx context.Context, region string) (int, error)
    UpdateLastLogin(ctx context.Context, id UserID) error
    FindInactiveOlderThan(ctx context.Context, d time.Duration) ([]*User, error)
}

// AuthService использует только FindByEmail — но зависит от всего интерфейса
type AuthService struct {
    repo UserRepository  // ❌ притащили 7 лишних методов
}

// Тест AuthService вынужден реализовывать mock с 8 методами
type mockUserRepo struct{}
func (m *mockUserRepo) FindByID(...) (*User, error)                    { panic("not used") }
func (m *mockUserRepo) FindByEmail(...) (*User, error)                 { return testUser, nil }
func (m *mockUserRepo) Save(...) error                                 { panic("not used") }
// ... ещё 5 методов которые не нужны
```

### Соответствие: интерфейс объявляет потребитель

```go
// Каждый потребитель объявляет минимальный нужный ему интерфейс

// AuthService — нужен только поиск по email
type AuthService struct {
    users interface {
        FindByEmail(ctx context.Context, email string) (*User, error)
    }
}

// OrderService — нужен только поиск по ID
type OrderService struct {
    users interface {
        FindByID(ctx context.Context, id UserID) (*User, error)
    }
}

// CleanupJob — нужны только "грязные" методы
type CleanupJob struct {
    users interface {
        FindInactiveOlderThan(ctx context.Context, d time.Duration) ([]*User, error)
        Delete(ctx context.Context, id UserID) error
    }
}

// Одна реализация удовлетворяет всем интерфейсам — Go duck typing
type pgUserRepository struct { db *pgxpool.Pool }
// реализует FindByID, FindByEmail, Save, Delete, FindInactiveOlderThan...

// Тест AuthService — mock теперь простой
type mockUserFinder struct{ user *User }
func (m *mockUserFinder) FindByEmail(_ context.Context, _ string) (*User, error) {
    return m.user, nil
}
```

### Стандартная библиотека как образец ISP

```go
// io пакет — каждый интерфейс минимален
type Reader interface { Read(p []byte) (n int, err error) }
type Writer interface { Write(p []byte) (n int, err error) }
type Closer interface { Close() error }

// Комбинации — только когда нужно
type ReadWriter  interface { Reader; Writer }
type ReadCloser  interface { Reader; Closer }
type WriteCloser interface { Writer; Closer }
type ReadWriteCloser interface { Reader; Writer; Closer }

// Функции принимают минимум:
func io.Copy(dst Writer, src Reader) (int64, error)  // не ReadWriteCloser
func io.ReadAll(r Reader) ([]byte, error)             // не ReadCloser
```

---

## D — Dependency Inversion Principle

> 1. Модули высокого уровня не должны зависеть от модулей низкого уровня. Оба должны зависеть от **абстракций**.
> 2. Абстракции не должны зависеть от деталей. Детали должны зависеть от абстракций.

В Go DIP выражается через правило: **интерфейс объявляет потребитель** (высокий уровень), **реализацию предоставляет поставщик** (низкий уровень). Зависимость инвертирована: инфраструктурный пакет зависит от domain-интерфейса, а не наоборот.

### Нарушение: зависимость от конкретного типа

```go
// Плохо — OrderService зависит от PostgreSQL напрямую
package service

import "github.com/jackc/pgx/v5/pgxpool"  // ❌ зависимость от инфраструктуры

type OrderService struct {
    db *pgxpool.Pool  // ❌ высокий уровень зависит от низкого
}

func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCmd) error {
    _, err := s.db.Exec(ctx, "INSERT INTO orders ...", cmd.UserID)
    return err
}
// Проблема: тест требует реального PostgreSQL
// Проблема: сменить Redis → переписывать service
```

### Соответствие: зависимость от абстракции

```go
// domain/ports.go — интерфейс в пакете ПОТРЕБИТЕЛЯ (domain/service)
package domain

// OrderRepository — абстракция, объявленная там где используется
type OrderRepository interface {
    Save(ctx context.Context, order *Order) error
    FindByID(ctx context.Context, id OrderID) (*Order, error)
}

// OrderService — зависит только от интерфейса
type OrderService struct {
    repo OrderRepository  // ✓ абстракция, не конкретный тип
}

func NewOrderService(repo OrderRepository) *OrderService {
    return &OrderService{repo: repo}
}

func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCmd) error {
    order, err := domain.NewOrder(cmd.OrderID, cmd.CustomerID, cmd.Address)
    if err != nil {
        return err
    }
    return s.repo.Save(ctx, order)
}
```

```go
// infra/postgres/order_repo.go — реализация в пакете ПОСТАВЩИКА
package postgres

import "github.com/myapp/internal/domain"

// pgOrderRepo зависит от domain-интерфейса — направление зависимости инвертировано
type pgOrderRepo struct { db *pgxpool.Pool }

func NewOrderRepo(db *pgxpool.Pool) domain.OrderRepository {  // возвращает интерфейс
    return &pgOrderRepo{db: db}
}

func (r *pgOrderRepo) Save(ctx context.Context, o *domain.Order) error { ... }
func (r *pgOrderRepo) FindByID(ctx context.Context, id domain.OrderID) (*domain.Order, error) { ... }
```

```
Направление зависимостей:
  postgres.pgOrderRepo ──► domain.OrderRepository ◄── domain.OrderService
  
Без DIP:
  domain.OrderService ──► postgres.pgOrderRepo  (высокий уровень зависит от низкого)
```

### DIP и тестируемость

```go
// Unit-тест без PostgreSQL — подменяем реализацию
type mockOrderRepo struct {
    saved []*domain.Order
}
func (m *mockOrderRepo) Save(_ context.Context, o *domain.Order) error {
    m.saved = append(m.saved, o)
    return nil
}
func (m *mockOrderRepo) FindByID(_ context.Context, id domain.OrderID) (*domain.Order, error) {
    for _, o := range m.saved {
        if o.ID() == id { return o, nil }
    }
    return nil, domain.ErrOrderNotFound
}

func TestCreateOrder(t *testing.T) {
    repo := &mockOrderRepo{}
    svc := domain.NewOrderService(repo)

    err := svc.CreateOrder(context.Background(), CreateOrderCmd{ ... })
    require.NoError(t, err)
    assert.Len(t, repo.saved, 1)
}
```

---

## Как принципы связаны между собой

```
       SRP                     ISP
  ┌─────────────┐         ┌─────────────┐
  │ Один повод  │         │  Маленькие  │
  │ для изменен.│         │ интерфейсы  │
  └──────┬──────┘         └──────┬──────┘
         │                       │
         └──────────┬────────────┘
                    │
                    ▼
             ┌─────────────┐
             │     DIP     │  ← объединяет: абстракции
             │ Зависеть от │    должны быть узкими (ISP)
             │ абстракций  │    и с одной ответственностью (SRP)
             └──────┬──────┘
                    │
         ┌──────────┴────────────┐
         ▼                       ▼
  ┌─────────────┐         ┌─────────────┐
  │     OCP     │         │     LSP     │
  │ Расширяем   │         │ Реализация  │
  │ через новые │         │ заменяема   │
  │ реализации  │         │ без сюрпри. │
  └─────────────┘         └─────────────┘
```

- **SRP + ISP** → маленькие интерфейсы с одной ответственностью → легко соблюдать LSP
- **DIP** требует маленьких интерфейсов (ISP) — жирный интерфейс сложно удовлетворить без нарушений
- **OCP** реализуется через интерфейсы (DIP) — новое поведение = новая реализация
- **LSP** — предпосылка для OCP: расширение работает только если реализации взаимозаменяемы

---

## Типичные нарушения в Go

### 1. Interface в пакете поставщика (нарушение DIP + ISP)

```go
// Плохо — интерфейс в пакете postgres (поставщик объявляет сам себя)
package postgres

type OrderRepository interface {  // ❌ поставщик объявляет абстракцию
    Save(ctx context.Context, ...) error
    FindByID(ctx context.Context, ...) (*Order, error)
    // + 10 других методов которые нужны только одному потребителю
}

// Хорошо — каждый потребитель объявляет нужный ему минимум в своём пакете
package service
type orderSaver interface { Save(ctx context.Context, o *Order) error }
```

### 2. `utils` пакет (нарушение SRP)

```go
// Плохо — utils как свалка
package utils

func ValidateEmail(s string) bool { ... }      // валидация
func FormatMoney(n int64) string { ... }        // форматирование
func GenerateID() string { ... }                // генерация
func ParseConfig(path string) (*Config, error)  // конфиг
// Меняется по 4 разным причинам

// Хорошо — каждая концепция в своём пакете
package emailvalidator
package money
package idgen
package config
```

### 3. Конкретный тип в поле структуры (нарушение DIP)

```go
// Плохо
type OrderService struct {
    repo *postgres.OrderRepository  // ❌ конкретный тип
    cache *redis.Client             // ❌ конкретный тип
}

// Хорошо
type OrderService struct {
    repo  orderRepository  // интерфейс (private — ISP: только нужные методы)
    cache orderCache       // интерфейс
}
```

### 4. Реализация делает неожиданное (нарушение LSP)

```go
// Нарушения контракта в реализации интерфейса:
// - panic вместо error
// - возврат nil там где ожидается non-nil
// - изменение глобального состояния
// - игнорирование ctx.Done()
// - разные коды ошибок при одинаковом сценарии

// Проверка: любая реализация должна проходить один набор тестов
func RunRepositoryContract(t *testing.T, repo OrderRepository) {
    t.Run("not found returns ErrOrderNotFound", func(t *testing.T) {
        _, err := repo.FindByID(context.Background(), "nonexistent")
        assert.ErrorIs(t, err, ErrOrderNotFound)
    })
    t.Run("save and find roundtrip", func(t *testing.T) { ... })
    t.Run("respects context cancellation", func(t *testing.T) { ... })
}

// Тест для каждой реализации
func TestPostgresRepo(t *testing.T) { RunRepositoryContract(t, newTestPostgresRepo(t)) }
func TestInMemoryRepo(t *testing.T) { RunRepositoryContract(t, NewInMemoryRepo()) }
```

---

## Interview-ready answer

> "SOLID в Go реализуется иначе, чем в Java, потому что нет классов и наследования.
>
> **SRP** — один повод для изменения. В Go это разделение на слои: handler меняется от изменений API, service — от бизнес-правил, repository — от схемы хранения. Свалка `utils` — классическое нарушение.
>
> **OCP** — расширяем без изменения. В Java это наследование, в Go — интерфейсы: новый тип Sender добавляется без правки существующего кода. Middleware цепочка — ещё один пример.
>
> **LSP** — реализация заменяема без сюрпризов. В Go это значит: любая реализация интерфейса должна вести себя предсказуемо — не паниковать, правильно обрабатывать ошибки, уважать context. `io.Reader` — образцовый пример: файл, буфер, gzip — все взаимозаменяемы.
>
> **ISP** — интерфейс объявляет потребитель с минимумом методов. Go делает это естественным через duck typing. `io.Reader`/`io.Writer` — один метод каждый. Жирный `UserRepository` с 8 методами вынуждает mock реализовывать всё — лучше 3 узких интерфейса.
>
> **DIP** — зависеть от абстракций. В Go: интерфейс объявляется в пакете потребителя, реализация — в инфраструктурном пакете. `domain.OrderRepository` — интерфейс в domain, `postgres.pgOrderRepo` — реализует его. Зависимость инвертирована: postgres пакет зависит от domain, не наоборот. Это и есть основа testability без реальной БД."