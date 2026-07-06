# Message Brokers: сравнение

Сводное сравнение для выбора инструмента — финальный раздел после изучения каждого брокера: [Kafka](./01-kafka.md), [RabbitMQ](./02-rabbitmq.md), [Redis Streams](./03-redis-streams.md), [Redis Pub/Sub](./04-redis-pubsub.md), [Cloud Pub/Sub](./05-cloud-pubsub.md), [gRPC Streaming](./06-grpc-streaming.md), [NATS](./08-nats.md).

## Содержание

- [Большая таблица](#большая-таблица)
- [Decision tree](#decision-tree)
- [Типичные ошибки выбора](#типичные-ошибки-выбора)
- [Когда несколько брокеров одновременно](#когда-несколько-брокеров-одновременно)
- [Быстрые характеристики](#быстрые-характеристики)
- [Interview-ready answer](#interview-ready-answer)

## Большая таблица

| | Kafka | RabbitMQ | NATS Core | NATS JetStream | Redis Streams | Redis Pub/Sub | Cloud (GCP Pub/Sub, SNS+SQS) |
|---|---|---|---|---|---|---|---|
| **Модель** | event log | message queue + routing | pub/sub + request-reply | стрим поверх subjects | append-only log | fire-and-forget | managed pub/sub + queue |
| **Persistence** | ✅ retention днями+ | ✅ durable/quorum | ❌ | ✅ file + Raft | ✅ до MAXLEN | ❌ | ✅ managed (обычно до 7 дней) |
| **Delivery** | at-least-once | at-least-once | at-most-once | at-least-once | at-least-once | at-most-once | at-least-once |
| **Exactly-once** | ✅ транзакции (дорого) | ❌ | ❌ | ⚠️ dedup window | ❌ | ❌ | ⚠️ опция GCP |
| **Распределение работы** | consumer groups (партиции) | competing consumers | queue groups | durable consumer / work-queue | XREADGROUP | ❌ broadcast | subscriptions / SQS consumers |
| **Replay** | ✅ любой offset | ❌ | ❌ | ✅ любая позиция | ✅ любой ID | ❌ | ⚠️ GCP seek; SQS ❌ |
| **Ordering** | per-partition | per-queue | per-publisher/subject | per-subject | per-stream | нет гарантий | ordering keys / FIFO |
| **Request-reply** | ❌ | руками (reply-to) | ✅ встроен | ✅ | ❌ | ❌ | ❌ |
| **Throughput** | 1M+ msg/s | 50–100k msg/s | миллионы msg/s | сотни тысяч | ~100k msg/s | 100k+ msg/s | managed, квоты |
| **Latency** | 5–20 мс (batching) | < 1 мс | < 1 мс | ~мс | < 1 мс | < 1 мс | десятки мс |
| **Routing** | ключ → партиция | exchange types | subject wildcards | subject wildcards | по stream key | channel/pattern | topics/filters |
| **Ops complexity** | высокая | умеренная | минимальная | низкая | низкая (есть Redis) | низкая (есть Redis) | нулевая (managed) |
| **DLQ** | ✅ отдельный топик | ✅ x-dead-letter | ❌ | ⚠️ advisories/вручную | вручную | ❌ | ✅ нативно |

gRPC Streaming в таблице нет намеренно: это транспорт точка-точка, а не брокер — сравнение в [06-grpc-streaming.md](./06-grpc-streaming.md).

## Decision tree

Идти сверху вниз, первый подходящий вопрос даёт ответ:

| # | Вопрос | Если да | Почему |
| --- | --- | --- | --- |
| 1 | Двусторонняя real-time связь клиент ↔ сервер? | **gRPC Streaming / WebSocket** | это транспорт, брокер не нужен |
| 2 | Потеря сообщений допустима, Redis уже в стеке (cache invalidation, live-обновления)? | **Redis Pub/Sub** | простейший broadcast |
| 3 | Потеря допустима, нужен ещё и request-reply между сервисами? | **NATS Core** | pub/sub + RPC, суб-мс latency |
| 4 | Нужны replay, долгое хранение, throughput 100k+ msg/s, несколько независимых групп читателей? | **Kafka** | распределённый лог |
| 5 | Надёжная очередь задач со сложной маршрутизацией (типы, приоритеты, TTL)? | **RabbitMQ** | exchange-модель |
| 6 | Надёжная очередь при минимальной эксплуатации (один бинарь, k8s/edge)? | **NATS JetStream** | стрим + очередь в одном сервере |
| 7 | Надёжная очередь, Redis уже в стеке, ставить отдельный брокер не хочется? | **Redis Streams** | consumer groups поверх Redis |
| 8 | Эксплуатировать самим не хочется вообще? | **GCP Pub/Sub / SNS+SQS** | managed, платить за объём |

## Типичные ошибки выбора

### Redis Pub/Sub для надёжной доставки

```text
❌ "Используем Redis Pub/Sub для отправки писем"
   → email-worker перезапустился — письма потеряны

✅ Redis Streams или RabbitMQ для очереди писем;
   Redis Pub/Sub — только "оповестить живые инстансы прямо сейчас"
```

То же относится к NATS Core: fire-and-forget не для задач, которые нельзя терять.

### Kafka для простой task queue

```text
❌ "Ставим Kafka для фоновых задач"
   → кластер, KRaft-кворум, мониторинг — ради простой очереди

✅ Без требований к replay и throughput:
   RabbitMQ, NATS JetStream или Redis Streams + Asynq/River
```

### Один consumer в Kafka при 12 партициях

```text
❌ Topic с 12 партициями, 1 consumer в группе
   → читает всё последовательно, партиции не используются

✅ partitions ≥ consumers в группе,
   либо параллелизм внутри consumer-а (goroutines по партициям)
```

### Consumer lag не мониторится

```text
❌ "Kafka справляется" → lag растёт неделями
   → retention вытесняет непрочитанные сообщения

✅ Consumer lag — SLO-метрика с алертом
```

## Когда несколько брокеров одновременно

Реальные системы часто комбинируют инструменты по их нишам:

```text
Kafka         — audit log, event sourcing, аналитические потоки
RabbitMQ/NATS — task queues, сервис-сервис messaging и RPC
Redis Pub/Sub — real-time cache invalidation, WebSocket backplane
```

Пример: e-commerce платформа

```text
1. Клиент создаёт заказ
   → REST API → Kafka topic "orders.created" (audit + replay)

2. Kafka consumer "fulfillment-service"
   → читает orders.created
   → публикует задачу в RabbitMQ queue "warehouse.tasks"
   → warehouse workers (competing consumers) разбирают задачи

3. При смене статуса заказа
   → Kafka topic "orders.status-changed"
   → Redis Pub/Sub "notifications:real-time"
   → WebSocket-серверы рассылают push-уведомления
```

## Быстрые характеристики

**Kafka** — distributed log: replay, высокий throughput, consumer groups по партициям, ordering per-partition; операционно самый тяжёлый.

**RabbitMQ** — классический broker: exchange/queue/binding, fanout/direct/topic routing, competing consumers, quorum queues, низкая latency; умеренная сложность.

**NATS** — лёгкий messaging: Core — at-most-once pub/sub + request-reply с минимальной эксплуатацией; JetStream — персистентные стримы с at-least-once, replay и KV store.

**Redis Streams** — append-only log внутри Redis: consumer groups (XREADGROUP), at-least-once через XACK, persistence до MAXLEN; хорош, когда Redis уже есть.

**Redis Pub/Sub** — fire-and-forget broadcast всем подписчикам: нет persistence, at-most-once; ниша — backplane и live-сигналы.

**Cloud (GCP Pub/Sub, SNS+SQS)** — managed: нулевая эксплуатация, нативный DLQ, at-least-once; платится объёмом и latency, ограничен replay.

## Interview-ready answer

**1. Какой брокер выбрать для задачи X?**

- Отвечать через уточнение требований, а не «X лучше»: нужен ли replay; какой throughput; допустима ли потеря; нужны ли consumer groups; какая latency; кто будет это эксплуатировать. Дальше trade-offs: Kafka даёт replay и throughput ценой эксплуатации; RabbitMQ — гибкий routing и низкую latency без replay; NATS — минимальную эксплуатацию и request-reply (JetStream добавляет надёжность); Redis Streams — разумный компромисс, если Redis уже есть; managed-облако — когда некому эксплуатировать.

**2. В чём разница delivery semantics?**

- At-most-once — потеря возможна, дубликатов нет (логи, метрики, live-сигналы). At-least-once — потерь нет, дубликаты возможны, поэтому consumer обязан быть идемпотентным. Exactly-once — самое дорогое, требует транзакций/дедупликации и работает только внутри конвейера брокера: для внешних side effects всё равно нужна идемпотентность приёмника. На практике стандарт — at-least-once + идемпотентный consumer.
