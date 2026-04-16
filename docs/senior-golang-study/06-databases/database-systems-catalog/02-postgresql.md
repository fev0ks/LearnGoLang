# PostgreSQL

PostgreSQL это open-source relational database, которую часто выбирают как основной storage для backend-сервисов.

## Содержание

- [Где используется](#где-используется)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
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

## Сильные стороны

- ACID transactions;
- constraints and foreign keys;
- powerful SQL;
- indexes: B-tree, partial, expression, GIN, GiST;
- JSONB для semi-structured данных;
- extensions;
- mature ecosystem;
- хороший balance между strict relational model и practical flexibility.

## Слабые стороны

- horizontal write scaling сложнее, чем в distributed NoSQL;
- шардирование требует отдельного design;
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

- использовать PostgreSQL как бездонную очередь;
- не ставить индексы под реальные queries;
- держать долгие транзакции;
- делать unbounded fan-out queries из Go;
- не мониторить connection pool.

## Interview-ready answer

PostgreSQL часто хороший default для backend, потому что дает транзакции, SQL, constraints и богатые индексы. Но его надо уметь эксплуатировать: смотреть query plans, locks, pool saturation и границы транзакций.

## Query examples

Создание таблицы:

```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Запись:

```sql
INSERT INTO users (email, status)
VALUES ('user@example.com', 'active');
```

Получить одну строку:

```sql
SELECT id, email, status
FROM users
WHERE id = 42;
```

Фильтр и сортировка:

```sql
SELECT id, email, created_at
FROM users
WHERE status = 'active'
ORDER BY created_at DESC
LIMIT 50;
```

Индекс под query pattern:

```sql
CREATE INDEX idx_users_status_created_at
ON users(status, created_at DESC);
```
