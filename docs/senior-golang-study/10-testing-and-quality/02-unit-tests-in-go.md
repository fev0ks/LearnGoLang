# Unit Tests In Go

Unit tests в Go должны проверять небольшие куски поведения быстро, детерминированно и без тяжелой инфраструктуры.

## Содержание

- [Когда unit tests особенно полезны](#когда-unit-tests-особенно-полезны)
- [Когда unit tests не дают достаточной уверенности](#когда-unit-tests-не-дают-достаточной-уверенности)
- [Стандартный стиль в Go](#стандартный-стиль-в-go)
- [Что делает unit test хорошим](#что-делает-unit-test-хорошим)
- [Что делает unit test плохим](#что-делает-unit-test-плохим)
- [Что тестировать в unit tests](#что-тестировать-в-unit-tests)
- [Table-driven tests](#table-driven-tests)
- [`cmp.Diff` и сравнение результатов](#cmpdiff-и-сравнение-результатов)
- [Тестирование ошибок](#тестирование-ошибок)
- [Тестирование времени и случайности](#тестирование-времени-и-случайности)
- [Тестирование HTTP без сети](#тестирование-http-без-сети)
- [Golden tests](#golden-tests)
- [Что обычно стоит тестировать первым](#что-обычно-стоит-тестировать-первым)
- [Команды](#команды)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)
- [Связанные темы](#связанные-темы)

## Когда unit tests особенно полезны

- чистая бизнес-логика;
- validation;
- pricing/rules/policies;
- mapping between DTO/domain/models;
- error handling branches;
- retry/backoff policy calculations;
- rate limiting math;
- parsing and formatting rules.

## Когда unit tests не дают достаточной уверенности

- SQL-запросы и транзакции;
- поведение Redis, Kafka, Mongo, Postgres drivers;
- HTTP middleware integration;
- JSON/protobuf compatibility;
- все, где реальная зависимость сама является частью риска.

В этих случаях unit tests полезны, но почти всегда недостаточны без integration layer.

## Стандартный стиль в Go

Самый частый и удобный формат:
- table-driven tests;
- subtests через `t.Run`;
- небольшие helpers;
- явные expected values;
- минимум магии.

Пример:

```go
func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "ru phone", input: "+7 (999) 111-22-33", want: "79991112233"},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePhone(tt.input)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
```

## Что делает unit test хорошим

- один понятный риск;
- быстрый запуск;
- отсутствие flaky behavior;
- минимум setup;
- failure message сразу объясняет, что сломалось.

## Что делает unit test плохим

- тест завязан на private implementation details;
- трудно понять intent;
- для запуска нужно поднять полсистемы;
- в тесте много mock expectation noise;
- любое небольшое изменение кода ломает десяток тестов без изменения поведения.

## Что тестировать в unit tests

### Happy path

Нужен, но недостаточен.

### Edge cases

Часто именно здесь тесты окупаются:
- пустые входы;
- нулевые значения;
- boundary conditions;
- duplicate values;
- invalid states;
- cancellation/timeouts;
- overflow/underflow-like сценарии.

### Error paths

Senior-level код часто ценнее тестировать именно на:
- propagation errors;
- wrapping;
- partial failure;
- retry stop conditions;
- idempotency behavior.

## Table-driven tests

Почему это стандартный стиль Go:
- компактно;
- легко расширять кейсы;
- хорошо читается в review;
- удобно прогонять много входных комбинаций.

Когда table-driven style не нужен:
- сценарий один и он очень специфичный;
- тест иначе становится менее читаемым, чем обычный линейный код.

## `cmp.Diff` и сравнение результатов

Для сложных структур полезно использовать `go-cmp`, а не писать вручную много `if`.

Идея:

```go
if diff := cmp.Diff(want, got); diff != "" {
	t.Fatalf("mismatch (-want +got):\n%s", diff)
}
```

Это особенно полезно для:
- вложенных структур;
- slices/maps;
- результатов трансформации данных.

## Тестирование ошибок

Если код использует `errors.Is` и `errors.As`, тесты тоже должны проверять это поведение.

Пример:

```go
if !errors.Is(err, ErrNotFound) {
	t.Fatalf("expected ErrNotFound, got %v", err)
}
```

Плохой паттерн:
- сравнивать только строку ошибки;
- ломается от harmless refactor.

## Тестирование времени и случайности

Чтобы unit tests были детерминированными:
- инжектируй `clock` или `now func()`;
- инжектируй `rand source`, если это важно;
- не используй реальные `time.Sleep`, если можно не использовать.

Плохой тест:

```go
time.Sleep(100 * time.Millisecond)
```

Лучше:
- fake clock;
- controllable timer abstraction;
- явно вызываемая функция advance/refill/retry.

## Тестирование HTTP без сети

Для handlers и middleware полезен `httptest`.

Пример:

```go
req := httptest.NewRequest(http.MethodGet, "/v1/users/42", nil)
rr := httptest.NewRecorder()

handler.ServeHTTP(rr, req)

if rr.Code != http.StatusOK {
	t.Fatalf("got %d", rr.Code)
}
```

Это часто still unit-ish тест:
- быстрый;
- без реальной сети;
- хорошо ловит handler logic.

## Golden tests

Подход полезен, когда результат большой и текстовый:
- JSON output;
- SQL/query rendering;
- code generation;
- templates.

Но golden tests опасны, если:
- файл обновляют механически без понимания;
- они скрывают реальный intent;
- diff трудно интерпретировать.

## Что обычно стоит тестировать первым

Для критичного куска логики:
1. Happy path.
2. Самый опасный edge case.
3. Main error path.
4. Regression test на уже найденный баг.

## Команды

```bash
go test ./...
go test ./... -run TestNormalizePhone
go test ./... -count=1
```

`-count=1` полезен, когда надо убрать влияние test cache.

## Что могут спросить на интервью

- когда unit test действительно полезен, а когда это ложная уверенность;
- почему table-driven tests так популярны в Go;
- как тестировать код с временем, retry и контекстом;
- почему сравнение строки ошибки часто плохая идея;
- чем `httptest` лучше ручных моков HTTP-слоя.

## Связанные темы

- [Automated Testing Strategy](./01-automated-testing-strategy.md)
- [Test Doubles And Test Design](./03-test-doubles-and-test-design.md)
