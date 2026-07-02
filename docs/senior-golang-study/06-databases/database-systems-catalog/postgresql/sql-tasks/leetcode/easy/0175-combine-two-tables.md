# 175. Combine Two Tables (Easy)

> LeetCode 175 · https://leetcode.com/problems/combine-two-tables/
> Тема: `LEFT JOIN`, сохранение строк без совпадения.

## Условие

```
Person:  personId (PK) | firstName | lastName
Address: addressId (PK) | personId  | city | state
```

Вернуть `firstName, lastName, city, state` по каждому человеку. Если адреса нет —
в `city`/`state` должен быть `NULL`. То есть человек попадает в результат всегда,
даже без записи в `Address`.

## Почему именно это спрашивают

Проверяют понимание разницы между `INNER JOIN` и `LEFT JOIN`. Частая ошибка
junior — поставить `INNER JOIN` и молча потерять людей без адреса.

## Решение

```sql
SELECT p.firstName,
       p.lastName,
       a.city,
       a.state
FROM Person p
LEFT JOIN Address a ON a.personId = p.personId;
```

`LEFT JOIN` гарантирует, что каждая строка левой таблицы (`Person`) попадёт в
результат. Колонки правой таблицы при отсутствии совпадения заполняются `NULL`.

## Подводные камни

- `INNER JOIN` отбросит людей без адреса — частая ошибка по невнимательности.
- Условие соединения должно быть в `ON`, а не в `WHERE`. Перенос условия по правой
  таблице в `WHERE` (например `WHERE a.state = 'CA'`) превращает `LEFT JOIN` обратно
  в `INNER JOIN`, потому что `NULL = 'CA'` ложно и строки без адреса отфильтруются.

## Что проверяет на собесе

Базовое понимание типов JOIN и того, как фильтры в `WHERE` ломают внешние
соединения. См. также [relational-databases-and-sql/01-relational-model-and-sql-basics.md](../../../../../relational-databases-and-sql/01-relational-model-and-sql-basics.md).
