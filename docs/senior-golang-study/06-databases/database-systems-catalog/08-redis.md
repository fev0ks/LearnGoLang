# Redis

Redis это in-memory data store с набором структур данных.

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

- cache;
- rate limiting;
- counters;
- sessions;
- distributed locks with caveats;
- leaderboards;
- queues-lite;
- ephemeral fast state.

## Сильные стороны

- очень низкая latency;
- богатые data structures;
- TTL;
- atomic commands;
- удобен для hot data;
- хорошо подходит как cache-aside layer.

## Слабые стороны

- memory cost;
- не всегда правильный source of truth;
- persistence and durability требуют понимания;
- cache invalidation сложно делать правильно;
- hot keys могут стать bottleneck.

## Когда выбирать

Выбирай Redis, если:
- нужен быстрый cache;
- нужны TTL and counters;
- надо ограничивать rate;
- надо хранить ephemeral state;
- primary DB не должна обслуживать hot read path.

## Когда не выбирать

Не лучший выбор, если:
- нужна сложная transactional model;
- данные должны быть source of truth без потерь;
- объем больше доступной памяти;
- нужны ad-hoc relational queries.

## Типичные ошибки

- считать Redis "просто быстрой базой";
- хранить критичные данные без продуманной durability story;
- не ставить TTL;
- делать unbounded keys;
- не думать о hot keys.

## Interview-ready answer

Redis чаще всего используют как cache или fast ephemeral state. Он силен в latency, TTL and atomic counters, но его надо осторожно использовать как primary storage.

## Query examples

Redis работает командами, а не SQL.

String key:

```text
SET user:42:email user@example.com
GET user:42:email
```

Hash:

```text
HSET user:42 email user@example.com status active
HGETALL user:42
HGET user:42 email
```

TTL:

```text
SET session:abc123 user-42 EX 3600
GET session:abc123
```

Counter:

```text
INCR rate:user:42
EXPIRE rate:user:42 60
```

Set membership:

```text
SADD online_users user-42
SISMEMBER online_users user-42
```
