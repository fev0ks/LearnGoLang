# Race, Fuzz And Benchmarks

Это не "дополнительные игрушки", а отдельные виды инженерной проверки, которые ловят классы проблем, плохо видимые обычными unit tests.

## Race tests

Race detector ищет data races во время выполнения тестов.

Команда:

```bash
go test -race ./...
```

Что он хорошо ловит:
- конкурентные чтения/записи без синхронизации;
- ошибки вокруг shared maps;
- небезопасную публикацию состояния;
- часть проблем с каналами, mutex и lifecycle goroutines.

Чего он не гарантирует:
- не находит баг, если путь не был исполнен;
- не ловит все logical concurrency bugs;
- не заменяет reasoning по memory model.

Когда особенно полезен:
- worker pools;
- caches;
- request deduplication;
- background goroutines;
- graceful shutdown;
- code с `sync.Map`, `atomic`, `mutex`, channels.

## Fuzz tests

Fuzzing полезен там, где входов слишком много, чтобы руками перечислить все кейсы.

Команда:

```bash
go test -fuzz=Fuzz -run=^$
```

Что хорошо подходит:
- parsers;
- validators;
- URL/token/ID normalization;
- JSON/protobuf transforms;
- custom protocol inputs;
- input sanitization.

Пример идеи:

```go
func FuzzNormalizePhone(f *testing.F) {
	f.Add("+7 (999) 111-22-33")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = NormalizePhone(input)
	})
}
```

Что fuzzing особенно хорошо ловит:
- panic;
- unexpected error paths;
- pathological input combinations;
- edge cases, которые команда не догадалась перечислить вручную.

## Benchmarks

Benchmark нужен, когда важна производительность, allocation profile или сравнение альтернатив.

Команды:

```bash
go test -bench=. ./...
go test -bench=. -benchmem ./...
```

Что обычно меряют:
- `ns/op`;
- `B/op`;
- `allocs/op`.

Что benchmark хорошо показывает:
- cost hot path;
- эффект изменения алгоритма;
- allocation reductions;
- regressions после рефакторинга.

## Когда benchmark врет

- тестирует toy scenario вместо реального hot path;
- компилятор оптимизирует полезную работу;
- нет representative input sizes;
- не учтены contention, IO, GC, production concurrency.

Поэтому benchmark всегда нужно читать вместе с:
- pprof;
- production metrics;
- realistic workload assumptions.

## Practical usage

`Race`:
- включай в CI хотя бы регулярно;
- особенно перед merge больших concurrent changes.

`Fuzz`:
- полезен для библиотек, parsing и security-sensitive inputs;
- можно гонять nightly или локально при изменении parser logic.

`Benchmarks`:
- нужны перед и после performance-tuning;
- полезны для сравнения вариантов API и data structures.

## Что могут спросить на интервью

- почему `go test -race` полезен, но недостаточен;
- какие задачи особенно хорошо покрываются fuzzing;
- когда benchmark нужен, а когда это premature optimization;
- почему `allocs/op` иногда важнее, чем `ns/op`;
- как понять, что benchmark действительно репрезентативен.

## Связанные темы

- [Automated Testing Strategy](./automated-testing-strategy.md)
- [Unit Tests In Go](./unit-tests-in-go.md)
