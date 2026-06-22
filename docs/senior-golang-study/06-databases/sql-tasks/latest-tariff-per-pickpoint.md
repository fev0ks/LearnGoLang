# Latest Tariff Per Pickpoint (Medium)

> Практическая задача.
> Тема: latest-row-per-group (последняя строка в каждой группе), `DISTINCT ON`,
> оконные функции, tie-breaker по `id`.

## Условие

```sql
CREATE TABLE pickpoint_tariff (
    id           bigserial,
    created      timestamp default now(),
    pickpoint_id bigint,
    percent      numeric
);
```

Таблица — журнал изменений тарифа (процента) по пунктам выдачи. Каждая новая запись
для `pickpoint_id` — это новое значение `percent` с момента `created`. Старые строки
не удаляются, поэтому по одному пункту может быть много версий.

Пунктов выдачи: 1, 2, 3, 4, 5.

```
| id | created     | pickpoint_id | percent |
|----|-------------|--------------|---------|
| 1  | 02.10 - 1   | 1            | 2%      |
| 2  | 02.10 - 2   | 2            | 2%      |
| 3  | 02.10 - 3   | 3            | 2%      |
| 4  | 02.10 - 4   | 4            | 2%      |
| 5  | 02.10 - 5   | 5            | 2%      |
| 6  | 03.10 - 1   | 1            | 3%      |
| 7  | 03.10 - 4   | 4            | 1%      |
```

Нужно вывести **актуальный (последний) процент по каждому пункту выдачи**. Для данных
выше ожидается:

```
| pickpoint_id | percent |
|--------------|---------|
| 1            | 3%      |   ← перебито 03.10
| 2            | 2%      |
| 3            | 2%      |
| 4            | 1%      |   ← перебито 03.10
| 5            | 2%      |
```

## Почему это не решается простым GROUP BY

Соблазн написать `SELECT pickpoint_id, MAX(created) ... GROUP BY pickpoint_id`. Но
тогда `percent` нельзя положить в `SELECT` напрямую — он не под агрегатом и не в
`GROUP BY`. А `MAX(percent)` вернёт *максимальный процент*, а не *процент из последней
строки*. Это классическая задача «взять всю строку, где значение в группе максимально»
(greatest-N-per-group), а не «взять максимум значения».

## Решение 1: DISTINCT ON (идиома PostgreSQL)

```sql
SELECT DISTINCT ON (pickpoint_id)
       pickpoint_id,
       percent,
       created
FROM pickpoint_tariff
ORDER BY pickpoint_id, created DESC, id DESC;
```

`DISTINCT ON (pickpoint_id)` оставляет по одной строке на каждый `pickpoint_id` —
**первую** в заданном `ORDER BY`. Поэтому порядок обязан начинаться с `pickpoint_id`,
а дальше идёт критерий «свежести»: `created DESC`, затем `id DESC` как tie-breaker.
Самый короткий и быстрый вариант в Postgres.

## Решение 2: оконная функция ROW_NUMBER (переносимо между СУБД)

```sql
SELECT pickpoint_id, percent, created
FROM (
  SELECT pickpoint_id, percent, created,
         ROW_NUMBER() OVER (
           PARTITION BY pickpoint_id
           ORDER BY created DESC, id DESC
         ) AS rn
  FROM pickpoint_tariff
) t
WHERE rn = 1;
```

`PARTITION BY pickpoint_id` нумерует строки внутри каждого пункта, `ORDER BY created
DESC, id DESC` ставит самую свежую на `rn = 1`. Работает в любой современной СУБД
(Postgres, MySQL 8+, SQL Server). Оконную функцию нельзя фильтровать в `WHERE` —
поэтому подзапрос.

## Решение 3: join с агрегатом (если оконных функций нет)

```sql
SELECT t.pickpoint_id, t.percent, t.created
FROM pickpoint_tariff t
JOIN (
  SELECT pickpoint_id, MAX(created) AS max_created
  FROM pickpoint_tariff
  GROUP BY pickpoint_id
) m ON m.pickpoint_id = t.pickpoint_id
   AND m.max_created  = t.created;
```

Минус виден сразу: если у одного пункта две строки с **одинаковым** `created`
(а `created` имеет `default now()` и при пакетной вставке может совпасть), join вернёт
обе — задвоение. Варианты 1 и 2 этого лишены за счёт tie-breaker по `id`.

## Tie-breaker: зачем `id DESC`

`created` — `timestamp` без гарантии уникальности. Две версии тарифа, вставленные в
одну транзакцию/миллисекунду, получат одинаковый `created`. Без вторичной сортировки
по `id` «последняя» строка стала бы недетерминированной. `bigserial id` монотонно
растёт, поэтому `id DESC` надёжно выбирает реально последнюю вставку.

## Подводные камни

- `MAX(percent)` вместо «percent из последней строки» — самая частая ошибка. Нужна
  именно последняя *строка*, а не максимум поля.
- Отсутствие tie-breaker по `id` → недетерминированный результат при равных `created`.
- Решение через join с `MAX(created)` даёт дубликаты при коллизии `created`.
- Если по пункту вообще нет строк, его не будет в выводе. Когда нужны все 5 пунктов
  даже без тарифа — нужен отдельный список пунктов и `LEFT JOIN` к нему.

## Что проверяет на собесе

Распознавание паттерна greatest-N-per-group и знание идиомы `DISTINCT ON` / оконного
`ROW_NUMBER`, плюс внимание к недетерминизму при неуникальном `created`. Родственные
задачи: [184. Department Highest Salary](leetcode/medium/0184-department-highest-salary.md),
[185. Department Top Three Salaries](leetcode/hard/0185-department-top-three-salaries.md).
