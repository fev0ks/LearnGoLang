# Stripe webhooks: подпись, inbox и reconciliation

## Содержание

- [Контракт доставки](#контракт-доставки)
- [Правильный приём webhook](#правильный-приём-webhook)
- [Durable inbox](#durable-inbox)
- [Идемпотентность бизнес-эффектов](#идемпотентность-бизнес-эффектов)
- [Нарушенный порядок событий](#нарушенный-порядок-событий)
- [Какие события нужны payment flow](#какие-события-нужны-payment-flow)
- [Refund и частичные суммы](#refund-и-частичные-суммы)
- [Reconciliation](#reconciliation)
- [Наблюдаемость](#наблюдаемость)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Webhook — не callback, который Stripe вызывает ровно один раз и ждёт завершения
всей бизнес-операции. Это at-least-once доставка уведомления через ненадёжную
сеть: событие может прийти повторно, позже другого события или не попасть в
локальную обработку из-за outage.

Надёжный endpoint подтверждает не завершение booking saga, а durable acceptance:
подпись проверена, исходное событие атомарно записано в локальный inbox и теперь
может быть обработано без участия Stripe.

---

## Контракт доставки

При проектировании нужно считать гарантированными следующие свойства:

- Stripe повторяет неуспешную доставку с backoff; в live mode окно автоматических
  повторов достигает трёх дней.
- Один `event.id` может быть доставлен больше одного раза.
- Два отдельных `Event` могут описывать один и тот же объект и логический эффект;
  для такой дедупликации нужен ключ `(event.type, data.object.id)`.
- Порядок доставки не гарантирован.
- Ответ `2xx` означает, что получатель взял ответственность за событие. После
  такого ответа нельзя рассчитывать на retry Stripe при локальной ошибке worker.

Из этих свойств не следует, что endpoint должен возвращать ошибку до завершения
всей бизнес-логики. Долгая синхронная обработка повышает число timeout и дублей.
Сначала событие сохраняется, затем отдельный worker выполняет эффекты.

---

## Правильный приём webhook

Синхронная часть endpoint должна быть короткой: прочитать ограниченный raw body
→ проверить `Stripe-Signature` → сохранить новое событие или дубль в inbox →
зафиксировать транзакцию → вернуть `2xx`. Только после этого отдельный worker
выполняет бизнес-логику.

Неверная подпись приводит к `400`, а временная ошибка до commit inbox — к `5xx`,
чтобы Stripe повторил доставку. Подробные правила разобраны ниже.

### Проверить именно исходное тело

Подпись вычисляется по неизменённым байтам request body. Нельзя сначала
декодировать JSON в структуру, затем снова сериализовать и проверять результат:
пробелы, порядок полей или представление чисел могут измениться.

Endpoint использует три входа:

- raw body;
- заголовок `Stripe-Signature`;
- signing secret конкретного endpoint.

Secret из `stripe listen` отличается от secret endpoint, созданного в Dashboard.
Оба начинаются с `whsec_`, но не взаимозаменяемы.

### Ограничить поверхность входа

Полезно ограничить размер тела, принимать только `POST`, слушать только нужные
event types и не логировать полный payload. В платёжном payload находятся email,
metadata и другие персональные данные; для диагностики обычно достаточно
`event.id`, `event.type`, object ID и Stripe request ID.

### Вернуть корректный код

| Ситуация | Ответ | Причина |
| --- | --- | --- |
| Нет подписи или подпись неверна | `400` | Запрос не считается доверенным событием Stripe |
| Payload нельзя декодировать | `400` | Повтор тех же байтов не исправит malformed event |
| Inbox DB временно недоступна | `500`/`503` | Событие ещё не принято durable, нужен retry Stripe |
| Новый event записан и commit прошёл | `200` | Дальше отвечает внутренний worker |
| Такой `event.id` уже записан | `200` | Повтор безопасно принят и не требует нового эффекта |
| Worker позже получил business error | Не влияет на уже выданный `200` | Ошибка повторяется внутренней очередью |

Главная граница проходит по commit inbox. Возвращать `200` после ошибки до этой
границы означает потерять событие; возвращать `500` после успешного commit
создаёт лишний повтор, который всё равно должен быть безопасен.

---

## Durable inbox

Минимальная таблица хранит не только факт наличия события, но и его обработку:

```sql
CREATE TABLE stripe_event_inbox (
    event_id          text PRIMARY KEY,
    event_type        text NOT NULL,
    object_id         text,
    api_version       text,
    payload           jsonb NOT NULL,
    status            text NOT NULL,
    attempt_count     integer NOT NULL DEFAULT 0,
    next_attempt_at   timestamptz NOT NULL DEFAULT now(),
    received_at       timestamptz NOT NULL DEFAULT now(),
    processed_at      timestamptz,
    last_error        text
);
```

Приём выполняет одну атомарную операцию:

```sql
INSERT INTO stripe_event_inbox (
    event_id, event_type, object_id, api_version, payload, status
)
VALUES ($1, $2, $3, $4, $5, 'pending')
ON CONFLICT (event_id) DO NOTHING;
```

Не нужна последовательность `SELECT exists` → business effect → `INSERT`.
Два параллельных запроса успеют пройти `SELECT` и оба выполнят эффект. Уникальный
`INSERT` должен находиться перед асинхронной обработкой.

Worker claim-ит записи короткой транзакцией через `FOR UPDATE SKIP LOCKED`,
увеличивает attempt, выполняет бизнес-логику и затем отмечает результат. После
исчерпания retry событие попадает в dead-letter/operations queue, а не навсегда
остаётся в `processing`.

Статус `processed` и бизнес-изменения желательно фиксировать в одной локальной
транзакции. Тогда crash не создаёт окно «заказ изменён, event ещё pending».
Удалённые вызовы в эту транзакцию не включаются: вместо них записывается outbox
command.

---

## Идемпотентность бизнес-эффектов

Inbox по `event.id` защищает только от повторной доставки одного Event. Этого
недостаточно для всех эффектов.

| Эффект | Возможный уникальный ключ |
| --- | --- |
| Перевод payment в `authorized` | `(payment_id, 'authorized', intent_id)` |
| Команда booking | `(order_id, 'book', booking_version)` |
| Команда capture | `(payment_id, 'capture', capture_version)` |
| Письмо об успешной оплате | `(order_id, template, payment_id)` |
| Analytics event | `(event_name, payment_id, business_version)` |
| Refund | внутренний `refund_operation_id` плюс Stripe idempotency key |

Это защищает от двух разных Stripe Events, которые сообщают один и тот же
бизнес-факт, и от crash между выполнением эффекта и обновлением inbox.

Переход состояния также должен быть условным. Вместо безусловного `UPDATE`:

```sql
UPDATE payments
SET status = 'captured', captured_amount = $2, updated_at = now()
WHERE id = $1
  AND status IN ('authorized', 'capture_pending', 'processing');
```

Нулевое число изменённых строк требует анализа: это может быть безопасный повтор,
устаревшее событие или нарушение инварианта. Молчаливо считать все три случая
успехом нельзя.

---

## Нарушенный порядок событий

Stripe прямо не гарантирует порядок доставки. Поэтому нельзя строить код на
предположении, что `payment_intent.created` обязательно обработан раньше
`payment_intent.succeeded`.

Для payment flow работают два приёма:

1. **Монотонные локальные переходы.** Финальный captured-платёж нельзя вернуть в
   `processing` более старым событием.
2. **Retrieve on ambiguity.** Если событие конфликтует с локальным состоянием,
   worker получает текущий `PaymentIntent` или `Refund` из Stripe и применяет
   фактическое состояние объекта.

Сравнение только по `event.created` недостаточно. Время говорит, когда создан
Event, но бизнес-объект мог измениться снова. Получение объекта дороже, поэтому
его используют при пропуске ожидаемого шага, конфликте или reconciliation, а не
обязательно для каждого штатного события.

Webhook может прийти раньше, чем локально сохранён `provider_payment_id`, если
процесс упал между Stripe API и DB. Такой event не нужно помечать безнадёжно
обработанным: он остаётся retryable, а create-команда восстанавливает связь по
стабильному idempotency key.

---

## Какие события нужны payment flow

Набор зависит от используемых payment methods и capture policy. Для карточного
manual-capture booking flow минимально рассматривают:

| Event | Возможное локальное действие |
| --- | --- |
| `payment_intent.amount_capturable_updated` | Проверить сумму/deadline, поставить `authorized`, enqueue booking |
| `payment_intent.succeeded` | Зафиксировать captured amount и завершить capture command |
| `payment_intent.payment_failed` | Записать попытку и разрешить новый payment method по правилам состояния |
| `payment_intent.canceled` | Зафиксировать освобождение hold и завершить cancel command |
| `refund.created` | Создать/обновить локальную refund operation как pending |
| `refund.updated` | Обновить сумму, ARN и состояние |
| `refund.failed` | Поставить retry/operations alert, не считать деньги возвращёнными |

`charge.refunded` полезен как агрегированный сигнал о сумме возврата по Charge,
но официальная документация рекомендует как минимум слушать `refund.created` для
данных о конкретной операции. Для partial refunds локальная модель `Refund`
намного точнее одного поля в `Payment`.

Неинтересный event можно подтвердить и записать как `ignored`, но список
подписанных событий лучше ограничить на стороне Stripe. Тогда случайное
расширение конфигурации не создаёт поток бесполезных payload.

---

## Refund и частичные суммы

Refund имеет три отдельных факта:

```text
refund requested -> refund accepted/processing -> refund succeeded или failed
```

Создание Refund API не означает, что деньги уже у клиента. Локально сначала
фиксируется `refund_pending`, а финальный результат подтверждается webhook или
reconciliation.

Для charge с `captured_amount = 10000` возможны состояния:

```text
refunded_amount = 0      -> captured
refunded_amount = 2500   -> partially_refunded
refunded_amount = 10000  -> refunded
```

Boolean `charge.refunded` означает полный refund charge. Если пришло
`charge.refunded`, но `amount_refunded < amount_captured`, нельзя отменять весь
заказ и весь payment plan. Сначала пересчитывается агрегат, затем применяется
бизнес-правило конкретной части заказа.

Refund-команда получает собственный idempotency key. Нельзя использовать один
key для двух намеренных частичных возвратов: это разные логические операции с
разными внутренними ID.

---

## Reconciliation

Webhook уменьшает задержку, но не заменяет сверку. Периодический job выбирает:

- payments в non-terminal status дольше ожидаемого времени;
- `authorized` с приближающимся `capture_before`;
- `capture_pending`, `cancel_pending` и `refund_pending` после timeout;
- заказы, у которых локальный payment и order status противоречат друг другу;
- inbox events с повторяющимися ошибками;
- captured payments без завершённого booking или явной compensation command.

Для каждой записи job получает актуальный объект Stripe по сохранённому ID и
идемпотентно применяет тот же transition, что webhook worker. Не нужно писать
вторую, отличающуюся реализацию state mapping.

Reconciliation не должен искать объекты Stripe через Search API в
read-after-write flow: поиск имеет eventual consistency. Надёжнее хранить
`pi_...`, `ch_...` и `re_...` локально и получать объект напрямую.

---

## Наблюдаемость

Минимальный production-набор включает:

- latency от `event.created` до durable inbox и до `processed_at`;
- количество дублей и out-of-order conflicts;
- inbox backlog, oldest event age и retry count;
- число `authorized` и минимальный остаток до `capture_before`;
- capture/cancel/refund pending age;
- расхождения локальной суммы и Stripe amount;
- число платежей `captured`, у которых booking не завершён;
- долю refund после ошибки бронирования;
- подписи, отклонённые endpoint, без сохранения чувствительного payload.

Alert «webhook endpoint отвечает 200» недостаточен: он не видит события,
застрявшие после durable acceptance.

---

## Типичные ошибки

- Возвращать `200` после любой business error, хотя event не сохранён для retry.
- Выполнять booking, email и analytics до атомарной дедупликации.
- Считать `event.id` единственным возможным business duplicate key.
- Предполагать порядок webhook и безусловно перетирать status.
- Проверять подпись по повторно сериализованному JSON.
- Использовать Dashboard signing secret для событий из Stripe CLI.
- Помечать partial refund как полный.
- Не иметь reconciliation, потому что «Stripe и так ретраит три дня».
- Игнорировать API version mismatch без contract tests на payload.

---

## Interview-ready answer

**1. Когда webhook endpoint должен вернуть `200`?**

- Условие — подпись проверена и событие durable записано либо уже существует в inbox.
- Граница — дальнейшая бизнес-обработка выполняется своей очередью и retry policy.
- Ошибка — если durable commit не прошёл, возвращается `5xx`, чтобы Stripe повторил доставку.

**2. Почему проверки `event_id exists` перед обработкой недостаточно?**

- Race — два запроса одновременно увидят отсутствие записи.
- Атомарность — уникальный `INSERT ... ON CONFLICT` должен решить, кто принял event.
- Эффекты — email, booking и capture дополнительно защищаются business idempotency keys.

**3. Что делать с нарушенным порядком событий?**

- State machine — разрешать только допустимые и монотонные локальные переходы.
- Conflict — при неоднозначности получить текущее состояние объекта из Stripe.
- Recovery — тот же transition используется webhook worker и reconciliation job.
