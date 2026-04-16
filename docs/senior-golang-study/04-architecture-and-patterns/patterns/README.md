# Patterns

Этот подпакет про паттерны, которые чаще всего встречаются в Go backend-разработке и на senior-собеседованиях.

Главная идея: паттерн полезен только тогда, когда снижает стоимость изменения, тестирования или эксплуатации. Если он добавляет слои без явной причины, это не архитектура, а ceremony.

Материалы:
- [01 Go Code Patterns](./01-go-code-patterns.md)
- [02 Architecture Patterns](./02-architecture-patterns.md)

Как читать:
- сначала понять Go-specific паттерны на уровне кода;
- потом перейти к архитектурным паттернам сервисов;
- после этого связать тему с [Service Topologies](../service-topologies/README.md), потому что выбор паттерна зависит от формы системы: монолит, modular monolith или микросервисы.

Что важно уметь объяснить:
- зачем нужен паттерн в конкретном контексте;
- какой trade-off он добавляет;
- где граница между полезной абстракцией и лишним boilerplate;
- как это будет тестироваться и поддерживаться через год.

## Interview-ready answer

Паттерны в Go я воспринимаю не как список классов из GoF, а как набор практик для управления зависимостями, изменениями и отказами. На уровне кода это small interfaces, constructor injection, functional options, middleware, adapter, decorator и strategy. На уровне архитектуры это layered или hexagonal architecture, modular monolith, outbox, saga, idempotency, CQRS, level-triggered reconciliation и anti-corruption layer. Я выбираю паттерн не по названию, а по проблеме: что меняется часто, где граница ответственности, где нужна удобная замена зависимости в тестах, где есть риск дублирования или distributed failure.
