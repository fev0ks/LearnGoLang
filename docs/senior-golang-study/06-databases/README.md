# Databases

Сюда складывай материалы по SQL и storage design.

Отдельные заметки:
- `migrations-in-go.md` - чем отличаются `goose`, `golang-migrate`, `Atlas`, `gormigrate`, `dbmate` и что выбирать на практике

Подпакеты:
- [Relational Databases And SQL](./relational-databases-and-sql/README.md)
- [Go Database Libraries](./go-database-libraries/README.md)
- [Database Systems Catalog](./database-systems-catalog/README.md)

Темы:
- PostgreSQL internals на практическом уровне;
- индексы: B-tree, partial, composite, covering;
- explain plans и оптимизация запросов;
- isolation levels, locking, deadlocks;
- транзакции и их границы в сервисах;
- pagination, keyset vs offset;
- connection pooling и saturation;
- Redis: caching, TTL, invalidation, hot keys;
- шардирование, репликация, read/write split;
- когда выбирать SQL, document DB, key-value, columnar storage.

Важные сравнения:
- Postgres vs MySQL для backend-сервисов;
- Redis cache-aside vs write-through;
- UUID vs sequence keys;
- soft delete vs hard delete vs event log.

## Подборка

- [PostgreSQL Documentation](https://www.postgresql.org/docs/current/index.htm)
- [PostgreSQL Indexes](https://www.postgresql.org/docs/current/indexes.html)
- [Using EXPLAIN](https://www.postgresql.org/docs/current/using-explain.html)
- [Concurrency Control](https://www.postgresql.org/docs/current/mvcc.html)
- [Redis Docs](https://redis.io/docs/latest/)
- [Redis Data Types](https://redis.io/docs/latest/develop/data-types/)
- [Redis Persistence](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)

## Вопросы

- как выбрать правильный индекс под конкретный query pattern;
- почему запрос может не использовать индекс, который "как будто подходит";
- когда транзакцию нужно расширить, а когда наоборот сузить;
- чем опасен долгий transaction scope в Go-сервисе;
- когда Redis оправдан как cache, а когда превращается в источник неконсистентности;
- чем keyset pagination лучше offset pagination;
- как бы ты расследовал deadlock или pool exhaustion.
