# 183. Customers Who Never Order (Easy)

> LeetCode 183 · https://leetcode.com/problems/customers-who-never-order/
> Тема: anti-join (поиск строк без совпадения), ловушка `NOT IN` + `NULL`.

## Условие

```
Customers: id (PK) | name
Orders:    id (PK) | customerId
```

Вернуть имена клиентов, которые ни разу ничего не заказали.

## Почему именно это спрашивают

«Найти то, чего нет» (anti-join) — частый паттерн. И именно здесь живёт классическая
ловушка с `NOT IN` и `NULL`.

## Решение через LEFT JOIN (рекомендуется)

```sql
SELECT c.name AS Customers
FROM Customers c
LEFT JOIN Orders o ON o.customerId = c.id
WHERE o.id IS NULL;
```

`LEFT JOIN` оставляет всех клиентов; у тех, кто не заказывал, колонки `Orders`
равны `NULL`. Фильтр `o.id IS NULL` оставляет ровно их.

## Альтернатива через NOT IN

```sql
SELECT name AS Customers
FROM Customers
WHERE id NOT IN (SELECT customerId FROM Orders);
```

Работает, **но опасно**: если в `Orders.customerId` встретится хотя бы один `NULL`,
`NOT IN` вернёт пустой результат для всех строк. Причина — трёхзначная логика:
`id <> NULL` даёт `UNKNOWN`, и `NOT IN` не может подтвердить «нет среди значений».
Безопаснее `NOT EXISTS` или `LEFT JOIN ... IS NULL`.

```sql
SELECT c.name AS Customers
FROM Customers c
WHERE NOT EXISTS (SELECT 1 FROM Orders o WHERE o.customerId = c.id);
```

## Подводные камни

- `NOT IN` + `NULL` в подзапросе → пустой результат. Главный смысл этой задачи.
- `WHERE o.id IS NULL` (не `o.customerId IS NULL`) — фильтруем по гарантированно
  непустой колонке правой таблицы (обычно PK).

## Что проверяет на собесе

Понимание anti-join, трёхзначной логики SQL и разницы `NOT IN` / `NOT EXISTS` /
`LEFT JOIN`. См. [database-fundamentals/04-interview-cases.md](../../../../../database-fundamentals/04-interview-cases.md).
