# Интеграция со Stripe

Stripe здесь рассматривается не как набор SDK-вызовов, а как внешняя платёжная
система со своей машиной состояний. Между локальным заказом, `PaymentIntent`,
бронированием у поставщика и webhook нет общей транзакции. Поэтому надёжная
интеграция строится как saga: каждый шаг фиксируется отдельно, повторяется
идемпотентно и имеет компенсацию.

Главный практический вопрос раздела — когда считать деньги полученными. При
автоматическом capture успешное подтверждение платежа сразу списывает деньги.
При `capture_method=manual` подтверждение только авторизует сумму, после чего
приложение либо захватывает её через capture, либо освобождает через cancel.

## Материалы

1. [PaymentIntent и жизненный цикл денег](./01-payment-intent-lifecycle.md) —
   роли `PaymentIntent`, `Charge`, `Refund` и `Event`, статусы, automatic capture
   против manual capture и локальная модель платежа.
2. [Authorization → booking → capture](./02-booking-authorization-capture.md) —
   рекомендуемый flow для бронирования, границы транзакций, идемпотентные команды,
   компенсации и Go-примеры.
3. [Webhooks, inbox и reconciliation](./03-webhooks-and-reconciliation.md) —
   проверка подписи, дубли, нарушенный порядок событий, durable inbox,
   обработка возвратов и сверка со Stripe.
4. [Локальное тестирование](./04-local-testing.md) — Stripe CLI, sandbox,
   тестовые способы оплаты, подписанные webhook и матрица отказов.
5. [Production-архитектура Stripe-интеграции](./production-architecture/README.md)
   — границы payment/booking-компонентов, структура Go-проекта, таблицы,
   контракты и короткие sequence diagram для каждого этапа workflow.

---

## Как читать

Если Stripe раньше не использовался, начать с жизненного цикла `PaymentIntent`.
Затем пройти booking-flow и webhooks: это две половины одной распределённой
операции. Локальное тестирование имеет смысл читать после них, потому что хороший
тест проверяет не отдельный HTTP 200, а переход денег и заказа через всю машину
состояний.

Production-архитектуру лучше читать последней: она собирает lifecycle,
booking-flow, webhook и reconciliation в одну структуру проекта и показывает,
какие компоненты и таблицы отвечают за каждый шаг.

---

## Быстрый выбор payment flow

| Сценарий | Базовый выбор | Почему |
| --- | --- | --- |
| Цифровой товар можно выдать сразу | automatic capture | Между оплатой и выдачей почти нет отказоустойчивого внешнего шага |
| Карта, а поставщик подтверждает бронь за минуты | manual capture | Сначала резервируются средства, после успешной брони выполняется capture |
| Поставщик отказал после авторизации | cancel `PaymentIntent` | Hold освобождается без отдельного refund |
| Деньги уже captured, услуга не оказана | Refund API | Capture необратим через cancel; нужна отдельная операция возврата |
| Способ оплаты не поддерживает нужный delayed capture | отдельный flow для этого способа | Нельзя переносить карточную семантику на все payment methods |
| Нужно сохранить способ оплаты без текущего списания | `SetupIntent` | `PaymentIntent` описывает платёж, `SetupIntent` — подготовку способа для будущего платежа |

`Manual capture` не является универсально правильным режимом. Выбор зависит от
поддержки конкретного payment method, длительности операции у поставщика,
авторизационного окна, правил бизнеса и цены компенсации. Если поставщик требует
уже оплаченный заказ, automatic capture с автоматическим refund может быть
осознанным trade-off, но это должно быть явно заложено в продукт и операции.

---

## Версии и официальные источники

В Go-коде лучше использовать явно внедрённый Stripe client, а major-версию SDK и
API version webhook endpoint обновлять вместе. Это убирает зависимость от
глобального ключа и позволяет проверять event payload contract fixtures.

Поведение платёжных методов и авторизационные окна меняются. Перед production
запуском нужно повторно проверить настройки аккаунта и официальные страницы:

- [PaymentIntent lifecycle](https://docs.stripe.com/payments/paymentintents/lifecycle)
- [Separate authorization and capture](https://docs.stripe.com/payments/place-a-hold-on-a-payment-method)
- [Payment method support](https://docs.stripe.com/payments/payment-methods/payment-method-support)
- [Webhook endpoint](https://docs.stripe.com/webhooks)
- [Idempotent requests](https://docs.stripe.com/api/idempotent_requests)
- [Testing](https://docs.stripe.com/testing)
- [Refunds](https://docs.stripe.com/refunds)
