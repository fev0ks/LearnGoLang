# Architecture And Patterns

Backend-архитектура и инженерные trade-offs для senior Go разработчика.

Senior-фокус:
- когда архитектура помогает, а когда это просто ceremony
- как не превратить "чистую" архитектуру в избыточный boilerplate
- как принимать решения под ограничения команды, дедлайна и текущей нагрузки

## Подпакеты

### [Patterns](./patterns/README.md)

- [01. Go Code Patterns](./patterns/01-go-code-patterns.md) — small interfaces, constructor injection, functional options, middleware, adapter, decorator, strategy, repository
- [02. Architecture Patterns](./patterns/02-architecture-patterns.md) — layered, hexagonal, CQRS, outbox, saga, idempotency, reconciliation, ACL, strangler fig
- [03. API Versioning](./patterns/03-api-versioning.md) — REST/gRPC versioning, backward compatibility, Protobuf rules, deprecation
- [04. Background Workers](./patterns/04-background-workers.md) — worker pool, graceful shutdown, distributed lease, idempotent workers

### [Service Topologies](./service-topologies/README.md)

- [01. Monolith vs Modular Monolith vs Microservices](./service-topologies/01-monolith-vs-modular-monolith-vs-microservices.md) — сравнение, эволюция, decision guide
- [02. Typical Problems And Mitigations](./service-topologies/02-typical-problems-and-how-to-mitigate-them.md) — circuit breaker, retry, distributed tracing, anti-patterns
- [03. Go Project Layout](./service-topologies/03-go-project-layout.md) — структура папок для разных архитектур

## Вопросы

- когда modular monolith лучше микросервисов и как его правильно реализовать в Go
- какие паттерны в Go действительно помогают, а какие превращаются в ceremony
- где проходит граница между domain logic и transport/storage concerns
- как сделать graceful shutdown воркера который обрабатывает задачи из очереди
- что такое outbox и почему он не даёт exactly-once
- чем level-triggered reconciliation отличается от обычного event handler'а
- backward compatibility в Protobuf: что можно менять, что нельзя
- как enforcement модульных границ работает через `internal/` в Go
- распределённый монолит: симптомы и как из него выбраться

## Подборка

- [Google SRE Books](https://sre.google/books/)
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)
- [Azure Cloud Design Patterns](https://learn.microsoft.com/en-us/azure/architecture/patterns/)
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout) (с критикой — не эталон)
- [gRPC Documentation](https://grpc.io/docs/)
