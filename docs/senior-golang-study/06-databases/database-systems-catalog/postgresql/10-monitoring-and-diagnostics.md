# Мониторинг и диагностика

## Содержание

- [Ключевые системные представления](#ключевые-системные-представления)
- [pg_stat_activity: активные соединения](#pg_stat_activity-активные-соединения)
- [pg_stat_user_tables: состояние таблиц](#pg_stat_user_tables-состояние-таблиц)
- [pg_stat_user_indexes: использование индексов](#pg_stat_user_indexes-использование-индексов)
- [pg_locks: блокировки](#pg_locks-блокировки)
- [pg_stat_statements: топ запросов](#pg_stat_statements-топ-запросов)
- [pg_stat_io: диагностика ввода-вывода](#pg_stat_io-диагностика-ввода-вывода)
- [Диагностика bloat](#диагностика-bloat)
- [Диагностика репликации](#диагностика-репликации)
- [Production checklist](#production-checklist)
- [Алерты (примерные пороги)](#алерты-примерные-пороги)
- [Инструменты](#инструменты)

---

## Ключевые системные представления

| Представление | Что показывает |
|---|---|
| `pg_stat_activity` | Активные соединения, состояние, запросы |
| `pg_stat_user_tables` | Статистика таблиц: scan, vacuum, analyze, dead tuples |
| `pg_stat_user_indexes` | Использование индексов |
| `pg_stat_statements` | Агрегированная статистика запросов (расширение) |
| `pg_locks` | Текущие блокировки |
| `pg_stat_replication` | Состояние реплик (на primary) |
| `pg_stat_subscription` | Состояние подписок (logical replication) |
| `pg_replication_slots` | Репликационные слоты и их lag |
| `pg_stat_bgwriter` | Фоновый writer: checkpoints, buffer alloc |
| `pg_stat_io` | I/O в разрезе backend_type/context (PG16+) |
| `pg_stat_database` | Статистика per-database |

---

## pg_stat_activity: активные соединения

```sql
-- все активные (не idle) соединения
SELECT pid, usename, application_name, client_addr,
       state, wait_event_type, wait_event,
       now() - xact_start AS xact_age,
       now() - query_start AS query_age,
       left(query, 100) AS query
FROM pg_stat_activity
WHERE state != 'idle'
ORDER BY xact_age DESC NULLS LAST;
```

State значения:
- `active` — выполняется запрос.
- `idle in transaction` — транзакция открыта, запрос не выполняется.
- `idle in transaction (aborted)` — ошибка в транзакции, ждёт rollback.
- `fastpath function call` — выполняется low-level функция.

Долгие `idle in transaction` — типичная проблема:

```sql
SELECT pid, usename, application_name,
       now() - xact_start AS idle_in_txn_age,
       left(query, 100) AS last_query
FROM pg_stat_activity
WHERE state = 'idle in transaction'
  AND xact_start < now() - interval '5 minutes'
ORDER BY xact_age DESC;
```

Завершить зависший процесс:

```sql
-- мягкое завершение (дождётся завершения текущей операции)
SELECT pg_cancel_backend(pid);

-- жёсткое завершение
SELECT pg_terminate_backend(pid);
```

---

## pg_stat_user_tables: состояние таблиц

```sql
SELECT relname,
       n_live_tup,
       n_dead_tup,
       round(n_dead_tup::numeric / NULLIF(n_live_tup + n_dead_tup, 0) * 100, 1) AS dead_pct,
       last_vacuum,
       last_autovacuum,
       last_analyze,
       last_autoanalyze,
       seq_scan,
       idx_scan,
       round(idx_scan::numeric / NULLIF(seq_scan + idx_scan, 0) * 100, 1) AS idx_scan_pct,
       n_mod_since_analyze
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC
LIMIT 20;
```

Что искать:
- `dead_pct > 10%` — нужен vacuum или починить autovacuum.
- `last_autovacuum IS NULL` или очень давно — autovacuum не запускается.
- `seq_scan` намного больше `idx_scan` для большой таблицы — возможно нужен индекс.
- `n_mod_since_analyze` большое — нужен ANALYZE (планировщик использует устаревшую статистику).

Сброс статистики:
```sql
SELECT pg_stat_reset();  -- все таблицы
SELECT pg_stat_reset_single_table_counters('public.orders'::regclass);
```

---

## pg_stat_user_indexes: использование индексов

```sql
-- неиспользуемые индексы (кандидаты на удаление)
SELECT schemaname, tablename, indexname,
       idx_scan,
       pg_size_pretty(pg_relation_size(indexrelid)) AS size
FROM pg_stat_user_indexes
WHERE idx_scan = 0
  AND schemaname NOT IN ('pg_catalog', 'pg_toast')
ORDER BY pg_relation_size(indexrelid) DESC;
```

```sql
-- самые используемые индексы
SELECT schemaname, tablename, indexname,
       idx_scan, idx_tup_read, idx_tup_fetch,
       pg_size_pretty(pg_relation_size(indexrelid)) AS size
FROM pg_stat_user_indexes
ORDER BY idx_scan DESC
LIMIT 20;
```

Внимание: `idx_scan = 0` может быть из-за недавней пересборки статистики. Смотреть за 2–4 недели минимум.

---

## pg_locks: блокировки

Текущие ожидающие блокировки (главный запрос для диагностики):

```sql
SELECT
    blocked.pid AS blocked_pid,
    blocked.usename AS blocked_user,
    blocked.application_name AS blocked_app,
    now() - blocked.query_start AS wait_duration,
    blocked.query AS blocked_query,
    blocking.pid AS blocking_pid,
    blocking.usename AS blocking_user,
    blocking.query AS blocking_query
FROM pg_stat_activity AS blocked
JOIN pg_stat_activity AS blocking
    ON blocking.pid = ANY(pg_blocking_pids(blocked.pid))
WHERE blocked.wait_event_type = 'Lock'
ORDER BY wait_duration DESC;
```

Все блокировки (включая выданные):

```sql
SELECT
    pid, locktype, relation::regclass AS table,
    page, tuple, transactionid,
    mode, granted,
    left(query, 80) AS query
FROM pg_locks l
JOIN pg_stat_activity a USING (pid)
ORDER BY granted, pid;
```

---

## pg_stat_statements: топ запросов

```sql
-- топ по total_exec_time
SELECT
    left(query, 100) AS query,
    calls,
    round(total_exec_time::numeric, 1) AS total_ms,
    round(mean_exec_time::numeric, 1) AS mean_ms,
    round(stddev_exec_time::numeric, 1) AS stddev_ms,
    rows,
    round(shared_blks_hit::numeric / NULLIF(shared_blks_hit + shared_blks_read, 0) * 100, 1) AS cache_hit_pct
FROM pg_stat_statements
ORDER BY total_exec_time DESC
LIMIT 20;
```

```sql
-- запросы, читающие много с диска
SELECT
    left(query, 100) AS query,
    calls,
    shared_blks_read,
    shared_blks_hit,
    round(shared_blks_hit::numeric / NULLIF(shared_blks_hit + shared_blks_read, 0) * 100, 1) AS cache_hit_pct
FROM pg_stat_statements
WHERE shared_blks_read > 1000
ORDER BY shared_blks_read DESC
LIMIT 10;
```

---

## pg_stat_io: диагностика ввода-вывода

`pg_stat_io` (PG16+) — отдельное представление, показывающее I/O **в разрезе типа активности** (`backend_type` × `context` × `object`). До PG16 такой детализации не было: непонятно, кто именно грузит диск — обычные запросы, autovacuum, checkpointer или bgwriter.

```sql
-- кто и сколько читает с диска (reads — мимо shared_buffers)
SELECT backend_type, object, context,
       reads, round(read_time::numeric, 0) AS read_ms,
       writes, round(write_time::numeric, 0) AS write_ms,
       extends                                  -- расширения файла (рост таблиц/индексов)
FROM pg_stat_io
WHERE reads > 0 OR writes > 0
ORDER BY reads DESC;
```

Что искать:
- **`context = 'normal'`, большой `reads`** у `backend_type='client backend'` — рабочая нагрузка не помещается в `shared_buffers` / нет нужных индексов.
- **`context = 'vacuum'`** с большим I/O — тяжёлый autovacuum, кандидат на throttling-тюнинг.
- **`evictions`** растут — буферный кеш под давлением, страницы вытесняются чаще, чем хотелось бы.
- Колонка `read_time`/`write_time` заполняется только при `track_io_timing = on`.

Это первый view, по которому видно **источник** I/O, а не только его суммарный объём (`pg_stat_database`). Для пер-запросного I/O по-прежнему `pg_stat_statements` + `BUFFERS`.

---

## Диагностика bloat

Приближённая оценка bloat (без расширений):

```sql
-- table bloat
SELECT
    schemaname, tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS total_size,
    pg_size_pretty(pg_relation_size(schemaname||'.'||tablename)) AS table_size,
    n_dead_tup,
    n_live_tup,
    round(n_dead_tup::numeric / NULLIF(n_live_tup, 0) * 100, 1) AS dead_ratio_pct
FROM pg_stat_user_tables
WHERE n_live_tup > 10000
ORDER BY n_dead_tup DESC
LIMIT 20;
```

Точная оценка через `pgstattuple`:

```sql
CREATE EXTENSION IF NOT EXISTS pgstattuple;

SELECT * FROM pgstattuple('orders');
-- tuple_len: живые данные
-- dead_tuple_len: мёртвые данные
-- free_space: свободное место в страницах

-- bloat в процентах
SELECT
    table_len,
    tuple_len,
    dead_tuple_len,
    free_space,
    round((dead_tuple_len + free_space)::numeric / table_len * 100, 1) AS bloat_pct
FROM pgstattuple('orders');
```

Bloat индексов:

```sql
SELECT * FROM pgstatindex('idx_orders_user_id');
-- leaf_fragmentation: фрагментация листовых страниц
```

---

## Диагностика репликации

```sql
-- на primary: состояние реплик
SELECT
    client_addr,
    state,
    sent_lsn,
    write_lsn,
    flush_lsn,
    replay_lsn,
    pg_size_pretty(pg_wal_lsn_diff(sent_lsn, replay_lsn)) AS replay_lag,
    sync_state
FROM pg_stat_replication;
```

```sql
-- репликационные слоты с большим lag
SELECT slot_name, active, slot_type,
       pg_size_pretty(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)) AS wal_lag,
       xmin, catalog_xmin
FROM pg_replication_slots
ORDER BY pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) DESC;
```

```sql
-- на replica: lag в секундах
SELECT
    now() - pg_last_xact_replay_timestamp() AS replication_delay;
```

---

## Production checklist

### При деплое

- [ ] Нет долгих транзакций (> 1 минуты) перед migration.
- [ ] DDL использует `CONCURRENTLY`, `NOT VALID`, или batched updates.
- [ ] Новые индексы созданы через `CREATE INDEX CONCURRENTLY`.
- [ ] `EXPLAIN ANALYZE` для новых тяжёлых запросов.

### Ежедневно (автоматически)

- [ ] `n_dead_tup` не растёт аномально быстро.
- [ ] `last_autovacuum` обновляется для горячих таблиц.
- [ ] Replication lag < порога (например, 30 секунд).
- [ ] Replication slots lag в разумных пределах (< 1GB WAL).
- [ ] Нет `idle in transaction` старше 5 минут.

### Еженедельно

- [ ] `pg_stat_user_indexes` — проверить `idx_scan = 0` (удалить ненужные).
- [ ] `pg_stat_statements` — топ запросов, нет деградации.
- [ ] Размер таблиц/индексов, нет аномального роста.
- [ ] `age(datfrozenxid)` — проверить расстояние до wraparound.

### Алерты (примерные пороги)

```sql
-- 1. Долгие транзакции
SELECT count(*) FROM pg_stat_activity
WHERE xact_start < now() - interval '10 minutes'
  AND state != 'idle';

-- 2. Replication lag
SELECT extract(epoch FROM (now() - pg_last_xact_replay_timestamp())) > 30;

-- 3. Replication slot bloat
SELECT pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) > 1073741824  -- 1GB
FROM pg_replication_slots WHERE active = false;

-- 4. Transaction ID wraparound
SELECT age(datfrozenxid) > 1500000000 FROM pg_database WHERE datname = current_database();

-- 5. High dead tuple ratio
SELECT n_dead_tup::float / NULLIF(n_live_tup, 0) > 0.2
FROM pg_stat_user_tables WHERE relname = 'orders';

-- 6. Ждущие соединения (lock contention)
SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock';
```

---

## Инструменты

- [pgBadger](https://pgbadger.darold.net/) — анализатор PostgreSQL логов, строит HTML отчёты.
- [pgAdmin](https://www.pgadmin.org/) — GUI для мониторинга и управления.
- [Prometheus + postgres_exporter](https://github.com/prometheus-community/postgres_exporter) — метрики для Grafana.
- [explain.dalibo.com](https://explain.dalibo.com/) — визуализация EXPLAIN плана.
- [pgMustard](https://www.pgmustard.com/) — рекомендации по EXPLAIN плану.
- [pgstattuple](https://www.postgresql.org/docs/current/pgstattuple.html) — точная оценка bloat.
