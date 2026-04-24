# Apache Kafka

Kafka — распределённый лог событий. Не очередь сообщений (как RabbitMQ), а append-only журнал с партицированием и consumer groups. Понимание архитектуры объясняет все его trade-offs.

---

## Архитектура: основные понятия

```
Producers ──► [Topic: orders]
                 │
        ┌────────┼────────┐
        │        │        │
   Partition 0  P1      P2    (N партиций = параллелизм)
   [0][1][2]  [0][1]  [0][1] ← offset
        │
   Leader (один брокер)
   ISR replicas (follower броkers копируют)
        │
Consumer Group A ──► consumer 1 читает P0, consumer 2 читает P1+P2
Consumer Group B ──► независимо читает те же партиции
```

### Topic

Логическая категория сообщений. Аналог таблицы в DB или очереди. Каждый топик делится на **партиции**.

### Partition

Физическая единица параллелизма. Append-only лог на диске. Каждое сообщение в партиции имеет уникальный **offset** (монотонно растущий integer).

Партиций больше → выше пропускная способность (параллельная запись/чтение).

### Offset

Позиция сообщения внутри партиции. Consumer хранит, **до какого offset** он дочитал. Это позволяет replay: начать с любого offset.

### Broker

Отдельный сервер Kafka. Кластер из нескольких брокеров для отказоустойчивости.

### ISR — In-Sync Replicas

Набор реплик, которые в sync с leader. Если leader упадёт, новым leader станет один из ISR. Размер ISR влияет на гарантии записи.

### Consumer Group

Несколько consumers, которые читают один топик **сообща**:
- Каждая партиция назначается **одному** consumer в группе
- Разные группы читают **независимо** (каждая с своего offset)
- Максимальный параллелизм группы = количество партиций

```
Topic: orders (3 партиции)

Consumer Group "shipping":
  consumer-1 → P0
  consumer-2 → P1
  consumer-3 → P2

Consumer Group "analytics":
  consumer-1 → P0, P1, P2 (один consumer = читает все)
```

---

## Delivery semantics

### At-most-once

```
Producer → (fire-and-forget) → Kafka
Consumer → читает → обрабатывает → коммитит offset ПЕРЕД обработкой
```

Если consumer упадёт после коммита но до обработки → сообщение потеряно.
Конфигурация: `acks=0`, auto-commit offset сразу.

### At-least-once

```
Producer → подтверждение от broker → повторная отправка при ошибке
Consumer → читает → обрабатывает → коммитит offset ПОСЛЕ обработки
```

Если consumer упадёт после обработки но до коммита → сообщение обработается дважды.
Конфигурация: `acks=1` или `acks=all`, manual commit.

Требует **идемпотентности** на стороне consumer.

### Exactly-once

Самая дорогая гарантия. Kafka реализует через:
1. **Idempotent producer** (`enable.idempotence=true`): брокер дедуплицирует дубликаты по sequence number
2. **Transactions** (`transactional.id`): атомарная запись в несколько топиков + коммит offset

```go
// Idempotent producer
producer, _ := kafka.NewProducer(&kafka.ConfigMap{
    "bootstrap.servers":  "localhost:9092",
    "enable.idempotence": true,
    "acks":               "all",
})
```

Exactly-once существенно снижает пропускную способность (2-phase commit). В большинстве случаев достаточно at-least-once + идемпотентный consumer.

---

## Producer: batching, compression, `acks`

### `acks` — уровень подтверждения

| `acks` | Поведение | Когда |
|---|---|---|
| `0` | Не ждать подтверждения | Logs, metrics — потеря OK |
| `1` | Ждать подтверждения от leader | Стандартный случай |
| `all` / `-1` | Ждать подтверждения от всех ISR | Критические данные |

```go
// acks=all + min.insync.replicas=2: запись подтверждена 2+ репликами
producer, _ := kafka.NewProducer(&kafka.ConfigMap{
    "acks":                    "all",
    "min.insync.replicas":     2, // на стороне broker (server config)
    "retries":                 3,
    "retry.backoff.ms":        100,
})
```

### Batching и linger

```
Producer → [batch buffer] → (flush при: batch.size ИЛИ linger.ms) → Broker
```

- `batch.size`: максимальный размер batch в байтах (default 16KB)
- `linger.ms`: ждать до N мс собирая сообщения в batch (default 0 = flush немедленно)

Увеличение `linger.ms` до 5–20мс значительно повышает throughput при незначительном росте latency.

### Compression

```
"compression.type": "snappy"  // snappy, gzip, lz4, zstd
```

Сжатие — на уровне batch. Kafka хранит и передаёт batch как есть, распаковывает только consumer.

- `snappy`: хороший баланс скорости и степени сжатия, рекомендован по умолчанию
- `lz4`: быстрее snappy, чуть хуже сжатие
- `zstd`: лучшее сжатие, медленнее (Go 1.21+ использует его для бинарей)
- `gzip`: медленный, высокое сжатие, legacy

---

## Consumer: poll loop, commit offset, rebalance

### Poll loop

Kafka consumer — pull-based. Consumer активно опрашивает broker.

```go
consumer, _ := kafka.NewConsumer(&kafka.ConfigMap{
    "bootstrap.servers": "localhost:9092",
    "group.id":          "my-group",
    "auto.offset.reset": "earliest",
})
consumer.SubscribeTopics([]string{"orders"}, nil)

for {
    msg, err := consumer.ReadMessage(time.Second)
    if err != nil {
        if err.(kafka.Error).Code() == kafka.ErrTimedOut {
            continue
        }
        log.Printf("consumer error: %v", err)
        break
    }
    
    // обработка
    processOrder(msg)
    
    // manual commit после успешной обработки
    consumer.CommitMessage(msg)
}
```

### Auto vs manual commit

```go
// Auto-commit: offset коммитится каждые N мс автоматически
// Риск: обработал, но commit не успел → reprocessing при restart (at-least-once)
// Хуже: commit прошёл, обработка не завершена → at-most-once
"enable.auto.commit":          true,
"auto.commit.interval.ms":     5000,

// Manual commit: явный контроль
"enable.auto.commit": false,
// После обработки:
consumer.CommitMessage(msg) // sync — блокирует
consumer.CommitAsync(nil)   // async — не блокирует, потенциальная потеря при crash
```

### Rebalance

Когда consumer добавляется или уходит из группы, Kafka **rebalance** — перераспределяет партиции между consumers.

Во время rebalance все consumers в группе **останавливают обработку**.

**Проблема**: при долгой обработке consumer не отправляет heartbeat → Kafka думает что умер → rebalance (даже если consumer жив).

```go
// Настройки для долгой обработки
"max.poll.interval.ms":    300000,  // max время между poll (default 5 min)
"session.timeout.ms":      30000,   // heartbeat timeout (default 45s)
"heartbeat.interval.ms":   3000,    // как часто слать heartbeat
```

---

## Kafka в Go: franz-go vs sarama vs confluent-kafka-go

| | franz-go | sarama | confluent-kafka-go |
|---|---|---|---|
| Тип | Pure Go | Pure Go | CGo (librdkafka) |
| Производительность | ⭐⭐⭐ лучший | ⭐⭐ средний | ⭐⭐⭐ лучший |
| API | modern, idiomatic | legacy, сложный | C-like |
| Поддержка | активная | медленная | Confluent |
| Зависимости | только stdlib | много | librdkafka |
| Cross-compile | ✅ | ✅ | ❌ (CGo) |
| Транзакции | ✅ | ✅ | ✅ |
| Рекомендация | новые проекты | legacy | высокая нагрузка |

### Пример с franz-go

```go
import "github.com/twmb/franz-go/pkg/kgo"

// Producer
client, _ := kgo.NewClient(
    kgo.SeedBrokers("localhost:9092"),
    kgo.RequiredAcks(kgo.AllISRAcks()),
    kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
)
defer client.Close()

// Sync produce
err := client.ProduceSync(ctx, &kgo.Record{
    Topic: "orders",
    Key:   []byte(orderID),
    Value: orderJSON,
}).FirstErr()

// Consumer
client, _ := kgo.NewClient(
    kgo.SeedBrokers("localhost:9092"),
    kgo.ConsumerGroup("my-group"),
    kgo.ConsumeTopics("orders"),
)

for {
    fetches := client.PollFetches(ctx)
    if errs := fetches.Errors(); len(errs) > 0 {
        log.Printf("fetch errors: %v", errs)
    }
    fetches.EachRecord(func(r *kgo.Record) {
        processOrder(r.Value)
        client.MarkCommitRecords(r)
    })
    client.CommitMarkedOffsets(ctx)
}
```

---

## DLQ — Dead Letter Queue паттерн

Сообщения, которые не удалось обработать N раз, перемещаются в отдельный топик для анализа.

```go
const maxRetries = 3

func processWithDLQ(ctx context.Context, client *kgo.Client, record *kgo.Record) {
    retries := getRetryCount(record.Headers)
    
    if err := processOrder(record.Value); err != nil {
        if retries >= maxRetries {
            // Отправляем в DLQ с метаданными об ошибке
            dlqRecord := &kgo.Record{
                Topic:   record.Topic + ".dlq",
                Key:     record.Key,
                Value:   record.Value,
                Headers: append(record.Headers,
                    kgo.RecordHeader{Key: "error", Value: []byte(err.Error())},
                    kgo.RecordHeader{Key: "original_topic", Value: []byte(record.Topic)},
                ),
            }
            client.ProduceSync(ctx, dlqRecord)
        } else {
            // Retry topic с увеличенным счётчиком
            retryRecord := &kgo.Record{
                Topic:   record.Topic + ".retry",
                Key:     record.Key,
                Value:   record.Value,
                Headers: setRetryCount(record.Headers, retries+1),
            }
            client.ProduceSync(ctx, retryRecord)
        }
        return
    }
    
    client.MarkCommitRecords(record)
}
```

---

## Log compaction vs retention

### Retention (time/size based)

Стандартный режим: сообщения удаляются по истечению времени или при превышении размера.

```
retention.ms=604800000     # хранить 7 дней
retention.bytes=1073741824 # или 1 GB
```

Используй когда важна история событий за период: clickstream, logs, транзакции.

### Log compaction

Kafka оставляет **только последнее значение** для каждого ключа.

```
Исходный лог:    user1:A  user2:B  user1:C  user3:D  user1:E
После compaction: user2:B  user3:D  user1:E   (только последние)
```

```
cleanup.policy=compact
```

Используй когда: топик = состояние (changelog), нужен последний known state для каждого ключа. Пример: цены товаров, настройки пользователей, состояние стримов (Kafka Streams).

**Tombstone**: значение `null` = удаление ключа из compacted лога.

---

## Когда Kafka не нужен

Kafka — не серебряная пуля. Добавляет значительную операционную сложность.

**Не используй Kafka когда:**

- Нужна **простая задачная очередь** (RabbitMQ, Redis будут проще)
- Команда < 5 инженеров и нет опыта с Kafka
- **Latency < 10ms** критична (Kafka добавляет batching latency)
- Нет replay/history требований
- Нет горизонтального масштабирования consumer'ов

**Kafka оправдан когда:**
- Throughput > 100k messages/sec
- Нужен replay/reprocessing исторических данных
- Несколько независимых consumer groups с разной логикой
- Event sourcing / CQRS архитектура
- Долгосрочное хранение событий (месяцы/годы)

---

## Типичные ошибки

### 1. Слишком мало партиций

Число партиций = максимальный параллелизм consumer group. 1 партиция → 1 active consumer.

```
Правило: partitions >= ожидаемый_max_consumers * 2
```

Партиции можно только добавлять, не уменьшать. Добавление партиций ломает ordering по ключу для существующих ключей.

### 2. Consumer lag не мониторится

Consumer lag = разница между latest offset и committed offset. Если lag растёт — consumer не справляется.

```bash
kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
  --group my-group --describe
```

### 3. Ordering нарушается при retry

При retry с новым producer сообщение может попасть в другую партицию или обогнать предыдущее.

```go
// Гарантия ordering: один ключ → одна партиция
// Используй ключ партицирования
record := &kgo.Record{
    Topic: "orders",
    Key:   []byte(userID), // все события user → одна партиция → ordered
    Value: orderData,
}
```

---

## Interview-ready answer

**Q: Чем Kafka отличается от RabbitMQ?**

Kafka — distributed log (append-only), RabbitMQ — message broker (queue semantics). Kafka хранит сообщения на диске по retention policy (дни/недели), RabbitMQ удаляет после consume. Kafka поддерживает replay — re-read с любого offset. RabbitMQ гибче в routing (exchange types). Kafka лучше для high-throughput event streaming (100k+ msg/s), RabbitMQ — для task queues и complex routing.

**Q: Что такое exactly-once и почему это дорого?**

Exactly-once в Kafka = idempotent producer (дедупликация по sequence number) + transactional API (атомарная запись + offset commit). Это добавляет round-trips для транзакционного coordinator, снижает throughput в 3–10 раз. В большинстве случаев достаточно at-least-once + идемпотентный consumer (проверяй по unique ID что уже обработал).

**Q: Как гарантировать ordering?**

Kafka гарантирует ordering только **внутри партиции**. Для ordering по сущности (все события user X по порядку) — используй user ID как partition key. Тогда все события одного пользователя попадают в одну партицию и читаются в порядке записи.
