# Patterns

Go-specific паттерны на уровне кода и архитектурные паттерны сервисов.

Главная идея: паттерн полезен только тогда, когда снижает стоимость изменения, тестирования или эксплуатации. Если он добавляет слои без явной причины — это не архитектура, а ceremony.

## Материалы

- [01. Go Code Patterns](./01-go-code-patterns.md) — small interfaces, constructor injection, functional options, middleware, adapter, decorator, strategy, repository, UoW
- [02. Architecture Patterns](./02-architecture-patterns.md) — layered, hexagonal, clean, DDD lite, CQRS, outbox, saga, idempotency, reconciliation, ACL, strangler fig
- [03. API Versioning](./03-api-versioning.md) — REST/gRPC versioning, backward compatibility, Protobuf rules, event schema, deprecation lifecycle
- [04. Background Workers](./04-background-workers.md) — worker pool, graceful shutdown, periodic jobs, distributed lease, idempotent workers, backpressure
- [05. DDD в Go](./05-ddd-in-go.md) — стратегический и тактический DDD: Entity, Value Object, Aggregate, Domain Events, Repository, Domain Service, Application Service с примерами кода
- [06. SOLID в Go](./06-solid-in-go.md) — SRP, OCP, LSP, ISP, DIP с Go-примерами: почему без классов и наследования принципы выглядят иначе
- [07. Проектирование REST API](./07-rest-api-design.md) — ресурсы vs действия, именование URL, HTTP-методы, path/query/body, типичные ошибки и как их избежать

## Как читать

1. `01` — Go-specific паттерны на уровне кода, основа для всего остального
2. `02` — архитектурные паттерны сервисов: когда и зачем
3. `03` — API versioning отдельно, часто спрашивают на интервью
4. `04` — background workers, тоже отдельная тема с нюансами Go
5. `05` — DDD в Go: когда оправдан и как реализовать тактические паттерны
6. `06` — SOLID в Go: те же принципы, но без классов — через интерфейсы и composition
7. `07` — REST API design: ресурсы, правила именования, типичные ошибки из реального опыта

## Что важно уметь объяснить

- зачем интерфейс объявляет потребитель, а не поставщик
- чем decorator отличается от adapter
- когда outbox необходим, а когда избыточен
- что такое level-triggered reconciliation и чем отличается от event-driven
- как сделать graceful shutdown воркера под нагрузкой
- backward compatibility Protobuf: что можно, что нельзя
- как distributed lease защищает от дублирования periodic jobs
- чем Entity отличается от Value Object и как это реализуется в Go
- что такое Aggregate и почему одна транзакция = один aggregate
- где в Go-структуре пакетов живут domain interfaces и кто их реализует
- когда DDD оправдан, а когда это ceremony
- чем REST отличается от RPC-style и почему глаголы в URL — проблема
- когда state transition выражать через PATCH, а когда через POST sub-resource
- почему POST для read-операций — плохая практика и как это исправить
- почему в Go интерфейс объявляет потребитель — это ISP + DIP одновременно
- чем OCP в Go отличается от OCP в Java (нет наследования → интерфейсы)
- как проверить что реализация соблюдает LSP — contract tests

## Interview-ready answer

Паттерны в Go я воспринимаю не как список классов из GoF, а как набор практик для управления зависимостями, изменениями и отказами. На уровне кода — small interfaces, constructor injection, functional options, middleware, adapter, decorator, strategy, repository. На уровне архитектуры — layered или hexagonal в зависимости от сложности домена, outbox для надёжного publish, saga для распределённых процессов, idempotency для retries, reconciliation для устойчивости к потере событий. Выбор всегда от проблемы: что меняется часто, где граница ответственности, где нужна удобная замена в тестах.
