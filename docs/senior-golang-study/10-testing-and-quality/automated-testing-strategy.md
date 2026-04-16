# Automated Testing Strategy

Автотесты в Go полезны не сами по себе, а как система обратной связи. Senior-level задача не "написать побольше тестов", а собрать такой набор проверок, который:
- реально ловит регрессии;
- не делает CI медленным и хрупким;
- помогает команде менять код без страха.

## Содержание

- [Главная идея](#главная-идея)
- [Что каким тестом лучше ловить](#что-каким-тестом-лучше-ловить)
- [Быстрый выбор](#быстрый-выбор)
- [Какой перекос встречается чаще всего](#какой-перекос-встречается-чаще-всего)
- [Practical testing pyramid для Go](#practical-testing-pyramid-для-go)
- [Что должно жить в CI](#что-должно-жить-в-ci)
- [Как не сделать тесты хрупкими](#как-не-сделать-тесты-хрупкими)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)
- [Связанные темы](#связанные-темы)

## Главная идея

Обычно тестовый набор состоит из нескольких слоев:
- `unit` - быстрая проверка логики в изоляции;
- `integration` - проверка взаимодействия с реальными зависимостями;
- `contract` - проверка совместимости API или сообщений между сервисами;
- `end-to-end` - проверка пользовательского сценария через всю систему.

Правильный вопрос почти всегда такой:
- какой самый дешевый тест поймает именно этот риск.

## Что каким тестом лучше ловить

`Unit tests`:
- ветвления в бизнес-логике;
- edge cases;
- расчетные функции;
- mapping, validation, policy logic.

`Integration tests`:
- SQL-запросы;
- Redis semantics;
- HTTP/gRPC integration;
- migrations;
- serialization/deserialization;
- взаимодействие с реальным драйвером или библиотекой.

`Contract tests`:
- backward compatibility API;
- формат Kafka/Rabbit/NATS messages;
- protobuf/JSON schema expectations;
- совместимость клиента и сервиса.

`E2E tests`:
- критичный пользовательский flow;
- happy path после деплоя;
- smoke tests на staging/prod-like среде.

## Быстрый выбор

Если меняется:

бизнес-логика:
- начинай с `unit test`.

работа с базой, Redis, HTTP transport:
- скорее нужен `integration test`.

граница между сервисами:
- часто нужен `contract test`.

критичный сценарий продукта:
- полезен `e2e` или хотя бы smoke test.

## Какой перекос встречается чаще всего

Слишком много mock-heavy unit tests:
- тесты быстрые, но плохо отражают реальное поведение;
- рефакторинг ломает тесты чаще, чем production behavior;
- взаимодействие с реальными зависимостями остается непроверенным.

Слишком много дорогих integration/e2e tests:
- CI медленный;
- flaky behavior;
- разработчики перестают доверять suite;
- локально запускать неудобно.

Лучший practical balance обычно такой:
- много быстрых unit tests на core logic;
- умеренное число integration tests на рискованные границы;
- небольшое число contract/e2e tests на critical paths.

## Practical testing pyramid для Go

Условно:
- основание: `unit tests`;
- средний слой: `integration + contract`;
- верхушка: `e2e`.

Но для backend в Go это часто не "классическая пирамида", а скорее:
- unit tests;
- integration tests с реальными зависимостями;
- немного e2e.

Причина:
- Go обычно хорошо тестируется через реальные adapters;
- mock-heavy тесты часто дают слабый сигнал;
- `httptest`, Docker и `testcontainers-go` делают интеграцию относительно доступной.

## Что должно жить в CI

Быстрый контур:

```bash
go test ./...
go test -race ./...
golangci-lint run
```

Медленный или выборочный контур:
- integration tests с контейнерами;
- e2e smoke tests;
- benchmark comparison;
- migration checks.

Частая практика:
- unit tests на каждый PR;
- integration tests на PR или nightly, зависит от скорости;
- e2e/smoke перед deploy или после deploy.

## Как не сделать тесты хрупкими

- тестируй поведение, а не внутреннюю реализацию;
- не привязывай каждый тест к приватным деталям;
- используй deterministic inputs;
- контролируй clock, random, network и внешние зависимости;
- отделяй "реально нужен mock" от "так было проще написать".

## Что могут спросить на интервью

- какие тесты ты бы написал первыми для нового Go-сервиса;
- почему десяток unit tests иногда хуже одного integration test;
- где mock-driven development начинает вредить;
- как сделать test suite быстрым и надежным;
- какие quality gates должны стоять в CI.

## Связанные темы

- [Unit Tests In Go](./unit-tests-in-go.md)
- [Integration, Contract And E2E Tests](./integration-contract-and-e2e-tests.md)
- [Test Doubles And Test Design](./test-doubles-and-test-design.md)
- [Race, Fuzz And Benchmarks](./race-fuzz-and-benchmarks.md)
