# Database Migrations

Как менять схему БД в production без даунтайма и без потери данных: выбор инструмента,
кто и где запускает миграции, откаты (forward-only), zero-downtime паттерны и DDL-safety.

## Материалы

- [migrations-in-go.md](./migrations-in-go.md) — выбор инструмента (`goose`, `golang-migrate`,
  `Atlas`, `gormigrate`, `dbmate`) и production-обвязка: где запускать миграции (CI step /
  init container / Helm hook, а **не** autoMigrate на старте поды), forward-only откаты,
  expand/contract, schema drift detection, dirty-state recovery, advisory locks, checklist.

## Смежное

- **Тяжёлый DDL под нагрузкой** — добавить колонку со значением на таблице в десятки млн
  строк (константный vs волатильный `DEFAULT`, missing value PG 11+, быстрый `SET NOT NULL`
  через `NOT VALID`, lock queue): [PostgreSQL highload: онлайн-миграция колонки](../database-systems-catalog/postgresql/highload-scenarios/05-online-schema-migration.md).
- **15 кейсов zero-downtime изменений схемы** — rename/смена типа колонки, UNIQUE/FK/индекс
  без блокировок, INT→UUID, партиционирование существующей таблицы, переезд на новую БД:
  [PostgreSQL highload: zero-downtime паттерны](../database-systems-catalog/postgresql/highload-scenarios/06-zero-downtime-patterns.md).
- **DDL-локи в PostgreSQL** — какие ALTER берут `ACCESS EXCLUSIVE` и как не поймать outage:
  [04-transactions-and-locking.md](../database-systems-catalog/postgresql/04-transactions-and-locking.md).
- **Go-библиотеки работы с БД** (drivers, sqlx/sqlc, ORM) — соседний раздел:
  [go-database-libraries](../go-database-libraries/README.md).
