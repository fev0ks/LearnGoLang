# Tempo And Trace Investigation

Эта заметка нужна, чтобы понимать `Tempo` как trace backend и уметь practically расследовать latency.

## Содержание

- [Что такое Tempo](#что-такое-tempo)
- [Где Tempo в стеке](#где-tempo-в-стеке)
- [Что можно увидеть в trace](#что-можно-увидеть-в-trace)
- [Service graph vs traces](#service-graph-vs-traces)
- [Как расследовать latency через Tempo](#как-расследовать-latency-через-tempo)
- [Как обычно искать traces](#как-обычно-искать-traces)
- [Как читать waterfall](#как-читать-waterfall)
- [Где traces особенно полезны](#где-traces-особенно-полезны)
- [Чего Tempo не даст сам по себе](#чего-tempo-не-даст-сам-по-себе)
- [Practical Rule](#practical-rule)

## Что такое Tempo

`Tempo` — backend для distributed traces в экосистеме `Grafana`.

Если коротко:
- приложение экспортирует spans;
- `Tempo` их хранит;
- `Grafana` показывает trace UI.

Очень важно:
- `Tempo` не равен `OpenTelemetry`;
- `OpenTelemetry` делает instrumentation/export;
- `Tempo` — storage/query backend для traces.

## Где Tempo в стеке

Типичный stack:

```text
app instrumentation -> OTLP exporter -> Tempo
metrics -> Prometheus
logs -> Loki / Elasticsearch
UI -> Grafana
```

Если смотреть на роли:
- `Prometheus` отвечает за metrics;
- `Tempo` отвечает за traces;
- `Grafana` объединяет investigation UI.

## Что можно увидеть в trace

Когда открываешь trace, обычно видишь:
- root span;
- duration всего trace;
- waterfall child spans;
- parent/child relationships;
- attributes;
- status/error;
- timings каждого шага.

Это особенно полезно, когда один запрос ходит:
- в `Postgres`;
- в `Redis`;
- в downstream service;
- в `Kafka`;
- в `ClickHouse`.

## Service graph vs traces

Важно не путать:

### Trace view

Показывает:
- один конкретный request или event;
- конкретный waterfall;
- конкретные timings.

### Service graph

Показывает:
- aggregated relationships между сервисами;
- service-to-service map;
- derived graph metrics.

Service graph требует отдельной настройки.

Если в UI написано:
- `No service graph data found`

это не значит, что traces не работают.

Обычно это значит:
- traces есть;
- но metrics generator / service graph pipeline не включен.

## Как расследовать latency через Tempo

Нормальный workflow такой:

### 1. Увидеть деградацию в метриках

Например:
- вырос `p95` по `POST /api/v1/links`
- вырос error rate
- вырос duration DB operation

### 2. Перейти в Grafana Explore

Выбираешь datasource `Tempo`.

### 3. Найти trace

Часто ищут по:
- `service.name`
- span name
- time range
- `trace_id`, если он уже известен из логов

### 4. Открыть waterfall

Смотришь:
- какой span самый длинный;
- где child span доминирует по времени;
- где ошибка;
- есть ли retry chain или downstream timeout.

### 5. Коррелировать с logs/metrics

После этого уже смотришь:
- метрики, чтобы понять масштаб;
- логи, чтобы понять точную ошибку/контекст.

## Как обычно искать traces

Частые фильтры:
- `service.name = shortener`
- `service.name = analytics`
- route-like span names
- errors only

Практический вопрос:
- ищешь "медленный request path" или "ошибочный path".

## Как читать waterfall

На что смотреть:

### Root duration

Показывает полное время запроса.

### Wide child span

Если один child span занимает почти все время:
- обычно bottleneck там.

### Последовательные vs параллельные spans

Если есть несколько child spans:
- они могли идти последовательно;
- или параллельно.

Waterfall это сразу показывает.

### Error status

Span может быть:
- `ok`
- `error`

Это помогает быстро локализовать failing hop.

## Где traces особенно полезны

### Slow API path

Например:
- handler кажется нормальным;
- но trace показывает, что `Redis` быстрый, а `Postgres` тормозит.

### Async pipeline

Например:
- publish прошел;
- consumer downstream тормозит;
- trace показывает delay и failure point.

### Cross-service request

Например:
- фронт говорит, что запрос медленный;
- trace показывает, что проблема не в API gateway, а в third service hop.

## Чего Tempo не даст сам по себе

`Tempo` не отвечает на вопросы:
- сколько таких ошибок в минуту;
- какой service error ratio;
- какой p95 по сервису за 30 минут.

Это уже метрики.

То есть:
- `Tempo` хорош для single-trace investigation;
- `Prometheus` хорош для aggregate health.

## Practical Rule

Если коротко:

- `Tempo` нужен не для графиков, а для trace investigation;
- он показывает timing одного execution path;
- service graph — отдельная optional фича, а не обязательная часть tracing;
- если нужно найти bottleneck в одном request, открывай trace;
- если нужно понять масштаб проблемы, смотри метрики;
- если нужно понять конкретную ошибку/данные, смотри логи.
