# Streams Tasks

Задачи на обработку потоков данных: непрерывный input, который обрабатывается **по мере поступления**. В отличие от batch (`[]ids` → results) — здесь stream **не имеет конца** (kafka consumer, log tail, sensor data).

## Задачи

1. [Deduplication](./01-deduplication.md) — убрать дубли в потоке (по ключу, sliding window, exactly-once при at-least-once delivery)
2. [Batching Writer](./02-batching-writer.md) — собирать события и слать пачками (size + time triggers, flush on shutdown)
3. [Streaming Aggregation](./03-streaming-aggregation.md) — sliding window aggregations (sum/avg/percentiles per minute)
4. [Backpressure Handling](./04-backpressure.md) — что делать когда producer быстрее consumer'а

## Контекст применения

Эти задачи приходят из реальных production-сценариев:

| Задача | Когда встречается |
|---|---|
| Deduplication | Kafka at-least-once → exactly-once, webhook retry, idempotent consumer |
| Batching | DB bulk insert, S3 multipart, API rate limit aggregation |
| Aggregation | Real-time metrics, analytics dashboards, anomaly detection |
| Backpressure | Slow downstream, overloaded subscriber, bursty traffic |

## Общие принципы streams

### 1. Streams не имеют конца

В отличие от batch, нет момента "обработали все элементы". Нужно:
- Постоянно работать (без exit на конец input)
- Уметь graceful shutdown (drain или drop pending)
- Обрабатывать back-pressure

### 2. Memory bounded

Stream может производить миллионы событий в секунду. Все примитивы должны быть **O(1)** или **O(window size)** в памяти — не O(stream length).

### 3. Time matters

Многие операции зависят от **времени**:
- Dedup window (последние N секунд)
- Batch flush timeout
- Sliding aggregation
- Late arrivals (event time vs processing time)

### 4. At-least-once → idempotent processing

В distributed streaming (Kafka, Kinesis) гарантия — at-least-once. Дубли неизбежны. Consumer должен быть idempotent или дедуплицировать.

## Связки

- [Kafka](../../../07-message-brokers-and-streaming/01-kafka.md) — production stream broker
- [Redis Streams](../../../07-message-brokers-and-streaming/03-redis-streams.md)
- [Sliding window counter](../data-structures/05-sliding-window-counter.md) — родственно aggregation
- [Pipeline](../concurrency/04-pipeline.md) — stages-based streaming
- [Pub/Sub](../concurrency/05-pubsub.md) — base для streams
