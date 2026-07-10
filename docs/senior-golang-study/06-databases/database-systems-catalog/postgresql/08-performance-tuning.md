# Performance Tuning

## Содержание

- [Ключевые параметры памяти](#ключевые-параметры-памяти)
- [WAL и checkpoint параметры](#wal-и-checkpoint-параметры)
- [Autovacuum tuning](#autovacuum-tuning)
- [Параметры планировщика](#параметры-планировщика)
- [pg_stat_statements](#pg_stat_statements)
- [Slow query log](#slow-query-log)
- [Профилировка конкретного запроса](#профилировка-конкретного-запроса)
- [Рекомендации для разных сценариев](#рекомендации-для-разных-сценариев)
- [Interview-ready answer](#interview-ready-answer)

---

## Ключевые параметры памяти

### shared_buffers

Размер shared memory buffer PostgreSQL для кеширования страниц.

```
shared_buffers = 25%–40% RAM
```

- 8GB RAM → 2–3GB
- 32GB RAM → 8–12GB
- Выше 40% — не улучшает (ОС page cache тоже важен)

### work_mem

Память для **одной операции** сортировки или hash. Множится на число одновременных операций.

```
work_mem = 4MB–64MB
```

Риск: при 200 соединениях × 5 операций × 64MB = 64GB. Не завышать глобально.

Для конкретного тяжёлого запроса:
```sql
SET LOCAL work_mem = '256MB';
SELECT * FROM large_table ORDER BY complex_expr;
```

Признак нехватки: `Sort Method: external merge Disk` в EXPLAIN ANALYZE.

### maintenance_work_mem

Для VACUUM, CREATE INDEX, ALTER TABLE ADD COLUMN.

```
maintenance_work_mem = 512MB–2GB
```

Влияет на скорость `CREATE INDEX` и `VACUUM` — можно безопасно увеличивать.

### effective_cache_size

Подсказка планировщику о размере OS page cache. Не выделяет реальную память.

```
effective_cache_size = 75% RAM
```

Влияет на выбор Index Scan vs Seq Scan (оценка стоимости).

### temp_buffers

Память для временных таблиц per session.
```
temp_buffers = 64MB  # default 8MB, повысить если используете TEMP TABLE
```

### Параметры соединений

```
max_connections = 100–200    # каждое соединение ~5–10MB overhead
```

Реальная нагрузка через PgBouncer, не через max_connections.

---

## WAL и checkpoint параметры

### max_wal_size / checkpoint_completion_target

```
max_wal_size = 2GB–8GB              # при больших bulk insert повысить
checkpoint_completion_target = 0.9  # растягивает checkpoint на 90% интервала
checkpoint_timeout = 15min          # частота checkpoint
```

Признак проблемы: в логах `checkpoint occurring too frequently` — нужно увеличить `max_wal_size`.

### wal_compression

```
wal_compression = lz4  # или zstd (PG 15+)
```

Снижает объём WAL без заметного CPU overhead. Особенно полезно при репликации.

### synchronous_commit

```
synchronous_commit = off  # для некритичных write (logs, analytics)
```

Ускоряет INSERT/UPDATE в ~2-3x для некритичных данных. Риск потери последних 200ms транзакций при crash.

### wal_buffers

```
wal_buffers = 64MB  # default auto (~3% shared_buffers, min 32kB)
```

---

## Autovacuum tuning

Параметры для высоко-нагруженных таблиц (в `postgresql.conf` или `ALTER TABLE SET`):

```
autovacuum_max_workers = 5           # default 3
autovacuum_vacuum_cost_delay = 2ms   # default 2ms
autovacuum_vacuum_cost_limit = 400   # default 200, throttling budget
```

Для конкретных больших таблиц (без перезагрузки):

```sql
-- агрессивный vacuum для таблицы с высокой частотой UPDATE
ALTER TABLE orders SET (
    autovacuum_vacuum_scale_factor = 0.01,    -- 1% вместо 20%
    autovacuum_vacuum_threshold = 100,
    autovacuum_vacuum_cost_delay = 0,          -- без throttling
    autovacuum_vacuum_cost_limit = 1000
);
```

---

## Параметры планировщика

```
random_page_cost = 1.1      # для SSD (default 4.0 для HDD)
seq_page_cost = 1.0
effective_io_concurrency = 200  # для SSD (default 1)
```

Для SSD-серверов `random_page_cost = 1.1` критически важен — без этого планировщик переоценивает стоимость Index Scan и выбирает Seq Scan.

```
default_statistics_target = 100  # увеличить до 200-500 для больших таблиц с неравномерным распределением
```

```
enable_partitionwise_join = on   # оптимизация JOIN партиционированных таблиц
enable_partitionwise_aggregate = on
```

---

## pg_stat_statements

Расширение для сбора статистики по запросам. Нормализует параметры: `WHERE id = 1` и `WHERE id = 2` → одна запись.

Включение:

```
# postgresql.conf
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.max = 10000
pg_stat_statements.track = all
```

```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

Топ запросов по суммарному времени:

```sql
SELECT
    left(query, 80) AS query,
    calls,
    round(total_exec_time::numeric, 2) AS total_ms,
    round(mean_exec_time::numeric, 2) AS mean_ms,
    round(stddev_exec_time::numeric, 2) AS stddev_ms,
    rows
FROM pg_stat_statements
ORDER BY total_exec_time DESC
LIMIT 20;
```

Запросы с высоким stddev (нестабильные):

```sql
SELECT left(query, 80), calls, mean_exec_time, stddev_exec_time
FROM pg_stat_statements
WHERE calls > 100
ORDER BY stddev_exec_time DESC
LIMIT 10;
```

Запросы с наибольшим числом блоков с диска (кандидаты для кеширования/индексов):

```sql
SELECT left(query, 80), calls,
       blk_read_time + blk_write_time AS io_time_ms,
       shared_blks_read, shared_blks_hit,
       round(shared_blks_hit::numeric / NULLIF(shared_blks_hit + shared_blks_read, 0) * 100, 1) AS hit_rate_pct
FROM pg_stat_statements
WHERE calls > 10
ORDER BY (blk_read_time + blk_write_time) DESC
LIMIT 20;
```

Сброс статистики:
```sql
SELECT pg_stat_statements_reset();
```

---

## Slow query log

```
# postgresql.conf
log_min_duration_statement = 1000   # логировать запросы > 1 секунды
log_min_duration_statement = 0      # все запросы (только для временной диагностики)

log_line_prefix = '%t [%p]: [%l-1] user=%u,db=%d,app=%a,client=%h '
log_checkpoints = on
log_connections = on
log_disconnections = on
log_lock_waits = on
log_temp_files = 0   # логировать создание temp файлов (sort/hash spill to disk)
```

Применить без перезапуска:
```sql
SELECT pg_reload_conf();
-- или
ALTER SYSTEM SET log_min_duration_statement = 1000;
SELECT pg_reload_conf();
```

---

## Профилировка конкретного запроса

Полный набор опций EXPLAIN:

```sql
EXPLAIN (
    ANALYZE,      -- реально выполнить
    BUFFERS,      -- показать shared/local hits and reads
    VERBOSE,      -- дополнительные детали
    SETTINGS,     -- показать изменённые параметры
    WAL,          -- WAL usage
    FORMAT JSON   -- машиночитаемый формат (удобно для pgMustard, explain.dalibo.com)
)
SELECT ...;
```

Визуализация: скопировать JSON в [explain.dalibo.com](https://explain.dalibo.com/) — удобно для сложных планов.

Полезные настройки для профилировки:

```sql
-- включить timing I/O
SET track_io_timing = on;

-- посмотреть работу буферов
EXPLAIN (ANALYZE, BUFFERS)
SELECT ...;
-- Buffers: shared hit=1000 read=50
-- hit — из cache, read — с диска (плохо)
```

Сессионный `work_mem` для проверки влияния:

```sql
SET work_mem = '256MB';
EXPLAIN ANALYZE SELECT * FROM big_table ORDER BY col1, col2;
-- сравнить Sort Method: quicksort (memory) vs external merge (disk)
SET work_mem = DEFAULT;
```

---

## Рекомендации для разных сценариев

### OLTP (много коротких транзакций)

```
shared_buffers = 25% RAM
work_mem = 4MB–16MB (осторожно при многих соединениях)
max_connections = 100-200 + PgBouncer
random_page_cost = 1.1 (SSD)
synchronous_commit = on (для финансовых данных)
```

### Аналитические запросы (OLAP или mixed)

```
work_mem = 512MB–2GB (выдавать per-query через SET)
max_parallel_workers_per_gather = 4
max_parallel_workers = 8
enable_partitionwise_join = on
```

### Bulk load (ETL, миграции)

```sql
-- на время загрузки
SET synchronous_commit = off;
SET work_mem = '1GB';
ALTER TABLE t SET UNLOGGED;  -- ускоряет в 3-10x (теряем WAL)

COPY t FROM '/tmp/data.csv';

ALTER TABLE t SET LOGGED;  -- вернуть
```

### Read-heavy workload

```
shared_buffers = 40% RAM
effective_cache_size = 75% RAM
random_page_cost = 1.1
```

---

## Interview-ready answer

**1. Ключевые параметры памяти?**

- `shared_buffers` — 25–40% RAM (кеш страниц), `work_mem` — память на одну sort/hash-операцию (не завышать глобально, лучше per-query через `SET LOCAL`), `effective_cache_size` — ~75% RAM (подсказка планировщику об ОС-кеше).

**2. Что критично настроить для SSD?**

- `random_page_cost = 1.1` (default 4.0 для HDD) — иначе планировщик завышает стоимость Index Scan и уходит в Seq Scan; плюс повышенный `effective_io_concurrency`.

**3. Зачем pg_stat_statements?**

- Агрегированная статистика по нормализованным запросам: топ по `total_exec_time`, высокий stddev = нестабильный план, низкий cache hit = читает с диска.

**4. Как ловить медленные запросы?**

- `log_min_duration_statement = 1000` (логировать >1с) + `EXPLAIN (ANALYZE, BUFFERS)` для конкретного запроса.

**5. Какие признаки проблем в плане?**

- `external merge Disk` в Sort → мало `work_mem`; `Seq Scan` на большой таблице → нет индекса или неверный `random_page_cost`; большой `Rows Removed by Filter` → нет покрывающего индекса.

**6. Как ускорить bulk load?**

- `SET synchronous_commit = off`, повышенный `work_mem`, `UNLOGGED`-таблица на время загрузки, COPY вместо INSERT.
