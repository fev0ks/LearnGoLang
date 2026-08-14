# Задачи на обработку потоков

Раздел посвящён stateful stream processing: вход обрабатывается по мере
поступления, а компонент должен явно ограничивать память, корректно завершаться
и переживать повторы, out-of-order delivery и медленный downstream.

Не каждый stream бесконечен: network connection или bounded replay может
завершиться. Но алгоритм не должен рассчитывать, что «скоро придёт последний
элемент» и тогда можно будет обработать всю накопленную историю.

## Материалы

1. [Deduplication](./01-deduplication.md) — точный и approximate dedup,
   retention window, Redis и transactional inbox.
2. [Batching Writer](./02-batching-writer.md) — size/time triggers, bounded
   buffer, retry и корректный drain при shutdown.
3. [Streaming Aggregation](./03-streaming-aggregation.md) — bucketed sliding
   windows, простые агрегаты и HDR Histogram.
4. [Backpressure Handling](./04-backpressure.md) — block/drop/sampling,
   атомарный `drop oldest`, capacity planning и fairness.
5. [Event Time и Watermarks](./05-event-time-and-watermarks.md) — out-of-order
   events, tumbling windows, partition watermarks и late-event policy.

Рекомендуемый порядок — по нумерации. Пятая задача развивает временную семантику
из aggregation; backpressure полезно знать до обсуждения нескольких partitions.

---

## Контекст применения

| Задача | Где встречается | Главный trade-off |
|---|---|---|
| Deduplication | broker redelivery, webhook retry | точность, retention и durability |
| Batching | bulk DB/API writes, telemetry | throughput против latency |
| Aggregation | metrics, rolling statistics | память/точность против granularity |
| Backpressure | slow downstream, bursts | loss/latency против bounded resources |
| Event time | mobile/IoT, replay, distributed logs | completeness против result latency |

---

## Общие принципы

### Bounded state

Память должна зависеть от явно ограниченного окна, capacity или cardinality, а
не от длины всей истории. `O(window)` само по себе недостаточно: нужно уточнять,
что является единицей окна — секунды, buckets или число уникальных IDs.

### Lifecycle

Фоновая goroutine должна иметь остановку, а `Close` — определённый контракт:
drain или drop, deadline, идемпотентность и результат ошибки. Проверка atomic
флага перед send не заменяет согласованный протокол concurrent `Add`/`Close`.

### Delivery semantics

At-least-once broker допускает redelivery, но не делает любой consumer
идемпотентным автоматически. Dedup state, business effect и acknowledgement
нужно связать транзакцией, checkpoint или idempotency protocol. In-memory buffer
не переживает crash только потому, что он дренируется при graceful shutdown.

### Time semantics

Processing time, ingestion time и event time отвечают на разные вопросы. TTL по
`time.Now`, event-time watermark и batch flush interval нельзя смешивать в одно
«окно N минут» без уточнения clock и границ.

### Observability

Для stream-компонентов обычно нужны:

- input/service rate и processing latency;
- queue depth, capacity и queueing time;
- dropped/rejected/late/duplicate по причинам;
- размер state и cleanup duration;
- retry attempts, terminal failures и shutdown drain duration.

---

## Связанные разделы

- [Kafka](../../../07-message-brokers-and-streaming/01-kafka.md)
- [Redis Streams](../../../07-message-brokers-and-streaming/04-redis-streams.md)
- [Sliding Window Counter](../data-structures/05-sliding-window-counter.md)
- [Pipeline](../concurrency/04-pipeline.md)
- [Pub/Sub](../concurrency/05-pubsub.md)
