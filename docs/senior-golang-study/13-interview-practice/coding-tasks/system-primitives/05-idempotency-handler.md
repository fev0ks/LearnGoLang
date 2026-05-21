# Задача 5: Idempotency Key Handler

Идемпотентный endpoint — тот, который при **повторном** вызове даёт **тот же** результат и **не делает действие дважды**. Критически важно для платежей, оформления заказов — то, что нельзя дублировать.

Stripe, Square, AWS API — все используют этот паттерн через header `Idempotency-Key`.

## Формулировка

> "Реализуй middleware для HTTP endpoint, который делает POST идемпотентным через `Idempotency-Key` header. При повторе того же ключа — вернуть кэшированный response."

Use cases:
- Платежи — два клика на "Pay" не должны charge'ать дважды
- Оформление заказа — flaky network, клиент retry — один заказ
- Webhook delivery — at-least-once delivery, обработать один раз

---

## Уточняющие вопросы

1. **Кто генерирует ключ — клиент или сервер?**
   "Клиент. Сервер не имеет inkling что 'это новый запрос'. UUID на стороне клиента."

2. **Сколько ключ валиден?**
   "Stripe — 24 часа. Зависит от business case."

3. **Что если body разный, ключ тот же?**
   "Stripe возвращает 422. Можешь fingerprint'ить body и проверять."

4. **Хранилище — Redis, БД?**
   "БД — persistent. Redis — fast, нужен fallback на TTL expiry."

5. **Concurrent requests с одинаковым ключом?**
   "Tricky! Нужен lock или 'in-progress' state."

6. **Что хранить — только response или full state?**
   "Status + response body + headers."

---

## Базовое решение

```go
package idempotency

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "sync"
    "time"
)

type Storage interface {
    Get(ctx context.Context, key string) (*Record, error)
    Save(ctx context.Context, key string, rec *Record, ttl time.Duration) error
}

type Record struct {
    Status     int
    Body       []byte
    Headers    map[string]string
    CreatedAt  time.Time
    RequestHash string  // для detect mismatch'ей
}

type Middleware struct {
    storage Storage
    ttl     time.Duration

    // Для concurrent same-key request — in-memory lock
    mu       sync.Mutex
    inFlight map[string]chan struct{}
}

func NewMiddleware(s Storage, ttl time.Duration) *Middleware {
    return &Middleware{
        storage:  s,
        ttl:      ttl,
        inFlight: make(map[string]chan struct{}),
    }
}

func (m *Middleware) Handle(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        key := r.Header.Get("Idempotency-Key")

        // Если ключа нет — пропускаем без идемпотентности (или required, как хочешь)
        if key == "" {
            next.ServeHTTP(w, r)
            return
        }

        // Только для mutations
        if r.Method == "GET" || r.Method == "HEAD" {
            next.ServeHTTP(w, r)
            return
        }

        // 1. Прочитать body чтобы fingerprint'нуть
        body, err := io.ReadAll(r.Body)
        if err != nil {
            http.Error(w, "read body", http.StatusBadRequest)
            return
        }
        r.Body = io.NopCloser(bytes.NewReader(body))  // restore для handler'а

        requestHash := hashRequest(r.Method, r.URL.Path, body)

        // 2. Wait если concurrent same-key
        m.mu.Lock()
        if ch, ok := m.inFlight[key]; ok {
            m.mu.Unlock()
            <-ch  // ждём пока first request закончит
        } else {
            ch := make(chan struct{})
            m.inFlight[key] = ch
            m.mu.Unlock()
            defer func() {
                m.mu.Lock()
                delete(m.inFlight, key)
                close(ch)
                m.mu.Unlock()
            }()
        }

        // 3. Проверить existing record
        rec, err := m.storage.Get(r.Context(), key)
        if err != nil && !isNotFound(err) {
            http.Error(w, "storage error", http.StatusInternalServerError)
            return
        }

        if rec != nil {
            // Уже обрабатывали — проверить что body тот же
            if rec.RequestHash != requestHash {
                http.Error(w, "idempotency key reused with different body", http.StatusUnprocessableEntity)
                return
            }

            // Вернуть cached response
            for k, v := range rec.Headers {
                w.Header().Set(k, v)
            }
            w.Header().Set("Idempotent-Replayed", "true")
            w.WriteHeader(rec.Status)
            w.Write(rec.Body)
            return
        }

        // 4. Первый раз — выполнить, записать ответ
        recorder := &responseRecorder{
            ResponseWriter: w,
            body:           &bytes.Buffer{},
            headers:        make(http.Header),
        }

        next.ServeHTTP(recorder, r)

        // 5. Сохранить если success (2xx, 4xx — но не 5xx)
        if recorder.status < 500 {
            headers := make(map[string]string)
            for k, v := range recorder.Header() {
                if len(v) > 0 {
                    headers[k] = v[0]
                }
            }

            rec := &Record{
                Status:      recorder.status,
                Body:        recorder.body.Bytes(),
                Headers:     headers,
                CreatedAt:   time.Now(),
                RequestHash: requestHash,
            }
            m.storage.Save(r.Context(), key, rec, m.ttl)
        }
    })
}

// responseRecorder перехватывает ответ от handler'а.
type responseRecorder struct {
    http.ResponseWriter
    body    *bytes.Buffer
    status  int
    headers http.Header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
    r.body.Write(b)
    return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteHeader(code int) {
    r.status = code
    r.ResponseWriter.WriteHeader(code)
}

func hashRequest(method, path string, body []byte) string {
    h := sha256.Sum256([]byte(method + path + string(body)))
    return hex.EncodeToString(h[:])
}
```

**Использование:**

```go
storage := postgres.NewIdempotencyStorage(db)
mw := idempotency.NewMiddleware(storage, 24*time.Hour)

mux := http.NewServeMux()
mux.Handle("POST /orders", mw.Handle(orderHandler))
```

**Запрос клиента:**
```http
POST /orders
Idempotency-Key: 8400e290-...
Content-Type: application/json

{"item": "book", "qty": 1}
```

Первый раз — создаём заказ. Второй — отдаём cached response (с header `Idempotent-Replayed: true`).

---

## Storage реализации

### PostgreSQL

```sql
CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    status INT NOT NULL,
    body BYTEA,
    headers JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_idempotency_keys_created ON idempotency_keys (created_at);
```

```go
type pgStorage struct {
    db *pgxpool.Pool
}

func (s *pgStorage) Get(ctx context.Context, key string) (*Record, error) {
    var r Record
    var headersJSON []byte
    err := s.db.QueryRow(ctx, `
        SELECT request_hash, status, body, headers, created_at
        FROM idempotency_keys WHERE key = $1
    `, key).Scan(&r.RequestHash, &r.Status, &r.Body, &headersJSON, &r.CreatedAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    json.Unmarshal(headersJSON, &r.Headers)
    return &r, nil
}

func (s *pgStorage) Save(ctx context.Context, key string, rec *Record, ttl time.Duration) error {
    headersJSON, _ := json.Marshal(rec.Headers)
    _, err := s.db.Exec(ctx, `
        INSERT INTO idempotency_keys (key, request_hash, status, body, headers, created_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (key) DO NOTHING
    `, key, rec.RequestHash, rec.Status, rec.Body, headersJSON, rec.CreatedAt)
    return err
}

// Periodic cleanup
func (s *pgStorage) Cleanup(ctx context.Context, age time.Duration) error {
    _, err := s.db.Exec(ctx, `
        DELETE FROM idempotency_keys WHERE created_at < $1
    `, time.Now().Add(-age))
    return err
}
```

### Redis

```go
type redisStorage struct {
    client *redis.Client
}

func (s *redisStorage) Get(ctx context.Context, key string) (*Record, error) {
    data, err := s.client.Get(ctx, "idem:"+key).Bytes()
    if errors.Is(err, redis.Nil) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    var r Record
    if err := json.Unmarshal(data, &r); err != nil {
        return nil, err
    }
    return &r, nil
}

func (s *redisStorage) Save(ctx context.Context, key string, rec *Record, ttl time.Duration) error {
    data, _ := json.Marshal(rec)
    // SET NX — если кто-то параллельно записал, не перезаписываем
    return s.client.SetNX(ctx, "idem:"+key, data, ttl).Err()
}
```

**Trade-off:**
- **PostgreSQL** — strong consistency, можно делать idempotency INSERT в той же transaction что и business write
- **Redis** — fast, но при crash может потерять (если AOF=everysec)

Для **платежей** — Postgres. Для **обычных API** — Redis OK.

---

## Атомарность idempotency + business write

Самый важный момент: idempotency record должен записываться **в той же транзакции** что и business write. Иначе:

```
1. Create order in DB → success
2. Save idempotency record → fails (network)
3. Client retry с тем же ключом
4. No record found → create order AGAIN → duplicate
```

**Правильно:**

```go
func (s *OrderService) CreateOrder(ctx context.Context, idemKey string, req CreateOrderRequest) (*Order, error) {
    return WithTx(ctx, s.db, func(tx pgx.Tx) (*Order, error) {
        // 1. Проверить existing record
        rec, err := getIdempotencyRecord(ctx, tx, idemKey)
        if err != nil && !errors.Is(err, ErrNotFound) {
            return nil, err
        }
        if rec != nil {
            // Уже обрабатывали — вернуть cached
            return rec.Order, nil
        }

        // 2. Создать order
        order, err := createOrder(ctx, tx, req)
        if err != nil {
            return nil, err
        }

        // 3. Записать idempotency record в ТОЙ ЖЕ tx
        err = saveIdempotencyRecord(ctx, tx, idemKey, &Record{...})
        if err != nil {
            return nil, err
        }

        return order, nil  // commit обоих изменений
    })
}
```

**Garantee:** либо и order создан, и idempotency record записан, либо ничего.

См. также [Saga и Outbox](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md) — там тот же принцип атомарности.

---

## Concurrent same-key requests

Самая тонкая часть. Сценарий:
- Клиент сделал retry слишком быстро
- Два запроса с одним ключом одновременно
- Первый ещё не дозаписал record, второй проверил — ничего нет
- Оба создают order

**Решения:**

### 1. Distributed lock per-key

```go
lock, err := dlocker.Acquire(ctx, "idem-lock:"+key, 30*time.Second)
if err != nil { ... }
defer lock.Release(ctx)

// Внутри lock — атомарно: check + write
```

### 2. INSERT с UNIQUE constraint

```sql
INSERT INTO idempotency_keys (key, status) VALUES ($1, 'pending')
ON CONFLICT (key) DO NOTHING
RETURNING ...
```

Если RETURNING пусто → кто-то другой работает, ждём.

### 3. In-memory lock (только для single-pod)

См. базовое решение выше — `inFlight` map. Не работает между pod'ами.

### 4. Optimistic с status field

```
1. INSERT key, status='pending' (UNIQUE — fail если уже есть)
2. Если INSERT success — process, update status='completed'
3. Если INSERT fail (conflict) — read existing
4. Если existing.status='pending' — wait/retry
```

---

## Тесты

```go
func TestIdempotency_FirstRequest(t *testing.T) {
    storage := newMockStorage()
    mw := NewMiddleware(storage, time.Hour)

    var handlerCalls int
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        handlerCalls++
        w.WriteHeader(201)
        w.Write([]byte(`{"id":1}`))
    })

    req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"item":"x"}`))
    req.Header.Set("Idempotency-Key", "abc-123")
    w := httptest.NewRecorder()

    mw.Handle(handler).ServeHTTP(w, req)

    if w.Code != 201 || handlerCalls != 1 {
        t.Errorf("first request failed")
    }
}

func TestIdempotency_Replay(t *testing.T) {
    storage := newMockStorage()
    mw := NewMiddleware(storage, time.Hour)

    var handlerCalls int
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        handlerCalls++
        w.WriteHeader(201)
        w.Write([]byte(`{"id":1}`))
    })

    // First request
    req1 := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"item":"x"}`))
    req1.Header.Set("Idempotency-Key", "abc-123")
    mw.Handle(handler).ServeHTTP(httptest.NewRecorder(), req1)

    // Second — should be replayed
    req2 := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"item":"x"}`))
    req2.Header.Set("Idempotency-Key", "abc-123")
    w2 := httptest.NewRecorder()
    mw.Handle(handler).ServeHTTP(w2, req2)

    if handlerCalls != 1 {
        t.Errorf("handler called %d times, want 1", handlerCalls)
    }
    if w2.Code != 201 {
        t.Errorf("replay status %d, want 201", w2.Code)
    }
    if w2.Header().Get("Idempotent-Replayed") != "true" {
        t.Error("missing Idempotent-Replayed header")
    }
}

func TestIdempotency_BodyMismatch(t *testing.T) {
    storage := newMockStorage()
    mw := NewMiddleware(storage, time.Hour)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(201)
    })

    req1 := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"item":"x"}`))
    req1.Header.Set("Idempotency-Key", "key1")
    mw.Handle(handler).ServeHTTP(httptest.NewRecorder(), req1)

    // Same key, DIFFERENT body
    req2 := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"item":"y"}`))
    req2.Header.Set("Idempotency-Key", "key1")
    w2 := httptest.NewRecorder()
    mw.Handle(handler).ServeHTTP(w2, req2)

    if w2.Code != http.StatusUnprocessableEntity {
        t.Errorf("status %d, want 422", w2.Code)
    }
}
```

---

## Подводные камни

### 1. Сохранять 5xx response как cached

```go
// ❌ Server error закэширован
if rec.Status == 500 {
    return rec  // ← каждый retry получает 500
}
```

5xx — transient. Клиент должен иметь право retry. Не сохранять, или сохранять с коротким TTL.

### 2. Не сохранять 4xx

```go
// Validation failed — но client должен видеть тот же error при retry
if rec.Status == 400 {
    // Сохранить и replay — клиент знает что валидация падает
}
```

4xx — должны быть сохранены (deterministic for given input).

### 3. Storing huge response body

```go
// API возвращает 100MB JSON
// Сохранили в idempotency table → 100MB row → slow lookup
```

Lim'ит size. Большие response через external store (S3) + reference.

### 4. Не учитывать concurrent same-key

Single-pod in-memory lock не работает между pod'ами. См. выше.

### 5. Storage failure → reject all

```go
rec, err := storage.Get(ctx, key)
if err != nil {
    return error  // ← Redis down → весь API down
}
```

При storage error лучше — degrade gracefully. Без идемпотентности но request проходит. Risk: дубликаты при retry.

### 6. Cleanup растущей таблицы

```go
// Без cleanup taблица растёт бесконечно
```

Periodic DELETE по `created_at < now() - 24h`. Или partition'ирование по date.

### 7. Hash body — sensitive to whitespace

```go
hash(`{"x":1}`)  != hash(`{ "x": 1 }`)
```

Идентичные JSON структуры → разный hash. Нормализуй before hashing (canonical JSON) — но это сложно.

Альтернатива — не проверять hash, доверять клиенту что Idempotency-Key уникален per logical request.

### 8. Сохранять request_id, не business данные

```go
// Хранить весь request body — privacy issue (могут быть PII)
```

Sensitive данные не должны лежать долго. Hash тела + response — обычно достаточно.

### 9. Key reuse across endpoints

```go
// Client использует тот же ключ для /orders и /payments
POST /orders   Idempotency-Key: abc
POST /payments Idempotency-Key: abc  // ← вернёт cached order response!
```

Включать path в hash или namespace ключ: `endpoint:key`.

### 10. No expiration in client

Клиент кэширует Idempotency-Key неделями. Через 7 дней TTL истёк, клиент retry с тем же ключом — выполнится снова.

Документировать: "key valid for 24h".

---

## Возможные расширения

### 1. Strict mode

При body mismatch — 422 (как Stripe).

### 2. In-progress state

Если запрос в процессе обработки — другие same-key получают 409 "currently in progress".

### 3. Distributed locking

Per-key dlock через Redis (см. [04-distributed-lock.md](./04-distributed-lock.md)).

### 4. Async processing

Сохранить request, вернуть `202 Accepted` + `Location: /jobs/123`. Клиент polls статус. Особенно для long-running.

### 5. Metrics

- Cache hit rate (сколько replay'ев)
- Conflict rate (mismatched body)
- Storage latency

---

## Что важно показать на собеседовании

1. **Client-generated key** (UUID) — не server-generated
2. **POST/PATCH/DELETE** — только mutations
3. **Body hash** — detect "same key, different body" (422)
4. **Atomicity** — idempotency record + business write в одной transaction
5. **TTL** — 24 часа стандарт, как Stripe
6. **Concurrent same-key handling** — lock или UNIQUE constraint
7. **5xx не cached** — transient errors retryable
8. **Storage choice** — DB для critical, Redis для fast
9. **Idempotent-Replayed header** — клиенту полезно знать
10. **Stripe API reference** — стандарт индустрии

## Связки

- [Idempotency (reliability)](../../../05-system-design/reliability-patterns/06-idempotency.md) — теория
- [Outbox pattern](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md) — atomic write
- [Distributed lock](./04-distributed-lock.md) — concurrent same-key
- [Stripe API docs: idempotency](https://stripe.com/docs/api/idempotent_requests) — industry standard
