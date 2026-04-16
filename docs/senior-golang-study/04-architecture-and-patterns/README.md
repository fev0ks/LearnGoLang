# Architecture And Patterns

Сюда выноси backend-архитектуру и инженерные trade-offs.

Темы:
- Go-specific patterns: small interfaces, functional options, middleware, adapter, decorator;
- layered, hexagonal, clean architecture;
- DDD lite vs pragmatic service design;
- monolith, modular monolith, microservices;
- idempotency, outbox, saga, retries;
- level-triggered reconciliation и control loops;
- configuration boundaries;
- dependency inversion в Go без переусложнения;
- error boundaries и mapping domain errors;
- background workers и job orchestration;
- versioning API и обратная совместимость.

Senior-фокус:
- когда архитектура помогает, а когда это просто ceremony;
- как не превратить "чистую" архитектуру в избыточный boilerplate;
- как принимать решения под ограничения команды, дедлайна и текущей нагрузки.

Подпакеты:
- [Patterns](./patterns/README.md)
- [Service Topologies](./service-topologies/README.md)

## Подборка

- [Google SRE Books](https://sre.google/books/)
- [Building Secure and Reliable Systems](https://sre.google/resources/practices-and-processes/building-secure-reliable-systems/)
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)
- [Azure Cloud Design Patterns](https://learn.microsoft.com/en-us/azure/architecture/patterns/)
- [gRPC Documentation](https://grpc.io/docs/)

## Вопросы

- когда modular monolith лучше микросервисов;
- какие паттерны в Go действительно помогают, а какие превращаются в ceremony;
- где проходит граница между domain logic и transport/storage concerns;
- как внедрить outbox pattern без избыточной сложности;
- что делать, если "чистая архитектура" замедляет поставку фич;
- как организовать idempotency в API и фоновых воркерах;
- как ты объяснишь выбор архитектуры через стоимость изменений через год.
