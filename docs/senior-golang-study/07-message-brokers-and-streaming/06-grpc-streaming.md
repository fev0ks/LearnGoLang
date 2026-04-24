# gRPC Bidirectional Streaming как Message Transport

gRPC streaming — альтернатива message broker для real-time связи сервис-сервис. Постоянное HTTP/2 соединение вместо poll/push очереди.

---

## Когда gRPC streaming как замена message broker

```
Message broker модель:
  Producer → [Kafka/RabbitMQ] → Consumer
  + Persistence, replay, fan-out к независимым consumers
  - Дополнительная инфраструктура, latency (batching)

gRPC Streaming модель:
  Client ←────────────────────────→ Server (bidi stream)
  + Нет дополнительной инфраструктуры, низкая latency, типобезопасно
  - Нет persistence, нет replay, нет fan-out из коробки
```

**Используй gRPC streaming когда:**
- Real-time двусторонняя связь (чат, collaboration, live updates)
- Нет нужды в persistence / replay
- Уже используешь gRPC для других сервисов
- Нужна типобезопасность через Protobuf

---

## 4 типа gRPC (напоминание)

```
Unary:             Client ──req──► Server ──resp──► Client
Server streaming:  Client ──req──► Server ──resp1,2,3...──► Client
Client streaming:  Client ──req1,2,3...──► Server ──resp──► Client
Bidirectional:     Client ◄──────────────────────────────► Server
```

Bidirectional streaming — полный дуплекс: обе стороны отправляют и получают независимо.

---

## Protobuf: определение сервиса

```protobuf
// proto/chat/chat.proto
syntax = "proto3";
package chat;

option go_package = "gen/chat;chat";

message ChatMessage {
    string sender  = 1;
    string text    = 2;
    int64  sent_at = 3;  // unix timestamp
}

service ChatService {
    // Bidirectional stream: gateway ↔ broker
    rpc Chat(stream ChatMessage) returns (stream ChatMessage);
}
```

---

## Архитектура: single broker

```
┌──────────┐        gRPC bidi stream        ┌──────────┐        gRPC bidi stream        ┌──────────┐
│  Alice   │◄──────────────────────────────►│  Broker  │◄──────────────────────────────►│   Bob    │
│ Gateway  │   Chat(stream ↔ stream)        │  :50051  │   Chat(stream ↔ stream)        │ Gateway  │
│  :8081   │                                │          │                                │  :8082   │
└──────────┘                                └──────────┘                                └──────────┘
```

Broker — gRPC сервер. Каждый gateway открывает один постоянный bidi поток. Broker при получении сообщения от одного gateway — рассылает всем остальным.

---

## Сервер (Broker): fan-out через registry

```go
// Из lrn-streams/internal/transport/grpcstream/server.go

type Server struct {
    pb.UnimplementedChatServiceServer
    
    mu      sync.Mutex
    // registry: все активные стримы
    clients map[grpc.BidiStreamingServer[pb.ChatMessage, pb.ChatMessage]]struct{}
    
    brokerID string
    rdb      *redis.Client // для Redis backplane (опционально)
}

// Chat — вызывается для каждого нового подключения
func (s *Server) Chat(stream grpc.BidiStreamingServer[pb.ChatMessage, pb.ChatMessage]) error {
    // Регистрируем стрим
    s.mu.Lock()
    s.clients[stream] = struct{}{}
    s.mu.Unlock()
    
    // Дерегистрация при disconnect
    defer func() {
        s.mu.Lock()
        delete(s.clients, stream)
        s.mu.Unlock()
    }()
    
    // Читаем входящие сообщения
    for {
        msg, err := stream.Recv()
        if err != nil {
            return err // соединение закрыто или ошибка
        }
        
        // Fan-out: всем кроме отправителя
        s.broadcastExcept(stream, msg)
        
        // Если multi-broker: публикуем в Redis backplane
        s.publishToBackplane(msg)
    }
}

func (s *Server) broadcastExcept(
    sender grpc.BidiStreamingServer[pb.ChatMessage, pb.ChatMessage],
    msg *pb.ChatMessage,
) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    for client := range s.clients {
        if client == sender {
            continue
        }
        if err := client.Send(msg); err != nil {
            // Логируем, но продолжаем — один медленный клиент не блокирует остальных
            log.Printf("[grpc] send error: %v", err)
        }
    }
}

func (s *Server) Start(addr string) error {
    lis, err := net.Listen("tcp", addr)
    if err != nil {
        return err
    }
    gs := grpc.NewServer()
    pb.RegisterChatServiceServer(gs, s)
    return gs.Serve(lis)
}
```

---

## Клиент (Gateway): отправка и получение

```go
// Из lrn-streams/internal/transport/grpcstream/client.go

type ClientTransport struct {
    conn   *grpc.ClientConn
    stream grpc.BidiStreamingClient[pb.ChatMessage, pb.ChatMessage]
    ch     chan model.ChatMessage
    cancel context.CancelFunc
}

func NewClientTransport(brokerAddr string) (*ClientTransport, error) {
    conn, err := grpc.NewClient(brokerAddr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        return nil, err
    }
    
    client := pb.NewChatServiceClient(conn)
    ctx, cancel := context.WithCancel(context.Background())
    
    // Открываем bidirectional stream — долгоживущее соединение
    stream, err := client.Chat(ctx)
    if err != nil {
        cancel(); conn.Close()
        return nil, err
    }
    
    ct := &ClientTransport{
        conn: conn, stream: stream,
        ch: make(chan model.ChatMessage, 64),
        cancel: cancel,
    }
    go ct.recvLoop() // горутина для входящих сообщений
    return ct, nil
}

// recvLoop — читает входящие сообщения в фоне
func (c *ClientTransport) recvLoop() {
    defer close(c.ch)
    for {
        msg, err := c.stream.Recv()
        if err != nil {
            log.Printf("[grpc] recv error: %v", err)
            return // соединение закрыто
        }
        c.ch <- fromProto(msg)
    }
}

// Send — отправить сообщение на broker
func (c *ClientTransport) Send(ctx context.Context, msg model.ChatMessage) error {
    return c.stream.Send(toProto(msg))
}

func (c *ClientTransport) Messages() <-chan model.ChatMessage { return c.ch }
func (c *ClientTransport) Close() error { c.cancel(); return c.conn.Close() }
```

---

## Multi-broker: Redis backplane

При горизонтальном масштабировании — несколько инстансов broker. Клиенты на разных инстансах не видят друг друга без backplane.

```
                    Redis Pub/Sub ("grpc:backplane")
                 ┌──────────────────────────────────────┐
                 │                                      │
                 ▼                                      ▼
┌──────────┐  ┌─────────┐  PUBLISH/SUBSCRIBE  ┌─────────┐  ┌──────────┐
│  Alice   │◄►│ BrokerA │ ──────────────────► │ BrokerB │◄►│   Bob    │
│ Gateway  │  │ :50051  │                     │ :50052  │  │ Gateway  │
└──────────┘  └─────────┘                     └─────────┘  └──────────┘
```

```go
// При получении сообщения от клиента:
// 1. Broadcast локальным clients
// 2. Publish в Redis backplane (с broker_id чтобы не echo)

func (s *Server) publishToBackplane(msg *pb.ChatMessage) {
    if s.rdb == nil { return }
    
    bm := backplaneMessage{
        BrokerID: s.brokerID, // уникальный ID этого брокера
        Sender:   msg.Sender,
        Text:     msg.Text,
        SentAt:   msg.SentAt,
    }
    data, _ := json.Marshal(bm)
    s.rdb.Publish(context.Background(), "grpc:backplane", data)
}

// subscribeBackplane — слушает сообщения от других брокеров
func (s *Server) subscribeBackplane() {
    pubsub := s.rdb.Subscribe(context.Background(), "grpc:backplane")
    defer pubsub.Close()
    
    for redisMsg := range pubsub.Channel() {
        var bm backplaneMessage
        json.Unmarshal([]byte(redisMsg.Payload), &bm)
        
        // Пропускаем собственные сообщения (защита от echo loop)
        if bm.BrokerID == s.brokerID { continue }
        
        // Relay всем локальным клиентам
        s.broadcastAll(&pb.ChatMessage{
            Sender: bm.Sender,
            Text:   bm.Text,
            SentAt: bm.SentAt,
        })
    }
}
```

---

## Backpressure в gRPC streams

HTTP/2 имеет встроенный flow control (window-based). Если получатель не успевает читать — отправитель замедляется.

```go
// Сервер: медленный клиент не должен блокировать broadcast
func (s *Server) broadcastExcept(sender stream, msg *pb.ChatMessage) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    for client := range s.clients {
        if client == sender { continue }
        
        // stream.Send блокируется если HTTP/2 window заполнено
        // Для broadcast: используй таймаут или non-blocking send
        if err := client.Send(msg); err != nil {
            log.Printf("send error (dropping client): %v", err)
            // В production: пометить клиента на удаление, не делать delete под mutex
        }
    }
}

// Лучший подход: буферизованные каналы per-client + отдельные write goroutines
// Как в WebSocket Hub паттерне
```

---

## gRPC Streaming vs Message Broker

| | gRPC Bidi Stream | Kafka/RabbitMQ |
|---|---|---|
| Persistence | ❌ | ✅ |
| Replay | ❌ | ✅ (Kafka) |
| Fan-out к независимым consumers | ❌ (вручную) | ✅ |
| Latency | Очень низкая | Выше |
| Типобезопасность | ✅ Protobuf | ⚠️ схема отдельно |
| Дополнительная инфраструктура | ❌ | ✅ broker cluster |
| Horizontal scaling | Через backplane | Встроено |
| Use case | Real-time, service mesh | Event streaming, async processing |

---

## Interview-ready answer

**Q: Когда gRPC streaming вместо Kafka?**

gRPC streaming — для real-time двусторонней связи где persistence не нужна: чат, collaborative editing, live dashboard. Нет дополнительной инфраструктуры, очень низкая latency. Kafka — когда нужна надёжная доставка, replay, независимые consumer groups, долгосрочное хранение событий.

**Q: Как масштабировать gRPC streaming сервер горизонтально?**

Та же проблема что и WebSocket: клиенты на разных инстансах не видят друг друга. Решение — pub/sub backplane через Redis: каждый инстанс публикует сообщения в Redis и подписан на него. При получении из Redis — relay локальным clients. Broker ID в сообщении предотвращает echo-loop.
