# RabbitMQ

RabbitMQ — классический message broker на основе протокола AMQP 0-9-1. Его сила — гибкая маршрутизация через модель exchange/binding. Ниша: task queues, event-driven пайплайны, сложный routing. Контраст с [Kafka](./01-kafka.md): очередь удаляет сообщение после подтверждения, реплея нет.

## Содержание

- [Архитектура: exchange → binding → queue](#архитектура-exchange--binding--queue)
- [Типы exchange](#типы-exchange)
- [Надёжность: confirms, ack/nack, prefetch, DLQ](#надёжность-confirms-acknack-prefetch-dlq)
- [Quorum queues](#quorum-queues)
- [Go код: publisher и subscriber](#go-код-publisher-и-subscriber)
- [Competing consumers паттерн](#competing-consumers-паттерн)
- [Production-грабли](#production-грабли)
- [Когда RabbitMQ, когда Kafka](#когда-rabbitmq-когда-kafka)
- [Interview-ready answer](#interview-ready-answer)

## Архитектура: exchange → binding → queue

```mermaid
flowchart LR
    P[Producer] --> E{{"Exchange<br/>(тип + routing key)"}}
    E -- binding --> Q1[Queue A]
    E -- binding --> Q2[Queue B]
    Q1 --> C1[Consumer 1]
    Q2 --> C2[Consumer 2]
```

- **Exchange** — точка входа. Producer всегда публикует в exchange, никогда напрямую в queue.
- **Queue** — хранилище сообщений. Consumer подписывается на queue, не на exchange.
- **Binding** — связь exchange → queue с опциональным routing key: правило «форвардить сюда, если ключ совпадает».

## Типы exchange

### Fanout — broadcast

```text
Exchange (fanout)
    ├── Queue A  ← все получают копию
    ├── Queue B  ← все получают копию
    └── Queue C  ← все получают копию

Routing key игнорируется
```

Use case: broadcast событий (notifications, cache invalidation); чат, где у каждого участника своя queue.

### Direct — точное совпадение

```text
Exchange (direct)
    ├── [binding: routing_key="error"] → Queue "errors"
    ├── [binding: routing_key="info"]  → Queue "logs"
    └── [binding: routing_key="warn"]  → Queue "alerts"
```

Use case: логи по уровням, задачи по типу.

### Topic — паттерн по ключу

```text
Exchange (topic)
    ├── [binding: "orders.#"]       → Queue "all-orders"
    ├── [binding: "orders.created"] → Queue "new-orders"
    └── [binding: "*.cancelled"]    → Queue "cancellations"

"#" — ноль или более слов
"*" — ровно одно слово
```

Use case: событийные системы с категоризацией (`orders.created.EU`, `payments.failed.USD`).

### Headers — по заголовкам

```text
Exchange (headers, match: all/any)
    ├── [binding: {"format":"json", "type":"order"}] → Queue A
    └── [binding: {"format":"xml"}]                  → Queue B
```

Routing key игнорируется, решение — по headers сообщения. Используется редко: дороже topic.

## Надёжность: confirms, ack/nack, prefetch, DLQ

Надёжная доставка в RabbitMQ собирается из **двух половин** — подтверждения на стороне публикации и на стороне потребления. Частая ошибка — настроить только вторую.

### Publisher confirms — подтверждение публикации

`DeliveryMode: Persistent` сам по себе **не гарантирует**, что сообщение доехало: publish — асинхронная операция, и без confirm-режима брокер может упасть до записи на диск, а publisher об этом не узнает. Confirm-режим заставляет брокер подтверждать каждую публикацию:

```go
ch.Confirm(false) // перевести канал в confirm mode

// amqp091-go: publish с ожиданием подтверждения
conf, err := ch.PublishWithDeferredConfirmWithContext(ctx,
    exchangeName, routingKey, false, false,
    amqp.Publishing{
        DeliveryMode: amqp.Persistent,
        ContentType:  "application/json",
        Body:         data,
    },
)
if err != nil {
    return err
}
if ok, err := conf.WaitContext(ctx); err != nil || !ok {
    return fmt.Errorf("publish not confirmed: %w", err) // retry / outbox
}
```

Итог: at-least-once на публикации = persistent message + durable queue + publisher confirm. Для критичных событий поверх этого — outbox-паттерн ([06-outbox-idempotency-and-payment-flow.md](../06-databases/relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md)).

### Manual acknowledgement — подтверждение обработки

```go
// autoAck=false → consumer явно подтверждает обработку
d.Ack(false)         // подтвердить это сообщение
d.Nack(false, true)  // отклонить, requeue=true → вернуть в очередь
d.Nack(false, false) // отклонить, requeue=false → в DLQ (если настроен)
```

Осторожно с `requeue=true` для постоянно падающих сообщений: без счётчика попыток получается бесконечный цикл (см. DLQ ниже).

### Prefetch (QoS)

```go
// Не отправлять consumer-у более N неподтверждённых (unacked) сообщений.
// Вызывать ДО ch.Consume — иначе первые доставки уйдут без лимита.
ch.Qos(
    10,    // prefetchCount: max N сообщений без ack
    0,     // prefetchSize: 0 = без лимита по байтам
    false, // global: false = per-consumer
)
```

Без prefetch брокер может отгрузить тысячи сообщений одному быстрому consumer-у, пока остальные простаивают, а при падении этого consumer-а все unacked вернутся в очередь разом.

### Dead Letter Queue (DLQ)

Сообщение попадает в DLQ, когда: consumer отклонил его с `requeue=false`; истёк message TTL; очередь переполнена.

```go
args := amqp.Table{
    "x-dead-letter-exchange":    "dlq.exchange",
    "x-dead-letter-routing-key": "failed",
    "x-message-ttl":             int32(30000), // TTL 30 секунд
}
ch.QueueDeclare("orders", true, false, false, false, args)
```

## Quorum queues

Классические зеркалируемые очереди (classic mirrored queues) **deprecated** и удалены в RabbitMQ 4.x. Современный ответ на HA — **quorum queues**: реплицируемая очередь на основе Raft.

- Данные реплицируются на кворум узлов; очередь переживает падение узла без потери подтверждённых сообщений.
- Всегда durable; в объявлении — `x-queue-type: quorum`.
- Встроенный счётчик доставок `x-delivery-count` → нативный `delivery-limit` (после N попыток — в DLQ), чего нет у классических очередей.
- Цена: больше памяти/диска, чуть выше latency, не поддерживают некоторые фичи классических (priority, exclusive).

```go
args := amqp.Table{"x-queue-type": "quorum", "x-delivery-limit": int32(5)}
ch.QueueDeclare("orders", true, false, false, false, args)
```

Правило: для всего, что нельзя терять, — quorum queue; классические — для эфемерных и exclusive-очередей.

## Go код: publisher и subscriber

Сценарий — fanout broadcast: каждый подписчик получает копию каждого события.

### Publisher (fanout exchange)

```go
package rabbitmq

import (
    "context"
    "encoding/json"
    "fmt"

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

    // Декларация idempotent — создаётся, только если не существует
    if err := ch.ExchangeDeclare(
        exchangeName, // name
        "fanout",     // type
        true,         // durable — переживёт перезапуск брокера
        false,        // auto-delete
        false,        // internal
        false,        // no-wait
        nil,          // args
    ); err != nil {
        ch.Close()
        conn.Close()
        return nil, fmt.Errorf("exchange declare: %w", err)
    }

    if err := ch.Confirm(false); err != nil { // publisher confirms
        ch.Close()
        conn.Close()
        return nil, fmt.Errorf("confirm mode: %w", err)
    }
    return &Publisher{conn: conn, ch: ch}, nil
}

func (p *Publisher) Publish(ctx context.Context, event any) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    conf, err := p.ch.PublishWithDeferredConfirmWithContext(ctx,
        exchangeName, // exchange
        "",           // routing key (fanout игнорирует)
        false, false,
        amqp.Publishing{
            ContentType:  "application/json",
            DeliveryMode: amqp.Persistent,
            Body:         data,
        },
    )
    if err != nil {
        return fmt.Errorf("publish: %w", err)
    }
    if ok, err := conf.WaitContext(ctx); err != nil || !ok {
        return fmt.Errorf("not confirmed: %w", err)
    }
    return nil
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

    ch.ExchangeDeclare(exchangeName, "fanout", true, false, false, false, nil)

    // Prefetch — ДО Consume
    if err := ch.Qos(10, 0, false); err != nil {
        ch.Close()
        conn.Close()
        return nil, err
    }

    // Exclusive queue: своя на каждое подключение, имя автогенерируется,
    // удаляется при disconnect — так fanout даёт broadcast каждому подписчику
    q, err := ch.QueueDeclare(
        "",    // name: auto-generated
        false, // durable: нет — очередь эфемерная
        true,  // auto-delete
        true,  // exclusive
        false,
        nil,
    )
    if err != nil {
        ch.Close()
        conn.Close()
        return nil, err
    }

    if err := ch.QueueBind(q.Name, "", exchangeName, false, nil); err != nil {
        ch.Close()
        conn.Close()
        return nil, err
    }

    deliveries, err := ch.Consume(
        q.Name, // queue
        "",     // consumer tag (auto)
        false,  // auto-ack: нет — подтверждаем вручную
        true,   // exclusive
        false,  // no-local
        false,  // no-wait
        nil,
    )
    if err != nil {
        ch.Close()
        conn.Close()
        return nil, err
    }

    s := &Subscriber[T]{conn: conn, ch: ch, msgs: make(chan T, 64)}
    go s.consumeLoop(deliveries)
    return s, nil
}

// handler вызывается синхронно: ack уходит только после успешной обработки.
// Если отправлять сообщение в канал и ack-ать сразу — подтверждение произойдёт
// ДО фактической обработки, и at-least-once превратится в "доставлено в буфер".
func (s *Subscriber[T]) consumeLoop(deliveries <-chan amqp.Delivery) {
    defer close(s.msgs)
    for d := range deliveries {
        var event T
        if err := json.Unmarshal(d.Body, &event); err != nil {
            d.Nack(false, false) // poison message не возвращаем в очередь — в DLQ
            continue
        }
        s.msgs <- event // блокирует, пока потребитель не заберёт
        d.Ack(false)
    }
}

func (s *Subscriber[T]) Messages() <-chan T { return s.msgs }

func (s *Subscriber[T]) Close() error {
    s.ch.Close()
    return s.conn.Close()
}
```

## Competing consumers паттерн

Несколько consumers на **одной** queue — задачи распределяются между ними:

```text
Queue "tasks"
    ├── Consumer A  ← task 1, 4, 7 ...
    ├── Consumer B  ← task 2, 5, 8 ...
    └── Consumer C  ← task 3, 6, 9 ...
```

```go
// Три инстанса одного сервиса подключаются к одной queue —
// RabbitMQ балансирует задачи (round-robin с учётом prefetch)
ch.QueueDeclare("tasks", true, false, false, false,
    amqp.Table{"x-queue-type": "quorum"})
ch.Consume("tasks", "worker-1", false, false, false, false, nil)
```

Отличие от Kafka: явного понятия consumer group нет — work-sharing получается просто несколькими подписчиками одной queue, а broadcast — отдельной queue на каждого подписчика (fanout).

## Production-грабли

- **`amqp091-go` не переподключается сам.** При обрыве соединения `Connection` и `Channel` мёртвые навсегда; канал `deliveries` закрывается. Нужен reconnect-слой: слушать `conn.NotifyClose(...)`, пересоздавать connection/channel/подписки с backoff — или взять обёртку с автопереподключением. Это самая частая причина «consumer молча перестал получать сообщения после сетевого моргания».
- **Unbounded queue.** Если consumers отстают, очередь растёт, пока не съест диск/память и брокер не начнёт душить publishers (flow control). Ставить `x-max-length`/TTL + алерт на глубину очереди.
- **Одно соединение на всё.** Каналы мультиплексируются в одном TCP-соединении, но publish и consume лучше разносить по разным соединениям: flow control на публикацию может заблокировать и потребление.

## Когда RabbitMQ, когда Kafka

| Критерий | RabbitMQ | Kafka |
|---|---|---|
| Основная модель | Message queue | Event log |
| Routing | Гибкий (exchange types) | По ключу → партиция |
| Throughput | Умеренный (50–100k msg/s) | Высокий (1M+ msg/s) |
| Persistence | Durable queues + persistent messages | По умолчанию, retention днями |
| Replay | ❌ (после ack сообщение удалено) | ✅ с любого offset |
| Распределение работы | Competing consumers (queue) | Consumer groups (partition assignment) |
| Ordering | Per-queue (ломается при requeue/нескольких consumers) | Per-partition |
| Latency | Очень низкая (доли мс) | Выше (batching) |
| Операционная сложность | Умеренная | Высокая |

RabbitMQ — когда: task queues (email, notifications, background jobs), сложный routing по типам событий, низкая latency, брокер нужен «попроще». Kafka — когда: high-throughput streaming, replay/reprocessing, event sourcing, несколько независимых групп читателей. Сводная таблица по всем брокерам — [07-comparison.md](./07-comparison.md).

## Interview-ready answer

**1. Объясни модель exchange/queue/binding.**

- Producer публикует в exchange — компонент маршрутизации, который сам сообщения не хранит. Binding связывает exchange с queue правилом «форвардить, если routing key совпадает». Queue хранит сообщения для consumers. Тип exchange задаёт логику: fanout — broadcast всем привязанным очередям, direct — точное совпадение ключа, topic — glob-паттерн (`orders.#`), headers — по заголовкам.

**2. Как получить надёжную доставку end-to-end?**

- Три составляющие: durable queue (лучше quorum) + persistent message + **publisher confirms** на публикации; manual ack после обработки на потреблении; идемпотентный consumer, потому что всё это даёт at-least-once — дубликаты возможны. Без confirms publish — fire-and-forget: брокер может упасть до записи, и publisher не узнает.

**3. Зачем exclusive queue в fanout-архитектуре?**

- При broadcast каждый consumer должен получить копию сообщения, поэтому каждый создаёт **свою** queue (exclusive, auto-delete) и привязывает её к fanout exchange — тот копирует сообщение во все очереди. Если бы все читали одну queue, получился бы competing consumers (распределение нагрузки), а не broadcast.

**4. Что такое quorum queues и чем они лучше mirrored?**

- Реплицируемые очереди на Raft: подтверждённое сообщение хранится на кворуме узлов и переживает падение узла. Классические mirrored queues имели проблемы с ресинхронизацией после failover и удалены в RabbitMQ 4.x. Бонус quorum — встроенный delivery-limit: после N неудачных доставок сообщение уходит в DLQ, что решает проблему бесконечного requeue.

**5. Зачем prefetch (QoS)?**

- Ограничивает число unacked-сообщений на consumer-а. Без него брокер отгружает всё одному быстрому потребителю: остальные простаивают, а при падении этого consumer-а вся пачка возвращается в очередь разом. Ставится до `Consume`; типичные значения — единицы–десятки при тяжёлой обработке.
