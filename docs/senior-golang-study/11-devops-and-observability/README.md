# DevOps And Observability

Этот раздел нужен, потому что senior backend обычно отвечает и за production readiness.

Базовые заметки:
- [Kibana And Elasticsearch](./kibana-and-elasticsearch.md)
- [Kibana And Elasticsearch Cheatsheet](./kibana-and-elasticsearch-cheatsheet.md)
- [Logging And Log Shipping](./logging-and-log-shipping/README.md)

Темы:
- Docker multi-stage builds;
- CI/CD pipelines;
- Kubernetes basics для backend-разработчика;
- health checks, readiness, liveness;
- metrics, logs, traces;
- Prometheus, Grafana, OpenTelemetry;
- Kibana, Elasticsearch, log investigation;
- dashboards, alerts, runbooks;
- graceful shutdown и rollout strategy;
- feature flags и safe deployment patterns.

Что важно уметь объяснить:
- как понять, что сервис деградирует;
- какие метрики нужны для API и worker;
- как расследовать инцидент;
- как деплоить без лишнего риска.

## Подборка

- [Docker Multi-stage Builds](https://docs.docker.com/build/building/multi-stage/)
- [Kubernetes Concepts](https://kubernetes.io/docs/concepts/index.html)
- [Prometheus Overview](https://prometheus.io/docs/introduction/overview/)
- [OpenTelemetry Docs](https://opentelemetry.io/docs/)
- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
- [Google SRE Books](https://sre.google/books/)

## Вопросы

- какие сигналы нужны, чтобы считать сервис production-ready;
- как выбрать правильный набор RED и USE метрик;
- почему логи без trace correlation быстро перестают помогать;
- как различать readiness и liveness probes и почему их часто путают;
- как проводить rollout так, чтобы быстро остановить ущерб;
- что ты сделаешь первым при росте ошибок и latency после деплоя.
