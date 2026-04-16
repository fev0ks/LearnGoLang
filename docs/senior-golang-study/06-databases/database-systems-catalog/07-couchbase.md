# Couchbase

Couchbase это distributed document database с key-value доступом и дополнительными возможностями для query, search, analytics и mobile sync scenarios.

## Где используется

- document data;
- low-latency key-value access;
- user profiles;
- catalogs;
- session-like data;
- distributed document workloads;
- mobile/offline sync scenarios.

## Сильные стороны

- document + key-value model;
- low-latency access patterns;
- distributed architecture;
- flexible JSON documents;
- query capabilities поверх document model;
- полезен в сценариях, где важны document access and sync.

## Слабые стороны

- не полноценная relational DB;
- сложные relational joins and constraints не основная сила;
- operational model сложнее простого PostgreSQL;
- надо проектировать documents and indexes под access patterns.

## Когда выбирать

Выбирай Couchbase, если:
- данные document-like;
- нужен быстрый key-value/document access;
- важна distributed document architecture;
- есть use case вокруг mobile/offline sync или low-latency document workloads.

## Когда не выбирать

Лучше подумать о PostgreSQL/MySQL, если:
- domain strongly relational;
- нужны strict constraints and transactions around joins;
- команда не готова к документной модели.

## Типичные ошибки

- использовать как SQL DB с JSON;
- не проектировать ключи и индексы;
- не понимать consistency trade-offs;
- выбирать только потому, что "JSON удобнее".

## Interview-ready answer

Couchbase занимает нишу document/key-value систем: хорош для document-centric low-latency workloads, но не заменяет relational database там, где важны constraints, joins and transactional modeling.

## Query examples

Couchbase поддерживает key-value access и SQL-like язык запросов `SQL++`.

Document example:

```json
{
  "type": "user",
  "email": "user@example.com",
  "status": "active",
  "created_at": "2026-04-16T10:00:00Z"
}
```

SQL++ запрос:

```sql
SELECT u.email, u.status
FROM `users` AS u
WHERE u.type = "user"
  AND u.status = "active"
LIMIT 50;
```

Индекс:

```sql
CREATE INDEX idx_users_status
ON `users`(status)
WHERE type = "user";
```

Получить документ по ключу обычно делают через SDK key-value API:

```text
bucket.collection("users").get("user::42")
```
