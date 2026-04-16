# HTTP Error Rate

Эта заметка про второй обязательный сигнал после throughput: ошибки.

## Содержание

- [Что это за метрика](#что-это-за-метрика)
- [Как это выглядит в Grafana](#как-это-выглядит-в-grafana)
- [Как это читать](#как-это-читать)
- [Что считать нормой](#что-считать-нормой)
- [Как отличать 4xx и 5xx](#как-отличать-4xx-и-5xx)
- [Полезные запросы](#полезные-запросы)
- [Как понимать, плохо это или нет](#как-понимать-плохо-это-или-нет)
- [Practical Rule](#practical-rule)

## Что это за метрика

Обычно error rate не существует как отдельная “волшебная” метрика.

Ее считают из request counter:

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
```

или как долю от всего трафика:

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(http_requests_total[5m]))
```

## Как это выглядит в Grafana

Обычно это:
- line chart для error ratio;
- stat panel для current 5xx rate;
- stacked graph по `status_code`.

## Как это читать

Важно различать:
- absolute error rate;
- error ratio.

### Absolute error rate

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
```

Показывает:
- сколько ошибок в секунду реально идет.

### Error ratio

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(http_requests_total[5m]))
```

Показывает:
- какой процент запросов завершился ошибкой.

## Что считать нормой

Идеальный сервис не всегда имеет `0`.

Нормально:
- predictable tiny background error level;
- короткие единичные spikes без user impact;
- controlled `4xx`, если это часть API semantics.

Ненормально:
- sustained `5xx`;
- error ratio растет вместе с latency;
- ошибки идут на одном hot route;
- error ratio растет после deploy.

## Как отличать 4xx и 5xx

### 4xx

Часто это:
- invalid input;
- unauthorized;
- not found;
- rate limit.

Они не всегда означают degradation.

### 5xx

Это уже гораздо ближе к настоящему operational problem:
- panic;
- DB failure;
- timeout;
- downstream unavailable;
- internal error.

Поэтому на dashboards:
- `4xx` и `5xx` почти всегда надо разделять.

## Полезные запросы

### 5xx rate

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
```

### 5xx ratio

```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(http_requests_total[5m]))
```

### 5xx ratio by route

```promql
sum by (route) (rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum by (route) (rate(http_requests_total[5m]))
```

### status code breakdown

```promql
sum by (status_code) (
  rate(http_requests_total[5m])
)
```

## Как понимать, плохо это или нет

Вопрос не в том, что на графике “красная линия”.

Вопросы должны быть такие:
- это `4xx` или `5xx`;
- это spike на 30 секунд или стабильная деградация;
- это один route или весь сервис;
- это совпало с rollout;
- это сопровождается ростом latency и saturation?

Если error rate растет вместе с:
- latency,
- DB duration,
- queue lag,

то это уже strong incident signal.

## Practical Rule

Если видишь error graph:
- сначала раздели `4xx` и `5xx`;
- потом посмотри absolute rate и ratio;
- потом режь по route;
- потом коррелируй с latency и deploy timeline.
