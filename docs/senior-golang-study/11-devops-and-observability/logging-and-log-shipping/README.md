# Logging And Log Shipping

Этот подпакет про практическую сторону логов:
- как они двигаются от приложения до системы поиска;
- чем отличаются `Elasticsearch`, `Loki` и облачные пайплайны;
- где нужен collector вроде `Fluent Bit`, `Vector`, `Logstash` или `OpenTelemetry Collector`;
- как выбирать между online-search и cold storage.

Материалы:
- [Logs Pipeline Overview](./logs-pipeline-overview.md)
- [Elasticsearch Log Pipeline](./elasticsearch-log-pipeline.md)
- [Loki Log Pipeline](./loki-log-pipeline.md)
- [Cloud Log Delivery: AWS And Google Cloud](./cloud-log-delivery-aws-and-google-cloud.md)

Что важно уметь объяснить:
- почему приложение обычно не должно писать прямо в `Elasticsearch`;
- зачем нужен shipper или collector;
- чем index-based поиск отличается от label-based;
- почему `S3` и `GCS` хороши для архива, но не заменяют нормальный log query engine;
- где главные риски: потеря логов, backpressure, cardinality, retention cost, noisy fields.
