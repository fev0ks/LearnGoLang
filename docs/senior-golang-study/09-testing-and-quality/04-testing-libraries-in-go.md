# Библиотеки для тестирования в Go

Стандартный `testing` — основа. Библиотеки добавляются когда дают измеримую пользу: лучший diff, удобные assert'ы, мок-генерация, реальные контейнеры.

## Содержание

- [testify — assert и require](#testify--assert-и-require)
- [go-cmp — сравнение структур](#go-cmp--сравнение-структур)
- [uber-go/mock (gomock) — генерация моков](#uber-gomock-gomock--генерация-моков)
- [testcontainers-go — реальные зависимости](#testcontainers-go--реальные-зависимости)
- [Минимальный stack](#минимальный-stack)

---

## testify — assert и require

[stretchr/testify](https://github.com/stretchr/testify)

Ускоряет написание тестов за счёт выразительных assert-функций.

### assert vs require

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestCreateUser(t *testing.T) {
    user, err := svc.Create(ctx, req)

    // require — останавливает тест при провале (как t.Fatal)
    require.NoError(t, err)          // если err != nil → тест немедленно останавливается
    require.NotNil(t, user)

    // assert — продолжает тест при провале (как t.Error)
    assert.Equal(t, "alice@example.com", user.Email)
    assert.Equal(t, "Alice", user.Name)
    assert.NotEmpty(t, user.ID)
    assert.True(t, user.CreatedAt.Before(time.Now()))
}
```

**Правило:** `require` для предусловий (если nil — дальше незачем проверять), `assert` для независимых проверок результата.

### Часто используемые функции

```go
// Равенство
assert.Equal(t, expected, actual)
assert.NotEqual(t, unexpected, actual)

// Nil / empty
assert.Nil(t, err)
assert.NoError(t, err)          // то же что Nil, но лучше сообщение для ошибок
assert.NotNil(t, obj)
assert.Empty(t, slice)          // len == 0
assert.NotEmpty(t, slice)

// Булевы
assert.True(t, condition)
assert.False(t, condition)

// Строки
assert.Contains(t, "hello world", "world")
assert.HasPrefix(t, "hello world", "hello")

// Коллекции
assert.Len(t, slice, 3)
assert.ElementsMatch(t, []int{1, 2, 3}, []int{3, 1, 2})  // порядок не важен
assert.Contains(t, map, key)

// Ошибки
assert.Error(t, err)
assert.EqualError(t, err, "not found")
assert.ErrorIs(t, err, ErrNotFound)      // проверяет через errors.Is
assert.ErrorAs(t, err, &target)          // проверяет через errors.As

// Типы
assert.IsType(t, (*User)(nil), obj)
assert.Implements(t, (*io.Reader)(nil), obj)
```

### Custom сообщение в assert

```go
assert.Equal(t, expected, actual, "user ID after create should match")
assert.NoError(t, err, "creating user with email %q", email)
```

### Когда testify не нужен

- простой тест с одной проверкой — `if got != want { t.Errorf(...) }` читается не хуже
- для сложных структур `go-cmp` даёт лучший diff
- `testify/mock` чаще всего хуже `gomock` или fake

---

## go-cmp — сравнение структур

[google/go-cmp](https://github.com/google/go-cmp)

Лучший инструмент для сравнения сложных структур. Показывает human-readable diff.

### Базовое использование

```go
import "github.com/google/go-cmp/cmp"

func TestGetUser(t *testing.T) {
    want := User{
        ID:    "123",
        Email: "alice@example.com",
        Name:  "Alice",
    }

    got, err := repo.GetByID(ctx, "123")
    require.NoError(t, err)

    if diff := cmp.Diff(want, got); diff != "" {
        t.Errorf("GetByID() mismatch (-want +got):\n%s", diff)
    }
}

// При провале вывод:
// GetByID() mismatch (-want +got):
//   user.User{
//     ID:    "123",
// -   Email: "alice@example.com",
// +   Email: "ALICE@EXAMPLE.COM",
//     Name:  "Alice",
//   }
```

### cmpopts — опции сравнения

```go
import "github.com/google/go-cmp/cmp/cmpopts"

// Игнорировать поля (например, ID и Timestamp которые генерируются)
if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(User{}, "ID", "CreatedAt")); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}

// Сортировать слайсы перед сравнением (когда порядок не важен)
if diff := cmp.Diff(wantUsers, gotUsers,
    cmpopts.SortSlices(func(a, b User) bool { return a.ID < b.ID }),
); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}

// Игнорировать неэкспортируемые поля
if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(User{})); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}

// Сравнить с допуском (для float)
if diff := cmp.Diff(want, got, cmpopts.EquateApprox(0, 0.001)); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}

// Считать пустой слайс и nil равными
if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}
```

### Сравнение с трансформацией

```go
// Сравнивать time.Time только до секунды (игнорировать наносекунды)
timeOpt := cmp.Comparer(func(a, b time.Time) bool {
    return a.Truncate(time.Second).Equal(b.Truncate(time.Second))
})

if diff := cmp.Diff(want, got, timeOpt); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}
```

### Собственный transformer

```go
// Нормализовать перед сравнением
emailTransformer := cmp.Transformer("lower", func(s string) string {
    return strings.ToLower(s)
})

if diff := cmp.Diff(want, got, emailTransformer); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}
```

---

## uber-go/mock (gomock) — генерация моков

[uber-go/mock](https://github.com/uber-go/mock) — актуальный форк исходного golang/mock.

### Установка

```bash
go install go.uber.org/mock/mockgen@latest
```

### Генерация

```bash
# Из файла с интерфейсом (source mode)
mockgen -source=internal/user/notifier.go \
        -destination=internal/user/mock_notifier_test.go \
        -package=user_test

# Из пакета (reflect mode) — удобно для интерфейсов из внешних пакетов
mockgen -destination=mock_db.go \
        -package=mytest \
        database/sql DB

# Через go:generate
//go:generate mockgen -source=notifier.go -destination=mock_notifier_test.go -package=user_test
```

### Базовый паттерн

```go
func TestPaymentService_Charge(t *testing.T) {
    ctrl := gomock.NewController(t)
    // ctrl.Finish() вызывается автоматически через t.Cleanup в новых версиях

    gateway := NewMockPaymentGateway(ctrl)
    repo := newFakeOrderRepo()
    svc := NewPaymentService(gateway, repo)

    // Задать ожидание
    gateway.EXPECT().
        Charge(gomock.Any(), ChargeRequest{
            OrderID: "order-123",
            Amount:  1500,
            Currency: "USD",
        }).
        Return(&ChargeResponse{TransactionID: "txn-456"}, nil).
        Times(1)

    result, err := svc.ProcessPayment(ctx, "order-123")
    require.NoError(t, err)
    assert.Equal(t, "txn-456", result.TransactionID)
}
```

### Matchers для гибких ожиданий

```go
// Any — любое значение
repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)

// Конкретное значение
notifier.EXPECT().Send(ctx, "user-123", "welcome")

// Условие через функцию
gateway.EXPECT().
    Charge(gomock.Any(), gomock.Cond(func(req ChargeRequest) bool {
        return req.Amount > 0 && req.Currency != ""
    })).
    Return(nil, nil)

// Do — выполнить функцию при вызове (полезно для захвата аргументов)
var capturedReq ChargeRequest
gateway.EXPECT().
    Charge(gomock.Any(), gomock.Any()).
    DoAndReturn(func(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
        capturedReq = req
        return &ChargeResponse{TransactionID: "txn-1"}, nil
    })
// Потом проверить capturedReq
```

### Порядок вызовов

```go
// Задать обязательный порядок
first := repo.EXPECT().Get(ctx, id).Return(order, nil)
second := gateway.EXPECT().Charge(ctx, req).Return(resp, nil).After(first)
repo.EXPECT().Update(ctx, order).Return(nil).After(second)
```

### Идиоматичная обёртка для удобства

```go
// Вместо повторения NewMockXxx + EXPECT в каждом тесте — фабрика
func newTestPaymentService(t *testing.T) (*PaymentService, *MockPaymentGateway) {
    t.Helper()
    ctrl := gomock.NewController(t)
    gateway := NewMockPaymentGateway(ctrl)
    return NewPaymentService(gateway, newFakeOrderRepo()), gateway
}

func TestPaymentService_Charge_GatewayError(t *testing.T) {
    svc, gateway := newTestPaymentService(t)

    gateway.EXPECT().
        Charge(gomock.Any(), gomock.Any()).
        Return(nil, errors.New("gateway timeout"))

    _, err := svc.ProcessPayment(ctx, "order-1")
    assert.ErrorIs(t, err, ErrPaymentFailed)
}
```

---

## testcontainers-go — реальные зависимости

[testcontainers-go](https://golang.testcontainers.org/) — поднять реальный Docker контейнер в тесте.

### PostgreSQL

```go
import (
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
)

func TestMain(m *testing.M) {
    ctx := context.Background()

    pg, err := postgres.Run(ctx,
        "postgres:16-alpine",
        postgres.WithDatabase("testdb"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).
                WithStartupTimeout(30*time.Second),
        ),
    )
    if err != nil {
        log.Fatalf("start postgres: %v", err)
    }
    defer testcontainers.TerminateContainer(pg)

    dsn, _ := pg.ConnectionString(ctx, "sslmode=disable")

    testDB, err = pgxpool.New(ctx, dsn)
    if err != nil {
        log.Fatalf("connect: %v", err)
    }
    defer testDB.Close()

    runMigrations(testDB)

    os.Exit(m.Run())
}
```

### Redis

```go
import "github.com/testcontainers/testcontainers-go/modules/redis"

func startTestRedis(t *testing.T) *goredis.Client {
    t.Helper()
    ctx := context.Background()

    rc, err := redis.Run(ctx, "redis:7-alpine")
    require.NoError(t, err)
    t.Cleanup(func() { testcontainers.TerminateContainer(rc) })

    addr, _ := rc.ConnectionString(ctx)
    return goredis.NewClient(&goredis.Options{Addr: strings.TrimPrefix(addr, "redis://")})
}
```

### Kafka

```go
import "github.com/testcontainers/testcontainers-go/modules/kafka"

func startTestKafka(t *testing.T) string {
    t.Helper()
    ctx := context.Background()

    kc, err := kafka.Run(ctx, "confluentinc/cp-kafka:7.6.1")
    require.NoError(t, err)
    t.Cleanup(func() { testcontainers.TerminateContainer(kc) })

    brokers, _ := kc.Brokers(ctx)
    return brokers[0]
}
```

### Переиспользование контейнера через TestMain

```go
// Контейнер поднимается один раз на весь пакет тестов — не на каждый тест
var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
    setup()
    code := m.Run()
    teardown()
    os.Exit(code)
}
```

### Изоляция между тестами

Вместо пересоздания контейнера — транзакции или truncate:

```go
// Вариант 1: откат транзакции после каждого теста
func withTx(t *testing.T, db *pgxpool.Pool, fn func(pgx.Tx)) {
    t.Helper()
    tx, err := db.Begin(context.Background())
    require.NoError(t, err)
    t.Cleanup(func() { tx.Rollback(context.Background()) })
    fn(tx)
}

// Вариант 2: truncate всех таблиц
func truncateTables(t *testing.T, db *pgxpool.Pool) {
    t.Helper()
    _, err := db.Exec(ctx, `TRUNCATE users, orders, payments RESTART IDENTITY CASCADE`)
    require.NoError(t, err)
}

func TestUserRepository_Create(t *testing.T) {
    truncateTables(t, testDB)
    repo := NewRepository(testDB)
    // тест...
}
```

---

## Минимальный stack

```
testing            — всегда
testify/require    — для удобных assertions и быстрого написания
go-cmp             — для сравнения сложных структур (вместо assert.Equal на структурах)
gomock             — только для мест где важен interaction contract
testcontainers-go  — для integration tests с реальными зависимостями
```

Что **не нужно** добавлять без причины:
- `testify/mock` — замени на fake или gomock
- отдельный BDD-фреймворк (goconvey, ginkgo) — оверхед без выгоды в большинстве случаев
- кастомный test runner — стандартный `go test` достаточен
