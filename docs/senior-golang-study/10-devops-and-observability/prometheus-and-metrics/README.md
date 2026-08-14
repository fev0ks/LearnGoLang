# Prometheus и проектирование метрик

Раздел объясняет полный путь метрики: от изменения counter в Go-процессе до
service-level PromQL, dashboard и alert. Основной акцент — не на запоминании
синтаксиса, а на контрактах, cardinality, агрегации реплик и диагностике
production-сигналов.

## Как читать

1. [Путь метрики от приложения до alert](./02-prometheus-metrics-flow.md) —
   инструментация, scrape, ingestion, staleness и pull/push trade-offs.
2. [Как Prometheus обнаруживает и scrapes несколько pods](./06-how-prometheus-discovers-and-scrapes-multiple-pods.md) —
   Kubernetes discovery, ServiceMonitor/PodMonitor и rollout.
3. [Relabeling и labels scrape target](./03-prometheus-relabeling-and-target-labels.md) —
   target/metric relabeling, служебные labels и отладка.
4. [Типы метрик и их проектирование](./01-metric-types-and-design.md) —
   Counter, Gauge, classic/native Histogram, Summary и визуальное дерево выбора.
5. [PromQL: практическая шпаргалка](./05-promql-cheatsheet.md) — selectors,
   counter functions, aggregation, vector matching, histograms и missing data.
6. [Prometheus UI и Grafana](./04-prometheus-ui-and-grafana.md) — table-first
   диагностика, service/per-pod views и dashboards.
7. [Практические паттерны метрик](./practical-metric-patterns/README.md) — HTTP
   traffic/errors/latency, queues, connection pools, cache и вызовы хранилища.

Нумерация имён файлов историческая, поэтому рекомендуемый порядок чтения не
совпадает с номерами.

---

## Карта вопросов

| Вопрос | Материал |
| --- | --- |
| Где создаётся time series и откуда берётся timestamp? | Metrics flow |
| Почему пять pods дают много рядов одной метрики? | Discovery и UI/Grafana |
| Чем `relabel_configs` отличается от `metric_relabel_configs`? | Relabeling |
| Чем Counter, Gauge, Histogram и Summary отличаются на графике и в запросе? | Metric types |
| Почему `rate()` выполняют до `sum()`? | PromQL |
| Как выбрать buckets и посчитать SLO? | Latency histograms |
| Когда queue depth нужно суммировать, а когда брать `max`? | Gauges |
| Как связать HTTP p95 с pool wait и Postgres latency? | Storage metrics |

---

## Что нужно уметь после раздела

- сформулировать operational question до добавления метрики;
- оценить cardinality как произведение возможных label values, replicas и
  histogram series;
- объяснить reset counter и правильный порядок `rate` → `sum`;
- различить classic histogram, native histogram и Summary;
- построить service-level RPS, error ratio, p95 и SLO ratio;
- отделить отсутствие target, `up=0`, stale series и реальный ноль;
- диагностировать путь discovery → relabeling → scrape → ingestion → query;
- связать RED symptoms с USE/saturation и dependency metrics;
- объяснить, почему request ID, raw URL, SQL и тексты ошибок относятся в traces
  или logs, а не в metric labels.

---

## Официальная документация

- [Prometheus Overview](https://prometheus.io/docs/introduction/overview/)
- [Metric types](https://prometheus.io/docs/concepts/metric_types/)
- [Histograms and summaries](https://prometheus.io/docs/practices/histograms/)
- [Metric and label naming](https://prometheus.io/docs/practices/naming/)
- [Instrumentation best practices](https://prometheus.io/docs/practices/instrumentation/)
- [PromQL basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [PromQL functions](https://prometheus.io/docs/prometheus/latest/querying/functions/)
- [Prometheus configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)
- [Native histograms specification](https://prometheus.io/docs/specs/native_histograms/)
- [Prometheus Operator design](https://prometheus-operator.dev/docs/getting-started/design/)
- [Grafana Prometheus template variables](https://grafana.com/docs/grafana/latest/datasources/prometheus/template-variables/)
