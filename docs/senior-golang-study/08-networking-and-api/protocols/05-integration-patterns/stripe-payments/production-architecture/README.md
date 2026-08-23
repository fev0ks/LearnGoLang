# Sequence diagram production-flow

Здесь production-архитектура Stripe-интеграции разобрана от общего устройства до
конкретного порядка локальных транзакций, удалённых вызовов, webhook и фоновой
обработки.

## Материалы

0. [Production-архитектура Stripe-интеграции](./00-production-architecture.md) —
   границы сервисов, структура Go-проекта, контракты, таблицы и инварианты.
1. [Checkout и создание PaymentIntent](./01-checkout-and-payment-intent.md) —
   доверенная цена, локальная operation, Stripe create и confirm на клиенте.
2. [Webhook, inbox и outbox](./02-webhook-inbox-and-outbox.md) — durable приём,
   worker transaction и доставка внутреннего события.
3. [Authorization, booking и capture](./03-booking-and-capture.md) — вызов
   поставщика, capture при успехе и cancel при отказе.
4. [Сбои и reconciliation](./04-failures-and-reconciliation.md) — recovery после
   crash, повтор с прежним key и восстановление потерянного webhook.
5. [Webhook retry и 30-минутный prebooking](./05-webhook-retry-and-prebooking-deadline.md)
   — почему нельзя ждать повтор Stripe, как сверить PaymentIntent до business
   deadline и что делать с поздним событием.

## Как читать схемы

- `BEGIN` и `COMMIT` показывают реальную границу DB-транзакции.
- Stripe, broker и supplier всегда находятся вне открытой DB-транзакции.
- Пунктирная стрелка означает ответ, acknowledgement или асинхронную доставку.
- Одинаковый idempotency key при retry обозначает ту же операцию, а не новую.
- Каждая схема намеренно ограничена несколькими участниками, чтобы подписи не
  становились мелкими при отображении Markdown.

После четырёх разборов должно быть понятно не только что вызывает система, но и
какой durable state уже существует в каждой возможной точке сбоя.
