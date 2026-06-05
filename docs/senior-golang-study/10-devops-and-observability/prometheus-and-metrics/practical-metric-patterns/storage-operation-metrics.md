# Storage Operation Metrics

Эта заметка про метрики для `Postgres`, `Redis`, `ClickHouse` и других storage-path.

## Содержание

- [Что это за сигналы](#что-это-за-сигналы)
- [Как это выглядит в Grafana](#как-это-выглядит-в-grafana)
- [Что считать хорошим дизайном](#что-считать-хорошим-дизайном)
- [Полезные запросы](#полезные-запросы)
- [Как это читать](#как-это-читать)
- [Как понимать “плохо” или “хорошо”](#как-понимать-плохо-или-хорошо)
- [Пример практического чтения](#пример-практического-чтения)
- [Practical Rule](#practical-rule)

## Что это за сигналы

Обычно делают две группы:

### Operation counters

Например:
- `postgres_operations_total`
- `redis_operations_total`

С labels:
- `operation`
- `result`

### Operation latency histograms

Например:
- `postgres_operation_duration_seconds`
- `redis_operation_duration_seconds`

С labels:
- `operation`
- `result`

## Как это выглядит в Grafana

Чаще всего:
- table или bar chart по `operation/result`;
- p95 panel по `operation`;
- stacked success/error chart.

## Что считать хорошим дизайном

Хорошо:
- `operation="link_create"`
- `operation="link_get_by_id"`
- `operation="link_cache_get"`
- `result="success"`
- `result="error"`
- `result="miss"`
- `result="hit"`

Плохо:
- raw SQL text;
- Redis key;
- full URL;
- user id;
- short code.

Storage metrics должны быть low-cardinality.

## Полезные запросы

### операции Postgres по типу и результату

```promql
sum by (operation, result) (
  rate(postgres_operations_total[5m])
)
```

### p95 Postgres latency by operation

```promql
histogram_quantile(
  0.95,
  sum by (operation, le) (
    rate(postgres_operation_duration_seconds_bucket[5m])
  )
)
```

### операции Redis по типу и результату

```promql
sum by (operation, result) (
  rate(redis_operations_total[5m])
)
```

### p95 Redis latency by operation

```promql
histogram_quantile(
  0.95,
  sum by (operation, le) (
    rate(redis_operation_duration_seconds_bucket[5m])
  )
)
```

## Как это читать

### Success counter высокий

Обычно это нормально:
- path реально используется.

### Miss counter растет

Это не всегда плохо.

Например для cache:
- часть `miss` естественна;
- вопрос в hit ratio и baseline.

### Error counter растет

Вот это уже важнее:
- storage issues;
- timeouts;
- serialization bugs;
- bad dependency state.

### Latency растет только у одного operation

Отличный узкий сигнал:
- ищи bottleneck именно в этом path.

### Latency растет по всем операциям storage

Это уже более системная проблема:
- сеть;
- DB overload;
- storage node pressure;
- connection pool saturation.

## Как понимать “плохо” или “хорошо”

Задавай вопросы:
- рост идет в `success`, `miss` или `error`;
- проблема локальна к одной операции или к целому storage;
- это сопровождается ростом user-facing latency;
- есть ли saturation signal;
- traces подтверждают storage bottleneck?

## Пример практического чтения

### Сценарий 1

- `postgres_operation_duration_seconds p95` растет на `link_create`
- `http p95` тоже растет на `POST /api/v1/links`
- error rate пока ровный

Вывод:
- storage path тормозит;
- вероятно, incident еще не дошел до errors, но уже бьет по latency.

### Сценарий 2

- `redis_operations_total{operation="link_cache_get",result="miss"}` растет
- `hit` падает
- `postgres link_resolve_and_increment` rate растет

Вывод:
- cache effectiveness упала;
- DB fallback стал горячим;
- user-facing latency может вырасти чуть позже.

## Practical Rule

Storage metrics хороши тем, что связывают пользовательскую деградацию с конкретным infra path.

Смотри их так:
- throughput storage operation;
- error profile;
- latency profile;
- correlation с HTTP latency и traces.
