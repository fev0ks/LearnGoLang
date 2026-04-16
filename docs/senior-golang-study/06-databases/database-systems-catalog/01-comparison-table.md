# Comparison Table

Эта таблица помогает быстро выбрать направление, но не заменяет design discussion.

| Database | Тип | Consistency / transactions | Сильна в | Слабее в | Типичный выбор |
| --- | --- | --- | --- | --- | --- |
| PostgreSQL | relational / object-relational SQL | ACID, сильные транзакции, constraints | транзакции, integrity, SQL, индексы, JSONB, расширяемость | экстремальный horizontal write scale без шардирования | основной backend storage |
| MySQL | relational SQL | ACID при нормальном engine/config, OLTP | простые OLTP workloads, read-heavy web apps, ecosystem | сложные PostgreSQL-style features, аналитика | классический web backend, legacy/ecosystem fit |
| MongoDB | document database | document-level atomicity, distributed transactions есть, но model не SQL-first | гибкая document model, JSON-like данные, быстрые product iterations | сложные joins, строгая relational consistency | catalog, profiles, content, document-centric apps |
| Cassandra | wide-column distributed DB | tunable consistency, BASE/eventual-consistency mindset | huge write scale, high availability, multi-node distribution | ad-hoc queries, joins, transactions | time-series-like writes, event storage, high scale |
| ClickHouse | columnar analytical DB | не OLTP/ACID storage для бизнес-транзакций | аналитика, агрегации, большие scans, logs/events analytics | transactional OLTP, frequent small updates | analytics, dashboards, event/log aggregates |
| Couchbase | distributed document + key-value | document/key-value consistency model, transactions есть, но не relational default | document access, cache-like access, mobile/sync scenarios | сложные relational queries, SQL-like strictness | low-latency document/key-value apps |
| Redis | in-memory key-value / data structures | atomic commands, но не полноценная relational ACID DB | cache, counters, locks, rate limits, queues-lite | durable source of truth для сложных данных | cache, ephemeral state, hot data |
| Elasticsearch/OpenSearch | search and analytics engine | search index / derived read model, eventual indexing mindset | full-text search, logs, filtering, relevance | primary transactional storage | search, observability, logs |
| DynamoDB | managed key-value/document NoSQL | single-item ACID, transactions есть, BASE/CAP trade-offs в distributed model | serverless scale, predictable key-value access, AWS integration | joins, ad-hoc queries, non-AWS portability | AWS-native high-scale key-value workloads |

## Частые аббревиатуры

### ACID

`ACID` описывает надежность транзакций.

Расшифровка:
- `Atomicity` - транзакция применяется целиком или не применяется вообще;
- `Consistency` - данные после транзакции остаются в валидном состоянии;
- `Isolation` - параллельные транзакции не должны ломать друг друга;
- `Durability` - после commit данные должны сохраниться.

Типичный пример:
- PostgreSQL;
- MySQL с transactional storage engine.

Когда это важно:
- платежи;
- заказы;
- балансы;
- любые critical business invariants.

### BASE

`BASE` часто противопоставляют `ACID` в distributed NoSQL системах.

Расшифровка:
- `Basically Available` - система старается оставаться доступной;
- `Soft state` - состояние может временно быть не полностью согласованным;
- `Eventually consistent` - данные со временем сходятся к согласованному состоянию.

Типичный mindset:
- Cassandra;
- DynamoDB-like distributed key-value/document systems;
- некоторые distributed document stores.

Когда это уместно:
- огромный scale;
- высокая доступность важнее мгновенной строгой согласованности;
- дубликаты и eventual consistency можно обработать на уровне приложения.

### CAP

`CAP` theorem говорит про distributed system trade-off при network partition.

Три свойства:
- `Consistency` - все клиенты видят согласованные данные;
- `Availability` - каждый запрос получает ответ;
- `Partition tolerance` - система продолжает работать при сетевом разделении.

Практический смысл:
- при partition нельзя идеально сохранить и строгую consistency, и availability одновременно;
- приходится выбирать поведение системы в деградации.

Важно:
- `CAP` не значит "можно выбрать любые 2 всегда";
- это упрощенная модель для разговора о distributed trade-offs.

### OLTP и OLAP

`OLTP`:
- Online Transaction Processing;
- много коротких transactional операций;
- типично: orders, payments, users.

Подходят:
- PostgreSQL;
- MySQL.

`OLAP`:
- Online Analytical Processing;
- большие scans, группировки, аналитика;
- типично: dashboards, events, reports.

Подходят:
- ClickHouse;
- columnar analytical stores.

## Как это связывать с выбором БД

Если вопрос про деньги, заказы и инварианты:
- сначала думай про `ACID` и relational DB.

Если вопрос про огромный distributed write scale:
- думай про `BASE`, partition key, eventual consistency и idempotency.

Если вопрос про аналитику:
- думай не `ACID vs BASE`, а `OLTP vs OLAP`.

Если вопрос про search:
- думай про search index как derived model, а не как primary transactional storage.

## Как выбирать

Если нужен default backend storage:
- сначала смотри PostgreSQL или MySQL.

Если данные естественно document-like:
- MongoDB или Couchbase.

Если нужен massive write scale и predictable partition-key access:
- Cassandra или DynamoDB.

Если нужна аналитика по большим объемам:
- ClickHouse.

Если нужен cache или ephemeral fast state:
- Redis.

Если нужен full-text search:
- Elasticsearch или OpenSearch.

## Главная ошибка

Плохой вопрос:

```text
Какая база быстрее?
```

Хороший вопрос:

```text
Какие у нас access patterns, consistency requirements, read/write ratio, latency target и operational constraints?
```
