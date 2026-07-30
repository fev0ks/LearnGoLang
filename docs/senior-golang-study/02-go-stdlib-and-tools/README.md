# Go Stdlib And Tools

Сюда собирай заметки по стандартной библиотеке и официальным инструментам.

## Материалы

- [01. Sort And Slices](./01-sort-and-slices.md) — sort vs slices/cmp (Go 1.21+), компараторы, multi-key (cmp.Or), бинарный поиск (lower/upper bound), stable vs unstable, паттерны в алго-задачах
- [02. encoding/json](./02-encoding-json.md) — теги и omitempty/omitzero, числа как float64 в interface{}, Marshaler/RawMessage, Encoder/Decoder, строгий разбор, загадки
- [03. log/slog](./03-slog.md) — структурированное логирование, Handler/Logger, проброс trace_id/request_id/user_id из ctx через кастомный Handler, LogValuer, LevelVar, загадки
- [04. reflect](./04-reflect.md) — интроспекция типов, три закона рефлексии, Type vs Kind, settability, теги/поля/методы, DeepEqual, цена reflection (бенчмарк), когда заменять дженериками
- [05. io и bufio](./05-io-and-bufio.md) — Reader/Writer как композиция, контракт Read (n/err/EOF), io.EOF, io.Copy и быстрые пути, ReadAll/LimitReader, Tee/Multi/Pipe, bufio.Writer Flush, Scanner и лимит токена
- [06. Строковые пакеты](./06-strings-package.md) — `strings` (Split vs Fields, Cut, Trim-ловушка cutset≠префикс, Replace/NewReplacer, EqualFold, итераторы 1.24), `unicode/utf8` (RuneCount/Decode/Valid, RuneError-ловушка), соседи `bytes`/`unicode`
- [07. Форматирование (fmt)](./07-fmt-formatting.md) — глаголы %v/%+v/%#v/%T, по типам (%s/%q/%d/%x/%f/%c/%U), флаги ширины/точности, Stringer/Formatter, %w в ошибках, %!verb-диагностика

## Многие stdlib-темы разобраны в профильных секциях

Чтобы не дублировать, эти пакеты живут там, где они в контексте. Здесь — карта ссылок:

| Пакет / тема | Где разобрано |
|---|---|
| `context` | [concurrency/04-context-patterns](../01-go-core/concurrency-and-performance/04-context-patterns.md) |
| `sync`, `sync/atomic` (Mutex/RWMutex, Once, Pool, Map, atomic, singleflight) | [concurrency/03-sync-primitives](../01-go-core/concurrency-and-performance/03-sync-primitives.md) |
| Модель памяти, happens-before, race detector | [concurrency/01-memory-model](../01-go-core/concurrency-and-performance/01-memory-model.md) |
| `errors` (Is/As/Join, wrapping `%w`) | [01-go-core/05-error-handling](../01-go-core/05-error-handling.md) |
| `strings` — устройство типа (byte vs rune, header, Builder, unsafe) | [01-go-core/07-strings](../01-go-core/07-strings.md) |
| `strings`/`unicode/utf8`/`bytes`/`unicode` — справочник функций | [06-strings-package](./06-strings-package.md) |
| `time` (Timer/Ticker, утечки, `time.After` в select) | [runtime-scheduler/04-timers](../01-go-core/runtime-scheduler/04-timers.md) |
| `net/http` — сервер | [networking/protocols/03-http-server](../08-networking-and-api/protocols/03-http-server.md), [http-servers/01-stdlib-net-http](../03-go-libraries-and-ecosystem/http-servers/01-stdlib-net-http.md) |
| `net/http` — клиент, Transport, reuse | [networking/protocols/04-http-client](../08-networking-and-api/protocols/04-http-client.md) |
| Таймауты и deadlines | [reliability/01-timeouts-and-deadlines](../05-system-design/reliability-patterns/01-timeouts-and-deadlines.md) |
| Graceful shutdown | [patterns/08-graceful-shutdown](../04-architecture-and-patterns/patterns/08-graceful-shutdown.md) |
| `database/sql`, пул соединений | [go-database-libraries/02-standard-library-database-sql](../06-databases/go-database-libraries/02-standard-library-database-sql.md), [postgresql/09-connection-pooling](../06-databases/database-systems-catalog/postgresql/09-connection-pooling.md) |
| `testing`, `httptest`, fuzzing, race | [09-testing-and-quality](../09-testing-and-quality/README.md), [11-race-fuzz-and-benchmarks](../09-testing-and-quality/11-race-fuzz-and-benchmarks.md) |
| `runtime/pprof`, `runtime/trace`, benchmarks, `go tool pprof/trace` | [01-go-core/profiling](../01-go-core/profiling/README.md), [06-benchmarks](../01-go-core/profiling/06-benchmarks.md) |

Ещё не покрыто (кандидаты на отдельные файлы здесь):
- `regexp` — RE2 (без backtracking), `MustCompile`, флаги, типичные ошибки производительности;
- `encoding/binary`, `flag`, `embed` — нишевые, по необходимости.

## Подборка

- [Standard Library Packages](https://pkg.go.dev/std)
- [net/http](https://pkg.go.dev/net/http)
- [database/sql](https://pkg.go.dev/database/sql)
- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [Fuzzing](https://go.dev/doc/fuzz/)
- [Profile-guided optimization](https://go.dev/doc/pgo)
- [Package testing](https://pkg.go.dev/testing)
