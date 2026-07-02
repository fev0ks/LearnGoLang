# 181. Employees Earning More Than Their Managers (Easy)

> LeetCode 181 · https://leetcode.com/problems/employees-earning-more-than-their-managers/
> Тема: self-join (соединение таблицы с самой собой).

## Условие

```
Employee: id (PK) | name | salary | managerId
```

`managerId` ссылается на `id` другого сотрудника (или `NULL` у топ-менеджеров).
Вернуть имена сотрудников, которые получают больше своего непосредственного
руководителя.

## Почему именно это спрашивают

Иерархия в одной таблице (adjacency list) — типовая модель. Чтобы сравнить
сотрудника с его менеджером, таблицу нужно соединить саму с собой и дать алиасы.

## Решение

```sql
SELECT e.name AS Employee
FROM Employee e
JOIN Employee m ON m.id = e.managerId
WHERE e.salary > m.salary;
```

`e` — строка сотрудника, `m` — строка его менеджера. `INNER JOIN` здесь корректен:
сотрудники без менеджера (`managerId IS NULL`) в ответ попадать не должны, и они же
естественно отсеиваются, так как `NULL` не соединится ни с одним `m.id`.

## Подводные камни

- Без алиасов сослаться на «ту же таблицу дважды» нельзя — обязательны `e` и `m`.
- Сравнивать нужно `e.salary > m.salary`, а не наоборот — легко перепутать сторону.

## Что проверяет на собесе

Понимание self-join и моделирования иерархий через `managerId`. Рекурсивный
вариант (вся цепочка начальников) — это уже `WITH RECURSIVE`, см.
[relational-databases-and-sql/01-relational-model-and-sql-basics.md](../../../../../relational-databases-and-sql/01-relational-model-and-sql-basics.md).
