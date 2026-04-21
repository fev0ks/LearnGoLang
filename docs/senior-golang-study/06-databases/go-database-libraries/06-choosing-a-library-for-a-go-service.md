# Choosing A Library For A Go Service

Выбор библиотеки для БД лучше делать от требований, а не от вкусов.

## Вопросы перед выбором

**1. Какая БД?**
- PostgreSQL-only → `pgxpool`, нативный API, pgtype, batch
- несколько SQL databases → `database/sql` + подходящие драйверы
- SQLite для тестов → `go-sqlite3` или `mattn/go-sqlite3`

**2. Насколько сложные queries?**
- simple CRUD → любой вариант
- много joins и сложных выборок → SQL-first (sqlc, sqlx, pgxpool raw)
- heavy reporting → raw SQL обязательно, ORM не поможет
- dynamic filters (поиск, фильтры по многим полям) → squirrel

**3. Как хранить и писать SQL?**
- SQL как контракт, хочется compile-time check → sqlc
- SQL в коде, меньше boilerplate → sqlx
- SQL в runtime строках, полный контроль → database/sql или pgxpool
- SQL конкатенировать нельзя, но нужен dynamic → squirrel builder

**4. Что важнее?**
- скорость разработки → GORM, Ent
- контроль SQL → pgxpool, sqlc
- type safety → sqlc, Ent
- меньше boilerplate → sqlx, squirrel

**5. Как будете тестировать?**
- integration tests с реальной БД → testcontainers + любой вариант
- unit tests с моками → sqlc (генерирует Querier interface), database/sql (sql.DB можно замокировать)

## Практические рецепты

**PostgreSQL service, SQL culture, средний проект:**
```
pgxpool + sqlc (статические queries) + squirrel (dynamic filters)
```

**PostgreSQL service, нужна простота:**
```
pgxpool (всё вручную, полный контроль)
```

**Минимум зависимостей, multi-database:**
```
database/sql + pgx/stdlib или lib/pq
```

**Raw SQL, но меньше scan boilerplate:**
```
sqlx
```

**Много SQL, нужна type-safety:**
```
sqlc + pgx/v5 backend
```

**CRUD-heavy MVP / admin panel:**
```
GORM
```

**Сложная domain модель:**
```
Ent
```

**Dynamic queries поверх любого SQL-first:**
```
squirrel (дополняет pgxpool/sqlx/sqlc, а не заменяет)
```

## Типы данных в Go + PostgreSQL

Независимо от выбранной ORM/driver:

| Тип данных | PostgreSQL | Go-библиотека |
|---|---|---|
| Деньги / финансы | `NUMERIC(10,2)` | `shopspring/decimal` |
| Уникальные ID | `UUID` | `google/uuid` (v7 для PK) |
| Bulk-трансформации | — | `samber/lo` |
| Ошибки со стектрейсом | — | `pkg/errors` или `fmt.Errorf("%w")` |

## Что я бы сказал на интервью

```text
Если это PostgreSQL-heavy сервис, я бы выбрал pgxpool для прямых
запросов и sqlc для статических queries. Для dynamic фильтров —
squirrel, чтобы не конкатенировать строки вручную.

Если команда хочет максимально явный SQL и минимум магии —
ORM не нужен. Если это CRUD-heavy internal app — GORM или Ent
ускорят разработку.

Для денег всегда shopspring/decimal, никогда float64.
UUID v7 для primary keys — sequential insert, меньше фрагментации.

Главное — понимать generated SQL, иметь миграции, индексы,
observability, integration tests и pool metrics.
```

## Red flags

- "ORM нужен, чтобы не знать SQL"
- "database/sql медленный, потому что стандартная библиотека"
- "pgxpool решит проблемы с плохими запросами"
- "sqlc сам оптимизирует SQL"
- "если есть GORM, миграции и индексы не важны"
- "буду хранить деньги в float64, там же есть округление"
- "UUID v4 и v7 — одно и то же"

## Production checklist

Независимо от библиотеки нужны:

- [ ] migrations (goose, atlas, migrate)
- [ ] transaction boundaries на бизнес-операции
- [ ] context timeout на каждый запрос
- [ ] pool limits (`MaxConns`, `ConnMaxLifetime`)
- [ ] pool metrics в Prometheus
- [ ] slow query logging
- [ ] integration tests с реальной БД (testcontainers)
- [ ] EXPLAIN ANALYZE для критичных queries
- [ ] понятный error mapping (pgconn.PgError → domain errors)
- [ ] graceful shutdown (закрыть pool при SIGTERM)
