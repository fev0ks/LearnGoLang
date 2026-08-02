# SOLID в Go

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

SOLID — пять принципов проектирования, которые сводятся к одной цели: изменение в одном месте не должно требовать правок в остальных. Сформулированы они были для языков с классами и наследованием, и в Go часть из них выглядит иначе.

Различия задают три особенности языка. Наследования нет — расширение выражается интерфейсом и композицией. Интерфейсы удовлетворяются неявно — тип не объявляет, какому контракту следует, а значит компилятор не проверяет соответствие поведения, только сигнатур. Интерфейс можно объявить в пакете, который его использует, — и это делает разворот зависимости настройкой по умолчанию, а не отдельным приёмом.

---

## Обзор

| Принцип | Суть | Главный инструмент в Go |
|---|---|---|
| SRP — Single Responsibility | Один повод для изменения | Маленькие пакеты, разделение слоёв |
| OCP — Open/Closed | Расширяем без изменения | Интерфейсы, strategy, middleware |
| LSP — Liskov Substitution | Реализация заменяема без сюрпризов | Соблюдение контракта интерфейса |
| ISP — Interface Segregation | Маленькие интерфейсы по назначению | `io.Reader`, `io.Writer`, узкие интерфейсы |
| DIP — Dependency Inversion | Зависеть от абстракций | Интерфейс объявляет потребитель |

---

## S — Single Responsibility Principle

> Модуль должен иметь одну причину для изменения.

Формулировка «делает одну вещь» слишком буквальна и заводит в спор о том, что считать вещью. Точнее — «меняется по одной причине»: handler меняется, когда меняется API, service — когда меняются бизнес-правила, repository — когда меняется схема хранения.

Проверка тоже идёт от причин, а не от размера: если два изменения, приходящие от разных людей и в разное время, затрагивают один файл, ответственностей в нём больше одной.

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

> Программный объект должен быть открыт для расширения, но закрыт для изменения.

В языках с наследованием расширение выражается подклассом. В Go — новой реализацией интерфейса и композицией.

Смысл принципа не в запрете править код, а в цене правки. Функция со `switch` по типу уведомления работает; проблема в том, что добавление канала требует изменить уже проверенную и работающую ветвь кода, а значит — заново её протестировать и заново рискнуть. Новая реализация интерфейса такого риска не создаёт: существующие типы не тронуты.

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

> Объекты подтипа должны быть заменяемы объектами базового типа без изменения корректности программы.

В Go это читается так: любая реализация интерфейса ведёт себя так, как ожидает потребитель. Нарушение — когда реализация делает меньше обещанного: паникует, возвращает неожиданные ошибки, игнорирует отмену контекста.

Особенность Go в том, что компилятор здесь не помогает. Он проверяет совпадение сигнатур, и на этом его участие заканчивается: `Set`, который вместо записи вызывает `panic`, — совершенно законная реализация интерфейса с точки зрения системы типов. Контракт живёт в документации и в тестах, поэтому единственный работающий способ его удержать — общий набор тестов, который проходят все реализации (пример — в конце статьи).

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

Стандартная библиотека Go — пример правильного LSP. Любой `io.Reader` — файл, тело HTTP-ответа, `bytes.Buffer`, `strings.Reader`, `gzip.Reader` — ведёт себя предсказуемо: `Read` возвращает данные, `io.EOF` или ошибку.

Контракт при этом сформулирован жёстче, чем кажется по сигнатуре, и две его детали регулярно нарушают потребители, а не реализации:

- Прочитанные байты обрабатываются раньше проверки ошибки. `Read` имеет право вернуть `n > 0` вместе с `io.EOF` в одном вызове, и код, который при непустой ошибке сразу выходит, теряет последнюю порцию данных.
- `n == 0` при `err == nil` не означает конец: это допустимый результат, на который нужно просто сделать следующую итерацию.

```go
// Эта функция работает с любым io.Reader — LSP в действии
func processStream(r io.Reader) error {
    buf := make([]byte, 4096)
    for {
        n, err := r.Read(buf)
        if n > 0 {
            // Сначала данные: их могли вернуть вместе с io.EOF.
            process(buf[:n])
        }
        if errors.Is(err, io.EOF) {
            return nil
        }
        if err != nil {
            return err
        }
    }
}
```

Сравнение через `errors.Is`, а не через `err == io.EOF`, здесь не педантизм: обёртка вокруг читателя может вернуть `fmt.Errorf("read body: %w", io.EOF)`, и прямое сравнение такую ошибку не распознает — цикл уйдёт в ветку общей ошибки.

```go
// Работает с любой реализацией без изменения кода:
processStream(os.Stdin)
processStream(bytes.NewReader(data))
processStream(resp.Body)

// gzip.NewReader возвращает две величины — ошибку нужно разобрать отдельно
zr, err := gzip.NewReader(file)
if err != nil {
    return fmt.Errorf("open gzip: %w", err)
}
defer zr.Close()
processStream(zr)
```

---

## I — Interface Segregation Principle

> Клиент не должен зависеть от методов, которые он не использует.

В Go принцип соблюдается почти сам собой: интерфейс объявляет потребитель и включает в него только нужные методы. Наследовать общий интерфейс целиком не требуется — тип подходит под любой интерфейс, чьи методы у него есть, ничего об этом не объявляя.

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
// Каждый потребитель объявляет минимальный нужный ему интерфейс —
// с именем, в своём пакете и неэкспортированный: снаружи он не нужен.

// AuthService — нужен только поиск по email
type userFinderByEmail interface {
    FindByEmail(ctx context.Context, email string) (*User, error)
}

type AuthService struct {
    users userFinderByEmail
}

// OrderService — нужен только поиск по ID
type userFinderByID interface {
    FindByID(ctx context.Context, id UserID) (*User, error)
}

type OrderService struct {
    users userFinderByID
}

// CleanupJob — нужны поиск неактивных и удаление
type inactiveUserCleaner interface {
    FindInactiveOlderThan(ctx context.Context, d time.Duration) ([]*User, error)
    Delete(ctx context.Context, id UserID) error
}

type CleanupJob struct {
    users inactiveUserCleaner
}

// Одна реализация удовлетворяет всем трём интерфейсам сразу и
// не упоминает ни один из них: соответствие в Go неявное.
type pgUserRepository struct { db *pgxpool.Pool }
// реализует FindByID, FindByEmail, Save, Delete, FindInactiveOlderThan...

// Тест AuthService — заглушка теперь в три строки
type mockUserFinder struct{ user *User }

func (m *mockUserFinder) FindByEmail(_ context.Context, _ string) (*User, error) {
    return m.user, nil
}
```

Именованный интерфейс здесь предпочтительнее анонимного (`users interface { ... }` прямо в поле структуры). Анонимный компилируется, но его нельзя упомянуть в сигнатуре конструктора, переиспользовать в тесте или назвать в сообщении об ошибке компилятора — вместо имени в нём будет весь набор методов.

Заметная деталь: `pgUserRepository` не знает ни об одном из трёх интерфейсов. Их можно добавлять, сужать и переименовывать, не трогая реализацию, — именно это и делает ISP в Go дешёвым.

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
//   io.Copy(dst Writer, src Reader) (int64, error)  — не ReadWriteCloser
//   io.ReadAll(r Reader) ([]byte, error)            — не ReadCloser
```

Выигрыш от узости виден на вызове: `io.Copy` принимает `Writer`, поэтому в него передаётся и файл, и сетевое соединение, и `bytes.Buffer`, и `http.ResponseWriter`. Потребуй он `WriteCloser`, буфер перестал бы подходить — не потому, что не умеет писать, а потому, что ему незачем уметь закрываться.

---

## D — Dependency Inversion Principle

> 1. Модули высокого уровня не должны зависеть от модулей низкого уровня. Оба должны зависеть от абстракций.
> 2. Абстракции не должны зависеть от деталей. Детали должны зависеть от абстракций.

В Go принцип сводится к одному правилу размещения: интерфейс объявляет потребитель (высокий уровень), реализацию предоставляет поставщик (низкий уровень). Зависимость при этом разворачивается — инфраструктурный пакет зависит от интерфейса в домене, а не домен от инфраструктуры.

Слово «инверсия» означает именно смену направления стрелки в графе импортов, а не появление интерфейса как такового. Интерфейс, объявленный в пакете `postgres` и импортируемый доменом, ничего не разворачивает: домен по-прежнему не собирается без инфраструктуры, а тест по-прежнему тянет драйвер базы.

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
type PgOrderRepo struct { db *pgxpool.Pool }

// Конструктор возвращает конкретный тип, а не интерфейс:
// «принимай интерфейсы, возвращай структуры».
func NewOrderRepo(db *pgxpool.Pool) *PgOrderRepo {
    return &PgOrderRepo{db: db}
}

func (r *PgOrderRepo) Save(ctx context.Context, o *domain.Order) error { ... }
func (r *PgOrderRepo) FindByID(ctx context.Context, id domain.OrderID) (*domain.Order, error) { ... }

// Проверка соответствия на этапе компиляции — нулевой стоимости в рантайме.
var _ domain.OrderRepository = (*PgOrderRepo)(nil)
```

Возврат интерфейса из конструктора выглядит «более абстрактно», но обходится дороже. Он скрывает от вызывающего кода методы, которых нет в интерфейсе, — а они у реализации обычно есть: `Close`, `Ping`, `WithTx`. Он мешает документации: `go doc` по такому конструктору покажет интерфейс, а не то, что реально возвращается. И он лишает будущей гибкости: расширить набор методов у структуры можно, у чужого интерфейса — нет.

Строка `var _ domain.OrderRepository = (*PgOrderRepo)(nil)` при этом сохраняет главное — проверку соответствия контракту во время компиляции. Забытый метод обнаружится при сборке пакета `postgres`, а не при внедрении зависимости в `main`.

```
Направление зависимостей:
  postgres.PgOrderRepo ──► domain.OrderRepository ◄── domain.OrderService

Без DIP:
  domain.OrderService ──► postgres.PgOrderRepo  (высокий уровень зависит от низкого)
```

Стрелка читается как «импортирует». В верхней схеме `domain` не импортирует ничего: интерфейс объявлен в нём самом, поэтому пакет собирается и тестируется без драйвера базы. В нижней — `domain` тянет за собой `pgx`, а вместе с ним и требование поднять PostgreSQL для любого теста бизнес-логики.

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

- **SRP и ISP.** Маленькие интерфейсы с одной ответственностью проще реализовать целиком, а значит LSP нарушить труднее: нечем.
- **DIP опирается на ISP.** Широкий интерфейс невозможно удовлетворить честно, и в реализациях появляются заглушки с `panic` — то есть нарушения LSP.
- **OCP опирается на DIP.** Новое поведение добавляется новой реализацией только там, где потребитель уже зависит от интерфейса, а не от конкретного типа.
- **LSP — предпосылка OCP.** Расширение через новую реализацию работает лишь тогда, когда реализации действительно взаимозаменяемы.

---

## Типичные нарушения в Go

### 1. Interface в пакете поставщика (нарушение DIP + ISP)

```go
// Плохо — интерфейс в пакете postgres (поставщик объявляет сам себя)
package postgres

type OrderRepository interface {  // ❌ поставщик объявляет абстракцию
    Save(ctx context.Context, ...) error
    FindByID(ctx context.Context, ...) (*Order, error)
    // + 10 других методов, нужных только одному потребителю
}

// Хорошо — каждый потребитель объявляет нужный ему минимум в своём пакете
package service

type orderSaver interface { Save(ctx context.Context, o *Order) error }
```

Такой интерфейс не разворачивает зависимость: чтобы его упомянуть, потребитель импортирует `postgres` — то есть остаётся привязан к инфраструктуре. Плюс он неизбежно растёт, потому что описывает возможности реализации, а не потребности вызывающего кода.

### 2. `utils` пакет (нарушение SRP)

```go
// Плохо — utils как свалка
package utils

func ValidateEmail(s string) bool { ... }       // валидация
func FormatMoney(n int64) string { ... }        // форматирование
func GenerateID() string { ... }                // генерация
func ParseConfig(path string) (*Config, error)  // конфигурация
// Меняется по четырём разным причинам

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

**1. Чем SOLID в Go отличается от SOLID в языках с наследованием?**

- Главное отличие — расширение выражается новой реализацией интерфейса и композицией, а не подклассом.
- Второе — интерфейсы удовлетворяются неявно, поэтому объявить узкий интерфейс ничего не стоит.
- Третье — интерфейс живёт в пакете потребителя, и разворот зависимости получается настройкой по умолчанию.
- Обратная сторона — компилятор проверяет только сигнатуры, поэтому за соблюдением контракта следят тесты.

**2. Что означает SRP на практике?**

- Формулировка — не «делает одну вещь», а «меняется по одной причине».
- Разбиение по слоям — handler меняется от API, service от бизнес-правил, repository от схемы хранения.
- Проверка — если два изменения от разных людей и в разное время правят один файл, ответственностей больше одной.
- Классическое нарушение — пакет `utils`: валидация, форматирование, генерация идентификаторов и конфигурация в одном месте.

**3. Как выглядит OCP без наследования?**

- Инструмент — интерфейс плюс новая реализация: добавление `TelegramSender` не трогает существующие типы.
- Смысл принципа — не в запрете правок, а в том, что не нужно перепроверять уже работающий код.
- Тот же принцип в транспорте — цепочка middleware: новое сквозное поведение добавляется без правки обработчиков.

**4. Почему ISP в Go особенно естественен и как его нарушают?**

- Причина естественности — интерфейс объявляет потребитель, а реализация о нём не знает и ничего не объявляет.
- Образец — `io.Reader` и `io.Writer` по одному методу; `io.Copy` принимает `Writer`, поэтому подходит и буфер, и сокет.
- Нарушение — общий `UserRepository` на восемь методов: каждая заглушка в тесте вынуждена реализовать все.
- Как чинить — несколько узких именованных интерфейсов в пакетах-потребителях; анонимные интерфейсы в полях структур не использовать.

**5. Как выглядит DIP в Go?**

- Правило размещения — интерфейс в пакете потребителя (`domain.OrderRepository`), реализация в инфраструктурном (`postgres.PgOrderRepo`).
- Что именно инвертируется — направление импорта: `domain` не импортирует `pgx` и собирается без базы.
- Частая ошибка — интерфейс объявлен в пакете `postgres`: абстракция есть, инверсии нет.
- Конструктор — возвращает конкретный тип, а соответствие контракту фиксируется строкой `var _ domain.OrderRepository = (*PgOrderRepo)(nil)`.

**6. Как поймать нарушение LSP, если компилятор молчит?**

- Причина проблемы — система типов проверяет сигнатуры, а не поведение: `panic` внутри метода её полностью устраивает.
- Инструмент — общий набор тестов контракта, который прогоняется для каждой реализации.
- Что проверять — одинаковые ошибки в одинаковых сценариях, реакция на отмену контекста, отсутствие паники.
- Признак неизбежного нарушения — реализации нечего делать в части методов; значит, интерфейс нужно разделить.
