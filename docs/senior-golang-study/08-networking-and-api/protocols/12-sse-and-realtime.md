# SSE и реалтайм-протоколы

"Реалтайм" в web означает: сервер может **самостоятельно** отправлять данные клиенту, не дожидаясь запроса. В обычном HTTP — клиент спрашивает, сервер отвечает. Для пуш-уведомлений, чатов, биржевых котировок, AI-streaming ответов — это не работает.

Есть три классических подхода: **long polling**, **Server-Sent Events (SSE)** и **WebSockets**. Каждый со своими плюсами, минусами и реальной нишей. Senior backend должен знать когда выбирать что — потому что выбор не "WebSocket для всего", это распространённое заблуждение.

См. также: [05-websocket.md](./05-websocket.md) — детальный разбор WebSocket.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Проблема: HTTP request-response](#проблема-http-request-response)
- [Long polling](#long-polling)
- [Server-Sent Events (SSE)](#server-sent-events-sse)
- [WebSocket — краткое сравнение](#websocket-краткое-сравнение)
- [Сравнительная таблица](#сравнительная-таблица)
- [SSE в Go: реализация](#sse-в-go-реализация)
- [Reconnect и Last-Event-ID](#reconnect-и-last-event-id)
- [SSE через прокси и балансировщики](#sse-через-прокси-и-балансировщики)
- [Когда что выбирать](#когда-что-выбирать)
- [SSE для streaming ответов LLM](#sse-для-streaming-ответов-llm)
- [Anti-patterns](#anti-patterns)

---

## Простая аналогия

Аналогия: ожидание почты от друга.

**Polling:** каждые 5 минут ходишь к почтовому ящику проверить. Утомительно, неэффективно. Большинство походов — пустые.

**Long polling:** ходишь к ящику, говоришь почтальону "если в течение часа что-то придёт — сразу беги ко мне". Стоишь и ждёшь. Когда что-то пришло — получаешь сразу. Через час, если ничего нет — заходишь снова, ждёшь ещё час.

**SSE:** от почтальона домой протянут телефонный провод. Почтальон звонит **сам**, когда есть новости, — остаётся слушать. Связь одна, **только от него**.

**WebSocket:** установлен домофон с двусторонней связью — говорить могут обе стороны. Полноценный диалог в обе стороны.

В реальном backend каждое решение имеет свою стоимость и применение. Главный вопрос — нужна ли двусторонняя связь.

---

## Проблема: HTTP request-response

Обычный HTTP — это **pull**:

```
Client                Server
  │                     │
  │  GET /messages      │
  │ ───────────────────→│
  │  200 OK + messages  │
  │ ←───────────────────│
  │                     │
  │  (нет новых данных) │
  │  GET /messages      │
  │ ───────────────────→│
  │  200 OK + []        │
  │ ←───────────────────│
  │                     │
```

Клиент должен **сам** спрашивать. Сервер не может "позвать" клиента. Проблемы:

**1. Latency.** Если опрашивать каждые 5 секунд — задержка получения нового сообщения до 5 секунд.

**2. Нагрузка.** Опрос 10000 клиентов раз в секунду = 10000 RPS, большая часть впустую (нет новых данных).

**3. Battery / network.** Для мобильных клиентов постоянный polling = разряд батареи и расход трафика.

**4. Не настоящий "real-time".** Полминутная задержка для чата — плохой UX.

Решения — три подхода ниже.

---

## Long polling

Старейший метод "реалтайма" в HTTP. Не нужны новые протоколы, работает везде где работает HTTP.

**Идея:** клиент делает запрос, **сервер не отвечает** пока нет новых данных или не истечёт длинный timeout (30-60 секунд).

```
Client                  Server
  │                       │
  │  GET /poll?since=42   │
  │ ─────────────────────→│
  │                       │  (ждёт новых данных)
  │                       │  (ждёт...)
  │                       │  (новое сообщение!)
  │  200 OK + message #43 │
  │ ←─────────────────────│
  │                       │
  │  GET /poll?since=43   │  (новый запрос сразу же)
  │ ─────────────────────→│
  │                       │  (ждёт следующего...)
```

Когда сервер отвечает — клиент **сразу** делает следующий запрос. Между запросами — постоянное "подвешенное" соединение.

### Реализация в Go

```go
func handleLongPoll(w http.ResponseWriter, r *http.Request) {
    sinceID, _ := strconv.Atoi(r.URL.Query().Get("since"))

    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    // Канал событий от пользователя (subscribe)
    eventsCh := subscribe(sinceID)
    defer unsubscribe(eventsCh)

    select {
    case msg := <-eventsCh:
        json.NewEncoder(w).Encode(msg)
    case <-ctx.Done():
        // Timeout — клиент сделает новый запрос
        w.WriteHeader(http.StatusNoContent)
    }
}
```

### Плюсы

- ✅ Работает везде: любой HTTP-клиент, любой прокси, любой firewall
- ✅ Просто реализовать на бэкенде
- ✅ Хорошо работает с обычной HTTP-инфраструктурой (load balancer, CDN)

### Минусы

- ❌ Overhead каждого запроса (TCP handshake если не keep-alive, HTTP headers)
- ❌ Каждое сообщение — отдельный round-trip
- ❌ Сервер должен держать **открытое соединение** для каждого клиента (как и SSE/WS)
- ❌ Не подходит для high-frequency обновлений (биржа, игры)

### Когда использовать

- Legacy системы без поддержки SSE/WS
- Низкочастотные обновления (минуты, не секунды)
- Когда инфраструктура (firewall, прокси) не пропускает WebSocket

В 2026 году используется редко — почти везде заменён SSE или WebSocket.

---

## Server-Sent Events (SSE)

**Идея:** клиент делает один HTTP-запрос, сервер держит его **открытым** и **постоянно пишет** в response stream — поток текстовых событий.

```
Client                  Server
  │                       │
  │  GET /events          │
  │  Accept: text/event-  │
  │  stream               │
  │ ─────────────────────→│
  │                       │
  │  data: message 1      │  (сервер пишет, соединение открыто)
  │ ←─────────────────────│
  │                       │
  │  data: message 2      │  (продолжает писать)
  │ ←─────────────────────│
  │                       │
  │  data: message 3      │
  │ ←─────────────────────│
  │  ... бесконечно ...   │
```

Это **обычное HTTP-соединение**, но сервер никогда не "завершает" ответ — продолжает писать пока клиент не отключится или сервер не закроет.

### Формат данных

Текстовый, очень простой:

```
data: {"id": 1, "text": "Hello"}

data: {"id": 2, "text": "World"}

event: message
data: {"text": "Specific event type"}

id: 42
data: {"text": "With event ID"}

retry: 5000
data: {"text": "Suggest reconnect delay"}
```

Каждое событие — несколько строк `field: value\n`, разделённых пустой строкой `\n\n`.

Поля:
- **`data:`** — собственно данные (может быть несколько строк)
- **`event:`** — тип события (для разной обработки на клиенте)
- **`id:`** — ID события (для reconnect, см. ниже)
- **`retry:`** — рекомендованный delay перед reconnect (мс)

### Native API в браузере

```javascript
const eventSource = new EventSource('/events');

eventSource.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log('Got message:', data);
};

// Кастомный event type
eventSource.addEventListener('user-joined', (event) => {
    console.log('User joined:', event.data);
});

eventSource.onerror = (err) => {
    console.error('SSE error:', err);
};
```

`EventSource` **автоматически реконнектится** при разрыве — это огромное преимущество.

### Плюсы

- ✅ **Стандартный HTTP** — работает через CDN, прокси, load balancer, firewall
- ✅ **Авто-reconnect** в браузерах из коробки
- ✅ **Last-Event-ID** для возобновления потока с правильного места
- ✅ **Простая реализация** на сервере (просто пиши в ResponseWriter)
- ✅ Хорошо подходит для **multiplex** (тысячи клиентов на одном сервере)
- ✅ Streaming работает с любой HTTP-инфраструктурой (Cloudflare, AWS ELB, и т.д.)
- ✅ Поддерживает HTTP/2 multiplexing (несколько SSE через одно TCP-соединение)

### Минусы

- ❌ **Только server → client** — клиент не может слать данные через тот же канал (нужен отдельный POST)
- ❌ **Только текст** — нет нативной поддержки бинарных данных (можно base64, но overhead)
- ❌ В HTTP/1.1 один SSE = одно TCP-соединение, браузер лимитирует 6 соединений на домен. HTTP/2 решает (multiplexing).
- ❌ Не работает в IE (но IE мёртв)

### Когда использовать

- **Server push без обратной связи** — уведомления, обновления, биржевые курсы
- **Streaming ответов LLM** (см. ниже)
- **Прогресс долгих операций** — server постит updates "5%, 20%, 50%, готово"
- **Live updates** на дашбордах
- **Event feed** социальной сети
- **Notifications** в админ-панели

Когда **не** надо: чат (двусторонняя связь), игры (низкая latency туда-обратно), бинарные данные.

---

## WebSocket — краткое сравнение

WebSocket — **двусторонний** протокол. После handshake (HTTP с Upgrade) соединение переходит на отдельный wire protocol.

```
Client                  Server
  │                       │
  │  GET /ws              │
  │  Upgrade: websocket   │
  │ ─────────────────────→│
  │  101 Switching        │
  │ ←─────────────────────│
  │                       │
  │  ↔  ↔  ↔  ↔  ↔  ↔    │  (двусторонний поток фреймов)
```

**Плюсы:**
- Двусторонний (client ↔ server)
- Бинарные данные нативно
- Низкий overhead каждого сообщения

**Минусы:**
- Не работает через простой HTTP-прокси (нужна поддержка Upgrade)
- **Нет auto-reconnect** — надо реализовать самому
- Балансировка трафика сложнее (sticky sessions)
- Stateful — каждое соединение требует state на сервере

Подробнее: [05-websocket.md](./05-websocket.md).

---

## Сравнительная таблица

| | Long polling | SSE | WebSocket |
|---|---|---|---|
| Транспорт | HTTP | HTTP | TCP (после HTTP upgrade) |
| Направление | Pull (с задержкой) | Server → Client | Bidirectional |
| Reconnect | Каждый запрос | Auto (browser) | Manual |
| Заголовки на сообщение | Большие | Минимум | Минимум |
| Бинарные данные | Через body | Только base64 | Нативно |
| Через CDN/proxy | Да | Да | Часто проблемы |
| HTTP/2 multiplex | Да | Да | Нет (только HTTP/3 ws) |
| Стандартный API в браузере | `fetch` | `EventSource` | `WebSocket` |
| Сложность сервера | Низкая | Низкая | Средняя |
| Сложность клиента | Низкая | Очень низкая | Средняя |
| Подходит для high-frequency | Нет | Да | Да |
| Двусторонний канал | Нет | Нет | Да |
| Last-Event-ID resume | Нет | Да | Только если реализовать |

---

## SSE в Go: реализация

Минимальный SSE-сервер:

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type Event struct {
    ID   int       `json:"id"`
    Time time.Time `json:"time"`
    Text string    `json:"text"`
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
    // Обязательные заголовки SSE
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no")  // отключить буферизацию в nginx

    // Flusher — обязателен для streaming
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }

    // Канал событий для этого клиента
    eventCh := make(chan Event, 16)
    clientID := registerClient(eventCh)
    defer unregisterClient(clientID)

    ticker := time.NewTicker(15 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-r.Context().Done():
            // Клиент отключился
            return

        case event := <-eventCh:
            data, _ := json.Marshal(event)
            fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, data)
            flusher.Flush()

        case <-ticker.C:
            // Heartbeat — комментарий (": something\n\n") не парсится клиентом,
            // но держит соединение живым через timeout'ы прокси
            fmt.Fprintf(w, ": heartbeat\n\n")
            flusher.Flush()
        }
    }
}

func main() {
    http.HandleFunc("/events", sseHandler)
    http.ListenAndServe(":8080", nil)
}
```

**Ключевые моменты:**

1. **Заголовки:** `Content-Type: text/event-stream`, `Cache-Control: no-cache`. `X-Accel-Buffering: no` отключает буферизацию в nginx (иначе события зависнут в буфере nginx до закрытия соединения).

2. **`http.Flusher`:** `w.Write()` буферизует данные в Go runtime. Без `Flusher.Flush()` клиент не получит событие до накопления значительного буфера. **Обязательно `Flush()` после каждого события.**

3. **Heartbeat:** прокси (nginx, AWS ELB) закрывают соединения после N секунд без активности. Каждые 15-30 секунд отправляй комментарий (`: ...\n\n`) для поддержания.

4. **`r.Context().Done()`:** если клиент отключился — context отменяется. Без этого goroutine утечёт.

### Бродкаст всем клиентам

```go
type Broker struct {
    clients map[int]chan Event
    mu      sync.RWMutex
    nextID  int
}

func (b *Broker) Subscribe() (int, chan Event) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.nextID++
    ch := make(chan Event, 16)
    b.clients[b.nextID] = ch
    return b.nextID, ch
}

func (b *Broker) Unsubscribe(id int) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if ch, ok := b.clients[id]; ok {
        close(ch)
        delete(b.clients, id)
    }
}

func (b *Broker) Publish(event Event) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for _, ch := range b.clients {
        select {
        case ch <- event:
        default:
            // Канал переполнен — пропускаем (или дисконнектим slow client)
        }
    }
}
```

**Важно — что делать со slow client:**
- Option A: пропустить событие (теряем данные)
- Option B: блокироваться (медленный клиент тормозит всех — плохо!)
- Option C: дисконнектить (брутально, но защищает систему)

Production-вариант — обычно C с метрикой "сколько раз дисконнектили из-за overflow".

---

## Reconnect и Last-Event-ID

SSE имеет встроенный механизм восстановления после разрыва.

**На клиенте:** `EventSource` автоматически переподключается. При reconnect отправляет заголовок:
```
Last-Event-ID: 42
```
(берётся из последнего полученного `id:` поля)

**На сервере:** читаем этот заголовок и отдаём события начиная с ID+1.

```go
func sseHandler(w http.ResponseWriter, r *http.Request) {
    // ... настройка заголовков ...

    lastEventID := r.Header.Get("Last-Event-ID")
    var startFrom int
    if lastEventID != "" {
        startFrom, _ = strconv.Atoi(lastEventID)
    }

    // Отправляем пропущенные события
    missedEvents := getEventsSince(startFrom)
    for _, event := range missedEvents {
        data, _ := json.Marshal(event)
        fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, data)
        flusher.Flush()
    }

    // Дальше — live events как обычно
    // ...
}
```

**Это требует storage для recent events** — обычно ring buffer в памяти или Redis Streams.

### retry hint

Сервер может предложить delay перед reconnect:

```
retry: 10000
data: ...
```

Клиент будет реконнектиться с задержкой 10 секунд. Полезно для graceful overload — "не возвращайтесь сразу, попробуйте через 30 секунд".

---

## SSE через прокси и балансировщики

Главная боль SSE в production — **прокси и буферизация**.

### nginx

По умолчанию nginx буферизует ответы. Это убийственно для SSE — события придут пачкой когда буфер заполнится или соединение закроется.

```nginx
location /events {
    proxy_pass http://backend;

    # Отключить буферизацию для SSE
    proxy_buffering off;
    proxy_cache off;

    # Большой timeout (или нет timeout вообще)
    proxy_read_timeout 24h;
    proxy_send_timeout 24h;

    # Передавать клиентские заголовки (для Last-Event-ID)
    proxy_set_header Last-Event-ID $http_last_event_id;

    # HTTP/1.1 для streaming
    proxy_http_version 1.1;
}
```

Сервер может выставить `X-Accel-Buffering: no` — nginx уважает этот заголовок и отключает буферизацию для конкретного ответа.

### AWS ALB / ELB

- ALB поддерживает SSE — настрой idle timeout (max 4000 секунд)
- ALB не поддерживает HTTP/2 server push, но SSE работает на HTTP/1.1 и HTTP/2

### Cloudflare

- SSE работает через Cloudflare, но есть лимиты на длительность соединения
- На Free plan — 100 секунд timeout, потом разрыв (клиент реконнектится)
- На платных — до часов

### HTTP/2 multiplexing

В HTTP/1.1 каждое SSE соединение = 1 TCP. Браузеры лимитируют 6 соединений на домен → max 6 SSE streams одновременно.

В HTTP/2 один TCP может мультиплексировать множество streams — лимит снимается. Поэтому HTTP/2 серьёзно улучшает SSE.

### Connection limits

Каждый SSE клиент держит соединение. Сервер должен выдерживать тысячи concurrent connections:

- **`ulimit -n`** — лимит файловых дескрипторов. Поднять до 65535+.
- **TCP keep-alive** — настроить, чтобы мёртвые соединения закрывались.
- **Memory per connection** — Go runtime: ~2KB goroutine stack + buffers. 100k клиентов = ~500MB только на goroutines.

Тут же — SSE масштабируется горизонтально через любой load balancer (в отличие от WebSocket, где sticky sessions усложняют).

---

## Когда что выбирать

### ✅ Long polling
- Legacy системы без поддержки модерн протоколов
- Когда firewall блокирует SSE/WS

### ✅ SSE (Server-Sent Events)
- **Server → client** push, без обратной связи
- Streaming ответов LLM ⭐ (см. ниже)
- Real-time дашборды, метрики
- Прогресс долгих операций
- Notifications в админке
- Event feeds (Twitter-like)
- Live результаты спортивных событий
- Биржевые котировки для viewers (не для активных трейдеров)

### ✅ WebSocket
- **Чаты** (двусторонняя связь критична)
- **Multiplayer игры** (низкая latency обоих направлений)
- **Collaborative editing** (Google Docs)
- **Voice/Video signaling** (для WebRTC)
- **Trading platforms** для активных трейдеров
- **IoT** — device telemetry с command channel

### Гибридные подходы

Иногда комбинируют. Пример:
- **SSE** для server push (real-time updates)
- **POST /api/...** для client → server (отправка сообщения)

Этого достаточно для большинства случаев, и проще чем full WebSocket.

---

## SSE для streaming ответов LLM

Современный killer-app для SSE — **streaming ответов от LLM**.

LLM генерирует токены **последовательно**. Если ждать весь ответ — user видит loading 5-30 секунд. Если стримить токены по мере генерации — user видит ответ "печатающимся" как в ChatGPT. UX совершенно другой.

OpenAI, Anthropic, и все основные LLM-провайдеры — используют SSE для streaming:

```
GET /v1/chat/completions
Accept: text/event-stream

← data: {"choices":[{"delta":{"content":"Hello"}}]}
← data: {"choices":[{"delta":{"content":" world"}}]}
← data: {"choices":[{"delta":{"content":"!"}}]}
← data: [DONE]
```

### Реализация в Go: проксирование LLM stream

```go
func handleChat(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("X-Accel-Buffering", "no")

    flusher := w.(http.Flusher)

    // Запрос к OpenAI с streaming
    resp, err := openaiClient.CreateChatCompletionStream(r.Context(), openai.ChatCompletionRequest{
        Model:    "gpt-4o-mini",
        Messages: messages,
        Stream:   true,
    })
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    defer resp.Close()

    for {
        chunk, err := resp.Recv()
        if errors.Is(err, io.EOF) {
            fmt.Fprintf(w, "data: [DONE]\n\n")
            flusher.Flush()
            return
        }
        if err != nil {
            fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
            flusher.Flush()
            return
        }

        content := chunk.Choices[0].Delta.Content
        if content == "" {
            continue
        }

        // Передаём токен клиенту
        data, _ := json.Marshal(map[string]string{"text": content})
        fmt.Fprintf(w, "data: %s\n\n", data)
        flusher.Flush()
    }
}
```

Клиент видит ответ по мере генерации — точно как в ChatGPT.

---

## Anti-patterns

**1. SSE для двусторонней связи.**
Если нужен chat — берёшь WebSocket. SSE + POST для каждого сообщения — работает, но громоздко. Если шлёшь много в обе стороны — WS чище.

**2. WebSocket для one-way push.**
Распространённое заблуждение: "real-time → WebSocket". Если данные идут только server → client — SSE проще и надёжнее (auto-reconnect, работает через любую инфраструктуру).

**3. Polling раз в секунду вместо SSE/WS.**
Создаёт огромную нагрузку на сервер и сеть. 10000 клиентов × 1 RPS = 10000 RPS впустую большую часть времени.

**4. Забыть `Flusher.Flush()`.**
Без него события буферизуются в Go runtime — клиент видит их пачкой или после закрытия соединения. Классическая ошибка новичков в SSE.

**5. Не настроить proxy buffering.**
Идеальный код в Go + nginx с buffering = события не доходят. `proxy_buffering off` или `X-Accel-Buffering: no`.

**6. Бесконечный SSE без heartbeat.**
Прокси и load balancers убивают idle connections через 30-120 секунд. Если нет heartbeat — клиент видит "оборванное" соединение. Шли `:heartbeat\n\n` каждые 15-30 секунд.

**7. Не обрабатывать reconnect.**
Если клиент реконнектится, а сервер не учитывает Last-Event-ID — клиент пропустит события из периода разрыва.

**8. Stateful SSE без replay.**
Если события важны (заказы, платежи) — недостаточно только live stream. Нужен ring buffer или Redis Streams для replay при reconnect.

**9. WS без auto-reconnect на клиенте.**
`new WebSocket('ws://...')` не реконнектится. Нужно явно. У SSE — встроено.

**10. Один сервер для всех SSE/WS клиентов.**
SSE масштабируется горизонтально просто (любой LB). WS сложнее (sticky sessions или shared state в Redis pub/sub). Pub/sub архитектура: backend публикует в Redis, все SSE/WS серверы подписаны и пробрасывают клиентам.

---

## Полезные ссылки

- [HTML5 Server-Sent Events spec](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- [MDN: Using Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)
- [OpenAI Streaming Guide](https://platform.openai.com/docs/api-reference/streaming) — официальный пример SSE
- [nginx Streaming Guide](https://www.nginx.com/blog/websocket-nginx/) — про SSE и WS в nginx
- [r3labs/sse](https://github.com/r3labs/sse) — Go библиотека SSE сервера/клиента (если не хочешь руками)
