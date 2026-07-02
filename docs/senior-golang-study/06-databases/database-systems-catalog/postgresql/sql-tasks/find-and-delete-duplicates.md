# Find And Delete Duplicates (Medium)

> Практическая задача.
> Тема: поиск дубликатов, удаление с сохранением одной строки, `ctid`,
> оконные функции, защита от дублей на будущее.

## Условие

```sql
CREATE TABLE events (
    id         bigserial PRIMARY KEY,
    user_id    bigint,
    event_type text,
    created    timestamp default now()
);
```

Дубликатом считаем строки с одинаковой парой `(user_id, event_type)`. Нужно:
1. **Найти** дубликаты (какие пары задвоены и сколько раз).
2. **Удалить** дубликаты, оставив по одной строке на пару (например самую раннюю —
   с минимальным `id`).

## Часть 1: поиск дубликатов

Сколько раз задвоена каждая пара:

```sql
SELECT user_id, event_type, COUNT(*) AS cnt
FROM events
GROUP BY user_id, event_type
HAVING COUNT(*) > 1;
```

Если нужны **сами строки-дубликаты** (со всеми колонками, не только ключ группировки)
— оконный `COUNT(*)` без схлопывания:

```sql
SELECT *
FROM (
  SELECT *,
         COUNT(*) OVER (PARTITION BY user_id, event_type) AS cnt
  FROM events
) t
WHERE cnt > 1
ORDER BY user_id, event_type, id;
```

Разница принципиальная: `GROUP BY` отвечает «какие пары дублируются», оконный
`COUNT(*) OVER (...)` — «вот конкретные строки, которые надо разрулить», не теряя `id`.

## Часть 2: удаление дубликатов

### Вариант A: DELETE ... USING (Postgres, есть PK)

```sql
DELETE FROM events a
USING events b
WHERE a.user_id    = b.user_id
  AND a.event_type = b.event_type
  AND a.id > b.id;
```

Соединяем таблицу саму с собой по ключу дубликата и удаляем те строки, у которых
есть «брат» с меньшим `id`. Остаётся строка с минимальным `id` в каждой группе.
Чтобы оставить **самую свежую** — поменять на `a.id < b.id` (или сравнивать `created`).

### Вариант B: оконная функция (переносимо)

```sql
DELETE FROM events
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (
             PARTITION BY user_id, event_type
             ORDER BY id
           ) AS rn
    FROM events
  ) t
  WHERE rn > 1
);
```

`ROW_NUMBER` нумерует строки внутри группы; `rn = 1` оставляем, `rn > 1` (все кроме
первой) удаляем. Поменяв `ORDER BY id` на `ORDER BY created DESC, id DESC`, легко
выбрать, какую именно строку считать «оставляемой».

### Вариант C: ctid (Postgres, когда нет уникального ключа)

Если PK/уникального столбца нет вообще и дублируются строки целиком, помогает
системный столбец `ctid` (физический адрес версии строки):

```sql
DELETE FROM events
WHERE ctid NOT IN (
  SELECT MIN(ctid)
  FROM events
  GROUP BY user_id, event_type
);
```

`ctid` всегда уникален и не `NULL`, поэтому здесь `NOT IN` безопасен. Но `ctid`
меняется при `UPDATE`/`VACUUM FULL` — использовать только внутри одной транзакции и
никогда не хранить.

## Часть 3: чтобы дубли не появлялись снова

Удаление — половина дела; без ограничения они вернутся. После очистки:

```sql
-- уникальность на уровне схемы
CREATE UNIQUE INDEX CONCURRENTLY idx_events_uniq
    ON events (user_id, event_type);

-- и идемпотентная вставка вместо «вставил-задвоил»
INSERT INTO events (user_id, event_type)
VALUES (42, 'click')
ON CONFLICT (user_id, event_type) DO NOTHING;
```

## Подводные камни

- **`NOT IN` + `NULL`**: если в подзапросе для `NOT IN` окажется `NULL`, результат
  обнулится для всех (трёхзначная логика, см.
  [183. Customers Who Never Order](leetcode/easy/0183-customers-who-never-order.md)).
  В варианте C `ctid` не `NULL` — безопасно; в варианте B подзапрос отдаёт непустые `id`.
- **Какую строку оставить** — решить осознанно (min/max `id`, самая свежая `created`).
  По умолчанию `a.id > b.id` хранит самую раннюю.
- **MySQL**: нельзя удалять из таблицы, напрямую упомянутой в подзапросе `IN` — нужно
  обернуть подзапрос ещё одним уровнем (`SELECT * FROM (...) x`) или использовать
  multi-table `DELETE` с join.
- **Большие таблицы**: разовый `DELETE` миллионов строк раздувает WAL и держит локи.
  Удалять батчами (`... WHERE id IN (SELECT ... LIMIT 10000)` в цикле).
- **Создание UNIQUE индекса** на таблице с ещё не удалёнными дублями упадёт — сначала
  чистка, потом индекс. `CONCURRENTLY` — чтобы не блокировать запись на проде.

## Что проверяет на собесе

Различие «найти дубли» (GROUP BY/HAVING) и «удалить, оставив одну» (self-join / окно /
ctid), знание ловушки `NOT IN`/`NULL` и понимание, что без `UNIQUE`-ограничения
проблема вернётся. Родственное: [182. Duplicate Emails](leetcode/easy/0182-duplicate-emails.md),
идемпотентность вставок — [relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md](../../../relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md).
