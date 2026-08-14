# Системные примитивы

Раздел тренирует не столько написание структуры данных, сколько проектирование
контракта при отказах: кто владеет ресурсом, что происходит при timeout, как
компонент завершается и какие гарантии сохраняются при конкурентном доступе.

## Материалы

1. [Connection Pool](./01-connection-pool.md) — лимит открытых ресурсов,
   ожидание с `context`, lease и корректный `Close`.
2. [Retry с Backoff](./02-retry-with-backoff.md) — bounded attempts,
   exponential backoff, Full Jitter и `Retry-After`.
3. [Circuit Breaker](./03-circuit-breaker.md) — состояния
   `closed/open/half-open`, единственный probe и классификация ошибок.
4. [Distributed Lock](./04-distributed-lock.md) — Redis lease, безопасный
   release, потеря владения и fencing tokens.
5. [Idempotency Key Handler](./05-idempotency-handler.md) — атомарный claim,
   replay результата и связь с business transaction.
6. [Балансировщик по наименьшей загрузке](./06-least-loaded-balancer.md) —
   атомарный выбор экземпляра, passive health check, min-heap против P2C и
   поведение под массовым timeout.

Рекомендуемый порядок — по нумерации. Retry, circuit breaker и idempotency
особенно важно рассматривать вместе: повтор без общего time budget и защиты от
повторного side effect легко ухудшает исходный сбой.

---

## Как выбирать примитив

| Проблема | Основной примитив | Чего он не гарантирует |
|---|---|---|
| дорого создавать resource | pool | здоровье зависимости |
| transient failure | retry | отсутствие duplicate side effect |
| зависимость массово падает | circuit breaker | ограничение локальной concurrency |
| несколько процессов претендуют на работу | distributed lock | исключительность после истечения lease |
| клиент повторяет mutation | idempotency key | exactly-once для внешнего side effect |
| пул одинаковых экземпляров, часть деградирует | least-loaded balancer + outlier detection | защиту от общей для пула причины сбоя |

Примитивы не взаимозаменяемы. Например, circuit breaker не ограничивает число
медленных вызовов в `closed`, а distributed lock без fencing token не может
остановить прежнего владельца после истечения TTL.

---

## Общие инварианты

- **Bounded resources —** ограничены attempts, очередь ожидания, открытые
  соединения, размер cached response и длительность lease.
- **Context —** каждый блокирующий wait, timer и внешний вызов имеет путь
  отмены. Наличие `context.Context` только в сигнатуре недостаточно.
- **Lifecycle —** `Close` или `Release` имеет явную семантику, повторный вызов
  определён, фоновая goroutine завершается.
- **Ownership —** ресурс возвращает только текущий владелец; потеря lease
  становится наблюдаемым событием, а не сообщением в логе.
- **Time budget —** timeout одной попытки меньше общего deadline операции;
  retry не создаёт неограниченный хвост latency.
- **Observability —** метрики различают отказ операции, rejection примитивом,
  ожидание, retry и потерю владения.

---

## Как проверять решение

Одних happy-path тестов недостаточно. Для каждого примитива нужны тесты на:

- конкурентный доступ под `go test -race`;
- отмену во время блокировки или timer;
- boundary вокруг TTL, timeout и смены состояния с управляемыми часами;
- повторный `Release`/`Close`;
- ошибку factory, storage или callback;
- отсутствие утечки goroutine и потерянного ресурса.

`time.Sleep` в тесте перехода состояния обычно делает тест flaky. Лучше
инъецировать clock, sleeper или синхронизироваться каналом.

---

## Связанные материалы

- [Reliability Patterns](../../../05-system-design/reliability-patterns/)
- [Connection Pooling](../../../06-databases/database-systems-catalog/postgresql/09-connection-pooling.md)
- [Bulkhead](../../../05-system-design/reliability-patterns/07-bulkhead.md)
- [Saga и Outbox](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md)
