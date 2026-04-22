# Service Topologies

Когда нужен монолит, когда modular monolith, а когда микросервисы — и как они выглядят на практике в Go.

## Материалы

- [01. Monolith vs Modular Monolith vs Microservices](./01-monolith-vs-modular-monolith-vs-microservices.md) — сравнительная таблица, эволюционный путь, decision guide, когда микросервисы — ошибка
- [02. Typical Problems And How To Mitigate Them](./02-typical-problems-and-how-to-mitigate-them.md) — circuit breaker, distributed tracing, retry strategy, distributed monolith anti-pattern
- [03. Go Project Layout](./03-go-project-layout.md) — структура папок для layered, hexagonal, modular monolith, микросервиса в монорепо
- [04. Modular Monolith In Depth](./04-modular-monolith-in-depth.md) — module.go паттерн, cross-module коммуникация, PostgreSQL schemas, enforcement, эволюция в микросервис

## Что важно понять

- "лучшей" архитектуры без контекста нет
- правильный выбор зависит от команды, стадии продукта, нагрузки и скорости изменений
- главная ошибка — не в том что выбрал монолит или микросервисы, а в том что не понимаешь цену этого выбора
- распределённый монолит — худшее из двух миров: все минусы обоих подходов

## Что важно уметь объяснить

- чем modular monolith отличается от "большого монолита с папками"
- что такое distributed monolith и почему это хуже обычного монолита
- как структура папок отражает архитектуру (не наоборот)
- circuit breaker: состояния, когда срабатывает, fallback стратегия
- как enforcement модульных границ работает через `internal/` в Go
