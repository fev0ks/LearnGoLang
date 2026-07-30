# Идемпотентность запросов

## Содержание

- [Зачем нужна идемпотентность](#зачем-нужна-идемпотентность)
- [Idempotency-Key header: стандарт отрасли](#idempotency-key-header-стандарт-отрасли)
- [Генерация idempotency key](#генерация-idempotency-key)
- [Серверная реализация: check-lock-process-store](#серверная-реализация-check-lock-process-store)
- [Реализация на Redis (recommended)](#реализация-на-redis-recommended)
- [Реализация на PostgreSQL](#реализация-на-postgresql)
- [Concurrency: race condition при параллельных запросах](#concurrency-race-condition-при-параллельных-запросах)
- [TTL: как долго хранить ключи](#ttl-как-долго-хранить-ключи)
- [Идемпотентность consumer'а (message broker)](#идемпотентность-consumerа-message-broker)
- [Database-level idempotency](#database-level-idempotency)
- [Scope и безопасность](#scope-и-безопасность)
- [Сохранять ли ошибочные ответы?](#сохранять-ли-ошибочные-ответы)
- [Итоговые правила](#итоговые-правила)
- [Interview-ready answer](#interview-ready-answer)

Идемпотентность — свойство операции, которая даёт одинаковый результат при любом количестве повторных вызовов. Критически важно для платёжных систем, order processing, webhook handlers — везде где возможны повторные запросы.

---

## Зачем нужна идемпотентность

### Проблема: повторные запросы неизбежны

```
Client ──POST /orders──► Server
                            │ (обрабатывает... ~500ms)
                            │
       timeout на client ◄──┘
       
Client: "запрос упал, надо retry"
Client ──POST /orders──► Server (уже создал заказ!)

Итог: два заказа вместо одного, дважды списание с карты
```

Повторные запросы появляются по многим причинам:
- Client timeout + retry
- Network instability
- Load balancer retry
- Webhook provider retry (at-least-once delivery)
- Message broker redelivery (at-least-once)
- Человек нажал кнопку дважды

### HTTP методы: что идемпотентно по спецификации

| Метод | Идемпотентен | Safe (read-only) | Комментарий |
|---|---|---|---|
| GET | ✅ | ✅ | Не меняет state |
| HEAD | ✅ | ✅ | Как GET без body |
| PUT | ✅ | ❌ | N раз = тот же результат |
| DELETE | ✅ | ❌ | N удалений = 1 удаление |
| **POST** | **❌** | ❌ | Создание — каждый раз новая запись |
| PATCH | ❌ | ❌ | Зависит от операции |

POST создаёт ресурс — не идемпотентен по природе. Именно здесь нужен Idempotency-Key.

---

## Idempotency-Key header: стандарт отрасли

### Как используют Stripe, GitHub, и другие

```http
POST /v1/charges
Idempotency-Key: a12b3c4d-5e6f-7890-abcd-ef1234567890
Content-Type: application/json

{
    "amount": 5000,
    "currency": "usd",
    "customer": "cus_abc123"
}
```

Первый вызов → обрабатывается, ответ сохраняется.  
Повторный с тем же ключом → возвращается сохранённый ответ без повторной обработки.

### Ключевые свойства поведения

1. **Одинаковый ответ**: повторный запрос возвращает точно тот же HTTP статус и body что и оригинальный
2. **TTL**: ключ действует ограниченное время (Stripe — 24 часа)
3. **Привязан к запросу**: если body другой — ошибка `422 Unprocessable Entity`
4. **Scope**: обычно scope — пара (user/app + key), не глобальный

---

## Генерация idempotency key

### Клиентская сторона: UUID v4

```go
import "github.com/google/uuid"

// Стандартный подход: client генерирует UUID v4 перед запросом
idempotencyKey := uuid.New().String() // "550e8400-e29b-41d4-a716-446655440000"

req, _ := http.NewRequestWithContext(ctx, "POST", "/orders", body)
req.Header.Set("Idempotency-Key", idempotencyKey)

// При retry — тот же ключ
```

UUID v4 — криптографически случаен (122 бита энтропии). Коллизия практически невозможна.

### Hash-based key: детерминированный по содержанию

Если клиент не может хранить UUID между попытками — генерирует ключ из содержимого запроса:

```go
import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
)

type OrderRequest struct {
    CustomerID string  `json:"customer_id"`
    ProductID  string  `json:"product_id"`
    Quantity   int     `json:"quantity"`
    Price      int64   `json:"price_cents"`
}

// Детерминированный ключ: sha256 от (userID + canonical request)
func idempotencyKeyFromRequest(userID string, req OrderRequest) string {
    data, _ := json.Marshal(req) // canonical JSON
    h := sha256.New()
    h.Write([]byte(userID + ":"))
    h.Write(data)
    return hex.EncodeToString(h.Sum(nil))
}

// Один и тот же заказ → один и тот же ключ
key := idempotencyKeyFromRequest("user-123", OrderRequest{
    CustomerID: "user-123",
    ProductID:  "prod-456",
    Quantity:   2,
    Price:      9900,
})
```

**Когда hash-based**: идемпотентность по смыслу операции (дублирующийся заказ = тот же заказ). Stripe использует именно UUID — явно от клиента.

### Composite key: бизнес-смысл

```go
// Идемпотентность на уровне бизнес-логики без отдельного header
// Например: один пользователь — один активный заказ на продукт

type OrderKey struct {
    UserID    string
    ProductID string
    Date      string // "2025-04-24" — в рамках одного дня
}

// INSERT INTO orders (...) ON CONFLICT (user_id, product_id, date) DO NOTHING
```

---

## Серверная реализация: check-lock-process-store

### Базовая схема

Обработка любого запроса с ключом идемпотентности сводится к четырём шагам, и порядок между ними важнее самих шагов: сначала проверить, не обрабатывался ли ключ, затем захватить его за собой, только потом выполнить операцию и в конце сохранить ответ. Ошибка почти всегда одна — выполнить операцию раньше, чем ключ захвачен.

```
Получить Idempotency-Key
       │
       ▼
Есть в storage?
    ├── ДА  → вернуть сохранённый ответ (не обрабатывать)
    └── НЕТ → обработать запрос → сохранить ответ → вернуть
```

### Проблема конкурентных запросов

```
Request A (key=abc) ──► check: "не существует"
Request B (key=abc) ──► check: "не существует"   ← race condition!
Request A ──► process: create order #1
Request B ──► process: create order #2  ← дублирование!
```

**Решение: distributed lock или INSERT ... ON CONFLICT.**

---

## Реализация на Redis (recommended)

### Атомарный SET NX + TTL

Проверка и захват ключа обязаны быть одной операцией: раздельные «посмотреть» и «записать» разъезжаются при параллельных запросах, и оба проходят проверку. Команда `SET key value NX EX ttl` делает и то, и другое атомарно — либо создаёт ключ и сообщает об успехе, либо не делает ничего.

В значение кладут не отметку присутствия, а состояние обработки: пока операция идёт, там маркер «в работе», после завершения — сохранённый ответ. Так второй запрос отличает «уже выполнено, вот результат» от «выполняется прямо сейчас, подожди».

<details>
<summary>Захват ключа и сохранение результата в Redis</summary>

```go
package idempotency

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

var ErrConflict = errors.New("idempotency key conflict: request body mismatch")

type StoredResponse struct {
    StatusCode int             `json:"status_code"`
    Body       json.RawMessage `json:"body"`
    RequestHash string         `json:"request_hash"` // для проверки совпадения body
}

type RedisStore struct {
    rdb *redis.Client
    ttl time.Duration
}

func NewRedisStore(rdb *redis.Client, ttl time.Duration) *RedisStore {
    return &RedisStore{rdb: rdb, ttl: ttl}
}

// Get возвращает сохранённый ответ или nil если ключа нет
func (s *RedisStore) Get(ctx context.Context, key string) (*StoredResponse, error) {
    data, err := s.rdb.Get(ctx, "idempotency:"+key).Bytes()
    if err == redis.Nil {
        return nil, nil // ключ не существует
    }
    if err != nil {
        return nil, fmt.Errorf("redis get: %w", err)
    }
    
    var resp StoredResponse
    if err := json.Unmarshal(data, &resp); err != nil {
        return nil, fmt.Errorf("unmarshal: %w", err)
    }
    return &resp, nil
}

// SetNX атомарно записывает ответ если ключа нет.
// Возвращает (true, nil) если записал, (false, nil) если ключ уже существовал.
func (s *RedisStore) SetNX(ctx context.Context, key string, resp *StoredResponse) (bool, error) {
    data, err := json.Marshal(resp)
    if err != nil {
        return false, err
    }
    set, err := s.rdb.SetNX(ctx, "idempotency:"+key, data, s.ttl).Result()
    return set, err
}

// Store перезаписывает (используется для сохранения финального ответа)
func (s *RedisStore) Store(ctx context.Context, key string, resp *StoredResponse) error {
    data, _ := json.Marshal(resp)
    return s.rdb.Set(ctx, "idempotency:"+key, data, s.ttl).Err()
}
```

</details>

### Middleware для HTTP handlers

Логичное место для этой механики — middleware, а не каждый обработчик: тогда идемпотентность включается наличием заголовка, а бизнес-код о ней не знает. Middleware перехватывает ответ обработчика, сохраняет его вместе с кодом статуса и на повтор отдаёт сохранённое, вообще не доходя до обработчика.

Две детали, которые обычно упускают. Первая — сохранять нужно и код ответа, и тело: иначе повтор вернёт `200` там, где исходный запрос ответил `201 Created`. Вторая — сохранять только успешные ответы, потому что закешированная ошибка закрепит временный сбой за этим ключом навсегда.

<details>
<summary>Middleware целиком: захват ответа, сохранение, повторная отдача</summary>

```go
package idempotency

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "io"
    "net/http"
)

// Middleware добавляет идемпотентность к POST/PATCH handlers
func Middleware(store *RedisStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Применяем только к мутирующим методам
            if r.Method == http.MethodGet || r.Method == http.MethodHead {
                next.ServeHTTP(w, r)
                return
            }
            
            key := r.Header.Get("Idempotency-Key")
            if key == "" {
                next.ServeHTTP(w, r) // без ключа — обычная обработка
                return
            }
            
            // Читаем body для hash (нужно восстановить r.Body после)
            bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
            if err != nil {
                http.Error(w, "read body", http.StatusBadRequest)
                return
            }
            r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
            
            // Hash тела запроса для проверки совпадения
            h := sha256.Sum256(bodyBytes)
            reqHash := hex.EncodeToString(h[:])
            
            // Проверяем наличие ключа
            existing, err := store.Get(r.Context(), key)
            if err != nil {
                http.Error(w, "idempotency check failed", http.StatusInternalServerError)
                return
            }
            
            if existing != nil {
                // Ключ найден
                if existing.RequestHash != reqHash {
                    // Тот же ключ, другой body — ошибка
                    http.Error(w, "idempotency key reuse with different request body",
                        http.StatusUnprocessableEntity)
                    return
                }
                // Возвращаем кешированный ответ
                w.Header().Set("Idempotent-Replayed", "true")
                w.WriteHeader(existing.StatusCode)
                w.Write(existing.Body)
                return
            }
            
            // Ключ не найден — обрабатываем, перехватываем ответ
            rec := &responseRecorder{
                ResponseWriter: w,
                statusCode:     http.StatusOK,
                buf:            &bytes.Buffer{},
            }
            next.ServeHTTP(rec, r)
            
            // Сохраняем ответ (только при успехе — 2xx)
            if rec.statusCode >= 200 && rec.statusCode < 300 {
                stored := &StoredResponse{
                    StatusCode:  rec.statusCode,
                    Body:        json.RawMessage(rec.buf.Bytes()),
                    RequestHash: reqHash,
                }
                // Игнорируем ошибку сохранения — запрос уже обработан
                store.Store(r.Context(), key, stored)
            }
        })
    }
}

// responseRecorder перехватывает WriteHeader и Write
type responseRecorder struct {
    http.ResponseWriter
    statusCode int
    buf        *bytes.Buffer
    written    bool
}

func (r *responseRecorder) WriteHeader(code int) {
    r.statusCode = code
    r.ResponseWriter.WriteHeader(code) // пишем в реальный ResponseWriter
    r.written = true
}

func (r *responseRecorder) Write(b []byte) (int, error) {
    if !r.written {
        r.WriteHeader(http.StatusOK)
    }
    r.buf.Write(b)                     // копируем в буфер
    return r.ResponseWriter.Write(b)   // пишем в реальный ResponseWriter
}
```

</details>

### Использование

```go
store := idempotency.NewRedisStore(rdb, 24*time.Hour)

mux.Handle("POST /orders",
    idempotency.Middleware(store)(
        http.HandlerFunc(createOrderHandler),
    ),
)

// Или через Chain:
handler := Chain(mux,
    Recovery,
    Logging,
    idempotency.Middleware(store), // только для POST/PATCH
)
```

---

## Реализация на PostgreSQL

Подходит если Redis недоступен или нужна транзакционная атомарность с основной операцией:

```sql
CREATE TABLE idempotency_keys (
    key          TEXT        PRIMARY KEY,
    request_hash TEXT        NOT NULL,
    status_code  INTEGER     NOT NULL,
    response     JSONB       NOT NULL,
    user_id      TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX ON idempotency_keys (expires_at); -- для очистки
```

```go
// Атомарная операция: вставить или вернуть существующее
func (s *PgStore) GetOrCreate(ctx context.Context, tx pgx.Tx, key, reqHash string) (*StoredResponse, bool, error) {
    // INSERT ... ON CONFLICT DO NOTHING + SELECT
    // Это атомарно в рамках одной транзакции
    
    var resp StoredResponse
    err := tx.QueryRow(ctx, `
        WITH inserted AS (
            INSERT INTO idempotency_keys (key, request_hash, status_code, response, user_id, expires_at)
            VALUES ($1, $2, 0, 'null'::jsonb, $3, NOW() + INTERVAL '24 hours')
            ON CONFLICT (key) DO NOTHING
            RETURNING NULL
        )
        SELECT status_code, response, request_hash
        FROM idempotency_keys
        WHERE key = $1
    `, key, reqHash, userID(ctx)).Scan(&resp.StatusCode, &resp.Body, &resp.RequestHash)
    
    if err != nil {
        return nil, false, err
    }
    
    if resp.StatusCode == 0 {
        return nil, true, nil // мы вставили placeholder — обрабатываем
    }
    return &resp, false, nil // уже существует — возвращаем
}

// После обработки — обновляем placeholder финальным ответом
func (s *PgStore) Complete(ctx context.Context, tx pgx.Tx, key string, resp *StoredResponse) error {
    _, err := tx.Exec(ctx, `
        UPDATE idempotency_keys
        SET status_code = $2, response = $3
        WHERE key = $1
    `, key, resp.StatusCode, resp.Body)
    return err
}
```

### Транзакционная атомарность с основной операцией

```go
func createOrder(ctx context.Context, db *pgxpool.Pool, key string, req CreateOrderRequest) (*Order, error) {
    tx, err := db.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)
    
    // Проверяем idempotency ключ в той же транзакции
    existing, isNew, err := idempStore.GetOrCreate(ctx, tx, key, hashRequest(req))
    if err != nil {
        return nil, err
    }
    if !isNew {
        // Уже обрабатывали — вернуть кешированный ответ
        var order Order
        json.Unmarshal(existing.Body, &order)
        return &order, nil
    }
    
    // Создаём заказ
    order, err := insertOrder(ctx, tx, req)
    if err != nil {
        return nil, err
    }
    
    // Сохраняем ответ в той же транзакции
    respBody, _ := json.Marshal(order)
    if err := idempStore.Complete(ctx, tx, key, &StoredResponse{
        StatusCode: 201,
        Body:       respBody,
    }); err != nil {
        return nil, err
    }
    
    return order, tx.Commit(ctx)
    // Если Commit упал → транзакция откатилась → idempotency запись не создана
    // Клиент получит ошибку и повторит с тем же ключом → новая попытка
}
```

**Преимущество PostgreSQL подхода**: запись в idempotency_keys и сам заказ создаются в одной транзакции — нет промежуточного состояния.

---

## Concurrency: race condition при параллельных запросах

Два запроса с одинаковым ключом пришли одновременно:

```
Request A: GET → nil; INSERT → ok  → process → store
Request B: GET → nil; INSERT → conflict!
```

### С Redis: SETNX решает проблему

Атомарный захват превращает гонку в предсказуемый сценарий: один запрос захватывает ключ и работает, остальные видят, что ключ занят, и получают либо сохранённый ответ, либо признак «в обработке». Ни один из них не выполняет операцию повторно.

```
SET idempotency:key value NX EX 86400
# NX = только если не существует
# Атомарная операция — race condition невозможен
```

```go
// Но нужен двухфазный подход:
// Фаза 1: резервируем ключ со статусом "in-progress"
// Фаза 2: обновляем с финальным ответом после обработки

type InProgressMarker struct {
    StartedAt time.Time `json:"started_at"`
    InProgress bool     `json:"in_progress"`
}

func (s *RedisStore) Reserve(ctx context.Context, key string) (bool, error) {
    marker := InProgressMarker{StartedAt: time.Now(), InProgress: true}
    data, _ := json.Marshal(marker)
    
    // Резервируем с небольшим TTL — если обработка зависнет, ключ истечёт
    set, err := s.rdb.SetNX(ctx, "idempotency:"+key, data, 30*time.Second).Result()
    return set, err
}
```

### Что возвращать пока запрос обрабатывается?

```
Request B получает "in-progress" → что ответить?
```

Три стратегии:
1. **`202 Accepted` + `Retry-After` header**: "обрабатывается, попробуй позже"
2. **`409 Conflict`**: "запрос с этим ключом уже выполняется"
3. **Ждать и retry**: клиент ждёт пока первый запрос завершится

```go
// Вариант 1: 409 + повтор
if existing != nil && existing.InProgress {
    w.Header().Set("Retry-After", "1")
    http.Error(w, "request with this key is already being processed", http.StatusConflict)
    return
}
```

---

## TTL: как долго хранить ключи

| Провайдер | TTL |
|---|---|
| Stripe | 24 часа |
| Adyen | 72 часа |
| PayPal | Вечно (для конкретных операций) |
| Типичный сервис | 24–48 часов |

**Правило выбора TTL**: дольше максимально возможного retry window клиента.

Если client retry logic — exponential backoff до 24 часов → TTL > 24 часов.

**Cleanup**: для PostgreSQL нужна периодическая очистка истёкших записей:

```sql
-- Cron job или background goroutine
DELETE FROM idempotency_keys WHERE expires_at < NOW();
```

---

## Идемпотентность consumer'а (message broker)

Не только HTTP — обработчик сообщений тоже должен быть идемпотентным.

### Паттерн: уникальный constraint в БД

У потребителя сообщений нет заголовка от клиента, зато есть идентификатор сообщения от брокера. Дедупликацию строят на нём, и самый надёжный способ — переложить проверку на базу: уникальный индекс по идентификатору не даст вставить запись дважды, каким бы конкурентным ни было потребление.

Преимущество перед проверкой «сначала посмотреть, потом вставить» в том, что база проверяет и вставляет атомарно; приложению остаётся отличить ошибку уникальности от прочих и считать её признаком уже обработанного сообщения.

```go
// Каждое сообщение имеет уникальный ID (Kafka offset или UUID)
func processMessage(ctx context.Context, msg KafkaMessage) error {
    // INSERT ... ON CONFLICT DO NOTHING
    result, err := db.Exec(ctx, `
        INSERT INTO processed_events (event_id, processed_at)
        VALUES ($1, NOW())
        ON CONFLICT (event_id) DO NOTHING
    `, msg.ID)
    
    if err != nil {
        return err
    }
    
    if result.RowsAffected() == 0 {
        // Уже обрабатывали — идемпотентный skip
        return nil
    }
    
    // Обрабатываем только один раз
    return handleEvent(ctx, msg.Payload)
}
```

### Redis-based dedup для message consumer

```go
func (c *Consumer) processWithDedup(ctx context.Context, msgID string, process func() error) error {
    key := "processed:" + msgID
    
    // SETNX: установить если не существует
    set, err := c.rdb.SetNX(ctx, key, "1", 24*time.Hour).Result()
    if err != nil {
        return fmt.Errorf("dedup check: %w", err)
    }
    
    if !set {
        // Уже обрабатывали
        return nil
    }
    
    if err := process(); err != nil {
        // Обработка упала — удаляем ключ чтобы можно было retry
        c.rdb.Del(ctx, key)
        return err
    }
    return nil
}
```

**Проблема**: если `process()` успешен, но `Del` после ошибки не выполнился — сообщение пропущено. Решение: транзакционная запись или Lua script в Redis.

---

## Database-level idempotency

### INSERT ... ON CONFLICT

Самый дешёвый вид идемпотентности — тот, что обеспечивает сама операция. Если у сущности есть естественный уникальный ключ, повторная вставка не создаёт дубликат, а превращается в отсутствие изменений, и никакого отдельного хранилища ключей не нужно.

```go
// Создать запись если не существует, иначе вернуть существующую
var orderID string
err := db.QueryRowContext(ctx, `
    INSERT INTO orders (id, customer_id, amount, status, external_ref)
    VALUES ($1, $2, $3, 'pending', $4)
    ON CONFLICT (external_ref) DO UPDATE SET id = orders.id
    RETURNING id
`, uuid.New(), customerID, amount, externalRef).Scan(&orderID)
```

### UPSERT для обновлений

```go
// Обновить запись если существует, создать если нет
_, err := db.ExecContext(ctx, `
    INSERT INTO user_preferences (user_id, theme, language, updated_at)
    VALUES ($1, $2, $3, NOW())
    ON CONFLICT (user_id) DO UPDATE
    SET theme = EXCLUDED.theme,
        language = EXCLUDED.language,
        updated_at = NOW()
`, userID, theme, language)
```

---

## Scope и безопасность

### Ключ должен быть привязан к пользователю или клиенту

Ключ идемпотентности приходит снаружи, поэтому глобальное пространство ключей — это уязвимость: чужой запрос с угаданным ключом получит сохранённый ответ, а это утечка данных. Область действия ключа всегда включает того, кто его прислал, и обычно ещё и операцию: один и тот же ключ на создание платежа и на отмену не должен считаться одним запросом.

```go
// Плохо: глобальный ключ — любой может "занять" чужой ключ
store.Get(ctx, idempotencyKey)

// Хорошо: scope = userID + key
scopedKey := userID + ":" + idempotencyKey
store.Get(ctx, scopedKey)
```

### Валидация формата ключа

```go
func validateIdempotencyKey(key string) error {
    if key == "" {
        return errors.New("idempotency key is required")
    }
    if len(key) > 255 {
        return errors.New("idempotency key too long (max 255)")
    }
    // UUID формат (рекомендуется)
    if _, err := uuid.Parse(key); err != nil {
        return errors.New("idempotency key must be a valid UUID")
    }
    return nil
}
```

---

## Сохранять ли ошибочные ответы?

Два подхода:

**Сохранять только успешные (2xx)**:
- Если запрос упал (5xx, timeout) — клиент может retry и получит новую попытку
- Если клиент прислал неверные данные (4xx) и исправил их — тот же ключ + новый body → ошибка `422`
- Stripe делает именно так

**Сохранять всё (включая 4xx)**:
- Повторный запрос с тем же ключом → тот же 4xx
- Защита от "бомбардировки" retry на заведомо невалидные запросы
- Требует явного управления "хочу retry с новым ключом или нет"

**Рекомендация**: сохранять все 2xx, не сохранять 5xx (retry разрешён), сохранять 4xx (повтор не поможет):

```go
if rec.statusCode >= 200 && rec.statusCode < 500 {
    store.Store(ctx, key, &StoredResponse{...})
}
// 5xx не сохраняем → retry разрешён с тем же ключом
```

---

## Итоговые правила

| Аспект | Правило |
|---|---|
| Где генерировать ключ | На клиенте (UUID v4) до отправки |
| Хранилище | Redis (TTL из коробки, быстро) или PostgreSQL (транзакционность) |
| TTL | > максимального retry window клиента (обычно 24–48ч) |
| Конкуренция | SETNX (Redis) или INSERT ON CONFLICT (Postgres) — атомарно |
| Scope | Всегда (userID + key), никогда глобальный |
| Body validation | Хранить hash body — возвращать 422 при несовпадении |
| 5xx ответы | Не сохранять — разрешить retry с тем же ключом |
| Consumer | INSERT ... ON CONFLICT DO NOTHING + уникальный event_id |

---

## Interview-ready answer

**1. Что такое ключ идемпотентности и зачем он нужен?**

- Задача — отличить повторную отправку того же запроса от нового запроса, чтобы `POST` можно было безопасно повторять.
- Механика — клиент генерирует токен на логическую операцию, присылает его в заголовке и повторяет с тем же значением; сервер по нему находит уже выполненную работу и отдаёт сохранённый ответ.
- Где критично — создание заказа, списание средств, отправка платежа: сетевой таймаут без ключа превращается в двойное списание.
- Ключевая деталь — токен генерируется один раз на операцию, а не на попытку отправки; новый токен при повторе означает новую операцию.

**2. Как реализовать проверку на сервере?**

- Порядок шагов — проверить и захватить ключ атомарно, выполнить операцию, сохранить ответ; выполнение раньше захвата и есть источник дубликатов.
- Redis — `SET NX EX` захватывает ключ одной командой, срок жизни встроен; минус в том, что это отдельное от основной транзакции хранилище.
- PostgreSQL — запись ключа и сама операция в одной транзакции дают настоящую атомарность без второй системы, ценой нагрузки на базу.
- Уровень бизнес-данных — уникальный индекс по естественному ключу вообще снимает вопрос: повторная вставка не создаёт дубликат.
- Что сохранять — код ответа вместе с телом, иначе повтор вернёт `200` там, где исходный запрос ответил `201`.

**3. Что происходит при двух одновременных запросах с одним ключом?**

- Атомарный захват гарантирует, что операцию выполнит ровно один: остальные видят занятый ключ.
- Второму запросу отдают либо сохранённый ответ, если работа уже закончена, либо признак «в обработке».
- Для «в обработке» обычно отвечают `409 Conflict` с указанием повторить позже — блокировать запрос до конца чужой обработки хуже, потому что таймаут клиента приведёт к новым повторам.
- Ключ должен освобождаться при аварии обработчика: срок жизни маркера «в работе» делают заметно короче срока хранения результата.

**4. Сколько хранить ключи и что делать с ошибками?**

- Срок хранения — не меньше окна повторов клиента: сутки для обычных операций, дольше для платежей, где расследование может занять дни.
- Ошибки клиента (`4xx`) кэшировать по ключу можно: повтор того же неверного запроса даст тот же результат.
- Ошибки сервера (`5xx`) кэшировать нельзя: временный сбой закрепится за ключом навсегда, и клиент не сможет повторить операцию.
- Ключ должен освобождаться и при неуспехе, иначе неудачная попытка блокирует всю операцию до истечения срока.

**5. Чем ключ идемпотентности отличается от дедупликации у потребителя сообщений?**

- Источник ключа — в HTTP его генерирует клиент, у потребителя сообщений берут идентификатор от брокера или бизнес-ключ события.
- Гарантия — at-least-once у брокеров означает, что дубликаты приходят всегда, а не только при сбоях, поэтому дедупликация обязательна, а не опциональна.
- Реализация — надёжнее всего уникальный индекс в базе: он проверяет и вставляет атомарно, в отличие от «сначала посмотреть, потом записать».
- Общее — в обоих случаях проверка обязана происходить до побочных эффектов, иначе дубликат отсекается уже после того, как деньги списаны.

**6. Какие ошибки в области действия ключа опасны?**

- Глобальное пространство ключей — чужой запрос с угаданным ключом получит сохранённый ответ, а это утечка данных; область всегда включает владельца ключа.
- Один ключ на разные операции — создание и отмена с одним значением не должны считаться одним запросом, поэтому в область входит и метод с адресом.
- Отсутствие проверки формата — ключ приходит снаружи, поэтому длину и алфавит ограничивают, иначе он становится вектором для переполнения хранилища.
- Отсутствие срока жизни — хранилище ключей растёт бесконечно и однажды становится проблемой само по себе.
