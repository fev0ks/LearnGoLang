# Message Brokers And Streaming

Этот раздел нужен для понимания асинхронной обработки и интеграций.

Темы:
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
