# Relational Databases And SQL

Этот подпакет про практическую базу SQL и PostgreSQL-style thinking для backend-разработчика.

Фокус:
- не зубрить SQL-синтаксис;
- понимать, как БД ведет себя под нагрузкой;
- уметь объяснить транзакции, индексы, locks, connection pool и типовые production проблемы;
- уверенно разобрать interview-сценарий вроде `PayOrder`.

Материалы:
- [01 Relational Model And SQL Basics](./01-relational-model-and-sql-basics.md)
- [02 Transactions Isolation And Locks](./02-transactions-isolation-and-locks.md)
- [03 Indexes And Query Plans](./03-indexes-and-query-plans.md)
- [04 Pagination And Query Patterns](./04-pagination-and-query-patterns.md)
- [05 Connection Pooling And Production Issues](./05-connection-pooling-and-production-issues.md)
- [06 Outbox Idempotency And Payment Flow](./06-outbox-idempotency-and-payment-flow.md)

Как читать:
- сначала пройти relational model и базовые SQL операции;
- потом транзакции, isolation и locks;
- после этого индексы и query plans;
- затем pagination, connection pooling и production debugging;
- в конце разобрать `PayOrder` flow как interview case.

Что важно уметь объяснить:
- зачем нужна транзакция и где ее границы;
- почему индекс не всегда ускоряет запрос;
- чем `SELECT ... FOR UPDATE` помогает от double-write;
- почему `offset pagination` деградирует;
- что такое outbox pattern и зачем он нужен.
