# Эксплуатация ClickHouse

## Содержание

- [Что означает здоровый кластер](#что-означает-здоровый-кластер)
- [Capacity planning](#capacity-planning)
- [Parts и merges](#parts-и-merges)
- [Queries и resource limits](#queries-и-resource-limits)
- [Replication и distributed queues](#replication-и-distributed-queues)
- [Backups и restore](#backups-и-restore)
- [Storage policies и TTL](#storage-policies-и-ttl)
- [Миграции схемы](#миграции-схемы)
- [Security и multi-tenancy](#security-и-multi-tenancy)
- [Production checklist](#production-checklist)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

ClickHouse редко падает из-за одного медленного `SELECT`. Чаще деградация накапливается: producers создают слишком много parts, merges съедают disk bandwidth, replication queue растёт, свободное место уменьшается, а тяжёлый query добивает память. Наблюдаемость должна показывать этот путь до ошибки `Too many parts` или `No space left on device`.

---

## Что означает здоровый кластер

Для каждой production-таблицы полезно определить измеримые инварианты:

- ingestion lag укладывается в SLO;
- скорость merges в среднем не ниже скорости появления merge-work;
- active parts в горячих partitions не растут без границы;
- replicas активны и их queue/delay возвращаются к норме после пика;
- disk free space оставляет запас на merges, mutations и restore;
- p95/p99 query latency измеряется отдельно по workload class;
- backup завершился, а restore регулярно проверяется;
- ошибки async inserts и materialized views не скрыты успешным client response.

«CPU ниже 50%» не является достаточным сигналом. ClickHouse может быть ограничен disk I/O, memory bandwidth, network, Keeper latency или количеством files при умеренном CPU.

---

## Capacity planning

Для накопления берут **средний** поток, для мгновенной пропускной способности — **пиковый**. Смешивание этих чисел завышает storage или занижает compute.

Пример с явно заданными допущениями:

```text
средний поток              = 200 000 events/s
пиковый поток              = 600 000 events/s, то есть 3x
средний raw event          = 300 bytes
compression ratio          = 5:1
retention                  = 30 days
replication factor         = 2
целевой свободный запас    = 30% provisioned capacity

raw в среднем:
  200 000 * 300 = 60 000 000 bytes/s = 60 MB/s

raw за сутки:
  60 000 000 * 86 400 = 5 184 000 000 000 bytes ~= 5.184 TB/day

compressed за сутки:
  5.184 / 5 = 1.0368 TB/day

уникальные данные за 30 дней:
  1.0368 * 30 = 31.104 TB

две replicas:
  31.104 * 2 = 62.208 TB used

capacity при требовании оставить 30% свободными:
  62.208 / 0.70 = 88.87 TB provisioned
```

88.87 TB — не точный заказ дисков, а нижняя оценка. Дополнительно нужны:

- временное место для merges и mutations;
- metadata, indexes, projections и materialized-view targets;
- запас на задержавшийся TTL/backup;
- skew между shards;
- filesystem/reserved space и рост до следующего расширения.

Peak `600 000 events/s` используется для проверки insert CPU, network и merge capacity. Умножать его на 30 дней нельзя, если peak длится только часть суток. Compression ratio измеряют после загрузки representative data через `system.columns`/`system.parts`, а не берут из презентации.

---

## Parts и merges

Parts по partitions:

```sql
SELECT
    database,
    table,
    partition,
    count()                                      AS active_parts,
    sum(rows)                                    AS rows,
    formatReadableSize(sum(bytes_on_disk))       AS size,
    formatReadableSize(avg(bytes_on_disk))       AS avg_part_size
FROM system.parts
WHERE active
GROUP BY database, table, partition
ORDER BY active_parts DESC
LIMIT 50;
```

Текущие merges:

```sql
SELECT
    database,
    table,
    elapsed,
    progress,
    num_parts,
    result_part_name,
    formatReadableSize(total_size_bytes_compressed) AS source_size,
    formatReadableSize(memory_usage)                AS memory
FROM system.merges
ORDER BY elapsed DESC;
```

Само число parts без контекста мало полезно. Важна динамика:

- после обычного пика parts выросли и затем уменьшились — cluster догнал поток;
- parts растут часами при стабильном ingestion — merge capacity ниже входа;
- много крошечных parts — слишком мелкие inserts или partitioning;
- несколько огромных parts и постоянный backlog новых — scheduler может не выбрать их совместный merge по size limits;
- merges стоят при полном диске — для нового part нет временного места.

Не лечить backlog расписанием `OPTIMIZE TABLE ... FINAL`. Команда обходит некоторые size safeguards, переписывает крупные partitions и конкурирует с теми merges, которые и так не успевают. Сначала уменьшают частоту inserts, исправляют partitioning и проверяют disk throughput.

История создания и merge parts находится в `system.part_log`, если таблица логирования включена. По ней можно отличить всплеск inserts от медленного merge и оценить write amplification.

---

## Queries и resource limits

Тяжёлые и ошибочные запросы:

```sql
SELECT
    event_time,
    query_duration_ms,
    read_rows,
    formatReadableSize(read_bytes)   AS read_size,
    formatReadableSize(memory_usage) AS memory,
    exception_code,
    query_id,
    normalized_query_hash,
    query
FROM system.query_log
WHERE event_time >= now() - INTERVAL 1 HOUR
  AND type IN ('QueryFinish', 'ExceptionWhileProcessing')
ORDER BY query_duration_ms DESC
LIMIT 50;
```

Полезно группировать по `normalized_query_hash`, иначе один template с разными literals выглядит как тысячи разных запросов. Для distributed query `initial_query_id` связывает initiator и remote work.

Основные guardrails задают через settings profiles и quotas:

| Ограничение | От чего защищает | Побочный эффект |
| --- | --- | --- |
| `max_memory_usage` | OOM одного query | query завершается или spill-ит при доступной стратегии |
| `max_execution_time` | зависшие/слишком долгие queries | длинный легитимный export нужно вынести в другой profile |
| `max_concurrent_queries_for_user` | один tenant занимает все slots | очередь/ошибка для burst workload |
| `max_rows_to_read`, `max_bytes_to_read` | случайный full scan | exploratory query может быть отклонён |
| external group/sort/join thresholds | OOM агрегата, sort, JOIN | disk spill увеличивает latency и I/O |

Разные workloads не должны иметь один profile: dashboard, ingestion, ad-hoc analytics и backfill конкурируют по-разному. Resource limits — последняя линия защиты; они не заменяют правильный `ORDER BY` и query review.

Наблюдать нужно и client-side saturation: если pool Go-приложения имеет 10 connections и все заняты долгими exports, dashboard ждёт ещё до ClickHouse. Метрики server queries и client pool рассматривают вместе.

---

## Replication и distributed queues

Для replicated tables проверяют `system.replicas` и `system.replication_queue`, для async `Distributed` inserts — `system.distribution_queue`. Практические alerts строят на длительности и тренде, а не на единичной записи queue: краткий backlog после deploy ожидаем, постоянный рост — нет.

Особенно опасны:

- `is_readonly = 1` и expired Keeper session;
- уменьшение `active_replicas`, из-за которого недоступен insert quorum;
- повторяющийся `last_exception` одного queue item;
- replication delay больше freshness SLO;
- distribution queue с растущими `data_files` и `error_count`;
- разные schemas replicas после неуспешного `ON CLUSTER` DDL.

Подробные запросы и failure semantics — [10-sharding-and-replication.md](./10-sharding-and-replication.md).

Для async inserts дополнительно смотрят `system.asynchronous_inserts` и `system.asynchronous_insert_log`: размер/возраст buffers, flush reasons и exceptions. `wait_for_async_insert = 0` особенно требует server-side alert, потому что client уже получил success.

---

## Backups и restore

Встроенная команда `BACKUP` умеет сохранять tables/databases в настроенный Disk, S3 и другие destinations.

```sql
BACKUP TABLE analytics.events
TO Disk('backups', 'events-2026-08-21');

RESTORE TABLE analytics.events AS analytics.events_restore_check
FROM Disk('backups', 'events-2026-08-21');
```

Backup-политика должна отвечать на пять вопросов:

1. **RPO:** сколько данных допустимо потерять между backups.
2. **RTO:** сколько времени займёт restore полного объёма и metadata.
3. **Scope:** tables, dictionaries, users/RBAC, configs, Keeper metadata, external object data.
4. **Retention:** сколько generations хранится и где находится immutable/off-site copy.
5. **Verification:** когда последний раз выполнялся полный restore и data reconciliation.

Факт `BACKUP ...` со статусом success не доказывает RTO. На полном bucket, другом cluster version и реальной network bandwidth restore может идти часы. Минимум периодически восстанавливать копию под другим именем, сверять row counts, partition list, checksums/aggregates и выполнять контрольные queries.

Для replicas обычно не нужно независимо копировать одинаковые parts с каждого узла, но exact strategy зависит от destination и topology. Нельзя удалять единственную backup generation до успешной проверки новой.

Статусы доступны в `system.backups` и история — в `system.backup_log`.

---

## Storage policies и TTL

Storage policy описывает disks и volumes, а table TTL перемещает или удаляет данные:

```sql
ALTER TABLE events MODIFY TTL
    event_date + INTERVAL 7 DAY TO VOLUME 'warm',
    event_date + INTERVAL 30 DAY TO VOLUME 'cold',
    event_date + INTERVAL 365 DAY DELETE;
```

Типичный tiering:

| Tier | Носитель | Данные | Trade-off |
| --- | --- | --- | --- |
| hot | local NVMe | последние часы/дни | дорого, минимальная latency |
| warm | более дешёвый block storage | недели | ниже IOPS |
| cold | object storage | месяцы/годы | network latency и cache dependency |

TTL выполняется во время background work, а не в точную секунду срока. План capacity обязан выдерживать задержку cleanup. `ttl_only_drop_parts = 1` дешёвый, когда весь part истекает одновременно; если TTL режет строки внутри mixed-age part, требуется rewrite.

Object storage меняет bottleneck: локального места меньше, но растёт значение filesystem cache, network throughput, request rate и availability внешнего storage. «S3 бесконечен» не означает, что cluster сможет прочитать любой объём за SLO.

Свободное место:

```sql
SELECT
    name,
    path,
    formatReadableSize(free_space)       AS free,
    formatReadableSize(total_space)      AS total,
    formatReadableSize(keep_free_space)  AS reserved
FROM system.disks;
```

Alert по проценту дополняют абсолютным запасом: 10% от 100 TB — ещё 10 TB, а 10% от 500 GB — только 50 GB, чего может не хватить на одну mutation.

---

## Миграции схемы

Безопасная миграция крупной таблицы обычно выглядит как shadow-table workflow:

1. Создать `events_v2` с новой schema/engine/`ORDER BY`.
2. Подключить live dual-write или materialized view с idempotency boundary.
3. Backfill history partitions небольшими batches.
4. Сверить counts, sums, distinct keys и representative queries по каждой partition.
5. Догнать watermark и атомарно переключить имена через `EXCHANGE TABLES` там, где поддерживается нужной database engine.
6. Сохранить старую таблицу на rollback window, затем удалить осознанно.

Нельзя оценивать миграцию только временем `INSERT SELECT`. Она одновременно читает старую таблицу, пишет новую, создаёт parts и запускает merges, то есть конкурирует и с ingestion, и с dashboards. Backfill throttling — часть плана.

Для `ON CLUSTER` DDL проверяют применение на каждом узле. Rolling upgrade к смешанным версиям дополнительно требует проверки совместимости форматов parts, patch parts и значений настроек по умолчанию.

---

## Security и multi-tenancy

Базовый production minimum:

- TLS между clients и servers, а для self-managed cluster — защищённые interserver/Keeper connections по threat model;
- отдельные users/roles для ingestion, dashboards, analysts и backups;
- secrets вне DDL, репозитория и query text;
- минимальные grants: writer не должен иметь `DROP`, dashboard — `ALTER`;
- quotas/settings profiles на tenant или workload;
- row policies только с пониманием их влияния на performance и distributed execution;
- audit важных DDL, grants, mutations и backup operations.

Multi-tenancy — не только `tenant_id` в `ORDER BY`. Нужно решить noisy-neighbor isolation, per-tenant retention, quota, deletion, sharding skew и возможность выгрузки/удаления данных одного tenant без полного scan.

---

## Production checklist

- [ ] Batched/async ingestion проверен на peak, parts после пика уменьшаются.
- [ ] Sharding key проверен на bytes/QPS skew, не только на число keys.
- [ ] Replication и Keeper имеют alerts и проверенный rejoin runbook.
- [ ] Disk capacity учитывает replicas, headroom, merges, projections и delayed TTL.
- [ ] Query profiles разделяют dashboard, ingestion, ad-hoc и backfill.
- [ ] Materialized views имеют backfill/retry/reconciliation procedure.
- [ ] Backup хранится отдельно от cluster и регулярно восстанавливается.
- [ ] Schema migration имеет watermark, validation и rollback window.
- [ ] Зависящие от версии значения по умолчанию зафиксированы в config/tests перед upgrade.
- [ ] SLO измеряется end-to-end: source offset → видимость в query, а не только server INSERT latency.

---

## Interview-ready answer

**1. За чем следить в ClickHouse в первую очередь?**

- За trends active parts, merges backlog, replication/distribution queues, disk free space и query memory.
- CPU сам по себе недостаточен: cluster часто ограничен disk, memory bandwidth, network или Keeper.

**2. Как считать storage capacity?**

- Средний поток × средний размер × retention / измеренное сжатие.
- Затем replication factor, projections/targets и требуемый free-space headroom.
- Peak используют для throughput, а не умножают на весь retention period.

**3. Почему нельзя запускать OPTIMIZE FINAL по cron?**

- Он переписывает крупные partitions, обходит часть merge safeguards и конкурирует с ingestion/обычными merges.
- Backlog лечат batching, partitioning и capacity, а не принудительной полной перезаписью.

**4. Реплики заменяют backup?**

- Нет: логическое удаление и плохая mutation реплицируются.
- Backup имеет отдельные RPO/RTO, retention, off-site copy и обязательный restore test.

**5. Как безопасно мигрировать ORDER BY или engine?**

- Создать shadow table, подключить live поток, backfill по watermark, сверить partitions и queries.
- Переключить имена после catch-up и сохранить rollback window.

---

## Официальная документация

- [System tables](https://clickhouse.com/docs/operations/system-tables)
- [BACKUP and RESTORE](https://clickhouse.com/docs/operations/backup)
- [Storage configuration](https://clickhouse.com/docs/operations/storing-data)
- [TTL](https://clickhouse.com/docs/guides/developer/ttl)
- [Quotas](https://clickhouse.com/docs/operations/quotas)
- [Users and roles](https://clickhouse.com/docs/operations/access-rights)
