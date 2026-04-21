# ClickHouse

ClickHouse это column-oriented analytical database, оптимизированная для OLAP-workloads.

## Содержание

- [Где используется](#где-используется)
- [Как устроено: MergeTree family](#как-устроено-mergetree-family)
- [Partition key vs ORDER BY vs Primary key](#partition-key-vs-order-by-vs-primary-key)
- [Materialized views](#materialized-views)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- product analytics dashboards;
- event analytics (clickstream, user behavior);
- log/metric aggregation;
- A/B test results;
- ad/marketing analytics;
- near-real-time reporting по большим объемам.

## Как устроено: MergeTree family

`MergeTree` — базовый движок. Данные пишутся в "parts" (неизменяемые отсортированные файлы), которые периодически сливаются в фоне (merge). Именно поэтому:
- запись быстрая (append в новый part);
- UPDATE/DELETE поддерживаются, но дороги (scheduled mutation, а не inline);
- read эффективен за счет column-oriented хранения + сортировки.

**Семейство движков:**

| Движок | Для чего |
|---|---|
| `MergeTree` | базовый, append-only events |
| `ReplacingMergeTree` | дедупликация по ключу (eventually, не сразу) |
| `SummingMergeTree` | pre-aggregation sum при merge |
| `AggregatingMergeTree` | произвольные aggregate-state (для materialized views) |
| `CollapsingMergeTree` | CDC-like update/delete через знак |

`ReplacingMergeTree` — дедупликация происходит при merge, не при записи. До merge дубликаты существуют. При запросе нужно использовать `FINAL` или `GROUP BY` для корректного результата.

## Partition key vs ORDER BY vs Primary key

В ClickHouse это три разные вещи:

**PARTITION BY** — физическое деление данных по файлам. Используется для pruning (пропуск ненужных partitions при query) и lifecycle management (DROP PARTITION). Хорошие примеры: по дате, месяцу. Плохо делать слишком мелкие partitions.

**ORDER BY** — ключ сортировки внутри partition. Определяет физический порядок хранения строк. Это главный ключ для range scans. Первые колонки ORDER BY наиболее эффективны для WHERE/GROUP BY.

**PRIMARY KEY** — по умолчанию совпадает с ORDER BY. Определяет sparse index (ClickHouse не хранит index на каждую строку, только на каждые ~8192 строк).

```sql
CREATE TABLE events (
    event_date Date,
    event_time DateTime,
    user_id UInt64,
    event_type LowCardinality(String),
    value Float64
)
ENGINE = MergeTree
PARTITION BY event_date
ORDER BY (event_type, user_id, event_time);
```

Здесь:
- `PARTITION BY event_date` → пропускаем целые дни при date-filtered queries;
- `ORDER BY (event_type, user_id, event_time)` → эффективны запросы по `event_type`, `(event_type, user_id)` и их prefixes.

## Materialized views

Materialized view в ClickHouse — не кэш снапшота, а триггер: при каждой вставке в source таблицу данные трансформируются и пишутся в target таблицу.

Используется для pre-aggregation: вместо подсчета агрегатов при каждом query — вести их инкрементально.

```sql
-- target таблица с AggregatingMergeTree
CREATE TABLE hourly_events_mv
(
    hour        DateTime,
    event_type  LowCardinality(String),
    count       AggregateFunction(count)
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(hour)
ORDER BY (hour, event_type);

-- materialized view — инкрементально обновляет target при каждой вставке
CREATE MATERIALIZED VIEW hourly_events_view
TO hourly_events_mv
AS
SELECT
    toStartOfHour(event_time) AS hour,
    event_type,
    countState() AS count
FROM events
GROUP BY hour, event_type;
```

## Сильные стороны

- колоночное хранение + компрессия → быстрые аналитические scans;
- эффективные агрегации по большим объемам;
- LowCardinality тип ускоряет операции с повторяющимися значениями;
- materialized views для pre-aggregation;
- хорошо работает по append-only данным.

## Слабые стороны

- не OLTP: UPDATE/DELETE дорогие (mutation);
- transactional business logic держи в другой БД;
- ReplacingMergeTree дедупликация eventual — нужен FINAL или GROUP BY;
- schema и partition design влияют на производительность критично.

## Когда выбирать

Выбирай ClickHouse, если:
- надо считать агрегаты по большим объемам событий;
- много append-only events;
- PostgreSQL/MySQL уже не справляются с analytical scans;
- нужны быстрые dashboards с sub-second latency.

## Когда не выбирать

Не лучший выбор, если:
- нужен primary transactional storage;
- много small row-by-row updates;
- нужен strict relational consistency;
- workload в основном point lookup и OLTP-транзакции.

## Типичные ошибки

- использовать ClickHouse как замену PostgreSQL для order/payment data;
- не продумать ORDER BY и PARTITION BY → медленные queries или слишком много parts;
- не учитывать eventual deduplication в ReplacingMergeTree;
- делать слишком маленькие партиции (per hour вместо per day);
- игнорировать `LowCardinality` для enum-like полей.

## Interview-ready answer

ClickHouse — column-oriented OLAP database. Данные хранятся по столбцам (а не строкам), что ускоряет aggregation: нужно читать только нужные колонки. Базовый движок — MergeTree: данные пишутся в immutable parts, сливаются в фоне. PARTITION BY для pruning по датам, ORDER BY определяет порядок хранения и sparse index — от него зависит эффективность range scans. ReplacingMergeTree дедуплицирует eventually, не мгновенно. Materialized views обновляются инкрементально при каждой вставке — основной инструмент для pre-aggregation. Для OLTP и транзакций ClickHouse не подходит.

## Query examples

Создание таблицы:

```sql
CREATE TABLE events (
    event_date  Date,
    event_time  DateTime,
    user_id     UInt64,
    event_type  LowCardinality(String),
    value       Float64
)
ENGINE = MergeTree
PARTITION BY event_date
ORDER BY (event_type, event_time);
```

Агрегация с фильтром по дате (pruning по partition):

```sql
SELECT event_type, count() AS cnt, sum(value) AS total
FROM events
WHERE event_date >= '2026-04-01'
  AND event_date < '2026-05-01'
GROUP BY event_type
ORDER BY cnt DESC;
```

Временной ряд (hourly):

```sql
SELECT
    toStartOfHour(event_time) AS hour,
    count()                   AS events
FROM events
WHERE event_date = today()
GROUP BY hour
ORDER BY hour;
```

Funnel (window function):

```sql
SELECT user_id,
       countIf(event_type = 'view')     AS views,
       countIf(event_type = 'cart')     AS carts,
       countIf(event_type = 'purchase') AS purchases
FROM events
WHERE event_date >= '2026-04-01'
GROUP BY user_id
HAVING views > 0;
```
