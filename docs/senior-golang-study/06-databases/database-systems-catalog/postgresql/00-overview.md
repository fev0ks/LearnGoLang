# PostgreSQL

PostgreSQL — open-source реляционная СУБД, которую часто выбирают как основной storage для backend-сервисов. Этот файл — обзорный: что это, в чём силён и слаб, когда брать. Глубокие темы вынесены в отдельные файлы раздела, здесь — карта и кросс-ссылки.

## Содержание

- [Где используется](#где-используется)
- [Карта раздела](#карта-раздела)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- транзакционные backend-системы;
- финансовые / заказные / платёжные данные;
- пользовательские данные;
- админ-панели и SaaS-продукты;
- системы со сложными SQL-запросами;
- системы, которым нужны строгие constraints и целостность данных.

## Карта раздела

Обзорно — что важно знать на senior-уровне, и где это разобрано детально:

| Тема | Суть в одном абзаце | Детально |
|------|---------------------|----------|
| **MVCC и Vacuum** | Каждый UPDATE создаёт новую версию строки, старые становятся dead tuples и зачищаются VACUUM. Длинная транзакция блокирует зачистку → table bloat. | [01-mvcc-and-vacuum.md](./01-mvcc-and-vacuum.md) |
| **Индексы** | B-tree по умолчанию (equality/range/sort); GIN для JSONB/массивов/FTS; GiST для геометрии/диапазонов; BRIN для append-only; partial/expression/covering. | [02-indexes.md](./02-indexes.md) |
| **Query planning** | Cost-based планировщик выбирает план по статистике; диагностика через `EXPLAIN (ANALYZE, BUFFERS)`. | [03-query-planning.md](./03-query-planning.md) |
| **Транзакции и блокировки** | Isolation levels (READ COMMITTED → SERIALIZABLE/SSI), row locks, `SKIP LOCKED`, deadlock, безопасный DDL. | [04-transactions-and-locking.md](./04-transactions-and-locking.md) |
| **Партиционирование** | Range/List/Hash, partition pruning, мгновенный DROP старых партиций. | [05-partitioning.md](./05-partitioning.md) |
| **Репликация** | WAL → streaming (физическая) и logical; sync/async; Patroni для HA. | [06-replication.md](./06-replication.md) |
| **JSONB и массивы** | Бинарный JSON под GIN, SQL/JSON path, FTS, pg_trgm. | [07-jsonb-and-arrays.md](./07-jsonb-and-arrays.md) |
| **Performance tuning** | `shared_buffers`, `work_mem`, `random_page_cost`, pg_stat_statements. | [08-performance-tuning.md](./08-performance-tuning.md) |
| **Connection pooling** | Process-per-connection дорог → PgBouncer (transaction mode) / pgxpool. | [09-connection-pooling.md](./09-connection-pooling.md) |
| **Мониторинг** | `pg_stat_*`, `pg_stat_io`, диагностика bloat и блокировок, чеклист. | [10-monitoring-and-diagnostics.md](./10-monitoring-and-diagnostics.md) |
| **Паттерны в Go** | pgx v5, транзакции, batch, COPY, обработка ошибок PG. | [11-go-patterns.md](./11-go-patterns.md) |
| **Шардирование** | Партиционирование vs шардирование, application-level, Citus, resharding. | [12-sharding.md](./12-sharding.md) |

## Сильные стороны

- ACID-транзакции;
- constraints, foreign keys, CHECK;
- мощный SQL: window functions, CTE, lateral join;
- богатый набор индексов (B-tree, partial, GIN, GiST, BRIN, expression);
- JSONB для semi-structured данных;
- расширения (pgvector, PostGIS, pg_cron);
- зрелая экосистема;
- баланс между строгой реляционной моделью и практической гибкостью.

## Слабые стороны

- горизонтальное масштабирование записи сложнее, чем в distributed NoSQL;
- шардирование требует отдельного дизайна (Citus, application-level — см. [12-sharding.md](./12-sharding.md));
- плохие запросы легко убивают производительность;
- нужно понимать indexes, locks, transactions, vacuum, connection pooling — иначе грабли.

## Когда выбирать

- нужен надёжный default для backend;
- важны транзакции и целостность данных;
- есть сложные query patterns;
- доменная модель реляционная;
- хочется не потерять гибкость на старте.

## Когда не выбирать

- нужен extreme write scale на множество нод;
- все запросы строго key-value и нужна serverless/cloud-native модель;
- нагрузка в основном аналитические сканы по огромным объёмам (тогда ClickHouse и подобные).

## Типичные ошибки

- использовать PostgreSQL как бездонную очередь (лучше Redis Streams или Kafka; если очередь всё же в PG — паттерн `SELECT ... FOR UPDATE SKIP LOCKED`, см. [04-transactions-and-locking.md](./04-transactions-and-locking.md));
- ставить индексы по схеме, а не по реальным запросам;
- держать долгие транзакции — bloat, lock contention, vacuum lag;
- делать unbounded `SELECT *` без `LIMIT` в production;
- не мониторить `pg_stat_activity`, `pg_stat_user_tables`, slow query log;
- работать без пула или держать слишком много прямых соединений;
- использовать `OFFSET` для пагинации больших таблиц — деградирует при росте (нужна keyset-пагинация, пример ниже).

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

## Query examples

Базовые паттерны (детали по pgx — в [11-go-patterns.md](./11-go-patterns.md)):

Создание таблицы:

```sql
CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Keyset-пагинация (вместо `OFFSET` — не деградирует на больших таблицах):

```sql
SELECT id, email, created_at
FROM users
WHERE created_at < $1  -- last seen created_at с предыдущей страницы
ORDER BY created_at DESC
LIMIT 50;
```

Upsert:

```sql
INSERT INTO users (email, status)
VALUES ('user@example.com', 'active')
ON CONFLICT (email) DO UPDATE
SET status = EXCLUDED.status;
```

Window function:

```sql
SELECT
    user_id,
    amount,
    SUM(amount) OVER (PARTITION BY user_id ORDER BY created_at) AS running_total
FROM orders;
```
