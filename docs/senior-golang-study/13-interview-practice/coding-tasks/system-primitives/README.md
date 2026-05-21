# System Primitives Tasks

Задачи на реализацию **production primitives** — стандартных блоков надёжных систем. На собеседовании их часто просят написать "по-простому", без полного production API, чтобы оценить понимание ключевых концепций.

## Задачи

1. [Connection Pool](./01-connection-pool.md) — переиспользование connections, lifecycle, health checks
2. [Retry с Backoff](./02-retry-with-backoff.md) — exponential backoff + jitter, идемпотентность, retryable errors
3. [Circuit Breaker](./03-circuit-breaker.md) — closed/open/half-open, sliding window, recovery
4. [Distributed Lock](./04-distributed-lock.md) — Redis-based, lease renewal, Redlock, fencing tokens
5. [Idempotency Key Handler](./05-idempotency-handler.md) — middleware для idempotent endpoint'ов

## Когда что просят

| Задача | Когда спрашивают |
|---|---|
| Connection pool | "Что делает database/sql под капотом?", "Реализуй pool ресурсов" |
| Retry | "Что делать при transient failure?", "Сделай надёжный HTTP client" |
| Circuit breaker | "Защити сервис от cascade failure", "Микросервисная resilience" |
| Distributed lock | "Только один сервис должен делать X", "Leader election" |
| Idempotency | "Как сделать чтобы повторный POST не дублировал заказ?" |

## Общие принципы

### Production primitives — это про **сценарии отказа**

Все эти задачи решают одну фундаментальную проблему: **внешние зависимости иногда падают**. Сервис должен:
- Не делать новых вызовов когда зависимость down (circuit breaker)
- Повторять transient ошибки правильно (retry с backoff)
- Не плодить новые соединения когда есть свободные (pool)
- Координироваться между instance'ами (distributed lock)
- Не дублировать действия при retry (idempotency)

### Context — везде

Каждая операция принимает `context.Context` для отмены/timeout. Это не опционально — это must.

### Метрики — обязательно

Production primitive без метрик — слепое пятно. Минимум:
- counter успехов/ошибок
- histogram latency
- gauge активных операций / open circuits

## Связки

- [Reliability Patterns](../../../05-system-design/reliability-patterns/) — теория retry, circuit breaker, idempotency
- [Connection Pooling](../../../06-databases/database-systems-catalog/postgresql/09-connection-pooling.md) — глубоко про БД pool
- [Saga и Outbox](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md) — distributed transactions
