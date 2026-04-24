# RabbitMQ

RabbitMQ — классический message broker на основе AMQP протокола. Его уникальная сила — гибкий routing через exchange/binding модель. Идеален для task queues, event-driven pipelines и сложного роутинга.

---

## Архитектура: exchange → binding → queue

```
Producer
    │
    ▼
[Exchange]  ←── тип и routing key определяют куда идёт сообщение
    │
  [Binding]  ←── правило "forward если routing key совпадает"
    │
  [Queue]   ←── очередь; consumer читает из неё
    │
    ▼
Consumer
```

**Exchange** — точка входа. Producer всегда публикует в exchange, никогда напрямую в queue.

**Queue** — хранилище сообщений. Consumer подписывается на queue, не на exchange.

**Binding** — связь между exchange и queue с опциональным routing key.

---

## Типы exchange

### Fanout — broadcast

```
Exchange (fanout)
    ├── Queue A  ← все получают копию
    ├── Queue B  ← все получают копию
    └── Queue C  ← все получают копию

Routing key игнорируется
```

Use case: broadcast событий (notifications, cache invalidation), чат (каждый participant имеет свою queue).

### Direct — точное совпадение

```
Exchange (direct)
    ├── [binding: routing_key="error"] → Queue "errors"
    ├── [binding: routing_key="info"]  → Queue "logs"
    └── [binding: routing_key="warn"]  → Queue "alerts"

Producer указывает routing_key → попадает в matching queue
```

Use case: логи по уровням, задачи по типу.

### Topic — паттерн по ключу

```
Exchange (topic)
    ├── [binding: "orders.#"]       → Queue "all-orders"
    ├── [binding: "orders.created"] → Queue "new-orders"
    └── [binding: "*.cancelled"]    → Queue "cancellations"

"#" — ноль или более слов
"*" — ровно одно слово
```

Use case: событийные системы с категоризацией (orders.created.EU, payments.failed.USD).

### Headers — по заголовкам

```
Exchange (headers, match: all/any)
    ├── [binding: {"format":"json", "type":"order"}] → Queue A
    └── [binding: {"format":"xml"}]                  → Queue B
```

Routing key игнорируется, решение по headers сообщения.
Используется редко — дороже topic.

---

## Delivery: ack/nack, prefetch, DLQ

### Manual Acknowledgement

```go
// AutoAck=false → consumer явно подтверждает обработку
delivery.Ack(false)    // подтвердить это сообщение
delivery.Nack(false, true)  // отклонить + requeue=true → вернуть в очередь
delivery.Reject(false) // отклонить + requeue=false → в DLQ (если настроен)
```

### Prefetch (QoS)

```go
// Не отправлять consumer более N несеwknitted сообщений
ch.Qos(
    10,    // prefetchCount: max N сообщений без ACK
    0,     // prefetchSize: 0 = без ограничения по байтам
    false, // global: false = per-consumer
)
```

Без prefetch — RabbitMQ может отправить все сообщения одному fast consumer'у, пока другой голодает.

### Dead Letter Queue (DLQ)

Сообщения попадают в DLQ когда:
- consumer отклонил (`nack` или `reject` с `requeue=false`)
- сообщение истекло (message TTL)
- очередь переполнена (queue overflow + `x-overflow: reject-publish`)

```go
// Объявить queue с DLQ
args := amqp.Table{
    "x-dead-letter-exchange":    "dlq.exchange",
    "x-dead-letter-routing-key": "failed",
    "x-message-ttl":             int32(30000), // TTL 30 секунд
}
ch.QueueDeclare("orders", true, false, false, false, args)
```

---

## Go код: publisher и subscriber

Пример основан на реальном коде из `lrn-streams/internal/transport/rabbitmq/`.

### Publisher (fanout exchange)

```go
package rabbitmq

import (
    "context"
    "encoding/json"
    
    amqp "github.com/rabbitmq/amqp091-go"
)

const exchangeName = "events.fanout"

type Publisher struct {
    conn *amqp.Connection
    ch   *amqp.Channel
}

func NewPublisher(amqpURL string) (*Publisher, error) {
    conn, err := amqp.Dial(amqpURL)
    if err != nil {
        return nil, fmt.Errorf("dial: %w", err)
    }
    ch, err := conn.Channel()
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("channel: %w", err)
    }

    // Декларируем exchange (idempotent — создаётся только если не существует)
    if err := ch.ExchangeDeclare(
        exchangeName,  // name
        "fanout",      // type
        true,          // durable — переживёт перезапуск брокера
        false,         // auto-delete
        false,         // internal
        false,         // no-wait
        nil,           // args
    ); err != nil {
        ch.Close(); conn.Close()
        return nil, fmt.Errorf("exchange declare: %w", err)
    }
    return &Publisher{conn: conn, ch: ch}, nil
}

func (p *Publisher) Publish(ctx context.Context, event any) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    return p.ch.PublishWithContext(ctx,
        exchangeName,  // exchange
        "",            // routing key (игнорируется для fanout)
        false,         // mandatory
        false,         // immediate
        amqp.Publishing{
            ContentType:  "application/json",
            DeliveryMode: amqp.Persistent, // пережить перезапуск
            Body:         data,
        },
    )
}

func (p *Publisher) Close() error {
    p.ch.Close()
    return p.conn.Close()
}
```

### Subscriber (exclusive queue per consumer)

```go
type Subscriber[T any] struct {
    conn *amqp.Connection
    ch   *amqp.Channel
    msgs chan T
}

func NewSubscriber[T any](amqpURL string) (*Subscriber[T], error) {
    conn, err := amqp.Dial(amqpURL)
    if err != nil {
        return nil, err
    }
    ch, err := conn.Channel()
    if err != nil {
        conn.Close()
        return nil, err
    }

    // Убедимся что exchange существует
    ch.ExchangeDeclare(exchangeName, "fanout", true, false, false, false, nil)

    // Exclusive queue: уникальная для этого подключения, удаляется при disconnect
    // Имя автогенерируется (пустая строка)
    q, err := ch.QueueDeclare(
        "",     // name: auto-generated
        false,  // durable: нет — exclusive queue ephemeral
        true,   // auto-delete: удалить при disconnect
        true,   // exclusive: только это соединение
        false,
        nil,
    )
    if err != nil {
        ch.Close(); conn.Close()
        return nil, err
    }

    // Связываем queue с exchange
    if err := ch.QueueBind(q.Name, "", exchangeName, false, nil); err != nil {
        ch.Close(); conn.Close()
        return nil, err
    }

    // Начинаем потребление
    deliveries, err := ch.Consume(
        q.Name, // queue
        "",     // consumer tag (auto)
        false,  // auto-ack: нет, подтверждаем вручную
        true,   // exclusive
        false,  // no-local
        false,  // no-wait
        nil,
    )
    if err != nil {
        ch.Close(); conn.Close()
        return nil, err
    }

    // Ограничиваем prefetch
    ch.Qos(10, 0, false)

    s := &Subscriber[T]{
        conn: conn,
        ch:   ch,
        msgs: make(chan T, 64),
    }
    go s.consumeLoop(deliveries)
    return s, nil
}

func (s *Subscriber[T]) consumeLoop(deliveries <-chan amqp.Delivery) {
    defer close(s.msgs)
    for d := range deliveries {
        var event T
        if err := json.Unmarshal(d.Body, &event); err != nil {
            d.Nack(false, false) // отклонить → в DLQ
            continue
        }
        s.msgs <- event
        d.Ack(false) // подтвердить после успешной обработки
    }
}

func (s *Subscriber[T]) Messages() <-chan T {
    return s.msgs
}

func (s *Subscriber[T]) Close() error {
    s.ch.Close()
    return s.conn.Close()
}
```

---

## Competing consumers паттерн

Несколько consumers на **одной** queue — задачи распределяются между ними:

```
Queue "tasks"
    ├── Consumer A  ← обрабатывает task 1, 4, 7 ...
    ├── Consumer B  ← обрабатывает task 2, 5, 8 ...
    └── Consumer C  ← обрабатывает task 3, 6, 9 ...
```

```go
// Три instance одного сервиса подключаются к одной queue
// RabbitMQ автоматически балансирует задачи (round-robin)
ch.QueueDeclare("tasks", true, false, false, false, nil) // shared, durable
ch.Consume("tasks", "worker-1", false, false, false, false, nil)
```

Отличие от Kafka: в RabbitMQ нет явного понятия consumer group, competing consumers реализуется просто несколькими подключениями к одной queue.

---

## Когда RabbitMQ, когда Kafka

| Критерий | RabbitMQ | Kafka |
|---|---|---|
| Основная модель | Message queue | Event log |
| Routing | Гибкий (exchange types) | По ключу → партиция |
| Throughput | Умеренный (50-100k msg/s) | Высокий (1M+ msg/s) |
| Persistence | Optional (durable) | По умолчанию |
| Replay | ❌ | ✅ |
| Consumer groups | Competing consumers (queue) | True groups (partition assignment) |
| Ordering | Per-queue | Per-partition |
| Latency | Очень низкая (мкс-мс) | Выше (batching) |
| Операционная сложность | Умеренная | Высокая |

**Выбирай RabbitMQ когда:**
- Task queues (email, notifications, background jobs)
- Сложный routing по типам событий
- Нужна низкая latency
- Команда не готова к Kafka

**Выбирай Kafka когда:**
- High-throughput event streaming
- Нужен replay/reprocessing
- Event sourcing / audit log
- Несколько независимых consumer groups

---

## Interview-ready answer

**Q: Объясни exchange/queue/binding в RabbitMQ**

Producer публикует в exchange — routing component. Exchange не хранит сообщения сам. Binding связывает exchange и queue с правилом: "если routing key совпадает — форвардить сюда". Queue хранит сообщения для consumer. Тип exchange определяет routing логику: fanout (broadcast всем), direct (точное совпадение ключа), topic (glob-паттерн по ключу).

**Q: Зачем exclusive queue в fanout-архитектуре?**

При broadcast каждый consumer должен получить копию сообщения. Для этого каждый consumer создаёт **свою** queue (exclusive, auto-delete) и привязывает к fanout exchange. Тогда fanout exchange копирует сообщение в каждую queue. Если бы все consumers читали из одной queue — это был бы competing consumers (load balancing), а не broadcast.
