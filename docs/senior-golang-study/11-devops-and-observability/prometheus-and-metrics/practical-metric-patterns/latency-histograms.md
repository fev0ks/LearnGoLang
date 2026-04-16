# Latency Histograms

Эта заметка про самый важный и самый часто неправильно читаемый тип панелей: latency.

## Что это за метрика

Обычно latency хранят в `Histogram`, например:

```text
http_request_duration_seconds_bucket
postgres_operation_duration_seconds_bucket
redis_operation_duration_seconds_bucket
```

Именно buckets потом превращаются в:
- p50
- p95
- p99

## Как это выглядит в Grafana

Обычно строят:
- line chart для `p95` или `p99`;
- иногда heatmap;
- иногда table по routes/operations.

Типичный p95 query:

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

По route:

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

## Как это читать

### p50

Показывает “обычный” запрос.

### p95

Показывает хвост задержки.

Обычно это самая полезная operational панель.

### p99

Показывает еще более тяжелый tail.

Полезно, но шумнее.

## Что считать нормой

Опять же, универсальной цифры нет.

Нужно смотреть на baseline:
- для internal API p95 `30ms` может быть нормой;
- для тяжелого search endpoint p95 `300ms` тоже может быть нормой.

Важнее не абсолютное число само по себе, а:
- было ли оно ожидаемым;
- резко ли оно выросло;
- где именно выросло.

## Плохие сигналы

### Растет только p95/p99, а p50 почти ровный

Это часто значит:
- tail latency problem;
- contention;
- occasional slow DB/cache path;
- noisy downstream.

### Растут и p50, и p95

Это уже более широкая деградация:
- сервис целиком стал медленнее;
- CPU saturation;
- DB/systemic slowness;
- degraded downstream.

### p95 растет только на одном route

Это хороший narrow signal:
- ищи конкретный use case или storage path.

## Что часто путают

### Raw bucket panel

Если ты смотришь прямо на `_bucket` и не понимаешь цифры, это нормально.

Raw bucket series:
- редко читаются как итоговый signal человеком;
- это строительный материал для percentile query.

То есть чаще нужно не смотреть:

```promql
http_request_duration_seconds_bucket
```

а смотреть:

```promql
histogram_quantile(...)
```

### Average latency

Среднее иногда скрывает проблему хвоста.

Можно вычислить average:

```promql
rate(http_request_duration_seconds_sum[5m])
/
rate(http_request_duration_seconds_count[5m])
```

Но operationally часто важнее `p95`.

## Полезные панели

### Global p95

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

### p95 by route

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(http_request_duration_seconds_bucket[5m])
  )
)
```

### p95 by storage operation

```promql
histogram_quantile(
  0.95,
  sum by (operation, le) (
    rate(postgres_operation_duration_seconds_bucket[5m])
  )
)
```

## Как понимать, плохо это или хорошо

Смотри не только на одну цифру.

Порядок анализа:
1. baseline нормальный или нет;
2. где растет — global или одна route/operation;
3. растет ли одновременно error rate;
4. растет ли saturation;
5. trace/logs подтверждают bottleneck?

## Practical Rule

Если в Grafana видишь latency panel:
- сначала пойми, это `avg`, `p95` или `p99`;
- потом пойми, глобальная она или разрезанная по route/operation;
- потом сравни с baseline;
- если растет tail, ищи bottleneck в traces и storage metrics.
