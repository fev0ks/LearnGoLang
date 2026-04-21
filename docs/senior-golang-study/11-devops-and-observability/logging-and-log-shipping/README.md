# Logging And Log Shipping

Этот подпакет про практическую сторону логов:
- как они двигаются от приложения до системы поиска;
- чем отличаются `Elasticsearch`, `Loki` и облачные пайплайны;
- где нужен collector вроде `Fluent Bit`, `Vector`, `Logstash` или `OpenTelemetry Collector`;
- как выбирать между online-search и cold storage;
- как расследовать инциденты в `Kibana` и работать с `Elasticsearch`.

Как читать:
- сначала понять общую схему движения логов;
- затем сравнить основные backend-платформы;
- после этого переходить к `Kibana`/`Elasticsearch` и практическому расследованию;
- в конце смотреть облачные варианты и выбор collector'ов.

Материалы:

Foundation:
- [Logs Pipeline Overview](./01-logs-pipeline-overview.md)
- [Logging In Go And Why Wrap Logger](./02-logging-in-go-and-why-wrap-logger.md)

Platforms:
- [Elasticsearch Log Pipeline](./04-elasticsearch-log-pipeline.md)
- [Loki Log Pipeline](./07-loki-log-pipeline.md)
- [Log Platforms Comparison Table](./03-log-platforms-comparison-table.md)

Investigation:
- [Grafana Overview And Functionality](./08-grafana-overview-and-functionality.md)
- [Kibana And Elasticsearch](./05-kibana-and-elasticsearch.md)
- [Kibana And Elasticsearch Cheatsheet](./06-kibana-and-elasticsearch-cheatsheet.md)
- [Grafana vs Kibana And Similar Tools](./09-grafana-vs-kibana-and-similar-tools.md)

Delivery And Collectors:
- [Cloud Log Delivery: AWS And Google Cloud](./11-cloud-log-delivery-aws-and-google-cloud.md)
- [Promtail vs Grafana Alloy vs Fluent Bit](./10-promtail-vs-grafana-alloy-vs-fluent-bit.md)

Что важно уметь объяснить:
- почему приложение обычно не должно писать прямо в `Elasticsearch`;
- зачем нужен shipper или collector;
- чем index-based поиск отличается от label-based;
- как различать `text`, `keyword`, `match`, `term` и `wildcard` при расследовании по логам;
- почему `S3` и `GCS` хороши для архива, но не заменяют нормальный log query engine;
- где главные риски: потеря логов, backpressure, cardinality, retention cost, noisy fields.
