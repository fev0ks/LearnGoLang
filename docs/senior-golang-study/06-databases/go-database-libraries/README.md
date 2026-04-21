# Go Database Libraries

Популярные способы работать с базами данных в Go: drivers, abstraction layers, query builders, ORM и helper types.

## Материалы

- [01. Comparison Table](./01-comparison-table.md) — все библиотеки, trade-offs, helper types в одной таблице
- [02. Standard Library database/sql](./02-standard-library-database-sql.md) — pool config, transaction pattern, nullable types, rows.Err(), lib/pq
- [03. pgx And pgxpool](./03-pgx-and-pgxpool.md) — pgxpool.Config, ошибки pgconn.PgError, batch queries, CopyFrom, pgtype, pool metrics
- [04. sqlx And sqlc](./04-sqlx-and-sqlc.md) — NamedExec, sqlx.In(), sqlc full example с pgx/v5 backend, testability
- [05. ORM And Query Builder Options](./05-orm-and-query-builder-options.md) — squirrel (dynamic WHERE), GORM N+1, Ent, Bun
- [06. Choosing A Library For A Go Service](./06-choosing-a-library-for-a-go-service.md) — decision guide, рецепты, production checklist
- [Helper Types And Tools](../../../03-go-libraries-and-ecosystem/) — shopspring/decimal, google/uuid, samber/lo, pkg/errors → раздел 03

## Как читать

1. Начать с [comparison table](./01-comparison-table.md) — понять landscape
2. Разобрать `database/sql` + `pgxpool` — это основа для всего остального
3. Посмотреть `sqlx` / `sqlc` — SQL-first с разными trade-offs
4. ORM и squirrel — когда нужны и когда опасны
5. `07-helper-types-and-tools.md` — типы которые используются везде

## Что важно уметь объяснить

- `database/sql` — abstraction layer, не драйвер; нужны pool limits и `rows.Err()`
- `lib/pq` — legacy driver, сейчас предпочтительнее `pgx/stdlib`
- `pgxpool` — нативный PostgreSQL клиент: `pgconn.PgError`, batch, CopyFrom, `pool.Stat()`
- `sqlx` — runtime convenience (NamedExec, SelectContext), ошибки SQL остаются runtime
- `sqlc` — SQL→Go codegen: type-safe методы, `Querier` interface для моков
- `squirrel` — query builder для dynamic WHERE; дополняет, не заменяет sqlc/pgxpool
- GORM N+1 и как его избежать через `Preload`
- `shopspring/decimal` — для денег, никогда `float64` → [подробнее](../../../03-go-libraries-and-ecosystem/shopspring-decimal.md)
- UUID v7 лучше v4 для primary keys → [подробнее](../../../03-go-libraries-and-ecosystem/google-uuid.md)

## Официальные ссылки

- [Go: Accessing relational databases](https://go.dev/doc/database/)
- [database/sql package](https://pkg.go.dev/database/sql)
- [pgx](https://github.com/jackc/pgx)
- [sqlx docs](https://jmoiron.github.io/sqlx/)
- [sqlc docs](https://docs.sqlc.dev/)
- [squirrel](https://github.com/Masterminds/squirrel)
- [GORM docs](https://gorm.io/docs/)
- [Ent docs](https://entgo.io/)
- [Bun docs](https://bun.uptrace.dev/guide/)
- [shopspring/decimal](https://github.com/shopspring/decimal)
- [google/uuid](https://github.com/google/uuid)
- [samber/lo](https://github.com/samber/lo)
- [pkg/errors](https://github.com/pkg/errors)
- [go-playground/validator](https://github.com/go-playground/validator)
