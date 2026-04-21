# Couchbase

Couchbase это distributed document database с key-value доступом и query, search, analytics и mobile sync capabilities.

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

- document data с low-latency key-value access;
- user profiles и session state;
- product catalogs;
- distributed document workloads;
- mobile/offline sync scenarios (Couchbase Lite + Sync Gateway).

## Сильные стороны

- document + key-value model в одной системе;
- sub-millisecond key-value access (данные в managed memory);
- flexible JSON documents;
- SQL++ (N1QL) поверх document model;
- distributed architecture с automatic sharding;
- mobile sync через Couchbase Lite.

## Слабые стороны

- не полноценная relational DB: joins возможны, но дороги;
- operational model сложнее PostgreSQL;
- consistency model требует понимания;
- меньший ecosystem и community, чем у MongoDB или Redis.

## Когда выбирать

Выбирай Couchbase, если:
- данные document-like и нужен быстрый key-value access;
- важна distributed document architecture;
- есть mobile/offline sync use case (Couchbase Lite).

## Когда не выбирать

Лучше подумать о других вариантах, если:
- domain strongly relational с constraints и joins;
- нужен pure cache — Redis проще;
- нужна гибкость MongoDB с более зрелым ecosystem;
- нет mobile sync — PostgreSQL или MongoDB закроют задачу дешевле.

## Типичные ошибки

- использовать как SQL DB с JSON;
- не проектировать ключи и индексы под access patterns;
- выбирать только потому, что "JSON удобнее" — MongoDB или даже PostgreSQL+JSONB могут быть проще в эксплуатации.

## Interview-ready answer

Couchbase занимает нишу document/key-value систем с акцентом на low-latency key-value access и mobile sync. Основное отличие от MongoDB — managed memory cache (данные частично в RAM) и Couchbase Lite для мобильных клиентов с offline-first sync. Для большинства document workloads без mobile sync MongoDB или PostgreSQL+JSONB будут проще в эксплуатации.

## Query examples

Document example:

```json
{
  "type": "user",
  "email": "user@example.com",
  "status": "active",
  "created_at": "2026-04-20T10:00:00Z"
}
```

SQL++ (N1QL) запрос:

```sql
SELECT u.email, u.status
FROM `users` AS u
WHERE u.type = "user"
  AND u.status = "active"
ORDER BY u.created_at DESC
LIMIT 50;
```

Индекс:

```sql
CREATE INDEX idx_users_status
ON `users`(status)
WHERE type = "user";
```

Key-value доступ через SDK (Go):

```go
collection := cluster.Bucket("users").DefaultCollection()

// get
result, err := collection.Get("user::42", nil)
var user UserDocument
result.Content(&user)

// upsert
_, err = collection.Upsert("user::42", user, nil)
```
