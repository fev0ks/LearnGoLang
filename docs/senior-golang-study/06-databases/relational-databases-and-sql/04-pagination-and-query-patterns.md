# Pagination And Query Patterns

Pagination кажется простой, пока таблица не становится большой.

## Offset pagination

Пример:

```sql
SELECT *
FROM orders
ORDER BY created_at DESC
LIMIT 50 OFFSET 10000;
```

Плюсы:
- просто реализовать;
- удобно для "страница 1, 2, 3".

Минусы:
- большой `OFFSET` становится дорогим;
- при изменении данных можно получить дубли или пропуски;
- БД все равно должна пройти много строк, чтобы их пропустить.

## Keyset pagination

Идея:
- вместо "пропусти 10000 строк" говорим "дай следующие строки после cursor".

Пример:

```sql
SELECT *
FROM orders
WHERE created_at < $1
ORDER BY created_at DESC
LIMIT 50;
```

Лучше с tie-breaker:

```sql
SELECT *
FROM orders
WHERE (created_at, id) < ($1, $2)
ORDER BY created_at DESC, id DESC
LIMIT 50;
```

Плюсы:
- стабильнее на больших таблицах;
- хорошо использует индекс;
- меньше деградации с глубиной страницы.

Минусы:
- нельзя легко прыгнуть на "страницу 100";
- нужен cursor;
- сложнее UI и API contract.

## Индекс под keyset pagination

Под такой запрос нужен индекс:

```sql
CREATE INDEX idx_orders_created_id
ON orders(created_at DESC, id DESC);
```

## N+1 query problem

Плохой pattern:
- загрузили 100 orders;
- потом для каждого order отдельно загрузили user.

Итого:
- 1 query + 100 queries.

Что помогает:
- join;
- batch query через `WHERE id = ANY($1)`;
- preload на уровне repository;
- careful GraphQL dataloader-like approach.

## Filtering and sorting

Опасная комбинация:
- много optional filters;
- dynamic sort;
- пользователь может сортировать по чему угодно.

Что важно:
- понимать реальные query patterns;
- не создавать индекс на каждую фантазию UI;
- ограничивать доступные sort/filter options;
- проверять через `EXPLAIN`.

## Search-like queries

Если нужен настоящий search:
- `LIKE '%text%'` по большой таблице часто плохо масштабируется;
- иногда нужен full-text search;
- иногда Elasticsearch/OpenSearch;
- иногда trigram index.

## Interview-ready summary

Offset pagination простая, но плохо масштабируется на глубоких страницах. Keyset pagination сложнее в API, но лучше для больших таблиц и real-time данных. Хороший ответ всегда связывает pagination с индексом и стабильным `ORDER BY`.
