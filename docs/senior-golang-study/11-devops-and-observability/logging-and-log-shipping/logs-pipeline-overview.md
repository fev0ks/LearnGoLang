# Logs Pipeline Overview

Эта тема нужна, чтобы понимать не только "где искать логи", но и "как они туда попали".

## Базовая модель

Обычно пайплайн логов выглядит так:

```text
application -> stdout/file -> collector/shipper -> processor/router -> storage -> query UI
```

Примеры:
- `application -> stdout -> Fluent Bit -> Elasticsearch -> Kibana`
- `application -> stdout -> Promtail -> Loki -> Grafana`
- `application -> stdout -> CloudWatch Logs -> OpenSearch`
- `application -> stdout -> Cloud Logging -> sink to GCS`

## Из чего состоит пайплайн

`application`:
- пишет structured logs, чаще всего JSON;
- не должен знать слишком много о downstream storage;
- обычно пишет в `stdout` в контейнерной среде.

`collector` или `shipper`:
- забирает логи из `stdout`, файлов, journald, syslog, Kafka или OTLP;
- добавляет metadata;
- буферизует, парсит, фильтрует, ретраит;
- отправляет в backend.

Типичные тулзы:
- `Fluent Bit`
- `Vector`
- `Logstash`
- `Promtail`
- `OpenTelemetry Collector`

`storage/query backend`:
- хранит логи и даёт возможность искать их;
- может быть оптимизирован под full-text, labels, time-series или object storage.

Примеры:
- `Elasticsearch` или `OpenSearch`
- `Loki`
- `CloudWatch Logs`
- `Google Cloud Logging`

`UI`:
- `Kibana`
- `Grafana`
- облачные консоли

## Почему сервис не пишет прямо в storage

Обычно direct-write из приложения в log storage избегают, потому что:
- приложение жёстко связывается с конкретным backend;
- ошибки логирования начинают влиять на request path;
- сложнее менять backend и роутинг;
- сложнее добавить buffering, retries и fan-out;
- труднее централизованно обогащать записи metadata.

Практическое правило:
- приложение пишет в `stdout` или локальный агент;
- агент уже занимается доставкой.

## Structured Logs

Для production почти всегда нужны structured logs.

Минимальный полезный набор полей:
- `timestamp`
- `service`
- `env`
- `level`
- `message`
- `request_id`
- `trace_id`
- `operation`
- `error_kind`
- `duration_ms`

Если всё лежит только в `message`, поиск и агрегации быстро становятся дорогими и хрупкими.

## Где запускают collector

### Sidecar

Один collector рядом с одним приложением.

Подходит, когда:
- нужен особый parsing для конкретного сервиса;
- есть сильная изоляция;
- допустим overhead на каждый pod.

Минусы:
- больше контейнеров;
- больше CPU и памяти;
- сложнее сопровождать в масштабе.

### DaemonSet или node-level agent

Один collector на ноду.

Подходит, когда:
- Kubernetes cluster собирает stdout контейнеров;
- хочется централизованный сбор;
- важна эффективность.

Обычно это самый частый вариант в `Kubernetes`.

### Host agent или VM agent

Подходит для:
- VM-based runtime;
- bare metal;
- systemd journald;
- file-based logs.

## Что происходит внутри collector

Collector обычно делает несколько шагов:

1. Получает запись.
2. Парсит raw log line.
3. Добавляет metadata:
   namespace, pod, container, host, region, cluster.
4. Фильтрует шум.
5. Преобразует поля.
6. Буферизует и отправляет в backend.
7. Ретраит при временных ошибках.

## Основные способы передачи логов

`stdout`:
- стандартный путь для контейнеров;
- удобно для `Docker`, `Kubernetes`, `ECS`, `Nomad`.

`file tailing`:
- collector читает файлы на диске;
- часто встречается на VM и legacy-сервисах.

`network forwarding`:
- syslog, fluent forward, HTTP ingestion, OTLP;
- полезно, когда нужен промежуточный hop.

`message broker`:
- Kafka между collector и storage;
- нужен, когда объём очень большой или требуется decoupling.

## Надёжность и trade-offs

Нужно понимать несколько базовых рисков:

`loss`:
- логи можно потерять при crash, OOM, network partition, переполнении буфера.

`backpressure`:
- storage может не успевать принимать логи;
- если pipeline fail-close, приложение начнёт страдать.

`cost`:
- полнотекстовый индекс дорог;
- длинный retention в search backend быстро становится очень дорогим.

`cardinality`:
- слишком много уникальных labels, fields или index dimensions убивают стоимость и производительность.

## Hot, Warm, Cold Storage

Частый production pattern:
- short retention в дорогом searchable storage;
- long retention в дешёвом object storage.

Пример:
- `7-14` дней в `Elasticsearch` или `Loki`;
- `30-180+` дней в `S3` или `GCS`.

Это почти всегда дешевле, чем хранить всё в индексируемом backend.

## Как выбирать backend

`Elasticsearch`:
- сильный full-text и агрегации;
- дорогой на больших объёмах;
- требует аккуратной схемы и retention.

`Loki`:
- дешевле для логов, если основной вход через labels;
- хуже, если нужен Elasticsearch-like full-text и сложная аналитика по многим полям.

`Cloud Logging / CloudWatch Logs`:
- быстро стартовать;
- удобно в managed cloud;
- могут оказаться дорогими и ограниченными для сложных расследований.

`S3 / GCS`:
- хорошо для архива;
- плохо для интерактивного расследования без дополнительного query engine.

## Частые ошибки

- писать plaintext вместо JSON;
- логировать слишком много high-cardinality полей как labels;
- слать debug-логи в production без sampling и retention strategy;
- делать backend логов частью критичного request path;
- пытаться использовать object storage как интерактивный search backend без дополнительного слоя.

## Что могут спросить на интервью

- почему `stdout + collector` обычно лучше direct-to-storage;
- чем `Elasticsearch` отличается от `Loki`;
- зачем нужен `Fluent Bit`;
- почему `S3` и `GCS` обычно используют как архив, а не как primary search engine;
- как построить pipeline, который не ломает приложение при проблемах с log backend.
