# PromQL Cheatsheet

Эта заметка нужна, чтобы быстро вспомнить синтаксис `PromQL` и не путаться в базовых конструкциях.

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Базовые сущности](#базовые-сущности)
- [Самые важные функции](#самые-важные-функции)
- [Фильтрация по labels](#фильтрация-по-labels)
- [Самые полезные практические запросы](#самые-полезные-практические-запросы)
- [`by` и `without`](#by-и-without)
- [Top queries](#top-queries)
- [Offset](#offset)
- [Useful comparisons](#useful-comparisons)
- [Частые ошибки](#частые-ошибки)
- [Practical набор запросов для senior backend](#practical-набор-запросов-для-senior-backend)

## Самая короткая интуиция

`PromQL` — это язык запросов к time series.

Главные вопросы, которые он решает:
- какое текущее значение?
- как быстро растет counter?
- сколько событий было за окно?
- какой p95 latency?
- как сгруппировать series по labels?

## Базовые сущности

### Instant vector

Текущее значение series:

```promql
http_requests_total
```

### Range vector

Значения за окно времени:

```promql
http_requests_total[5m]
```

### Scalar

Обычное число:

```promql
5
```

## Самые важные функции

### rate()

Скорость роста counter в секунду:

```promql
rate(http_requests_total[5m])
```

Это один из самых частых запросов в Prometheus.

### increase()

Насколько counter вырос за окно:

```promql
increase(http_requests_total[15m])
```

Полезно для:
- количества ошибок за последние 15 минут;
- числа событий за окно;
- числа publish attempts за окно.

### sum()

Сумма по series:

```promql
sum(rate(http_requests_total[5m]))
```

### sum by (...)

Группировка по labels:

```promql
sum by (route, status_code) (
  rate(http_requests_total[5m])
)
```

### avg(), max(), min()

Обычные агрегаты:

```promql
avg by (service) (up)
```

### histogram_quantile()

Percentile по histogram buckets:

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

## Фильтрация по labels

### exact match

```promql
http_requests_total{service="shortener"}
```

### несколько labels

```promql
http_requests_total{service="shortener",route="POST /api/v1/links"}
```

### negative match

```promql
http_requests_total{status_code!="200"}
```

### regex match

```promql
http_requests_total{status_code=~"5.."}
```

### regex negative match

```promql
http_requests_total{route!~"/health/.*"}
```

## Самые полезные практические запросы

### RPS

```promql
sum(rate(http_requests_total[5m]))
```

### RPS по route

```promql
sum by (route) (
  rate(http_requests_total[5m])
)
```

### Error rate

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
```

### Error rate ratio

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(http_requests_total[5m]))
```

### p95 latency

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

### p95 latency by route

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

### Количество событий за 15 минут

```promql
increase(link_visit_publish_total[15m])
```

### Storage operations by operation/result

```promql
sum by (operation, result) (
  rate(storage_postgres_operations_total[5m])
)
```

### p95 storage latency

```promql
histogram_quantile(
  0.95,
  sum by (operation, le) (
    rate(storage_postgres_operation_duration_seconds_bucket[5m])
  )
)
```

## `by` и `without`

### by

Оставляет только выбранные labels:

```promql
sum by (route) (
  rate(http_requests_total[5m])
)
```

### without

Агрегирует по всем labels, кроме указанных:

```promql
sum without (instance, pod) (
  rate(http_requests_total[5m])
)
```

Это часто удобно в Kubernetes, где есть noisy infra labels.

## Top queries

### topk

```promql
topk(5, sum by (route) (rate(http_requests_total[5m])))
```

Показывает самые горячие routes.

### bottomk

```promql
bottomk(5, some_metric)
```

## Offset

Сравнить с прошлым периодом:

```promql
sum(rate(http_requests_total[5m] offset 1h))
```

## Useful comparisons

### больше порога

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m])) > 1
```

### latency выше порога

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
) > 0.5
```

## Частые ошибки

### 1. Считать counter как gauge

Плохо:

```promql
http_requests_total
```

Лучше:

```promql
rate(http_requests_total[5m])
```

### 2. Забывать `sum by (le)` для histogram quantile

Плохо:

```promql
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))
```

Обычно нужно:

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

### 3. Делать queries по high-cardinality labels

Если ты засунул в labels `user_id`, `trace_id`, `request_id`, то `PromQL` уже не спасет.  
Проблема в metric design, не в query syntax.

### 4. Слишком маленькое окно для rate()

Если окно слишком маленькое:

```promql
rate(http_requests_total[10s])
```

то signal может быть шумным.

Обычно лучше:
- `1m`
- `5m`
- `15m`

в зависимости от задачи.

## Practical набор запросов для senior backend

Нужно уметь быстро написать:

RPS:
```promql
sum(rate(http_requests_total[5m]))
```

5xx ratio:
```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(http_requests_total[5m]))
```

p95 latency:
```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

database latency by operation:
```promql
histogram_quantile(
  0.95,
  sum by (operation, le) (
    rate(storage_postgres_operation_duration_seconds_bucket[5m])
  )
)
```

redis/cache outcomes:
```promql
sum by (operation, result) (
  rate(storage_redis_operations_total[5m])
)
```

consumer success/error:
```promql
sum by (phase, result) (
  rate(analytics_consumer_events_total[5m])
)
```
