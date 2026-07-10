# Партиционирование таблиц

## Содержание

- [Что такое партиционирование](#что-такое-партиционирование)
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

## Что такое партиционирование

**Партиционирование** — это разбиение одной большой таблицы на несколько физически отдельных таблиц-**партиций** по значению **ключа партиционирования** (дата, регион, `tenant_id`, …). Ключевое: для приложения это по-прежнему **одна таблица** — `SELECT`/`INSERT` идут в `orders`, а PostgreSQL сам направляет каждую строку в нужную партицию и при чтении обходит только подходящие. Логически одна таблица, физически — набор дочерних.

```text
              orders   (одна логическая таблица, PARTITION BY RANGE (created_at))
                 │      PostgreSQL сам маршрутизирует строку по created_at
   ┌─────────────┼─────────────┐
   ▼             ▼             ▼
orders_2024_01  orders_2024_02  orders_2024_03   ← физически отдельные таблицы (файлы)

INSERT (…, created_at='2024-02-15') ──► уедет в orders_2024_02
SELECT … WHERE created_at >= '2024-03-01'  ──► прочитает только orders_2024_03
```

Минимальный пример: сначала объявляем родителя (чем бьём), потом дочерние партиции (какой диапазон куда):

```sql
CREATE TABLE orders (id bigint, created_at timestamptz)
    PARTITION BY RANGE (created_at);          -- ключ партиционирования

CREATE TABLE orders_2024_01 PARTITION OF orders
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
CREATE TABLE orders_2024_02 PARTITION OF orders
    FOR VALUES FROM ('2024-02-01') TO ('2024-03-01');

INSERT INTO orders VALUES (1, '2024-02-15');  -- строка сама попадёт в orders_2024_02
```

Типы разбиения (Range / List / Hash) и остальное — ниже.

---

## Зачем партиционировать

Что это даёт:
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

**Существующие данные при этом не перераспределяются** — ни сразу, ни постепенно. Новая партиция создаётся **пустой** и принимает только **будущие** строки, чей ключ попадает в её диапазон. Строка выбирает партицию в момент `INSERT` и остаётся там; фонового перекладывания «задним числом» в Postgres нет. Что из этого следует:

- **Обычную таблицу с данными нельзя сделать партиционированной на месте** — Postgres не раскидает её данные сам. Это ручная миграция (новая партиционированная таблица + бэкфилл + swap): [highload: zero-downtime, кейс 12](./highload-scenarios/06-zero-downtime-patterns.md).
- **Если есть DEFAULT-партиция** и в ней уже лежат строки из диапазона новой партиции — создать её не выйдет (ошибка). Сначала перенести эти строки из default вручную, потом добавлять партицию.
- **`UPDATE` ключа партиционирования двигает строку сам**: смена `created_at` на значение из другого диапазона физически переносит строку (delete+insert) в нужную партицию. Это единственный автоматический переезд — и то по факту апдейта конкретной строки.
- **Разделить/слить *существующие* партиции** — только в PG 17 (`ALTER TABLE … SPLIT PARTITION` / `MERGE PARTITIONS`), и это перезапись данных под `ACCESS EXCLUSIVE` (блокирующая, не онлайн). До PG 17 такого нет.

### Отсоединить и переподключить (ATTACH/DETACH)

```sql
-- DETACH CONCURRENTLY: без ACCESS EXCLUSIVE на родителе, не блокирует запросы
ALTER TABLE orders DETACH PARTITION orders_2023_01 CONCURRENTLY;
-- партиция становится обычной таблицей → архивировать / перенести на cold storage

-- ATTACH берёт SHARE UPDATE EXCLUSIVE на родителе (НЕ ACCESS EXCLUSIVE),
-- но СКАНИРУЕТ присоединяемую таблицу: проверяет, что все строки попадают в границы
ALTER TABLE orders ATTACH PARTITION orders_2023_01
    FOR VALUES FROM ('2023-01-01') TO ('2023-02-01');
```

> **Важно: `ATTACH PARTITION CONCURRENTLY` не существует** — `CONCURRENTLY` есть только у `DETACH`. Чтобы ATTACH не сканировал таблицу под локом, нужно **заранее** навесить на неё совпадающий `CHECK`-констрейнт: PostgreSQL увидит, что границы уже гарантированы, и пропустит скан.

```sql
-- избегаем скан при ATTACH: ставим CHECK заранее (можно как NOT VALID + VALIDATE, без долгого лока)
ALTER TABLE orders_2023_01 ADD CONSTRAINT ck_2023_01
    CHECK (created_at >= '2023-01-01' AND created_at < '2023-02-01');

-- теперь ATTACH видит совпадающий CHECK и НЕ сканирует данные
ALTER TABLE orders ATTACH PARTITION orders_2023_01
    FOR VALUES FROM ('2023-01-01') TO ('2023-02-01');
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

### На что влияют эти операции и что в проде

Каждая манипуляция берёт лок **на родительской таблице** (или партиции) — то есть влияет не только на саму партицию, но и на все запросы к `orders`:

| Операция | Лок | Прод-эффект |
|---|---|---|
| `CREATE TABLE … PARTITION OF` (новая пустая) | краткий `ACCESS EXCLUSIVE` на родителе | быстрый, но встаёт в очередь за долгими запросами к таблице (lock queue) → ставь `lock_timeout` |
| `ATTACH PARTITION` | `SHARE UPDATE EXCLUSIVE` + **скан** присоединяемой | без совпадающего `CHECK` сканирует всю таблицу под локом (долго); с `CHECK` — мгновенно |
| `DETACH PARTITION` (обычный) | `ACCESS EXCLUSIVE` на родителе | блокирует **все** запросы к таблице на время операции |
| `DETACH … CONCURRENTLY` | слабый, не блокирует | нельзя в транзакции; при сбое оставляет партицию в состоянии «detach pending» → нужен `FINALIZE` |
| `DROP TABLE partition` | `ACCESS EXCLUSIVE` на партиции (+ лок на родителе) | мгновенно освобождает место, но ждёт завершения запросов, читающих эту партицию |

Реальные грабли на проде:

- **DEFAULT-партиция делает `ATTACH` дорогим.** Если есть DEFAULT-партиция, при `ATTACH` новой PostgreSQL **сканирует всю DEFAULT-партицию** под `ACCESS EXCLUSIVE` — проверяет, что ни одна её строка не попадает в диапазон новой. На большой default это внезапный долгий блокирующий скан. Вывод: либо не держать DEFAULT, либо держать её пустой.
- **Кончились партиции — отказ вставок.** Если строка не попадает ни в одну партицию, а DEFAULT нет, `INSERT` падает: `no partition of relation "orders" found for row`. Классический инцидент — забыли создать партицию на новый месяц, и на границе месяца вставки начинают падать. Лечится **созданием партиций заранее** (cron-джоба или `pg_partman`).
- **Слишком много партиций бьёт по планировщику.** Тысячи партиций раздувают planning time (планировщик учитывает их все до pruning), память и число захватываемых локов на запрос. Держи счёт в разумных пределах (сотни, не десятки тысяч); для «партиция на день × годы» — суб-партиционирование или агрессивный `DROP` старых.
- **`DROP`/`DETACH` под нагрузкой ждут читателей.** Обе операции берут сильный лок и встают за запросами, которые сейчас читают партицию; на проде их делают в окна или с `lock_timeout` + ретраем, как любой DDL (см. [04-transactions-and-locking.md](./04-transactions-and-locking.md), `lock_timeout`).

Разбор «как партиционировать уже существующую большую таблицу без даунтайма» — [highload: zero-downtime, кейс 12](./highload-scenarios/06-zero-downtime-patterns.md).

---

## Подводные камни

**Foreign keys и партиции — зависит от направления FK:**

*FK **из** партиционированной таблицы наружу* (партиционированная — ссылающаяся сторона) — работает без проблем, FK наследуется всеми партициями:

```sql
-- orders партиционирована и ссылается на обычную users — ок
ALTER TABLE orders ADD CONSTRAINT fk_user
    FOREIGN KEY (user_id) REFERENCES users(id);
```

*FK **на** партиционированную таблицу* (партиционированная — та, на которую ссылаются) — можно, но с условием: ссылаться нужно на **уникальный ключ** партиционированной таблицы, а он (как и PK) обязан включать **ключ партиционирования**. Значит сослаться только на `orders(id)` нельзя — уникален лишь `(id, created_at)`, поэтому и ссылающаяся таблица должна тащить оба столбца:

```sql
-- НЕ выйдет: id сам по себе не уникален в партиционированной таблице
ALTER TABLE refunds ADD FOREIGN KEY (order_id) REFERENCES orders(id);  -- ERROR

-- Работает, но ребёнок обязан хранить и order_created_at
ALTER TABLE refunds ADD FOREIGN KEY (order_id, order_created_at)
    REFERENCES orders(id, created_at);
```

На практике из-за этого «тащи ещё и partition key в ребёнка» ссылки на партиционированные таблицы часто заменяют логической проверкой на уровне приложения или вовсе не делают.

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

**1. Что такое партиционирование и какие типы?**

- Разбиение таблицы на физически отдельные партиции с единым логическим интерфейсом; Range (по времени, самый частый), List (по значениям), Hash (равномерное распределение).

**2. В чём главная выгода?**

- Partition pruning — запрос с фильтром по ключу читает только нужные партиции; плюс мгновенный `DROP`/`DETACH` старой партиции вместо `DELETE` миллионов строк.

**3. Какие ограничения?**

- Unique-констрейнт (и PK) обязан включать ключ партиционирования; FK на партиционированную таблицу не поддерживается; при 1000+ партиций растёт overhead планирования.

**4. Как присоединять/отцеплять без долгого лока?**

- `DETACH PARTITION ... CONCURRENTLY`; у ATTACH `CONCURRENTLY` нет — скан пропускается заранее навешенным совпадающим `CHECK`.

**5. Лучший use case?**

- Таблица событий/логов с monthly-партициями и автоматическим DROP старше N месяцев (pg_partman); связанные данные — colocate по ключу.
