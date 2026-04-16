# ClickHouse

ClickHouse это column-oriented analytical database.

## Где используется

- analytics;
- dashboards;
- event analytics;
- log/event aggregation;
- large scans and aggregations;
- near-real-time reporting.

## Сильные стороны

- быстрые analytical queries;
- columnar storage;
- compression;
- эффективные агрегации;
- хорошо работает по большим объемам append-like данных.

## Слабые стороны

- не OLTP database;
- frequent small updates не основной сценарий;
- transactional business logic лучше держать в другой БД;
- schema and partition design важны.

## Когда выбирать

Выбирай ClickHouse, если:
- надо считать агрегаты по большим объемам;
- много append-only events;
- нужны быстрые dashboards;
- PostgreSQL/MySQL уже не справляются с analytical scans.

## Когда не выбирать

Не лучший выбор, если:
- нужен primary transactional storage;
- много small row-by-row updates;
- нужен strict relational consistency;
- workload в основном point lookup and transactions.

## Типичные ошибки

- использовать ClickHouse как замену PostgreSQL для order/payment data;
- не продумать partition/order key;
- забыть, что быстрые aggregations не равны удобным transactions;
- пытаться делать частые updates как в OLTP.

## Interview-ready answer

ClickHouse хорош для аналитики, событий и больших агрегаций. Его не стоит выбирать как основную transactional базу для бизнес-сущностей, где нужны частые update, constraints и транзакции.

## Query examples

Создание таблицы:

```sql
CREATE TABLE events (
    event_date Date,
    event_time DateTime,
    user_id UInt64,
    event_type String,
    value Float64
)
ENGINE = MergeTree
PARTITION BY event_date
ORDER BY (event_type, event_time);
```

Запись:

```sql
INSERT INTO events (event_date, event_time, user_id, event_type, value)
VALUES ('2026-04-16', now(), 42, 'page_view', 1.0);
```

Агрегация:

```sql
SELECT event_type, count() AS cnt
FROM events
WHERE event_date = '2026-04-16'
GROUP BY event_type
ORDER BY cnt DESC;
```

Временной ряд:

```sql
SELECT
    toStartOfMinute(event_time) AS minute,
    count() AS events
FROM events
WHERE event_time >= now() - INTERVAL 1 HOUR
GROUP BY minute
ORDER BY minute;
```
