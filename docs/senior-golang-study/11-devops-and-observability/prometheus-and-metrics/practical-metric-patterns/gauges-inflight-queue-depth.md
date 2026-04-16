# Gauges: In-Flight, Queue Depth, Current State

Эта заметка про `Gauge`-метрики, которые показывают текущее состояние, а не накопленные события.

## Содержание

- [Что это за метрика](#что-это-за-метрика)
- [Как это выглядит в Grafana](#как-это-выглядит-в-grafana)
- [Как это читать](#как-это-читать)
- [Что считать нормой](#что-считать-нормой)
- [Плохие сигналы](#плохие-сигналы)
- [Полезные запросы](#полезные-запросы)
- [Как понимать, плохо это или хорошо](#как-понимать-плохо-это-или-хорошо)
- [Practical Rule](#practical-rule)

## Что это за метрика

`Gauge` может:
- расти;
- падать;
- прыгать вверх-вниз.

Примеры:
- `http_in_flight_requests`
- `worker_queue_depth`
- `active_goroutines`
- `db_open_connections`

## Как это выглядит в Grafana

Чаще всего:
- line chart;
- stat panel;
- иногда gauge/thermometer panel.

## Как это читать

Gauge — это snapshot.

То есть raw число уже meaningful:

```promql
worker_queue_depth
```

или

```promql
http_in_flight_requests
```

## Что считать нормой

Опять baseline.

Например:
- `in_flight=3..10` для сервиса может быть нормой;
- `queue_depth=0` большую часть времени может быть нормой;
- `db_open_connections=20` может быть нормой при пуле `max=50`.

## Плохие сигналы

### Постоянный рост queue depth

Это сильный сигнал saturation:
- producer быстрее consumer;
- worker не успевает;
- downstream тормозит;
- retry storm.

### In-flight request count растет и не опускается

Это может значить:
- сервис висит на slow dependencies;
- CPU starvation;
- thread/goroutine pool saturation;
- stuck requests.

### Current connection gauges прижались к лимиту

Например:
- DB connection pool близок к max;
- Redis clients резко выросли.

Это уже capacity risk.

## Полезные запросы

### current queue depth

```promql
worker_queue_depth
```

### max queue depth за окно

```promql
max_over_time(worker_queue_depth[15m])
```

### average in-flight за окно

```promql
avg_over_time(http_in_flight_requests[5m])
```

## Как понимать, плохо это или хорошо

Gauge почти никогда не интерпретируют изолированно.

Примеры:
- queue depth растет + processing rate не растет -> плохо;
- in-flight растет + latency растет -> плохо;
- in-flight растет, но throughput тоже растет и latency стабильна -> возможно, это просто load spike.

## Practical Rule

Если видишь gauge:
- смотри на trend, а не только на single point;
- сравнивай с лимитами и baseline;
- почти всегда коррелируй с throughput и latency.
