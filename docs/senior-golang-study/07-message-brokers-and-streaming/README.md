# Message Brokers And Streaming

Этот раздел нужен для понимания асинхронной обработки и интеграций.

## Материалы

- [00. Comparison](./00-comparison.md) — точка входа: decision tree, большая таблица по всем брокерам, каталог реальных задач с выбором инструмента, разборы с числами, эволюция одного кейса под разной нагрузкой, что ломается первым при росте, типичные ошибки выбора
- [01. Kafka](./01-kafka.md) — архитектура (topic/partition/offset/ISR/consumer group), хранение лога на диске, KRaft, репликация и high watermark, delivery semantics с разбором exactly-once, producer acks/batching, consumer poll loop и static membership, партиционирование, DLQ и retry-топики, compaction и tiered storage, schema evolution
- [02. RabbitMQ](./02-rabbitmq.md) — exchange/queue/binding, типы exchange, немаршрутизируемые сообщения (mandatory, alternate exchange), confirms/ack/nack/prefetch, порядок и single active consumer, хранение и flow control, quorum queues, Streams, Khepri
- [03. NATS](./03-nats.md) — Core NATS (subjects/wildcards, request-reply, queue groups, at-most-once) и slow consumer, JetStream (streams/consumers, ack/redelivery, AckWait и MaxAckPending, dedup window, KV store), nats.go, когда NATS вместо Kafka или RabbitMQ
- [04. Redis Streams](./04-redis-streams.md) — XADD/XREADGROUP/XACK, идентификатор как позиция, consumer groups и PEL, обрезка потока и потеря выданных записей, память и забытые consumers, durability, отставание группы через XINFO, шардирование
- [05. Redis Pub/Sub](./05-redis-pubsub.md) — PUBLISH/SUBSCRIBE/PSUBSCRIBE, at-most-once, медленный подписчик и лимиты буфера, RESP2 против RESP3, sharded Pub/Sub в кластере, keyspace notifications, backplane-паттерн
- [06. Cloud Pub/Sub](./06-cloud-pubsub.md) — Google Cloud Pub/Sub (topics/subscriptions/ack deadline/DLT, ordering keys, exactly-once, seek), push против pull, AWS SNS+SQS (fan-out, standard против FIFO, visibility timeout), лимиты и стоимость
- [07. gRPC Streaming](./07-grpc-streaming.md) — bidirectional stream как транспорт, реестр стримов с горутиной-писателем на клиента, backplane на Redis, backpressure, переподключение и балансировка долгих соединений

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
