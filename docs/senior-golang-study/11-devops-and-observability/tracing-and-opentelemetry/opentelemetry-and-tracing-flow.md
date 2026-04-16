# OpenTelemetry And Tracing Flow

Эта заметка нужна, чтобы понимать distributed tracing как flow, а не как набор buzzwords.

Коротко:

```text
request / event
  -> span creation in service A
  -> context propagation to service B
  -> child spans in service B/C/DB/cache
  -> exporter sends spans via OTLP
  -> backend stores traces
  -> Grafana / Tempo lets you inspect waterfall and timings
```

## Самая короткая интуиция

Если сильно упростить:

- `metrics` говорят, что есть проблема;
- `logs` помогают понять, что именно произошло;
- `traces` показывают, где именно в цепочке времени потерялось.

То есть trace отвечает на вопрос:
- как один конкретный request прошел через систему?

## Базовые сущности

### Trace

`Trace` — это один end-to-end execution path.

Например:
- пользователь отправил `POST /api/v1/links`
- сервис сходил в `Postgres`
- потом вызвал `Redis`
- потом отправил событие в `Kafka`

Все эти шаги могут принадлежать одному trace.

### Span

`Span` — один шаг внутри trace.

У span обычно есть:
- `name`
- `start time`
- `end time`
- `duration`
- `attributes`
- `status`
- `span id`
- `parent span id`

Примеры span names:
- `POST /api/v1/links`
- `link_create`
- `postgres insert`
- `redis get`
- `publish link.visited`

### Context propagation

Чтобы trace не развалился между сервисами, trace context надо передавать дальше.

Именно это и называется propagation.

Без propagation:
- каждый сервис начнет новый trace;
- end-to-end chain потеряется.

С propagation:
- сервис B понимает, что он child от span сервиса A.

## Что такое OpenTelemetry по сути

`OpenTelemetry` — это не storage и не UI.

Это стандарт и инструментарий для:
- создания spans;
- propagation context;
- export telemetry наружу.

То есть роль `OpenTelemetry`:
- сделать instrumented app;
- отдать telemetry дальше.

Роль backend storage:
- хранить traces, metrics, logs.

Роль UI:
- показывать их человеку.

Простой mental model:

```text
OpenTelemetry = instrumentation + propagation + export
Tempo = trace storage
Grafana = UI for investigation
```

## Как выглядит tracing flow на практике

### 1. Входящий request

Приходит HTTP request.

Middleware делает:
- извлекает trace context из headers, если он уже есть;
- или создает новый root span;
- кладет span context в `context.Context`.

### 2. Внутренние шаги

Дальше код создает дочерние spans:
- handler span
- service span
- repository span
- Kafka publish span
- cache span

Это дает waterfall по времени.

### 3. Propagation наружу

Если сервис зовет downstream:
- HTTP
- gRPC
- Kafka
- queue/message bus

он должен перенести trace context в transport metadata.

Например:
- HTTP headers
- gRPC metadata
- Kafka headers

### 4. Export

Когда spans завершаются, они экспортируются.

Частый путь:

```text
app -> OTLP exporter -> Tempo / OTel Collector -> storage
```

### 5. Investigation

Потом оператор открывает trace в `Grafana/Tempo` и видит:
- root request duration;
- child spans;
- какой шаг был самым долгим;
- какой downstream вызов упал.

## Что такое OTLP

`OTLP` = `OpenTelemetry Protocol`.

Это стандартный transport для отправки telemetry.

Частые варианты:
- `OTLP/gRPC`
- `OTLP/HTTP`

Практический смысл:
- приложение экспортирует telemetry не в конкретный vendor API;
- а в стандартный protocol.

Это уменьшает vendor lock-in.

## Чем traces отличаются от logs и metrics

### Metrics

Хороши, чтобы понять:
- растет ли latency;
- вырос ли error rate;
- деградирует ли cache hit ratio.

Но они не показывают судьбу одного запроса.

### Logs

Хороши, чтобы увидеть:
- конкретную ошибку;
- payload shape;
- request_id;
- stack trace;
- reason text.

Но логи тяжело дают точный waterfall по времени across services.

### Traces

Хороши, чтобы увидеть:
- как один запрос прошел через сервисы;
- где был самый долгий шаг;
- где случился timeout;
- какой downstream завис.

Но traces не заменяют логи:
- trace не всегда даст достаточно доменного контекста;
- лог все равно нужен для rich error details.

## Что важно не делать

### 1. Не путать tracing с metrics

Trace не должен отвечать на вопрос:
- сколько ошибок в минуту?

Это задача metrics.

### 2. Не вешать бесконтрольные attributes

Сильно опасные вещи:
- `user_id`
- `email`
- raw SQL
- secrets
- full body payload

Span attributes тоже надо проектировать аккуратно.

### 3. Не терять context.Context

Если сервис делает `context.Background()` внутри business logic:
- propagation ломается;
- cancellation ломается;
- trace lineage ломается.

Это один из самых частых practical bugs.

### 4. Не считать, что tracing бесплатен

Tracing — это overhead:
- CPU
- memory
- network
- storage

Поэтому:
- sampling важен;
- instrumentation нужно делать осмысленно.

## Где tracing особенно полезен

- HTTP request проходит через несколько сервисов;
- есть message-driven pipeline;
- есть несколько storage hops;
- жалобы на latency, которую не видно из logs alone;
- есть intermittent failures и timeout chains.

## Practical Rule

Если коротко:

- trace = судьба одного request/event;
- span = один шаг в этой судьбе;
- `OpenTelemetry` создает spans, propagates context и export'ит данные;
- `Tempo` хранит traces;
- `Grafana` показывает waterfall и timings;
- traces не заменяют metrics и logs, а дают третью ось расследования.
