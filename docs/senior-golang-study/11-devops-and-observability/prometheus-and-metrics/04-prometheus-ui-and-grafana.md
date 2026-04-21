# Prometheus UI And Grafana

Эта заметка нужна, чтобы убрать типичную путаницу:
- почему один metric name превращается в много series;
- что значат `job`, `instance`, `pod`;
- почему в `Prometheus UI` и `Grafana` ты видишь “кучу строк” вместо одной метрики.

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Что ты реально видишь в Prometheus UI](#что-ты-реально-видишь-в-prometheus-ui)
- [Что обычно значат основные labels](#что-обычно-значат-основные-labels)
- [Почему в Grafana тоже “много линий”](#почему-в-grafana-тоже-много-линий)
- [Как сделать график читаемым](#как-сделать-график-читаемым)
- [Что обычно путают новички](#что-обычно-путают-новички)
- [Самые полезные паттерны](#самые-полезные-паттерны)
- [Как думать о Prometheus UI vs Grafana](#как-думать-о-prometheus-ui-vs-grafana)
- [Practical Rule](#practical-rule)

## Самая короткая интуиция

В `Prometheus` метрика — это не просто имя.

Правильная модель такая:

```text
metric name + label set = одна time series
```

Поэтому:
- `http_requests_total` само по себе — это еще не одна линия;
- `http_requests_total{job="shortener",pod="shortener-abc",route="POST /api/v1/links",status_code="201"}` — это уже одна конкретная series.

## Что ты реально видишь в Prometheus UI

Если в `Prometheus` вбить:

```promql
http_requests_total
```

ты обычно увидишь много строк, потому что они различаются по labels:
- `job`
- `instance`
- `pod`
- `route`
- `status_code`
- `service`

То есть это не “Prometheus сломался”, а нормальная модель данных.

## Что обычно значат основные labels

### `job`

Обычно это логическая группа scrape target'ов.

Примеры:
- `shortener`
- `analytics`
- `prometheus`

Практически:
- это один из самых полезных labels для service-level запросов.

### `instance`

Обычно это конкретный target address.

Например:
- `10.42.1.15:8080`
- `shortener:8080`

Практически:
- это label для per-target debugging.

### `pod`

Обычно это конкретный pod name в Kubernetes.

Например:
- `shortener-7f8d9b4c97-abcde`

Практически:
- полезно, если надо понять, что один pod деградирует, а остальные нет.

### `service`

Это уже часто app-level label, который команда сама добавляет в метрики.

Например:
- `service="shortener"`
- `service="analytics"`

Это удобно, потому что:
- label стабилен;
- он не зависит от rollout hash, как `pod`;
- на dashboard его часто приятнее использовать, чем `instance`.

### `route`

Это уже app-level label для HTTP метрик.

Например:
- `route="POST /api/v1/links"`
- `route="GET /{shortCode}"`

Очень важно:
- хороший `route` low-cardinality;
- плохой `route` — это raw path с динамическими id.

## Почему в Grafana тоже “много линий”

Потому что Grafana просто показывает результат `PromQL`.

Если ты спросишь:

```promql
rate(http_requests_total[5m])
```

ты получишь line per series.

Если series отличаются по:
- `job`
- `instance`
- `pod`
- `route`
- `status_code`

то линий будет много.

Это нормально.

## Как сделать график читаемым

Нужно агрегировать.

### Сервисный throughput

```promql
sum(rate(http_requests_total{job="shortener"}[5m]))
```

Теперь это уже одна aggregated линия.

### Throughput по route

```promql
sum by (route) (
  rate(http_requests_total{job="shortener"}[5m])
)
```

Теперь будет одна линия на route, а не на каждый pod+instance+status combination.

### Throughput по pod

```promql
sum by (pod) (
  rate(http_requests_total{job="shortener"}[5m])
)
```

Теперь ты специально смотришь imbalance между pod'ами.

## Что обычно путают новички

### 1. “Я вижу много строк, значит метрика дублируется”

Нет.

Это разные series одной метрики.

### 2. “Почему один pod показывает одно число, а другой другое?”

Потому что каждый pod считает свои локальные события.

Глобальная картина собирается только через:
- `sum`
- `sum by`
- `sum without`

### 3. “Почему raw metric неудобно читать?”

Потому что raw metric без aggregation почти всегда показывает слишком низкий уровень детализации.

## Самые полезные паттерны

### Посмотреть жив ли target

```promql
up
```

или:

```promql
up{job="shortener"}
```

Если `up=1`, target жив.
Если `up=0`, scrape не удался.

### Сервисный view

```promql
sum without (instance, pod) (
  rate(http_requests_total{job="shortener"}[5m])
)
```

Это хороший способ убрать target noise.

### Pod-level view

```promql
sum by (pod) (
  rate(http_requests_total{job="shortener"}[5m])
)
```

Это хороший способ искать broken replica.

## Как думать о Prometheus UI vs Grafana

### Prometheus UI

Полезно для:
- quick raw checks;
- проверить, есть ли вообще series;
- проверить labels;
- быстро дернуть query.

### Grafana

Полезно для:
- dashboards;
- красивой визуализации;
- time range comparison;
- operational investigation.

Но underlying model у них одна и та же:
- `PromQL` + time series + labels.

## Practical Rule

Если видишь “слишком много линий”:
1. проверь, какие labels отличаются;
2. реши, какой уровень тебе нужен:
   - service
   - route
   - pod
   - status_code
3. добавь aggregation:
   - `sum(...)`
   - `sum by (...)`
   - `sum without (...)`

И помни:
- много lines в raw query — это не баг;
- это просто честное отражение того, что series в Prometheus строятся по label set.
