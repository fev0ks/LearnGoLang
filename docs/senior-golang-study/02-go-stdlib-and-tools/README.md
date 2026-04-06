# Go Stdlib And Tools

Сюда собирай заметки по стандартной библиотеке и официальным инструментам.

Что покрыть:
- `net/http`, middleware, transports, connection reuse;
- `context`, `database/sql`, `sync`, `sync/atomic`;
- `encoding/json`, ограничения и типовые ошибки;
- `expvar`, `pprof`, `runtime`, `runtime/trace`;
- `testing`, `httptest`, benchmark и fuzzing;
- `log/slog`, structured logging;
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
