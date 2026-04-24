# Redis Streams

Redis Streams — append-only лог встроенный в Redis. Уникальная позиция: проще и дешевле Kafka, надёжнее Redis Pub/Sub (есть persistence и consumer groups), часть существующего Redis инстанса.

---

## Базовые команды: XADD, XREADGROUP, XACK

### XADD — добавить сообщение

```redis
XADD stream-key * field1 value1 field2 value2
# "*" = auto-generated ID вида "timestamp-sequence", например "1711300000000-0"

XADD orders * order_id "123" user_id "alice" status "created"
# → "1711300000000-0"

# Ограничение длины потока (MAXLEN)
XADD orders MAXLEN ~ 10000 * order_id "123"
# "~" = приблизительное ограничение (быстрее, чем точное)
```

### XREAD — читать без consumer group

```redis
# Читать новые сообщения с последнего known ID
XREAD COUNT 10 BLOCK 1000 STREAMS orders $
# BLOCK 1000 = блокировать до 1 секунды если нет новых сообщений
# "$" = начать с новых (не перечитывать старые)
# "0" = читать с самого начала
```

### XREADGROUP — читать с consumer group

```redis
# Создать группу
XGROUP CREATE orders shipping-group $ MKSTREAM
# "$" = читать только новые сообщения
# "0" = читать с начала

# Читать как consumer
XREADGROUP GROUP shipping-group worker-1 COUNT 10 BLOCK 1000 STREAMS orders >
# ">" = получить новые (не pending) сообщения
```

### XACK — подтвердить обработку

```redis
XACK orders shipping-group "1711300000000-0"
# Убирает из Pending Entries List
```

### Дополнительные команды

```redis
# Посмотреть все сообщения в стриме
XRANGE orders - +

# Количество сообщений
XLEN orders

# Pending Entries List (необработанные)
XPENDING orders shipping-group - + 10

# Перенять застрявшее сообщение (XCLAIM)
XCLAIM orders shipping-group worker-2 60000 "1711300000000-0"
# 60000 = min-idle-time в мс (передать если висит > 1 мин)
```

---

## Consumer groups: механика

```
Stream: orders
  │
  ├── [1711300000000-0] order_id=123
  ├── [1711300000001-0] order_id=124
  └── [1711300000002-0] order_id=125

Consumer Group "shipping" (group offset ►)
  │
  ├── worker-1  ← обрабатывает 1711300000000-0 (pending)
  └── worker-2  ← обрабатывает 1711300000001-0 (pending)
  
  last-delivered: 1711300000001-0
  
Consumer Group "analytics" (независимый group offset)
  │
  └── analyst-1 ← читает с начала, может читать те же сообщения
```

**Ключевые свойства:**

1. Каждое сообщение доставляется **одному** consumer в группе (не broadcast!)
2. Разные группы читают **независимо** — как Kafka consumer groups
3. Consumer хранит **Pending Entries List (PEL)** — полученные, но не подтверждённые

### PEL и восстановление

```
Scenario: worker-1 получил сообщение, но упал

PEL worker-1: [1711300000000-0] (idle > 60 секунд)

XAUTOCLAIM orders shipping-group worker-2 60000 0-0 COUNT 10
# Автоматически перенять сообщения idle > 60 сек
```

---

## Go код: producer и consumer (из lrn-streams)

### Producer

```go
package redisstream

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/redis/go-redis/v9"
)

const streamKey = "events:stream"

type Producer struct {
    rdb *redis.Client
}

func NewProducer(rdb *redis.Client) *Producer {
    return &Producer{rdb: rdb}
}

func (p *Producer) Send(ctx context.Context, event any) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    return p.rdb.XAdd(ctx, &redis.XAddArgs{
        Stream: streamKey,
        MaxLen: 10000,    // ограничиваем длину
        Approx: true,     // ~ (приблизительно, быстрее)
        Values: map[string]any{"data": string(data)},
    }).Err()
}
```

### Consumer с XREADGROUP

```go
type Consumer struct {
    rdb      *redis.Client
    group    string
    consumer string
    ch       chan []byte
    cancel   context.CancelFunc
}

func NewConsumer(rdb *redis.Client, group, consumerName string) *Consumer {
    ctx, cancel := context.WithCancel(context.Background())
    
    // Создаём группу и stream если не существуют
    rdb.XGroupCreateMkStream(ctx, streamKey, group, "$").Err()
    // Игнорируем ошибку "BUSYGROUP Consumer Group name already exists"
    
    c := &Consumer{
        rdb:      rdb,
        group:    group,
        consumer: consumerName,
        ch:       make(chan []byte, 64),
        cancel:   cancel,
    }
    go c.readLoop(ctx)
    return c
}

func (c *Consumer) readLoop(ctx context.Context) {
    defer close(c.ch)
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    c.group,
            Consumer: c.consumer,
            Streams:  []string{streamKey, ">"},
            Count:    10,
            Block:    time.Second, // блокируем на 1 сек если нет данных
        }).Result()
        
        if err != nil {
            if err == redis.Nil || ctx.Err() != nil {
                continue // timeout или контекст отменён
            }
            log.Printf("[redis-stream] read error: %v", err)
            time.Sleep(time.Second) // backoff перед retry
            continue
        }

        for _, stream := range streams {
            for _, msg := range stream.Messages {
                data, ok := msg.Values["data"].(string)
                if !ok {
                    c.rdb.XAck(ctx, streamKey, c.group, msg.ID)
                    continue
                }
                c.ch <- []byte(data)
                
                // ACK после успешной обработки
                c.rdb.XAck(ctx, streamKey, c.group, msg.ID)
            }
        }
    }
}

func (c *Consumer) Messages() <-chan []byte {
    return c.ch
}

func (c *Consumer) Close() error {
    c.cancel()
    return nil
}
```

### Восстановление pending сообщений

```go
// При старте consumer — проверить свои pending сообщения
func (c *Consumer) recoverPending(ctx context.Context) error {
    streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
        Group:    c.group,
        Consumer: c.consumer,
        Streams:  []string{streamKey, "0"}, // "0" = читать pending, не новые
        Count:    100,
    }).Result()
    if err != nil {
        return err
    }
    // Обработать и XAck как обычно
    for _, stream := range streams {
        for _, msg := range stream.Messages {
            c.processAndAck(ctx, msg)
        }
    }
    return nil
}

// XAUTOCLAIM — перенять застрявшие сообщения от других consumer
func (c *Consumer) claimStuck(ctx context.Context) {
    // Перенять pending дольше 5 минут
    msgs, _, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
        Stream:   streamKey,
        Group:    c.group,
        Consumer: c.consumer,
        MinIdle:  5 * time.Minute,
        Start:    "0-0",
        Count:    10,
    }).Result()
    // обработать msgs...
}
```

---

## Когда Redis Streams vs Kafka vs RabbitMQ

| | Redis Streams | Kafka | RabbitMQ |
|---|---|---|---|
| Persistence | ✅ (до MAXLEN или TTL) | ✅ долгосрочная | ✅ durable queues |
| Consumer groups | ✅ встроено | ✅ | Competing consumers |
| Replay | ✅ по ID | ✅ по offset | ❌ |
| Throughput | Умеренный | Очень высокий | Умеренный |
| Операционная сложность | Низкая (уже есть Redis) | Высокая | Умеренная |
| Ordering | Per-stream | Per-partition | Per-queue |
| Дополнительные зависимости | ❌ (используй свой Redis) | ✅ Kafka cluster | ✅ RabbitMQ |

**Выбирай Redis Streams когда:**
- Redis уже в стеке
- Нужны consumer groups + at-least-once, но без Kafka
- Умеренный throughput (< 100k msg/s)
- Нужна простота операции (один Redis, не отдельный кластер)

**Важное ограничение**: Redis Streams — не true event log как Kafka. MAXLEN ограничивает историю. При больших объёмах и нужде в долгосрочном хранении — Kafka предпочтительнее.

---

## Interview-ready answer

**Q: Чем Redis Streams отличается от Redis Pub/Sub?**

Pub/Sub — fire-and-forget, нет persistence. Если subscriber не подключён в момент PUBLISH — сообщение потеряно. Streams — append-only лог с persistence, consumer groups, подтверждениями (XACK) и возможностью replay. Streams дают at-least-once гарантию, Pub/Sub — at-most-once.

**Q: Что такое PEL и зачем нужен XCLAIM?**

Pending Entries List — список сообщений, которые consumer получил но не XACK'нул. Если consumer упал — его pending сообщения висят в PEL. XCLAIM (или XAUTOCLAIM) позволяет другому consumer забрать эти сообщения и переобработать. Так реализуется failover без потери данных.
