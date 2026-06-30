# PostgreSQL: хайлоад-сценарии

Companion к разделу [postgresql](../README.md). Конкретные production-практики write-heavy нагрузки: «проблема → паттерн → Go-код → подводные камни». Механика, на которую опираются сценарии (MVCC, индексы, партиционирование, COPY), разобрана в профильных файлах — здесь только как это применяют под нагрузкой.

## Сценарии

- [01 Массовая вставка](./01-bulk-insert.md) — COPY vs multi-row INSERT vs batch, `UNLOGGED`, drop/rebuild индексов, загрузка в партицию + ATTACH, идемпотентный перезапуск.
- [02 Bulk UPDATE/DELETE без bloat и долгих локов](./02-bulk-update-delete.md) — батчи по ключу/`ctid`, throttle, partition DROP вместо DELETE.
- [03 Upsert под нагрузкой](./03-upsert-at-scale.md) — `ON CONFLICT`, дедупликация, батч-upsert, конкуренция за горячие строки.
- [04 Горячие строки и счётчики](./04-hot-rows-and-counters.md) — lock contention на одной строке, sharded counters, INCR+flush.

## Смежные практики (уже в других файлах)

Чтобы не дублировать — эти хайлоад-паттерны разобраны в других местах раздела:

- **Очередь задач на `SKIP LOCKED`** → [04-transactions-and-locking.md](../04-transactions-and-locking.md), раздел «SKIP LOCKED — паттерн очереди».
- **Keyset-пагинация** (вместо `OFFSET`) → [relational-databases-and-sql/04-pagination-and-query-patterns.md](../../../relational-databases-and-sql/04-pagination-and-query-patterns.md).
- **Outbox / idempotency** при высокой записи → [relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md](../../../relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md).
- **Connection pooling под нагрузкой** → [09-connection-pooling.md](../09-connection-pooling.md).
- **Кеш перед БД (cache-aside, hot key)** → [08a-redis-real-scenarios.md](../../08a-redis-real-scenarios.md).
- **Общие хайлоад-паттерны** (walls по RPS, async-first, backpressure) → [05-system-design/highload-design-patterns.md](../../../../05-system-design/highload-design-patterns.md).
