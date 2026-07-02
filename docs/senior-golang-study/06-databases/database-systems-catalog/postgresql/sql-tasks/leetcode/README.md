# LeetCode SQL

Решения настоящих SQL-задач LeetCode, разложенные по уровню сложности. Аналог
[go-задач LeetCode](../../../../../../../topics/06-algorithms-and-tasks/leetcode), только
вместо Go — SQL.

## Структура

```
leetcode/
├── easy/
├── medium/
└── hard/
```

Каждая задача — отдельный файл `<номер>-<kebab-имя>.md` (номер с ведущими нулями для
сортировки). Уровень сложности кодируется родительской папкой. Внутри файла: условие
со схемой таблиц, разбор «от почему», решение, альтернативы, подводные камни.

## Диалект

Решения написаны на стандартном SQL, дружелюбном к PostgreSQL (основная СУБД в этом
гайде — см. [postgresql](../../README.md)).
На LeetCode дефолтный движок — MySQL; различия в синтаксисе (например кавычки вокруг
зарезервированного `rank`) отмечены прямо в задачах.

## Индекс задач

Колонка «Решение» — ссылка для готовых задач или 🔜 для запланированных.

### Easy

| № | Задача | Тема | Решение |
| --- | --- | --- | --- |
| 175 | Combine Two Tables | LEFT JOIN | [0175-combine-two-tables](easy/0175-combine-two-tables.md) |
| 181 | Employees Earning More Than Managers | Self-join | [0181-employees-earning-more-than-managers](easy/0181-employees-earning-more-than-managers.md) |
| 182 | Duplicate Emails | GROUP BY / HAVING | [0182-duplicate-emails](easy/0182-duplicate-emails.md) |
| 183 | Customers Who Never Order | Anti-join, NOT IN + NULL | [0183-customers-who-never-order](easy/0183-customers-who-never-order.md) |
| 196 | Delete Duplicate Emails | DELETE + self-join | 🔜 |
| 595 | Big Countries | WHERE / OR | 🔜 |
| 620 | Not Boring Movies | WHERE + ORDER BY | 🔜 |
| 1757 | Recyclable and Low Fat Products | WHERE | 🔜 |

### Medium

| № | Задача | Тема | Решение |
| --- | --- | --- | --- |
| 176 | Second Highest Salary | Подзапрос, LIMIT/OFFSET, NULL | [0176-second-highest-salary](medium/0176-second-highest-salary.md) |
| 177 | Nth Highest Salary | Функция / OFFSET | 🔜 |
| 178 | Rank Scores | RANK vs DENSE_RANK | [0178-rank-scores](medium/0178-rank-scores.md) |
| 180 | Consecutive Numbers | LEAD/LAG, серии | [0180-consecutive-numbers](medium/0180-consecutive-numbers.md) |
| 184 | Department Highest Salary | Top-1-per-group | [0184-department-highest-salary](medium/0184-department-highest-salary.md) |
| 550 | Game Play Analysis IV | Self-join по датам | 🔜 |
| 570 | Managers with at Least 5 Reports | GROUP BY / HAVING | 🔜 |
| 1934 | Confirmation Rate | LEFT JOIN + AVG | 🔜 |

### Hard

| № | Задача | Тема | Решение |
| --- | --- | --- | --- |
| 185 | Department Top Three Salaries | Top-N-per-group, DENSE_RANK | [0185-department-top-three-salaries](hard/0185-department-top-three-salaries.md) |
| 262 | Trips and Users | JOIN + условная агрегация | 🔜 |
| 569 | Median Employee Salary | Оконные + медиана | 🔜 |
| 601 | Human Traffic of Stadium | Gaps-and-islands | [0601-human-traffic-of-stadium](hard/0601-human-traffic-of-stadium.md) |

## Что относится сюда, а что нет

Сюда кладём только задачи с официальным номером LeetCode. Кастомные задачи «по
мотивам собеса» (без номера LeetCode) живут уровнем выше — в [../](../README.md).
