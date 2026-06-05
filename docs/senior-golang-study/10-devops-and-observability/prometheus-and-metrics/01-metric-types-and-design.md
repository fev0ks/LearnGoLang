# Metric Types And Design

Эта заметка нужна, чтобы различать типы метрик не теоретически, а по задаче.

## Содержание

- [Counter](#counter)
- [Gauge](#gauge)
- [Histogram](#histogram)
- [Summary](#summary)
- [Что обычно использовать в backend](#что-обычно-использовать-в-backend)
- [Как выбирать labels](#как-выбирать-labels)
- [Что обычно значат result labels](#что-обычно-значат-result-labels)
- [Базовый набор метрик для API-сервиса](#базовый-набор-метрик-для-api-сервиса)
- [Базовый набор для worker / consumer](#базовый-набор-для-worker--consumer)
- [RED и USE](#red-и-use)
- [Practical Rule](#practical-rule)

## Counter

`Counter` подходит для вещей, которые означают количество событий:
- requests
- errors
- retries
- published events
- consumed events

Примеры:
- `http_requests_total`
- `redirect_resolves_total`
- `kafka_publish_total`

Что важно:
- `Counter` почти никогда не смотрят как raw absolute number;
- его почти всегда читают через `rate()` или `increase()`.

Нормально:

```promql
rate(my_service_http_requests_total[5m])
```

Нормально:

```promql
increase(my_service_http_requests_total[15m])
```

Плохо:

```promql
my_service_http_requests_total
```

Такой raw value редко полезен operationally.

## Gauge

`Gauge` подходит для текущего состояния:
- in-flight requests
- current queue depth
- number of connected clients
- memory pressure
- active goroutines

Примеры:
- `http_in_flight_requests`
- `worker_queue_depth`
- `redis_connected_clients`

Главная идея:
- `Gauge` можно считать как snapshot.

То есть raw значение уже осмысленно:

```promql
my_service_in_flight_requests
```

## Histogram

`Histogram` нужен для распределения:
- latency
- payload size
- query duration
- processing duration

Примеры:
- `http_request_duration_seconds`
- `postgres_operation_duration_seconds`
- `clickhouse_operation_duration_seconds`

В `Prometheus` histogram дает набор series:
- `_bucket`
- `_sum`
- `_count`

Например:
- `http_request_duration_seconds_bucket`
- `http_request_duration_seconds_sum`
- `http_request_duration_seconds_count`

Что это дает:
- можно считать p50/p95/p99;
- можно агрегировать между инстансами;
- можно строить heatmaps.

Типичный p95:

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

Если нужен p95 по route:

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

## Summary

`Summary` тоже про latency/distribution, но в `Prometheus` мире чаще выбирают `Histogram`.

Почему:
- `Summary` сложнее агрегировать между несколькими instance;
- quantiles считаются на стороне приложения;
- на fleet-level это часто хуже, чем histogram.

Когда `Summary` может быть ок:
- single-process local metric;
- тебе не нужна агрегация across replicas;
- нужна простая app-local статистика.

Когда лучше `Histogram`:
- почти всегда для backend service latency.

## Что обычно использовать в backend

### HTTP

- request count -> `Counter`
- request latency -> `Histogram`
- in-flight requests -> `Gauge`

### Database / cache

- operations total -> `Counter`
- operation duration -> `Histogram`

### Workers / consumers

- processed events -> `Counter`
- failed events -> `Counter`
- retry attempts -> `Counter`
- queue lag / backlog -> `Gauge`
- processing duration -> `Histogram`

## Как выбирать labels

Нужна простая эвристика:

хорошие labels:
- `service`
- `method`
- `route`
- `status_code`
- `operation`
- `result`
- `phase`

плохие labels:
- `request_id`
- `trace_id`
- `user_id`
- `email`
- `short_code`
- `url`
- `sql`
- `redis_key`

Правило:
- label должен иметь маленький и предсказуемый набор значений.

## Что обычно значат result labels

Очень хороший pattern:
- `result=success`
- `result=error`
- `result=not_found`
- `result=timeout`
- `result=retry`
- `result=limited`

Это лучше, чем:
- raw error message
- exception class name как label
- SQLSTATE text как label

Потому что:
- signal остается low-cardinality;
- query остается читаемым;
- dashboard не разваливается от новых вариантов ошибок.

## Базовый набор метрик для API-сервиса

Минимум:
- `http_requests_total`
- `http_request_duration_seconds`
- `http_in_flight_requests`

Очень хороший следующий уровень:
- domain counters
- storage operation counters
- storage duration histograms
- eventing counters

## Базовый набор для worker / consumer

Минимум:
- `events_total{phase,result}`
- `processing_duration_seconds`
- `dlq_total`
- queue lag gauge

## RED и USE

На интервью и в жизни полезно помнить:

### RED

Для request-driven сервисов:
- `Rate`
- `Errors`
- `Duration`

### USE

Для ресурсов:
- `Utilization`
- `Saturation`
- `Errors`

Например:
- API endpoint -> RED
- database connection pool -> USE
- Kafka consumer queue -> saturation + processing metrics

## Practical Rule

Если не уверен, что выбрать:
- count events -> `Counter`
- measure current level -> `Gauge`
- measure latency/distribution -> `Histogram`
- `Summary` используй редко и осознанно

И еще важнее:
- выбирай не тип метрики сам по себе;
- выбирай operational question, на который она должна отвечать.
