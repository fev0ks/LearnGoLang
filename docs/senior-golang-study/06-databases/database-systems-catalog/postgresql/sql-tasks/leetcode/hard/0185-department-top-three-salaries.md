# 185. Department Top Three Salaries (Hard)

> LeetCode 185 · https://leetcode.com/problems/department-top-three-salaries/
> Тема: top-N-per-group, `DENSE_RANK` с `PARTITION BY`, «различные» зарплаты.

## Условие

```
Employee:   id (PK) | name | salary | departmentId (FK → Department.id)
Department: id (PK) | name
```

Для каждого отдела вернуть сотрудников, входящих в **три самые высокие различные**
зарплаты отдела (high earners). Вывод: `Department, Employee, Salary`.

«Три различные зарплаты» — ключевое: если в отделе зарплаты 90, 90, 85, 80, 75, то
топ-3 различных = {90, 85, 80}, и оба сотрудника с 90 попадают в ответ.

## Почему именно это спрашивают

Обобщение задачи 184 на top-N с N>1 и явным требованием «различных» значений.
Прямое попадание под `DENSE_RANK` (равные зарплаты делят ранг, нумерация без пропусков).

## Решение

```sql
SELECT d.name AS Department,
       t.name AS Employee,
       t.salary AS Salary
FROM (
  SELECT name, salary, departmentId,
         DENSE_RANK() OVER (
           PARTITION BY departmentId
           ORDER BY salary DESC
         ) AS rnk
  FROM Employee
) t
JOIN Department d ON d.id = t.departmentId
WHERE t.rnk <= 3;
```

Разбор:
- `PARTITION BY departmentId` — ранжируем зарплаты внутри каждого отдела отдельно.
- `DENSE_RANK` — равные зарплаты получают один ранг, следующая идёт без пропуска,
  поэтому «три различных зарплаты» = `rnk <= 3` (а не «три сотрудника»).
- `WHERE rnk <= 3` фильтрует уже после оконного вычисления (окна нельзя в `WHERE`
  напрямую, поэтому нужен подзапрос/CTE).

## Почему именно DENSE_RANK, а не RANK/ROW_NUMBER

| Функция | Зарплаты 90,90,85,80 | `<= 3` отберёт |
| --- | --- | --- |
| `ROW_NUMBER` | 1,2,3,4 | 90, 90, 85 — потеряли 80, хотя это лишь 3-я различная |
| `RANK` | 1,1,3,4 | 90, 90 — потеряли 85 и 80 (после tie скачок до 3) |
| `DENSE_RANK` | 1,1,2,3 | 90, 90, 85, 80 — корректно: три различные зарплаты |

## Подводные камни

- Оконную функцию нельзя использовать в `WHERE` той же выборки — оборачиваем в
  подзапрос/CTE и фильтруем снаружи.
- Спутать «топ-3 различных зарплат» с «топ-3 сотрудников» — разные ответы при ties.

## Что проверяет на собесе

Уверенное владение оконными функциями и осознанный выбор `DENSE_RANK`. База — задача
184, [см. medium](../medium/0184-department-highest-salary.md).
