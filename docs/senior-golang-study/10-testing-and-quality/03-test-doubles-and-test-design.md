# Test Doubles и проектирование тестируемого кода

Test double — любой заменитель реальной зависимости в тестах. Понимание разницы между видами double помогает выбирать правильный инструмент.

## Содержание

- [Виды test doubles](#виды-test-doubles)
- [Fake — in-memory реализация](#fake--in-memory-реализация)
- [Stub — заготовленный ответ](#stub--заготовленный-ответ)
- [Mock — проверка взаимодействия](#mock--проверка-взаимодействия)
- [Spy — запись вызовов](#spy--запись-вызовов)
- [Когда что выбирать](#когда-что-выбирать)
- [Проектирование кода для тестируемости](#проектирование-кода-для-тестируемости)
- [Антипаттерны mock-heavy тестов](#антипаттерны-mock-heavy-тестов)

---

## Виды test doubles

| Тип | Что делает | Проверяет |
|---|---|---|
| **Dummy** | заполнитель, не вызывается | ничего |
| **Stub** | возвращает заготовленный результат | состояние (что вернулось) |
| **Fake** | рабочая упрощённая реализация | состояние через реальную логику |
| **Spy** | записывает вызовы | взаимодействие post-factum |
| **Mock** | ожидает конкретные вызовы | взаимодействие до факта |

---

## Fake — in-memory реализация

Fake — рабочая, но упрощённая реализация. Отражает бизнес-логику зависимости без инфраструктуры.

```go
type UserRepository interface {
    Create(ctx context.Context, u User) error
    GetByID(ctx context.Context, id string) (User, error)
    GetByEmail(ctx context.Context, email string) (User, error)
    Delete(ctx context.Context, id string) error
}

// fakeUserRepo — in-memory реализация для тестов
type fakeUserRepo struct {
    mu    sync.RWMutex
    users map[string]User
    // Управляемые ошибки для конкретных сценариев
    createErr error
    getErr    error
}

func newFakeUserRepo() *fakeUserRepo {
    return &fakeUserRepo{users: make(map[string]User)}
}

func (f *fakeUserRepo) Create(ctx context.Context, u User) error {
    if f.createErr != nil {
        return f.createErr
    }
    f.mu.Lock()
    defer f.mu.Unlock()
    if _, exists := f.users[u.ID]; exists {
        return ErrAlreadyExists
    }
    // Проверить уникальность email — как в реальной БД
    for _, existing := range f.users {
        if existing.Email == u.Email {
            return ErrDuplicateEmail
        }
    }
    f.users[u.ID] = u
    return nil
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id string) (User, error) {
    if f.getErr != nil {
        return User{}, f.getErr
    }
    f.mu.RLock()
    defer f.mu.RUnlock()
    u, ok := f.users[id]
    if !ok {
        return User{}, ErrNotFound
    }
    return u, nil
}

func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (User, error) {
    f.mu.RLock()
    defer f.mu.RUnlock()
    for _, u := range f.users {
        if u.Email == email {
            return u, nil
        }
    }
    return User{}, ErrNotFound
}

func (f *fakeUserRepo) Delete(ctx context.Context, id string) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    delete(f.users, id)
    return nil
}

// Тест с fake
func TestUserService_Create(t *testing.T) {
    repo := newFakeUserRepo()
    svc := NewUserService(repo)

    t.Run("success", func(t *testing.T) {
        err := svc.Create(ctx, CreateUserRequest{
            Email: "alice@example.com",
            Name:  "Alice",
        })
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
    })

    t.Run("duplicate email", func(t *testing.T) {
        // Первое создание
        _ = svc.Create(ctx, CreateUserRequest{Email: "bob@example.com"})
        // Второе с тем же email
        err := svc.Create(ctx, CreateUserRequest{Email: "bob@example.com"})
        if !errors.Is(err, ErrDuplicateEmail) {
            t.Errorf("got %v, want ErrDuplicateEmail", err)
        }
    })

    t.Run("repo returns error", func(t *testing.T) {
        repo.createErr = errors.New("database unavailable")
        err := svc.Create(ctx, CreateUserRequest{Email: "carol@example.com"})
        if err == nil {
            t.Fatal("expected error")
        }
    })
}
```

**Fake хорош когда:**
- зависимость stateful (хранит данные)
- нужно проверить несколько взаимодействий подряд
- бизнес-логика важнее, чем порядок вызовов
- real dependency слишком дорого поднимать

---

## Stub — заготовленный ответ

Stub возвращает заготовленное значение. Проще fake, не имеет собственной логики.

```go
type stubExchangeRateClient struct {
    rate float64
    err  error
}

func (s *stubExchangeRateClient) GetRate(ctx context.Context, from, to string) (float64, error) {
    return s.rate, s.err
}

func TestPriceCalculator_InCurrency(t *testing.T) {
    tests := []struct {
        name     string
        rate     float64
        rateErr  error
        price    float64
        currency string
        want     float64
        wantErr  bool
    }{
        {
            name:     "converts at given rate",
            rate:     1.2,
            price:    100,
            currency: "EUR",
            want:     120,
        },
        {
            name:     "rate service unavailable",
            rateErr:  errors.New("timeout"),
            price:    100,
            currency: "EUR",
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            client := &stubExchangeRateClient{rate: tt.rate, err: tt.rateErr}
            calc := NewPriceCalculator(client)

            got, err := calc.ConvertPrice(ctx, tt.price, tt.currency)
            if tt.wantErr {
                if err == nil {
                    t.Fatal("expected error")
                }
                return
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

---

## Mock — проверка взаимодействия

Mock проверяет не только что вернулось, но и как именно была вызвана зависимость.

Генерация через `mockgen` (uber-go/mock):

```bash
# Из интерфейса в файле
mockgen -source=notifier.go -destination=mock_notifier.go -package=usertest

# Из пакета
mockgen -destination=mock_repo.go -package=usertest github.com/myapp/user UserRepository
```

```go
//go:generate mockgen -source=notifier.go -destination=mock_notifier.go -package=usertest

type Notifier interface {
    SendWelcomeEmail(ctx context.Context, userID, email string) error
    SendPasswordReset(ctx context.Context, userID, token string) error
}
```

```go
// Использование mock в тесте
func TestUserService_Create_SendsWelcomeEmail(t *testing.T) {
    ctrl := gomock.NewController(t)  // t.Cleanup(ctrl.Finish) вызывается автоматически

    notifier := NewMockNotifier(ctrl)
    repo := newFakeUserRepo()
    svc := NewUserService(repo, notifier)

    // Ожидание: SendWelcomeEmail вызван ровно один раз с любым ctx и конкретным email
    notifier.EXPECT().
        SendWelcomeEmail(gomock.Any(), gomock.Any(), "alice@example.com").
        Return(nil).
        Times(1)

    err := svc.Create(ctx, CreateUserRequest{Email: "alice@example.com"})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // gomock проверит что ожидание выполнено после теста
}

func TestUserService_Create_NotifierError_DoesNotFail(t *testing.T) {
    ctrl := gomock.NewController(t)
    notifier := NewMockNotifier(ctrl)
    repo := newFakeUserRepo()
    svc := NewUserService(repo, notifier)

    // Email ошибка не должна фейлить создание пользователя
    notifier.EXPECT().
        SendWelcomeEmail(gomock.Any(), gomock.Any(), gomock.Any()).
        Return(errors.New("smtp unavailable"))

    err := svc.Create(ctx, CreateUserRequest{Email: "bob@example.com"})
    if err != nil {
        t.Errorf("notifier error should not propagate: %v", err)
    }
}
```

**Полезные matchers:**
```go
gomock.Any()                          // любое значение
gomock.Eq("alice@example.com")        // точное равенство
gomock.Not(gomock.Eq(""))             // не равно
gomock.InAnyOrder([]string{"a","b"})  // список в любом порядке

// Кастомный matcher
gomock.AssignableToTypeOf((*User)(nil))  // тип совпадает
```

**Times/AnyTimes/AtLeast:**
```go
.Times(1)         // ровно 1 раз
.Times(0)         // не должен вызываться
.AnyTimes()       // 0 или больше раз
.MinTimes(1)      // минимум 1 раз
.MaxTimes(3)      // максимум 3 раза
```

---

## Spy — запись вызовов

Spy записывает факты взаимодействия для проверки post-factum. Проще mock — не нужно задавать ожидания заранее.

```go
type spyEventPublisher struct {
    mu     sync.Mutex
    events []Event
    err    error
}

func (s *spyEventPublisher) Publish(ctx context.Context, e Event) error {
    s.mu.Lock()
    s.events = append(s.events, e)
    s.mu.Unlock()
    return s.err
}

func (s *spyEventPublisher) PublishedEvents() []Event {
    s.mu.Lock()
    defer s.mu.Unlock()
    cp := make([]Event, len(s.events))
    copy(cp, s.events)
    return cp
}

// Тест со spy
func TestOrderService_Checkout_PublishesEvent(t *testing.T) {
    publisher := &spyEventPublisher{}
    svc := NewOrderService(fakeRepo, publisher)

    order, err := svc.Checkout(ctx, cartID)
    if err != nil {
        t.Fatalf("checkout: %v", err)
    }

    events := publisher.PublishedEvents()
    if len(events) != 1 {
        t.Fatalf("expected 1 event, got %d", len(events))
    }
    if events[0].Type != "order.created" {
        t.Errorf("got event type %q, want %q", events[0].Type, "order.created")
    }
    if events[0].OrderID != order.ID {
        t.Errorf("event order ID mismatch")
    }
}
```

Spy удобен когда:
- нужно проверить факт вызова, но не строгий порядок
- ожидания сложно задать заранее (зависят от результата операции)
- mock framework избыточен

---

## Когда что выбирать

```
Нужно проверить результат вычисления?
  → unit test + stub для простых зависимостей

Dependency stateful (хранит данные)?
  → fake (in-memory реализация)

Важен факт вызова side effect (email, event, audit)?
  → spy или mock

Важен точный порядок и параметры вызовов?
  → mock (gomock)

Нужна реальная семантика DB/Redis/Kafka?
  → integration test с реальной зависимостью
    (testcontainers или httptest.Server)
```

---

## Проектирование кода для тестируемости

**Инъекция зависимостей через конструктор:**

```go
// Плохо — зависимости скрыты, нельзя подменить
func NewUserService() *UserService {
    return &UserService{
        repo:     postgres.NewRepository(os.Getenv("DB_DSN")),
        notifier: smtp.NewNotifier(os.Getenv("SMTP_HOST")),
        clock:    realClock{},
    }
}

// Хорошо — все зависимости явны и подменяемы
func NewUserService(repo UserRepository, notifier Notifier, clock Clock) *UserService {
    return &UserService{repo: repo, notifier: notifier, clock: clock}
}
```

**Интерфейсы объявляются потребителем:**

```go
// Плохо — интерфейс в пакете реализации (producer-defined)
// package postgres
type UserRepository interface { ... }  // ← в пакете postgres

// Хорошо — интерфейс в пакете потребителя (consumer-defined)
// package user
type UserRepository interface {  // ← только методы которые нужны user-пакету
    Create(ctx context.Context, u User) error
    GetByID(ctx context.Context, id string) (User, error)
}
```

**Не смешивать transport, логику и persistence:**

```go
// Плохо — всё в одном месте
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    json.NewDecoder(r.Body).Decode(&req)       // transport
    if req.Email == "" { /* validate */ }       // validation
    db.Exec("INSERT INTO users ...", req.Email) // persistence
    sendEmail(req.Email)                        // side effect
    json.NewEncoder(w).Encode(resp)             // transport
}

// Хорошо — разделение ответственности
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    json.NewDecoder(r.Body).Decode(&req)   // transport: parse
    user, err := h.svc.Create(r.Context(), req)  // сервис: всё остальное
    if err != nil { h.writeError(w, err); return }
    writeJSON(w, http.StatusCreated, user)  // transport: encode
}
```

---

## Антипаттерны mock-heavy тестов

**Repository mock:** тестировать repository через mock самого repository — бессмысленно. Тест проверяет что код вызывает методы, а не что данные корректно сохраняются.

```go
// Бессмысленно
repo.EXPECT().Create(ctx, user).Return(nil)
// Что мы проверили? Что Create был вызван. Это очевидно из кода.
// Реальный вопрос — правильно ли данные попадают в DB.
// На это отвечает только integration test.
```

**Тест завязан на порядок вызовов без причины:**
```go
// Хрупко — порядок вызовов может измениться при рефакторинге
gomock.InOrder(
    repo.EXPECT().GetByEmail(ctx, email),
    notifier.EXPECT().SendWelcomeEmail(ctx, userID, email),
    repo.EXPECT().UpdateLastLogin(ctx, userID),
)
```

**Слишком много `.Times(N)`:** если тест ломается когда метод вызван 2 раза вместо 1 без бизнес-причины — тест хрупкий.
