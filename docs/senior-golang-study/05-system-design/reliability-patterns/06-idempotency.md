# Idempotency

Идемпотентная операция — операция которую можно выполнить несколько раз с тем же результатом что и один раз. Это фундаментальное требование для надёжных retry и at-least-once delivery.

## Содержание

- [Зачем idempotency](#зачем-idempotency)
- [Idempotency key паттерн](#idempotency-key-паттерн)
- [Реализация в PostgreSQL](#реализация-в-postgresql)
- [At-most-once vs At-least-once vs Exactly-once](#at-most-once-vs-at-least-once-vs-exactly-once)
- [Idempotency в message queues](#idempotency-в-message-queues)
- [Антипаттерны](#антипаттерны)

---

## Зачем idempotency

Сетевые запросы ненадёжны. Клиент не знает дошёл ли запрос до сервера, если получил timeout или сетевую ошибку:

```
Клиент                          Сервер
  │──── POST /payments ────────►│ (сервер принял)
  │                             │ (обрабатывает 1000ms)
  │◄─── TCP timeout ────────────│ (сервер завершил, ответ потерян)
  │
  │ Что делать?
  │ Retry? Может списали деньги дважды.
  │ Не retry? Может не списали вообще.
```

Идемпотентный API позволяет retry без риска дублей:
```
Клиент посылает уникальный ключ:
  POST /payments
  Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000

Первый запрос: списать деньги, сохранить результат под ключом.
Повторный запрос с тем же ключом: вернуть сохранённый результат, не списывать снова.
```

---

## Idempotency key паттерн

Клиент генерирует уникальный ключ (UUID v4 или v7) и отправляет с каждым запросом. Сервер:
1. Проверяет есть ли уже результат для этого ключа.
2. Если да — возвращает сохранённый результат.
3. Если нет — выполняет операцию, сохраняет результат, возвращает клиенту.

```go
// Handler
func (h *PaymentHandler) Create(w http.ResponseWriter, r *http.Request) {
    idempKey := r.Header.Get("Idempotency-Key")
    if idempKey == "" {
        http.Error(w, "Idempotency-Key header required", http.StatusBadRequest)
        return
    }

    var req CreatePaymentRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid body", http.StatusBadRequest)
        return
    }

    result, err := h.service.CreatePayment(r.Context(), idempKey, req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    // Возвращать 200 или 201 — зависит от того, новый это результат или cached
    if result.WasCached {
        w.WriteHeader(http.StatusOK)
    } else {
        w.WriteHeader(http.StatusCreated)
    }
    json.NewEncoder(w).Encode(result.Payment)
}
```

---

## Реализация в PostgreSQL

Таблица для хранения idempotency records:

```sql
CREATE TABLE idempotency_keys (
    key         TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,  -- hash запроса для обнаружения коллизий ключей
    response     JSONB,          -- сохранённый ответ
    status_code  INT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ  -- NULL пока в процессе
);

-- Автоудаление старых записей
CREATE INDEX ON idempotency_keys (created_at);
-- Крон или pg_partman для очистки записей старше 24 часов
```

Сервис:

```go
type IdempotencyRecord struct {
    Key         string
    RequestHash string
    Response    []byte
    StatusCode  int
    CompletedAt *time.Time
}

type PaymentService struct {
    db *pgxpool.Pool
}

func (s *PaymentService) CreatePayment(ctx context.Context, idempKey string, req CreatePaymentRequest) (*PaymentResult, error) {
    // 1. Проверить существующую запись
    existing, err := s.getIdempRecord(ctx, idempKey)
    if err != nil && !errors.Is(err, pgx.ErrNoRows) {
        return nil, err
    }

    reqHash := hashRequest(req)

    if existing != nil {
        // Ключ уже существует
        if existing.RequestHash != reqHash {
            // Тот же ключ, другой запрос — конфликт
            return nil, ErrIdempotencyConflict
        }
        if existing.CompletedAt == nil {
            // Операция ещё выполняется (concurrent request)
            return nil, ErrRequestInProgress
        }
        // Вернуть кешированный результат
        return &PaymentResult{
            Payment:   deserialize(existing.Response),
            WasCached: true,
        }, nil
    }

    // 2. Создать запись (in-progress)
    if err := s.insertIdempRecord(ctx, idempKey, reqHash); err != nil {
        if isUniqueViolation(err) {
            // Конкурентный запрос создал запись — retry
            return nil, ErrRequestInProgress
        }
        return nil, err
    }

    // 3. Выполнить операцию
    payment, err := s.doCreatePayment(ctx, req)
    if err != nil {
        // Удалить запись чтобы позволить retry
        s.deleteIdempRecord(ctx, idempKey)
        return nil, err
    }

    // 4. Сохранить результат
    responseBytes, _ := json.Marshal(payment)
    if err := s.completeIdempRecord(ctx, idempKey, responseBytes, http.StatusCreated); err != nil {
        // Не критично — операция выполнена, следующий retry вернёт результат или выполнит снова
        // (зависит от требований)
    }

    return &PaymentResult{Payment: payment, WasCached: false}, nil
}

func (s *PaymentService) insertIdempRecord(ctx context.Context, key, reqHash string) error {
    _, err := s.db.Exec(ctx, `
        INSERT INTO idempotency_keys (key, request_hash)
        VALUES ($1, $2)
    `, key, reqHash)
    return err
}

func (s *PaymentService) completeIdempRecord(ctx context.Context, key string, response []byte, statusCode int) error {
    _, err := s.db.Exec(ctx, `
        UPDATE idempotency_keys
        SET response = $1, status_code = $2, completed_at = NOW()
        WHERE key = $3
    `, response, statusCode, key)
    return err
}
```

### Транзакция + idempotency key в одной операции

Для атомарности: сохранить ключ и результат операции в одной транзакции:

```sql
BEGIN;
  -- Вставить idempotency record
  INSERT INTO idempotency_keys (key, request_hash, completed_at)
  VALUES ($1, $2, NOW())
  ON CONFLICT (key) DO NOTHING;

  -- Если вставка не произошла (дубль) — откатить
  -- SELECT ... FOR UPDATE чтобы прочитать существующий

  -- Выполнить бизнес-операцию
  INSERT INTO payments (id, user_id, amount) VALUES (...)
  RETURNING id, amount, created_at;

  -- Сохранить ответ
  UPDATE idempotency_keys SET response = $3 WHERE key = $1;
COMMIT;
```

---

## At-most-once vs At-least-once vs Exactly-once

| Семантика | Поведение | Риск |
|---|---|---|
| At-most-once | отправить один раз, не retry | потеря сообщения |
| At-least-once | retry до подтверждения | дубли |
| Exactly-once | ровно один раз | сложно, требует idempotency |

**Exactly-once** в распределённых системах — не примитив, а результат комбинации:
- At-least-once delivery (retry до получения ack)
- Idempotent receiver (дедупликация дублей на стороне получателя)

```
Exactly-once = At-least-once + Idempotent consumer
```

---

## Idempotency в message queues

При обработке сообщений из Kafka / RabbitMQ / SQS — всегда assume at-least-once:

```go
func (h *PaymentEventHandler) Handle(ctx context.Context, msg kafka.Message) error {
    var event PaymentEvent
    if err := json.Unmarshal(msg.Value, &event); err != nil {
        return err  // skip bad message
    }

    // Дедупликация по event ID
    processed, err := h.repo.IsProcessed(ctx, event.ID)
    if err != nil {
        return err
    }
    if processed {
        return nil  // уже обработано — skip
    }

    // Обработать + пометить как processed в одной транзакции
    return h.repo.WithTx(ctx, func(tx pgx.Tx) error {
        if err := processPaymentEvent(ctx, tx, event); err != nil {
            return err
        }
        return markProcessed(ctx, tx, event.ID)
    })
}
```

```sql
-- Таблица processed events
CREATE TABLE processed_events (
    event_id    TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Антипаттерны

**Клиент повторно использует один ключ для разных операций** — один idempotency key на всю сессию или на тип операции. Ключ должен быть уникален для каждой логической операции.

**Не хранить хэш запроса** — без `request_hash` нельзя обнаружить конфликт (один ключ, разные данные) и вернуть ошибку вместо неправильного результата.

**Бесконечное хранение idempotency keys** — диск заполнится. TTL 24-48 часов достаточно для большинства retry-окон.

**Idempotency key в теле запроса, а не в header** — тело может быть прочитано один раз. Header доступен в middleware для ранней дедупликации.

**Делать операцию idempotent через SELECT + INSERT** — это race condition без транзакции:
```go
// плохо — race condition
exists, _ := repo.Exists(ctx, key)
if !exists {
    repo.Insert(ctx, key, data)  // два конкурентных запроса оба пройдут сюда
}

// хорошо — атомарно
_, err := db.Exec(ctx, `
    INSERT INTO idempotency_keys (key) VALUES ($1)
    ON CONFLICT (key) DO NOTHING
`, key)
// Проверить был ли INSERT или CONFLICT
```
