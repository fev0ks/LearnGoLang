# Practical Metric Patterns

Этот подпакет нужен, чтобы разбирать метрики не по абстрактным типам, а по operational use case.

Идея простая:
- одна заметка = один тип сигнала;
- внутри есть `что это`, `как выглядит в Grafana`, `какие PromQL-запросы писать`, `что считается норм`, `что считается деградацией`.

Материалы:
- [HTTP Request Rate Counters](./http-request-rate-counters.md)
- [HTTP Error Rate](./http-error-rate.md)
- [Latency Histograms](./latency-histograms.md)
- [Gauges: In-Flight, Queue Depth, Current State](./gauges-inflight-queue-depth.md)
- [Storage Operation Metrics](./storage-operation-metrics.md)

Как читать:
- сначала `HTTP Request Rate Counters`;
- потом `HTTP Error Rate`;
- потом `Latency Histograms`;
- после этого `Gauges`;
- в конце `Storage Operation Metrics`.

Что важно уметь после этого блока:
- смотреть на панель и понимать, она про throughput, errors или latency;
- отличать “signal двигается” от “signal деградирует”;
- не путать raw counter с rate;
- не пытаться читать histogram bucket как готовый latency answer;
- понимать, какую панель строить для API, worker, Redis, Postgres и Kafka path.
