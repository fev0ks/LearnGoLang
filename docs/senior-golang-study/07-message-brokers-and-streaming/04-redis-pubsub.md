# Redis Pub/Sub

Redis Pub/Sub — простейший механизм broadcast сообщений. Publisher не знает о subscribers, subscriber получает все сообщения канала. At-most-once, нет persistence. Простейший из всех broker-механизмов.

---

## Механика: PUBLISH / SUBSCRIBE / PSUBSCRIBE

### PUBLISH — отправить в канал

```redis
PUBLISH chat:room '{"sender":"alice","text":"hello"}'
# Возвращает количество получателей
# Если 0 subscribers — сообщение потеряно навсегда
```

### SUBSCRIBE — подписаться на канал

```redis
SUBSCRIBE chat:room notifications:user:42
# Блокирующий режим — connection посвящён только получению
# Нельзя одновременно SUBSCRIBE и PUBLISH на одном соединении
```

### PSUBSCRIBE — подписка по glob-паттерну

```redis
PSUBSCRIBE notifications:user:*   # все уведомления всех пользователей
PSUBSCRIBE chat:*                  # все chat-каналы
PSUBSCRIBE orders.[a-z]*          # каналы orders.* начиная с буквы
```

```
Паттерны:
  *    — любая строка (включая `:`)
  ?    — один символ
  [ae] — один символ из набора
```

### Отписка

```redis
UNSUBSCRIBE chat:room
PUNSUBSCRIBE notifications:user:*
```

---

## At-most-once: отличие от Redis Streams

```
Redis Pub/Sub:
  Publisher → Redis → [Subscriber A]  ← получил
                    → [Subscriber B]  ← не подключён → ПОТЕРЯНО

Redis Streams:
  Producer → Redis Stream → [Consumer Group]
                          → сообщение хранится до XACK + MAXLEN
                          → Subscriber B подключился позже → прочитает
```

**Pub/Sub гарантирует**: если subscriber подключён в момент PUBLISH — доставка произойдёт.

**Pub/Sub НЕ гарантирует**: доставку если subscriber временно оффлайн, retry, порядок при высокой нагрузке.

---

## Отличие от Redis Streams

| | Redis Pub/Sub | Redis Streams |
|---|---|---|
| Persistence | ❌ ephemeral | ✅ |
| Delivery | At-most-once | At-least-once (с XACK) |
| Consumer groups | ❌ | ✅ |
| Replay | ❌ | ✅ |
| Offline consumer | ❌ потеря | ✅ прочитает при подключении |
| Overhead | Минимальный | Выше (persistence, PEL) |
| Broadcast | ✅ все получают | Per-group: один consumer |

---

## Go код из lrn-streams

### Publisher

```go
package redispubsub

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/redis/go-redis/v9"
)

const channel = "events:channel"

type Publisher struct {
    rdb *redis.Client
}

func NewPublisher(rdb *redis.Client) *Publisher {
    return &Publisher{rdb: rdb}
}

func (p *Publisher) Publish(ctx context.Context, event any) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    return p.rdb.Publish(ctx, channel, data).Err()
}
```

### Subscriber

```go
type Subscriber struct {
    pubsub *redis.PubSub
    ch     chan []byte
    cancel context.CancelFunc
}

func NewSubscriber(rdb *redis.Client, channels ...string) *Subscriber {
    ctx, cancel := context.WithCancel(context.Background())
    pubsub := rdb.Subscribe(ctx, channels...)
    
    s := &Subscriber{
        pubsub: pubsub,
        ch:     make(chan []byte, 64),
        cancel: cancel,
    }
    go s.recvLoop(ctx)
    return s
}

func (s *Subscriber) recvLoop(ctx context.Context) {
    defer close(s.ch)
    ch := s.pubsub.Channel() // <-chan *redis.Message
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-ch:
            if !ok {
                return
            }
            s.ch <- []byte(msg.Payload)
        }
    }
}

func (s *Subscriber) Messages() <-chan []byte {
    return s.ch
}

func (s *Subscriber) Close() error {
    s.cancel()
    return s.pubsub.Close()
}

// Использование
func main() {
    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    
    sub := NewSubscriber(rdb, "events:channel", "alerts:channel")
    defer sub.Close()
    
    for msg := range sub.Messages() {
        var event Event
        json.Unmarshal(msg, &event)
        fmt.Println(event)
    }
}
```

### Subscriber с паттерном

```go
func NewPatternSubscriber(rdb *redis.Client, patterns ...string) *Subscriber {
    ctx, cancel := context.WithCancel(context.Background())
    pubsub := rdb.PSubscribe(ctx, patterns...) // Pattern subscribe
    
    s := &Subscriber{pubsub: pubsub, ch: make(chan []byte, 64), cancel: cancel}
    go s.recvLoop(ctx)
    return s
}
```

---

## Use cases: когда Redis Pub/Sub подходит

### Backplane между инстансами сервиса

```
Instance A ──PUBLISH──► Redis ──message──► Instance B
                                 └────────► Instance C

Пример: WebSocket server — broadcast сообщения всем клиентам
        на всех инстансах (горизонтальное масштабирование)
```

```go
// Hub: при получении WebSocket сообщения
func (h *Hub) broadcastViaRedis(ctx context.Context, msg []byte) {
    h.pub.Publish(ctx, "ws:broadcast", msg)
}

// Каждый инстанс подписан на ws:broadcast → forward всем local clients
```

### Cache invalidation

```go
// Когда запись в DB изменилась — инвалидируем кэш на всех инстансах
func (s *Service) UpdateUser(ctx context.Context, user *User) error {
    if err := s.repo.Update(ctx, user); err != nil {
        return err
    }
    // Инвалидируем на всех инстансах
    s.pub.Publish(ctx, "cache:invalidate", user.ID)
    return nil
}

// Каждый инстанс слушает:
go func() {
    for msg := range sub.Messages() {
        userID := string(msg)
        cache.Delete(userID)
    }
}()
```

### Presence (online/offline статус)

```go
// Уведомить всех о подключении пользователя
pub.Publish(ctx, "presence", fmt.Sprintf(`{"user":"%s","status":"online"}`, userID))

// At-most-once OK: если сообщение о "online" потерялось —
// клиент обновит статус при следующем heartbeat
```

---

## Когда НЕ использовать Redis Pub/Sub

❌ **Надёжная доставка** — используй Redis Streams или RabbitMQ

❌ **Offline consumers** — используй Redis Streams (XREADGROUP с >)

❌ **Audit trail / история** — используй Redis Streams или Kafka

❌ **High-volume processing** — для тысяч msg/sec и complex routing — Kafka или RabbitMQ

**Redis Pub/Sub = real-time, ephemeral, fire-and-forget**

---

## Interview-ready answer

**Q: Когда Redis Pub/Sub, когда Redis Streams?**

Pub/Sub — когда нужен простой broadcast без гарантий доставки: real-time уведомления, cache invalidation между инстансами, presence. Если subscriber оффлайн — потеря допустима. Streams — когда нужна at-least-once гарантия, offline consumers, consumer groups для распределённой обработки, replay.

**Q: Как масштабировать WebSocket сервер горизонтально?**

Каждый WebSocket инстанс подписывается на Redis Pub/Sub канал. При получении сообщения от клиента — публикуем в Redis. Redis доставляет всем инстансам → каждый инстанс форвардит своим local WebSocket клиентам. Это "pub/sub backplane" паттерн. Для надёжности (если инстанс недоступен при публикации) — Redis Streams с consumer group.
