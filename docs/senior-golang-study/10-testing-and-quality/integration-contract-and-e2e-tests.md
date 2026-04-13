# Integration, Contract And E2E Tests

Эти тесты стоят дороже unit tests, но именно они часто ловят самые болезненные production-баги.

## Integration tests

Integration test проверяет код вместе с реальной зависимостью или с очень близкой к реальности средой.

Типичные примеры:
- repository + реальный Postgres;
- cache layer + реальный Redis;
- HTTP client + test server через `httptest.Server`;
- migrations against real database;
- serialization against real library behavior.

### Когда integration test лучше unit test

- SQL сам по себе часть риска;
- важны индексы, транзакции, lock behavior;
- нужно проверить реальную схему JSON/protobuf;
- драйвер/ORM/query builder может вести себя неожиданно;
- behavior зависит от timeouts, headers, transport details.

### Когда integration tests особенно нужны в Go

- `database/sql`, `pgx`, `mongo-driver`;
- Redis operations;
- gRPC/HTTP clients;
- outbox/inbox patterns;
- migrations and schema evolution.

## `httptest.Server`

Для тестов HTTP client logic это обычно лучший first choice.

Пример:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":"rate limited"}`))
}))
defer srv.Close()

client := NewClient(srv.URL)
err := client.DoSomething(context.Background())
```

Это лучше, чем mock интерфейса `http.Client`, потому что:
- проверяется реальный transport-level behavior;
- можно тестировать headers, status codes, body, timeouts;
- меньше ложной уверенности.

## Testcontainers

`testcontainers-go` полезен, когда нужна реальная инфраструктура, но вручную ее поднимать неудобно.

Хорошо подходит для:
- Postgres;
- Redis;
- Kafka/RabbitMQ;
- Elasticsearch/OpenSearch;
- MinIO/S3-compatible tests.

Плюсы:
- высокое сходство с production;
- меньше mock drift;
- удобно для repository/integration tests.

Минусы:
- медленнее;
- Docker dependency;
- в CI нужен контроль времени и ресурсов.

## Contract tests

Contract test нужен, когда граница между системами сама по себе является риском.

Примеры:
- JSON REST contract;
- protobuf/gRPC compatibility;
- schema Kafka event;
- webhook payload;
- обязательные headers и auth expectations.

Что обычно проверяют:
- обязательные поля;
- backward compatibility;
- enum values;
- nullability;
- versioned changes.

## E2E tests

E2E test проходит через максимально реальный пользовательский или системный сценарий.

Примеры:
- регистрация -> логин -> создание сущности -> чтение результата;
- webhook received -> job queued -> DB updated;
- HTTP request -> service -> DB -> message broker -> consumer.

Плюсы:
- высокая уверенность;
- хороший smoke signal после deploy.

Минусы:
- дорогие;
- медленные;
- flakiness выше;
- диагностировать падение сложнее.

## Что не стоит делать

- заменять весь integration layer моками;
- писать много e2e tests на все подряд;
- ожидать, что e2e покроют всю matrix edge cases;
- держать тесты, которые случайно зависят от порядка, времени и внешней среды.

## Practical layering

Обычно хороший набор выглядит так:
- unit tests закрывают branching logic;
- integration tests закрывают adapters и persistence;
- contract tests закрывают межсервисные границы;
- e2e покрывают 2-5 самых критичных flows.

## Команды

```bash
go test ./...
go test ./... -run Integration
go test ./... -run Contract
```

Часто integration tests маркируют именованием, build tags или отдельными пакетами.

## Что могут спросить на интервью

- когда integration test полезнее десятка unit tests;
- зачем contract tests, если есть integration tests;
- почему e2e tests нельзя делать единственной стратегией;
- где `httptest.Server` лучше mock-интерфейсов;
- когда `testcontainers-go` оправдан, а когда избыточен.

## Связанные темы

- [Automated Testing Strategy](./automated-testing-strategy.md)
- [Test Doubles And Test Design](./test-doubles-and-test-design.md)
