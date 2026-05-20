# Saga и Outbox: distributed transactions

В монолите с одной БД транзакция атомарна: или все изменения применились, или ни одно. В микросервисах **этой гарантии нет**. Сервис A списал деньги, должен сказать сервису B "отправь товар". Что если A списал, но событие не дошло до B? Деньги ушли, товар не отправлен.

Эта проблема — **самая важная** в практической distributed systems. Решается двумя паттернами, которые **обычно используются вместе**:

- **Outbox pattern** — надёжная доставка событий из одного сервиса в другие. "Если я записал в БД — значит, событие гарантированно будет опубликовано."
- **Saga pattern** — координация **logical** транзакции через несколько сервисов с откатом при ошибках. "Если что-то пошло не так на шаге 5, шаги 1-4 откатываются (компенсируются)."

Это не альтернативы, а **слои**. Saga — высокоуровневая логика. Outbox — низкоуровневая гарантия доставки между шагами.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Проблема: dual write](#проблема-dual-write)
- [Почему 2PC обычно не работает](#почему-2pc-обычно-не-работает)
- [Outbox pattern: идея](#outbox-pattern-идея)
- [Outbox: полная реализация в Go](#outbox-полная-реализация-в-go)
- [At-least-once: дубликаты неизбежны](#at-least-once-дубликаты-неизбежны)
- [Inbox pattern: consumer-side дедупликация](#inbox-pattern-consumer-side-дедупликация)
- [CDC как альтернатива polling](#cdc-как-альтернатива-polling)
- [Saga pattern: идея](#saga-pattern-идея)
- [Choreography saga](#choreography-saga)
- [Orchestration saga](#orchestration-saga)
- [Compensation logic](#compensation-logic)
- [Saga + Outbox: как они комбинируются](#saga-outbox-как-они-комбинируются)
- [Temporal.io: готовое решение](#temporalio-готовое-решение)
- [Подводные камни](#подводные-камни)
- [Когда что выбирать](#когда-что-выбирать)

См. также: [06-databases/relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md](../../06-databases/relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md) — короткая interview-шпаргалка по outbox в контексте платежей.

---

## Простая аналогия

Представь онлайн-заказ еды:
1. С твоего счёта списать $30
2. Ресторан получает заказ
3. Курьер назначается
4. Тебе приходит SMS с ETA

Каждый шаг — отдельный сервис. Что если на шаге 3 не нашли курьеров? Деньги списаны, ресторан уже готовит, но доставить некому.

**Без правильного дизайна:** деньги уйдут, еда приготовится, курьера не будет, ты будешь звонить в поддержку. Бизнес выглядит плохо, клиент злой.

**С Saga:** при неудаче на шаге 3 — **откатываются** шаги 1 и 2: вернуть деньги, отменить заказ в ресторане. Полная "транзакция" либо выполняется до конца, либо откатывается.

**С Outbox:** между шагами 1-2-3 события передаются **надёжно**. Если шаг 1 закоммитил DB изменения (списал деньги), событие "deposit_done" **гарантированно** дойдёт до сервиса заказов. Не теряется при сетевых сбоях, рестартах, всём чём угодно.

Saga — это **логика** distributed transaction. Outbox — это **транспорт**, на котором она работает.

---

## Проблема: dual write

Самая частая ошибка новичка в микросервисах:

```go
// АНТИ-ПАТТЕРН: dual write
func chargePayment(orderID string, amount float64) error {
    // 1. Записать в БД
    if err := db.Exec("INSERT INTO payments ...", ...); err != nil {
        return err
    }

    // 2. Опубликовать событие в Kafka
    if err := kafka.Publish("payment-completed", event); err != nil {
        // Что делать?? БД уже записала, а Kafka — нет
        return err
    }

    return nil
}
```

**Что может пойти не так:**

| Сценарий | Что происходит |
|---|---|
| Сервис упал между шагом 1 и 2 | БД записала, событие не опубликовано → downstream сервисы не знают о платеже |
| Kafka недоступна | Шаг 2 fails, шаг 1 уже committed → данные расходятся |
| Сеть нестабильна | Шаг 2 может частично доставиться, retry дублирует |
| БД rollback после publish | Событие опубликовано, но платежа нет → downstream обрабатывает несуществующий платёж |

Это **не теоретические сценарии**. На production-сервисе при 1000 RPS это произойдёт **много раз в день**.

**Корень:** две системы (БД и broker) не имеют общей транзакции. Между шагом 1 и шагом 2 — окно, в котором сервис может упасть.

### Очевидное "решение" которое не работает

```go
// А может сначала publish, потом commit?
func chargePayment(orderID string, amount float64) error {
    if err := kafka.Publish("payment-completed", event); err != nil {
        return err
    }
    return db.Exec("INSERT INTO payments ...", ...)
}
```

Ещё хуже. Опубликовали событие "payment-completed", но БД-запись могла не пройти. Downstream думает что платёж есть. Worst case.

---

## Почему 2PC обычно не работает

**2PC (Two-Phase Commit)** — классическое решение distributed transactions в академической литературе. Координатор спрашивает всех participants "готов commit?", потом если все ОК — "commit". Атомарно.

**Звучит идеально.** Реально не используется в современных микросервисах. Почему:

**1. Blocking protocol.** Если координатор упал между phase 1 и phase 2, participants блокируют ресурсы пока координатор не очнётся. В реальной системе с тысячами транзакций — приговор.

**2. Не работает с большинством message brokers.** Kafka, RabbitMQ, Redis, AWS SQS — никто не поддерживает XA transactions. 2PC требует XA-совместимых ресурсов.

**3. Низкая throughput.** Локная производительность × число участников. Для high-throughput системы — недопустимо.

**4. Coupling.** Все participants должны быть available одновременно. Если один сервис тормозит — все стоят.

**5. Single point of failure.** Координатор. Если он упал — все висят.

В банковских системах с двумя БД того же провайдера (Oracle XA) 2PC ещё встречается. В микросервисах с разнородным storage — практически нет.

**Современный подход:** принять что атомарной транзакции нет, и проектировать **eventually consistent** систему через Saga + Outbox.

---

## Outbox pattern: идея

**Цель:** "если я записал X в БД, то событие про X гарантированно будет опубликовано — независимо от того, упал сервис, broker, или сеть."

**Идея:**
1. Не публикуем напрямую в broker
2. Записываем событие в **специальную таблицу "outbox"** в той же БД
3. Запись в outbox **и** business данные — в **одной транзакции**
4. Отдельный **relay worker** периодически читает outbox и публикует в broker
5. После успешной публикации — помечает запись как "sent"

```
┌─────────── Service ────────────┐
│                                 │
│  ┌─ Business transaction ─┐    │
│  │  1. INSERT INTO orders │    │   ←─ всё в одной TX
│  │  2. INSERT INTO outbox │    │
│  │  COMMIT                │    │
│  └────────────────────────┘    │
│                                 │
│  ┌─ Relay worker (фоновый) ┐   │
│  │  Каждые N мс:           │   │
│  │  - SELECT outbox WHERE  │   │
│  │    sent_at IS NULL       │   │
│  │  - Publish to broker    │   │   ──→  Kafka / RabbitMQ
│  │  - UPDATE sent_at        │   │
│  └─────────────────────────┘   │
└─────────────────────────────────┘
```

**Ключ:** транзакция БД атомарна. Либо и order, и outbox записаны (значит, событие будет опубликовано), либо ни одно (значит, и order не создан).

**Между записью и публикацией** — окно. Но это окно **рестартоустойчиво**: после рестарта relay worker подберёт unsent записи и опубликует.

---

## Outbox: полная реализация в Go

### Схема таблицы

```sql
CREATE TABLE outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL,       -- "order", "payment", и т.д.
    aggregate_id TEXT NOT NULL,         -- ID сущности
    event_type TEXT NOT NULL,           -- "OrderCreated", "PaymentCompleted"
    payload JSONB NOT NULL,             -- сам event
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,                -- NULL = не отправлено
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT
);

-- Индекс для быстрого поиска неотправленных
CREATE INDEX outbox_unsent_idx ON outbox (created_at) WHERE sent_at IS NULL;
```

### Saver — запись в outbox в той же транзакции

```go
package outbox

import (
    "context"
    "encoding/json"
    "github.com/jackc/pgx/v5"
)

type Event struct {
    AggregateType string
    AggregateID   string
    EventType     string
    Payload       any
}

// SaveInTx добавляет событие в outbox в рамках уже открытой транзакции
func SaveInTx(ctx context.Context, tx pgx.Tx, event Event) error {
    payload, err := json.Marshal(event.Payload)
    if err != nil {
        return err
    }

    _, err = tx.Exec(ctx, `
        INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ($1, $2, $3, $4)
    `, event.AggregateType, event.AggregateID, event.EventType, payload)

    return err
}
```

### Использование в business logic

```go
func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*Order, error) {
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)

    // 1. Business write
    order := &Order{
        ID:        uuid.New().String(),
        UserID:    req.UserID,
        Total:     req.Total,
        Status:    "pending",
        CreatedAt: time.Now(),
    }
    if _, err := tx.Exec(ctx, `
        INSERT INTO orders (id, user_id, total, status, created_at)
        VALUES ($1, $2, $3, $4, $5)
    `, order.ID, order.UserID, order.Total, order.Status, order.CreatedAt); err != nil {
        return nil, err
    }

    // 2. Outbox write в той же транзакции
    if err := outbox.SaveInTx(ctx, tx, outbox.Event{
        AggregateType: "order",
        AggregateID:   order.ID,
        EventType:     "OrderCreated",
        Payload: map[string]any{
            "order_id": order.ID,
            "user_id":  order.UserID,
            "total":    order.Total,
        },
    }); err != nil {
        return nil, err
    }

    // 3. Commit обоих изменений атомарно
    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }

    return order, nil
}
```

После commit'а **гарантировано**: либо order создан и событие в outbox (будет опубликовано), либо ничего из этого.

### Relay worker — polling и publish

```go
package outbox

import (
    "context"
    "log/slog"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type Publisher interface {
    Publish(ctx context.Context, topic string, payload []byte) error
}

type Relay struct {
    db        *pgxpool.Pool
    publisher Publisher
    interval  time.Duration
    batchSize int
}

func NewRelay(db *pgxpool.Pool, pub Publisher) *Relay {
    return &Relay{
        db:        db,
        publisher: pub,
        interval:  1 * time.Second,
        batchSize: 100,
    }
}

func (r *Relay) Run(ctx context.Context) {
    ticker := time.NewTicker(r.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := r.processBatch(ctx); err != nil {
                slog.Error("outbox: process batch", "err", err)
            }
        }
    }
}

func (r *Relay) processBatch(ctx context.Context) error {
    // Используем FOR UPDATE SKIP LOCKED — позволяет нескольким worker'ам работать параллельно
    rows, err := r.db.Query(ctx, `
        SELECT id, aggregate_type, aggregate_id, event_type, payload
        FROM outbox
        WHERE sent_at IS NULL
        ORDER BY created_at
        LIMIT $1
        FOR UPDATE SKIP LOCKED
    `, r.batchSize)
    if err != nil {
        return err
    }
    defer rows.Close()

    type pending struct {
        id            string
        topic         string
        aggregateID   string
        payload       []byte
    }

    var batch []pending
    for rows.Next() {
        var p pending
        var aggregateType, eventType string
        if err := rows.Scan(&p.id, &aggregateType, &p.aggregateID, &eventType, &p.payload); err != nil {
            return err
        }
        // Topic для broker, например, "orders.created"
        p.topic = aggregateType + "." + snake(eventType)
        batch = append(batch, p)
    }
    rows.Close()

    if len(batch) == 0 {
        return nil
    }

    // Публикуем по одной (можно делать batch publish для эффективности)
    sentIDs := make([]string, 0, len(batch))
    failedIDs := make(map[string]string)  // id → error
    for _, p := range batch {
        if err := r.publisher.Publish(ctx, p.topic, p.payload); err != nil {
            failedIDs[p.id] = err.Error()
            slog.Warn("outbox: publish failed", "id", p.id, "err", err)
            continue
        }
        sentIDs = append(sentIDs, p.id)
    }

    // Помечаем successfully published
    if len(sentIDs) > 0 {
        if _, err := r.db.Exec(ctx, `
            UPDATE outbox
            SET sent_at = NOW()
            WHERE id = ANY($1::uuid[])
        `, sentIDs); err != nil {
            return err
        }
    }

    // Инкремент attempt для failed (для visibility)
    for id, errMsg := range failedIDs {
        r.db.Exec(ctx, `
            UPDATE outbox
            SET attempts = attempts + 1, last_error = $1
            WHERE id = $2
        `, errMsg, id)
    }

    return nil
}

func snake(s string) string {
    // OrderCreated → order_created
    // ...упрощено для примера
    return s
}
```

**Ключевые моменты:**

1. **`FOR UPDATE SKIP LOCKED`** — несколько relay worker'ов могут работать параллельно. Каждый забирает свой набор строк, не блокирует друг друга. Критично для масштабирования.

2. **Batch processing** — за один цикл забираем до 100 событий. Меньше overhead'а на каждое.

3. **Sent tracking через `sent_at`** — не удаляем строки (для аудита), просто помечаем.

4. **Retry автоматический** — если publish failed, запись остаётся `sent_at IS NULL`, следующий цикл попробует снова.

5. **Attempts счётчик** — для visibility. Если событие застряло на 100+ попытках — операционная проблема.

6. **Periodic cleanup** — отдельный job удаляет старые отправленные записи (например, старше 7 дней):
   ```sql
   DELETE FROM outbox WHERE sent_at < NOW() - INTERVAL '7 days';
   ```

### Запуск

```go
func main() {
    db := connectDB()
    publisher := kafka.NewPublisher(...)

    relay := outbox.NewRelay(db, publisher)

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    go relay.Run(ctx)

    // ... HTTP server, etc.
    <-ctx.Done()
}
```

В production обычно — отдельный deployment с N pod'ами relay worker'а, рядом с основным сервисом или независимо.

---

## At-least-once: дубликаты неизбежны

После publish'а relay должен пометить `sent_at`. Что если:
1. Publish прошёл успешно
2. Сервис упал **до** UPDATE outbox

Следующий цикл найдёт ту же запись, опубликует **снова**. **Дубликат.**

Это фундаментальное свойство — **at-least-once delivery**. С outbox **невозможно** гарантировать exactly-once без дополнительной дедупликации на стороне consumer'а.

Поэтому каждое событие должно содержать **уникальный ID**, и consumer должен дедуплицировать (см. Inbox pattern ниже).

```go
event := outbox.Event{
    Payload: map[string]any{
        "event_id": uuid.New().String(),  // ← consumer'у для дедупликации
        // ... остальные поля
    },
}
```

---

## Inbox pattern: consumer-side дедупликация

**Inbox** — зеркальное решение для consumer'а. Записываем ID полученного события в БД, **прежде чем** обрабатывать. Если уже было — пропускаем.

### Схема таблицы

```sql
CREATE TABLE inbox (
    event_id UUID PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Consumer

```go
func (h *OrderHandler) HandleOrderCreated(ctx context.Context, msg Message) error {
    var event OrderCreatedEvent
    if err := json.Unmarshal(msg.Payload, &event); err != nil {
        return err
    }

    tx, err := h.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    // 1. Попытка вставить в inbox
    _, err = tx.Exec(ctx, `
        INSERT INTO inbox (event_id) VALUES ($1)
        ON CONFLICT (event_id) DO NOTHING
    `, event.EventID)
    if err != nil {
        return err
    }

    // 2. Проверить — был ли это новый event или дубль
    var alreadyProcessed bool
    err = tx.QueryRow(ctx, `
        SELECT TRUE FROM inbox
        WHERE event_id = $1 AND received_at < NOW() - INTERVAL '1 second'
    `, event.EventID).Scan(&alreadyProcessed)

    if alreadyProcessed {
        // Уже обрабатывали — просто ack без работы
        return tx.Commit(ctx)
    }

    // 3. Обработать event (например, создать запись в локальной БД)
    if _, err := tx.Exec(ctx, `
        INSERT INTO order_replica (id, user_id, total, status)
        VALUES ($1, $2, $3, $4)
    `, event.OrderID, event.UserID, event.Total, event.Status); err != nil {
        return err
    }

    return tx.Commit(ctx)
}
```

**Идея:**
- Inbox + business write в одной транзакции
- При retry того же события — INSERT в inbox fails (PRIMARY KEY conflict), мы знаем что обработано
- Если transaction failed после inbox-insert но до бизнес-логики — следующий retry увидит inbox запись и пропустит, **но business данных не появится**

Чтобы это работало правильно — inbox INSERT и business updates **должны быть в одной транзакции**. Иначе можно зафиксировать inbox без applied изменений.

### Альтернативный вариант с idempotency key в business таблице

Иногда не делают отдельную inbox таблицу, а используют **уникальный constraint** в business-таблице:

```sql
CREATE TABLE order_replica (
    id UUID PRIMARY KEY,
    source_event_id UUID UNIQUE,  -- ← дедупликация по event ID
    ...
);

INSERT INTO order_replica (id, source_event_id, ...)
VALUES (...)
ON CONFLICT (source_event_id) DO NOTHING;
```

Проще, но работает только для simple "create" операций. Для сложной логики (update + sideeffects) лучше явный inbox.

---

## CDC как альтернатива polling

Polling outbox таблицы каждую секунду — простой подход, но имеет минусы:
- Latency 0-N миллисекунд (по среднему — N/2)
- Нагрузка на БД (постоянные SELECT'ы)
- Тяжело сделать sub-100ms latency без агрессивного polling

**Альтернатива: CDC (Change Data Capture)** — читать **WAL** (Write-Ahead Log) Postgres напрямую и эмитить события без polling.

### Debezium

[Debezium](https://debezium.io/) — самое популярное CDC решение. Подписывается на logical replication в Postgres, читает все INSERT/UPDATE/DELETE и публикует в Kafka.

**Идея:**
1. Создаёшь logical replication slot в Postgres
2. Debezium читает changes из этого слота
3. Каждый INSERT в outbox таблицу → message в Kafka **мгновенно**

```yaml
# Debezium config для outbox таблицы
{
  "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
  "database.hostname": "postgres",
  "database.dbname": "myapp",
  "table.include.list": "public.outbox",
  "transforms": "outbox",
  "transforms.outbox.type": "io.debezium.transforms.outbox.EventRouter",
  "transforms.outbox.route.by.field": "aggregate_type",
  "transforms.outbox.table.field.event.payload": "payload",
  "transforms.outbox.table.field.event.id": "id"
}
```

Debezium имеет **специальный transformer для outbox pattern** — routes events в правильные Kafka topics, payload как тело message.

**Плюсы CDC:**
- Sub-millisecond latency
- Нет нагрузки от polling на БД
- Не нужно code'ить relay worker

**Минусы:**
- **Operational complexity** — нужно поддерживать Debezium кластер
- Logical replication имеет свои подводные камни (slot накапливает WAL если не consumed)
- Привязка к Postgres feature set

**Когда выбирать CDC:**
- Sub-second latency критична
- High throughput (тысячи событий в секунду)
- Уже используется Kafka Connect инфраструктура

**Когда polling proще:**
- Латенси 1-5 секунд OK
- Маленькая команда без DevOps capacity на Debezium
- Простой setup приоритетен

---

## Saga pattern: идея

Outbox решает **доставку события** надёжно. Saga решает **что делать когда события вызывают цепочку действий и одно из них fails**.

**Пример:** оформление заказа.

```
1. PaymentService: списать деньги
2. InventoryService: зарезервировать товар
3. ShippingService: создать доставку
4. NotificationService: отправить SMS
```

Что если на шаге 2 нет товара? Деньги уже списаны. **Saga должна откатить шаг 1** — вернуть деньги.

Saga — это **последовательность шагов с компенсациями**. Каждый шаг имеет:
- **Forward action** — основное действие ("списать деньги")
- **Compensating action** — отмена ("вернуть деньги")

Если шаг N fails — выполняются compensations для шагов 1..N-1 в обратном порядке.

### Eventual consistency

Saga **не атомарна**. В середине процесса система **видна** в "промежуточном" состоянии: деньги списаны, но товар ещё не зарезервирован. Это **по дизайну** — мы принимаем eventually consistent как компромисс за то, что 2PC не работает.

UI должен это учитывать: показывать "обработка", не давать сделать действие дважды, и т.д.

### Два подхода: choreography vs orchestration

**Choreography** — сервисы общаются **через события**, никто не "управляет" процессом. Каждый знает что делать на свои события.

**Orchestration** — центральный **orchestrator** говорит каждому сервису что делать в каком порядке.

Оба паттерна валидны, у каждого свои trade-offs.

---

## Choreography saga

Каждый сервис подписан на события и реагирует. Логика "что после чего" распределена.

```
[Order created]
   │
   ▼
PaymentService → [Payment completed]
   │                    │
   │                    ▼
   │            InventoryService → [Inventory reserved]
   │                                       │
   │                                       ▼
   │                          ShippingService → [Shipment created]
   │
   └─ (если payment failed) → [Order cancelled]
```

### Реализация

Каждый сервис:

**PaymentService:**
```go
func (s *PaymentService) HandleOrderCreated(ctx context.Context, event OrderCreated) error {
    return s.db.Tx(ctx, func(tx pgx.Tx) error {
        if err := s.charge(ctx, tx, event.OrderID, event.Amount); err != nil {
            // Опубликовать compensating event
            return outbox.SaveInTx(ctx, tx, outbox.Event{
                EventType: "PaymentFailed",
                Payload:   PaymentFailed{OrderID: event.OrderID, Reason: err.Error()},
            })
        }

        return outbox.SaveInTx(ctx, tx, outbox.Event{
            EventType: "PaymentCompleted",
            Payload:   PaymentCompleted{OrderID: event.OrderID, Amount: event.Amount},
        })
    })
}
```

**InventoryService:**
```go
func (s *InventoryService) HandlePaymentCompleted(ctx context.Context, event PaymentCompleted) error {
    return s.db.Tx(ctx, func(tx pgx.Tx) error {
        if err := s.reserve(ctx, tx, event.OrderID, event.Items); err != nil {
            return outbox.SaveInTx(ctx, tx, outbox.Event{
                EventType: "InventoryReservationFailed",
                Payload:   InventoryReservationFailed{OrderID: event.OrderID, Reason: err.Error()},
            })
        }

        return outbox.SaveInTx(ctx, tx, outbox.Event{
            EventType: "InventoryReserved",
            Payload:   InventoryReserved{OrderID: event.OrderID},
        })
    })
}

// Compensation: если payment failed после inventory reserved
func (s *InventoryService) HandlePaymentFailed(ctx context.Context, event PaymentFailed) error {
    // Освободить резерв (если был)
    return s.release(ctx, event.OrderID)
}
```

**PaymentService** также подписан на InventoryReservationFailed — чтобы вернуть деньги:
```go
func (s *PaymentService) HandleInventoryReservationFailed(ctx context.Context, event InventoryReservationFailed) error {
    return s.refund(ctx, event.OrderID)
}
```

### Плюсы choreography

- **Loose coupling** — сервисы не знают друг о друге, только о событиях
- **No SPOF** — нет центрального координатора
- **Естественно для event-driven архитектур**

### Минусы

- **Логика распределена** — чтобы понять весь flow, читаешь код 5 сервисов
- **Сложно отлаживать** — где именно затрял процесс?
- **Cyclical dependencies** — сервисы должны знать чужие compensation events
- **Тяжело менять flow** — добавить новый шаг = изменения в нескольких сервисах
- **Невозможно явно "паузить" saga** — она расщеплена по событиям

Choreography подходит для **2-3 шагов** и **stable** процессов. Для сложных — становится спагетти.

---

## Orchestration saga

Центральный **orchestrator** управляет процессом. Каждый сервис делает только то что ему скажут.

```
[Orchestrator]
    │
    ├─ Step 1 → PaymentService.Charge()
    │              ← success/failure
    │
    ├─ Step 2 → InventoryService.Reserve()
    │              ← success/failure
    │
    ├─ Step 3 → ShippingService.CreateShipment()
    │
    └─ On failure → запустить compensations в обратном порядке
```

### Реализация

Orchestrator хранит **state machine** для каждой saga instance:

```sql
CREATE TABLE order_saga (
    id UUID PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    current_step TEXT NOT NULL,        -- "payment", "inventory", "shipping", "completed", "compensating", "failed"
    state JSONB NOT NULL,              -- persisted state машины
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

Orchestrator-код:

```go
type OrderSaga struct {
    db       *pgxpool.Pool
    payment  PaymentClient
    inventory InventoryClient
    shipping ShippingClient
}

type SagaState struct {
    OrderID     string
    Amount      float64
    Items       []Item
    PaymentID   string  // заполняется после payment
    Reserved    bool    // заполняется после inventory
    ShipmentID  string  // после shipping
}

func (s *OrderSaga) Start(ctx context.Context, orderID string, amount float64, items []Item) error {
    state := SagaState{OrderID: orderID, Amount: amount, Items: items}
    stateJSON, _ := json.Marshal(state)

    _, err := s.db.Exec(ctx, `
        INSERT INTO order_saga (id, order_id, current_step, state)
        VALUES (gen_random_uuid(), $1, 'payment', $2)
    `, orderID, stateJSON)
    if err != nil {
        return err
    }

    // Запустить async обработку
    go s.process(ctx, orderID)
    return nil
}

func (s *OrderSaga) process(ctx context.Context, orderID string) {
    for {
        // Загрузить текущее состояние
        var step string
        var stateJSON []byte
        err := s.db.QueryRow(ctx, `
            SELECT current_step, state FROM order_saga WHERE order_id = $1
        `, orderID).Scan(&step, &stateJSON)
        if err != nil {
            slog.Error("saga: load state", "err", err)
            return
        }

        var state SagaState
        json.Unmarshal(stateJSON, &state)

        switch step {
        case "payment":
            s.doPayment(ctx, &state)
        case "inventory":
            s.doInventory(ctx, &state)
        case "shipping":
            s.doShipping(ctx, &state)
        case "completed":
            return  // success
        case "compensating_inventory":
            s.compensateInventory(ctx, &state)
        case "compensating_payment":
            s.compensatePayment(ctx, &state)
        case "failed":
            return  // конечное состояние
        }
    }
}

func (s *OrderSaga) doPayment(ctx context.Context, state *SagaState) {
    paymentID, err := s.payment.Charge(ctx, state.OrderID, state.Amount)
    if err != nil {
        s.transition(ctx, state.OrderID, "failed", state, err.Error())
        return
    }

    state.PaymentID = paymentID
    s.transition(ctx, state.OrderID, "inventory", state, "")
}

func (s *OrderSaga) doInventory(ctx context.Context, state *SagaState) {
    err := s.inventory.Reserve(ctx, state.OrderID, state.Items)
    if err != nil {
        // Compensation: вернуть деньги
        s.transition(ctx, state.OrderID, "compensating_payment", state, err.Error())
        return
    }

    state.Reserved = true
    s.transition(ctx, state.OrderID, "shipping", state, "")
}

func (s *OrderSaga) doShipping(ctx context.Context, state *SagaState) {
    shipmentID, err := s.shipping.CreateShipment(ctx, state.OrderID)
    if err != nil {
        // Compensation: освободить inventory + вернуть деньги
        s.transition(ctx, state.OrderID, "compensating_inventory", state, err.Error())
        return
    }

    state.ShipmentID = shipmentID
    s.transition(ctx, state.OrderID, "completed", state, "")
}

// Compensations
func (s *OrderSaga) compensateInventory(ctx context.Context, state *SagaState) {
    if state.Reserved {
        s.inventory.Release(ctx, state.OrderID)
        state.Reserved = false
    }
    s.transition(ctx, state.OrderID, "compensating_payment", state, "")
}

func (s *OrderSaga) compensatePayment(ctx context.Context, state *SagaState) {
    if state.PaymentID != "" {
        s.payment.Refund(ctx, state.PaymentID)
        state.PaymentID = ""
    }
    s.transition(ctx, state.OrderID, "failed", state, "")
}

func (s *OrderSaga) transition(ctx context.Context, orderID, newStep string, state *SagaState, errMsg string) {
    stateJSON, _ := json.Marshal(state)
    s.db.Exec(ctx, `
        UPDATE order_saga
        SET current_step = $1, state = $2, updated_at = NOW()
        WHERE order_id = $3
    `, newStep, stateJSON, orderID)
}
```

### Recovery после рестарта

После рестарта сервиса — найди все saga в "in-progress" состоянии и продолжи:

```go
func (s *OrderSaga) RecoverPending(ctx context.Context) error {
    rows, err := s.db.Query(ctx, `
        SELECT order_id FROM order_saga
        WHERE current_step NOT IN ('completed', 'failed')
    `)
    if err != nil {
        return err
    }
    defer rows.Close()

    for rows.Next() {
        var orderID string
        rows.Scan(&orderID)
        go s.process(ctx, orderID)
    }
    return nil
}
```

При старте сервиса — `RecoverPending` поднимает все недозавершённые saga.

### Плюсы orchestration

- **Логика в одном месте** — легко читать и понимать flow
- **Easy to modify** — добавил шаг, изменил один файл
- **Visible state** — `SELECT * FROM order_saga` показывает прогресс
- **Easy to debug** — видно где saga застряла
- **Timeouts and retries** — централизованная политика

### Минусы

- **Coupling** — orchestrator знает про API всех сервисов
- **SPOF** — если orchestrator умер, всё стоит (mitigation: replicas + state in DB)
- **Сложнее для event-driven архитектур** — orchestrator делает RPC, не subscribes

**В большинстве enterprise сценариев orchestration выигрывает** благодаря visibility.

---

## Compensation logic

Compensation — это **не undo**, это **семантическое противодействие**.

**Простой случай:** payment → refund. Симметрично.

**Сложный случай:** "email sent" — невозможно "unsend". Compensation = send another email "извините, заказ отменён".

### Правила хорошей compensation

**1. Идемпотентная.**
Compensation может вызваться несколько раз (after retry). Каждый раз — одинаковый результат.

```go
func refund(paymentID string) error {
    // Сначала проверить — уже refunded?
    if payment, _ := getPayment(paymentID); payment.Refunded {
        return nil  // idempotent
    }
    return processRefund(paymentID)
}
```

**2. Не может fail силами бизнес-логики.**
Если refund fail'ит из-за expired credit card — что делать? Saga **зависнет в compensation**. Нужен human escalation.

В реальности: **compensation failures escalate to manual review**. Очередь "failed compensations" → ops/support обрабатывает.

**3. Communicates intent.**
Compensation должна быть видна в системе. UI показывает "deze order cancelled and refunded", не молча возвращает деньги.

### Backwards recovery vs Forwards recovery

**Backwards (классическая saga):** failure → откат всего сделанного.

**Forwards:** failure → попытка alternative path. Например, payment failed → попробуй другой payment method.

Forwards чаще на практике у user-facing flows (UX лучше: "выберите другую карту"), backwards — для технических процессов.

---

## Saga + Outbox: как они комбинируются

Saga и Outbox — **разные слои**:
- **Outbox** обеспечивает что событие, опубликованное в saga, **гарантированно** доставится
- **Saga** обеспечивает что **последовательность** событий приводит к согласованному состоянию

В реальной системе они комбинируются:

```
Orchestrator делает шаг:
   1. Begin TX
   2. Call payment service (RPC)
   3. Update saga state в БД
   4. Outbox INSERT "PaymentCompleted" (если нужно нотифицировать downstream)
   5. Commit TX

Outbox relay worker отдельно публикует событие.
```

Или choreography saga **вся работает через outbox**:

```
PaymentService:
   1. Begin TX
   2. Charge payment (update local DB)
   3. Outbox INSERT "PaymentCompleted"
   4. Commit

InventoryService (отдельно):
   1. Consume event PaymentCompleted
   2. Begin TX
   3. Reserve inventory + Inbox check
   4. Outbox INSERT "InventoryReserved"
   5. Commit

И так далее.
```

В этом случае каждое событие проходит через outbox + inbox, обеспечивая надёжную доставку, а saga-логика "что после чего" — естественная цепочка событий.

---

## Temporal.io: готовое решение

Писать orchestrator руками — много работы: state machine, persistence, recovery, retries, timeouts. Существуют готовые workflow engines.

**[Temporal.io](https://temporal.io/)** — один из самых популярных. Написан в Uber, открытый source.

Идея: пишешь "workflow code" как обычные функции, Temporal обеспечивает **durability**.

```go
// workflow.go
func OrderWorkflow(ctx workflow.Context, orderID string, amount float64) error {
    // Шаг 1: payment
    var paymentID string
    err := workflow.ExecuteActivity(ctx, ChargePayment, orderID, amount).Get(ctx, &paymentID)
    if err != nil {
        return fmt.Errorf("payment failed: %w", err)
    }

    // Шаг 2: inventory
    err = workflow.ExecuteActivity(ctx, ReserveInventory, orderID).Get(ctx, nil)
    if err != nil {
        // Compensate
        workflow.ExecuteActivity(ctx, RefundPayment, paymentID)
        return fmt.Errorf("inventory failed: %w", err)
    }

    // Шаг 3: shipping
    var shipmentID string
    err = workflow.ExecuteActivity(ctx, CreateShipment, orderID).Get(ctx, &shipmentID)
    if err != nil {
        // Compensate
        workflow.ExecuteActivity(ctx, ReleaseInventory, orderID)
        workflow.ExecuteActivity(ctx, RefundPayment, paymentID)
        return fmt.Errorf("shipping failed: %w", err)
    }

    return nil
}

// activities.go
func ChargePayment(ctx context.Context, orderID string, amount float64) (string, error) {
    // Обычная функция — Temporal сам обеспечит retry, persistence, recovery
    return paymentClient.Charge(ctx, orderID, amount)
}
```

Temporal:
- **Persists state** автоматически (нет ручного state machine)
- **Retries** activities автоматически с настраиваемой политикой
- **Survives crashes** — workflow продолжается с того же места после рестарта сервиса
- **Observability** — UI для visual debugging workflow'ов
- **Timeouts** — на activities и workflow целиком
- **Signals** — внешние события (например, "пользователь отменил" → workflow реагирует)

**Минусы:**
- Operational overhead — нужно поднимать и поддерживать Temporal сервер
- Code looks like normal Go but has **non-obvious constraints** (определенный код deterministic must be — нельзя `time.Now()`, надо `workflow.Now()`)
- Vendor-ish lock-in (хотя open-source)

**Альтернативы:** Cadence (предшественник Temporal), Zeebe, AWS Step Functions, Conductor.

**Когда использовать workflow engine:**
- Много сложных, долгоживущих процессов (часы, дни)
- Нужна visibility / debug
- Команда не хочет писать orchestration code сама

**Когда custom orchestrator OK:**
- 1-2 простых saga
- Лимит операционных ресурсов
- Уже есть простая state machine инфраструктура

---

## Подводные камни

**1. Non-commutative compensations.**
Compensations должны быть в **обратном порядке**. Если порядок не важен для compensation (а он часто важен) — bugs.

**2. Compensation для side effects.**
Email sent — нельзя undo. Push notification — то же. Compensation должна быть semantic, не technical.

**3. Concurrent saga instances.**
Два пользователя одновременно бронируют последний билет. Saga 1 reserves, saga 2 reserves... обе видят "success". Нужны **locking** или **optimistic concurrency** на ресурсе.

**4. Saga timeout.**
Saga зависла, не доделалась. Без timeout — она будет висеть вечно. Нужен timeout и решение что делать (compensate или alert).

**5. Idempotency на каждом шаге.**
Любой шаг может ретраи. Если шаг не идемпотентен — duplicate side effects.

**6. Outbox table растёт неконтролируемо.**
Без cleanup — миллионы строк → slow queries → проблема для основной БД. Регулярный cleanup обязателен.

**7. Polling latency vs БД нагрузка.**
Polling раз в 100ms = latency хорошая, нагрузка большая. Раз в 5 секунд — наоборот. Балансируй.

**8. Lock contention на outbox.**
Без `FOR UPDATE SKIP LOCKED` — два relay worker'а блокируют друг друга. Производительность падает.

**9. Ordering гарантии.**
Если два события в outbox должны быть доставлены **по порядку** — не гарантировано! Параллельные relay workers могут отправить out of order. Решение: ordering key (partition в Kafka by aggregate_id) + сериализация per-aggregate.

**10. Eventual consistency assumed.**
UI / клиенты должны быть **дизайнены** под eventual consistency. "После charge показать success" — нельзя. Покажи "processing" пока не пришло "completed".

**11. Inbox table растёт.**
Так же как outbox — cleanup старых записей (после TTL когда дубликаты уже не придут).

**12. Compensation failed = manual.**
Если compensation не получается даже после retries — escalate to humans. Не делать silent failure.

---

## Когда что выбирать

### Используй Outbox если

- ✅ Любой production-grade сервис, публикующий события в broker
- ✅ Дубликаты приемлемы (consumer'ы идемпотентны)
- ✅ Latency 1-5 секунд OK
- ✅ Малая команда без DevOps на Debezium

### Используй CDC (Debezium) если

- ✅ Sub-second latency нужна
- ✅ Высокая throughput (тысячи событий/сек)
- ✅ Уже есть Kafka Connect инфраструктура

### Используй Inbox если

- ✅ Consumer обрабатывает events, и retry / duplicates возможны
- ✅ Side effects (запись в БД, external API call) не идемпотентны сами по себе

### Используй Choreography Saga если

- ✅ 2-3 simple шага
- ✅ Event-driven архитектура уже есть
- ✅ Слабая coupling важна
- ✅ Процесс не часто меняется

### Используй Orchestration Saga если

- ✅ 4+ шага, сложная логика
- ✅ Нужна visibility и debuggability
- ✅ Процесс часто меняется
- ✅ Бизнес-команда хочет видеть прогресс

### Используй Temporal/workflow engine если

- ✅ Много долгоживущих процессов (часы-дни)
- ✅ Нужны human-in-the-loop (waiting for approval, etc.)
- ✅ Команда готова поддерживать operational overhead

---

## Полезные ссылки

- ["Pattern: Saga"](https://microservices.io/patterns/data/saga.html) — Chris Richardson, классическое описание
- ["Pattern: Transactional Outbox"](https://microservices.io/patterns/data/transactional-outbox.html) — там же
- [Debezium docs](https://debezium.io/documentation/) — CDC implementation
- [Temporal docs](https://docs.temporal.io/) — workflow engine
- [Designing Data-Intensive Applications](https://www.oreilly.com/library/view/designing-data-intensive-applications/9781491903063/) — Martin Kleppmann, глубокий разбор distributed systems
- [Microservices Patterns](https://microservices.io/book) — Chris Richardson, полный обзор
- [eventuate.io](https://eventuate.io/) — Java framework для transactional messaging
