# Практические паттерны метрик

Здесь типы метрик рассматриваются через operational questions: есть ли трафик,
ошибаются ли пользователи, растёт ли latency, накапливается ли работа и какая
зависимость создаёт симптом.

## Как читать

1. [Request rate: сколько трафика получает HTTP API](./01-http-request-rate-counters.md)
2. [HTTP error rate: абсолютные ошибки, ratio и SLO](./02-http-error-rate.md)
3. [Latency histograms: среднее, квантили и границы SLO](./03-latency-histograms.md)
4. [Gauges: in-flight, queue depth и текущее состояние](./04-gauges-inflight-queue-depth.md)
5. [Метрики операций с хранилищем](./05-storage-operation-metrics.md)

Первые три статьи образуют RED-view для request-driven сервиса. Gauges
добавляют saturation и capacity context. Storage metrics связывают
пользовательский симптом с клиентским вызовом Postgres, Redis или другой
зависимости.

---

## Общий порядок расследования

```mermaid
flowchart LR
    R["Rate<br/>изменился профиль трафика?"] --> E["Errors<br/>кто и как ошибается?"]
    E --> D["Duration<br/>где вырос tail?"]
    D --> S["Saturation<br/>есть очередь или лимит?"]
    S --> DEP["Dependencies<br/>какая операция тормозит?"]
```

Схема задаёт порядок проверки, а не утверждает единственную причинность. Можно
начать с любого известного симптома, но вывод подтверждают несколькими
независимыми сигналами.

После блока нужно уметь:

- отличать raw counter от скорости и числа событий за окно;
- читать error rate вместе с ratio и объёмом трафика;
- не усреднять percentiles и выбирать buckets рядом с SLO;
- проверять владельца gauge перед `sum()`;
- связывать HTTP latency с operation-level latency и saturation;
- переходить от dashboard к traces и logs, не помещая request-level IDs в
  metric labels.
