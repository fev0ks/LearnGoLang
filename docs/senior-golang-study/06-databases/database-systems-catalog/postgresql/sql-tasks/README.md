# SQL Tasks

Задачи на SQL с разбором и решениями — для подготовки к собеседованиям и тренировки
запросов. Теория по реляционным БД и SQL лежит рядом:
[postgresql](../README.md).

## Структура

```
sql-tasks/
├── README.md          ← этот файл (общий индекс)
└── leetcode/          ← задачи с официальным номером LeetCode, по сложности
    ├── easy/
    ├── medium/
    └── hard/
```

- **`leetcode/`** — настоящие задачи LeetCode с разбивкой по сложности. Свой индекс:
  [leetcode/README.md](leetcode/README.md).
- **Кастомные задачи** (без номера LeetCode) кладём отдельными
  файлами прямо сюда, в корень `sql-tasks/`, и добавляем в таблицу ниже.

Каждая задача — отдельный `.md`-файл: условие со схемой таблиц, разбор «от почему»,
решение, альтернативы, подводные камни, «что проверяет на собесе».

## Как тренироваться

1. Прочитать условие, не подглядывая в решение. Набросать запрос самому.
2. Сравнить **подход**, а не код-в-код: какие edge cases упущены (NULL, ties, пустой
   результат)?
3. Разобрать альтернативы — когда оконная функция, когда подзапрос, когда JOIN.
4. Проговорить решение вслух, как интервьюеру.

## Индекс задач LeetCode

Полный индекс с темами — в [leetcode/README.md](leetcode/README.md). Кратко готовые:

| № | Задача | Сложность | Тема | Решение |
| --- | --- | --- | --- | --- |
| 175 | Combine Two Tables | Easy | LEFT JOIN | [easy/0175](leetcode/easy/0175-combine-two-tables.md) |
| 181 | Employees Earning More Than Managers | Easy | Self-join | [easy/0181](leetcode/easy/0181-employees-earning-more-than-managers.md) |
| 182 | Duplicate Emails | Easy | GROUP BY / HAVING | [easy/0182](leetcode/easy/0182-duplicate-emails.md) |
| 183 | Customers Who Never Order | Easy | Anti-join, NOT IN + NULL | [easy/0183](leetcode/easy/0183-customers-who-never-order.md) |
| 176 | Second Highest Salary | Medium | Подзапрос, LIMIT/OFFSET, NULL | [medium/0176](leetcode/medium/0176-second-highest-salary.md) |
| 178 | Rank Scores | Medium | RANK vs DENSE_RANK | [medium/0178](leetcode/medium/0178-rank-scores.md) |
| 180 | Consecutive Numbers | Medium | LEAD/LAG, серии | [medium/0180](leetcode/medium/0180-consecutive-numbers.md) |
| 184 | Department Highest Salary | Medium | Top-1-per-group | [medium/0184](leetcode/medium/0184-department-highest-salary.md) |
| 185 | Department Top Three Salaries | Hard | Top-N-per-group, DENSE_RANK | [hard/0185](leetcode/hard/0185-department-top-three-salaries.md) |
| 601 | Human Traffic of Stadium | Hard | Gaps-and-islands | [hard/0601](leetcode/hard/0601-human-traffic-of-stadium.md) |

## Темы, которые покрывают задачи

| Тема | Где встречается | Ключевой приём |
| --- | --- | --- |
| Типы JOIN | 175, 183 | `LEFT JOIN`, фильтр `IS NULL` против anti-join |
| Self-join | 181 | таблица + сама себя через алиасы |
| Агрегация | 182 | `GROUP BY` + `HAVING` против `WHERE` |
| Трёхзначная логика | 183 | ловушка `NOT IN` + `NULL` |
| Top-N | 176, 184, 185 | подзапрос vs оконные функции |
| Latest-row-per-group | Latest Tariff Per Pickpoint | `DISTINCT ON`, `ROW_NUMBER`, tie-breaker по `id` |
| Оконные функции | 178, 184, 185, 601 | `RANK` / `DENSE_RANK` / `ROW_NUMBER`, `PARTITION BY` |
| Соседние строки | 180 | `LEAD` / `LAG` |
| Gaps-and-islands | 601 | трюк `id - ROW_NUMBER()` |
| Поиск/удаление дублей | 182, Find And Delete Duplicates | `GROUP BY/HAVING`, `DELETE ... USING`, `ctid`, UNIQUE |
| MVCC и конкурентность | Concurrent Full-Table Update | снапшот, row lock (`xmax`), deadlock, EvalPlanQual |

## Кастомные задачи

Практические задачи без номера LeetCode. Формат файла повторяет задачи в
`leetcode/`, только без номера: `<kebab-имя>.md`.

| Задача | Сложность | Тема | Решение |
| --- | --- | --- | --- |
| Latest Tariff Per Pickpoint | Medium | Latest-row-per-group, `DISTINCT ON` | [latest-tariff-per-pickpoint](latest-tariff-per-pickpoint.md) |
| Find And Delete Duplicates | Medium | Поиск/удаление дублей, `ctid`, UNIQUE | [find-and-delete-duplicates](find-and-delete-duplicates.md) |
| Concurrent Full-Table Update | Hard | MVCC, row lock, deadlock, bloat/WAL | [concurrent-full-table-update](concurrent-full-table-update.md) |
