# PostgreSQL: углублённый разбор

PostgreSQL — open-source реляционная СУБД, основной выбор для backend-сервисов: строгая реляционная модель без потери практической гибкости. Этот раздел покрывает темы уровня senior. Ниже — обзор (что, когда, сильные/слабые стороны) и карта материалов.

## Где используется

- транзакционные backend-системы;
- финансовые / заказные / платёжные данные;
- пользовательские данные, админ-панели, SaaS;
- системы со сложными SQL-запросами и строгими constraints/целостностью.

## Материалы

- [00 Основы SQL и синтаксис](./00-sql-basics-and-syntax.md) — реляционная модель, CRUD, constraints, нормализация; JOIN-типы, LATERAL, агрегация/FILTER, оконные функции, CTE/RECURSIVE, множества, подзапросы
- [01 MVCC и Vacuum](./01-mvcc-and-vacuum.md) — как работает MVCC, dead tuples, autovacuum, transaction ID wraparound
- [02 Индексы](./02-indexes.md) — B-tree, GIN, GiST, BRIN, operator classes, partial, expression, covering, bloat, стратегии
- [03 Query Planning](./03-query-planning.md) — планировщик, статистика, EXPLAIN (ANALYZE, BUFFERS), узлы плана, типы сканов, JOIN-стратегии
- [04 Транзакции и Блокировки](./04-transactions-and-locking.md) — isolation levels, аномалии, row locks, SELECT FOR UPDATE, SKIP LOCKED, deadlock, DDL locks
- [05 Партиционирование](./05-partitioning.md) — Range/List/Hash, partition pruning, индексы, управление партициями, прод-грабли
- [06 Репликация](./06-replication.md) — WAL (checkpoint, FPI, PITR), streaming и logical replication, sync/async, Patroni
- [07 JSONB и Массивы](./07-jsonb-and-arrays.md) — JSONB операторы, GIN индексы, arrays, полнотекстовый поиск, pg_trgm
- [08 Performance Tuning](./08-performance-tuning.md) — shared_buffers, work_mem, pg_stat_statements, slow query log
- [09 Connection Pooling](./09-connection-pooling.md) — PgBouncer режимы, pgxpool, расчёт числа соединений
- [10 Мониторинг и Диагностика](./10-monitoring-and-diagnostics.md) — pg_stat_* представления, bloat, production checklist
- [11 Паттерны в Go](./11-go-patterns.md) — pgx v5, транзакции, batch, COPY, обработка ошибок
- [12 Шардирование](./12-sharding.md) — партиционирование vs шардирование, application-level sharding, Citus, FDW, resharding
- [13 Пагинация](./13-pagination.md) — offset vs keyset (cursor), tie-breaker, индекс, проблема COUNT, динамическая сортировка, N+1
- [14 Transactional Outbox и идемпотентность](./14-outbox-and-idempotency.md) — dual-write, дизайн таблицы outbox, relay-воркер (SKIP LOCKED), at-least-once, дедупликация, double-pay, CDC
- [Хайлоад-сценарии](./highload-scenarios/README.md) — write-heavy практики: массовая вставка, bulk UPDATE/DELETE, upsert под нагрузкой, горячие строки и счётчики, онлайн-миграция колонки, 15 кейсов zero-downtime изменений схемы
- [SQL-задачи](./sql-tasks/README.md) — задачи на SQL с разбором (LeetCode по сложности + кастомные: MVCC/локи/дедлоки, дубли, latest-per-group)

## Сильные стороны

- ACID-транзакции; constraints, foreign keys, CHECK;
- мощный SQL: window functions, CTE, lateral join;
- богатый набор индексов (B-tree, partial, GIN, GiST, BRIN, expression);
- JSONB для semi-structured данных;
- расширения (pgvector, PostGIS, pg_cron);
- зрелая экосистема и баланс строгой модели с гибкостью.

## Слабые стороны

- горизонтальное масштабирование записи сложнее, чем в distributed NoSQL;
- шардирование требует отдельного дизайна (Citus, application-level — см. [12-sharding.md](./12-sharding.md));
- плохие запросы легко убивают производительность;
- нужно понимать indexes, locks, transactions, vacuum, connection pooling — иначе грабли.

## Когда выбирать

- нужен надёжный default для backend;
- важны транзакции и целостность данных;
- есть сложные query patterns, доменная модель реляционная;
- хочется не потерять гибкость на старте.

## Когда не выбирать

- нужен extreme write scale на множество нод;
- всё строго key-value и нужна serverless/cloud-native модель;
- нагрузка — в основном аналитические сканы по огромным объёмам (тогда ClickHouse и подобные).

## Типичные ошибки

- использовать PostgreSQL как бездонную очередь (лучше Redis Streams или Kafka; если очередь всё же в PG — паттерн `SELECT ... FOR UPDATE SKIP LOCKED`, см. [04-transactions-and-locking.md](./04-transactions-and-locking.md));
- ставить индексы по схеме, а не по реальным запросам;
- держать долгие транзакции — bloat, lock contention, vacuum lag;
- делать unbounded `SELECT *` без `LIMIT` в production;
- не мониторить `pg_stat_activity`, `pg_stat_user_tables`, slow query log;
- работать без пула или держать слишком много прямых соединений;
- `OFFSET` для пагинации больших таблиц — деградирует, нужна keyset ([13-pagination.md](./13-pagination.md)).

## Interview-ready answer

**1. Почему PostgreSQL как default для backend?**

- ACID-транзакции, constraints/FK/CHECK, богатые индексы и мощный SQL (window, CTE, lateral), JSONB и расширения — закрывает большинство backend-задач без потери гибкости.

**2. Что senior обязан понимать про MVCC?**

- Каждый UPDATE создаёт новую версию строки, старые становятся dead tuples и зачищаются vacuum; длинная транзакция держит горизонт и блокирует зачистку → table bloat.

**3. Какие isolation levels и когда?**

- READ COMMITTED (default, снапшот на statement), REPEATABLE READ для согласованных отчётов, SERIALIZABLE (SSI, retry на `40001`) для сложных инвариантов.

**4. Какой индекс под какую задачу?**

- B-tree для equality/range/sort, GIN для JSONB/массивов/FTS, GiST для геометрии/диапазонов, BRIN для append-only, partial — для выборки по условию.

**5. Зачем connection pooling?**

- PostgreSQL — process-per-connection (~5–10 MB на соединение); тысячи прямых соединений съедают память и CPU, поэтому pgxpool и/или PgBouncer в transaction mode.

**6. Где PostgreSQL слаб?**

- Горизонтальное масштабирование записи без шардирования; для тяжёлых аналитических сканов по огромным объёмам лучше колоночные СУБД (ClickHouse).

## Официальная документация

- [PostgreSQL Docs](https://www.postgresql.org/docs/current/)
- [pgx GitHub](https://github.com/jackc/pgx)
- [PgBouncer Docs](https://www.pgbouncer.org/config.html)
- [Patroni Docs](https://patroni.readthedocs.io/)
