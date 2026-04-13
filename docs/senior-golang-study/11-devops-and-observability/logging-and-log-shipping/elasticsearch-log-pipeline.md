# Elasticsearch Log Pipeline

`Elasticsearch` используют, когда нужен удобный поиск по логам, фильтрация по полям и агрегации поверх большого потока structured events.

## Базовый путь

Чаще всего схема такая:

```text
application -> stdout/file -> Fluent Bit/Vector/Logstash -> Elasticsearch -> Kibana
```

Иногда между collector и `Elasticsearch` вставляют `Kafka`, если:
- объёмы очень большие;
- нужен decoupling;
- ingestion в `Elasticsearch` может временно проседать.

## Как это работает по шагам

1. Приложение пишет JSON-лог.
2. Collector получает запись из `stdout` или файла.
3. Collector парсит JSON и добавляет metadata:
   `service`, `env`, `host`, `pod`, `container`, `region`.
4. Collector отправляет документ в `Elasticsearch`.
5. `Elasticsearch` индексирует документ.
6. `Kibana` ищет по индексам и строит dashboards.

## Почему часто используют Fluent Bit

`Fluent Bit`:
- лёгкий;
- хорошо подходит как node-level collector;
- умеет parsing, filtering, buffering и output в `Elasticsearch`;
- дешевле по ресурсам, чем `Logstash`.

`Logstash` используют, когда нужна более тяжёлая обработка:
- сложные parsing pipelines;
- enrichment;
- legacy integrations.

## Что важно для Elasticsearch

### Mapping

Нужно заранее понимать, какие поля:
- `keyword`
- `text`
- `date`
- `long`, `double`, `boolean`

Если schema management плохой:
- получаются кривые mappings;
- поиск и агрегации работают хуже;
- storage растёт слишком быстро.

### Index Lifecycle

Обычно логи хранят не в одном большом индексе, а в rollover-индексах или data streams.

Частый подход:
- daily или size-based rollover;
- `hot` tier для свежих данных;
- `warm` или `cold` tier для старых;
- удаление по retention policy.

### Structured Fields

Хорошие поля для расследования:
- `service.name`
- `service.version`
- `env`
- `log.level`
- `trace.id`
- `request_id`
- `operation`
- `error.type`
- `error.kind`
- `http.method`
- `url.path`
- `http.status_code`

## Когда Elasticsearch хорош

Подходит, когда:
- нужен сильный full-text search;
- нужны aggregations и dashboards;
- много structured fields;
- нужно быстро искать по `request_id`, `trace_id`, `user_id`, `error_kind`.

## Когда Elasticsearch дорогой или неудобный

Плохо подходит, когда:
- логов очень много, а бюджет ограничен;
- всё подряд индексируется без разбора;
- retention нужен на месяцы и годы в searchable виде;
- команда не готова поддерживать mappings, ILM и capacity planning.

## Типичный production pipeline

### В Docker или Kubernetes

```text
Go service -> stdout JSON
Kubernetes/Docker runtime -> container logs
Fluent Bit DaemonSet -> parse, enrich, buffer
Elasticsearch/OpenSearch -> index
Kibana -> search
```

### С промежуточной Kafka

```text
Go service -> stdout
Fluent Bit -> Kafka
Kafka -> Logstash/Vector
Logstash/Vector -> Elasticsearch
Kibana
```

Это усложняет систему, но даёт:
- буферизацию;
- reprocessing;
- decoupling ingest от storage.

## Частые failure modes

`Elasticsearch slow or red`:
- collector buffers растут;
- часть логов может дропаться;
- растёт lag.

`bad mappings`:
- `status_code` случайно становится `text`;
- exact filters и aggregations ломаются.

`too much cardinality`:
- уникальные значения в огромном количестве полей;
- рост индекса и memory pressure.

`oversharding`:
- слишком много маленьких индексов и shard'ов;
- cluster начинает страдать даже при умеренном объёме данных.

## Что важно объяснить на интервью

- почему приложение лучше не подключать напрямую к `Elasticsearch`;
- зачем нужен collector;
- почему `keyword` и `text` нельзя путать;
- зачем нужны rollover, retention и hot/warm/cold tiers;
- когда лучше выбрать `Loki` вместо `Elasticsearch`.
