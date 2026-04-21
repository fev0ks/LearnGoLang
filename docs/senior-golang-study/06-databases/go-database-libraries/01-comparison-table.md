# Comparison Table

Таблица помогает понять trade-offs, а не говорит «всегда бери X».

## SQL drivers и abstraction layers

| Библиотека | Что это | Плюсы | Минусы | Когда выбирать |
|---|---|---|---|---|
| `database/sql` | стандартный abstraction layer | stdlib, нет зависимостей, много драйверов | много boilerplate, ручной scan | multi-database, минимум зависимостей |
| `lib/pq` | legacy PostgreSQL driver | работает с `database/sql` | maintenance mode, нет batch/COPY | legacy проекты на `database/sql` |
| `pgx/v5` | PostgreSQL driver + toolkit | нативный PG API, batch, COPY, pgtype | только PostgreSQL | любой новый PG-проект |
| `pgxpool` | connection pool поверх pgx | нативный pool, Stat(), thread-safe | только PostgreSQL | production Go + PostgreSQL |

## SQL-first подходы

| Библиотека | Что это | Плюсы | Минусы | Когда выбирать |
|---|---|---|---|---|
| `sqlx` | runtime extensions над `database/sql` | меньше boilerplate, NamedExec, SelectContext | SQL в строках, runtime ошибки | migration path от database/sql |
| `sqlc` | code generator из SQL | type-safe Go methods, SQL читается, compile-time check | generation step, сложнее dynamic queries | SQL-first проект, большая команда |
| `squirrel` | query builder | dynamic WHERE без строк, type-safe builder | только builder — не ORM, не driver | dynamic filters, поиск, pagination |

## ORM и query builders

| Библиотека | Что это | Плюсы | Минусы | Когда выбирать |
|---|---|---|---|---|
| `GORM` | ORM | быстро стартовать, associations, hooks | магия, N+1 риск, сложнее debug | CRUD-heavy app, admin |
| `Ent` | schema-first entity framework | type-safe API, graph model | framework lock-in, codegen | сложный domain model |
| `Bun` | SQL-first ORM/query builder | ближе к SQL, меньше магии | ещё один abstraction layer | query builder + mapping без GORM magic |

## Как думать про выбор

**PostgreSQL-first сервис:**
- `pgxpool` + `sqlc` для статических queries + `squirrel` для dynamic

**Минимум зависимостей / multi-database:**
- `database/sql` + driver

**Raw SQL, но меньше boilerplate:**
- `sqlx`

**Dynamic filtering (поиск, фильтры):**
- `squirrel` — query builder без ORM

**CRUD-heavy / internal admin:**
- `GORM` или `Bun`

**Сложный domain model с relationships:**
- `Ent`

## Practical warning

Любая библиотека не отменяет:
- схему и миграции
- индексы и query plans
- транзакции и transaction boundaries
- connection pool tuning
- observability (slow query logging)
- integration tests

Самая частая ошибка — выбрать ORM «чтобы не думать про SQL», а потом не понимать, почему production query медленный.
