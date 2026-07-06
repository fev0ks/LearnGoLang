# Testing And Quality

Раздел про тестирование Go-бэкенда: от стратегии до конкретных инструментов и паттернов для каждого слоя.

## Материалы

**Основы:**
- [01. Стратегия тестирования](./01-testing-strategy.md) — пирамида, build tags, TestMain, CI
- [02. Unit Tests в Go](./02-unit-tests-in-go.md) — table-driven, t.Parallel, t.Helper, t.Cleanup, clock injection
- [03. Test Doubles и проектирование](./03-test-doubles-and-test-design.md) — fake, stub, mock, spy, инъекция зависимостей
- [04. Библиотеки для тестирования](./04-testing-libraries-in-go.md) — testify, go-cmp, gomock, testcontainers

**По типу зависимости:**
- [05. Тестирование HTTP-сервера](./05-http-server-testing.md) — httptest.NewRecorder, httptest.NewServer, middleware, router
- [06. Тестирование gRPC-сервера](./06-grpc-testing.md) — bufconn, interceptors, streaming, status codes
- [07. Тестирование с реальной БД](./07-database-testing.md) — testcontainers postgres, миграции, изоляция транзакциями
- [08. Тестирование Redis и кэша](./08-redis-and-cache-testing.md) — testcontainers, miniredis, TTL, cache-aside, distributed lock
- [09. Тестирование Kafka](./09-kafka-testing.md) — testcontainers kafka, producer/consumer, outbox, DLQ

**Продвинутые темы:**
- [10. Integration, Contract и E2E](./10-integration-and-e2e.md) — HTTP client testing, contract tests, E2E flows, CI структура
- [11. Race detector, Fuzzing, Benchmarks](./11-race-fuzz-and-benchmarks.md) — -race, fuzz corpus, benchmem, benchstat

## Подборка

- [Package testing](https://pkg.go.dev/testing)
- [Fuzzing в Go](https://go.dev/doc/fuzz/)
- [govulncheck Tutorial](https://go.dev/doc/tutorial/govulncheck)
- [golangci-lint Docs](https://golangci-lint.run/docs/)
- [Testcontainers for Go](https://golang.testcontainers.org/)
- [alicebob/miniredis](https://github.com/alicebob/miniredis)
- [uber-go/mock](https://github.com/uber-go/mock)
- [google/go-cmp](https://github.com/google/go-cmp)

## Вопросы для подготовки

- какие тесты написать первыми для критичного Go-сервиса;
- когда mock уместен, а когда лучше поднять реальную зависимость;
- чем integration test полезнее десятка unit test в конкретном кейсе;
- как не превратить CI в медленный и хрупкий bottleneck;
- зачем нужны race test, fuzz test и benchmark, и где они реально окупаются;
- в чём разница между fake и mock и когда выбирать каждый.
