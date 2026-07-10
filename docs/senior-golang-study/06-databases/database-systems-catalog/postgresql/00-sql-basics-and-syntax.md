# Основы SQL и синтаксис запросов

Стартовый файл раздела: реляционная модель и базовые понятия, затем справочник по конструкциям запросов — JOIN-типы, агрегация, оконные функции, CTE, множества, подзапросы. Как это исполняется под капотом (join-стратегии, сканы) — в [03-query-planning.md](./03-query-planning.md); задачи на применение — в [sql-tasks](./sql-tasks/README.md).

Сквозные таблицы для примеров:

```sql
users  (id, name, country)
orders (id, user_id, amount, status, created_at)   -- user_id → users.id
```

## Содержание

- [Реляционная модель](#реляционная-модель)
- [CRUD](#crud)
- [Constraints: целостность на уровне БД](#constraints-целостность-на-уровне-бд)
- [Нормализация](#нормализация)
  - [Когда отказываться от нормализации](#когда-отказываться-от-нормализации-денормализация)
- [JOIN: типы соединений](#join-типы-соединений)
  - [INNER JOIN](#inner-join)
  - [LEFT / RIGHT / FULL OUTER JOIN](#left--right--full-outer-join)
  - [CROSS JOIN](#cross-join)
  - [Self-join](#self-join)
  - [LATERAL JOIN](#lateral-join)
  - [Semi- и anti-join (EXISTS / NOT EXISTS)](#semi--и-anti-join-exists--not-exists)
  - [ON vs USING, и почему не NATURAL](#on-vs-using-и-почему-не-natural)
- [Агрегация и группировка](#агрегация-и-группировка)
- [Оконные функции](#оконные-функции)
- [CTE (WITH)](#cte-with)
- [Операции над множествами](#операции-над-множествами)
- [Подзапросы](#подзапросы)
- [Полезные конструкции](#полезные-конструкции)
- [Interview-ready answer](#interview-ready-answer)

---

## Реляционная модель

Реляционная БД хранит данные в **таблицах** и связывает их через ключи:

- **table** — набор строк одного типа (`users`, `orders`, `payments`);
- **row** — одна запись;
- **column** — одно поле записи (`id`, `email`, `created_at`) со своим типом;
- **primary key (PK)** — уникальный идентификатор строки;
- **foreign key (FK)** — ссылка на строку в другой таблице (обеспечивает ссылочную целостность).

```sql
CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id),   -- FK на users
    status     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## CRUD

Четыре базовые операции над данными:

```sql
INSERT INTO users (email) VALUES ('user@example.com');            -- Create
SELECT id, email FROM users WHERE email = 'user@example.com';     -- Read
UPDATE orders SET status = 'paid' WHERE id = 42;                  -- Update
DELETE FROM users WHERE id = 42;                                  -- Delete
```

## Constraints: целостность на уровне БД

Constraints не дают данным попасть в некорректное состояние. Основные:

| Constraint | Гарантирует |
|---|---|
| `PRIMARY KEY` | уникальность + `NOT NULL` идентификатора строки |
| `FOREIGN KEY` | ссылка указывает на существующую строку (нет «сирот») |
| `UNIQUE` | нет дублей по столбцу/набору |
| `NOT NULL` | значение обязательно |
| `CHECK` | произвольное условие на значение |

```sql
ALTER TABLE orders
ADD CONSTRAINT orders_status_check
CHECK (status IN ('new', 'paid', 'cancelled'));
```

**Почему не полагаться только на валидацию в коде:** в одну БД пишут несколько сервисов, есть миграции/скрипты/ручные фиксы, а конкурентность обходит наивные проверки «прочитал → проверил → записал». Правило: бизнес-инвариант, который **нельзя** нарушать, защищают и на уровне БД тоже (constraint / unique / `CHECK`), а не только в приложении.

## Нормализация

Нормализация — про то, чтобы **каждый факт хранился в одном месте** и не дублировался без необходимости. Плохая идея — держать `user_email` в каждой строке `orders`, если email уже есть в `users`: при смене email придётся обновлять во множестве мест, и они разъедутся (аномалия обновления).

Формы идут по нарастанию строгости, каждая включает предыдущую:

| Форма | Требование (простыми словами) | Что чинит |
|---|---|---|
| **1NF** | атомарные значения: одна ячейка = одно значение, без списков «в строке» и повторяющихся групп колонок | `tags = 'a,b,c'` или `phone1/phone2/phone3` → отдельные строки/таблица |
| **2NF** | 1NF + каждый неключевой атрибут зависит от **всего** составного ключа, а не от его части | в `(order_id, product_id, product_name)` `product_name` зависит только от `product_id` → вынести товар в свою таблицу |
| **3NF** | 2NF + неключевые атрибуты зависят **только от ключа**, а не друг от друга (нет транзитивных зависимостей) | в `(user_id, city, city_zip)` `city_zip` зависит от `city`, а не от `user_id` → вынести город/индекс |
| **BCNF** | усиленная 3NF: **любая** детерминанта обязана быть ключом (чинит редкие случаи с перекрывающимися кандидатными ключами) | нишевые случаи; на практике 3NF почти всегда достаточно |

Практический ориентир: **целься в 3NF** — этого хватает подавляющему большинству OLTP-схем. 1NF (атомарность) обязательна; 2NF/3NF — почти всегда. 4NF/5NF — теоретические, на практике не встречаются. Мнемоника 3NF: «каждый неключевой атрибут зависит от **ключа, всего ключа и ничего кроме ключа**».

### Когда отказываться от нормализации (денормализация)

Нормализация оптимизирует **запись и целостность** (факт в одном месте → нет аномалий), но платит **join'ами на чтении**. Денормализация — осознанное дублирование ради скорости чтения; это выбор под конкретные запросы, а не «забыл нормализовать». Когда оправдано:

- **read-heavy, а join дорогой** — часто читаемое поле дублируют, чтобы не джойнить (`user_name` в `orders` для ленты). Плата — синхронизировать при изменении источника (триггер / приложение / пересчёт).
- **предвычисленные агрегаты** — `orders_count`/`total_spent` в `users` вместо `COUNT`/`SUM` на каждый запрос (обновлять при изменении; горячие счётчики — [highload-scenarios](./highload-scenarios/04-hot-rows-and-counters.md)).
- **исторические снапшоты** — цену/адрес на момент заказа **специально** копируют в `order_items`. Это не дубль, а фиксация факта «сколько стоило тогда»: источник может поменяться, а заказ — нет. Здесь денормализация **правильнее** нормализованного варианта.
- **гибкие/редкие атрибуты в JSONB** — вместо десятков nullable-колонок или EAV ([07-jsonb-and-arrays.md](./07-jsonb-and-arrays.md)).
- **аналитика/OLAP** — звёздная схема с денормализованными измерениями (там чтение важнее целостности записи).

Правило: **сначала нормализуй, денормализуй по профилированию** — когда конкретный запрос доказанно горячий и join реально дорог. И дублирование почти всегда защищают (триггер/constraint/пересчёт), чтобы копии не разъезжались.

> SQL из Go: не собирать запрос конкатенацией строк (только placeholders `$1`), обрабатывать `pgx.ErrNoRows`, передавать `context.Context` с таймаутом, не держать транзакцию дольше нужного — детали в [11-go-patterns.md](./11-go-patterns.md).

---

## JOIN: типы соединений

JOIN связывает строки двух таблиц по условию. Тип определяет, что делать со строками **без пары**.

| Тип | Что возвращает |
|---|---|
| `INNER JOIN` | только строки, у которых есть пара в обеих таблицах |
| `LEFT [OUTER] JOIN` | все строки левой + пары из правой (нет пары → `NULL` справа) |
| `RIGHT [OUTER] JOIN` | все строки правой + пары из левой |
| `FULL [OUTER] JOIN` | все строки обеих; где нет пары — `NULL` с той стороны |
| `CROSS JOIN` | декартово произведение (каждая с каждой) |

### INNER JOIN

```sql
-- заказы вместе с данными их пользователей; пользователи без заказов и заказы без юзера отсеются
SELECT u.name, o.amount
FROM users u
JOIN orders o ON o.user_id = u.id;
```

`JOIN` без слова = `INNER JOIN`.

### LEFT / RIGHT / FULL OUTER JOIN

```sql
-- ВСЕ пользователи, даже без заказов (у таких o.* будет NULL)
SELECT u.name, o.amount
FROM users u
LEFT JOIN orders o ON o.user_id = u.id;

-- найти пользователей БЕЗ заказов (anti-join через LEFT + IS NULL)
SELECT u.name
FROM users u
LEFT JOIN orders o ON o.user_id = u.id
WHERE o.id IS NULL;
```

`RIGHT JOIN` — зеркало `LEFT` (обычно вместо него просто меняют таблицы местами). `FULL OUTER JOIN` — строки без пары с **обеих** сторон.

Ловушка: условие на правую таблицу в `WHERE` превращает `LEFT JOIN` в `INNER` (строки с `NULL` отсеются). Если фильтр должен применяться до соединения — его место в `ON`:

```sql
-- НЕ то: WHERE o.status='paid' отбросит пользователей без заказов
SELECT u.name, o.amount FROM users u
LEFT JOIN orders o ON o.user_id = u.id
WHERE o.status = 'paid';

-- то: все пользователи, а справа — только их оплаченные заказы
SELECT u.name, o.amount FROM users u
LEFT JOIN orders o ON o.user_id = u.id AND o.status = 'paid';
```

### CROSS JOIN

Декартово произведение — каждая строка с каждой. Полезен для генерации комбинаций (например, все пары «пользователь × месяц» для отчёта с нулями):

```sql
SELECT u.id, m.month
FROM users u
CROSS JOIN generate_series('2024-01-01'::date, '2024-12-01', '1 month') AS m(month);
```

### Self-join

Таблица соединяется сама с собой через алиасы — например, сотрудник и его менеджер в одной таблице:

```sql
SELECT e.name AS employee, m.name AS manager
FROM employees e
LEFT JOIN employees m ON m.id = e.manager_id;
```

### LATERAL JOIN

`LATERAL` позволяет правой части ссылаться на столбцы левой — то есть выполнить подзапрос **для каждой строки** левой таблицы. Классика — **top-N на группу** (последние 3 заказа каждого пользователя):

```sql
SELECT u.name, o.amount, o.created_at
FROM users u
CROSS JOIN LATERAL (
    SELECT amount, created_at
    FROM orders
    WHERE user_id = u.id            -- ← ссылка на внешнюю строку, это и есть LATERAL
    ORDER BY created_at DESC
    LIMIT 3
) o;
```

Без `LATERAL` подзапрос в `FROM` не видит `u.id`. Альтернатива для top-N — оконная функция `ROW_NUMBER()` (см. ниже); LATERAL часто эффективнее, когда N маленькое и есть индекс.

### Semi- и anti-join (EXISTS / NOT EXISTS)

Иногда нужен факт наличия/отсутствия пары, а не сами строки правой таблицы — тогда join не нужен, есть `EXISTS`:

```sql
-- semi-join: пользователи, у которых ЕСТЬ хоть один заказ (без размножения строк)
SELECT u.name FROM users u
WHERE EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id);

-- anti-join: пользователи БЕЗ заказов
SELECT u.name FROM users u
WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id);
```

Чем это лучше `IN`/`JOIN`: `EXISTS` не размножает строки (в отличие от `JOIN`, если пар несколько) и **безопаснее `NOT IN`**. Главная ловушка — `NOT IN` с `NULL`:

```sql
-- ОПАСНО: если подзапрос вернёт хоть один NULL, результат ПУСТОЙ (трёхзначная логика)
SELECT * FROM users WHERE id NOT IN (SELECT user_id FROM orders);
-- Безопасно: NOT EXISTS не ломается на NULL
SELECT * FROM users u WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id);
```

Разбор ловушки на задаче — [sql-tasks: 0183](./sql-tasks/leetcode/easy/0183-customers-who-never-order.md).

### ON vs USING, и почему не NATURAL

```sql
JOIN orders o ON o.user_id = u.id       -- явное условие
JOIN orders USING (user_id)             -- если столбец назван одинаково в обеих; в SELECT колонка одна
```

`NATURAL JOIN` (соединить по всем одноимённым столбцам автоматически) в проде **не используют**: добавили одноимённый столбец — и запрос молча меняет смысл. Всегда явный `ON`.

---

## Агрегация и группировка

Агрегатные функции схлопывают набор строк в одно значение. `GROUP BY` считает их **по группам**:

```sql
SELECT country,
       count(*)            AS users,          -- строк в группе
       count(phone)        AS with_phone,     -- count(col) НЕ считает NULL
       count(DISTINCT city) AS cities,
       avg(age)            AS avg_age
FROM users
GROUP BY country
HAVING count(*) > 100;                          -- фильтр ПО группам (после агрегации)
```

- **`WHERE` vs `HAVING`**: `WHERE` фильтрует строки *до* группировки, `HAVING` — группы *после*. `WHERE` дешевле, применяй его, если условие не про агрегат.
- **`count(*)` vs `count(col)`**: `*` считает все строки, `count(col)` — только где `col IS NOT NULL`.

**`FILTER (WHERE …)`** — условная агрегация без `CASE` (аккуратнее и читаемее):

```sql
SELECT
    count(*)                              AS total,
    count(*) FILTER (WHERE status='paid') AS paid,
    sum(amount) FILTER (WHERE status='paid') AS revenue
FROM orders;
```

<details>
<summary>Пример с данными</summary>

`orders`:

```text
user_id | amount | status
   1    |  100   | paid
   1    |   50   | failed
   1    |   30   | paid
   2    |  200   | paid
```

Запрос выше (без `GROUP BY`, по всей таблице) → один ряд:

```text
total | paid | revenue
  4   |  3   |  330      -- paid: 100+30+200; failed не попал в FILTER
```

`FILTER (WHERE …)` = «посчитай этот агрегат только по строкам, где условие истинно» — рядом с общим, в одной выборке.

</details>

**`GROUPING SETS` / `ROLLUP` / `CUBE`** — несколько уровней агрегации за один проход (итоги + подытоги):

```sql
-- суммы по (country, city), по country, и общий итог — одним запросом
SELECT country, city, sum(amount)
FROM orders JOIN users ON users.id = orders.user_id
GROUP BY ROLLUP (country, city);
```

Полезные агрегаты: `sum`, `avg`, `min`, `max`, `string_agg(name, ', ')` (склейка в строку), `array_agg(id)` (в массив), `bool_and`/`bool_or`, `jsonb_agg`.

<details>
<summary>Пример: string_agg / array_agg</summary>

```sql
SELECT user_id,
       array_agg(amount ORDER BY amount)       AS amounts,    -- собрать в массив
       string_agg(status, ',' ORDER BY amount) AS statuses    -- склеить в строку
FROM orders GROUP BY user_id;
```

```text
user_id | amounts     | statuses
   1    | {30,50,100} | paid,failed,paid
   2    | {200}       | paid
```

Оба схлопывают группу, но не в число, а в массив / строку — удобно «собрать все значения группы в одну ячейку».

</details>

---

## Оконные функции

Оконная функция считает агрегат **по окну строк, но не схлопывает их** — в отличие от `GROUP BY`, каждая исходная строка остаётся, а рядом появляется вычисленное значение.

```sql
-- к каждому заказу — его доля в сумме заказов пользователя, и порядковый номер
SELECT
    o.*,
    sum(amount)  OVER (PARTITION BY user_id)                        AS user_total,
    row_number() OVER (PARTITION BY user_id ORDER BY created_at)    AS seq
FROM orders o;
```

`OVER (PARTITION BY … ORDER BY …)`: `PARTITION BY` — на какие группы бить (как `GROUP BY`, но строки остаются), `ORDER BY` — порядок внутри окна (нужен для ранжирования и рамок).

**Ранжирование:**

| Функция | Что даёт |
|---|---|
| `row_number()` | сквозной номер 1,2,3… (уникальный) |
| `rank()` | ранг с пропусками при ничьих: 1,1,3 |
| `dense_rank()` | ранг без пропусков: 1,1,2 |
| `ntile(n)` | разбить на n примерно равных корзин |

<details>
<summary>Пример: rank / dense_rank / row_number + PARTITION</summary>

`players`:

```text
name | team | score
 A   | red  |  100
 B   | red  |  100
 C   | red  |   90
 D   | blue |   80
```

```sql
SELECT name, team, score,
       rank()       OVER (PARTITION BY team ORDER BY score DESC) AS rnk,
       dense_rank() OVER (PARTITION BY team ORDER BY score DESC) AS drnk,
       row_number() OVER (PARTITION BY team ORDER BY score DESC) AS rn
FROM players;
```

```text
name | team | score | rnk | drnk | rn
 A   | red  |  100  |  1  |  1   | 1
 B   | red  |  100  |  1  |  1   | 2   ← ничья по score: rank/dense_rank совпали, row_number всё равно разный
 C   | red  |   90  |  3  |  2   | 3   ← rank ПРОПУСТИЛ 2 (было две «1»); dense_rank не пропускает
 D   | blue |   80  |  1  |  1   | 1   ← новый team → PARTITION BY отсчитывает заново
```

`ntile(2) OVER (ORDER BY score DESC)` разбил бы всех на 2 корзины: A,B → `1`, C,D → `2`.

</details>

```sql
-- top-3 заказа каждого пользователя (частая задача top-N-per-group)
SELECT * FROM (
    SELECT o.*, row_number() OVER (PARTITION BY user_id ORDER BY amount DESC) AS rn
    FROM orders o
) t
WHERE rn <= 3;
```

**Смещение и границы:**

```sql
-- сравнить с предыдущей строкой (LAG) — например, дельта суммы день ко дню
SELECT day, revenue,
       revenue - lag(revenue) OVER (ORDER BY day) AS delta
FROM daily;

lead(x)         -- следующая строка
first_value(x)  -- первое значение окна
last_value(x)   -- последнее (осторожно с рамкой, см. ниже)
```

<details>
<summary>Пример: lag / delta / running total</summary>

`daily`:

```text
day | revenue
 d1 |  100
 d2 |  150
 d3 |  120
```

```sql
SELECT day, revenue,
       lag(revenue) OVER (ORDER BY day)             AS prev,
       revenue - lag(revenue) OVER (ORDER BY day)   AS delta,
       sum(revenue) OVER (ORDER BY day
             ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS running
FROM daily;
```

```text
day | revenue | prev | delta | running
 d1 |   100   | NULL | NULL  |  100
 d2 |   150   | 100  |  50   |  250
 d3 |   120   | 150  | -30   |  370
```

`lag` берёт предыдущую строку окна (для первой — `NULL`); `running` копит сумму от начала окна до текущей строки (это и задаёт рамка `ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW`).

</details>

**Рамка окна (frame)** — сколько строк вокруг текущей учитывать. Скользящая сумма:

```sql
SELECT day, revenue,
       sum(revenue) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS rolling_7d
FROM daily;
```

Ловушка: при `ORDER BY` без явной рамки дефолт — `RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW`, поэтому `last_value()` вернёт не конец окна, а текущую строку. Для «последнего в окне» задавай рамку явно (`ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING`).

**Окно vs `GROUP BY`:** `GROUP BY` возвращает **одну строку на группу**; оконная функция — **все строки** плюс агрегат рядом. Нужен и детальный список, и итог по группе → окно.

---

## CTE (WITH)

`WITH` выносит подзапрос в именованный блок — читается сверху вниз, как переменные:

```sql
WITH paid AS (
    SELECT user_id, sum(amount) AS total
    FROM orders WHERE status = 'paid'
    GROUP BY user_id
)
SELECT u.name, paid.total
FROM users u JOIN paid ON paid.user_id = u.id
WHERE paid.total > 1000;
```

- **`MATERIALIZED` / `NOT MATERIALIZED`**: по умолчанию PostgreSQL **встраивает** (инлайнит) CTE в запрос, если он используется один раз, — планировщик оптимизирует целиком. `WITH x AS MATERIALIZED (…)` заставляет вычислить CTE отдельно и один раз (полезно, если он дорогой и используется многократно, или нужен барьер оптимизации). `NOT MATERIALIZED` — форсировать инлайн.
- **`WITH RECURSIVE`** — иерархии и графы (дерево категорий, цепочка менеджеров):

```sql
WITH RECURSIVE tree AS (
    SELECT id, parent_id, name, 1 AS depth
    FROM categories WHERE parent_id IS NULL      -- якорь: корни
  UNION ALL
    SELECT c.id, c.parent_id, c.name, tree.depth + 1
    FROM categories c JOIN tree ON c.parent_id = tree.id   -- рекурсивный шаг
)
SELECT * FROM tree ORDER BY depth;
```

- **Data-modifying CTE** — `INSERT`/`UPDATE`/`DELETE ... RETURNING` внутри `WITH` (переместить строки одним запросом):

```sql
WITH moved AS (
    DELETE FROM orders WHERE created_at < '2023-01-01' RETURNING *
)
INSERT INTO orders_archive SELECT * FROM moved;
```

---

## Операции над множествами

Комбинируют результаты двух `SELECT` с **совпадающими** колонками:

| Оператор | Что делает |
|---|---|
| `UNION` | объединение, **с удалением дублей** (дороже — сортирует/хэширует) |
| `UNION ALL` | объединение **без** удаления дублей (быстрее, чаще нужно именно оно) |
| `INTERSECT` | строки, присутствующие в обоих |
| `EXCEPT` | строки из первого, которых нет во втором (разность) |

```sql
SELECT id FROM active_users
UNION ALL
SELECT id FROM trial_users;
```

Правило: если дубли невозможны или не мешают — всегда `UNION ALL` (не платить за дедупликацию).

---

## Подзапросы

- **Скалярный** — возвращает одно значение, используется как выражение:
  ```sql
  SELECT name, (SELECT count(*) FROM orders o WHERE o.user_id = u.id) AS orders_cnt
  FROM users u;
  ```
- **Коррелированный** — ссылается на внешний запрос (выполняется для каждой внешней строки; `EXISTS` — его частный случай).
- **`IN` / `ANY` / `ALL`**:
  ```sql
  WHERE amount > ALL (SELECT amount FROM orders WHERE status='refunded')  -- больше всех
  WHERE user_id = ANY (ARRAY[1,2,3])                                       -- = IN (1,2,3)
  ```

Когда join, когда подзапрос: если нужны **колонки** второй таблицы — join; если только **факт наличия/фильтр** — `EXISTS`/`IN`. Планировщик часто превращает одно в другое, но `EXISTS`/`NOT EXISTS` безопаснее по `NULL` и не размножают строки.

---

## Полезные конструкции

**Три формы `DISTINCT`** — их часто путают:

| Форма | Что делает |
|---|---|
| `SELECT DISTINCT a, b` | убирает дубли по **всему набору** выбранных столбцов (уникальные комбинации) |
| `SELECT DISTINCT ON (a) …` | **PostgreSQL-специфично**: по одной строке на каждое значение `a` (какой именно — задаёт `ORDER BY`) |
| `count(DISTINCT a)`, `array_agg(DISTINCT a)` | `DISTINCT` **внутри агрегата**: считать/собирать только уникальные значения |

Ключевая разница `DISTINCT` vs `DISTINCT ON`: обычный `DISTINCT` схлопывает **полностью одинаковые** строки, а `DISTINCT ON (a)` оставляет **по одной строке на группу `a`**, возвращая при этом **все** её столбцы (то, что оконкой делают через `row_number() = 1`).

```sql
SELECT DISTINCT status FROM orders;          -- список уникальных статусов

-- последний заказ каждого пользователя (первая строка на user_id по порядку)
SELECT DISTINCT ON (user_id) *
FROM orders
ORDER BY user_id, created_at DESC;   -- ORDER BY обязан начинаться со столбцов DISTINCT ON
```

<details>
<summary>Пример: DISTINCT ON</summary>

`orders`:

```text
id | user_id | created_at | amount
 1 |    1    |  10:00     |  100
 2 |    1    |  12:00     |   50    ← самый свежий у user 1
 3 |    2    |  09:00     |  200    ← единственный у user 2
```

`SELECT DISTINCT ON (user_id) * ... ORDER BY user_id, created_at DESC` →

```text
id | user_id | created_at | amount
 2 |    1    |  12:00     |   50
 3 |    2    |  09:00     |  200
```

По одной строке на `user_id` — с самой свежей `created_at` (её выбрал `ORDER BY ... created_at DESC`).

</details>

**`CASE`** — условное выражение:

```sql
SELECT name,
       CASE WHEN age < 18 THEN 'minor'
            WHEN age < 65 THEN 'adult'
            ELSE 'senior' END AS bucket
FROM users;
```

**Работа с NULL:**

```sql
COALESCE(phone, 'нет')      -- первое не-NULL значение
NULLIF(a, 0)                -- NULL, если a = 0 (защита от деления на ноль)
GREATEST(a, b, c) / LEAST(...)  -- макс/мин из аргументов (не агрегат)
```

**`VALUES` как таблица** — набор строк «на лету» (для JOIN/маппинга без временной таблицы):

```sql
SELECT * FROM (VALUES ('paid', 1), ('failed', 0)) AS s(status, weight);
```

Idempotent-вставка `INSERT … ON CONFLICT` — в [sql-tasks: find-and-delete-duplicates](./sql-tasks/find-and-delete-duplicates.md) и highload-сценариях.

---

## Interview-ready answer

**1. Чем `LEFT JOIN` отличается от `INNER`, и как найти строки без пары?**

- `INNER` оставляет только совпавшие; `LEFT` — все строки левой, без пары справа `NULL`. Anti-join: `LEFT JOIN … WHERE right.id IS NULL` или `NOT EXISTS`. Ловушка: условие на правую таблицу в `WHERE` превращает `LEFT` в `INNER` — фильтр правой стороны кладут в `ON`.

**2. `WHERE` vs `HAVING`?**

- `WHERE` фильтрует строки до группировки, `HAVING` — группы после агрегации. Условие не про агрегат — всегда в `WHERE` (дешевле).

**3. Чем оконная функция отличается от `GROUP BY`?**

- `GROUP BY` схлопывает группу в одну строку; оконная функция считает агрегат по окну (`OVER (PARTITION BY …)`), но **сохраняет все строки**. Для top-N-per-group — `row_number() OVER (PARTITION BY … ORDER BY …)`.

**4. `NOT IN` vs `NOT EXISTS`?**

- `NOT IN` с подзапросом, где встретится `NULL`, даёт **пустой** результат (трёхзначная логика). `NOT EXISTS` устойчив к `NULL` и обычно эффективнее — предпочтительный anti-join.

**5. `UNION` vs `UNION ALL`?**

- `UNION` удаляет дубли (сортировка/хэш — дорого), `UNION ALL` — нет. Если дубли не мешают, всегда `UNION ALL`.

**6. Что такое `LATERAL` и зачем?**

- Подзапрос в `FROM`, который видит столбцы левой таблицы → выполняется для каждой её строки. Основной кейс — top-N на группу с `LIMIT` внутри, часто эффективнее оконки при малом N и наличии индекса.
