# DevOps And Observability

Этот раздел нужен, потому что senior backend обычно отвечает и за production readiness.

Базовые заметки:
- [Logging And Log Shipping](./logging-and-log-shipping/README.md)
- [Prometheus And Metrics](./prometheus-and-metrics/README.md)
- [Tracing And OpenTelemetry](./tracing-and-opentelemetry/README.md)
- [Incident Investigation And Profiling](./incident-investigation-and-profiling/README.md)
- [Kubernetes](./kubernetes/README.md)
- [Docker](./docker/README.md)
- [Docker Compose](./docker-compose/README.md)
- [Dockerfiles For Go](./dockerfiles-for-go/README.md)

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

Структура раздела:
- `logging-and-log-shipping` - пайплайны логов, log platforms, Kibana/Elasticsearch и log investigation
- `prometheus-and-metrics` - как работает flow метрик, типы метрик, PromQL и практический metric design
- `tracing-and-opentelemetry` - как устроены traces, OpenTelemetry instrumentation, propagation и Tempo investigation
- `incident-investigation-and-profiling` - как искать production проблемы, читать профили и отличать network issue от app issue
- `kubernetes` - базовые сущности, rollout, failover, конфиги и что реально спрашивают на интервью
- `docker` - image/container model, сети, volumes, runtime-практика для Go-сервисов
- `docker-compose` - локальные multi-service стеки, profiles, healthchecks, примеры compose-файлов
- `dockerfiles-for-go` - production/dev Dockerfile patterns для Go-проектов

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
