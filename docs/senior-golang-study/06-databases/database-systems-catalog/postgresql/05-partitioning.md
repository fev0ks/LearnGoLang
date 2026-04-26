# Партиционирование таблиц

## Содержание

- [Зачем партиционировать](#зачем-партиционировать)
- [Range Partitioning](#range-partitioning)
- [List Partitioning](#list-partitioning)
- [Hash Partitioning](#hash-partitioning)
- [Partition Pruning](#partition-pruning)
- [Индексы в партиционированных таблицах](#индексы-в-партиционированных-таблицах)
- [Управление партициями](#управление-партициями)
- [Подводные камни](#подводные-камни)
- [Когда партиционировать](#когда-партиционировать)
- [Interview-ready answer](#interview-ready-answer)

---

## Зачем партиционировать

Партиционирование разбивает большую таблицу на физически отдельные куски (партиции). Логически — одна таблица, физически — разные файлы.

Что даёт:
- **Partition pruning** — запрос с предикатом по ключу партиционирования читает только релевантные партиции.
- **Maintenance** — `DROP PARTITION` мгновенно удаляет старые данные (vs DELETE миллионов строк).
- **Vacuum** — vacuum работает по партициям параллельно, меньше bloat накапливается.
- **Bulk load** — вставка в отдельную партицию + attach.

Когда имеет смысл:
- Таблица > 50–100 GB.
- Есть natural partition key (дата, регион, tenant_id).
- Регулярное удаление старых данных.
- Запросы почти всегда фильтруют по ключу партиционирования.

---

## Range Partitioning

Наиболее распространённый тип — по диапазону значений. Идеален для временных данных.

```sql
CREATE TABLE orders (
    id          BIGSERIAL,
    user_id     BIGINT NOT NULL,
    total       NUMERIC(12, 2),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at);

-- создать партиции по месяцам
CREATE TABLE orders_2024_01 PARTITION OF orders
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');

CREATE TABLE orders_2024_02 PARTITION OF orders
    FOR VALUES FROM ('2024-02-01') TO ('2024-03-01');

-- DEFAULT партиция для значений вне диапазонов
CREATE TABLE orders_default PARTITION OF orders DEFAULT;
```

Автоматическое создание партиций через pg_partman:

```sql
-- расширение pg_partman управляет созданием/удалением партиций
SELECT partman.create_parent(
    p_parent_table => 'public.orders',
    p_control => 'created_at',
    p_type => 'native',
    p_interval => 'monthly',
    p_premake => 3  -- создавать 3 будущих партиции заранее
);
```

---

## List Partitioning

По конкретным значениям. Для tenant isolation, регионов, статусов.

```sql
CREATE TABLE events (
    id       BIGSERIAL,
    region   TEXT NOT NULL,
    payload  JSONB,
    ts       TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY LIST (region);

CREATE TABLE events_eu PARTITION OF events
    FOR VALUES IN ('EU', 'EMEA');

CREATE TABLE events_us PARTITION OF events
    FOR VALUES IN ('US', 'CA');

CREATE TABLE events_apac PARTITION OF events
    FOR VALUES IN ('APAC', 'AU', 'JP');

CREATE TABLE events_other PARTITION OF events DEFAULT;
```

---

## Hash Partitioning

Равномерное распределение по hash от значения. Для tenant_id, user_id когда нет natural range и нужно снизить размер партиций.

```sql
CREATE TABLE user_events (
    id        BIGSERIAL,
    user_id   BIGINT NOT NULL,
    event     TEXT,
    ts        TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY HASH (user_id);

-- 8 партиций, остаток от деления hash на 8
CREATE TABLE user_events_0 PARTITION OF user_events
    FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE user_events_1 PARTITION OF user_events
    FOR VALUES WITH (MODULUS 8, REMAINDER 1);
-- ... до 7
```

---

## Partition Pruning

Планировщик исключает нерелевантные партиции на этапе planning или execution.

```sql
-- запрос только по одному месяцу → читается одна партиция
EXPLAIN SELECT * FROM orders WHERE created_at >= '2024-01-01' AND created_at < '2024-02-01';

-- Append
--   -> Seq Scan on orders_2024_01
--      Filter: ...
-- (остальные 11 партиций исключены)
```

Условие для pruning:
- Предикат должен содержать ключ партиционирования.
- Значение должно быть константой или параметром (не функцией от другого столбца).

Enable/disable:
```sql
SET enable_partition_pruning = on;  -- default on
```

Runtime pruning (для параметризованных запросов):
```sql
-- $1 = конкретная дата → pruning на этапе execution
SELECT * FROM orders WHERE created_at = $1;
```

---

## Индексы в партиционированных таблицах

### Глобальный (партиционированный) индекс

Создаётся на родительской таблице — автоматически создаётся на каждой партиции.

```sql
CREATE INDEX idx_orders_user_id ON orders (user_id);
-- эквивалентно создать idx_orders_YYYY_MM_user_id на каждой партиции
```

### Уникальные индексы

Уникальный индекс должен включать ключ партиционирования:

```sql
-- работает
CREATE UNIQUE INDEX ON orders (id, created_at);

-- НЕ работает — id уникален только внутри партиции
CREATE UNIQUE INDEX ON orders (id);  -- ERROR
```

Альтернатива для глобальной уникальности: sequence + приложение.

### Primary Key

Аналогично уникальному индексу — должен включать ключ партиционирования:

```sql
CREATE TABLE orders (
    id          BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    -- ...
    PRIMARY KEY (id, created_at)  -- включаем partition key
) PARTITION BY RANGE (created_at);
```

---

## Управление партициями

### Добавление новой партиции

```sql
CREATE TABLE orders_2025_01 PARTITION OF orders
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
```

### Отсоединить и переподключить (ATTACH/DETACH)

```sql
-- отсоединить партицию (мгновенно, партиция становится обычной таблицей)
ALTER TABLE orders DETACH PARTITION orders_2023_01;

-- архивировать / перенести на cold storage

-- attach с проверкой ограничений (может сканировать партицию)
ALTER TABLE orders ATTACH PARTITION orders_2023_01
    FOR VALUES FROM ('2023-01-01') TO ('2023-02-01');

-- CONCURRENTLY — attach без ACCESS EXCLUSIVE lock (PG 14+)
ALTER TABLE orders ATTACH PARTITION orders_2023_01
    FOR VALUES FROM ('2023-01-01') TO ('2023-02-01')
    NOT NULL; -- если есть constraint — проверка пропускается
```

### Удалить старые данные

```sql
-- мгновенно, без bloat (vs DELETE)
DROP TABLE orders_2022_01;

-- или: detach + drop позже
ALTER TABLE orders DETACH PARTITION orders_2022_01 CONCURRENTLY;
DROP TABLE orders_2022_01;
```

### Bulk load + attach

```sql
-- создать временную таблицу как отдельную (не партицию)
CREATE TABLE orders_import (LIKE orders INCLUDING DEFAULTS);

-- загрузить данные без overhead партиционирования
COPY orders_import FROM '/tmp/orders_2025_02.csv';
CREATE INDEX ON orders_import (user_id);

-- быстрый attach — данные уже проверены
ALTER TABLE orders ATTACH PARTITION orders_import
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
```

---

## Подводные камни

**Foreign keys на партиционированную таблицу** — нельзя создать FK, ссылающийся на партиционированную таблицу (только на конкретную партицию или родительскую без PARTITION BY).

**ON CONFLICT (upsert)** — работает только если ключ конфликта включает ключ партиционирования:

```sql
INSERT INTO orders (id, created_at, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (id, created_at) DO UPDATE SET user_id = EXCLUDED.user_id;
```

**Переключение партиции** — UPDATE строки, меняющий ключ партиционирования, физически переносит строку в другую партицию (DELETE + INSERT).

**Subpartitioning** — таблица может быть партиционирована по двум уровням (сначала по дате, потом по региону), но усложняет management.

**Лимит партиций** — при большом числе партиций (1000+) overhead на planning становится заметным. Рекомендуют не более 100–200 партиций на одну таблицу.

---

## Когда партиционировать

Стоит:
- Таблица логов/событий с retention policy (ежемесячный DROP старой партиции).
- Таблица платёжных транзакций > 100M строк с range по дате.
- Multi-tenant данные (partition by tenant_id для isolation).

Не стоит:
- Таблица небольшого размера (< 10M строк) — overhead без выгоды.
- Нет natural partition key — hash partitioning без pruning не даёт преимущества в скорости запросов.
- Запросы не фильтруют по ключу партиционирования — partition pruning не работает.

---

## Interview-ready answer

Партиционирование в PostgreSQL разбивает таблицу на физически отдельные файлы (партиции) с единым логическим интерфейсом. Три типа: Range (по времени — наиболее популярен), List (по конкретным значениям), Hash (равномерное распределение). Главная выгода — partition pruning: запрос с фильтром по ключу читает только нужные партиции. Вторая выгода — мгновенный DROP старой партиции вместо DELETE миллионов строк. Ограничения: unique constraint должен включать ключ партиционирования, FK на партиционированные таблицы не поддерживаются. Лучший use case — таблица событий/логов с monthly partitioning и автоматическим DROP партиций старше N месяцев (pg_partman).
