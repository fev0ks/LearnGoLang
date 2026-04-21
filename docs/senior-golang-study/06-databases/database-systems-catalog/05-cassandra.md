# Cassandra

Cassandra это distributed wide-column database, рассчитанная на масштабируемость и высокую доступность.

## Содержание

- [Где используется](#где-используется)
- [Как устроено: token ring и replication](#как-устроено-token-ring-и-replication)
- [Consistency levels](#consistency-levels)
- [Tombstones и compaction](#tombstones-и-compaction)
- [Partition key design](#partition-key-design)
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
- IoT, activity feeds, audit logs;
- multi-datacenter distributed storage;
- systems where availability > strict consistency.

## Как устроено: token ring и replication

Cassandra использует consistent hashing: каждая нода отвечает за диапазон токенов на кольце. При записи данные направляются на ноды, ответственные за hash partition key.

`Replication factor (RF)` определяет, на скольких нодах хранится каждая партиция. RF=3 означает, что данные записаны на 3 ноды.

Virtual nodes (`vnodes`) — каждая физическая нода отвечает не за один диапазон, а за много маленьких диапазонов. Это упрощает балансировку при добавлении нод.

Запись проходит через:
1. `CommitLog` — WAL для durability;
2. `Memtable` — in-memory структура;
3. `SSTable` — flush на диск при заполнении memtable.

Cassandra writes-optimized: запись всегда append (никаких in-place update), поэтому throughput записи очень высокий.

## Consistency levels

Consistency level задается per-query (отдельно для read и write). Определяет, сколько реплик должны ответить.

| Level | Сколько реплик отвечают |
|---|---|
| `ONE` | одна |
| `TWO` | две |
| `QUORUM` | большинство (RF/2 + 1) |
| `LOCAL_QUORUM` | большинство в локальном DC |
| `ALL` | все реплики |

Формула strong consistency: `write CL + read CL > RF`.

При RF=3: `QUORUM` write + `QUORUM` read = 2+2 > 3 → strong consistency.  
`ONE` write + `ONE` read = 1+1 = 2 ≤ 3 → eventual consistency (может вернуть старое значение).

Для большинства production workloads: запись `LOCAL_QUORUM`, чтение `LOCAL_QUORUM` — это strong consistency в рамках одного DC с хорошим throughput.

`ALL` — самый сильный, но теряет availability если одна нода недоступна.

## Tombstones и compaction

`Tombstone` — маркер удаления. Cassandra не удаляет данные сразу при DELETE — создает tombstone. Настоящее удаление происходит при compaction после `gc_grace_seconds` (default 10 дней).

Проблема tombstones:
- при запросе, захватывающем tombstones, Cassandra должна их перебрать;
- большое количество tombstones замедляет read;
- `tombstone_failure_threshold` защищает от OOM, но вызывает ошибки запроса.

Паттерны, генерирующие много tombstones:
- DELETE строк в часто читаемых партициях;
- TTL на колонки (каждый истекший TTL = tombstone);
- wide row с частыми обновлениями отдельных колонок.

`Compaction` — процесс слияния SSTables и зачистки tombstones:
- `STCS` (SizeTieredCompactionStrategy) — хорош для write-heavy workloads;
- `LCS` (LeveledCompactionStrategy) — хорош для read-heavy, меньше read amplification;
- `TWCS` (TimeWindowCompactionStrategy) — оптимален для time-series данных с TTL.

## Partition key design

Partition key определяет, как данные распределяются по кластеру. Это главное архитектурное решение.

**Hot partition** — когда один partition key получает непропорционально большой трафик. Пример: `user_id` активного VIP пользователя с миллионами событий в день → один shard перегружен.

Решение для time-series: bucket по времени.

```sql
-- плохо: один partition может стать огромным
PRIMARY KEY (user_id, event_time)

-- хорошо: bucket ограничивает размер партиции
PRIMARY KEY ((user_id, bucket), event_time)
-- bucket = YYYYMM или YYYYMMDD в зависимости от нагрузки
```

**Unbounded partition**: partition без ограничения размера растет бесконечно → проблемы с read latency, compaction, repair.

## Сильные стороны

- горизонтальная масштабируемость без single point of failure;
- high write throughput (append-only, WAL);
- multi-datacenter replication;
- tunable consistency per query;
- хорошо работает при заранее известных access patterns.

## Слабые стороны

- нет ad-hoc queries и joins;
- queries проектируются от partition/clustering keys;
- tombstones и compaction требуют понимания;
- operational complexity выше, чем у single-node SQL;
- eventual consistency — нужно проектировать для idempotency.

## Когда выбирать

Выбирай Cassandra, если:
- нужен huge write scale (миллионы записей в секунду);
- данные можно партиционировать хорошим partition key;
- queries заранее известны и стабильны;
- acceptable eventual consistency или tunable consistency;
- нужна multi-datacenter replication.

## Когда не выбирать

Не лучший выбор, если:
- нужен flexible querying;
- важны joins и relational constraints;
- команда не готова к distributed database operations;
- объемы не оправдывают сложность (рассмотри PostgreSQL).

## Типичные ошибки

- проектировать схему как для SQL (нормализация, joins);
- выбирать плохой partition key → hot partition или unbounded partition;
- игнорировать tombstones при design паттернов с частыми DELETE;
- использовать `ALLOW FILTERING` — это full scan по всей таблице;
- ожидать strong consistency при `ONE` write + `ONE` read.

## Interview-ready answer

Cassandra — distributed wide-column DB, оптимизированная для write-heavy workloads. Архитектура: token ring (consistent hashing), данные распределяются по нодам по partition key, replication factor определяет количество копий. Strong consistency достигается через `QUORUM` write + `QUORUM` read при RF=3. Главное архитектурное решение — partition key: он должен равномерно распределять нагрузку и ограничивать размер партиции (особенно для time-series используй time buckets). Tombstones — маркеры удаления, которые зачищаются при compaction; их много → медленные reads. Cassandra не подходит для ad-hoc queries, joins и транзакционных инвариантов.

## Query examples

Создание таблицы для event stream (с time bucket):

```sql
CREATE TABLE user_events (
    user_id uuid,
    bucket  text,           -- 'YYYYMM'
    event_time timestamp,
    event_type text,
    payload text,
    PRIMARY KEY ((user_id, bucket), event_time)
) WITH CLUSTERING ORDER BY (event_time DESC)
  AND COMPACTION = {'class': 'TimeWindowCompactionStrategy',
                    'compaction_window_size': 1,
                    'compaction_window_unit': 'DAYS'};
```

Запись:

```sql
INSERT INTO user_events (user_id, bucket, event_time, event_type, payload)
VALUES (11111111-1111-1111-1111-111111111111, '202604',
        toTimestamp(now()), 'login', '{}');
```

Получить последние события (всегда указывать полный partition key):

```sql
SELECT event_time, event_type, payload
FROM user_events
WHERE user_id = 11111111-1111-1111-1111-111111111111
  AND bucket = '202604'
LIMIT 20;
```

`ALLOW FILTERING` — почти всегда антипаттерн:

```sql
-- плохо: full scan по всей таблице
SELECT * FROM user_events WHERE event_type = 'login' ALLOW FILTERING;
```
