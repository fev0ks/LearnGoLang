# Notification Service

Разбор задачи "Спроектируй систему уведомлений". Типична для компаний с мобильным приложением, e-commerce, финтехом. Проверяет знание очередей, fan-out, delivery semantics.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Кандидат: Давайте уточню каналы и use cases.

Вопросы:
  - Какие каналы нужно поддерживать?
    → Push (iOS/Android), Email, SMS, in-app?
  - Уведомления транзакционные (OTP, подтверждение заказа) или маркетинговые (рассылки)?
  - Нужна ли шаблонизация сообщений? (переменные в тексте, локализация)
  - Нужно ли управление предпочтениями пользователя (opt-out)?
  - Нужна ли аналитика (delivered, opened)?
```

**Договорились (scope):**
- Каналы: Push (iOS + Android), Email, SMS
- Оба типа: транзакционные (немедленно) + маркетинговые (bulk, с расписанием)
- Шаблонизация: есть (переменные типа `{{user.name}}`, `{{order.id}}`)
- User preferences: пользователь может отключить отдельные каналы
- Delivery status: delivered/failed (opened — out of scope)

**Out of scope:** in-app уведомления, rich media (картинки в push), A/B testing контента, webhooks.

### Нефункциональные требования

```
- Транзакционные: latency < 5 сек end-to-end (OTP нужен быстро)
- Маркетинговые: могут ждать, но нужен throughput для 10M+ рассылки
- Delivery semantics: at-least-once (лучше дублировать, чем потерять)
- Idempotency: повторная отправка одного уведомления = одно сообщение пользователю
- Scale: 10M пользователей, 1M уведомлений/день в среднем
- High availability: 99.9%
```

---

## Фаза 2: Оценка нагрузки

```
Daily notifications = 1M
  Транзакционные: ~100K/day (OTP, order confirmations)
  Маркетинговые: ~900K/day (кампании)

Среднее:
  1M / 86400 ≈ 12 notifications/sec

Пиковая нагрузка (маркетинговая рассылка):
  Одна кампания на 1M пользователей за 1 час
  = 1M / 3600 ≈ 280 notifications/sec на каждый канал

Хранилище:
  Статус каждого уведомления: ~200 bytes
  1M/day × 365 × 3 года хранения = 1.1B записей ≈ 220 GB
  → Вполне управляемо

External provider rate limits:
  Firebase FCM: до 1000 msg/sec per project
  Twilio SMS: до 100 msg/sec по умолчанию
  SendGrid Email: до 600 emails/sec (paid tier)
  → Provider limits диктуют throughput, нужен backpressure
```

---

## Фаза 3: Высокоуровневый дизайн

```
                           ┌──────────────────────────────────┐
  Service A                │      Notification Service        │
  (Order placed)  ────────►│                                  │
                           │  ┌─────────────┐                 │
  Service B                │  │   API       │  validate,      │
  (OTP request)  ─────────►│  │   Gateway   │  template,      │
                           │  │             │  preferences    │
  Admin Panel              │  └──────┬──────┘                 │
  (bulk campaign)─────────►│         │                        │
                           │  ┌──────▼──────┐                 │
                           │  │  Message    │                 │
                           │  │  Queue      │  Kafka          │
                           │  └──────┬──────┘                 │
                           │         │                        │
                           │  ┌──────▼────────────────────┐   │
                           │  │     Dispatcher Workers    │   │
                           │  │  (Push | Email | SMS)     │   │
                           │  └──────┬────────────────────┘   │
                           └─────────┼────────────────────────┘
                                     │
          ┌──────────────────────────┼──────────────────────────┐
          │                          │                          │
   ┌──────▼──────┐            ┌──────▼──────┐           ┌──────▼──────┐
   │   Firebase  │            │  SendGrid   │           │   Twilio    │
   │    (Push)   │            │   (Email)   │           │    (SMS)    │
   └─────────────┘            └─────────────┘           └─────────────┘
```

---

## Фаза 4: Deep Dive

### Notification Pipeline

**Шаги обработки одного уведомления:**

```
1. Входящий запрос:
   POST /notifications
   {
     "type": "order_confirmed",
     "user_id": 12345,
     "template_id": "order-confirm-v2",
     "variables": { "order_id": "ORD-789", "amount": "4200 RUB" },
     "channels": ["push", "email"],         // или берём из user preferences
     "idempotency_key": "order-789-confirm"
   }

2. Validation:
   - user_id существует?
   - template_id существует?
   - Каналы активны для данного пользователя? (проверка preferences)

3. Template rendering:
   "Ваш заказ {{order_id}} на сумму {{amount}} подтверждён"
   → "Ваш заказ ORD-789 на сумму 4200 RUB подтверждён"

4. Idempotency check:
   - Проверить idempotency_key в Redis (TTL 24h)
   - Если ключ уже есть → вернуть cached response, не отправлять повторно

5. Fan-out в очереди:
   Для каждого канала → отдельное сообщение в Kafka:
   topic: notifications.push   → { user_id, rendered_body, device_tokens }
   topic: notifications.email  → { user_id, rendered_html, recipient_email }

6. Channel workers читают из своих топиков
   → вызывают external provider
   → обновляют статус в DB
```

---

### Kafka топики и партиционирование

```
Топики:
  notifications.push    (10 партиций)
  notifications.email   (10 партиций)
  notifications.sms     (5 партиций)
  notifications.dlq     (dead letter queue — ошибки после N retries)

Partition key = user_id:
  - Гарантирует ordering для одного пользователя
  - Равномерное распределение (если нет hotspot users)

Consumer groups:
  push-workers:  10 воркеров (по 1 на партицию)
  email-workers: 10 воркеров
  sms-workers:   5 воркеров
```

---

### Retry и Dead Letter Queue

**Логика retry для transient errors (5xx от провайдера, timeout):**

```
Попытка 1: немедленно
Попытка 2: через 30 сек (exponential backoff)
Попытка 3: через 5 мин
Попытка 4: через 30 мин
После 4-й: → DLQ + alert + статус = FAILED

Permanent errors (4xx — неверный токен, невалидный email):
  → сразу в DLQ, не retry
  → пометить device token как inactive (для push)
```

**DLQ обработчик:**
- Алертинг инженерам на необычный объём
- Manual replay или skip
- Анализ причин для мониторинга

---

### User Preferences

```sql
CREATE TABLE user_notification_preferences (
  user_id     BIGINT NOT NULL,
  channel     VARCHAR(20) NOT NULL,  -- 'push', 'email', 'sms'
  category    VARCHAR(50) NOT NULL,  -- 'transactional', 'marketing', 'security'
  enabled     BOOLEAN NOT NULL DEFAULT true,
  updated_at  TIMESTAMP NOT NULL,
  PRIMARY KEY (user_id, channel, category)
);
```

**Кеш preferences:**
```
Redis: HGETALL prefs:{user_id}
TTL: 5 минут (preferences меняются редко)

При изменении settings → invalidate cache немедленно
```

**Важно:** транзакционные (OTP, security alerts) нельзя отключить. Проверять в validation layer до сохранения в очередь.

---

### Транзакционные vs Маркетинговые

```
Транзакционные (OTP, confirmations):
  - Высокий приоритет → отдельный Kafka топик (notifications.priority)
  - Воркеры с меньшим батчингом → ниже latency
  - No rate limiting throttling

Маркетинговые (campaigns):
  - Low priority топик
  - Throttling: не более X msg/sec на провайдера
  - Scheduled: "отправить в 10:00 по timezone пользователя"
  - Unsubscribe link обязателен (CAN-SPAM, GDPR)
```

---

### Scheduled Notifications

```
Сценарий: кампания "отправить 1M пользователям в 10:00 UTC+3"

Подход: delayed message scheduling

1. Admin создаёт кампанию:
   { campaign_id, template_id, audience_segment_id, schedule_time }
   → сохранить в PostgreSQL, статус = SCHEDULED

2. Scheduler job (cron каждую минуту):
   SELECT * FROM campaigns WHERE schedule_time <= NOW() AND status = 'SCHEDULED'
   → статус = PROCESSING
   → начать fan-out: загрузить audience (user IDs из segment)
   → push сообщения в Kafka батчами по 1000

3. Throttling:
   Kafka consumer worker проверяет token bucket:
   "не более 500 email/sec через SendGrid"
   → при превышении → sleep → продолжить
```

---

### Delivery Status Tracking

```sql
CREATE TABLE notification_log (
  id              BIGINT GENERATED ALWAYS AS IDENTITY,
  idempotency_key VARCHAR(128) UNIQUE,
  user_id         BIGINT NOT NULL,
  channel         VARCHAR(20) NOT NULL,
  template_id     VARCHAR(100),
  status          VARCHAR(20) NOT NULL,  -- QUEUED/SENT/DELIVERED/FAILED
  provider_msg_id VARCHAR(256),          -- ID от FCM/SendGrid/Twilio
  error_message   TEXT,
  created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);
```

**Delivery confirmation:**
- Email (SendGrid): webhook `delivered` event → UPDATE status
- Push (FCM): response при отправке + webhook для confirmation
- SMS (Twilio): webhook delivery receipt

---

### Что если провайдер недоступен?

```
FCM недоступен 30 минут:
  - Retry с exponential backoff
  - Сообщения накапливаются в Kafka (retention = 7 дней)
  - Alert инженерам при lag > 10000 сообщений
  - При восстановлении FCM → воркеры продолжат автоматически
  - Transactional OTP: уведомить через SMS как fallback (если настроено)
```

---

## Трейдоффы

| Решение | Принятое | Альтернатива | Причина |
|---|---|---|---|
| Queue | Kafka | SQS/RabbitMQ | Replay, retention, consumer groups |
| Fan-out | По каналам | По пользователям | Независимый throttling для каждого провайдера |
| Idempotency | Redis + idempotency_key | DB уникальный индекс | Redis быстрее для check на hot path |
| Scheduling | Cron + DB | Kafka delayed messages | Проще, легко мониторить |
| Status tracking | PostgreSQL | ClickHouse | Достаточен для аналитики, не нужен OLAP |

---

## Interview-ready ответ (2 минуты)

> "Notification service — это fan-out система с несколькими каналами и сильно разными нагрузками: транзакционные требуют latency < 5 сек, маркетинговые — throughput для миллионных рассылок.
>
> Ключевая архитектура: API принимает запрос, рендерит шаблон, проверяет idempotency через Redis, затем fan-out в Kafka — отдельный топик для каждого канала. Это позволяет независимо масштабировать и throttle каждый канал под лимиты провайдера.
>
> Отдельный высокоприоритетный топик для транзакционных — чтобы OTP не застрял за батчем маркетинговой рассылки.
>
> Retry с exponential backoff, после N попыток → DLQ. Permanent errors (невалидный токен) — сразу в DLQ без retry.
>
> User preferences в PostgreSQL, кешированы в Redis с TTL 5 минут. Транзакционные уведомления нельзя отключить — это проверяется до постановки в очередь."
