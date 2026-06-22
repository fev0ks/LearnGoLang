# 184. Department Highest Salary (Medium)

> LeetCode 184 · https://leetcode.com/problems/department-highest-salary/
> Тема: максимум внутри группы, ties, коррелированный подзапрос vs оконная функция.

## Условие

```
Employee:   id (PK) | name | salary | departmentId (FK → Department.id)
Department: id (PK) | name
```

Для каждого отдела вернуть сотрудника(ов) с максимальной зарплатой:
`Department, Employee, Salary`. Если максимум делят несколько человек — вывести всех.

## Почему именно это спрашивают

«Top-1 внутри каждой группы» с корректной обработкой ties. Нельзя просто
`GROUP BY departmentId` + `MAX(salary)` — потеряется имя сотрудника.

## Решение через коррелированный подзапрос

```sql
SELECT d.name AS Department,
       e.name AS Employee,
       e.salary AS Salary
FROM Employee e
JOIN Department d ON d.id = e.departmentId
WHERE e.salary = (
  SELECT MAX(salary)
  FROM Employee
  WHERE departmentId = e.departmentId
);
```

Для каждого сотрудника подзапрос считает максимум по его отделу; остаются только те,
кто этому максимуму равен. Все сотрудники с максимальной зарплатой попадают в ответ.

## Альтернатива через оконную функцию

```sql
SELECT d.name AS Department,
       t.name AS Employee,
       t.salary AS Salary
FROM (
  SELECT name, salary, departmentId,
         RANK() OVER (PARTITION BY departmentId ORDER BY salary DESC) AS rnk
  FROM Employee
) t
JOIN Department d ON d.id = t.departmentId
WHERE t.rnk = 1;
```

`PARTITION BY departmentId` считает ранг внутри каждого отдела отдельно. `RANK` (а не
`ROW_NUMBER`) важно: при ties все максимальные получат ранг 1 и попадут в ответ.

## Подводные камни

- `ROW_NUMBER` вместо `RANK` оставит только одного из равных по максимуму — потеря строк.
- Коррелированный подзапрос пересчитывается для строк; на больших таблицах оконный
  вариант обычно эффективнее (один проход с сортировкой по партиции).

## Что проверяет на собесе

Top-N-per-group, разница `RANK`/`ROW_NUMBER`, выбор между подзапросом и окном.
Продолжение — задача 185 (топ-3 на отдел), [см. hard](../hard/0185-department-top-three-salaries.md).
