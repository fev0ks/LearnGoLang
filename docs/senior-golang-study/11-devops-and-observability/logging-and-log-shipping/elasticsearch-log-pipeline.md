# Elasticsearch Log Pipeline

Этот файл про то, как логи попадают в `Elasticsearch` и что важно для стабильного ingestion/storage. Поиск в `Kibana`, `KQL`, `DSL`, `text` vs `keyword` и расследование инцидентов разобраны отдельно в [Kibana And Elasticsearch](./kibana-and-elasticsearch.md).

## Содержание

- [Когда этот стек вообще выбирают](#когда-этот-стек-вообще-выбирают)
- [Базовая схема](#базовая-схема)
- [Роли компонентов](#роли-компонентов)
- [Почему часто используют `Fluent Bit`](#почему-часто-используют-fluent-bit)
- [Что важно для стабильного Elasticsearch ingestion](#что-важно-для-стабильного-elasticsearch-ingestion)
- [Какие поля обычно полезны](#какие-поля-обычно-полезны)
- [Частые production topologies](#частые-production-topologies)
- [Частые failure modes](#частые-failure-modes)
- [Practical rule](#practical-rule)
- [Что важно объяснить на интервью](#что-важно-объяснить-на-интервью)
- [Связанные темы](#связанные-темы)

## Когда этот стек вообще выбирают

`Elasticsearch` или `OpenSearch` обычно берут, когда:
- нужен сильный full-text search;
- важны aggregations по полям;
- команда хочет document-oriented log investigation workflow;
- searchable storage важнее, чем минимальная стоимость ingest.

## Базовая схема

Частый production path:

```text
application -> stdout/file -> collector -> Elasticsearch -> Kibana
```

Типичный вариант в контейнерной среде:

```text
Go service -> stdout JSON
container runtime -> container logs
Fluent Bit/Vector/Logstash -> Elasticsearch/OpenSearch
Kibana -> search and dashboards
```

Если ingestion и storage нужно развязать:

```text
application -> collector -> Kafka -> processor -> Elasticsearch
```

Это дороже по сложности, но дает:
- буферизацию;
- decoupling;
- reprocessing;
- более мягкое поведение при просадке storage.

## Роли компонентов

`application`:
- пишет structured logs;
- не должна знать детали backend storage;
- обычно пишет в `stdout`.

`collector`:
- забирает логи;
- парсит и обогащает metadata;
- буферизует и ретраит;
- отправляет в `Elasticsearch`.

`Elasticsearch`:
- индексирует документы;
- хранит searchable data;
- отвечает за mappings, shards, lifecycle и query performance.

`Kibana`:
- UI для поиска, dashboards и incident investigation.

## Почему часто используют `Fluent Bit`

`Fluent Bit`:
- легкий;
- хорошо работает как node-level agent;
- умеет parsing, filtering, buffering и output в `Elasticsearch`;
- обычно дешевле по ресурсам, чем `Logstash`.

`Logstash` чаще нужен, когда:
- требуется тяжелый pipeline processing;
- нужно сложное enrichment;
- есть legacy integrations;
- pipeline строится вокруг более "ETL-like" обработки логов.

## Что важно для стабильного Elasticsearch ingestion

### 1. Mapping discipline

Нужно заранее понимать типы полей:
- `keyword`
- `text`
- `date`
- `long`, `double`, `boolean`

Плохой mapping почти всегда приводит к проблемам:
- filter и aggregations работают не так, как ожидалось;
- storage растет слишком быстро;
- query latency становится непредсказуемой.

### 2. Index lifecycle

Логи почти никогда не стоит хранить в одном большом индексе.

Обычно используют:
- rollover по времени или размеру;
- `hot` tier для свежих данных;
- `warm`/`cold` tier для старых;
- retention policy на удаление.

### 3. Shards and index sizing

Одна из самых частых operational проблем:
- слишком много маленьких индексов и shard'ов;
- cluster тратит ресурсы на metadata и coordination вместо нормальной работы.

Практически важно:
- не плодить индексы без необходимости;
- не создавать новый индекс на каждую мелочь;
- думать о rollover и shard count заранее.

### 4. Cardinality control

Опасные поля:
- почти уникальные значения;
- noisy dimensions;
- бесконтрольные user-generated identifiers как indexed fields everywhere.

Это бьет по:
- storage;
- memory;
- query performance;
- aggregation cost.

## Какие поля обычно полезны

Хороший минимальный набор для searchable logs:
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

Если structured fields слабые, даже хороший `Elasticsearch` не даст хорошего опыта расследования.

## Частые production topologies

### Kubernetes / Docker

```text
Go service -> stdout JSON
runtime container logs
Fluent Bit DaemonSet
Elasticsearch/OpenSearch
Kibana
```

### Kafka as intermediate buffer

```text
Go service -> stdout
Fluent Bit -> Kafka
Kafka -> Logstash/Vector
Logstash/Vector -> Elasticsearch
Kibana
```

Это оправдано, когда:
- объемы большие;
- storage иногда деградирует;
- нужен replay;
- ingestion и indexing нужно масштабировать отдельно.

## Частые failure modes

`Elasticsearch slow / red`:
- collector buffers растут;
- ingestion lag увеличивается;
- часть логов может дропаться, если buffering ограничен.

`bad mappings`:
- поле типа `status_code` внезапно становится `text`;
- exact filters и aggregations ломаются.

`too much cardinality`:
- memory pressure;
- дорогое индексирование;
- тяжелые aggregations.

`oversharding`:
- cluster страдает даже при умеренных объемах;
- растет operational pain.

## Practical rule

`Elasticsearch` хорош, когда:
- нужен сильный поиск;
- searchable logs реально используются командой;
- есть дисциплина по mappings и retention.

`Elasticsearch` быстро становится плохим выбором, когда:
- логов очень много;
- все подряд индексируется без разбора;
- searchable retention хотят держать слишком долго;
- команда не готова заниматься lifecycle, shards и capacity.

## Что важно объяснить на интервью

- почему приложение лучше не подключать напрямую к `Elasticsearch`;
- зачем нужен collector между приложением и search backend;
- почему lifecycle и shard management важны не меньше самого поиска;
- зачем нужен intermediate buffer вроде `Kafka` на больших объемах;
- какие признаки говорят, что Elasticsearch-стек уже деградирует operationally.

## Связанные темы

- [Logs Pipeline Overview](./logs-pipeline-overview.md)
- [Kibana And Elasticsearch](./kibana-and-elasticsearch.md)
- [Log Platforms Comparison Table](./log-platforms-comparison-table.md)
