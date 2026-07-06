# JSONB, Массивы и Полнотекстовый Поиск

## Содержание

- [JSON vs JSONB](#json-vs-jsonb)
- [JSONB операторы](#jsonb-операторы)
- [SQL JSON path и операторы jsonpath](#sql-json-path-и-операторы-jsonpath)
- [SQL JSON конструкторы и JSON_TABLE](#sql-json-конструкторы-и-json_table)
- [Индексы для JSONB](#индексы-для-jsonb)
- [Массивы (Arrays)](#массивы-arrays)
- [Полнотекстовый поиск (FTS)](#полнотекстовый-поиск-fts)
- [Trigram similarity (pg_trgm)](#trigram-similarity-pg_trgm)
- [Когда использовать JSONB vs нормализовать](#когда-использовать-jsonb-vs-нормализовать)
- [Interview-ready answer](#interview-ready-answer)

---

## JSON vs JSONB

| Характеристика | `json` | `jsonb` |
|---|---|---|
| Хранение | Текст as-is | Бинарный разобранный формат |
| Запись | Быстрее | Медленнее (парсинг при записи) |
| Чтение | Медленнее (парсинг при чтении) | Быстрее |
| Индексы | Нет GIN | GIN |
| Порядок ключей | Сохраняется | Не гарантируется |
| Дубликаты ключей | Сохраняются | Последнее значение выигрывает |
| Whitespace | Сохраняется | Убирается |

**Всегда использовать `jsonb`**, если нужны запросы или индексы.

```sql
CREATE TABLE products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    attributes  JSONB
);

INSERT INTO products (name, attributes) VALUES (
    'MacBook Pro',
    '{"color": "silver", "ram": 32, "tags": ["laptop", "apple"], "specs": {"cpu": "M3", "cores": 12}}'
);
```

---

## JSONB операторы

### Извлечение значений

```sql
-- -> возвращает JSON-значение (тип json/jsonb)
SELECT attributes->'color' FROM products;  -- "silver" (с кавычками)

-- ->> возвращает TEXT
SELECT attributes->>'color' FROM products;  -- silver (без кавычек)

-- #> путь через массив ключей
SELECT attributes#>'{specs,cpu}' FROM products;  -- "M3"

-- #>> то же, но TEXT
SELECT attributes#>>'{specs,cpu}' FROM products;  -- M3
```

### Проверка содержимого

```sql
-- @> содержит (containment)
SELECT * FROM products WHERE attributes @> '{"color": "silver"}';
SELECT * FROM products WHERE attributes @> '{"tags": ["apple"]}';  -- массив содержит элемент

-- ? содержит ключ
SELECT * FROM products WHERE attributes ? 'color';

-- ?| содержит хотя бы один ключ
SELECT * FROM products WHERE attributes ?| ARRAY['color', 'size'];

-- ?& содержит все ключи
SELECT * FROM products WHERE attributes ?& ARRAY['color', 'ram'];
```

### Модификация

```sql
-- || объединение
UPDATE products
SET attributes = attributes || '{"weight": 1.5}'
WHERE id = 1;

-- jsonb_set: установить значение по пути
UPDATE products
SET attributes = jsonb_set(attributes, '{specs,cores}', '16')
WHERE id = 1;

-- - удалить ключ
UPDATE products
SET attributes = attributes - 'color'
WHERE id = 1;

-- #- удалить по пути
UPDATE products
SET attributes = attributes #- '{specs,cpu}'
WHERE id = 1;
```

### Разворачивание (unnest)

```sql
-- jsonb_each: развернуть в key-value пары
SELECT key, value FROM products, jsonb_each(attributes) WHERE id = 1;

-- jsonb_array_elements: развернуть массив
SELECT elem FROM products, jsonb_array_elements(attributes->'tags') AS elem WHERE id = 1;

-- jsonb_array_elements_text: развернуть массив в TEXT
SELECT elem FROM products, jsonb_array_elements_text(attributes->'tags') AS elem WHERE id = 1;
```

### Агрегация

```sql
-- собрать JSONB объект из запроса
SELECT jsonb_object_agg(key, value) FROM ...;

-- собрать JSONB массив
SELECT jsonb_agg(attributes) FROM products WHERE ...;
```

---

## SQL JSON path и операторы jsonpath

**Зачем.** Операторы `->`, `@>` хороши для простых проверок, но не умеют условий внутри документа («есть ли тег с `priority > 5`», «любой элемент массива дороже 100»). Для этого есть стандартный язык **SQL/JSON path** (тип `jsonpath`, PG12+) — аналог XPath для JSON.

Базовый синтаксис пути: `$` — корень, `.key` — поле, `[*]` — все элементы массива, `?(...)` — фильтр, `@` — текущий элемент в фильтре.

```sql
-- jsonb_path_query: вернуть все значения по пути
SELECT jsonb_path_query(attributes, '$.tags[*]') FROM products WHERE id = 1;
-- "laptop"
-- "apple"

-- фильтр: теги, начинающиеся на 'a' (предикат внутри пути)
SELECT jsonb_path_query(attributes, '$.tags[*] ? (@ like_regex "^a")') FROM products;

-- обращение к вложенным полям + арифметика/сравнение
SELECT jsonb_path_query(attributes, '$.specs.cores ? (@ > 8)') FROM products;
```

Два оператора для `WHERE` (оба индексируются GIN с `jsonb_ops`):

```sql
-- @? : существует ли хоть один матч пути (возвращает bool)
SELECT * FROM products WHERE attributes @? '$.specs.cores ? (@ >= 12)';

-- @@ : jsonpath-ПРЕДИКАТ истинен (путь сразу содержит условие, без ?(...))
SELECT * FROM products WHERE attributes @@ '$.specs.cpu == "M3"';
```

Разница: `@?` спрашивает «нашёлся ли элемент по пути-с-фильтром», `@@` вычисляет путь как булев предикат. Для `like_regex`, диапазонов и арифметики внутри JSON это единственный способ без распаковки документа в строки.

```sql
-- strict/lax: lax (default) прощает обращение к несуществующим полям, strict — ошибка
SELECT jsonb_path_query(attributes, 'strict $.missing.field') FROM products;  -- ошибка пути
SELECT jsonb_path_query(attributes, 'lax $.missing.field') FROM products;     -- пусто, без ошибки
```

---

## SQL JSON конструкторы и JSON_TABLE

PostgreSQL 16–17 добавили стандартные SQL/JSON-функции — раньше их заменяли `jsonb_build_*` и ручная распаковка.

**Проверка и извлечение (PG16):**

```sql
-- IS JSON: проверить, что текст — валидный JSON (и какого вида)
SELECT '{"a":1}' IS JSON OBJECT;        -- true
SELECT '[1,2]'   IS JSON ARRAY;         -- true
SELECT 'oops'    IS JSON;               -- false

-- JSON_EXISTS / JSON_VALUE / JSON_QUERY — типобезопасное извлечение по jsonpath
SELECT
    JSON_EXISTS(attributes, '$.specs.cpu')              AS has_cpu,    -- bool
    JSON_VALUE(attributes, '$.specs.cores' RETURNING int) AS cores,    -- скаляр нужного типа
    JSON_QUERY(attributes, '$.tags')                     AS tags_json  -- фрагмент JSON
FROM products;
```

`JSON_VALUE` возвращает **скаляр** (с приведением через `RETURNING`), `JSON_QUERY` — **объект/массив** как JSON. Это чище и строже, чем `->>`/`#>>` с ручным `::int`.

**JSON_TABLE (PG17)** — разложить JSON-документ в реляционную таблицу прямо в `FROM`. Заменяет связки `jsonb_array_elements` + `->>`:

```sql
-- каждый элемент массива tags → строка таблицы с типизированными колонками
SELECT p.id, t.tag, t.ord
FROM products p,
     JSON_TABLE(p.attributes, '$.tags[*]'
         COLUMNS (
             ord  FOR ORDINALITY,              -- порядковый номер элемента
             tag  text PATH '$'                -- значение элемента
         )) AS t;

-- разложить массив объектов в плоские строки
SELECT j.*
FROM orders o,
     JSON_TABLE(o.payload, '$.items[*]'
         COLUMNS (
             sku    text    PATH '$.sku',
             qty    int     PATH '$.qty',
             price  numeric PATH '$.price'
         )) AS j;
```

---

## Индексы для JSONB

### GIN (общий случай)

```sql
-- индексирует все ключи и значения (для @>, ?, ?|, ?&)
CREATE INDEX idx_products_attrs ON products USING GIN (attributes);

-- jsonb_path_ops: только для @> (меньше размер, быстрее для containment)
CREATE INDEX idx_products_attrs_path ON products USING GIN (attributes jsonb_path_ops);
```

### Expression index (конкретный ключ)

```sql
-- индекс только по одному ключу — меньше, быстрее
CREATE INDEX idx_products_color ON products ((attributes->>'color'));

-- использование
SELECT * FROM products WHERE attributes->>'color' = 'silver';
```

### Что индексировать

- Если запросы используют `@>` — GIN с `jsonb_path_ops`.
- Если запросы используют `?`, `?|`, `?&` — GIN без ops.
- Если запрос по конкретному ключу с высокой selectivity — expression index быстрее GIN.

---

## Массивы (Arrays)

PostgreSQL поддерживает нативные массивы любого типа.

```sql
CREATE TABLE users (
    id      BIGSERIAL PRIMARY KEY,
    email   TEXT NOT NULL,
    roles   TEXT[] DEFAULT '{}',
    scores  INTEGER[]
);

INSERT INTO users (email, roles) VALUES ('user@example.com', ARRAY['reader', 'editor']);
```

### Операторы

```sql
-- @ содержит (все элементы)
SELECT * FROM users WHERE roles @> ARRAY['admin'];

-- && пересекаются (хотя бы один общий)
SELECT * FROM users WHERE roles && ARRAY['admin', 'editor'];

-- = равенство массивов
SELECT * FROM users WHERE roles = ARRAY['reader'];

-- ANY: значение равно хотя бы одному элементу массива
SELECT * FROM users WHERE 'admin' = ANY(roles);

-- ALL: значение равно всем
SELECT * FROM users WHERE 'reader' = ALL(roles);
```

### Модификация

```sql
-- append
UPDATE users SET roles = roles || ARRAY['moderator'] WHERE id = 1;
-- или
UPDATE users SET roles = array_append(roles, 'moderator') WHERE id = 1;

-- remove элемент
UPDATE users SET roles = array_remove(roles, 'editor') WHERE id = 1;

-- длина
SELECT array_length(roles, 1) FROM users;

-- развернуть
SELECT id, unnest(roles) AS role FROM users;
```

### Индекс для массивов

```sql
CREATE INDEX idx_users_roles ON users USING GIN (roles);

-- запрос использует GIN
SELECT * FROM users WHERE roles @> ARRAY['admin'];
```

---

## Полнотекстовый поиск (FTS)

PostgreSQL включает встроенный FTS через `tsvector` и `tsquery`.

```sql
-- tsvector: нормализованный вектор слов
SELECT to_tsvector('russian', 'Новый MacBook Pro от Apple появился в продаже');
-- 'apple':7 'macbook':2 'pro':3 'появил':6 'продаж':9

-- tsquery: поисковый запрос
SELECT to_tsquery('russian', 'MacBook & Apple');

-- матч: @@ оператор
SELECT to_tsvector('russian', 'Новый MacBook от Apple') @@ to_tsquery('russian', 'MacBook');
-- true
```

### Таблица с FTS

```sql
CREATE TABLE articles (
    id      BIGSERIAL PRIMARY KEY,
    title   TEXT NOT NULL,
    body    TEXT NOT NULL,
    fts     TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('russian', title), 'A') ||
        setweight(to_tsvector('russian', body), 'B')
    ) STORED
);

CREATE INDEX idx_articles_fts ON articles USING GIN (fts);

-- поиск
SELECT id, title,
       ts_rank(fts, query) AS rank
FROM articles,
     to_tsquery('russian', 'PostgreSQL & индекс') AS query
WHERE fts @@ query
ORDER BY rank DESC
LIMIT 20;
```

`ts_rank` — relevance score. `setweight` задаёт вес (A > B > C > D) для ранжирования.

### Поиск с OR, NOT, фразы

```sql
-- OR (|)
SELECT * FROM articles WHERE fts @@ to_tsquery('russian', 'PostgreSQL | MySQL');

-- NOT (!)
SELECT * FROM articles WHERE fts @@ to_tsquery('russian', 'PostgreSQL & !MySQL');

-- фразовый поиск (слова рядом)
SELECT * FROM articles WHERE fts @@ phraseto_tsquery('russian', 'база данных');

-- websearch синтаксис (как Google)
SELECT * FROM articles WHERE fts @@ websearch_to_tsquery('russian', '"база данных" PostgreSQL -MySQL');
```

Highlight результата:
```sql
SELECT ts_headline('russian', body, to_tsquery('russian', 'PostgreSQL'), 
                   'StartSel=<b>, StopSel=</b>, MaxWords=50')
FROM articles
WHERE fts @@ to_tsquery('russian', 'PostgreSQL');
```

---

## Trigram similarity (pg_trgm)

`pg_trgm` — поиск похожих строк по триграммам (наборам из 3 символов). Полезен для "опечатки", autocomplete, fuzzy search.

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- similarity: 0.0 (разные) до 1.0 (одинаковые)
SELECT similarity('PostgreSQL', 'Postgresq');  -- ~0.57

-- % оператор: похожесть > pg_trgm.similarity_threshold (default 0.3)
SELECT * FROM products WHERE name % 'Macbok';

-- <-> distance (обратная схожесть, для ORDER BY)
SELECT name, name <-> 'MacBook' AS dist FROM products ORDER BY dist LIMIT 5;
```

Индекс для %:
```sql
CREATE INDEX idx_products_name_trgm ON products USING GIN (name gin_trgm_ops);
-- или GiST (медленнее build, быстрее % для очень похожих строк)
CREATE INDEX idx_products_name_trgm_gist ON products USING GiST (name gist_trgm_ops);
```

LIKE с GIN индексом (pg_trgm позволяет ускорить LIKE):
```sql
-- с установленным GIN trgm индексом
SELECT * FROM products WHERE name LIKE '%MacBook%';
-- планировщик может использовать GIN индекс для LIKE
```

---

## Когда использовать JSONB vs нормализовать

**Используй JSONB если:**
- Схема атрибутов непредсказуема или разная для каждого объекта (EAV замена).
- Данные приходят из внешних API и нужно хранить их "как есть".
- Редко делаешь запросы по конкретным полям JSONB.
- Semi-structured данные: теги, метаданные, конфигурации.

**Нормализуй если:**
- Часто нужны запросы, joins, агрегации по конкретным полям.
- Нужны foreign keys или constraints на значения.
- Поля имеют фиксированную схему и типы важны.
- Много записей — индекс по нормализованной колонке эффективнее GIN.

**Гибридный подход** — лучший в реальности:
```sql
CREATE TABLE products (
    id       BIGSERIAL PRIMARY KEY,
    name     TEXT NOT NULL,      -- часто в WHERE/ORDER BY → нормализовать
    price    NUMERIC(10, 2),     -- часто в WHERE/ORDER BY → нормализовать
    category TEXT,               -- часто в WHERE → нормализовать
    metadata JSONB               -- редкие атрибуты → JSONB
);
```

---

## Interview-ready answer

**1. Чем jsonb отличается от json?**

- jsonb — бинарный разобранный формат: быстрый read и GIN-индексы; json — текст as-is (быстрее запись, но без индексов, с сохранением порядка и дубликатов ключей). Для запросов всегда jsonb.

**2. Что делает оператор `@>`?**

- Containment — содержит ли документ заданный ключ/значение/элемент массива; индексируется GIN.

**3. Какой GIN-opclass для JSONB выбрать?**

- `jsonb_path_ops` — только `@>`, меньше и быстрее; `jsonb_ops` (default) — ещё `?`, `?|`, `?&` и jsonpath-операторы.

**4. Как делать условия внутри документа?**

- Язык SQL/JSON path (`jsonpath`, PG12) с фильтрами `?(...)`, `like_regex`, диапазонами; операторы `@?` (есть ли матч) и `@@` (предикат истинен), оба под GIN.

**5. Что нового в SQL/JSON (PG16–17)?**

- `IS JSON`, типобезопасные `JSON_VALUE`/`JSON_QUERY`/`JSON_EXISTS` и `JSON_TABLE` — раскладка JSON в реляционные строки прямо в `FROM`.

**6. Чем индексировать массивы и LIKE?**

- Массивы — GIN (`@>`, `&&`, `ANY`); `LIKE '%..%'` и похожесть — pg_trgm (GIN/GiST по триграммам).

**7. JSONB vs нормализация?**

- JSONB — динамическая/редко запрашиваемая схема, метаданные; нормализация — фиксированная схема, частые запросы/joins/constraints; на практике гибрид (горячие поля колонками, остальное в JSONB). Массивы в PostgreSQL нативные: операторы `@>`, `&&`, `ANY()`, GIN индекс. Полнотекстовый поиск через `tsvector`/`tsquery`, GENERATED ALWAYS STORED вычисляемая колонка для FTS вектора, `ts_rank` для ранжирования. pg_trgm — fuzzy search по триграммам, ускоряет LIKE и `%` оператор похожести. Когда использовать JSONB: динамическая схема, редкие запросы по полям; нормализовать: фиксированная схема, частые запросы, joins, constraints.
