# Go Version Differences

Сюда собирай изменения между релизами Go, которые реально влияют на production-код, tooling и ожидания на senior-интервью.

Этот модуль полезен для двух задач:
- быстро понимать, что меняется при апгрейде toolchain;
- уметь объяснить, какие изменения важны для runtime, CI, observability и backward compatibility.

Период:
- [Go 1.24](./go1.24.md) — релиз февраля 2025;
- [Go 1.25](./go1.25.md) — релиз августа 2025;
- [Go 1.26](./go1.26.md) — релиз февраля 2026.

## Как читать

- сначала смотри на language/runtime/tooling изменения;
- затем отмечай, что требует миграции, а что можно внедрять постепенно;
- отдельно фиксируй, какие изменения затрагивают контейнеры, perf, security и debugging.

Что особенно важно уметь проговаривать:
- какие изменения безопасны и почти прозрачны для приложения;
- где изменилось поведение runtime или компилятора;
- какие новые инструменты стоит внедрить в CI и локальную разработку;
- какие фичи ещё экспериментальные и не должны попадать в критичный production без проверки.

## Сравнительная таблица версий

| Категория | Go 1.24 (фев 2025) | Go 1.25 (авг 2025) | Go 1.26 (фев 2026) |
|---|---|---|---|
| **Язык** | Generic type aliases | Нет изменений | `new(expr)`, self-referential generic constraints |
| **Tooling** | `tool` directive в `go.mod`, `go build -json`, `GOCACHEPROG` GA | `go build -asan` leak detection, `ignore` directive в `go.mod`, `go doc -http` | `go fix` как modernizer platform, `go mod init` пишет N-1 версию, pprof flamegraph по умолчанию |
| **Runtime** | Swiss Tables для maps, -2–3% CPU | Container-aware `GOMAXPROCS`, Green Tea GC (эксперимент) | Green Tea GC по умолчанию, cgo -30% overhead, heap address randomization |
| **Observability** | — | `runtime/trace.FlightRecorder` | Goroutine leak profile (experimental) |
| **Stdlib** | `os.Root`, `testing.B.Loop`, `runtime.AddCleanup`, FIPS 140-3 | `testing/synctest` GA, `encoding/json/v2` (experiment) | `crypto/hpke`, `bytes.Buffer.Peek`, `runtime/secret` (experiment) |
| **Breaking / осторожно** | Swiss Tables: проверить `unsafe` map assumptions | Nil check bug fix: скрытые паники | `cmd/doc` удалён, cgo behavior changes, crypto random source |

## Какую версию использовать сейчас

**Go 1.26** — текущая production-ready версия (февраль 2026). Это разумная цель для новых сервисов и планового апгрейда.

Если сервис работает на **Go 1.24 или 1.25** — апгрейд малорискованный: основные изменения либо прозрачны, либо дают выигрыш производительности без миграции кода. Тем не менее, перед апгрейдом стоит проверить три зоны риска:

- **Nil check bug fix (1.25):** компилятор исправил ошибку, при которой nil-разыменование в редких случаях не паниковало. После апгрейда такой код начнёт паниковать корректно — скрытые баги выйдут наружу.
- **Container-aware `GOMAXPROCS` (1.25):** если сервис работает в Kubernetes с заданными CPU limits, `GOMAXPROCS` теперь выставляется автоматически по квоте контейнера, а не по числу ядер хоста. Поведение улучшается, но стоит убедиться, что явное выставление `GOMAXPROCS` в коде не конфликтует с новым поведением.
- **Green Tea GC по умолчанию (1.26):** для GC-heavy нагрузок (большие кучи, частые аллокации) рекомендуется провести нагрузочное тестирование после апгрейда. Green Tea GC снижает tail latency, но профиль памяти может немного измениться.

## Быстрый срез

`1.24`: generic type aliases, `tool` directive, Swiss Tables maps, `os.Root`, `testing.B.Loop`, `runtime.AddCleanup`.

`1.25`: container-aware `GOMAXPROCS`, experimental Green Tea GC, `runtime/trace.FlightRecorder`, `testing/synctest` GA, experimental `encoding/json/v2`, `ignore` directive.

`1.26`: `new(expr)` и self-referential generic constraints, `go fix` как платформа modernizers, Green Tea GC по умолчанию, goroutine leak profile, `crypto/hpke`, experimental `runtime/secret`.

## Подборка

- [Go Release History](https://go.dev/doc/devel/release)
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24)
- [Go 1.25 Release Notes](https://go.dev/doc/go1.25)
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26)

## Вопросы

- что из изменений в новой версии влияет на latency, memory и scheduler behavior;
- что нужно проверить перед апгрейдом Go в Kubernetes-кластере;
- какие новые возможности стоит добавить в тестовый стек команды;
- какие фичи уже production-ready, а какие пока экспериментальные;
- где после апгрейда возможны поведенческие регрессии, даже если код компилируется.
