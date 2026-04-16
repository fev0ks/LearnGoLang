# Cassandra

Cassandra это distributed wide-column database, рассчитанная на масштабируемость и высокую доступность.

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

- huge write throughput;
- event/time-series-like data;
- metrics-like workloads;
- multi-node distributed storage;
- systems where availability and scale are more important than relational querying.

## Сильные стороны

- horizontal scalability;
- high write throughput;
- high availability;
- replication across nodes;
- хорошо работает при заранее известных query patterns.

## Слабые стороны

- не для ad-hoc queries;
- нет привычных relational joins;
- data modeling сложнее;
- queries надо проектировать от access patterns;
- operational complexity выше, чем у single-node SQL DB.

## Когда выбирать

Выбирай Cassandra, если:
- нужен огромный write scale;
- данные можно партиционировать хорошим partition key;
- queries заранее известны;
- acceptable eventual consistency or tunable consistency model.

## Когда не выбирать

Не лучший выбор, если:
- нужен flexible querying;
- важны joins and relational constraints;
- команда не готова к distributed database operations;
- объемы не оправдывают сложность.

## Типичные ошибки

- проектировать модель как для SQL;
- выбирать плохой partition key;
- делать hot partitions;
- ожидать, что Cassandra сама хорошо ответит на любые queries.

## Interview-ready answer

Cassandra выбирают не потому, что это "быстрая база", а потому что нужен distributed write scale и availability под заранее известные access patterns. Цена этого выбора - сложное моделирование данных и отсутствие привычной relational гибкости.

## Query examples

Cassandra Query Language похож на SQL, но модель другая: queries должны соответствовать partition/clustering keys.

Создание таблицы:

```sql
CREATE TABLE user_events (
    user_id uuid,
    event_time timestamp,
    event_type text,
    payload text,
    PRIMARY KEY (user_id, event_time)
) WITH CLUSTERING ORDER BY (event_time DESC);
```

Запись:

```sql
INSERT INTO user_events (user_id, event_time, event_type, payload)
VALUES (11111111-1111-1111-1111-111111111111, toTimestamp(now()), 'login', '{}');
```

Получить последние события пользователя:

```sql
SELECT event_time, event_type, payload
FROM user_events
WHERE user_id = 11111111-1111-1111-1111-111111111111
LIMIT 20;
```

Важный нюанс:
- Cassandra не предназначена для произвольных `WHERE` по любым колонкам;
- сначала проектируется query pattern, потом таблица.
