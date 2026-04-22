# Payment System

Разбор задачи "Спроектируй платёжную систему". Проверяет знание ACID транзакций в распределённых системах, idempotency, двойных списаний, reconciliation. Критично для fintech компаний.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Вопросы:
  - Это внутренняя система для маркетплейса или платёжный шлюз (как Stripe)?
  - Какие операции: charge, refund, payout?
  - Работа с внешними PSP (Stripe, PayPal) или напрямую с банками?
  - Нужна ли мультивалютность?
  - Recurring payments (подписки)?
  - Fraud detection — в scope?
```

**Договорились (scope):**
- Внутренняя система маркетплейса (как Amazon, Uber, Airbnb)
- Операции: charge пользователя, split между платформой и продавцом, payout продавцу
- Интеграция через внешние PSP (Stripe, PayPal как gateway)
- Мультивалютность: да (базовая конвертация по курсу)
- Базовая fraud prevention (rate limiting + velocity checks)

**Out of scope:** собственный card processing (нужна лицензия), криптовалюты, налоговая отчётность, recurring billing (усложняет на интервью).

### Нефункциональные требования

```
- TPS: 1000 транзакций/сек в пике
- Latency: ответ пользователю < 3 сек (включая внешний PSP вызов)
- Durability: потеря платежа недопустима. НИКОГДА.
- Idempotency: дублирование запроса = одно списание (не два!)
- Consistency: strong consistency для балансов (CAP: CP, не AP)
- Availability: 99.99% (4 минуты downtime/год)
- Audit log: каждая операция должна быть записана неизменяемо
- Compliance: PCI DSS (данные карт не хранить в открытом виде)
```

---

## Фаза 2: Оценка нагрузки

```
TPS:
  1000 транзакций/сек
  1000 × 86400 = 86.4M транзакций/день
  
Storage:
  1 транзакция: ~500 bytes (metadata + entries)
  86.4M × 500B = 43 GB/day
  43 GB × 365 × 7 лет хранения (compliance) = ~110 TB
  → PostgreSQL или специализированная финансовая DB

Балансы:
  1 запись на аккаунт: ~100 bytes
  10M аккаунтов × 100B = 1 GB → умещается в памяти DB

Внешние PSP вызовы:
  1000 TPS × 1 PSP call = 1000 external HTTP calls/sec
  Каждый PSP call: 200-1500ms (нестабильно!)
  → Нужен async flow для large-scale, но sync для UX
```

---

## Фаза 3: Ключевые концепции

Прежде чем архитектура — важные принципы для платёжных систем.

### Double-Entry Bookkeeping (двойная запись)

```
Принцип: каждая транзакция = две записи (дебет + кредит)
Сумма всех записей в системе = 0

Пример: Пользователь платит 100 RUB за заказ

  Entries:
    user_account (DEBIT):    -100.00 RUB
    merchant_account (CREDIT): +95.00 RUB  (95%, за вычетом комиссии)
    platform_account (CREDIT):  +5.00 RUB  (5% комиссия)
    
  Сумма: -100 + 95 + 5 = 0 ✓

Зачем:
  + Математически верифицируется: SUM(all entries) должна быть 0
  + При баге всегда видно что пошло не так
  + Стандарт бухгалтерии (GAAP)
```

### Idempotency — ключевое требование

```
Проблема без idempotency:
  1. Клиент → POST /payments (charge $100)
  2. Сервер: списал, но ответ потерялся (network error)
  3. Клиент: ответа не было → повторить!
  4. POST /payments снова → ВТОРОЕ СПИСАНИЕ!
  
Решение: Idempotency Key
  Клиент генерирует UUID ОДИН РАЗ для данного платежа
  POST /payments
    Idempotency-Key: "order-789-charge-attempt-1"
    Body: { "amount": 100, "currency": "RUB", ... }
  
  Сервер:
    IF EXISTS payment WHERE idempotency_key = ? AND status != FAILED:
      RETURN cached_response  // не делать повторно!
    ELSE:
      process payment → сохранить с idempotency_key
```

---

## Фаза 4: Архитектура

```
  Client
    │
    │ POST /payments (Idempotency-Key: X)
    ▼
┌─────────────────────────────────────────────────────────────┐
│                    Payment Service                          │
│                                                             │
│  ┌──────────────┐    ┌─────────────┐    ┌────────────────┐  │
│  │  Idempotency │    │  Payment    │    │   Ledger       │  │
│  │  Check       │───►│  Processor  │───►│   Service      │  │
│  └──────────────┘    └──────┬──────┘    └────────────────┘  │
│                             │                               │
│                    ┌────────▼────────┐                      │
│                    │  PSP Gateway    │                      │
│                    │  (Stripe/PP)    │                      │
│                    └────────┬────────┘                      │
│                             │                               │
└─────────────────────────────┼───────────────────────────────┘
                              │
    ┌─────────────────────────┼──────────────────────┐
    │                         │                      │
    ▼                         ▼                      ▼
┌──────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  PostgreSQL  │    │  Audit Log      │    │  Kafka          │
│  (ledger,    │    │  (append-only)  │    │  (events for    │
│   accounts)  │    │                 │    │   reconcile)    │
└──────────────┘    └─────────────────┘    └─────────────────┘
```

---

## Фаза 5: Deep Dive

### Payment Flow: Sync vs Async

**Sync flow (для простых случаев):**

```
POST /payments
  { "user_id": 123, "amount": 100, "currency": "RUB", "order_id": "ord-456" }

1. Idempotency check:
   SELECT * FROM payments WHERE idempotency_key = ? FOR UPDATE
   Если нашли: вернуть cached result

2. Fraud check:
   Velocity check: не > 5 транзакций за 1 мин для user_id?
   Amount check: не > 50K за раз?
   Если suspicious → REJECT

3. Reserve balance (pre-authorization):
   BEGIN TRANSACTION
     SELECT balance FROM accounts WHERE id = user_id FOR UPDATE
     IF balance < amount: ROLLBACK → insufficient funds
     UPDATE accounts SET balance = balance - amount, reserved = reserved + amount
       WHERE id = user_id
     INSERT INTO payments (id, user_id, amount, status = PENDING, idempotency_key)
   COMMIT

4. Call external PSP (Stripe):
   POST https://api.stripe.com/v1/charges
   { "amount": 10000, "currency": "rub", "source": "card_token" }
   
   Response: { "id": "ch_xyz", "status": "succeeded" }

5. Complete payment:
   BEGIN TRANSACTION
     UPDATE payments SET status = COMPLETED, psp_id = "ch_xyz" WHERE id = ?
     UPDATE accounts SET reserved = reserved - amount WHERE id = user_id
     -- Double-entry ledger entries:
     INSERT INTO ledger_entries (account_id=user, amount=-100, type=DEBIT)
     INSERT INTO ledger_entries (account_id=merchant, amount=+95, type=CREDIT)
     INSERT INTO ledger_entries (account_id=platform, amount=+5, type=CREDIT)
   COMMIT

6. Publish event: Kafka topic=payment.completed

7. Return 200 { "payment_id": "...", "status": "COMPLETED" }
```

**Проблема sync flow:** PSP может ответить через 2 сек (или таймаут через 30 сек).

**Async flow для высоконагруженных систем:**

```
POST /payments → немедленный ответ: { "payment_id": "...", "status": "PENDING" }

Background worker:
  → Kafka consumer: payments.pending
  → Вызвать PSP
  → Обновить статус

Client: polling GET /payments/{id} или WebSocket/webhook уведомление

Trade-off:
  Async: лучше throughput, user ждёт подтверждения дольше
  Sync:  user получает ответ сразу, но PSP timeout = проблема
  
Выбор: sync для UX, с timeout 5 сек.
  При PSP timeout → status = PENDING → async retry → notify via WebSocket
```

---

### Distributed Transactions: проблема с PSP

**Сценарий: банк списал деньги, сервер упал до обновления БД**

```
Step 1: BEGIN TRANSACTION
Step 2: UPDATE accounts SET balance -= 100  ← записали
Step 3: INSERT INTO payments ...             ← записали
Step 4: Call Stripe → charge success         ← Stripe списал деньги!
Step 5: COMMIT  ←── CRASH! Транзакция откатилась

Результат: деньги у Stripe списаны, в нашей БД — нет. Пользователь заплатил дважды (при retry).
```

**Решение: Saga Pattern (Outbox Pattern)**

```
Шаг 1: Сохранить PENDING платёж в БД атомарно
Шаг 2: Вызвать PSP
Шаг 3: Обновить статус

Ключевое: любой шаг может упасть → compensating transaction

Outbox Table:
  BEGIN TRANSACTION
    INSERT INTO payments (status=PENDING, ...)
    INSERT INTO outbox (event_type='CALL_PSP', payload={...})
  COMMIT
  
  Outbox Worker:
    Читать из outbox WHERE processed = false
    Вызвать PSP
    BEGIN TRANSACTION
      UPDATE payments SET status = COMPLETED WHERE id = ?
      UPDATE outbox SET processed = true WHERE id = ?
    COMMIT

  При падении после PSP call:
    Outbox Worker перезапустится → попробует снова
    PSP: idempotency key в запросе → не двойное списание у Stripe
    → At-least-once + idempotency = exactly-once semantics
```

---

### Ledger: неизменяемый лог

```sql
-- Append-only таблица (никогда не UPDATE/DELETE)
CREATE TABLE ledger_entries (
  id              BIGSERIAL     PRIMARY KEY,
  payment_id      UUID          NOT NULL,
  account_id      UUID          NOT NULL,
  amount          NUMERIC(15,2) NOT NULL,  -- положительные и отрицательные
  currency        CHAR(3)       NOT NULL,
  entry_type      VARCHAR(10)   NOT NULL,  -- DEBIT / CREDIT
  created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
  -- нет updated_at, нет deleted_at — только INSERT
  
  CONSTRAINT chk_nonzero CHECK (amount != 0)
);

-- Текущий баланс = SUM всех entries для account
-- Проверка целостности: SUM(amount) для всех entries = 0

-- Для performance: материализованный баланс
CREATE TABLE account_balances (
  account_id  UUID          PRIMARY KEY,
  balance     NUMERIC(15,2) NOT NULL DEFAULT 0,
  reserved    NUMERIC(15,2) NOT NULL DEFAULT 0,  -- pre-authorized
  updated_at  TIMESTAMPTZ   NOT NULL
);
```

**Audit и compliance:**
```
Ledger entries НИКОГДА не изменяются и не удаляются.
Для исправления ошибки → reversing entry:

Ошибочная запись:   account=A, amount=-100 (DEBIT)
Исправление:        account=A, amount=+100 (CREDIT), reason="reversal of entry #123"
Новая корректная:   account=A, amount=-100 (DEBIT), reason="corrected charge"

Так работает бухгалтерия в реальном мире.
```

---

### Reconciliation: сверка с PSP

**Проблема:** наши данные могут расходиться с данными Stripe.

```
Ежедневная reconciliation job:

1. Скачать отчёт от Stripe за вчера:
   GET /v1/balance/history?created[gte]=yesterday&created[lt]=today
   → Список всех charge_id, amount, status, currency

2. Сравнить с нашей БД:
   SELECT psp_id, amount, status FROM payments 
   WHERE created_at BETWEEN yesterday AND today

3. Найти расхождения:
   - В Stripe есть, у нас нет → создать запись со статусом NEEDS_REVIEW
   - У нас есть как COMPLETED, у Stripe как FAILED → alert! потенциальная потеря денег
   - Сумма не совпадает → currency conversion issue?

4. Alert команде финансов для ручной проверки

Автоматическое исправление: только для безопасных случаев
  PENDING > 24 часов без ответа PSP → пометить FAILED, вернуть средства

Frequency:
  Hourly mini-reconciliation: сверять последние 1000 транзакций
  Daily full reconciliation: весь день
```

---

### Fraud Prevention (базовый уровень)

```
Velocity checks (до обращения к PSP):
  User-level:
    Rate: > 5 транзакций за 1 мин → block (Redis: INCR + EXPIRE)
    Amount: > 50K RUB за раз → manual review
    
  Card-level:
    Один card_token → > 10 транзакций за 1 час → flag
    
  Geo check:
    Транзакция из России, предыдущая из США 10 минут назад → impossible travel → block

Реализация:
  Redis: rate limit per user_id, per card_token
  Rules Engine: конфигурируемые правила (без хардкода)
  
  if fraud_score(transaction) > threshold:
      → Reject with code FRAUD_SUSPECTED
      → Log в fraud_events table
      → Alert fraud team
```

---

### Multi-currency

```
Хранение:
  Все суммы в PostgreSQL NUMERIC(15,2)
  Валюта хранится отдельным полем CHAR(3) (ISO 4217: RUB, USD, EUR)
  
  НИКОГДА не хранить как FLOAT (floating point ошибки!)
  1000.10 USD как FLOAT может стать 1000.0999999...

Конвертация:
  Exchange Rate Service: кешировать курсы, обновлять раз в час
  При конвертации: сохранять exchange_rate на момент транзакции
  
  1 USD = 89.50 RUB (на момент транзакции) → ЗАФИКСИРОВАТЬ в записи
  Не пересчитывать постфактум по текущему курсу

Рисковая позиция:
  Если маркетплейс держит баланс в разных валютах → forex risk
  Hedging: out of scope для интервью, но упомянуть
```

---

## Трейдоффы

| Решение | Принятое | Альтернатива | Причина |
|---|---|---|---|
| Consistency | Strong (CP) | Eventual (AP) | Финансы: нельзя потерять или задвоить |
| Distributed TX | Saga + Outbox | Two-Phase Commit | 2PC: проблемы с доступностью, blocking |
| Хранение | PostgreSQL | NoSQL | ACID, SUM(entries) = 0 проверяемо |
| Idempotency | DB unique key | Redis cache | DB: durability, Redis: может упасть |
| PSP | Внешний (Stripe) | Кастомный | PCI DSS: огромные требования |
| Баланс | Материализованный | SUM каждый раз | Performance: баланс нужен на каждый запрос |

### Почему не NoSQL?

```
MongoDB, Cassandra: eventual consistency по умолчанию
  Баланс может быть "50 RUB" на одной реплике и "150 RUB" на другой
  → Можно списать с одного, списать с другого → двойное списание

PostgreSQL SERIALIZABLE isolation:
  SELECT ... FOR UPDATE гарантирует что параллельные транзакции
  увидят актуальный баланс → нет двойного списания

ACID нужен там где нарушение = потеря денег / регуляторные штрафы
```

---

## Interview-ready ответ (2 минуты)

> "Платёжная система — это идемпотентность, double-entry bookkeeping и reconciliation.
>
> Идемпотентность обязательна: клиент генерирует UUID один раз для платежа, сервер при повторном запросе возвращает кешированный результат. Без этого network error → retry → двойное списание.
>
> Double-entry: каждый платёж = набор ledger entries с нулевой суммой. Пользователь -100, продавец +95, платформа +5. Математически проверяемо, стандарт бухгалтерии. Entries — append-only, ничего не удаляется.
>
> Распределённые транзакции через Saga + Outbox Pattern: атомарно сохраняю PENDING + event в outbox, worker вызывает PSP с idempotency key, при успехе — обновляет статус. При crash → retry от outbox, PSP не дублирует списание.
>
> Strong consistency: PostgreSQL с SELECT FOR UPDATE. NoSQL с eventual consistency недопустим — баланс может разойтись между репликами.
>
> Reconciliation: ежедневная сверка с PSP отчётом. Любое расхождение → alert финансовой команде. PostgreSQL, NUMERIC(15,2) — никакого float."
