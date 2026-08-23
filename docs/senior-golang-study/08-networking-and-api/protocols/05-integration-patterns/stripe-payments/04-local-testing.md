# Локальное тестирование Stripe-интеграции

## Содержание

- [Три уровня тестов](#три-уровня-тестов)
- [Stripe CLI и локальный webhook](#stripe-cli-и-локальный-webhook)
- [Транспортный тест и end-to-end](#транспортный-тест-и-end-to-end)
- [Тестовые способы оплаты](#тестовые-способы-оплаты)
- [Проверка подписанного webhook в Go](#проверка-подписанного-webhook-в-go)
- [Матрица сценариев](#матрица-сценариев)
- [Тестирование сбоев](#тестирование-сбоев)
- [Checklist перед production](#checklist-перед-production)
- [Interview-ready answer](#interview-ready-answer)

Sandbox позволяет проверить настоящую машину состояний Stripe без движения
денег. Но один успешный платёж картой `4242` проверяет только happy path. Для
booking-flow важнее доказать, что authorization не превращается в ранний capture,
дубли webhook не дублируют эффекты, а timeout восстанавливается тем же ключом.

---

## Три уровня тестов

| Уровень | Что подменяется | Что проверяется |
| --- | --- | --- |
| Unit | Stripe gateway и supplier gateway | Локальные переходы, idempotency keys, суммы, compensation decisions |
| Webhook contract | Stripe API не вызывается, payload подписан тестовым secret | Raw body, signature, inbox conflict, HTTP-коды, routing |
| Sandbox end-to-end | Только реальные деньги заменены sandbox | 3-D Secure, `requires_capture`, capture/cancel/refund и настоящие events |

Unit-тесты должны быть быстрыми и покрывать большинство переходов. Sandbox
подтверждает контракт с реальным Stripe, но не заменяет unit-тесты: внешний
сервис медленнее, имеет rate limits и не даёт удобно воспроизвести каждый crash.

Stripe отдельно предупреждает не использовать testing environment для load
testing. Нагрузку inbox и worker проверяют локальными генераторами событий и fake
gateway, а не тысячами sandbox API calls.

---

## Stripe CLI и локальный webhook

После установки CLI:

```bash
stripe login
```

Локальный listener принимает snapshot events и пересылает их приложению:

```bash
stripe listen \
    --events payment_intent.amount_capturable_updated,payment_intent.succeeded,payment_intent.payment_failed,payment_intent.canceled,refund.created,refund.updated,refund.failed \
    --forward-to localhost:8080/webhooks/stripe
```

CLI напечатает отдельный signing secret:

```text
Ready! Your webhook signing secret is 'whsec_...'
```

Именно это значение временно задаётся локальному приложению:

```bash
export STRIPE_WEBHOOK_SECRET='whsec_from_stripe_listen'
```

Secret зарегистрированного Dashboard endpoint здесь не сработает. Secret API
ключа (`sk_test_...`) и webhook signing secret (`whsec_...`) также решают разные
задачи и не заменяют друг друга.

В отдельном терминале можно проверить транспорт и routing:

```bash
stripe trigger payment_intent.succeeded
```

Для manual capture полезен и capturable event:

```bash
stripe trigger payment_intent.amount_capturable_updated
```

---

## Транспортный тест и end-to-end

`stripe trigger` создаёт fixture со своими object IDs и metadata. Такой event
может не соответствовать локальному `order_id`, поэтому он хорошо проверяет
маршрут, подпись и общий handler, но не обязательно завершает бизнес-flow.

Полный end-to-end проходит через обычный API приложения:

1. Создать локальный заказ и `Payment`.
2. Получить `PaymentIntent` с `capture_method=manual`.
3. Подтвердить его через Stripe Elements или тестовый `PaymentMethod`.
4. Убедиться, что intent имеет `requires_capture`,
   `amount_capturable == expected_amount`, `amount_received == 0`.
5. Дождаться обработки `payment_intent.amount_capturable_updated`.
6. Смоделировать успех поставщика и выполнить capture.
7. Убедиться, что intent стал `succeeded`, а локальный payment — `captured`.

В отрицательной ветке после пункта 5 поставщик отказывает, приложение вызывает
cancel, intent становится `canceled`, а Refund не создаётся.

Проверка `amount_received == 0` до capture принципиальна: один status без денежных
полей может скрыть ошибочное automatic capture.

---

## Тестовые способы оплаты

В server-side тестовом коде Stripe рекомендует использовать готовые
`PaymentMethod` IDs, а не передавать номера карт напрямую.

| Сценарий | PaymentMethod |
| --- | --- |
| Успешная Visa | `pm_card_visa` |
| Общий decline | `pm_card_visa_chargeDeclined` |
| Недостаточно средств | `pm_card_visa_chargeDeclinedInsufficientFunds` |
| Ошибка обработки | `pm_card_visa_chargeDeclinedProcessingError` |

Интерактивный checkout через Elements дополнительно проверяется тестовыми
номерами:

| Сценарий | Номер |
| --- | --- |
| Обычный успех | `4242 4242 4242 4242` |
| 3-D Secure обязателен и успешен | `4000 0000 0000 3220` |
| 3-D Secure обязателен, затем decline | `4000 0084 0000 1629` |
| Недостаточно средств | `4000 0000 0000 9995` |

Используются любая будущая дата и допустимый CVC. Реальные карты и live keys в
тестах запрещены. Актуальный список нужно брать из
[официальной страницы Testing](https://docs.stripe.com/testing), потому что
сценарии и идентификаторы могут меняться.

---

## Проверка подписанного webhook в Go

`stripe-go` содержит helper для создания тестовой подписи. Тест передаёт handler
те же raw bytes, которые были подписаны.

```go
func TestWebhookStoresSignedEvent(t *testing.T) {
    secret := "whsec_test"
    payload := []byte(`{
        "id":"evt_test_1",
        "object":"event",
        "api_version":"` + stripe.APIVersion + `",
        "type":"payment_intent.amount_capturable_updated",
        "data":{"object":{"id":"pi_test_1","object":"payment_intent"}}
    }`)

    signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
        Payload: payload,
        Secret:  secret,
    })

    req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
    req.Header.Set("Stripe-Signature", signed.Header)
    rec := httptest.NewRecorder()

    handler(secret).ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status: got %d, want 200", rec.Code)
    }
}
```

Отдельные тесты меняют один байт payload после подписи и ожидают `400`, а также
подменяют inbox repository ошибкой и ожидают `5xx`. Это защищает самую важную
границу acknowledgement.

---

## Матрица сценариев

| Сценарий | Проверяемое состояние Stripe | Локальный инвариант |
| --- | --- | --- |
| Успешная authorization | `requires_capture`, capturable amount совпадает | Booking разрешён, `captured_amount = 0` |
| Успешный capture | `succeeded`, received amount совпадает | Одна capture command, payment `captured` |
| Отказ поставщика | После cancel intent `canceled` | Нет Refund, hold больше не active |
| Card decline | `requires_payment_method` | Заказ не `paid`, можно дать новую попытку |
| 3-D Secure | Сначала `requires_action`, затем `requires_capture` | Booking не стартует до authorization |
| Дубликат одного `evt_...` | Stripe object не меняется | Ровно один booking/email/analytics effect |
| Два events об одном эффекте | Object ID одинаков | Business unique key не даёт второй effect |
| Out-of-order event | Доставка переставлена | Финальное состояние не откатывается назад |
| Partial refund | Refunded amount меньше captured | `partially_refunded`, заказ не отменён целиком автоматически |
| Refund failure | Refund `failed` | Payment не помечен `refunded`, создан alert |
| Authorization expiry | Intent `canceled` | Booking не начинается после deadline |

Тест считается сильным, если проверяет и provider state, и локальную базу, и
число внешних эффектов. Один HTTP 200 ничего не говорит о correctness платежа.

---

## Тестирование сбоев

### Crash после удалённого ответа

Fake gateway возвращает успешный create/capture, а repository падает до
сохранения результата. Повторный worker должен использовать тот же idempotency
key. В sandbox этот сценарий дополнительно сверяют retrieval по сохранённому
object ID или результатом повторного запроса.

### Два параллельных webhook

Две goroutine одновременно отправляют один подписанный payload. В inbox должна
остаться одна запись, business effect выполняется один раз, оба HTTP-запроса
получают `200`.

### DB недоступна на приёме

Handler не должен скрывать ошибку: если `INSERT inbox` не закоммичен, ответ
остаётся `5xx`. После восстановления повтор события создаёт inbox record.

### Worker временно упал

HTTP уже может быть `200`, потому что inbox durable. Запись получает
`next_attempt_at`, повторно claim-ится и не блокирует новые события.

### Неоднозначный timeout поставщика

Перед повторным create booking выполняется lookup по merchant reference. Тест
должен показать, что timeout после фактического успеха поставщика не создаёт
вторую бронь.

---

## Checklist перед production

- Test keys, live keys и оба signing secrets разделены по окружениям.
- Endpoint получает raw body, ограничивает размер и проверяет подпись.
- В Dashboard подписаны только нужные event types.
- `2xx` выдаётся только после durable inbox commit.
- Duplicate и out-of-order тесты выполняются конкурентно.
- Проверены 3-D Secure, decline, processing и authorization expiry.
- Capture, cancel и refund имеют разные стабильные idempotency keys.
- Проверены partial refund и refund failure.
- Есть reconciliation job и alert до `capture_before`.
- Есть operations runbook для «бронь есть, capture не подтверждён».
- Ни `sk_live_...`, ни `sk_test_...`, ни `whsec_...`, ни `client_secret` не
  попадают в репозиторий и логи.

---

## Interview-ready answer

**1. Как локально проверить Stripe webhook?**

- Listener — `stripe listen --forward-to` пересылает sandbox events и выдаёт отдельный `whsec_...`.
- Trigger — `stripe trigger` проверяет транспорт и routing.
- End-to-end — реальный sandbox `PaymentIntent` через приложение проверяет metadata и всю машину состояний.

**2. Какие сценарии важнее happy path?**

- Деньги — authorization без capture, decline, 3-D Secure, expiry и partial refund.
- Доставка — duplicate, out-of-order, invalid signature и DB outage до inbox commit.
- Recovery — crash после удалённого успеха и повтор с тем же idempotency key.

**3. Почему нельзя нагрузочно тестировать через Stripe sandbox?**

- Ограничение — testing environment имеет rate limits и не предназначен для генератора нагрузки.
- Замена — throughput inbox и workers проверяется локально с fake gateway.
- Contract — небольшое число sandbox-тестов оставляют для реального Stripe lifecycle.
