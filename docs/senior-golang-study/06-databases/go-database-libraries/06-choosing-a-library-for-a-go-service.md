# Choosing A Library For A Go Service

Выбор библиотеки для БД лучше делать от требований, а не от вкусов.

## Вопросы перед выбором

1. Какая БД?
- PostgreSQL-only;
- несколько SQL databases;
- SQLite for local/tests.

2. Насколько сложные queries?
- simple CRUD;
- много joins;
- heavy reporting;
- dynamic filters.

3. Кто будет читать SQL?
- backend команда;
- DBA;
- reviewers;
- аналитики.

4. Что важнее?
- скорость разработки;
- контроль SQL;
- type safety;
- меньше boilerplate.

5. Как будете тестировать?
- unit tests with fakes;
- integration tests with real DB;
- testcontainers;
- migration-based schema setup.

## Практические default варианты

PostgreSQL service с strong SQL culture:
- `pgxpool`
- или `sqlc + pgx`

Минимум зависимостей:
- `database/sql` + driver

Raw SQL, но меньше boilerplate:
- `sqlx`

Много SQL и хочется type-safe API:
- `sqlc`

CRUD-heavy MVP or admin:
- `GORM`

Typed entity framework:
- `Ent`

SQL-first ORM/query builder:
- `Bun`

## Что я бы сказал на интервью

Хороший ответ:

```text
Если это PostgreSQL-heavy сервис, я бы сначала смотрел на pgxpool или sqlc+pgx.
Если команда хочет максимально явный SQL и меньше магии, ORM не нужен.
Если это CRUD-heavy internal app, GORM или Ent могут ускорить разработку.
Главное — понимать generated SQL, иметь миграции, индексы, observability и integration tests.
```

## Red flags

- "ORM нужен, чтобы не знать SQL"
- "database/sql медленный, потому что стандартная библиотека"
- "pgxpool решит проблемы с плохими запросами"
- "sqlc сам оптимизирует SQL"
- "если есть GORM, миграции и индексы не важны"

## Хороший production checklist

Независимо от библиотеки нужны:
- migrations;
- transaction boundaries;
- context timeouts;
- pool metrics;
- slow query visibility;
- integration tests;
- explain plan для критичных queries;
- понятный error mapping.
