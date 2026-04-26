# MVCC и Vacuum

## Содержание

- [Как работает MVCC](#как-работает-mvcc)
- [Видимость строк: xmin и xmax](#видимость-строк-xmin-и-xmax)
- [Dead tuples и table bloat](#dead-tuples-и-table-bloat)
- [VACUUM: виды и когда запускать](#vacuum-виды-и-когда-запускать)
- [Autovacuum: как настроить](#autovacuum-как-настроить)
- [Transaction ID Wraparound](#transaction-id-wraparound)
- [HOT updates](#hot-updates)
- [Мониторинг](#мониторинг)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

---

## Как работает MVCC

MVCC (Multi-Version Concurrency Control) — механизм, который позволяет читателям не блокировать писателей и наоборот.

Принцип: вместо изменения строки "на месте" PostgreSQL создаёт **новую версию строки** (tuple). Старая версия остаётся видимой для транзакций, которые начались до изменения.

При `UPDATE`:
1. Старая версия строки помечается как "удалённая" (`xmax = txid текущей транзакции`).
2. Новая версия строки вставляется (`xmin = txid текущей транзакции`).

При `DELETE`:
1. Строка помечается `xmax = txid` — физически не удаляется.

При `INSERT`:
1. Новая строка добавляется с `xmin = txid`.

Итог: в таблице одновременно могут существовать несколько версий одной строки.

---

## Видимость строк: xmin и xmax

Каждая строка хранит системные поля:

| Поле | Значение |
|---|---|
| `xmin` | txid транзакции, которая создала строку |
| `xmax` | txid транзакции, которая удалила/обновила строку (0 = живая) |
| `ctid` | физическое расположение: `(page, tuple_index)` |

Правила видимости (упрощённо):
- Строка видна, если `xmin` закоммичен И (`xmax` = 0 ИЛИ `xmax` не закоммичен).
- Для snapshot isolation: `xmin` ≤ snapshot.xmin И `xmax` > snapshot.xmin.

```sql
-- посмотреть системные поля строки
SELECT xmin, xmax, ctid, id, email FROM users LIMIT 5;
```

---

## Dead tuples и table bloat

После `UPDATE` или `DELETE` старые версии строк ("мёртвые кортежи") остаются на страницах таблицы. Они занимают место и замедляют sequential scan.

Следствия накопления dead tuples:
- **Table bloat** — файл таблицы на диске растёт без реального роста данных.
- **Index bloat** — индексы тоже содержат указатели на dead tuples.
- **Деградация seq scan** — читаются лишние страницы.

Причины накопления:
- Долгие транзакции (vacuum не может зачистить, пока транзакция не закончится).
- Репликационные слоты без активных подписчиков (удерживают `xmin`).
- Autovacuum отключён или неправильно настроен.
- Высокая скорость `UPDATE`/`DELETE` превышает скорость vacuum.

---

## VACUUM: виды и когда запускать

### `VACUUM` (обычный)

Помечает dead tuples как свободное пространство для повторного использования. **Не возвращает** место ОС. Не блокирует чтение/запись.

```sql
VACUUM users;
VACUUM VERBOSE users;  -- подробный вывод
```

### `VACUUM ANALYZE`

Vacuum + обновление статистики планировщика. Запускать после массовых INSERT/UPDATE.

```sql
VACUUM ANALYZE users;
```

### `VACUUM FULL`

Полная перестройка таблицы — возвращает место ОС. **Требует exclusive lock** — блокирует всю таблицу на время работы. Использовать только в maintenance window.

```sql
VACUUM FULL users;  -- блокирует таблицу!
```

Альтернатива без блокировки: расширение `pg_repack`.

### `ANALYZE`

Только обновляет статистику — без зачистки. Нужен после bulk load.

```sql
ANALYZE users;
```

---

## Autovacuum: как настроить

Autovacuum запускается автоматически, когда число dead tuples превышает порог:

```
autovacuum_vacuum_threshold + autovacuum_vacuum_scale_factor * n_live_tup
```

По умолчанию: 50 + 0.2 * размер_таблицы. Для большой таблицы (10M строк) это 2 000 050 — очень много.

Настройка на уровне таблицы (без перезагрузки PostgreSQL):

```sql
ALTER TABLE orders SET (
    autovacuum_vacuum_scale_factor = 0.01,   -- 1% вместо 20%
    autovacuum_vacuum_threshold = 1000,
    autovacuum_analyze_scale_factor = 0.005,
    autovacuum_vacuum_cost_delay = 2         -- ms, снизить throttling
);
```

Глобальные параметры в `postgresql.conf`:

```
autovacuum_max_workers = 5          # параллельных воркеров (default 3)
autovacuum_vacuum_cost_delay = 2ms  # throttling задержка
autovacuum_vacuum_cost_limit = 400  # budget за один цикл (default 200)
```

Проверить активность autovacuum:

```sql
SELECT schemaname, relname, last_vacuum, last_autovacuum,
       last_analyze, last_autoanalyze,
       n_dead_tup, n_live_tup,
       autovacuum_count
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC;
```

---

## Transaction ID Wraparound

PostgreSQL использует 32-битный `txid` (transaction ID). Максимум ~2.1 млрд транзакций. Когда счётчик доходит до предела, происходит wraparound.

Если PostgreSQL не успевает выполнить VACUUM FREEZE (заморозку старых строк), старые строки становятся "невидимыми" — это называется **transaction ID wraparound catastrophe**.

Признак приближения: warning в логах за ~11 млн транзакций до предела.

```sql
-- проверить расстояние до wraparound
SELECT datname,
       age(datfrozenxid) AS xid_age,
       2147483647 - age(datfrozenxid) AS remaining
FROM pg_database
ORDER BY xid_age DESC;
```

Если `age` близко к 2 млрд — нужен экстренный `VACUUM FREEZE`.

```sql
VACUUM FREEZE VERBOSE users;
```

Параметры заморозки:
- `vacuum_freeze_min_age` (default 50M) — возраст транзакции, с которого начинается заморозка.
- `autovacuum_freeze_max_age` (default 200M) — принудительно запускает autovacuum для заморозки.

---

## HOT updates

HOT (Heap Only Tuple) — оптимизация: если обновляемые поля **не входят в индексы** и новая версия строки помещается на той же странице, PostgreSQL создаёт chain в heap без обновления индекса.

Результат: быстрее UPDATE, меньше index bloat.

Условие: нужно свободное место на странице — влияет `fillfactor` (default 100%).

```sql
-- оставить 20% страницы свободным для HOT updates
ALTER TABLE users SET (fillfactor = 80);
```

Проверить HOT:
```sql
SELECT n_tup_hot_upd, n_tup_upd FROM pg_stat_user_tables WHERE relname = 'users';
-- высокое n_tup_hot_upd / n_tup_upd — HOT работает
```

---

## Мониторинг

Долгие транзакции (основная причина bloat):

```sql
SELECT pid,
       now() - xact_start AS duration,
       state,
       left(query, 100) AS query
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
  AND state != 'idle'
ORDER BY duration DESC;
```

Таблицы с большим bloat:

```sql
SELECT relname,
       n_dead_tup,
       n_live_tup,
       round(n_dead_tup::numeric / NULLIF(n_live_tup, 0) * 100, 1) AS dead_ratio_pct,
       last_autovacuum
FROM pg_stat_user_tables
WHERE n_live_tup > 1000
ORDER BY dead_ratio_pct DESC NULLS LAST;
```

Репликационные слоты (могут удерживать xmin):

```sql
SELECT slot_name, active, xmin, catalog_xmin,
       pg_size_pretty(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)) AS lag
FROM pg_replication_slots;
```

---

## Типичные ошибки

- Долгие транзакции в OLTP коде — vacuum не может зачистить мёртвые строки.
- Оставлять репликационные слоты без подписчиков — удерживают xmin и копят WAL.
- Запускать `VACUUM FULL` в production без maintenance window — exclusive lock.
- Игнорировать рост `n_dead_tup` пока не стало заметно в latency.
- Не снижать `autovacuum_vacuum_scale_factor` для больших таблиц.

---

## Interview-ready answer

MVCC в PostgreSQL: каждый UPDATE создаёт новую физическую версию строки, старая остаётся с `xmax = txid` обновляющей транзакции. Читатели видят снимок данных на момент начала своей транзакции — поэтому read не блокирует write. Следствие: накапливаются dead tuples, которые зачищает VACUUM. Autovacuum запускается по порогу (по умолчанию 20% таблицы), что слишком много для больших таблиц — нужно снижать `autovacuum_vacuum_scale_factor`. Главная опасность: длинная транзакция держит `xmin snapshot`, vacuum не может зачищать, таблица раздувается (table bloat). Ещё одна опасность: transaction ID wraparound при 2 млрд txid — мониторить `age(datfrozenxid)`. HOT updates ускоряют UPDATE когда индексированные поля не меняются — помогает `fillfactor = 80`.
