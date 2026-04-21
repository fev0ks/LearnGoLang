# MySQL

MySQL это популярная relational SQL database с большим ecosystem и долгой историей в web backend.

## Содержание

- [Где используется](#где-используется)
- [InnoDB: MVCC и locking](#innodb-mvcc-и-locking)
- [Репликация](#репликация)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- классические web applications;
- read-heavy systems;
- CMS/e-commerce/legacy systems;
- backend services с простыми relational workloads;
- managed cloud databases (RDS MySQL, Cloud SQL, PlanetScale).

## InnoDB: MVCC и locking

Единственный storage engine для production — `InnoDB`. `MyISAM` не поддерживает транзакции и foreign keys — использовать не стоит.

InnoDB использует MVCC похожим на PostgreSQL образом: читатели не блокируют писателей. Default isolation level — `REPEATABLE READ` (в отличие от PostgreSQL, где default `READ COMMITTED`).

**Gap locks** — особенность InnoDB для предотвращения phantom reads в `REPEATABLE READ`. При range query (`WHERE id BETWEEN 1 AND 10`) InnoDB лочит не только существующие строки, но и "промежутки" между ними, чтобы новые INSERT туда не мог сделать другой transaction.

Gap locks могут вызвать неожиданные дедлоки при высоком concurrency. Если phantom reads не критичны, можно снизить уровень до `READ COMMITTED`:

```sql
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
```

`SELECT ... FOR UPDATE` — explicit write lock. Используй осторожно: держи транзакцию короткой.

## Репликация

MySQL поддерживает асинхронную репликацию primary → replicas. Важные параметры:

- **Statement-based replication**: реплицируются SQL-запросы. Проблема: недетерминированные функции (`NOW()`, `UUID()`) дают разные результаты на replicas.
- **Row-based replication** (рекомендуется): реплицируются изменения строк. Безопаснее, но больше трафик бинлога.
- **GTID** (Global Transaction ID): каждая транзакция получает уникальный ID, что упрощает failover и point-in-time recovery.

Replication lag — отставание реплики от primary. Мониторить через `Seconds_Behind_Source`. При чтении с реплики может вернуться устаревшее значение — учитывай это при проектировании.

## Сильные стороны

- mature ecosystem и managed offerings (RDS, Cloud SQL, Aurora);
- широко известная operational model;
- хорош для типичных OLTP workloads;
- много специалистов и tooling;
- managed MySQL-совместимые сервисы (Aurora, PlanetScale).

## Слабые стороны

- часть advanced SQL features (оконные функции, CTEs, JSONB) слабее чем в PostgreSQL (улучшается с каждой версией);
- default isolation `REPEATABLE READ` + gap locks → неожиданные дедлоки;
- analytical queries лучше выносить в отдельный аналитический storage;
- плохие индексы и long transactions — те же проблемы, что и в PostgreSQL.

## Когда выбирать

Выбирай MySQL, если:
- команда и инфраструктура уже вокруг MySQL;
- workload простой OLTP;
- нужен stable relational storage с managed offering;
- есть legacy MySQL или Aurora-совместимые требования.

## Когда не выбирать

Лучше подумать о PostgreSQL, если:
- нужны PostgreSQL-specific features (advanced indexing, extensions, rich SQL);
- workload скорее analytical;
- старт нового проекта без legacy constraints.

## Типичные ошибки

- использовать `MyISAM` (нет транзакций, нет FK);
- не понимать gap locks → дедлоки при high concurrency;
- использовать `OFFSET` пагинацию на больших таблицах;
- не мониторить slow query log и replication lag;
- не понимать `EXPLAIN` plan.

## Interview-ready answer

MySQL это зрелая SQL база для OLTP и web workloads. InnoDB — единственный production engine: MVCC, transactions, FK. Default isolation `REPEATABLE READ` + gap locks могут вызвать неожиданные дедлоки при concurrent writes — иногда снижают до `READ COMMITTED`. Репликация row-based + GTID — стандартная рекомендация. Основные проблемы те же, что у PostgreSQL: индексы, долгие транзакции, replication lag при чтении с реплик.

## Query examples

Создание таблицы:

```sql
CREATE TABLE orders (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    status     VARCHAR(32) NOT NULL,
    amount     DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_status (user_id, status),
    INDEX idx_created_at  (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

Cursor-based пагинация (вместо OFFSET):

```sql
SELECT id, user_id, amount, created_at
FROM orders
WHERE created_at < ?
ORDER BY created_at DESC
LIMIT 50;
```

EXPLAIN для проверки плана:

```sql
EXPLAIN SELECT id, amount
FROM orders
WHERE user_id = 42 AND status = 'pending'
ORDER BY created_at DESC
LIMIT 10;
```
