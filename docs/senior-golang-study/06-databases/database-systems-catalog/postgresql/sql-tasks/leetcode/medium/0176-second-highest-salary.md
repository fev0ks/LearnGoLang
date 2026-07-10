# 176. Second Highest Salary (Medium)

> LeetCode 176 · https://leetcode.com/problems/second-highest-salary/
> Тема: подзапрос, `DISTINCT`, `LIMIT/OFFSET`, обработка «нет результата → NULL».

## Условие

```
Employee: id (PK) | salary
```

Вернуть вторую по величине **различную** зарплату. Если её нет (все зарплаты равны
или строка одна) — вернуть `NULL`.

## Почему именно это спрашивают

Проверяют два момента: (1) `DISTINCT`, чтобы дубликаты максимума не считались за
разные места; (2) корректный возврат `NULL`, когда второй зарплаты нет — частая
причина «правильный запрос, но не проходит тест».

## Решение через LIMIT/OFFSET

```sql
SELECT (
  SELECT DISTINCT salary
  FROM Employee
  ORDER BY salary DESC
  LIMIT 1 OFFSET 1
) AS SecondHighestSalary;
```

Внешний `SELECT (...)` — ключ к `NULL`: если подзапрос не вернул строк, скалярный
подзапрос даёт `NULL`, и результат — одна строка с `NULL`. Без обёртки запрос вернул
бы 0 строк, что тест не примет.

`OFFSET 1` пропускает максимум, `LIMIT 1` берёт следующую различную зарплату.

## Альтернатива через подзапрос с MAX

```sql
SELECT MAX(salary) AS SecondHighestSalary
FROM Employee
WHERE salary < (SELECT MAX(salary) FROM Employee);
```

`MAX` по пустому множеству сам возвращает `NULL`, поэтому обёртка не нужна.
Хорошо обобщается слабо (для N-й зарплаты громоздко).

## Альтернатива через оконную функцию (обобщается на N-ю)

```sql
SELECT DISTINCT salary AS SecondHighestSalary
FROM (
  SELECT salary, DENSE_RANK() OVER (ORDER BY salary DESC) AS rnk
  FROM Employee
) t
WHERE rnk = 2;
```

`DENSE_RANK` присваивает одинаковым зарплатам один ранг, поэтому «второе место»
корректно при дубликатах. Минус: сама по себе пустой результат не превратит в `NULL`.

## Подводные камни

- Забыть `DISTINCT` → при двух одинаковых максимумах «вторая» окажется равна первой.
- Забыть про `NULL` при отсутствии второй зарплаты → запрос вернёт 0 строк.
- Путать `DENSE_RANK` и `RANK`: `RANK` оставляет пропуски в нумерации при tie.

## Что проверяет на собесе

Top-N с учётом дубликатов и edge case «данных нет». Развитие — задача 177 (N-я
зарплата). Оконные функции подробно: [postgresql/13-pagination.md](../../../13-pagination.md).
