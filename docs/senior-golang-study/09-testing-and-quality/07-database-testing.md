# Тестирование с реальной базой данных

Репозитории нужно тестировать с реальной БД. Mock репозитория ничего не проверяет — он лишь подтверждает что код вызывает методы, но не то, что SQL-запрос корректен.

## Содержание

- [Когда нужна реальная БД](#когда-нужна-реальная-бд)
- [testcontainers-go: Postgres](#testcontainers-go-postgres)
- [Миграции в тестах](#миграции-в-тестах)
- [Изоляция между тестами](#изоляция-между-тестами)
- [Паттерны тестирования репозитория](#паттерны-тестирования-репозитория)
- [Тестирование транзакций](#тестирование-транзакций)
- [pgx-специфичные паттерны](#pgx-специфичные-паттерны)

---

## Когда нужна реальная БД

**Всегда** для:
- SQL-запросы и сложные JOIN'ы
- транзакции, уровни изоляции
- индексы и UNIQUE constraint'ы
- ON CONFLICT / UPSERT
- JSON/JSONB операции
- миграции схемы

**Не нужна** для:
- бизнес-логика в сервисах (там fake-репозиторий)
- mapping полей DTO → domain

---

## testcontainers-go: Postgres

```go
//go:build integration

package user_test

import (
    "context"
    "log"
    "os"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
)

var testPool *pgxpool.Pool

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
        log.Fatalf("start postgres container: %v", err)
    }
    defer testcontainers.TerminateContainer(pg)

    dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        log.Fatalf("get connection string: %v", err)
    }

    testPool, err = pgxpool.New(ctx, dsn)
    if err != nil {
        log.Fatalf("connect to postgres: %v", err)
    }
    defer testPool.Close()

    if err := runMigrations(ctx, testPool); err != nil {
        log.Fatalf("run migrations: %v", err)
    }

    os.Exit(m.Run())
}
```

---

## Миграции в тестах

### Через golang-migrate

```go
import (
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
    // DSN уже есть в pool.Config().ConnString()
    dsn := pool.Config().ConnString()

    m, err := migrate.New("file://../../migrations", "pgx5://"+dsn)
    if err != nil {
        return fmt.Errorf("new migrate: %w", err)
    }
    defer m.Close()

    if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
        return fmt.Errorf("migrate up: %w", err)
    }
    return nil
}
```

### Через goose

```go
import "github.com/pressly/goose/v3"

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
    db := stdlib.OpenDBFromPool(pool)  // pgx/stdlib

    goose.SetBaseFS(migrationsFS)      // embed.FS для тестов

    if err := goose.SetDialect("postgres"); err != nil {
        return err
    }
    return goose.Up(db, "migrations")
}
```

### Embedded миграции

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Не нужно думать о путях — миграции собраны в бинарь
```

---

## Изоляция между тестами

Контейнер поднимается один раз для всего пакета. Тесты изолируются через транзакции или truncate.

### Вариант 1: откат транзакции

Каждый тест работает в транзакции, которая откатывается в Cleanup. Самый надёжный способ — тест не может засорить данные других тестов.

```go
func withTx(t *testing.T, pool *pgxpool.Pool, fn func(ctx context.Context, tx pgx.Tx)) {
    t.Helper()
    ctx := context.Background()

    tx, err := pool.Begin(ctx)
    require.NoError(t, err)
    t.Cleanup(func() {
        // Всегда откатываем — даже если тест прошёл
        _ = tx.Rollback(ctx)
    })

    fn(ctx, tx)
}

func TestUserRepository_Create(t *testing.T) {
    withTx(t, testPool, func(ctx context.Context, tx pgx.Tx) {
        repo := NewRepository(tx)

        err := repo.Create(ctx, User{ID: "u1", Email: "alice@example.com"})
        require.NoError(t, err)

        got, err := repo.GetByID(ctx, "u1")
        require.NoError(t, err)
        assert.Equal(t, "alice@example.com", got.Email)
        // После теста транзакция откатится — u1 исчезнет из БД
    })
}
```

**Ограничение:** если код сам создаёт вложенные транзакции или использует SERIALIZABLE — с savepoint'ами могут быть проблемы.

### Вариант 2: truncate таблиц

Быстро, но нужно правильно перечислить таблицы и порядок (из-за FK).

```go
func truncate(t *testing.T, pool *pgxpool.Pool, tables ...string) {
    t.Helper()
    ctx := context.Background()
    query := fmt.Sprintf("TRUNCATE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
    _, err := pool.Exec(ctx, query)
    require.NoError(t, err)
}

func TestUserRepository_Create(t *testing.T) {
    truncate(t, testPool, "users", "sessions")

    repo := NewRepository(testPool)
    err := repo.Create(context.Background(), User{ID: "u1", Email: "alice@example.com"})
    require.NoError(t, err)
}
```

---

## Паттерны тестирования репозитория

```go
func TestUserRepository_Create(t *testing.T) {
    withTx(t, testPool, func(ctx context.Context, tx pgx.Tx) {
        repo := NewRepository(tx)

        t.Run("success", func(t *testing.T) {
            err := repo.Create(ctx, User{
                ID:    "u1",
                Email: "alice@example.com",
                Name:  "Alice",
            })
            require.NoError(t, err)

            // Сразу прочитать из БД — проверить что данные реально сохранились
            got, err := repo.GetByID(ctx, "u1")
            require.NoError(t, err)

            if diff := cmp.Diff(User{ID: "u1", Email: "alice@example.com", Name: "Alice"}, got,
                cmpopts.IgnoreFields(User{}, "CreatedAt", "UpdatedAt"),
            ); diff != "" {
                t.Errorf("mismatch (-want +got):\n%s", diff)
            }
        })

        t.Run("duplicate email returns error", func(t *testing.T) {
            _ = repo.Create(ctx, User{ID: "u2", Email: "bob@example.com"})

            err := repo.Create(ctx, User{ID: "u3", Email: "bob@example.com"})
            require.Error(t, err)
            assert.ErrorIs(t, err, ErrDuplicateEmail)
        })
    })
}

func TestUserRepository_GetByEmail(t *testing.T) {
    withTx(t, testPool, func(ctx context.Context, tx pgx.Tx) {
        repo := NewRepository(tx)

        // Вставить данные напрямую через SQL — быстрее чем через репозиторий
        _, err := tx.Exec(ctx,
            `INSERT INTO users (id, email, name, created_at) VALUES ($1, $2, $3, NOW())`,
            "u1", "alice@example.com", "Alice",
        )
        require.NoError(t, err)

        got, err := repo.GetByEmail(ctx, "alice@example.com")
        require.NoError(t, err)
        assert.Equal(t, "u1", got.ID)

        _, err = repo.GetByEmail(ctx, "missing@example.com")
        assert.ErrorIs(t, err, ErrNotFound)
    })
}
```

### Bulk-операции

```go
func TestUserRepository_ListByIDs(t *testing.T) {
    withTx(t, testPool, func(ctx context.Context, tx pgx.Tx) {
        repo := NewRepository(tx)

        ids := []string{"u1", "u2", "u3"}
        for i, id := range ids {
            require.NoError(t, repo.Create(ctx, User{
                ID:    id,
                Email: fmt.Sprintf("user%d@example.com", i),
            }))
        }

        got, err := repo.ListByIDs(ctx, ids)
        require.NoError(t, err)
        assert.Len(t, got, 3)

        // Порядок не гарантирован — сравнить через ElementsMatch
        gotIDs := make([]string, len(got))
        for i, u := range got {
            gotIDs[i] = u.ID
        }
        assert.ElementsMatch(t, ids, gotIDs)
    })
}
```

---

## Тестирование транзакций

```go
// Проверить что при ошибке транзакция откатывается
func TestOrderService_Checkout_RollsBackOnPaymentError(t *testing.T) {
    truncate(t, testPool, "orders", "order_items", "inventory")

    // Заполнить начальные данные
    seedInventory(t, testPool, "product-1", 5)

    repo := NewOrderRepository(testPool)
    paymentGW := &stubPaymentGateway{err: errors.New("payment declined")}
    svc := NewOrderService(repo, paymentGW)

    _, err := svc.Checkout(context.Background(), CheckoutRequest{
        ProductID: "product-1",
        Quantity:  2,
    })
    require.Error(t, err)

    // Проверить что инвентарь не уменьшился — транзакция откатилась
    count := getInventoryCount(t, testPool, "product-1")
    assert.Equal(t, 5, count)
}

// Проверить уровень изоляции
func TestOrderService_Checkout_SerializableConflict(t *testing.T) {
    truncate(t, testPool, "orders", "inventory")
    seedInventory(t, testPool, "product-1", 1)

    repo := NewOrderRepository(testPool)
    paymentGW := &stubPaymentGateway{}
    svc := NewOrderService(repo, paymentGW)

    ctx := context.Background()

    // Два параллельных заказа на один товар
    var wg sync.WaitGroup
    errs := make([]error, 2)
    for i := 0; i < 2; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            _, errs[i] = svc.Checkout(ctx, CheckoutRequest{ProductID: "product-1", Quantity: 1})
        }(i)
    }
    wg.Wait()

    // Ровно один должен выиграть
    successCount := 0
    for _, err := range errs {
        if err == nil {
            successCount++
        }
    }
    assert.Equal(t, 1, successCount, "only one checkout should succeed")
}
```

---

## pgx-специфичные паттерны

### Тест JSONB

```go
func TestUserRepository_SavePreferences(t *testing.T) {
    withTx(t, testPool, func(ctx context.Context, tx pgx.Tx) {
        repo := NewRepository(tx)

        prefs := UserPreferences{
            Theme:    "dark",
            Language: "ru",
            Notifications: NotificationSettings{
                Email: true,
                Push:  false,
            },
        }

        require.NoError(t, repo.SavePreferences(ctx, "u1", prefs))

        got, err := repo.GetPreferences(ctx, "u1")
        require.NoError(t, err)

        if diff := cmp.Diff(prefs, got); diff != "" {
            t.Errorf("preferences mismatch (-want +got):\n%s", diff)
        }
    })
}
```

### Тест pgx.ErrNoRows маппинга

```go
// Убедиться что репозиторий правильно маппит pgx.ErrNoRows → ErrNotFound
func TestUserRepository_GetByID_NotFound(t *testing.T) {
    withTx(t, testPool, func(ctx context.Context, tx pgx.Tx) {
        repo := NewRepository(tx)

        _, err := repo.GetByID(ctx, "non-existent")
        require.Error(t, err)

        // Убедиться что pgx.ErrNoRows не вытекает наружу
        assert.False(t, errors.Is(err, pgx.ErrNoRows),
            "pgx.ErrNoRows should be wrapped as domain error")
        assert.True(t, errors.Is(err, ErrNotFound),
            "should return ErrNotFound")
    })
}
```

### pgxmock — когда контейнер избыточен

Для unit-тестов репозитория когда важно проверить конкретный SQL без полного раунда через БД:

```go
import "github.com/pashagolub/pgxmock/v3"

func TestUserRepository_Create_SQL(t *testing.T) {
    mock, err := pgxmock.NewPool()
    require.NoError(t, err)
    defer mock.Close()

    mock.ExpectExec(`INSERT INTO users`).
        WithArgs("u1", "alice@example.com", "Alice", pgxmock.AnyArg()).
        WillReturnResult(pgxmock.NewResult("INSERT", 1))

    repo := NewRepository(mock)
    err = repo.Create(context.Background(), User{ID: "u1", Email: "alice@example.com", Name: "Alice"})

    require.NoError(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
}
```

**Когда pgxmock оправдан:** проверить точный SQL, параметры запроса, или поведение при конкретной ошибке БД (например, deadlock) — без поднятия контейнера.

**Когда не нужен:** если уже есть integration тест с реальным Postgres — pgxmock только дублирует проверку.
