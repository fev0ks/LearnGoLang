# Test Doubles And Test Design

Одна из самых частых ошибок в Go-тестах: команда пишет много моков и думает, что это хорошая test strategy. На практике нужно различать виды test doubles и понимать, когда каждый из них уместен.

## Основные варианты

`Dummy`:
- просто заполнитель;
- нужен, чтобы удовлетворить сигнатуру, но в тесте не используется.

`Stub`:
- возвращает заранее заданный результат;
- полезен, когда нужно контролировать ветвление.

`Fake`:
- рабочая, но упрощенная реализация;
- пример: in-memory repository вместо Postgres.

`Mock`:
- не только возвращает данные, но и проверяет ожидания по вызовам;
- полезен, когда важен interaction contract.

`Spy`:
- записывает, как его вызвали;
- потом тест проверяет факты взаимодействия.

## Что в Go чаще работает лучше

Часто practical best choice такой:
- `fake` для stateful dependency;
- `stub` для простого ветвления;
- `httptest.Server` для HTTP;
- реальная база/Redis для integration layer;
- `mock` только там, где interaction действительно и есть риск.

## Когда mock уместен

- нужно проверить, что side effect вызван;
- важен exact interaction contract;
- реальную зависимость поднимать слишком дорого;
- dependency нестабильна или плохо контролируется в тесте.

Примеры:
- notifier;
- publisher;
- audit sink;
- metrics/tracing facade, если поведение важно через interaction.

## Когда mock вреден

- repository tests против замоканного repository;
- HTTP client tests через моки вместо test server;
- DB logic тестируется без реальной DB semantics;
- тесты привязаны к конкретной sequence of calls, хотя бизнес-результат тот же.

Итог:
- код рефакторится тяжело;
- тесты шумные;
- confidence невысокая.

## Fakes

Fakes в Go часто очень сильный инструмент.

Пример хорошего fake:
- in-memory implementation того же интерфейса;
- детерминированное поведение;
- удобно настраивать тестом;
- отражает business semantics лучше, чем мок ожиданий.

Но fake опасен, если:
- его поведение слишком сильно расходится с production dependency;
- команда начинает доверять fake больше, чем реальной интеграции.

## How to design code for testability

- зависимость должна быть инъецируема;
- интерфейсы лучше объявлять со стороны потребителя;
- не смешивай transport, business logic и persistence в одну функцию;
- избегай скрытых глобальных зависимостей;
- контроль времени и randomness должен быть подменяемым.

## Пример stub/fake-подхода

```go
type UserReader interface {
	GetByID(ctx context.Context, id string) (User, error)
}

type fakeRepo struct {
	users map[string]User
	err   error
}

func (f *fakeRepo) GetByID(ctx context.Context, id string) (User, error) {
	if f.err != nil {
		return User{}, f.err
	}
	user, ok := f.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return user, nil
}
```

Такой fake часто дает лучший сигнал, чем mock framework с длинным списком expectation calls.

## Библиотеки

`testify`:
- удобные asserts;
- многим командам нравится за скорость написания;
- mocks есть, но их стоит использовать осторожно.

`go-cmp`:
- отличный выбор для сравнения структур;
- обычно полезнее, чем набор generic assert helpers.

`gomock`:
- мощный инструмент для strict interaction testing;
- полезен, но легко перегрузить тесты лишней связностью.

## Practical rule of thumb

Если проверяешь:

результат вычисления:
- начинай с обычного unit test.

простое ветвление на ошибке:
- чаще хватит stub/fake.

вызов важного side effect:
- возможен spy/mock.

реальное взаимодействие с инфраструктурой:
- чаще нужен integration test, а не mock.

## Что могут спросить на интервью

- чем fake отличается от mock;
- почему mock-heavy suite может мешать рефакторингу;
- когда `httptest.Server` лучше, чем mock HTTP client;
- когда in-memory fake полезен, а когда опасен;
- как проектировать код так, чтобы его было удобно тестировать.

## Связанные темы

- [Unit Tests In Go](./unit-tests-in-go.md)
- [Integration, Contract And E2E Tests](./integration-contract-and-e2e-tests.md)
