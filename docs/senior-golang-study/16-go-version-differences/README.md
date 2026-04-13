# Go Version Differences

Сюда собирай изменения между релизами Go, которые реально влияют на production-код, tooling и ожидания на senior-интервью.

Этот модуль полезен для двух задач:
- быстро понимать, что меняется при апгрейде toolchain;
- уметь объяснить, какие изменения важны для runtime, CI, observability и backward compatibility.

Период:
- [Go 1.24](./go1.24.md) - релиз февраля 2025;
- [Go 1.25](./go1.25.md) - релиз августа 2025;
- [Go 1.26](./go1.26.md) - релиз февраля 2026.

Как читать:
- сначала смотри на language/runtime/tooling изменения;
- затем отмечай, что требует миграции, а что можно внедрять постепенно;
- отдельно фиксируй, какие изменения затрагивают контейнеры, perf, security и debugging.

Что особенно важно уметь проговаривать:
- какие изменения безопасны и почти прозрачны для приложения;
- где изменилось поведение runtime или компилятора;
- какие новые инструменты стоит внедрить в CI и локальную разработку;
- какие фичи еще экспериментальные и не должны попадать в критичный production без проверки.

## Быстрый срез

`1.24`:
- generic type aliases;
- `tool` directive в `go.mod`;
- Swiss Table maps и ускорение runtime;
- `os.Root`, `testing.B.Loop`, `runtime.AddCleanup`.

`1.25`:
- container-aware `GOMAXPROCS`;
- experimental Green Tea GC;
- `runtime/trace.FlightRecorder`;
- `testing/synctest`, experimental `encoding/json/v2`;
- `ignore` directive в `go.mod`.

`1.26`:
- `new(expr)` и self-referential generic constraints;
- новый `go fix` как платформа modernizers;
- Green Tea GC включен по умолчанию;
- leak-профиль для горутин;
- `crypto/hpke`, experimental `simd/archsimd`, `runtime/secret`.

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
