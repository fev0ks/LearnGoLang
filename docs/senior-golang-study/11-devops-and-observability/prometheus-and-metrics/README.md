# Prometheus And Metrics

Этот подпакет про практическую работу с метриками:
- как данные доходят от приложения до `Prometheus`;
- какие бывают типы метрик и когда какой использовать;
- как устроен синтаксис `PromQL`;
- какие метрики обычно нужны для API, worker и storage-path;
- какие ошибки чаще всего делают с labels, cardinality и counters.

Как читать:
- сначала понять общий flow в `Prometheus`;
- потом разобрать типы метрик и дизайн сигналов;
- затем перейти к `PromQL`;
- после этого перейти в `practical-metric-patterns`;
- и только потом уже смотреть Grafana dashboards и alerting.

Материалы:

Foundation:
- [Prometheus Metrics Flow](./prometheus-metrics-flow.md)
- [How Prometheus Discovers And Scrapes Multiple Pods](./how-prometheus-discovers-and-scrapes-multiple-pods.md)
- [Prometheus Relabeling And Target Labels](./prometheus-relabeling-and-target-labels.md)
- [Prometheus UI And Grafana](./prometheus-ui-and-grafana.md)

Design:
- [Metric Types And Design](./metric-types-and-design.md)
- [Practical Metric Patterns](./practical-metric-patterns/README.md)

Querying:
- [PromQL Cheatsheet](./promql-cheatsheet.md)

Что важно уметь объяснить:
- почему `Prometheus` обычно scrapes, а не принимает push от приложений;
- чем `Counter`, `Gauge`, `Histogram` и `Summary` отличаются practically;
- почему `rate()` и `increase()` почти всегда нужны для counters;
- что такое label cardinality и как ею случайно убить observability stack;
- какие метрики реально нужны для `HTTP`, `Kafka`, `PostgreSQL`, `Redis` и workers;
- чем raw metric name отличается от operational question.
