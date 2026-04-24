# Message Brokers: Сравнение

Быстрое сравнение для выбора инструмента. Финальный раздел после изучения каждого брокера.

---

## Большая таблица

| | Kafka | RabbitMQ | Redis Streams | Redis Pub/Sub | gRPC Stream |
|---|---|---|---|---|---|
| **Модель** | Event log | Message queue | Append-only log | Fire-and-forget | Bidirectional stream |
| **Persistence** | ✅ дни/недели | ✅ durable | ✅ до MAXLEN | ❌ ephemeral | ❌ |
| **Delivery** | At-least-once | At-least-once | At-least-once | At-most-once | At-least-once |
| **Exactly-once** | ✅ (дорого) | ❌ | ❌ | ❌ | ❌ |
| **Consumer groups** | ✅ partition assignment | Competing consumers | ✅ XREADGROUP | ❌ broadcast | N/A |
| **Replay** | ✅ с любого offset | ❌ | ✅ с любого ID | ❌ | ❌ |
| **Ordering** | Per-partition | Per-queue | Per-stream | Нет гарантий | Per-stream |
| **Throughput** | 1M+ msg/s | 50-100k msg/s | 100k msg/s | 100k+ msg/s | Зависит от сети |
| **Latency** | 5-20ms (batching) | < 1ms | < 1ms | < 1ms | < 1ms |
| **Routing** | По ключу → партиция | Exchange types | По stream key | По channel/pattern | N/A |
| **Ops complexity** | Высокая (кластер) | Умеренная | Низкая (есть Redis) | Низкая (есть Redis) | Низкая |
| **Доп. зависимость** | Kafka cluster | RabbitMQ server | Redis | Redis | нет |
| **Broadcast** | Через consumer groups | Fanout exchange | По одной группе | ✅ нативно | ✅ через registry |
| **DLQ** | ✅ отдельный топик | ✅ x-dead-letter | Вручную | ❌ | ❌ |

---

## Decision tree: когда что выбирать

```
Нужна надёжная доставка?
├── НЕТ → Redis Pub/Sub
│         (real-time broadcast, cache invalidation)
│
└── ДА → Нужен replay/долгосрочное хранение?
         ├── ДА → Kafka
         │        (event sourcing, audit log, high-throughput)
         │
         └── НЕТ → Нужен сложный routing (exchange types)?
                   ├── ДА → RabbitMQ
                   │        (task queues, complex pipelines)
                   │
                   └── НЕТ → Redis уже в стеке?
                             ├── ДА → Redis Streams
                             │        (at-least-once, consumer groups, умеренный throughput)
                             │
                             └── НЕТ → RabbitMQ или Redis Streams
                                        (зависит от операционных предпочтений)

Нужна real-time двусторонняя связь client-server?
└── gRPC Streaming (или WebSocket)
```

---

## Типичные ошибки выбора

### Redis Pub/Sub для надёжной доставки

```
❌ Неправильно:
   "Используем Redis Pub/Sub для отправки emails"
   → Если email-worker оффлайн — письма потеряны

✅ Правильно:
   Redis Streams или RabbitMQ для email queue
   Redis Pub/Sub только для "оповестить живые инстансы прямо сейчас"
```

### Kafka для простой task queue

```
❌ Неправильно:
   "Ставим Kafka для фоновых задач"
   → ZooKeeper/KRaft, брокеры, мониторинг — ради простой очереди

✅ Правильно:
   Если нет нужды в replay и высоком throughput:
   RabbitMQ или Redis Streams + Asynq/River
```

### Один consumer в Kafka = не использует партиционирование

```
❌ Неправильно:
   Topic с 12 партициями, 1 consumer в group
   → Читает последовательно, партиции не помогают

✅ Правильно:
   partitions ≥ consumers в группе
   Или использовать параллелизм внутри одного consumer (goroutines)
```

### Игнорирование consumer lag

```
❌ Неправильно:
   "Kafka справляется" → lag растёт неделями
   → В конце концов retention вытесняет непрочитанные сообщения

✅ Правильно:
   Мониторить consumer lag как SLO-метрику
   Алерт при lag > threshold
```

---

## Когда несколько брокеров одновременно

Реальные системы часто комбинируют:

```
Kafka (audit log, event sourcing)
  + RabbitMQ (task queues, email/notification delivery)
  + Redis Pub/Sub (real-time cache invalidation, WebSocket backplane)
```

Пример: e-commerce платформа

```
1. Клиент создаёт заказ
   → REST API → Kafka topic "orders.created" (audit + replay)

2. Kafka consumer "fulfillment-service"
   → читает orders.created
   → публикует задачу в RabbitMQ queue "warehouse.tasks"
   → Warehouse workers (competing consumers) берут задачи

3. При смене статуса заказа
   → Kafka topic "orders.status-changed"
   → Redis Pub/Sub "notifications:real-time"
   → WebSocket серверы рассылают push-уведомления
```

---

## Быстрые характеристики для интервью

**Kafka**: distributed log, replay, высокий throughput, consumer groups по партициям, ordering per-partition, операционная сложность высокая.

**RabbitMQ**: классический broker, exchange/queue/binding, fanout/direct/topic routing, competing consumers, low latency, умеренная сложность.

**Redis Streams**: встроен в Redis, append-only log, consumer groups (XREADGROUP), at-least-once (XACK), persistence до MAXLEN, умеренный throughput.

**Redis Pub/Sub**: fire-and-forget, at-most-once, broadcast всем subscribers, нет persistence, простейший механизм, backplane паттерн.

---

## Interview-ready answer

**Q: Какой брокер выбрать для задачи X?**

Структура ответа:
1. Уточнить: нужен replay? какой throughput? нужны consumer groups? какая latency?
2. Ответить через trade-offs: "Kafka даёт replay и высокий throughput, но операционно дороже. RabbitMQ проще и гибче в routing, но нет replay. Redis Streams — разумный компромисс если Redis уже есть."
3. Не давать абстрактный ответ "Kafka лучше" — всегда через конкретные требования.

**Q: Объясни разницу delivery semantics**

- At-most-once: потеря OK, дубликаты исключены. Логи, метрики.
- At-least-once: потеря недопустима, дубликаты возможны → consumer должен быть идемпотентным.
- Exactly-once: и потеря и дубликаты исключены → самое дорогое, требует транзакций.

На практике: большинство production систем — at-least-once + идемпотентность на consumer стороне.
