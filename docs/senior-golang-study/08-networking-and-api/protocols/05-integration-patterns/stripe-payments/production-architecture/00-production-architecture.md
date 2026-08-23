# Production-архитектура Stripe-интеграции

## Содержание

- [Цель и границы](#цель-и-границы)
- [Подробные sequence-разборы](#подробные-sequence-разборы)
- [Компоненты и процессы](#компоненты-и-процессы)
- [Структура Go-проекта](#структура-go-проекта)
- [Контракты между сервисами](#контракты-между-сервисами)
- [Таблицы payment-service](#таблицы-payment-service)
- [Таблицы booking-service](#таблицы-booking-service)
- [Основные workflow](#основные-workflow)
- [Границы транзакций и идемпотентность](#границы-транзакций-и-идемпотентность)
- [Статусы и инварианты](#статусы-и-инварианты)
- [Обработка сбоев](#обработка-сбоев)
- [Стратегия тестирования](#стратегия-тестирования)
- [Production checklist](#production-checklist)
- [Interview-ready answer](#interview-ready-answer)

Этот документ описывает целевую структуру Stripe-интеграции для системы, в
которой после авторизации карты нужно выполнить внешний шаг: например,
забронировать услугу у поставщика. Это не аудит конкретного проекта, а
самостоятельный reference design с именами компонентов, каталогов, таблиц и
контрактов.

Подробности жизненного цикла денег разобраны в
[PaymentIntent и жизненный цикл денег](../01-payment-intent-lifecycle.md), а
правила доставки событий — в
[Webhooks, inbox и reconciliation](../03-webhooks-and-reconciliation.md).

---

## Цель и границы

Для карточного payment method основной flow выглядит так:

`authorization → supplier booking → capture` при успехе или
`authorization → supplier booking → cancel` при отказе.

Архитектура должна обеспечивать следующие свойства:

- frontend не задаёт итоговую сумму и не переводит заказ в `paid`;
- только payment-компонент знает Stripe API key и вызывает Stripe;
- только booking-компонент знает протоколы поставщиков;
- webhook считается принятым после durable commit, а не после выполнения всей
  бизнес-логики;
- create, capture, cancel и refund повторяются с тем же idempotency key;
- статус оплаты и статус бронирования — две разные state machine;
- зависшие операции восстанавливаются reconciliation job.

`payment-service` и `booking-service` ниже — логические границы владения. Их
можно развернуть как отдельные сервисы либо сначала оставить модулями одного
приложения. Важнее не число deployment units, а раздельные модели, интерфейсы и
владение таблицами.

---

## Подробные sequence-разборы

Основной документ фиксирует целевую структуру, но не пытается поместить весь
распределённый flow в одну широкую схему. Точные границы транзакций, удалённые
вызовы, webhook и фоновые workers разобраны отдельно:

1. [Checkout и создание PaymentIntent](./01-checkout-and-payment-intent.md)
   — от server-side цены до `client_secret`, включая crash между Stripe API и
   локальным commit.
2. [Webhook, inbox и outbox](./02-webhook-inbox-and-outbox.md)
   — что происходит до HTTP `2xx`, как worker применяет событие и как outbox
   доставляет внутренний event.
3. [Authorization, booking и capture](./03-booking-and-capture.md)
   — supplier call, `BookingConfirmed`/`BookingRejected`, capture или cancel.
4. [Сбои и reconciliation](./04-failures-and-reconciliation.md)
   — неоднозначный результат, lease timeout, потерянный webhook и восстановление.
5. [Webhook retry и deadline prebooking](./05-webhook-retry-and-prebooking-deadline.md)
   — независимые таймеры Stripe delivery, prebooking TTL и authorization hold;
   локальная сверка Stripe state до истечения business deadline.

Во всех sequence diagram DB-транзакция показана явными сообщениями `BEGIN` и
`COMMIT`. Если между ними расположен Stripe или supplier call, это означает
ошибочную границу; в рекомендуемом flow удалённые вызовы выполняются после
commit.

---

## Компоненты и процессы

| Компонент | Роль | Чего он не делает |
| --- | --- | --- |
| `booking-api` | Проверяет заказ, цену, доступность checkout и создаёт запрос на оплату | Не принимает сумму от frontend как доверенную и не вызывает Stripe |
| `payment-api` | Создаёт локальный `Payment`, отдаёт состояние оплаты, принимает подписанные webhook | Не бронирует услугу у поставщика |
| `payment-worker` | Обрабатывает webhook inbox, выполняет create/capture/cancel/refund operations, публикует outbox | Не держит DB-транзакцию во время Stripe API call |
| `booking-worker` | После события `PaymentAuthorized` вызывает поставщика и публикует результат | Не помечает деньги captured по ответу frontend |
| `reconciliation-worker` | Проверяет зависшие и неоднозначные операции через Stripe API | Не является основным happy path |
| PostgreSQL payment schema | Хранит payments, operations, refunds, Stripe inbox и payment outbox | Не хранит карточные реквизиты |
| PostgreSQL booking schema | Хранит orders, supplier bookings и booking-payment saga | Не дублирует полный Stripe payload |
| Message broker | Доставляет внутренние commands/events между владельцами данных | Не заменяет inbox/outbox и не гарантирует exactly-once effects |

Практичный deployment без лишнего дробления:

- `payment-api` и `payment-worker` — два процесса из одного payment-модуля;
- `booking-api` и `booking-worker` — два процесса из одного booking-модуля;
- reconciliation запускается отдельным режимом `payment-worker` или CronJob;
- webhook endpoint находится в `payment-api`, а не в отдельном микросервисе.

Так API можно масштабировать по входящим запросам, а workers — по глубине очереди
и latency внешних систем.

---

## Структура Go-проекта

Один из рабочих вариантов для репозитория с несколькими сервисами:

```text
services/
  payment/
    cmd/
      payment-api/
        main.go
      payment-worker/
        main.go
    internal/
      domain/
        payment.go
        payment_status.go
        operation.go
        refund.go
        events.go
        errors.go
      app/
        create_payment.go
        get_payment.go
        apply_stripe_event.go
        request_capture.go
        request_cancel.go
        request_refund.go
        reconcile_payment.go
        ports.go
      adapters/
        stripe/
          client.go
          payment_intents.go
          refunds.go
          webhook.go
          mapper.go
        postgres/
          payment_repository.go
          operation_repository.go
          refund_repository.go
          inbox_repository.go
          outbox_repository.go
        broker/
          publisher.go
          booking_events_consumer.go
      transport/
        http/
          payments_handler.go
          stripe_webhook_handler.go
          middleware.go
      workers/
        webhook_worker.go
        operation_worker.go
        outbox_worker.go
        reconciliation_worker.go
    migrations/
      0001_payments.sql
      0002_payment_operations.sql
      0003_stripe_event_inbox.sql
      0004_payment_outbox.sql
      0005_refunds.sql

  booking/
    cmd/
      booking-api/
        main.go
      booking-worker/
        main.go
    internal/
      domain/
        booking.go
        booking_payment_saga.go
        events.go
      app/
        start_checkout.go
        handle_payment_authorized.go
        book_with_supplier.go
        handle_payment_captured.go
        ports.go
      adapters/
        payment/
          client.go
          events_consumer.go
        suppliers/
          supplier.go
          provider_a.go
        postgres/
          booking_repository.go
          saga_repository.go
          inbox_repository.go
          outbox_repository.go
      transport/
        http/
          checkout_handler.go
      workers/
        booking_worker.go
        outbox_worker.go
    migrations/
      0001_bookings.sql
      0002_booking_payment_sagas.sql
      0003_booking_inbox_outbox.sql

contracts/
  events/
    payment_authorized_v1.go
    booking_confirmed_v1.go
    booking_rejected_v1.go
    payment_captured_v1.go
    payment_failed_v1.go
```

### Что хранить в слоях

- `domain` содержит статусы, инварианты и чистые переходы без Stripe SDK и SQL.
- `app` оркестрирует use cases через интерфейсы из `ports.go`.
- `adapters/stripe` — единственное место, где типы Stripe SDK видны приложению.
- `adapters/postgres` реализует repositories и atomic inbox/outbox operations.
- `transport/http` проверяет вход, но не содержит payment state machine.
- `workers` отвечает за claim, retry, backoff, lease и dead-letter policy.

Stripe SDK-типы не должны проникать в domain. Например, domain работает с
`AuthorizedAmount`, `CaptureDeadline` и `ProviderPaymentID`, а `mapper.go`
преобразует `stripe.PaymentIntent` в эти значения.

### Основные Go-интерфейсы

```go
type PaymentGateway interface {
    CreateIntent(ctx context.Context, cmd CreateIntentCommand) (Intent, error)
    GetIntent(ctx context.Context, providerID string) (Intent, error)
    Capture(ctx context.Context, cmd CaptureCommand) (Intent, error)
    Cancel(ctx context.Context, cmd CancelCommand) (Intent, error)
    CreateRefund(ctx context.Context, cmd RefundCommand) (RefundResult, error)
}

type PaymentRepository interface {
    GetForUpdate(ctx context.Context, id uuid.UUID) (Payment, error)
    Save(ctx context.Context, payment Payment) error
}
```

Idempotency key входит в каждую command, меняющую деньги. Gateway не генерирует
его сам: стабильный ключ создаётся и сохраняется приложением до сетевого вызова.

---

## Контракты между сервисами

### Внешний HTTP API

| Endpoint | Владелец | Назначение |
| --- | --- | --- |
| `POST /v1/orders/{order_id}/payments` | `booking-api` | Проверить заказ и начать checkout с серверной суммой |
| `GET /v1/orders/{order_id}/payment` | `booking-api` | Вернуть агрегированное состояние для UI |
| `POST /internal/payments` | `payment-api` | Создать или вернуть payment по стабильному business key |
| `GET /internal/payments/{payment_id}` | `payment-api` | Получить авторитетное payment state |
| `POST /webhooks/stripe` | `payment-api` | Проверить подпись и durable сохранить Stripe event |

Frontend получает `client_secret` только для завершения конкретного checkout.
Он не отправляет callback вида `mark-paid`: после confirm UI опрашивает backend
или получает application-level notification.

### Внутренние события

Минимальный набор versioned contracts:

| Событие | Producer | Consumer | Ключевые поля |
| --- | --- | --- | --- |
| `PaymentAuthorized.v1` | payment | booking | `event_id`, `payment_id`, `order_id`, amount, currency, `capture_before` |
| `BookingConfirmed.v1` | booking | payment | `event_id`, `booking_id`, `payment_id`, supplier reference |
| `BookingRejected.v1` | booking | payment | `event_id`, `payment_id`, reason category |
| `PaymentCaptured.v1` | payment | booking | `event_id`, `payment_id`, captured amount |
| `PaymentCanceled.v1` | payment | booking | `event_id`, `payment_id` |
| `PaymentCaptureFailed.v1` | payment | booking/operations | `event_id`, `payment_id`, retryability, error category |
| `RefundUpdated.v1` | payment | booking/support | `event_id`, `refund_id`, amount, status |

У сообщения есть уникальный `event_id`, версия schema и `occurred_at`. Consumer
дедуплицирует доставку по `event_id`, но бизнес-эффект дополнительно защищается
уникальным business key. Broker offset сам по себе недостаточен: после rebalance
или публикации из outbox сообщение может прийти повторно.

---

## Таблицы payment-service

### `payments`

Одна строка описывает один логический платёж или один installment, но не весь
заказ со всеми попытками оплаты.

```sql
CREATE TABLE payments (
    id                      uuid PRIMARY KEY,
    business_key            text NOT NULL UNIQUE,
    order_id                uuid NOT NULL,
    payment_plan_item_id    uuid,
    provider                text NOT NULL DEFAULT 'stripe',
    provider_payment_id     text,
    capture_policy          text NOT NULL,
    status                  text NOT NULL,
    provider_status         text,
    currency                char(3) NOT NULL,
    amount_expected         bigint NOT NULL CHECK (amount_expected > 0),
    amount_authorized       bigint NOT NULL DEFAULT 0 CHECK (amount_authorized >= 0),
    amount_captured         bigint NOT NULL DEFAULT 0 CHECK (amount_captured >= 0),
    amount_refunded         bigint NOT NULL DEFAULT 0 CHECK (amount_refunded >= 0),
    capture_before          timestamptz,
    next_reconcile_at       timestamptz,
    last_reconciled_at      timestamptz,
    version                 bigint NOT NULL DEFAULT 0,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CHECK (capture_policy IN ('manual', 'automatic')),
    CHECK (amount_refunded <= amount_captured)
);

CREATE UNIQUE INDEX payments_provider_id_uq
    ON payments (provider, provider_payment_id)
    WHERE provider_payment_id IS NOT NULL;

CREATE INDEX payments_order_id_idx ON payments (order_id);
CREATE INDEX payments_pending_idx ON payments (status, updated_at);
CREATE INDEX payments_reconcile_idx ON payments (next_reconcile_at)
    WHERE next_reconcile_at IS NOT NULL;
```

Все суммы хранятся в минимальных единицах валюты. Полный `client_secret`, PAN,
CVC и другие карточные данные в эту таблицу не записываются. `business_key`
формируется из стабильной бизнес-операции, например
`order:{orderID}:plan-item:{itemID}:attempt:{n}`.

### `payment_operations`

Таблица делает удалённый effect наблюдаемой и повторяемой командой.

```sql
CREATE TABLE payment_operations (
    id                  uuid PRIMARY KEY,
    payment_id          uuid NOT NULL REFERENCES payments(id),
    operation_type      text NOT NULL,
    operation_key       text NOT NULL UNIQUE,
    stripe_idempotency_key text NOT NULL UNIQUE,
    amount              bigint,
    status              text NOT NULL,
    attempt_count       integer NOT NULL DEFAULT 0,
    next_attempt_at     timestamptz NOT NULL DEFAULT now(),
    lease_until         timestamptz,
    provider_request_id text,
    last_error_code     text,
    last_error_message  text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (operation_type IN ('create_intent', 'capture', 'cancel', 'refund')),
    CHECK (status IN ('pending', 'running', 'retry_scheduled',
                      'succeeded', 'failed', 'unknown')),
    CHECK (amount IS NULL OR amount > 0)
);

CREATE INDEX payment_operations_ready_idx
    ON payment_operations (next_attempt_at)
    WHERE status IN ('pending', 'retry_scheduled', 'unknown');
```

`unknown` означает неоднозначный результат: запрос мог дойти до Stripe, но
ответ потерян. Такой статус нельзя автоматически превращать в новый effect с
другим key. Worker повторяет запрос с прежним key или сначала читает объект из
Stripe.

### `refunds`

Refund — отдельная сущность, потому что запрос возврата и фактическое завершение
возврата происходят не одновременно.

| Поле | Назначение |
| --- | --- |
| `id` | Внутренний refund ID |
| `payment_id` | Исходный captured payment |
| `provider_refund_id` | Stripe `re_...`, unique при наличии |
| `operation_id` | Команда, создавшая refund |
| `amount`, `currency` | Сумма возврата в минимальных единицах |
| `reason` | Нормализованная business reason |
| `status` | `pending`, `succeeded`, `failed`, `canceled` |
| `failure_code` | Машиночитаемая причина без секретных данных |
| `created_at`, `updated_at` | Аудит lifecycle |

Сумма всех успешных refunds обновляет `payments.amount_refunded`. Полный
`refunded` выставляется только когда возвращена вся captured-сумма; иначе статус
равен `partially_refunded`.

### `stripe_event_inbox`

```sql
CREATE TABLE stripe_event_inbox (
    event_id          text PRIMARY KEY,
    event_type        text NOT NULL,
    object_id         text,
    api_version       text,
    payload           jsonb NOT NULL,
    status            text NOT NULL DEFAULT 'pending',
    attempt_count     integer NOT NULL DEFAULT 0,
    next_attempt_at   timestamptz NOT NULL DEFAULT now(),
    lease_until       timestamptz,
    received_at       timestamptz NOT NULL DEFAULT now(),
    processed_at      timestamptz,
    last_error        text,
    CHECK (status IN ('pending', 'processing', 'retry_scheduled',
                      'processed', 'dead_letter'))
);

CREATE INDEX stripe_event_inbox_ready_idx
    ON stripe_event_inbox (next_attempt_at)
    WHERE status IN ('pending', 'retry_scheduled');
```

Endpoint вставляет событие через `INSERT ... ON CONFLICT DO NOTHING` и возвращает
`2xx` только после commit. Payload содержит персональные данные, поэтому для него
нужны ограниченный доступ, срок хранения и redaction в логах.

### `payment_outbox`

| Поле | Назначение |
| --- | --- |
| `id` | ID внутреннего сообщения |
| `aggregate_id` | `payment_id` |
| `event_type`, `event_version` | Тип и версия контракта |
| `dedup_key` | Unique business key публикации |
| `payload` | Versioned event payload |
| `status` | `pending`, `publishing`, `published`, `retry_scheduled`, `dead_letter` |
| `attempt_count`, `next_attempt_at` | Retry state |
| `created_at`, `published_at` | Аудит доставки |

Payment state и outbox event создаются в одной DB-транзакции. Публикация в broker
происходит после commit и может повторяться.

---

## Таблицы booking-service

Booking-service остаётся владельцем заказа и результата поставщика.

### `bookings`

Минимально нужны `id`, `order_id`, `status`, `supplier`,
`supplier_booking_id`, price snapshot, timestamps и version для optimistic
locking. Supplier reference должен иметь unique index в пределах поставщика.

### `booking_payment_sagas`

Здесь saga — обычная строка состояния распределённого flow, а не отдельная
технология. Она помнит, что уже произошло между authorization, supplier booking
и capture/cancel, и какой шаг worker должен выполнить дальше. Подробный пример —
в разделе [Saga простыми словами](./03-booking-and-capture.md#saga-простыми-словами).

| Поле | Назначение |
| --- | --- |
| `id` | ID одной попытки провести authorization → booking → capture/cancel |
| `order_id`, `booking_id`, `payment_id` | Связь двух state machines |
| `state` | Текущий шаг и подсказка worker, какое действие должно быть следующим |
| `capture_before` | Deadline авторизации, полученный от payment-service |
| `prebooking_expires_at` | Когда поставщик или локальный inventory освободит временную бронь |
| `decision_deadline` | Последний безопасный момент начать оставшиеся шаги с учётом safety margin |
| `supplier_operation_key` | Стабильный ключ бронирования у поставщика |
| `last_error_code` | Нормализованная причина сбоя |
| `version` | Optimistic locking |
| `created_at`, `updated_at` | Аудит |

Рекомендуемые states: `waiting_authorization`, `booking_pending`, `booking`,
`supplier_result_unknown`, `booking_confirmed`, `capture_requested`, `completed`,
`cancel_requested`, `compensation_pending`, `failed`.

Unique constraint на `payment_id` не позволяет двум saga независимо выполнить
supplier booking по одной авторизации.

### `booking_event_inbox` и `booking_outbox`

Booking-service также использует inbox/outbox:

- inbox дедуплицирует `PaymentAuthorized` и `PaymentCaptured`;
- outbox публикует `BookingConfirmed` или `BookingRejected` в одной транзакции с
  изменением booking/saga;
- unique `dedup_key` защищает повтор business effect, а не только повтор broker
  delivery.

Если оба модуля используют один PostgreSQL, таблицы всё равно лучше разделить по
schema и repository. Shared database не должен превращаться в произвольные
записи payment-кода в booking-таблицы и наоборот.

---

## Основные workflow

### Создание оплаты и confirm

1. `booking-api` загружает заказ и заново получает доверенную сумму и валюту.
2. Он формирует стабильный `business_key` и вызывает `payment-api`.
3. Payment-service короткой транзакцией создаёт `payments` и
   `payment_operations(create_intent)`, затем делает commit.
4. Stripe `PaymentIntent` создаётся вне DB-транзакции с `capture_method=manual`
   для поддерживаемого payment method и сохранённым idempotency key.
5. Результат сохраняется короткой транзакцией. При crash повтор использует тот же
   key и не создаёт второй intent.
6. Frontend получает `client_secret` и выполняет confirm/3-D Secure средствами
   Stripe SDK.
7. Redirect или ответ client SDK меняет только экран UI; источником истины для
   backend остаётся webhook и последующая reconciliation.

Create можно выполнить синхронным application use case, если latency приемлема.
Но сетевой вызов всё равно располагается между двумя короткими транзакциями, а
не внутри одной длинной.

### Приём webhook

1. Endpoint ограничивает размер и читает исходные bytes request body.
2. Проверяет `Stripe-Signature` secret конкретного endpoint.
3. Вставляет `event.id` и payload в `stripe_event_inbox`.
4. После commit возвращает `2xx`; при недоступной inbox DB возвращает `5xx`.
5. `webhook_worker` асинхронно применяет событие и безопасно переживает дубли и
   нарушенный порядок доставки.

Для manual capture событие `payment_intent.amount_capturable_updated` служит
сигналом проверить `amount_capturable` и перевести локальный payment в
`authorized`. Статус не выводится только из имени event: worker сверяет object
ID, currency, суммы и допустимый локальный переход.

### Authorization → booking → capture

1. Payment-worker фиксирует `authorized`, `amount_authorized` и
   `capture_before`, а в той же транзакции создаёт `PaymentAuthorized.v1` в
   outbox.
2. Booking-worker дедуплицирует event и проверяет, что до `capture_before`
   остаётся запас на supplier call и capture.
3. Он создаёт или claim-ит `booking_payment_sagas`, затем вызывает поставщика со
   стабильным supplier operation key вне DB-транзакции.
4. При успехе booking и `BookingConfirmed.v1` фиксируются атомарно.
5. Payment-worker создаёт одну capture operation, вызывает Stripe с сохранённым
   idempotency key и ждёт финальное состояние через response/webhook.
6. Только `PaymentCaptured.v1` позволяет booking-service показать заказ как
   оплаченный и полностью завершённый.

Если поставщик отказал до capture, booking-worker публикует
`BookingRejected.v1`. Payment-worker создаёт cancel operation, а освобождение
hold подтверждается Stripe state/webhook.

### Поздняя отмена и refund

После capture отменить `PaymentIntent` уже нельзя. Booking-service создаёт
business command на возврат, payment-service фиксирует отдельную строку
`refunds` и `payment_operations(refund)`, затем вызывает Stripe.

Заказ не становится `refunded` по факту отправки API request. Нужно дождаться
успешного состояния refund через response, webhook или reconciliation. Partial
refund не должен отменять весь order, если это не задано отдельным business rule.

---

## Границы транзакций и идемпотентность

### Правило удалённого вызова

Правильная последовательность для Stripe или supplier API:

1. В DB сохранить operation и стабильный key.
2. Commit.
3. Выполнить удалённый вызов.
4. Короткой транзакцией сохранить результат и outbox event.

Нельзя удерживать transaction/connection/row lock во время потенциально долгого
HTTP-вызова. Crash между шагами 3 и 4 восстанавливается тем же idempotency key и
reconciliation, а не новой денежной операцией.

### Где нужен какой ключ

| Граница | Ключ | От чего защищает |
| --- | --- | --- |
| Повтор checkout | `payments.business_key` | Второй локальный payment для той же попытки |
| Stripe create | `stripe:create:{paymentID}` | Второй `PaymentIntent` |
| Stripe capture | `stripe:capture:{operationID}` | Повторный capture effect |
| Stripe cancel | `stripe:cancel:{operationID}` | Несогласованные повторы отмены |
| Stripe refund | `stripe:refund:{refundID}` | Второй возврат денег |
| Stripe webhook | `stripe_event_inbox.event_id` | Повторная обработка `evt_...` |
| Internal event | consumer inbox `event_id` | Повтор broker delivery |
| Supplier booking | `supplier_operation_key` | Двойное бронирование, если supplier поддерживает ключ |

Ключ дедупликации endpoint не заменяет idempotency внешнего effect. Например,
уникальный `event_id` защищает приём `BookingConfirmed`, но capture отдельно
защищается строкой operation и Stripe key.

---

## Статусы и инварианты

### Payment states

| Status | Смысл |
| --- | --- |
| `creating` | Локальная запись есть, создание intent ещё не завершено |
| `requires_payment_method` | Нужен новый payment method |
| `requires_action` | Клиент должен завершить 3-D Secure или другое действие |
| `processing` | Provider ещё обрабатывает асинхронный payment/capture result |
| `authorized` | Сумма доступна для capture, но ещё не captured |
| `capture_pending` | Capture command создана или выполняется |
| `captured` | Capture подтверждён provider state |
| `cancel_pending` | Идёт освобождение authorization |
| `canceled` | Intent отменён, capture больше не ожидается |
| `refund_pending` | Запрошен возврат captured-средств |
| `partially_refunded` | Возвращена часть captured amount |
| `refunded` | Возвращена вся captured amount |
| `failed` | Платёжная попытка завершилась терминальной ошибкой |

Provider status хранится отдельно в `provider_status`. Локальный `authorized` —
это business interpretation проверенного `requires_capture`, а не строковая
копия Stripe status.

### Инварианты

- booking начинается только при достаточном `amount_authorized` и безопасном
  запасе до `capture_before`;
- capture создаётся только для подтверждённого booking;
- cancel до capture и refund после capture — разные операции;
- `amount_refunded <= amount_captured`;
- заказ не становится `paid` из redirect, client callback или одного только
  `BookingConfirmed`;
- metadata Stripe используется для диагностики, а связь с order загружается по
  локальному `payment_id`/`provider_payment_id`;
- каждый переход либо идемпотентен, либо отвергает недопустимое предыдущее
  состояние;
- события могут прийти повторно и не по порядку, поэтому `event.created` нельзя
  использовать как единственный механизм ordering.

---

## Обработка сбоев

| Сбой | Поведение системы |
| --- | --- |
| Ответ Stripe create потерян | Operation становится `unknown`; повтор с тем же key или retrieve находит исходный intent |
| Один webhook пришёл дважды | Второй `INSERT inbox` конфликтует по `event_id`, endpoint всё равно отвечает `2xx` |
| Inbox DB недоступна | Endpoint отвечает `5xx`, чтобы Stripe повторил доставку |
| Worker упал после локального commit | Незавершённая inbox/operation запись снова claim-ится после lease timeout |
| Supplier booking отклонён | Публикуется `BookingRejected`, payment-service выполняет cancel |
| Supplier timeout дал неизвестный результат | Saga не создаёт новую бронь с новым key; сначала выполняется supplier reconciliation |
| Capture timeout | Повторяется та же capture operation с прежним Stripe key; payment не считается captured по timeout |
| Authorization скоро истечёт | Новое booking не стартует; существующее переводится в operations alert или компенсацию по policy |
| Webhook capture потерян | Reconciliation получает PaymentIntent и завершает локальный переход |
| Refund failed | Refund остаётся отдельной failed operation; заказ не помечается полностью refunded |

Retry применяется только к временным ошибкам: timeout, `429`, сетевой сбой,
некоторые `5xx`. Decline, неподдерживаемый payment method и нарушение локального
инварианта требуют нового пользовательского действия или ручного решения, а не
бесконечного retry.

Reconciliation должна регулярно находить как минимум:

- `creating`, `capture_pending`, `cancel_pending`, `refund_pending` старше SLA;
- `authorized` с приближающимся `capture_before`;
- booking confirmed без captured payment;
- captured payment без ожидаемого booking state;
- inbox events в `retry_scheduled`/`dead_letter`;
- операции со статусом `unknown`.

---

## Стратегия тестирования

### Unit tests

- таблица допустимых и запрещённых переходов payment state;
- `authorized → capture_pending` только после `BookingConfirmed`;
- `authorized → cancel_pending` после `BookingRejected`;
- partial и full refund;
- вычисление deadline buffer;
- классификация retryable и terminal errors.

### Integration tests с PostgreSQL

- два конкурентных create с одним `business_key` дают один payment;
- два одинаковых webhook дают одну inbox row и один business effect;
- payment state и outbox event коммитятся атомарно;
- worker lease позволяет подобрать задачу после crash;
- параллельные workers не выполняют одну operation дважды локально;
- повтор internal event безопасен для booking saga.

### Contract tests

- fixtures всех используемых Stripe event types для закреплённой API version;
- проверка подписи именно по raw body;
- mapping Stripe status/amounts/deadline в domain model;
- versioned contracts между payment и booking;
- supplier adapter обрабатывает timeout и повтор с тем же operation key.

### End-to-end в Stripe sandbox

Минимальные сценарии:

1. Успешная authorization → booking → capture.
2. 3-D Secure перед authorization.
3. Decline и повтор с другой картой.
4. Supplier rejection → cancel без refund.
5. Duplicate и delayed webhook.
6. Capture timeout с recovery.
7. Полный и частичный refund.
8. Reconciliation после намеренно пропущенного webhook.

---

## Production checklist

- [ ] Итоговая сумма и currency формируются сервером.
- [ ] `capture_method=manual` включён только для поддерживаемых payment methods.
- [ ] Authorization window больше supplier SLA плюс safety margin.
- [ ] Все Stripe mutations имеют заранее сохранённые idempotency keys.
- [ ] Stripe client внедрён через interface, global API key не используется.
- [ ] Webhook signature проверяется по raw body.
- [ ] `2xx` возвращается только после commit inbox.
- [ ] Payment и outbox event фиксируются одной транзакцией.
- [ ] Booking consumer имеет inbox и business deduplication.
- [ ] Capture/cancel/refund выполняются вне DB-транзакций.
- [ ] Есть partial/full refund model.
- [ ] Есть reconciliation для pending/unknown states.
- [ ] Метрики и alerts привязаны к SLA и `capture_before`.
- [ ] Логи не содержат `client_secret`, карточные данные и полный webhook payload.
- [ ] API version webhook endpoint и major-версия SDK закреплены и покрыты fixtures.

---

## Interview-ready answer

> Надёжную Stripe-интеграцию для booking-flow я разделяю на две state machine:
> payment и booking. Payment-service владеет `PaymentIntent`, webhook inbox,
> capture/cancel/refund operations и reconciliation; booking-service владеет
> заказом и вызовами поставщика. Для подходящих карт сначала создаётся manual
> authorization, затем выполняется booking, после успеха — идемпотентный capture,
> после отказа — cancel. Каждый удалённый effect хранится как operation до вызова,
> webhook подтверждается только после durable inbox commit, а переход состояния
> и outbox event коммитятся атомарно. Дубли, нарушенный порядок событий и crash
> между HTTP-вызовом и локальным commit закрываются idempotency keys, inbox/outbox
> и reconciliation.
