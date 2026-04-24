# Webhooks

Webhook — HTTP callback: сервер сам приходит к тебе когда что-то произошло. В отличие от polling — не ты опрашиваешь, а тебя уведомляют.

---

## Механика: POST на URL потребителя

```
Event source          Your service
    │                      │
    │  POST /webhooks/github
    │  Content-Type: application/json
    │  X-Hub-Signature-256: sha256=abc...
    │  {
    │    "action": "push",
    │    "repository": {...},
    │    "commits": [...]
    │  }
    ├─────────────────────►│
    │                       │ process event
    │         200 OK        │ (быстро! < 5 секунд)
    │◄──────────────────────│
```

**Правило**: обработчик webhook должен ответить **как можно быстрее** (2–5 секунд). Тяжёлую работу — в background queue.

---

## Delivery guarantees: at-least-once

Большинство webhook провайдеров (GitHub, Stripe, Twilio):
- Ожидают `2xx` ответ
- При ошибке (`4xx`, `5xx`, timeout) — **повторяют** с exponential backoff
- Повторяют несколько часов/дней

Следствие: **твой handler может получить одно событие несколько раз**.

### Idempotency key

```go
// Stripe, GitHub, etc. присылают уникальный event ID в headers
// Stripe: Stripe-Signature содержит timestamp + event ID
// GitHub: X-GitHub-Delivery — уникальный UUID события

func handleWebhook(w http.ResponseWriter, r *http.Request) {
    eventID := r.Header.Get("X-GitHub-Delivery")
    if eventID == "" {
        http.Error(w, "missing delivery id", http.StatusBadRequest)
        return
    }
    
    // Проверяем — обрабатывали уже?
    if processed, _ := store.IsProcessed(eventID); processed {
        w.WriteHeader(http.StatusOK) // OK, но не обрабатываем снова
        return
    }
    
    // Обработка
    if err := processEvent(r.Context(), r.Body); err != nil {
        http.Error(w, "processing failed", http.StatusInternalServerError)
        return
    }
    
    // Запоминаем как обработанный (с TTL например 7 дней)
    store.MarkProcessed(eventID, 7*24*time.Hour)
    w.WriteHeader(http.StatusOK)
}
```

---

## Security: HMAC-SHA256 signature verification

Webhook провайдеры подписывают payload чтобы ты мог убедиться что запрос действительно от них.

### Механизм (GitHub/Stripe подход)

```
Provider → подписывает payload секретным ключом:
  signature = HMAC-SHA256(secretKey, rawBody)

Отправляет в header:
  X-Hub-Signature-256: sha256=<hex(signature)>

Получатель:
  1. Читает rawBody
  2. Вычисляет HMAC-SHA256(secretKey, rawBody)
  3. Сравнивает с header (constant-time!)
  4. Отклоняет если не совпадает
```

### Реализация на Go

```go
const webhookSecret = "my-webhook-secret" // из env

func verifyGitHubSignature(r *http.Request) ([]byte, error) {
    signature := r.Header.Get("X-Hub-Signature-256")
    if signature == "" {
        return nil, errors.New("missing signature")
    }
    
    // Читаем body (НЕЛЬЗЯ читать дважды — сохраняем)
    body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
    if err != nil {
        return nil, fmt.Errorf("read body: %w", err)
    }
    
    // Вычисляем ожидаемую подпись
    mac := hmac.New(sha256.New, []byte(webhookSecret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    
    // Constant-time сравнение — защита от timing attack
    if !hmac.Equal([]byte(signature), []byte(expected)) {
        return nil, errors.New("invalid signature")
    }
    
    return body, nil
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
    body, err := verifyGitHubSignature(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusUnauthorized)
        return
    }
    
    // body проверен — можно парсить
    var event GitHubPushEvent
    if err := json.Unmarshal(body, &event); err != nil {
        http.Error(w, "invalid json", http.StatusBadRequest)
        return
    }
    
    // Быстрый ответ + async обработка
    go processEventAsync(event)
    w.WriteHeader(http.StatusOK)
}
```

### Почему `hmac.Equal`, а не `==`

Обычное сравнение строк завершается при первом несовпадении байта — по времени выполнения атакующий может определить насколько близка его подделанная подпись (timing attack). `hmac.Equal` / `subtle.ConstantTimeCompare` всегда сравнивает все байты за одинаковое время.

```go
// Плохо — timing attack уязвимость
if signature == expected { ... }

// Хорошо — constant-time
if !hmac.Equal([]byte(signature), []byte(expected)) { ... }
```

---

## Retry стратегия и exponential backoff

### Что делают провайдеры

GitHub пример:
```
Attempt 1: immediately
Attempt 2: 5 seconds
Attempt 3: 25 seconds
Attempt 4: 2 minutes
Attempt 5: 10 minutes
...продолжает до 72 часов
```

### Твоя сторона: принять быстро, обработать async

```go
// Anti-pattern: тяжёлая обработка в handler
func webhookHandler(w http.ResponseWriter, r *http.Request) {
    body, _ := verifySignature(r)
    
    // Долгая обработка: DB, внешние API → ТАЙМАУТ → провайдер решит что упало
    sendEmail(body)          // 2+ сек
    updateDatabase(body)     // 1+ сек
    callExternalAPI(body)    // 3+ сек
    w.WriteHeader(200)       // слишком поздно
}

// Правильно: быстрый ack + queue
func webhookHandler(w http.ResponseWriter, r *http.Request) {
    body, err := verifySignature(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    
    // Сохраняем в очередь и немедленно отвечаем
    if err := queue.Enqueue(body); err != nil {
        http.Error(w, "queue error", http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK) // ← ответили за <100ms
}

// Background worker читает из очереди и обрабатывает
```

---

## Отличие от polling и WebSocket

| | Polling | Webhook | WebSocket |
|---|---|---|---|
| Инициатор | Клиент | Сервер | Оба |
| Real-time | ❌ задержка = interval | ✅ при событии | ✅ немедленно |
| Нагрузка на client | Высокая (непрерывные запросы) | Нет | Idle соединение |
| Нагрузка на server | Высокая (обрабатывать все polls) | Только при событиях | Keep-alive |
| Persistence | Клиент должен быть онлайн | ❌ (но есть retry) | ❌ |
| Use case | Простота, legacy | External integrations | Real-time chat/gaming |

---

## Outbox паттерн для надёжной отправки webhooks

При отправке webhooks от своего сервиса — гарантированная доставка:

```go
// 1. В той же транзакции что и основное действие — записать в outbox
BEGIN;
  INSERT INTO orders (...) VALUES (...);
  INSERT INTO webhook_outbox (event_type, payload, status, created_at)
    VALUES ('order.created', $1, 'pending', NOW());
COMMIT;

// 2. Background worker читает pending и отправляет
// 3. При успехе (2xx) → status = 'delivered'
// 4. При ошибке → status = 'failed', retry_count++, next_retry_at = NOW() + backoff
// 5. После max_retries → status = 'dead', алерт

type WebhookOutbox struct {
    ID          uuid.UUID
    EventType   string
    Payload     json.RawMessage
    Status      string // pending, delivering, delivered, failed, dead
    RetryCount  int
    NextRetryAt time.Time
    CreatedAt   time.Time
}
```

---

## Interview-ready answer

**Q: Как защитить webhook endpoint?**

Три уровня:
1. **HMAC подпись**: провайдер подписывает payload своим секретом → ты верифицируешь HMAC-SHA256, используя `hmac.Equal` (constant-time) — защита от timing attack.
2. **Idempotency key**: проверять уникальный event ID из header → если уже обрабатывали — возвращать 200 без повторной обработки.
3. **Быстрый ответ**: ответить 200 за < 5 секунд, тяжёлую работу в background queue — иначе провайдер решит что упало и пришлёт повторно.

**Q: Почему webhook + at-least-once delivery?**

Провайдер не может знать обработал ли ты сообщение, если ты не ответил 2xx. При network timeout, restart сервера, 5xx — провайдер повторит. Это intentional: лучше доставить дважды, чем потерять. Твоя ответственность — сделать handler идемпотентным через проверку event ID.
