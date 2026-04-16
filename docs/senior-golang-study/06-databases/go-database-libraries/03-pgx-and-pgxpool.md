# pgx And pgxpool

`pgx` это PostgreSQL driver and toolkit для Go.

`pgxpool` это connection pool для `pgx`.

## Когда его часто выбирают

Если проект PostgreSQL-first, `pgx/pgxpool` часто выглядит естественнее, чем generic `database/sql`.

Причины:
- нативный PostgreSQL focus;
- удобная работа с pool;
- доступ к PostgreSQL-specific возможностям;
- хороший контроль над SQL.

## Базовый пример `pgxpool`

```go
pool, err := pgxpool.New(ctx, dsn)
if err != nil {
    return err
}
defer pool.Close()

row := pool.QueryRow(ctx, `
    SELECT id, email
    FROM users
    WHERE id = $1
`, userID)

var user User
if err := row.Scan(&user.ID, &user.Email); err != nil {
    return err
}
```

## Что дает `pgxpool`

- pool соединений;
- acquire/release lifecycle;
- context-aware queries;
- удобную работу с PostgreSQL;
- настройки pool через config.

## Плюсы

- сильный PostgreSQL focus;
- часто меньше friction с Postgres-specific features;
- можно писать plain SQL;
- хороший выбор для performance-sensitive сервисов.

## Минусы

- привязка к PostgreSQL ecosystem;
- все еще много explicit mapping;
- нужно понимать pool behavior;
- не дает ORM-level abstractions.

## `pgx` и `database/sql`

`pgx` можно использовать:
- через native API;
- или как driver для `database/sql`.

Практически:
- если вся система Postgres-first, часто выбирают native `pgxpool`;
- если нужна совместимость с библиотеками поверх `database/sql`, используют stdlib compatibility.

## Когда выбирать

Выбирай `pgxpool`, если:
- основная БД PostgreSQL;
- нужен контроль SQL;
- важны transactions, pool tuning и production clarity;
- не хочется ORM magic.

## Interview-ready answer

`pgxpool` это хороший default для Go + PostgreSQL, когда команда хочет писать SQL явно, но пользоваться нативным Postgres-oriented драйвером и нормальным connection pooling.
