# Indexes And Query Plans

Индекс нужен, чтобы база могла быстрее находить строки под конкретный query pattern.

## Содержание

- [Самая простая идея](#самая-простая-идея)
- [B-tree index](#b-tree-index)
- [Composite index](#composite-index)
- [Partial index](#partial-index)
- [Covering index](#covering-index)
- [Почему индекс не всегда помогает](#почему-индекс-не-всегда-помогает)
- [EXPLAIN](#explain)
- [Цена индексов](#цена-индексов)
- [Interview-ready summary](#interview-ready-summary)

## Самая простая идея

Без индекса БД часто вынуждена делать sequential scan:

```text
пройти много строк и проверить условие
```

С индексом БД может быстрее найти нужный диапазон или конкретные строки.

## B-tree index

Самый частый индекс в PostgreSQL-style системах.

Подходит для:
- equality lookup;
- range queries;
- sorting by indexed columns;
- prefix usage в composite indexes.

Пример:

```sql
CREATE INDEX idx_users_email ON users(email);
```

Запрос:

```sql
SELECT *
FROM users
WHERE email = 'user@example.com';
```

## Composite index

Индекс по нескольким колонкам:

```sql
CREATE INDEX idx_orders_user_status
ON orders(user_id, status);
```

Хорош для:

```sql
SELECT *
FROM orders
WHERE user_id = 10 AND status = 'paid';
```

Важно:
- порядок колонок в composite index имеет значение;
- индекс `(user_id, status)` не то же самое, что `(status, user_id)`.

## Partial index

Индекс только по части строк:

```sql
CREATE INDEX idx_orders_unpaid
ON orders(created_at)
WHERE status = 'new';
```

Хорош, когда:
- активная часть данных маленькая;
- большинство запросов ходит только по subset.

## Covering index

Идея:
- индекс содержит все, что нужно запросу;
- БД может меньше ходить в таблицу.

В PostgreSQL это часто делают через `INCLUDE`:

```sql
CREATE INDEX idx_orders_user_created
ON orders(user_id, created_at)
INCLUDE (status);
```

## Почему индекс не всегда помогает

Причины:
- условие не селективное;
- таблица маленькая;
- статистика устарела;
- используется функция над колонкой;
- не тот порядок колонок;
- запрос возвращает слишком много строк.

Пример плохого паттерна:

```sql
WHERE lower(email) = lower($1)
```

Обычный индекс по `email` может не помочь, если нет expression index.

## EXPLAIN

`EXPLAIN` показывает план запроса:

```sql
EXPLAIN
SELECT *
FROM orders
WHERE user_id = 10;
```

`EXPLAIN ANALYZE` реально выполняет запрос и показывает фактическое время:

```sql
EXPLAIN ANALYZE
SELECT *
FROM orders
WHERE user_id = 10;
```

Что смотреть:
- sequential scan или index scan;
- estimated rows vs actual rows;
- sort;
- nested loop;
- hash join;
- buffers;
- execution time.

## Цена индексов

Индекс не бесплатный.

Минусы:
- занимает место;
- замедляет writes;
- требует maintenance;
- может стать лишним и неиспользуемым.

## Interview-ready summary

Индекс выбирают не "на колонку", а под конкретный query pattern: `WHERE`, `JOIN`, `ORDER BY`, cardinality и expected rows. Сильный ответ всегда упоминает `EXPLAIN`, селективность и цену индекса на write path.
