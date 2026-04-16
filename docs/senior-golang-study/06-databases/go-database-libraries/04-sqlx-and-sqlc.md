# sqlx And sqlc

`sqlx` и `sqlc` часто сравнивают, потому что оба сохраняют SQL-first подход, но решают разные проблемы.

## `sqlx`

`sqlx` это extensions над `database/sql`.

Он помогает:
- сканировать строки в structs;
- делать named queries;
- уменьшить ручной boilerplate.

Пример:

```go
var users []User
err := db.SelectContext(ctx, &users, `
    SELECT id, email
    FROM users
    WHERE active = $1
`, true)
```

Плюсы:
- SQL остается явным;
- меньше boilerplate, чем `database/sql`;
- проще migration path от стандартной библиотеки.

Минусы:
- SQL все еще строки;
- нет compile-time проверки SQL;
- mapping ошибки часто runtime-only.

## `sqlc`

`sqlc` генерирует Go-код из SQL queries.

Ты пишешь SQL:

```sql
-- name: GetUser :one
SELECT id, email
FROM users
WHERE id = $1;
```

`sqlc` генерирует Go method:

```go
user, err := queries.GetUser(ctx, id)
```

Плюсы:
- SQL остается SQL;
- Go-код type-safe;
- меньше ручного mapping;
- queries можно review-ить как обычный SQL.

Минусы:
- нужен generation step;
- структура проекта чуть сложнее;
- dynamic queries менее удобны;
- надо поддерживать sync между schema, queries и generated code.

## Главное отличие

`sqlx`:
- runtime helper над `database/sql`;
- меньше boilerplate;
- SQL не становится compile-time safe.

`sqlc`:
- code generator;
- превращает SQL в typed Go API;
- требует build/generation discipline.

## Когда что выбирать

`sqlx`:
- если нужен минимальный переход от `database/sql`;
- если queries простые;
- если хочется меньше ручного `Scan`.

`sqlc`:
- если SQL важен как контракт;
- если команда хочет type-safe repository methods;
- если проект растет и runtime SQL errors становятся дорогими.

## Interview-ready answer

`sqlx` делает ручной SQL удобнее, а `sqlc` делает ручной SQL типобезопаснее через генерацию кода. Оба подхода оставляют SQL в руках разработчика, в отличие от ORM.
