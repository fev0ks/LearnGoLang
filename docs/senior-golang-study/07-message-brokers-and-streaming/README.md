# Message Brokers And Streaming

Этот раздел нужен для понимания асинхронной обработки и интеграций.

## Материалы

- [01. Kafka](./01-kafka.md) — архитектура (topic/partition/offset/ISR/consumer group), delivery semantics, producer acks/batching, consumer poll loop, franz-go vs sarama, DLQ, log compaction
- [02. RabbitMQ](./02-rabbitmq.md) — exchange/queue/binding, типы exchange (fanout/direct/topic/headers), ack/nack/prefetch, DLQ, Go publisher/subscriber
- [03. Redis Streams](./03-redis-streams.md) — XADD/XREADGROUP/XACK, consumer groups, PEL, XCLAIM для failover, Go producer/consumer
- [04. Redis Pub/Sub](./04-redis-pubsub.md) — PUBLISH/SUBSCRIBE/PSUBSCRIBE, at-most-once, backplane паттерн, Go publisher/subscriber
- [07. Comparison](./07-comparison.md) — большая таблица, decision tree, типичные ошибки выбора

## Темы
- RabbitMQ, Kafka, NATS, Redis Streams, SQS/SNS;
- at-most-once, at-least-once, effectively-once;
- ordering, partitions, consumer groups;
- retries, DLQ, poison messages;
- backpressure и flow control;
- exactly-once claims и их реальные ограничения;
- outbox/inbox pattern;
- schema evolution и contract compatibility.

Что важно уметь объяснить:
- почему Kafka и RabbitMQ решают разные классы задач;
- когда нужен стриминг, а когда обычная очередь;
- как проектировать consumer, чтобы он был идемпотентным.

## Подборка

- [Apache Kafka Documentation](https://kafka.apache.org/documentation/)
- [RabbitMQ Documentation](https://www.rabbitmq.com/docs)
- [RabbitMQ Tutorials](https://www.rabbitmq.com/tutorials)
- [NATS Docs](https://docs.nats.io/)
- [JetStream](https://docs.nats.io/nats-concepts/jetstream)

## Вопросы

- чем event streaming отличается от очереди задач;
- почему exactly-once почти всегда требует аккуратной оговорки;
- как обрабатывать poison messages и где хранить DLQ;
- что делать, если consumer отстает от producer;
- когда ordering действительно нужен, а когда за него слишком дорого платить;
- как сделать consumer идемпотентным при повторной доставке;
- почему Kafka, RabbitMQ и NATS нельзя честно сравнить одной фразой "что лучше".
