# Database Systems Catalog

Этот подпакет нужен, чтобы быстро сравнивать популярные базы данных и понимать, когда какую систему выбирать.

Формат каждого файла:
- что это за БД;
- для чего обычно используется;
- сильные стороны;
- слабые стороны;
- когда выбирать;
- когда лучше не выбирать;
- что могут спросить на интервью.

Материалы:
- [01 Comparison Table](./01-comparison-table.md)
- [02 PostgreSQL](./02-postgresql.md)
- [03 MySQL](./03-mysql.md)
- [04 MongoDB](./04-mongodb.md)
- [04a MongoDB: реальные сценарии](./04a-mongodb-real-scenarios.md)
- [05 Cassandra](./05-cassandra.md)
- [06 ClickHouse](./06-clickhouse.md)
- [07 Couchbase](./07-couchbase.md)
- [08 Redis](./08-redis.md)
- [08a Redis: реальные сценарии](./08a-redis-real-scenarios.md)
- [08b Redis: rate limiters](./08b-redis-rate-limiters.md)
- [09 Elasticsearch And OpenSearch](./09-elasticsearch-and-opensearch.md)
- [10 DynamoDB](./10-dynamodb.md)

Официальные ссылки:
- [PostgreSQL Docs](https://www.postgresql.org/docs/)
- [MySQL Docs](https://dev.mysql.com/doc/mysql/en/)
- [MongoDB Docs](https://www.mongodb.com/docs/)
- [Apache Cassandra Docs](https://cassandra.apache.org/doc/stable/)
- [ClickHouse Docs](https://clickhouse.com/docs/en)
- [Couchbase Docs](https://docs.couchbase.com/home/index.html)
- [Redis Docs](https://redis.io/docs/latest/)
- [Elastic Docs](https://www.elastic.co/docs)
- [OpenSearch Docs](https://docs.opensearch.org/)
- [DynamoDB Docs](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Introduction)

Важная мысль:
- база данных выбирается не по популярности;
- она выбирается под access patterns, consistency requirements, latency, write/read profile, operational maturity и стоимость сопровождения.
