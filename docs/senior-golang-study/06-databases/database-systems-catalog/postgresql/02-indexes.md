# Индексы в PostgreSQL

Схема типов индексов: [pg_indexes.png](./02a-pg_indexes.png)

## Содержание

- [Как работает B-tree индекс](#как-работает-b-tree-индекс)
- [Типы индексов](#типы-индексов)
- [Partial index](#partial-index)
- [Expression index](#expression-index)
- [Covering index (INCLUDE)](#covering-index-include)
- [Multi-column индексы](#multi-column-индексы)
- [Когда индекс не используется](#когда-индекс-не-используется)
- [Index bloat](#index-bloat)
- [Стратегия выбора индексов](#стратегия-выбора-индексов)
- [Concurrent index build](#concurrent-index-build)
- [Interview-ready answer](#interview-ready-answer)

---

## Как работает B-tree индекс

B-tree (Balanced Tree) — дефолтный тип. Хранит отсортированные значения в сбалансированном дереве страниц по 8KB.

Структура:
- **Root page** — одна страница верхнего уровня.
- **Internal pages** — промежуточные, хранят диапазоны ключей и указатели на дочерние страницы.
- **Leaf pages** — нижний уровень, хранят пары `(key_value, ctid)`, отсортированные по ключу.

Операции:
- Поиск по равенству: O(log n).
- Range scan: O(log n + k), где k — число найденных строк.
- ORDER BY без sort: индекс уже отсортирован, scan идёт по leaf pages последовательно.

Depth типичного B-tree:
- 1M строк → глубина ~3.
- 1B строк → глубина ~5.

```sql
-- создание B-tree (явно, default и так B-tree)
CREATE INDEX idx_users_email ON users USING btree (email);

-- DESC индекс для ORDER BY created_at DESC LIMIT N
CREATE INDEX idx_orders_created_desc ON orders (created_at DESC);

-- NULLS LAST/FIRST
CREATE INDEX idx_tasks_deadline ON tasks (deadline NULLS LAST);
```

---

## Типы индексов

### Hash

Только equality (`=`). Быстрее B-tree для equality, но не поддерживает range, ORDER BY, LIKE. Редко нужен.

```sql
CREATE INDEX idx_sessions_token ON sessions USING hash (token);
```

### GIN (Generalized Inverted Index)

Индексирует элементы внутри составных значений: JSONB, массивы, tsvector.

```sql
-- JSONB поиск по ключу/значению
CREATE INDEX idx_products_attrs ON products USING GIN (attributes);
SELECT * FROM products WHERE attributes @> '{"color": "red"}';

-- массив содержит элемент
CREATE INDEX idx_users_roles ON users USING GIN (roles);
SELECT * FROM users WHERE roles @> ARRAY['admin'];

-- полнотекстовый поиск
CREATE INDEX idx_articles_fts ON articles USING GIN (to_tsvector('russian', body));
SELECT * FROM articles WHERE to_tsvector('russian', body) @@ to_tsquery('russian', 'PostgreSQL');
```

GIN медленнее при write (нужно обновить инвертированный список), зато быстрый при read.

### GiST (Generalized Search Tree)

Для геометрических данных (PostGIS), диапазонов (`tsrange`, `int4range`), similarity search (`pg_trgm`).

```sql
-- диапазоны: найти события, пересекающиеся с периодом
CREATE INDEX idx_events_period ON events USING GiST (period);
SELECT * FROM events WHERE period && '[2024-01-01, 2024-12-31]'::tsrange;

-- trigram similarity (расширение pg_trgm)
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_products_name_trgm ON products USING GiST (name gist_trgm_ops);
SELECT * FROM products WHERE name % 'Postgresq';  -- ~похожие названия
```

### BRIN (Block Range Index)

Хранит min/max значения для блоков страниц. Очень маленький, полезен для **натурально отсортированных** больших таблиц (события по времени, append-only логи).

```sql
-- для таблицы логов с монотонно растущим created_at
CREATE INDEX idx_logs_created_brin ON logs USING BRIN (created_at);
```

BRIN не подходит, если данные вставляются в случайном порядке — большие min/max диапазоны делают его бесполезным.

### SP-GiST (Space-Partitioned GiST)

Для непересекающихся пространств: IP-адреса (`inet`), точки, телефонные номера. Нишевый.

---

## Partial index

Индексирует только строки, удовлетворяющие условию. Меньше размер, быстрее build и scan.

```sql
-- индексировать только активные заказы (minority)
CREATE INDEX idx_orders_active ON orders (created_at)
WHERE status = 'pending';

-- запрос ДОЛЖЕН содержать то же условие, чтобы планировщик выбрал индекс
SELECT id, created_at FROM orders
WHERE status = 'pending' AND created_at > now() - interval '7 days';

-- partial unique: каждый пользователь может иметь один активный профиль
CREATE UNIQUE INDEX idx_profiles_active_user
ON profiles (user_id)
WHERE deleted_at IS NULL;
```

---

## Expression index

Индексирует результат выражения. Запрос должен использовать идентичное выражение.

```sql
-- case-insensitive поиск
CREATE INDEX idx_users_email_lower ON users (LOWER(email));
SELECT * FROM users WHERE LOWER(email) = 'user@example.com';

-- индекс по дате из timestamp
CREATE INDEX idx_orders_date ON orders (DATE(created_at));
SELECT * FROM orders WHERE DATE(created_at) = '2024-01-15';

-- индекс по JSONB полю
CREATE INDEX idx_users_country ON users ((metadata->>'country'));
SELECT * FROM users WHERE metadata->>'country' = 'RU';
```

---

## Covering index (INCLUDE)

`INCLUDE` добавляет дополнительные столбцы в leaf pages индекса без включения их в ключ сортировки. Позволяет выполнить **Index Only Scan** — без чтения heap.

```sql
-- запрос: SELECT email, status FROM users WHERE email = $1
-- обычный индекс требует heap fetch для status
CREATE INDEX idx_users_email ON users (email);

-- covering index: status включён в leaf, heap не нужен
CREATE INDEX idx_users_email_covering ON users (email) INCLUDE (status);
```

EXPLAIN покажет `Index Only Scan` вместо `Index Scan` — значит heap не читается.

```sql
EXPLAIN SELECT email, status FROM users WHERE email = 'user@example.com';
-- Index Only Scan using idx_users_email_covering on users
```

---

## Multi-column индексы

Порядок столбцов важен:

```sql
CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);
```

Этот индекс используется для:
- `WHERE user_id = $1` (prefix match).
- `WHERE user_id = $1 ORDER BY created_at DESC` (prefix + sort).
- `WHERE user_id = $1 AND created_at > $2`.

Не используется для:
- `WHERE created_at > $1` (нет prefix по user_id).

Правило: первым ставить поле с наибольшей selectivity или поле equality-предиката.

---

## Когда индекс не используется

Планировщик может игнорировать индекс:
- **Маленькая таблица** — seq scan дешевле.
- **Низкая selectivity** — индекс по boolean с 90% true.
- **Функция над indexed column**: `WHERE UPPER(email) = ...` не использует индекс на `email` (нужен expression index).
- **Implicit cast**: `WHERE id = '123'` если `id` integer, может не использовать индекс.
- **LIKE с wildcard в начале**: `WHERE name LIKE '%foo'` — не использует B-tree.
- **Устаревшая статистика** — планировщик оценивает неправильно. Решение: `ANALYZE table`.

Проверить используется ли индекс:

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM users WHERE email = 'x@example.com';
```

Принудительно отключить seq scan для проверки:
```sql
SET enable_seqscan = off;
EXPLAIN SELECT * FROM users WHERE email = 'x@example.com';
SET enable_seqscan = on;
```

---

## Index bloat

Индексы накапливают bloat аналогично таблицам — dead versions остаются в leaf pages.

Диагностика (упрощённый запрос):

```sql
SELECT schemaname, tablename, indexname,
       pg_size_pretty(pg_relation_size(indexrelid)) AS index_size,
       idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
ORDER BY pg_relation_size(indexrelid) DESC;
```

Для точной оценки bloat использовать расширение `pgstattuple`:

```sql
CREATE EXTENSION pgstattuple;
SELECT * FROM pgstattuple('idx_users_email');
-- free_percent показывает долю "пустого" места
```

Пересборка индекса без блокировки:

```sql
REINDEX INDEX CONCURRENTLY idx_users_email;
```

---

## Стратегия выбора индексов

1. Начинать с EXPLAIN ANALYZE реальных медленных запросов — не угадывать.
2. Индексировать поля в WHERE, JOIN ON, ORDER BY, GROUP BY если selectivity высокая.
3. Partial index когда условие фильтрует большинство строк.
4. INCLUDE для Index Only Scan на часто читаемых полях.
5. GIN для JSONB и массивов.
6. BRIN для append-only таблиц событий/логов с монотонным timestamp.
7. Не создавать индекс на каждое поле — overhead на write и vacuum.
8. Удалять неиспользуемые индексы (idx_scan = 0 в pg_stat_user_indexes за длительный период).

```sql
-- найти неиспользуемые индексы
SELECT schemaname, tablename, indexname,
       idx_scan,
       pg_size_pretty(pg_relation_size(indexrelid)) AS size
FROM pg_stat_user_indexes
WHERE idx_scan = 0
  AND schemaname NOT IN ('pg_catalog', 'pg_toast')
ORDER BY pg_relation_size(indexrelid) DESC;
```

---

## Concurrent index build

`CREATE INDEX` по умолчанию блокирует запись на всё время build. Для production использовать `CONCURRENTLY`.

```sql
-- не блокирует DML, но дольше и требует больше ресурсов
CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);

-- пересборка
REINDEX INDEX CONCURRENTLY idx_orders_status;
```

Ограничения CONCURRENTLY:
- Нельзя внутри транзакции.
- При падении оставляет "INVALID" индекс — нужно дропнуть и пересоздать.

```sql
-- найти сломанные индексы
SELECT indexname FROM pg_indexes
JOIN pg_class ON pg_class.relname = pg_indexes.indexname
WHERE pg_class.relkind = 'i'
  AND NOT indisvalid
FROM pg_index JOIN pg_class ON pg_class.oid = pg_index.indexrelid;
```

---

## Interview-ready answer

Дефолтный индекс в PostgreSQL — B-tree: сбалансированное дерево, поддерживает equality, range, ORDER BY, O(log n) поиск. Для JSONB и массивов — GIN (инвертированный индекс по элементам). Для геометрии и диапазонов — GiST. Для append-only таблиц с монотонным timestamp — BRIN (хранит min/max по блокам, очень маленький). Partial index уменьшает размер индексируя только нужное подмножество строк. Expression index индексирует результат функции. INCLUDE создаёт covering index для Index Only Scan без чтения heap. Multi-column: первым идёт поле equality или наибольшей selectivity. В production — только `CREATE INDEX CONCURRENTLY`. Неиспользуемые индексы удалять (overhead на write), смотреть `idx_scan = 0` в `pg_stat_user_indexes`.
