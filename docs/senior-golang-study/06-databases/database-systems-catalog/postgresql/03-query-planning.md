# Query Planning и EXPLAIN

## Содержание

- [Как работает планировщик](#как-работает-планировщик)
- [Статистика: pg_statistic и ANALYZE](#статистика-pg_statistic-и-analyze)
- [EXPLAIN: анатомия вывода](#explain-анатомия-вывода)
- [EXPLAIN ANALYZE и EXPLAIN BUFFERS](#explain-analyze-и-explain-buffers)
- [Типы сканов](#типы-сканов)
- [Стратегии соединения (JOIN)](#стратегии-соединения-join)
- [Типичные проблемы планов](#типичные-проблемы-планов)
- [Управление планировщиком](#управление-планировщиком)
- [Interview-ready answer](#interview-ready-answer)

---

## Как работает планировщик

PostgreSQL использует **cost-based query planner**. При выполнении запроса:

1. **Parser** — разбирает SQL в AST.
2. **Analyzer** — проверяет семантику, резолвит типы.
3. **Rewriter** — применяет правила (views, row security).
4. **Planner/Optimizer** — перебирает возможные планы, выбирает с минимальной ожидаемой стоимостью.
5. **Executor** — выполняет выбранный план.

Стоимость (cost) — абстрактная единица, основанная на:
- числе страниц для чтения (sequential read стоит 1.0, random read — 4.0 по умолчанию);
- числе обрабатываемых строк;
- стоимости CPU-операций (cpu_tuple_cost, cpu_operator_cost).

---

## Статистика: pg_statistic и ANALYZE

Планировщик принимает решения на основе **статистики**, собранной командой `ANALYZE`.

Что хранится:
- Число различных значений (n_distinct).
- Гистограмма распределения значений.
- Наиболее частые значения (MCV — Most Common Values).
- Корреляция с физическим порядком в таблице.

```sql
-- посмотреть статистику по столбцу
SELECT attname, n_distinct, correlation
FROM pg_stats
WHERE tablename = 'orders' AND attname = 'status';

-- MCV: наиболее частые значения
SELECT tablename, attname, most_common_vals, most_common_freqs
FROM pg_stats
WHERE tablename = 'orders' AND attname = 'status';
```

Параметр `default_statistics_target` (default 100) — сколько значений хранить в статистике. Для плохо оцениваемых столбцов увеличить:

```sql
ALTER TABLE orders ALTER COLUMN status SET STATISTICS 500;
ANALYZE orders;
```

---

## EXPLAIN: анатомия вывода

```sql
EXPLAIN SELECT id, email FROM users WHERE status = 'active' ORDER BY created_at DESC LIMIT 50;
```

Пример вывода:
```
Limit  (cost=0.43..10.28 rows=50 width=24)
  ->  Index Scan Backward using idx_users_created on users
        (cost=0.43..2847.43 rows=14482 width=24)
        Filter: ((status)::text = 'active'::text)
```

Как читать:
- `cost=start..total` — стоимость до первой строки и полная стоимость.
- `rows=N` — оценочное число строк.
- `width=N` — средняя ширина строки в байтах.
- `Filter:` — условие применяется ПОСЛЕ чтения (не индексный предикат).
- `Index Cond:` — условие используется индексом.

Дерево читается **снизу вверх** — нижние узлы выполняются первыми.

---

## EXPLAIN ANALYZE и EXPLAIN BUFFERS

`EXPLAIN ANALYZE` **реально выполняет** запрос и показывает фактические метрики:

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT id, email FROM users WHERE status = 'active' ORDER BY created_at DESC LIMIT 50;
```

Пример с ANALYZE:
```
Index Scan Backward using idx_users_created on users
  (cost=0.43..2847.43 rows=14482 width=24)
  (actual time=0.082..1.432 rows=50 loops=1)
  Filter: ((status)::text = 'active'::text)
  Rows Removed by Filter: 312
Buffers: shared hit=58 read=12
Planning Time: 0.312 ms
Execution Time: 1.521 ms
```

Что смотреть:
- `actual rows` vs `rows` (estimate) — большое расхождение = плохая статистика.
- `loops` — сколько раз выполнялся узел (для Nested Loop).
- `Rows Removed by Filter` — сколько строк отброшено после чтения = нет подходящего индекса.
- `Buffers: shared hit` — из cache, `read` — с диска.
- `Planning Time` vs `Execution Time`.

Для DELETE/UPDATE с риском использовать `EXPLAIN` без `ANALYZE`:
```sql
EXPLAIN UPDATE orders SET status = 'done' WHERE id = $1;
```

---

## Типы сканов

### Sequential Scan (Seq Scan)

Читает всю таблицу страница за страницей. Эффективен когда нужна большая доля строк или таблица маленькая.

```
Seq Scan on users  (cost=0.00..1549.00 rows=100000 width=24)
```

### Index Scan

Читает индекс, потом для каждой строки идёт в heap за дополнительными полями. Random I/O.

```
Index Scan using idx_users_email on users  (cost=0.43..8.45 rows=1 width=24)
  Index Cond: (email = 'x@example.com')
```

### Index Only Scan

Читает только индекс, heap не нужен (все нужные данные есть в covering index). Самый быстрый для точечных запросов.

```
Index Only Scan using idx_users_email_covering on users
  Heap Fetches: 0
```

### Bitmap Index Scan + Bitmap Heap Scan

Используется когда нужно много строк через индекс. Строит bitmap страниц из индекса, потом читает страницы heap в порядке физического расположения.

```
Bitmap Heap Scan on orders
  Recheck Cond: (status = 'pending')
  ->  Bitmap Index Scan on idx_orders_status
        Index Cond: (status = 'pending')
```

Эффективнее Index Scan при большом числе строк (снижает random I/O).

---

## Стратегии соединения (JOIN)

### Nested Loop

Для каждой строки outer таблицы ищет совпадения в inner. O(N * M) в худшем случае. Эффективен с индексом на inner, для маленьких наборов.

```
Nested Loop
  ->  Seq Scan on orders (small table)
  ->  Index Scan on users using idx_users_id
        Index Cond: (users.id = orders.user_id)
```

### Hash Join

Строит hash table из меньшей таблицы, проходит по большей. O(N + M). Эффективен для средних и больших таблиц без подходящего индекса.

```
Hash Join
  Hash Cond: (orders.user_id = users.id)
  ->  Seq Scan on orders
  ->  Hash
        ->  Seq Scan on users
```

### Merge Join

Оба входа отсортированы (или индексы), соединяет за один проход. O(N + M). Эффективен когда оба набора уже отсортированы по ключу join.

```
Merge Join
  Merge Cond: (orders.user_id = users.id)
  ->  Index Scan using idx_orders_user on orders
  ->  Index Scan using idx_users_id on users
```

---

## Типичные проблемы планов

**Плохая оценка rows:**
```
rows=14482 (actual rows=1)
```
Причина: устаревшая или недостаточная статистика. Решение: `ANALYZE`, увеличить `statistics_target`.

**Filter вместо Index Cond:**
```
Filter: (status = 'active')
Rows Removed by Filter: 99500
```
Причина: нет подходящего индекса. Решение: добавить индекс или использовать partial/composite.

**Seq Scan вместо Index Scan:**
Планировщик решил, что seq scan дешевле. Варианты:
- Таблица маленькая — норма.
- Низкая selectivity — норма.
- Устаревшая статистика — `ANALYZE`.
- Неправильный `random_page_cost` для SSD — снизить до 1.1.

**Hash Join вместо Index Scan (unexpected):**
Может означать что `work_mem` слишком мал для hash table и идёт spill на диск:
```sql
SET work_mem = '64MB';
EXPLAIN ANALYZE <query>;
```

**Плохой план для параметризованных запросов:**
PostgreSQL кеширует план после 5 выполнений. Если распределение данных неоднородно — план может быть плохим. Решение: `SET plan_cache_mode = force_custom_plan;` (только для диагностики).

---

## Управление планировщиком

Включить/выключить типы планов (для диагностики, не для production):

```sql
SET enable_seqscan = off;
SET enable_hashjoin = off;
SET enable_nestloop = off;
```

Параметры, влияющие на выбор плана:

| Параметр | Default | Описание |
|---|---|---|
| `random_page_cost` | 4.0 | Стоимость random read. Для SSD: 1.1–2.0 |
| `seq_page_cost` | 1.0 | Стоимость seq read |
| `effective_cache_size` | 4GB | Оценка OS page cache для планировщика |
| `work_mem` | 4MB | Память для sort/hash per operation |
| `cpu_tuple_cost` | 0.01 | Стоимость обработки строки |

Рекомендации для SSD:
```sql
-- postgresql.conf
random_page_cost = 1.1
effective_cache_size = 12GB  # ~75% RAM
```

---

## Interview-ready answer

Планировщик PostgreSQL — cost-based: перебирает возможные планы и выбирает с наименьшей расчётной стоимостью. Стоимость зависит от статистики (гистограммы, MCV, n_distinct), собранной ANALYZE. Плохая статистика = плохой план — проявляется как большое расхождение `rows` (estimate) vs actual в EXPLAIN ANALYZE. Типы сканов: Seq Scan (вся таблица), Index Scan (random I/O через индекс), Index Only Scan (только индекс, heap не нужен), Bitmap Scan (bitmap страниц, эффективен для диапазонов). Стратегии JOIN: Nested Loop (inner с индексом, маленькие наборы), Hash Join (средние таблицы, hash table в памяти), Merge Join (оба набора отсортированы). Для SSD снижать `random_page_cost` до 1.1 — иначе планировщик занижает стоимость Index Scan. Диагностика: `EXPLAIN (ANALYZE, BUFFERS)` — смотреть actual rows, Buffers hit/read, Filter vs Index Cond.
