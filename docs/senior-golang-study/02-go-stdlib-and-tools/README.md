# Go Stdlib And Tools

Сюда собирай заметки по стандартной библиотеке и официальным инструментам.

## Материалы

- [01. Sort And Slices](./01-sort-and-slices.md) — sort vs slices/cmp (Go 1.21+), компараторы, multi-key (cmp.Or), бинарный поиск (lower/upper bound), stable vs unstable, паттерны в алго-задачах
- [02. encoding/json](./02-encoding-json.md) — теги и omitempty/omitzero, числа как float64 в interface{}, Marshaler/RawMessage, Encoder/Decoder, строгий разбор, загадки
- [03. log/slog](./03-slog.md) — структурированное логирование, Handler/Logger, проброс trace_id/request_id/user_id из ctx через кастомный Handler, LogValuer, LevelVar, загадки
- [04. reflect](./04-reflect.md) — интроспекция типов, три закона рефлексии, Type vs Kind, settability, теги/поля/методы, DeepEqual, цена reflection (бенчмарк), когда заменять дженериками

> `context` подробно разобран в [01-go-core/concurrency-and-performance/05-context-patterns](../01-go-core/concurrency-and-performance/05-context-patterns.md).

Что покрыть:
- `net/http`, middleware, transports, connection reuse;
- `database/sql`, `sync`, `sync/atomic`;
- `expvar`, `pprof`, `runtime`, `runtime/trace`;
- `testing`, `httptest`, benchmark и fuzzing;
- `go test`, `go vet`, `go tool pprof`, `go tool trace`, `go generate`.

Полезные сравнения:
- `http.Client` reuse vs создание клиента на каждый запрос;
- `sync.Mutex` vs `sync.RWMutex`;
- channels vs mutexes;
- `database/sql` напрямую vs ORM/query builder поверх него.

## Подборка

- [Standard Library Packages](https://pkg.go.dev/std)
- [net/http](https://pkg.go.dev/net/http)
- [database/sql](https://pkg.go.dev/database/sql)
- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [Fuzzing](https://go.dev/doc/fuzz/)
- [Profile-guided optimization](https://go.dev/doc/pgo)
- [Package testing](https://pkg.go.dev/testing)

## Вопросы

- почему `http.Client` обычно должен жить долго;
- когда `RWMutex` дает выигрыш, а когда делает хуже;
- чем `context.Context` отличается от контейнера для любых значений;
- какие типовые ошибки совершают при работе с `database/sql`;
- что ты делаешь первым при unexplained latency spike в Go-сервисе;
- когда benchmark в Go врет и как это заметить.
