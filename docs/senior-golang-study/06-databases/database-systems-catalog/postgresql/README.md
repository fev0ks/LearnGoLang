# PostgreSQL: углублённый разбор

PostgreSQL — open-source relational database, основной выбор для backend-сервисов. Этот раздел покрывает темы уровня senior.

## Материалы

- [00 Обзор](./00-overview.md) — где используется, strengths/weaknesses, когда выбирать
- [01 MVCC и Vacuum](./01-mvcc-and-vacuum.md) — как работает MVCC, dead tuples, autovacuum, transaction ID wraparound
- [02 Индексы](./02-indexes.md) — B-tree, GIN, GiST, BRIN, partial, expression, covering, bloat, стратегии
- [03 Query Planning](./03-query-planning.md) — планировщик, статистика, EXPLAIN ANALYZE, типы сканов, JOIN стратегии
- [04 Транзакции и Блокировки](./04-transactions-and-locking.md) — isolation levels, row locks, SELECT FOR UPDATE, SKIP LOCKED, deadlock, DDL locks
- [05 Партиционирование](./05-partitioning.md) — Range/List/Hash, partition pruning, индексы, управление партициями
- [06 Репликация](./06-replication.md) — WAL, streaming replication, logical replication, sync/async, Patroni
- [07 JSONB и Массивы](./07-jsonb-and-arrays.md) — JSONB операторы, GIN индексы, arrays, полнотекстовый поиск, pg_trgm
- [08 Performance Tuning](./08-performance-tuning.md) — shared_buffers, work_mem, pg_stat_statements, slow query log
- [09 Connection Pooling](./09-connection-pooling.md) — PgBouncer режимы, pgxpool, расчёт числа соединений
- [10 Мониторинг и Диагностика](./10-monitoring-and-diagnostics.md) — pg_stat_* представления, bloat, production checklist
- [11 Паттерны в Go](./11-go-patterns.md) — pgx v5, транзакции, batch, COPY, обработка ошибок

## Официальная документация

- [PostgreSQL Docs](https://www.postgresql.org/docs/current/)
- [pgx GitHub](https://github.com/jackc/pgx)
- [PgBouncer Docs](https://www.pgbouncer.org/config.html)
- [Patroni Docs](https://patroni.readthedocs.io/)
