# MySQL

MySQL это популярная relational SQL database с большим ecosystem и долгой историей в web backend.

## Где используется

- классические web applications;
- read-heavy systems;
- CMS/e-commerce/legacy systems;
- backend services с простыми relational workloads;
- managed cloud databases.

## Сильные стороны

- mature ecosystem;
- много managed offerings;
- широко известная operational model;
- хорош для типичных OLTP workloads;
- много специалистов и tooling.

## Слабые стороны

- часть advanced SQL/extension story обычно сильнее у PostgreSQL;
- сложные analytical queries лучше выносить в analytical storage;
- поведение зависит от engine/configuration;
- как и любая SQL DB, страдает от плохих индексов и long transactions.

## Когда выбирать

Выбирай MySQL, если:
- команда и инфраструктура уже вокруг MySQL;
- workload простой OLTP;
- нужен stable relational storage;
- есть managed MySQL offering и стандартные web patterns.

## Когда не выбирать

Лучше подумать о другом варианте, если:
- нужны PostgreSQL-specific features;
- нужно много complex SQL and advanced indexing;
- workload скорее analytical, чем transactional.

## Типичные ошибки

- считать MySQL "простым" и не думать об индексах;
- не понимать isolation and locking behavior;
- использовать offset pagination на больших таблицах;
- не мониторить slow queries and replication lag.

## Interview-ready answer

MySQL это зрелая SQL база для OLTP и web workloads. Ее часто выбирают из-за ecosystem и operational familiarity, но она не отменяет базовых проблем SQL: индексы, транзакции, locks, replication и query plans.

## Query examples

Создание таблицы:

```sql
CREATE TABLE users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Запись:

```sql
INSERT INTO users (email, status)
VALUES ('user@example.com', 'active');
```

Получить данные:

```sql
SELECT id, email, status
FROM users
WHERE id = 42;
```

Фильтр:

```sql
SELECT id, email
FROM users
WHERE status = 'active'
ORDER BY created_at DESC
LIMIT 50;
```

Индекс:

```sql
CREATE INDEX idx_users_status_created_at
ON users(status, created_at);
```
