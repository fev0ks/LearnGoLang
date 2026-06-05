# Стратегия тестирования

Хорошая тестовая стратегия — не "писать побольше тестов", а собрать такой набор проверок, который реально ловит регрессии, не замедляет CI и не ломается от каждого рефакторинга.

## Содержание

- [Четыре слоя](#четыре-слоя)
- [Что каким слоем ловить](#что-каким-слоем-ловить)
- [Типичные перекосы](#типичные-перекосы)
- [Организация тестов в Go](#организация-тестов-в-go)
- [Build tags для разделения слоёв](#build-tags-для-разделения-слоёв)
- [TestMain — общий setup и teardown](#testmain--общий-setup-и-teardown)
- [Что должно быть в CI](#что-должно-быть-в-ci)

---

## Четыре слоя

```
         ┌────────────────┐
         │    E2E / smoke  │  мало, медленные, критичные flows
         ├────────────────┤
         │   integration   │  repository, HTTP client, gRPC, broker
         ├────────────────┤
         │    unit tests   │  логика, validation, mapping
         └────────────────┘
```

Каждый слой дороже предыдущего по времени запуска. Нижние слои должны быть самыми многочисленными.

---

## Что каким слоем ловить

**Unit** — изолированная проверка логики без внешних зависимостей:
- бизнес-правила и расчёты
- валидация, нормализация, маппинг
- error branching
- policy/retry/rate-limit логика

**Integration** — код + реальная или почти реальная зависимость:
- SQL-запросы и транзакции (реальный Postgres)
- cache операции (реальный Redis или miniredis)
- HTTP client (httptest.Server)
- gRPC client (bufconn)
- consumer/producer (реальный Kafka через testcontainers)
- миграции схемы

**Contract** — проверка границы между сервисами:
- JSON схема ответов API
- protobuf/Avro совместимость
- формат Kafka events
- обязательные заголовки

**E2E** — 2-5 самых критичных пользовательских сценариев на prod-like среде.

---

## Типичные перекосы

**Слишком много mock-heavy unit tests.** Тесты быстрые, но:
- реальное взаимодействие с DB/Redis/Kafka не проверяется
- рефакторинг внутренностей ломает тесты без изменения поведения
- confidence ложная

**Слишком много integration/e2e.** Тесты честные, но:
- CI занимает 20+ минут
- flaky failures из-за сети и контейнеров
- разработчики перестают запускать тесты локально

**Практический баланс для Go-бэкенда:**
```
unit tests       — много, быстрые (< 1s total на пакет)
integration      — умеренно, на реальных зависимостях
contract         — на каждой межсервисной границе
e2e              — 2-5 smoke сценариев
```

---

## Организация тестов в Go

### Файл рядом с кодом

```
user/
├── service.go
├── service_test.go        # unit tests
├── repository.go
└── repository_test.go     # unit + integration tests
```

### Отдельный пакет для white-box vs black-box

```go
// service_test.go — black-box test (package user_test)
// тестирует только экспортируемый API
package user_test

// service_internal_test.go — white-box test (package user)
// тестирует внутренние детали
package user
```

### Отдельная директория для E2E

```
tests/
└── e2e/
    ├── create_user_test.go
    └── checkout_flow_test.go
```

### Именование тестов

```go
// Формат: Test<ТипОбъекта>_<Метод>_<Сценарий>
func TestUserService_Create_DuplicateEmail(t *testing.T) { ... }
func TestOrderService_Checkout_InsufficientBalance(t *testing.T) { ... }

// Subtests через t.Run
func TestUserService_Create(t *testing.T) {
    t.Run("success", func(t *testing.T) { ... })
    t.Run("duplicate email", func(t *testing.T) { ... })
    t.Run("invalid email format", func(t *testing.T) { ... })
}
```

---

## Build tags для разделения слоёв

Без тегов `go test ./...` запустит всё, включая медленные integration тесты.

```go
//go:build integration

package user_test

// Этот тест запускается только при: go test -tags=integration ./...
func TestUserRepository_Create_Integration(t *testing.T) { ... }
```

```bash
# Только unit tests (быстро, для pre-commit)
go test ./...

# Unit + integration (в CI)
go test -tags=integration ./...

# Только integration тесты
go test -tags=integration -run Integration ./...
```

Альтернатива тегам — `testing.Short()`:

```go
func TestUserRepository_Create(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    // поднять контейнер, тестировать...
}
```

```bash
go test -short ./...   # пропустит тесты с t.Skip при testing.Short()
```

---

## TestMain — общий setup и teardown

`TestMain` выполняется один раз для всего пакета. Полезен для поднятия контейнеров, которые дорого создавать на каждый тест.

```go
// repository_test.go
package user_test

import (
    "context"
    "os"
    "testing"

    "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
    ctx := context.Background()

    // Поднять Postgres один раз для всего пакета
    pg, err := postgres.Run(ctx, "postgres:16-alpine",
        postgres.WithDatabase("testdb"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
    )
    if err != nil {
        panic(err)
    }
    defer testcontainers.TerminateContainer(pg)

    dsn, _ := pg.ConnectionString(ctx, "sslmode=disable")
    testDB, _ = sql.Open("pgx", dsn)

    // Применить миграции
    runMigrations(testDB)

    // Запустить тесты
    code := m.Run()
    os.Exit(code)
}

func TestUserRepository_Create(t *testing.T) {
    repo := NewRepository(testDB)
    // использовать testDB из TestMain
}
```

---

## Что должно быть в CI

**Быстрый контур** (каждый PR, < 2 мин):
```bash
go build ./...
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
```

**Полный контур** (каждый PR или по расписанию):
```bash
go test -tags=integration ./...
go test -tags=integration -race ./...
```

**Pre-deploy**:
```bash
go test -tags=e2e ./tests/e2e/...
```

Структура в GitHub Actions:
```yaml
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go test -race ./...

  integration:
    steps:
      - run: go test -tags=integration -timeout=5m ./...
```
