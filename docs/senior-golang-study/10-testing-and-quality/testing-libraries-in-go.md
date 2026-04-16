# Testing Libraries In Go

В Go почти всегда стоит начинать со стандартного `testing` package. Библиотеки нужны не для того, чтобы заменить базовый подход, а чтобы сделать конкретные типы тестов удобнее и надежнее.

## Содержание

- [Базовая позиция](#базовая-позиция)
- [1. `testify`](#1-testify)
- [2. `go-cmp`](#2-go-cmp)
- [3. `gomock`](#3-gomock)
- [4. `testcontainers-go`](#4-testcontainers-go)
- [Сравнение](#сравнение)
- [Что выбрать в типичных сценариях](#что-выбрать-в-типичных-сценариях)
- [Частые ошибки](#частые-ошибки)
- [Минимальный practical stack для команды](#минимальный-practical-stack-для-команды)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)
- [Связанные темы](#связанные-темы)

## Базовая позиция

Начинай с:
- стандартного `testing`;
- `httptest` для HTTP;
- обычных helper functions;
- реальных зависимостей или fakes там, где это разумно.

Подключай библиотеку, когда она дает измеримую пользу:
- лучше diff;
- удобнее сравнение структур;
- mock generation;
- контейнеры для integration tests.

## 1. `testify`

Сайт: [stretchr/testify](https://github.com/stretchr/testify)

### Что дает

Обычно используют две части:
- `assert` / `require`;
- `mock`.

### Когда полезен

`assert` / `require` удобны, когда:
- надо быстро писать readable failure checks;
- тестов много и хочется меньше boilerplate;
- команда любит такой стиль и использует его консистентно.

Пример:

```go
func TestNormalizePhone(t *testing.T) {
	got, err := NormalizePhone("+7 (999) 111-22-33")
	require.NoError(t, err)
	assert.Equal(t, "79991112233", got)
}
```

### Плюсы

- низкий порог входа;
- тесты пишутся быстро;
- `require` удобно останавливает тест на критической ошибке;
- хороший выбор для командного everyday use.

### Минусы

- `assert` может скрыть, что сравнение на самом деле не очень хорошее;
- чрезмерное использование `testify/mock` делает тесты шумными;
- для сложных структур `go-cmp` часто лучше, чем `assert.Equal`.

### Когда `testify/mock` лучше не использовать

- для repository logic;
- для HTTP client tests;
- для cases, где real integration test дает лучший сигнал;
- когда тест начинает проверять sequence of calls вместо business behavior.

## 2. `go-cmp`

Сайт: [google/go-cmp](https://github.com/google/go-cmp)

### Что дает

Это библиотека для качественного сравнения структур и вывода diff.

Пример:

```go
if diff := cmp.Diff(want, got); diff != "" {
	t.Fatalf("mismatch (-want +got):\n%s", diff)
}
```

### Когда полезен

- сложные nested structs;
- slices, maps, DTOs;
- результаты трансформации;
- API responses;
- domain objects.

### Плюсы

- хороший diff;
- меньше ручного кода;
- проще ревьюить падения;
- лучше подходит для semantic comparison, чем generic assert helpers.

### Минусы

- нужно понимать options вроде `cmpopts.IgnoreFields`;
- если злоупотреблять ignore-опциями, тест становится слишком мягким.

### Когда особенно хорош

Если тест проверяет "что получилось", а не "как вызывалось", `go-cmp` часто один из лучших инструментов.

## 3. `gomock`

Сайт: [uber-go/mock](https://github.com/uber-go/mock)

Нужно уточнение:
- исторически часто встречается `github.com/golang/mock/gomock`;
- в актуальных проектах нередко используют форк/продолжение от Uber.

### Что дает

- генерацию mock implementations;
- строгую проверку interaction expectations;
- удобен, когда dependency interface уже есть и нужно проверить контракт вызовов.

Пример идеи:

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

repo := NewMockUserRepo(ctrl)
repo.EXPECT().GetByID(gomock.Any(), "42").Return(User{ID: "42"}, nil)
```

### Когда полезен

- есть важный interaction contract;
- важно проверить side effect;
- зависимость сложно поднять реально;
- тест должен подтвердить, что конкретный вызов действительно произошел.

Примеры:
- publisher;
- audit logger;
- notifier;
- billing adapter;
- external command dispatcher.

### Плюсы

- строгость;
- автоматическая генерация;
- хорошо ловит неожиданное изменение interaction contract.

### Минусы

- тесты могут стать переусложненными;
- сильная связность с implementation details;
- рефакторинг без изменения поведения может ломать тесты;
- легко уйти в mock-driven development.

### Practical rule

Если важен результат, чаще лучше unit/integration test.
Если важен факт вызова и его параметры, `gomock` может быть уместен.

## 4. `testcontainers-go`

Сайт: [Testcontainers for Go](https://golang.testcontainers.org/)

### Что дает

Позволяет поднимать реальные зависимости в Docker для integration tests.

Типичные use cases:
- Postgres;
- Redis;
- Kafka;
- RabbitMQ;
- MongoDB;
- Elasticsearch/OpenSearch;
- MinIO.

### Когда полезен

- нужно протестировать реальную инфраструктурную семантику;
- mock/fake дает слишком слабый сигнал;
- поведение драйвера, схемы или infra itself является риском.

### Пример сценария

```go
ctx := context.Background()
pg, err := postgres.Run(ctx, "postgres:16-alpine")
if err != nil {
	t.Fatal(err)
}
defer func() { _ = testcontainers.TerminateContainer(pg) }()
```

### Плюсы

- высокая близость к production;
- сильные integration tests;
- меньше зависимости от ручной локальной среды.

### Минусы

- медленнее unit tests;
- нужен Docker;
- CI становится тяжелее;
- при плохом lifecycle management легко получить flaky tests.

### Когда особенно оправдан

- repository layer;
- migrations;
- transactional behavior;
- cache semantics;
- search/logging integrations;
- messaging infrastructure.

## Сравнение

`testing`:
- база по умолчанию;
- почти всегда нужен.

`testify`:
- ускоряет повседневные assertions;
- хорош как ergonomic helper layer.

`go-cmp`:
- лучший выбор для сравнения структур и meaningful diff.

`gomock`:
- полезен для strict interaction tests;
- легко переиспользовать не там, где нужно.

`testcontainers-go`:
- важен для real integration tests;
- дорогой, но часто самый честный вариант.

## Что выбрать в типичных сценариях

Проверка business logic:
- `testing`, иногда `testify/require`;
- для сложных результатов `go-cmp`.

Проверка nested DTO/domain mapping:
- `go-cmp`.

Проверка HTTP handler:
- `testing` + `httptest`;
- `testify` по вкусу команды.

Проверка HTTP client:
- `httptest.Server`;
- не mock `http.Client`, если нет очень веской причины.

Проверка вызова notifier/publisher:
- возможен `gomock` или простой spy/fake.

Проверка Postgres/Redis/Kafka behavior:
- `testcontainers-go`.

## Частые ошибки

- использовать `gomock` там, где хватило бы fake;
- писать repository tests на моках repository;
- заменять `go-cmp` на плохие generic asserts;
- строить integration tests через shared external environment вместо self-contained setup;
- тащить библиотеку просто потому, что "так принято".

## Минимальный practical stack для команды

Часто хватает такого:
- `testing`;
- `httptest`;
- `go-cmp`;
- `testify/require` по вкусу;
- `testcontainers-go` для integration tests;
- `gomock` только для отдельных interaction-heavy мест.

## Что могут спросить на интервью

- почему стандартного `testing` часто достаточно;
- когда `go-cmp` лучше `testify`;
- когда `gomock` оправдан, а когда он вредит;
- почему `testcontainers-go` может быть ценнее множества моков;
- какой testing stack ты бы выбрал для нового Go-сервиса и почему.

## Связанные темы

- [Unit Tests In Go](./unit-tests-in-go.md)
- [Integration, Contract And E2E Tests](./integration-contract-and-e2e-tests.md)
- [Test Doubles And Test Design](./test-doubles-and-test-design.md)
