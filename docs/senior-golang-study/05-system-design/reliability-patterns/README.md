# Reliability Patterns

Паттерны надёжности для highload сервисов: как сервис выживает под нагрузкой, изолирует отказы зависимостей и не разрушает систему каскадно.

## Материалы

- [01 Timeouts и Deadlines](./01-timeouts-and-deadlines.md) — deadline propagation, latency budget, контекст с таймаутом на каждый внешний вызов
- [02 Retries и Backoff](./02-retries-and-backoff.md) — exponential backoff + jitter, retry budget, retry amplification
- [03 Circuit Breaker](./03-circuit-breaker.md) — состояния, gobreaker, предотвращение cascade failure
- [04 Rate Limiting](./04-rate-limiting.md) — token bucket, sliding window, distributed RL через Redis
- [05 Backpressure и Load Shedding](./05-backpressure-and-shedding.md) — bounded queues, semaphore, 503 как сигнал, graceful degradation
- [06 Idempotency](./06-idempotency.md) — idempotency keys, at-least-once delivery, дедупликация в PostgreSQL
- [07 Bulkhead](./07-bulkhead.md) — изоляция пулов по зависимостям, semaphore per downstream

## Вопросы

- как deadline propagation спасает от latency amplification в цепочке сервисов
- почему retry без jitter создаёт thundering herd
- как circuit breaker отличается от простой проверки ошибок
- когда load shedding лучше чем backpressure
- как идемпотентный ключ защищает от дублей при сетевом сбое
- почему один общий goroutine pool для всех зависимостей опасен
