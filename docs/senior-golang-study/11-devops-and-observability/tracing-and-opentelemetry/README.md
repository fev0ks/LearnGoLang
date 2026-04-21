# Tracing And OpenTelemetry

Этот подпакет про practical tracing:
- что такое `trace`, `span`, `context propagation`;
- зачем нужен `OpenTelemetry` и где его роль заканчивается;
- как `Tempo` хранит traces и чем он отличается от `Prometheus` и `Loki`;
- как расследовать latency и error path через `Grafana + Tempo`.

Как читать:
- сначала открыть practical example про push model, `trace_id` и spans;
- потом понять общую trace model;
- после этого посмотреть, как `OpenTelemetry` встраивается в Go-сервис;
- затем разбирать `Tempo` и trace investigation workflow.

Материалы:

Practical first:
- [Push Model, TraceId And Spans Example](./04-push-model-traceid-and-spans-example.md)

Foundation:
- [OpenTelemetry And Tracing Flow](./01-opentelemetry-and-tracing-flow.md)

Instrumentation:
- [OpenTelemetry In Go Services](./02-opentelemetry-in-go-services.md)

Investigation:
- [Tempo And Trace Investigation](./03-tempo-and-trace-investigation.md)

Что важно уметь объяснить:
- чем `trace` отличается от `metric` и `log`;
- что такое `span` и parent/child relationship;
- как propagation идет через `HTTP`, `Kafka` и другие transport boundaries;
- зачем нужен `OTLP` и чем `OpenTelemetry` отличается от backend storage;
- как найти медленный downstream вызов в `Tempo`;
- почему traces не заменяют logs и metrics, а дополняют их.
