# Booking flow: authorization → booking → capture

## Содержание

- [Какую проблему решает flow](#какую-проблему-решает-flow)
- [Рекомендуемый карточный workflow](#рекомендуемый-карточный-workflow)
- [Границы транзакций](#границы-транзакций)
- [Идемпотентные команды в Stripe](#идемпотентные-команды-в-stripe)
- [Сбои и компенсации](#сбои-и-компенсации)
- [Рассрочка и составная бронь](#рассрочка-и-составная-бронь)
- [Когда manual capture не подходит](#когда-manual-capture-не-подходит)
- [Минимальный Go-пример](#минимальный-go-пример)
- [Interview-ready answer](#interview-ready-answer)

Booking-система координирует как минимум две независимые системы: Stripe и API
поставщика. Зафиксировать capture и подтверждение номера одной ACID-транзакцией
невозможно. Нужна saga, в которой каждый удалённый шаг имеет локально записанную
команду, повтор с тем же ключом и компенсацию.

---

## Какую проблему решает flow

При automatic capture последовательность часто выглядит так:

```text
confirm -> деньги captured -> попытка бронирования -> поставщик отказал -> refund
```

Этот flow может быть допустимым, но у него есть цена: клиент видит реальное
списание, refund занимает отдельное время, возможна комиссия, а поддержка должна
объяснять расхождение между «оплачено» и «не забронировано».

Для поддерживаемого карточного платежа manual capture меняет порядок:

```text
confirm -> деньги authorized -> попытка бронирования -> capture
                                         |
                                         +-> cancel при отказе
```

Такой порядок уменьшает число refund, но переносит риск: после успешной брони
capture может временно не пройти или hold может истечь. Поэтому manual capture
не устраняет распределённую согласованность — он выбирает более подходящую точку
компенсации.

---

## Рекомендуемый карточный workflow

Коротко: создать локальный `Payment` → создать `PaymentIntent` с manual capture
→ получить авторизацию через webhook → попытаться забронировать → выполнить
capture при успехе или cancel при отказе поставщика.

Ниже каждый шаг разобран отдельно, включая границы DB-транзакций,
идемпотентность и обработку сбоев.

### 1. Зафиксировать локальную оплату

В короткой DB-транзакции сервер:

- перечитывает цену и валюту из доверенного источника;
- проверяет срок жизни предложения;
- создаёт `Payment` со стабильным внутренним ID;
- сохраняет ключ операции create;
- фиксирует транзакцию.

Клиент присылает выбор способа оплаты, но не итоговую сумму. Если один и тот же
checkout повторён, сервер возвращает существующий живой платёж вместо создания
нового.

### 2. Создать `PaymentIntent`

После коммита сервер вызывает Stripe с `capture_method=manual` и idempotency key,
полученным из ID локальной операции. Если процесс падает после ответа Stripe, но
до сохранения `pi_...`, повтор с тем же ключом возвращает тот же результат.

`PaymentIntent` должен содержать диагностическую metadata, но связь с заказом
хранится локальным foreign key. После ответа `pi_...` записывается отдельной
короткой транзакцией.

### 3. Подтвердить платёж на клиенте

Stripe Elements или мобильный SDK получает `client_secret` и выполняет confirm.
Клиент обрабатывает `requires_action`, но не переводит заказ в `paid` собственным
запросом. Redirect можно использовать для экрана «проверяем оплату», который
читает состояние с backend.

### 4. Дождаться авторизации

Webhook `payment_intent.amount_capturable_updated` сохраняется в durable inbox.
Worker проверяет:

- локальный `Payment` действительно связан с этим `pi_...`;
- `amount_capturable` покрывает ожидаемую сумму;
- валюта совпадает;
- заказ ещё допускает бронирование;
- authorization deadline оставляет запас на работу поставщика и capture.

После этого локальный статус становится `authorized`, а команда бронирования
попадает в outbox/очередь.

### 5. Выполнить бронирование

Booking worker атомарно claim-ит заказ и вызывает поставщика вне долгой
DB-транзакции. Запрос поставщику также получает idempotency key, если его API это
поддерживает. Если нет, перед повтором нужна операция lookup по собственной
reference, иначе timeout может породить две брони.

### 6. Захватить или освободить сумму

После подтверждения поставщика создаётся локальная команда capture. Capture
повторяется с одним ключом, пока Stripe не подтвердит результат или
reconciliation не обнаружит финальное состояние.

Если поставщик окончательно отказал, создаётся команда cancel. Заказ становится
`booking_failed` только вместе с записанной компенсацией, а не с надеждой, что
последующий best-effort вызов когда-нибудь сработает.

---

## Границы транзакций

Нельзя держать SQL-транзакцию открытой во время вызова Stripe или поставщика.
Удалённый запрос может занять секунды, вызвать 3-D Secure или зависнуть до
таймаута, пока row locks и соединение из пула остаются занятыми.

Правильная граница выглядит так:

```text
DB tx: записать command + state -> COMMIT
HTTP: выполнить удалённую операцию с idempotency key
DB tx: записать remote ID/result -> COMMIT
```

Между шагами возможен crash. Это не ошибка схемы, если worker умеет найти
незавершённую команду и повторить её с тем же ключом.

Для create нельзя генерировать новый key при каждом retry. Ключ описывает
логическую операцию, например:

```text
payment:{payment_id}:create:v1
payment:{payment_id}:capture:v1
payment:{payment_id}:cancel:v1
refund:{refund_operation_id}:create:v1
```

Версия меняется только тогда, когда бизнес намеренно создаёт новую операцию с
другими параметрами. Сетевой timeout новой операцией не является.

---

## Идемпотентные команды в Stripe

Stripe сохраняет первый результат `POST` для idempotency key и при повторе
возвращает его, включая некоторые ошибки `500`. Поэтому после неоднозначного
ответа алгоритм такой:

1. Повторить тот же endpoint с теми же параметрами и тем же key.
2. Если локально известен `PaymentIntent ID`, дополнительно получить его текущее
   состояние.
3. Не запускать альтернативный capture/refund с новым key, пока исход операции
   не определён.

Idempotency Stripe защищает только удалённую команду. Она не делает локальную
отправку письма, изменение заказа или вызов поставщика идемпотентными. Для каждого
эффекта нужен собственный уникальный business key.

---

## Сбои и компенсации

| Точка сбоя | Что уже произошло | Recovery |
| --- | --- | --- |
| Create PI ответил, DB не сохранила `pi_...` | Intent существует | Повторить create с тем же key и сохранить тот же ID |
| Клиент закрыл страницу после 3DS | Авторизация могла пройти | Webhook и reconciliation продолжают flow без клиента |
| Webhook получен дважды | Состояние Stripe одно, доставок несколько | Уникальный inbox key и идемпотентный business effect |
| Supplier timeout | Бронь могла создаться | Lookup по merchant reference; не повторять blind create |
| Supplier окончательно отказал | Средства authorized | Идемпотентный cancel, контроль до финального `canceled` |
| Capture timeout | Деньги могли быть captured | Повтор с тем же key и retrieve PI до новой команды |
| Capture окончательно не прошёл после брони | Бронь есть, оплаты нет | Высокоприоритетный retry/alert и компенсация брони по правилам поставщика |
| Authorization близка к expiry | Hold ещё существует | Не начинать долгую бронь либо отменить и запросить новый checkout |
| Процесс умер после локального commit | Команда записана, remote call не начат | Worker подбирает pending command |

Capture failure после успешной брони — главный остаточный риск manual capture.
Его нельзя спрятать под общий retry. Нужны отдельная метрика, короткий retry
interval, deadline до `capture_before` и manual operations queue.

---

## Рассрочка и составная бронь

Первый взнос, полная оплата и последующий installment — разные бизнес-события.
Один статус `paid` для них слишком грубый.

Например:

```text
deposit authorized -> booking allowed
deposit captured   -> booking financial condition satisfied
remaining due      -> заказ забронирован, но план ещё active
plan fully paid    -> финансовые обязательства закрыты
```

Если первый installment разрешает бронирование, это должно выражаться явным
правилом `booking_payment_condition_satisfied`, а не случайным присваиванием
`order.status = paid`. Иначе второй платёж, refund одного взноса и напоминания
становятся неразличимыми.

Для составного заказа нужно заранее решить порядок:

- бронировать все части, затем один capture;
- делать partial capture только при подтверждённой поддержке и ясной цене каждой
  части;
- при частичном успехе отменять уже созданные брони;
- принимать частично оказанный заказ и capture только подтверждённую сумму.

Это бизнес-решение, а не деталь Stripe SDK. Код должен отражать выбранную saga и
её компенсации.

---

## Когда manual capture не подходит

Manual capture не стоит выбирать автоматически в следующих случаях:

- payment method не поддерживает separate authorization and capture;
- поставщик отвечает дольше надёжного authorization window;
- поставщик требует captured-платёж до создания брони;
- команда не готова эксплуатировать capture deadlines и зависшие authorization;
- стоимость редких refund ниже сложности новой saga;
- асинхронный способ оплаты имеет другой жизненный цикл и правила возврата.

Код, который предлагает одновременно card и Klarna, не должен молча применять к
ним одинаковую политику. Stripe поддерживает separate capture для ряда методов,
включая Klarna, но окна и поведение отличаются. Настройки нужно проверять для
конкретного аккаунта, страны, валюты и flow.

В актуальном API можно задавать capture policy только для отдельных payment
method options. Альтернатива — разные `PaymentIntent` flows для card и методов с
другой семантикой. Это увеличивает код, зато не маскирует разные денежные
гарантии одним статусом.

---

## Минимальный Go-пример

Пример использует клиентский API `stripe-go/v82`. Он показывает только удалённые
команды; durable command table и переходы локальной машины состояний остаются
обязательной частью production-flow.

```go
package payments

import (
    "context"

    "github.com/stripe/stripe-go/v82"
)

type StripeGateway struct {
    client *stripe.Client
}

func NewStripeGateway(secretKey string) *StripeGateway {
    return &StripeGateway{client: stripe.NewClient(secretKey)}
}

func (g *StripeGateway) CreateManualIntent(
    ctx context.Context,
    paymentID string,
    amountMinor int64,
    currency string,
) (*stripe.PaymentIntent, error) {
    params := &stripe.PaymentIntentCreateParams{
        Amount:        stripe.Int64(amountMinor),
        Currency:      stripe.String(currency),
        CaptureMethod: stripe.String(string(stripe.PaymentIntentCaptureMethodManual)),
        PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
        Metadata: map[string]string{
            "payment_id": paymentID,
        },
    }
    params.SetIdempotencyKey("payment:" + paymentID + ":create:v1")

    return g.client.V1PaymentIntents.Create(ctx, params)
}

func (g *StripeGateway) Capture(
    ctx context.Context,
    paymentID string,
    intentID string,
) (*stripe.PaymentIntent, error) {
    params := &stripe.PaymentIntentCaptureParams{}
    params.SetIdempotencyKey("payment:" + paymentID + ":capture:v1")

    return g.client.V1PaymentIntents.Capture(ctx, intentID, params)
}

func (g *StripeGateway) Cancel(
    ctx context.Context,
    paymentID string,
    intentID string,
) (*stripe.PaymentIntent, error) {
    params := &stripe.PaymentIntentCancelParams{}
    params.SetIdempotencyKey("payment:" + paymentID + ":cancel:v1")

    return g.client.V1PaymentIntents.Cancel(ctx, intentID, params)
}
```

В реальном коде gateway внедряется через интерфейс. Unit-тесты подменяют его
fake-реализацией, а sandbox-тесты проверяют фактические статусы Stripe.

---

## Interview-ready answer

**1. Как провести оплату для бронирования, которое ещё может не подтвердиться?**

- Шаг 1 — создать `PaymentIntent` с manual capture и подтвердить его на клиенте.
- Шаг 2 — по durable webhook зафиксировать `authorized` и запустить booking saga.
- Шаг 3 — после подтверждения поставщика идемпотентно capture; при отказе cancel.

**2. Почему это всё равно saga, а не двухфазная транзакция?**

- Граница — Stripe, локальная БД и поставщик не участвуют в одном transaction coordinator.
- Надёжность — каждый удалённый шаг записывается как команда и безопасно повторяется.
- Компенсация — отказ брони освобождает authorization, а отказ после capture требует refund.

**3. Что делать после timeout на capture?**

- Запрет — не создавать новую capture-команду с новым ключом.
- Повтор — вызвать тот же endpoint с теми же параметрами и idempotency key.
- Сверка — получить текущее состояние `PaymentIntent` и завершить локальный transition.
