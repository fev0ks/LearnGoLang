# WebSocket

WebSocket — протокол поверх TCP для двусторонней real-time связи. После HTTP Upgrade-рукопожатия — full-duplex соединение без overhead request/response.

---

## Handshake: HTTP Upgrade

```
Client → Server:
  GET /ws HTTP/1.1
  Host: example.com
  Upgrade: websocket
  Connection: Upgrade
  Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
  Sec-WebSocket-Version: 13

Server → Client:
  HTTP/1.1 101 Switching Protocols
  Upgrade: websocket
  Connection: Upgrade
  Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

После `101 Switching Protocols` HTTP-соединение превращается в WebSocket. Нет больше запросов — только frames в обоих направлениях.

### Framing — структура WebSocket фрейма

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |            (16/64)            |
|N|V|V|V|       |S|             |   (if payload len==126/127)   |
| |1|2|3|       |K|             |                               |
+-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - -+
```

### Opcodes — типы фреймов

| Opcode | Тип | Описание |
|---|---|---|
| `0x0` | Continuation | Продолжение fragmented сообщения |
| `0x1` | Text | UTF-8 текст |
| `0x2` | Binary | Бинарные данные |
| `0x8` | Close | Закрытие соединения |
| `0x9` | Ping | Keepalive ping |
| `0xA` | Pong | Ответ на ping |

**Ping/Pong** — keepalive механизм. Сервер должен отвечать Pong на каждый Ping. Клиент может тоже слать Ping.

---

## Go: gorilla/websocket vs nhooyr.io/websocket

| | gorilla/websocket | nhooyr.io/websocket |
|---|---|---|
| Популярность | ⭐⭐⭐ де-факто стандарт | ⭐⭐ растёт |
| Поддержка | 🔴 архивирован (2023) | 🟢 активная |
| API | callback-based | context-based |
| Goroutine safety | ❌ не concurrent | ✅ concurrent writes/reads |
| Wasm поддержка | ❌ | ✅ |
| Compression | ✅ permessage-deflate | ✅ |
| Рекомендация | legacy проекты | новые проекты |

---

## Паттерн: read-goroutine + write-goroutine

WebSocket соединение не thread-safe — одновременная запись из нескольких горутин недопустима. Стандартный паттерн: одна горутина читает, одна пишет.

```go
type Client struct {
    conn    *websocket.Conn
    send    chan []byte
    hub     *Hub
    userID  string
}

// Pump: две горутины на клиента
func (c *Client) pump() {
    go c.writePump()
    c.readPump() // в вызывающей горутине
}

func (c *Client) readPump() {
    defer func() {
        c.hub.unregister <- c
        c.conn.Close()
    }()
    
    c.conn.SetReadLimit(512 * 1024) // 512 KB max message
    c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
        return nil
    })
    
    for {
        _, message, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err,
                websocket.CloseGoingAway,
                websocket.CloseAbnormalClosure) {
                log.Printf("ws read error: %v", err)
            }
            break
        }
        c.hub.broadcast <- &Message{data: message, from: c.userID}
    }
}

func (c *Client) writePump() {
    ticker := time.NewTicker(54 * time.Second) // ping interval
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()
    
    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if !ok {
                // Hub закрыл канал
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }
        case <-ticker.C:
            c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}
```

---

## Hub — центральный registry клиентов

```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan *Message
    register   chan *Client
    unregister chan *Client
}

func NewHub() *Hub {
    return &Hub{
        broadcast:  make(chan *Message, 256),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        clients:    make(map[*Client]bool),
    }
}

// Run: единственная горутина управляет map клиентов
// Нет mutex — все изменения через каналы (channel ownership)
func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.clients[client] = true
            
        case client := <-h.unregister:
            if _, ok := h.clients[client]; ok {
                delete(h.clients, client)
                close(client.send)
            }
            
        case message := <-h.broadcast:
            for client := range h.clients {
                if client.userID == message.from {
                    continue // не отправлять себе
                }
                select {
                case client.send <- message.data:
                default:
                    // Буфер полный — клиент медленный → disconnect
                    delete(h.clients, client)
                    close(client.send)
                }
            }
        }
    }
}
```

### HTTP handler для WebSocket upgrade

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        // Проверяй Origin для защиты от CSRF
        origin := r.Header.Get("Origin")
        return origin == "https://myapp.com"
    },
}

func wsHandler(hub *Hub, w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("ws upgrade error: %v", err)
        return
    }
    
    client := &Client{
        conn:   conn,
        send:   make(chan []byte, 256),
        hub:    hub,
        userID: getUserID(r),
    }
    hub.register <- client
    go client.pump()
}
```

---

## Scaling: sticky sessions vs pub/sub backplane

### Проблема горизонтального масштабирования

```
Instance A: [client1, client3, client5]
Instance B: [client2, client4, client6]

Client1 (на Instance A) отправляет сообщение client4 (на Instance B)
→ Instance A не знает о client4 → сообщение потеряно
```

### Sticky sessions (session affinity)

Load balancer направляет одного клиента всегда на один инстанс (по cookie или IP).

**Минусы:**
- Неравномерная нагрузка при отключении клиентов
- При падении инстанса — все его клиенты теряют соединение
- Сложно масштабировать

### Pub/Sub backplane (предпочтительно)

```
Instance A: [client1, client3]    ←─── Redis Pub/Sub ───→    Instance B: [client2, client4]
     │                                  "ws:room:42"                            │
     └── PUBLISH "msg from client1" ──────────────────────── ─► broadcast local
```

```go
// При получении WebSocket сообщения
func (h *Hub) handleMessage(msg *Message) {
    // Broadcast локально
    h.broadcastLocal(msg)
    
    // Publish в Redis для других инстансов
    data, _ := json.Marshal(msg)
    h.rdb.Publish(ctx, "ws:broadcast:"+msg.RoomID, data)
}

// Подписка на Redis (при старте инстанса)
func (h *Hub) subscribeRedis() {
    pubsub := h.rdb.Subscribe(ctx, "ws:broadcast:*")
    go func() {
        for msg := range pubsub.Channel() {
            var wsMsg Message
            json.Unmarshal([]byte(msg.Payload), &wsMsg)
            h.broadcastLocal(&wsMsg) // broadcast только локальным клиентам
        }
    }()
}
```

---

## WebSocket vs SSE vs Long Polling

| | WebSocket | SSE | Long Polling |
|---|---|---|---|
| Направление | Full-duplex | Сервер → клиент | Сервер → клиент |
| Protocol | Отдельный | HTTP | HTTP |
| Reconnect | Вручную | Автоматически | Вручную |
| Browser support | ✅ | ✅ | ✅ |
| Firewall/Proxy | ❌ проблемы | ✅ | ✅ |
| Use case | Chat, gaming, collab | Feeds, notifications | Fallback |

---

## Interview-ready answer

**Q: Как масштабировать WebSocket сервер?**

Два подхода: sticky sessions (один клиент всегда на один инстанс) — проще, но неравномерная нагрузка и сложный failover. Pub/Sub backplane (Redis) — каждый инстанс публикует сообщения в Redis, остальные инстансы подписаны и форвардят своим локальным клиентам. Второй подход лучше: инстансы stateless, любой может упасть без потери других клиентов.

**Q: Почему отдельные горутины для read и write?**

WebSocket соединение не позволяет конкурентную запись. Если несколько горутин пишут одновременно — frames перемешаются и протокол сломается. Read и write goroutine — устоявшийся паттерн: write goroutine — единственный writer, читает из buffered channel; read goroutine — единственный reader. Hub управляет всеми клиентами из одной горутины через каналы — без mutex на map клиентов.
