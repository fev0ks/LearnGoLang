# Go Database Libraries

Этот подпакет про популярные способы работать с базами данных в Go.

Фокус:
- чем отличается стандартный `database/sql` от драйверов и ORM;
- когда выбирать `pgxpool`, `sqlx`, `sqlc`, `GORM`, `Ent`, `Bun`;
- какие trade-offs важны на backend-собеседовании и в production.

Материалы:
- [01 Comparison Table](./01-comparison-table.md)
- [02 Standard Library database/sql](./02-standard-library-database-sql.md)
- [03 pgx And pgxpool](./03-pgx-and-pgxpool.md)
- [04 sqlx And sqlc](./04-sqlx-and-sqlc.md)
- [05 ORM And Query Builder Options](./05-orm-and-query-builder-options.md)
- [06 Choosing A Library For A Go Service](./06-choosing-a-library-for-a-go-service.md)

Официальные ссылки:
- [Go: Accessing relational databases](https://go.dev/doc/database/)
- [database/sql package](https://pkg.go.dev/database/sql)
- [pgx](https://github.com/jackc/pgx)
- [GORM docs](https://gorm.io/docs/)
- [sqlc docs](https://docs.sqlc.dev/)
- [sqlx docs](https://jmoiron.github.io/sqlx/)
- [Ent docs](https://entgo.io/)
- [Bun docs](https://bun.uptrace.dev/guide/)

Как читать:
- сначала открыть comparison table;
- потом пройти `database/sql` и `pgxpool`;
- затем посмотреть `sqlx/sqlc`;
- в конце сравнить ORM-подходы и decision guide.

Что важно уметь объяснить:
- что `database/sql` это общий abstraction layer, а не драйвер конкретной БД;
- почему `pgxpool` часто выбирают для PostgreSQL;
- чем `sqlc` отличается от ORM;
- почему GORM ускоряет разработку, но может усложнить контроль SQL;
- почему выбор библиотеки влияет на тестирование, миграции, observability и performance debugging.
