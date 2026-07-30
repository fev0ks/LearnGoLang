# Webhooks

## Содержание

- [Механика: POST на URL потребителя](#механика-post-на-url-потребителя)
- [Delivery guarantees: at-least-once](#delivery-guarantees-at-least-once)
- [Security: HMAC-SHA256 signature verification](#security-hmac-sha256-signature-verification)
- [Retry стратегия и exponential backoff](#retry-стратегия-и-exponential-backoff)
- [Отличие от polling и WebSocket](#отличие-от-polling-и-websocket)
- [Outbox паттерн для надёжной отправки webhooks](#outbox-паттерн-для-надёжной-отправки-webhooks)
- [Interview-ready answer](#interview-ready-answer)

Webhook — HTTP callback: сервер сам присылает уведомление, когда что-то произошло. В отличие от polling опрашивает не потребитель — его уведомляют.

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

Следствие: handler может получить одно событие несколько раз.

### Idempotency key

<details>
<summary>Отсечение повторов по идентификатору события</summary>

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

</details>

---

## Security: HMAC-SHA256 signature verification

Webhook-провайдеры подписывают payload, чтобы получатель мог убедиться, что запрос действительно от них.

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

Проверка состоит из трёх шагов: прочитать тело целиком, посчитать код аутентификации по тому же алгоритму и сравнить с присланным. Метка времени в подписываемой строке нужна, чтобы перехваченный запрос нельзя было отправить повторно через час — слишком старые отвергают, не проверяя подпись дальше.

<details>
<summary>Проверка подписи целиком: чтение сырого тела и сравнение постоянным по времени</summary>

```go
const webhookSecret = "my-webhook-secret" // из env

func verifyGitHubSignature(r *http.Request) ([]byte, error) {
    signature := r.Header.Get("X-Hub-Signature-256")
    if signature == "" {
        return nil, errors.New("missing signature")
    }
    
    // Тело читается один раз, поэтому сохраняем его для проверки подписи
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

</details>

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

Отправитель не знает, что происходит на стороне получателя, поэтому единственный доступный ему сигнал — код ответа. Из этого вырастает вся схема повторов: успех подтверждается быстрым `2xx`, а всё остальное считается поводом попробовать снова.

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

### Сторона получателя: принять быстро, обработать асинхронно

Главное правило обработчика — вернуть `2xx` как можно раньше, а работу выполнить отдельно. Причина в том, как устроены отправители: у них жёсткий таймаут (обычно от 5 до 30 секунд), и медленный ответ засчитывается как неудача. Обработчик, который синхронно ходит в базу и в три соседних сервиса, получает повторную доставку того же события просто потому, что не успел ответить — и создаёт дубликаты своей же работой.

Правильная последовательность: проверить подпись, сохранить событие в свою очередь или таблицу, ответить `2xx`, обработать асинхронно. Тогда повтор от отправителя отсекается ключом идемпотентности на этапе сохранения, а не после выполнения побочных эффектов.

<details>
<summary>Обработчик: быстрый ответ и асинхронная обработка</summary>

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

</details>

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

<details>
<summary>Отправка через outbox: запись в транзакции и отдельный отправитель</summary>

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

</details>

---

## Interview-ready answer

**1. Как защитить эндпоинт, принимающий webhooks?**

- Подпись — отправитель считает код аутентификации по сырому телу и общему секрету, получатель пересчитывает и сравнивает; проверка идёт до разбора JSON, потому что подпись считается по байтам, а не по объекту.
- Сравнение — только постоянным по времени способом (`hmac.Equal`): обычное сравнение строк утекает информацию о том, сколько первых байт совпало.
- Метка времени в подписываемой строке — защита от повторной отправки перехваченного запроса: слишком старые отвергают, не проверяя подпись.
- Идемпотентность — уникальный идентификатор события из заголовка сохраняют и на повтор отвечают успехом без повторной обработки.
- Ограничения — предел размера тела и предел частоты запросов, потому что адрес эндпоинта публичен и отправить туда может кто угодно.

**2. Почему доставка at-least-once и что это значит для получателя?**

- Отправитель не знает, обработано ли событие, пока не получит успешный ответ; при таймауте, разрыве или ошибке он повторит.
- Это осознанный выбор: потерять событие хуже, чем доставить его дважды.
- Следствие для получателя — обработчик обязан быть идемпотентным, и проверка по идентификатору события должна происходить до побочных эффектов, а не после.
- Порядок тоже не гарантирован: события могут прийти не в том порядке, в каком произошли, поэтому состояние восстанавливают по данным события, а не по факту его прихода.

**3. Почему обработчик должен отвечать быстро?**

- У отправителей жёсткий таймаут, обычно от 5 до 30 секунд, и медленный ответ засчитывается как неудача.
- Следствие — синхронная тяжёлая обработка сама порождает повторные доставки и, значит, дубликаты работы.
- Правильная последовательность — проверить подпись, сохранить событие, ответить успехом, обработать асинхронно.
- Дополнительно это разделяет отказы: недоступность внутреннего сервиса больше не превращается в повторную доставку снаружи.

**4. Чем webhooks отличаются от опроса и от WebSocket?**

- Опрос — получатель сам спрашивает по расписанию: простой, но создаёт лишние запросы и даёт задержку до следующего интервала.
- Webhooks — отправитель сам стучится в эндпоинт получателя: нет задержки и лишних запросов, но нужен публичный адрес, подпись и обработка повторов.
- WebSocket — постоянное соединение для двустороннего обмена в реальном времени; для межсистемной интеграции избыточен и хрупок, потому что требует, чтобы обе стороны были онлайн одновременно.
- Практический признак выбора — webhooks для интеграции между системами с редкими событиями, WebSocket для интерфейса пользователя с потоком обновлений.

**5. Как отправлять webhooks надёжно со своей стороны?**

- Прямая отправка из обработчика бизнес-операции теряет события при сбое: транзакция прошла, а отправка нет.
- Решение — outbox: событие пишется в ту же транзакцию, что и изменение данных, а отправкой занимается отдельный процесс ([outbox и идемпотентность](../../06-databases/database-systems-catalog/postgresql/14-outbox-and-idempotency.md)).
- Повторы — нарастающая выдержка с разбросом, ограниченное число попыток, после исчерпания событие уходит в очередь разбора, а подписка помечается проблемной.
- Наблюдаемость — доля успешных доставок по каждому получателю и возраст самого старого неотправленного события: без них о сломанной интеграции узнают от клиента.
