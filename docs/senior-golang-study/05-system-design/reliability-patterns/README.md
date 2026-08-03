# Reliability Patterns

Паттерны надёжности: как сервис переживает нагрузку, изолирует отказы зависимостей и не превращает чужую поломку в свою.

Материалы делятся на две части. Первые семь — механизмы в коде, работающие в момент отказа. Последние три — практики вокруг кода: как измерять надёжность, что делать после инцидента и как искать слабые места до него.

Порядок в первой части не случаен и отражает уровни защиты. Таймаут ограничивает одно ожидание. Повтор компенсирует кратковременный сбой. Предохранитель прекращает бессмысленные попытки. Ограничение частоты и отбрасывание нагрузки защищают от превышения ёмкости. Идемпотентность делает повторы безопасными. Переборка не даёт одной зависимости занять все ресурсы.

---

## Материалы

- [01 Timeouts и Deadlines](./01-timeouts-and-deadlines.md) — deadline propagation, latency budget, контекст с таймаутом на каждый внешний вызов
- [02 Retries и Backoff](./02-retries-and-backoff.md) — exponential backoff + jitter, retry budget, retry amplification
- [03 Circuit Breaker](./03-circuit-breaker.md) — состояния, gobreaker, предотвращение cascade failure
- [04 Rate Limiting](./04-rate-limiting.md) — token bucket, sliding window, distributed RL через Redis
- [05 Backpressure и Load Shedding](./05-backpressure-and-shedding.md) — bounded queues, semaphore, 503 как сигнал, graceful degradation
- [06 Idempotency](./06-idempotency.md) — idempotency keys, at-least-once delivery, дедупликация в PostgreSQL
- [07 Bulkhead](./07-bulkhead.md) — изоляция пулов по зависимостям, semaphore per downstream
- [08 SLO/SLI и Error Budgets](./08-slo-sli-error-budgets.md) — SLI/SLO/SLA, error budget, burn rate alerting, multi-window alerts
- [09 Постмортемы](./09-postmortem.md) — blameless culture, 5 Whys, структура и шаблон, action items
- [10 Chaos Engineering](./10-chaos-engineering.md) — намеренные сбои в prod, blast radius, game days, инструменты (Chaos Monkey, Toxiproxy), Go-примеры

---

## Как эти механизмы сочетаются

Порядок обёрток вокруг вызова задан их назначением и меняться не должен:

```
запрос
  → переборка      (есть ли у нас ресурсы на этот вызов?)
    → предохранитель (отвечает ли зависимость вообще?)
      → повторы      (можно ли пережить единичный сбой?)
        → таймаут    (сколько ждём одну попытку?)
          → вызов
```

Нарушения этого порядка дают предсказуемые эффекты. Повторы снаружи предохранителя расходуют все попытки на мгновенные отказы разомкнутой цепи. Переборка внутри предохранителя засчитывает собственную перегрузку как отказ зависимости и размыкает цепь к исправному сервису. Повтор без таймаута на попытку зависает на первой же.

Отдельно стоит идемпотентность: она не встраивается в цепочку, а является условием, при котором повторы вообще допустимы.

---

## Вопросы

- как deadline propagation спасает от latency amplification в цепочке сервисов
- почему retry без jitter создаёт thundering herd
- как circuit breaker отличается от простой проверки ошибок
- когда load shedding лучше чем backpressure
- как идемпотентный ключ защищает от дублей при сетевом сбое
- почему один общий goroutine pool для всех зависимостей опасен
- чем SLO отличается от SLA, как считать error budget
- зачем нужен burn rate alerting вместо threshold-алертов
- почему blameless подход в постмортемах эффективнее наказаний
- как 5 Whys помогает дойти до root cause
- зачем намеренно ломать production и как контролировать blast radius
- что такое game day и чем он отличается от обычного chaos test
