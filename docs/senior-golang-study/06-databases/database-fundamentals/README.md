# Database Fundamentals

Этот подпакет про базовые модели и trade-offs в базах данных и распределенных storage-системах: `ACID`, `CAP`, `BASE`, `OLTP` и `OLAP`.

Цель:
- не выучить аббревиатуры как определения;
- понимать, какие гарантии реально получает backend-сервис;
- уметь объяснять компромиссы на примерах: платежи, корзина, лента, аналитика, кэш, репликация;
- связывать выбор БД с access patterns, consistency requirements, latency и operational complexity.

Материалы:
- [01 ACID](./01-acid.md)
- [02 CAP And BASE](./02-cap-and-base.md)
- [03 OLTP vs OLAP](./03-oltp-vs-olap.md)
- [04 Interview Cases](./04-interview-cases.md)

Как читать:
- сначала разобраться с `ACID`, потому что это основа транзакций и инвариантов;
- затем перейти к `CAP` и `BASE`, чтобы понять, что меняется в распределенных системах;
- после этого сравнить `OLTP` и `OLAP`, потому что разные нагрузки требуют разных storage-моделей;
- в конце разобрать кейсы и потренироваться формулировать короткие практические ответы.

Связанные материалы:
- [Transactions Isolation And Locks](../relational-databases-and-sql/02-transactions-isolation-and-locks.md)
- [Indexes And Query Plans](../relational-databases-and-sql/03-indexes-and-query-plans.md)
- [Database Systems Catalog](../database-systems-catalog/README.md)

Официальные ссылки:
- [PostgreSQL Transactions](https://www.postgresql.org/docs/current/tutorial-transactions.html)
- [PostgreSQL MVCC](https://www.postgresql.org/docs/current/mvcc.html)
- [MongoDB Read Concern](https://www.mongodb.com/docs/manual/reference/read-concern/)
- [Cassandra Architecture](https://cassandra.apache.org/doc/stable/cassandra/architecture/overview.html)
- [ClickHouse Docs](https://clickhouse.com/docs/)

Что важно уметь объяснить:
- почему `ACID` не означает "все параллельные операции всегда идеально сериализованы";
- почему `Consistency` в `ACID` и `Consistency` в `CAP` - разные идеи;
- почему `CAP` проявляется именно при network partition, а не при обычной высокой latency;
- когда eventual consistency допустима, а когда ломает бизнес-инвариант;
- почему транзакционная БД не заменяет аналитическое хранилище под тяжелые отчеты;
- как разделять write path, read model и analytics pipeline.
