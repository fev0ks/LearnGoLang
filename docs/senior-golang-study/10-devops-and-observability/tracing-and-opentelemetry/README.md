# Distributed Tracing и OpenTelemetry

Этот подпакет объясняет distributed tracing с позиции backend-разработчика: как один запрос превращается в дерево spans, где переносится trace context, как инструментировать Go-сервис и как расследовать проблему в Grafana Tempo.

После раздела нужно уметь:

- отличать `trace`, `span`, `SpanContext`, resource attributes и baggage;
- объяснить роли OpenTelemetry API, SDK, Collector, OTLP, Tempo и Grafana;
- не создавать второй HTTP server span поверх автоматической instrumentation;
- передавать context через HTTP, gRPC и message headers;
- выбирать между head sampling и tail sampling;
- связывать traces, metrics и logs через exemplars и `trace_id`;
- искать slow/error traces по TraceQL и читать waterfall без ложных выводов;
- выбирать stable semantic conventions, отделять собственные attributes и планировать их миграцию;
- объяснить, как RCA-agent читает telemetry и как трассировать работу самого агента.

## Материалы и порядок чтения

1. [Модель distributed tracing и flow OpenTelemetry](./01-opentelemetry-and-tracing-flow.md) — trace, span, propagation, OTLP, sampling и стоимость.
2. [OpenTelemetry в Go-сервисе](./02-opentelemetry-in-go-services.md) — bootstrap SDK, HTTP/gRPC instrumentation, manual spans, ошибки, goroutines и тесты.
3. [Сквозной пример trace](./03-end-to-end-trace-example.md) — реальные `trace_id`/`span_id`, synchronous и asynchronous boundaries, links и корреляция с logs.
4. [Tempo и расследование по traces](./04-tempo-and-trace-investigation.md) — workflow расследования, TraceQL, waterfall, service graphs и диагностика пропавших spans.
5. [AI-агенты для анализа телеметрии и RCA](./05-ai-agents-for-telemetry-analysis.md) — два контура `telemetry → agent` и `agent → telemetry`, роли LangGraph/LangChain/Langfuse, tools, guardrails и поэтапное внедрение.

---

## Связанные материалы

- [Prometheus и метрики](../prometheus-and-metrics/README.md) — aggregate-сигнал, RED-метрики и exemplars.
- [Logging и log shipping](../logging-and-log-shipping/README.md) — structured logs и корреляция по `trace_id`.
- [Incident response](../incident-response-and-investigation/README.md) — как встроить traces в общий investigation workflow.

---

## Официальная документация

- [OpenTelemetry: traces](https://opentelemetry.io/docs/concepts/signals/traces/)
- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
- [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/)
- [OpenTelemetry semantic conventions for GenAI](https://github.com/open-telemetry/semantic-conventions-genai)
- [OpenTelemetry Collector: Tail Sampling Processor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/tailsamplingprocessor)
- [OpenTelemetry Collector: Load Balancing Exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/loadbalancingexporter)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
- [Grafana Tempo](https://grafana.com/docs/tempo/latest/)
- [TraceQL](https://grafana.com/docs/tempo/latest/traceql/)
- [Grafana Tempo MCP server](https://grafana.com/docs/tempo/latest/api_docs/mcp-server/)
- [LangGraph](https://docs.langchain.com/oss/python/langgraph/overview)
- [Langfuse](https://langfuse.com/docs)
