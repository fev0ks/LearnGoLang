# 182. Duplicate Emails (Easy)

> LeetCode 182 · https://leetcode.com/problems/duplicate-emails/
> Тема: `GROUP BY` + `HAVING`.

## Условие

```
Person: id (PK) | email
```

Вернуть все email, которые встречаются в таблице более одного раза.

## Почему именно это спрашивают

Базовая агрегация: разница между `WHERE` (фильтр строк *до* группировки) и
`HAVING` (фильтр групп *после* агрегации). Частая путаница у начинающих.

## Решение

```sql
SELECT email AS Email
FROM Person
GROUP BY email
HAVING COUNT(*) > 1;
```

`GROUP BY email` схлопывает строки с одинаковым email в одну группу, `COUNT(*)`
считает размер группы, `HAVING` оставляет только группы крупнее одной строки.

## Альтернатива через self-join

```sql
SELECT DISTINCT a.email AS Email
FROM Person a
JOIN Person b ON b.email = a.email AND b.id <> a.id;
```

На больших объёмах вариант с `GROUP BY` обычно эффективнее и читабельнее.

## Подводные камни

- `COUNT(*)` против фильтра по агрегату нельзя писать в `WHERE` — только в `HAVING`.
- Если в email бывают `NULL`, они в группировку попадут как одна группа `NULL`;
  по условию задачи email непустой, так что это не мешает.

## Что проверяет на собесе

Различие `WHERE`/`HAVING` и осознанный выбор группировки вместо самосоединения.
