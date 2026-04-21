# PostgreSQL

PostgreSQL это open-source relational database, которую часто выбирают как основной storage для backend-сервисов.

## Содержание

- [Где используется](#где-используется)
- [Как устроено: MVCC и Vacuum](#как-устроено-mvcc-и-vacuum)
- [Isolation levels](#isolation-levels)
- [Типы индексов](#типы-индексов)
- [Connection pooling](#connection-pooling)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Go: pgx и pgxpool](#go-pgx-и-pgxpool)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- transactional backend systems;
- financial/order/payment data;
- user data;
- admin panels;
- SaaS products;
- systems with complex SQL queries;
- systems that need strong constraints and data integrity.

## Как устроено: MVCC и Vacuum

PostgreSQL использует `MVCC` (Multi-Version Concurrency Control): при UPDATE или DELETE старая версия строки не удаляется сразу — создается новая версия, а старая помечается как "видимая только для старых транзакций". Это позволяет читателям не блокировать писателей.

Следствие: удаленные и обновленные строки остаются на диске как "мертвые кортежи". `VACUUM` периодически их убирает. `AUTOVACUUM` делает это автоматически.

Почему длинные транзакции опасны:
- открытая транзакция держит `xmin` — минимальный transaction ID, до которого vacuum не может зачищать мертвые кортежи;
- если транзакция висит часами, таблицы начинают раздуваться;
- это приводит к `table bloat` и деградации производительности.

Мониторинг:

```sql
-- найти долгие транзакции
SELECT pid, now() - xact_start AS duration, query
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
ORDER BY duration DESC;

-- таблицы с большим количеством мертвых кортежей
SELECT relname, n_dead_tup, n_live_tup
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC;
```

## Isolation levels

PostgreSQL поддерживает три практически полезных уровня:

| Уровень | Dirty read | Non-repeatable read | Phantom read | Serialization anomaly |
|---|---|---|---|---|
| `READ COMMITTED` (default) | нет | возможен | возможен | возможен |
| `REPEATABLE READ` | нет | нет | нет в PG | возможен |
| `SERIALIZABLE` | нет | нет | нет | нет |

`READ COMMITTED` — одна транзакция может видеть изменения другой транзакции, если та уже закоммитила. Транзакция A читает строку дважды и видит разный результат, если между чтениями B закоммитила UPDATE. Достаточно для большинства OLTP.

`REPEATABLE READ` — снапшот данных фиксируется в начале транзакции. Повторные чтения возвращают одинаковый результат. Нужен для отчетов и агрегатов, которые должны видеть согласованный срез.

`SERIALIZABLE` — все транзакции выполняются так, как будто они последовательны. PostgreSQL использует SSI (Serializable Snapshot Isolation). Нужен для сложных инвариантов (например: "сумма всех балансов не должна изменяться").

```sql
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
-- операции
COMMIT;
```

## Типы индексов

`B-tree` (default): равенство, сравнение, сортировка. Подходит для 95% случаев.

```sql
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_orders_created ON orders(created_at DESC);
```

`Partial index`: индексирует только строки, удовлетворяющие условию. Меньше, быстрее.

```sql
-- только активные пользователи
CREATE INDEX idx_active_users ON users(email) WHERE status = 'active';
```

`GIN` (Generalized Inverted Index): массивы, JSONB, полнотекстовый поиск. Индексирует элементы внутри значения.

```sql
-- быстрый поиск по JSONB полю
CREATE INDEX idx_users_tags ON users USING GIN(tags);

-- запрос с GIN
SELECT * FROM users WHERE tags @> '["admin"]';
```

`Expression index`: индексирует результат выражения.

```sql
-- поиск без учёта регистра
CREATE INDEX idx_users_email_lower ON users(LOWER(email));

SELECT * FROM users WHERE LOWER(email) = 'user@example.com';
```

`EXPLAIN ANALYZE` — обязательный инструмент для понимания плана запроса:

```sql
EXPLAIN ANALYZE
SELECT id, email FROM users WHERE status = 'active' ORDER BY created_at DESC LIMIT 50;
```

Смотреть на: `Seq Scan` vs `Index Scan`, `rows` vs актуальные, стоимость.

## Connection pooling

PostgreSQL создает отдельный процесс на каждое соединение. При 1000+ соединений память и CPU на context switching становятся заметными.

Решение: `PgBouncer` перед базой — принимает много соединений от приложений и держит меньшее число реальных соединений к PostgreSQL.

Режимы PgBouncer:
- `session` — соединение с PG держится на протяжении всей сессии клиента;
- `transaction` (рекомендуется) — соединение с PG выдается только на время транзакции, потом возвращается в пул;
- `statement` — для stateless single-statement workloads.

В Go `pgxpool` управляет пулом соединений на стороне приложения. Без PgBouncer рекомендуется не держать больше 20-30 соединений на инстанс (`MaxConns`).

## Сильные стороны

- ACID transactions;
- constraints, foreign keys, CHECK;
- powerful SQL с window functions, CTEs, lateral joins;
- богатые индексы: B-tree, partial, GIN, GiST, expression;
- JSONB для semi-structured данных;
- extensions (pg_vector, PostGIS, pg_cron);
- mature ecosystem;
- balance между strict relational model и practical flexibility.

## Слабые стороны

- horizontal write scaling сложнее, чем в distributed NoSQL;
- шардирование требует отдельного дизайна (Citus, pgpool, application-level);
- плохие queries легко убивают performance;
- нужно понимать indexes, locks, transactions, vacuum, connection pooling.

## Когда выбирать

Выбирай PostgreSQL, если:
- нужен надежный default database для backend;
- важны транзакции и data integrity;
- есть сложные query patterns;
- domain model relational;
- хочется не потерять гибкость на старте.

## Когда не выбирать

Лучше подумать о другом варианте, если:
- нужен extreme write scale across many nodes;
- все queries строго key-value и нужна serverless/cloud-native модель;
- workload в основном analytical scans по огромным объемам.

## Типичные ошибки

- использовать PostgreSQL как бездонную очередь (лучше Redis Streams или Kafka);
- не ставить индексы под реальные queries, ориентируясь только на схему;
- держать долгие транзакции — bloat, lock contention, vacuum lag;
- делать unbounded `SELECT *` без `LIMIT` в production;
- не мониторить `pg_stat_activity`, `pg_stat_user_tables`, slow query log;
- хранить соединения без пула или держать слишком много соединений напрямую;
- использовать `OFFSET` для пагинации больших таблиц — деградирует при росте.

## Go: pgx и pgxpool

`pgx` — рекомендуемый драйвер для Go (быстрее `database/sql`, нативные типы PG).

```go
import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

// создание пула
pool, err := pgxpool.New(ctx, "postgres://user:pass@localhost/db")
if err != nil {
    return err
}
defer pool.Close()

// настройка пула
cfg, _ := pgxpool.ParseConfig("postgres://user:pass@localhost/db")
cfg.MaxConns = 20
cfg.MinConns = 2
pool, _ = pgxpool.NewWithConfig(ctx, cfg)

// запрос
rows, err := pool.Query(ctx, "SELECT id, email FROM users WHERE status = $1", "active")
defer rows.Close()

// транзакция
tx, err := pool.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback(ctx) // no-op после Commit

_, err = tx.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromID)
if err != nil {
    return err
}
_, err = tx.Exec(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toID)
if err != nil {
    return err
}
return tx.Commit(ctx)
```

Всегда передавать `context.Context` — это позволяет отменить query при timeout или cancellation запроса.

## Interview-ready answer

PostgreSQL хороший default для backend: ACID транзакции, constraints, богатые индексы и SQL. Для senior уровня важно понимать MVCC — каждый UPDATE создает новую версию строки, старые зачищает vacuum. Длинная транзакция блокирует vacuum и вызывает table bloat. Isolation по умолчанию — READ COMMITTED, для отчётов нужен REPEATABLE READ, для сложных инвариантов — SERIALIZABLE. Индексы: B-tree для сравнений, GIN для JSONB/массивов, partial там где выборка по условию. Connection pooling обязателен — либо pgxpool на стороне Go, либо PgBouncer в transaction mode. Слабость: horizontal write scale без шардирования; для analytical scans лучше ClickHouse.

## Query examples

Создание таблицы:

```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Cursor-based пагинация (лучше OFFSET):

```sql
SELECT id, email, created_at
FROM users
WHERE created_at < $1  -- last seen created_at
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
