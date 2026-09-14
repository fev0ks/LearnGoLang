# Database Fundamentals

Этот подпакет про базовые модели и trade-offs в базах данных и распределённых
хранилищах: `ACID`, `CAP`, `BASE`, `OLTP`, `OLAP`, репликацию и consensus.

Цель:

- не выучить аббревиатуры как определения;
- понимать, какие гарантии реально получает backend-сервис;
- уметь объяснять компромиссы на примерах: платежи, корзина, лента, аналитика, кэш, репликация;
- связывать выбор БД с access patterns, consistency requirements, latency и operational complexity.

Материалы:

- [01 ACID: транзакции и инварианты](./01-acid.md)
- [02 CAP, BASE и распределённая консистентность](./02-cap-and-base.md)
- [03 OLTP vs OLAP](./03-oltp-vs-olap.md)
- [04 Interview-кейсы](./04-interview-cases.md)
- [05 Модели репликации: single-leader, multi-leader и leaderless](./05-replication-models.md)
- [06 Consensus: Raft и границы сравнения с Paxos](./06-consensus-raft-and-paxos.md)

Как читать:

- сначала разобраться с `ACID`, потому что это основа транзакций и инвариантов;
- затем перейти к `CAP` и `BASE`, чтобы понять, что меняется в распределенных системах;
- после этого сравнить `OLTP` и `OLAP`, потому что разные нагрузки требуют разных storage-моделей;
- разобрать кейсы и потренироваться формулировать короткие практические ответы;
- затем сравнить модели репликации и их аномалии чтения;
- в конце изучить Raft и Paxos, чтобы отделять consensus от обычного копирования данных, lock и распределённой транзакции.

Связанные материалы:

- [Transactions Isolation And Locks](../database-systems-catalog/postgresql/04-transactions-and-locking.md)
- [Indexes And Query Plans](../database-systems-catalog/postgresql/02-indexes.md)
- [Database Systems Catalog](../database-systems-catalog/README.md)
- [Highload Design Patterns](../../05-system-design/highload-design-patterns.md)

Официальные ссылки:

- [PostgreSQL Transactions](https://www.postgresql.org/docs/current/tutorial-transactions.html)
- [PostgreSQL MVCC](https://www.postgresql.org/docs/current/mvcc.html)
- [MongoDB Read Concern](https://www.mongodb.com/docs/manual/reference/read-concern/)
- [Cassandra Architecture](https://cassandra.apache.org/doc/stable/cassandra/architecture/overview.html)
- [ClickHouse Docs](https://clickhouse.com/docs/)
- [Raft paper](https://raft.github.io/raft.pdf)
- [Paxos Made Simple](https://lamport.azurewebsites.net/pubs/paxos-simple.pdf)

Что важно уметь объяснить:

- почему `ACID` не означает "все параллельные операции всегда идеально сериализованы";
- почему `Consistency` в `ACID` и `Consistency` в `CAP` - разные идеи;
- почему `CAP` проявляется именно при network partition, а не при обычной высокой latency;
- когда eventual consistency допустима, а когда ломает бизнес-инвариант;
- почему транзакционная БД не заменяет аналитическое хранилище под тяжелые отчеты;
- как разделять write path, read model и analytics pipeline;
- чем single-leader, multi-leader и leaderless replication платят за доступность и локальную latency;
- почему `W + R > N` гарантирует пересечение обычных кворумов, но само по себе не даёт linearizability;
- как Raft выбирает leader, реплицирует журнал и фиксирует запись через majority;
- чем consensus отличается от replication, distributed lock и Two-Phase Commit.
