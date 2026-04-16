# Prometheus Relabeling And Target Labels

Эта заметка нужна, чтобы понять, что происходит между service discovery и реальным scrape target.

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Откуда берутся сырые labels](#откуда-берутся-сырые-labels)
- [Что делает relabeling](#что-делает-relabeling)
- [Почему это важно](#почему-это-важно)
- [Частые действия в relabel_configs](#частые-действия-в-relabel_configs)
- [Важные служебные labels](#важные-служебные-labels)
- [Как labels попадают в time series](#как-labels-попадают-в-time-series)
- [Где тут главные риски](#где-тут-главные-риски)
- [Practical example](#practical-example)
- [Practical Rule](#practical-rule)

## Самая короткая интуиция

Полезно держать в голове такую цепочку:

```text
service discovery
  -> raw discovered labels
  -> relabel_configs
  -> final scrape target
  -> scraped metrics
  -> time series with final labels
```

То есть relabeling отвечает не за запросы к данным, а за форму target'ов и labels до scrape.

## Откуда берутся сырые labels

Например в Kubernetes `Prometheus` через `kubernetes_sd_configs` получает много служебных полей:
- namespace
- pod name
- container name
- service name
- node name
- annotations
- pod IP
- ports
- phase

Они часто приходят как labels вида:
- `__meta_kubernetes_pod_name`
- `__meta_kubernetes_namespace`
- `__meta_kubernetes_pod_annotation_prometheus_io_scrape`
- `__meta_kubernetes_pod_container_port_number`

Это еще не те labels, которые ты потом хочешь видеть на dashboard.

## Что делает relabeling

С relabeling обычно делают три вещи:

### 1. Фильтруют targets

Например оставить только pod'ы с аннотацией:

```yaml
relabel_configs:
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
    regex: "true"
    action: keep
```

Это значит:
- если annotation не `true`, target выкидывается.

### 2. Меняют scrape address

Например discovery нашел pod IP и port, а ты хочешь собрать из них итоговый `__address__`:

```yaml
relabel_configs:
  - source_labels:
      [__meta_kubernetes_pod_ip, __meta_kubernetes_pod_annotation_prometheus_io_port]
    separator: ":"
    target_label: __address__
```

Это определяет, куда именно Prometheus пойдет по `HTTP`.

### 3. Создают нормальные labels

Например:

```yaml
relabel_configs:
  - source_labels: [__meta_kubernetes_namespace]
    target_label: namespace

  - source_labels: [__meta_kubernetes_pod_name]
    target_label: pod
```

После этого в series уже будут человеческие labels:
- `namespace`
- `pod`

вместо длинных `__meta_*`.

## Почему это важно

Если relabeling не продуман:
- Prometheus будет скрейпить лишние targets;
- labels будут шумными и бессмысленными;
- dashboards станут неудобными;
- cardinality может вырасти без причины.

Если relabeling сделан нормально:
- targets предсказуемы;
- labels readable;
- queries проще;
- dashboards стабильнее.

## Частые действия в relabel_configs

### keep

Оставить target:

```yaml
action: keep
```

### drop

Выкинуть target:

```yaml
action: drop
```

### replace

Записать значение в новый label:

```yaml
target_label: pod
action: replace
```

### labelmap

Массово переносить набор labels по regex.

Это мощно, но использовать надо аккуратно, чтобы не потянуть лишний мусор.

### labeldrop / labelkeep

Удалять или сохранять только часть labels.

Очень полезно против label noise.

## Важные служебные labels

### `__address__`

Это конечный адрес scrape target.

Если его не настроить как нужно, Prometheus может:
- идти не туда;
- ходить на неверный порт;
- вообще не достучаться до endpoint.

### `__metrics_path__`

Позволяет поменять путь вместо `/metrics`.

Например:

```yaml
relabel_configs:
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
    target_label: __metrics_path__
```

### `job`

Часто задается в scrape config и потом используется в queries:

```promql
rate(http_requests_total{job="shortener"}[5m])
```

### `instance`

Обычно отражает конкретный target address.

Это полезно для per-target debugging.

## Как labels попадают в time series

После scrape:
- labels из самой метрики;
- labels target'а;
- labels после relabeling

объединяются в итоговую time series.

Например приложение отдало:

```text
http_requests_total{route="/api/v1/links",status_code="201"}
```

После scrape и relabeling ты можешь получить:

```text
http_requests_total{
  job="shortener",
  namespace="prod",
  pod="shortener-abc",
  instance="10.42.1.15:8080",
  route="/api/v1/links",
  status_code="201"
}
```

Именно с этим набором labels потом работает `PromQL`.

## Где тут главные риски

### 1. Тащить слишком много infra labels

Если бездумно тащить все `__meta_*`, будет:
- шум;
- путаница;
- лишняя cardinality.

### 2. Ломать `__address__`

Если неверно склеить IP/port:
- target станет `down`;
- `/metrics` не будет скрейпиться.

### 3. Строить dashboards по нестабильным labels

Плохо строить панель по label, который меняется при каждом rollout:
- случайный pod hash
- ephemeral container name

Лучше строить по:
- `job`
- `service`
- `route`
- `operation`
- `namespace`

### 4. Путать target labels и metric labels

`route="/api"` обычно приходит из приложения.

`pod="shortener-abc"` обычно приходит из target discovery/relabeling.

Это разные уровни данных, хотя потом они лежат в одной series.

## Practical example

Типичный mental model для Kubernetes:

1. `Prometheus` спрашивает API server про pod'ы.
2. Получает все `__meta_kubernetes_*`.
3. `relabel_configs`:
   - оставляет только scrape-enabled pod'ы;
   - собирает `__address__`;
   - кладет `namespace`, `pod`, `service` в итоговые labels;
   - выкидывает лишний мусор.
4. Потом уже делает `GET /metrics`.
5. Сохраняет series с clean labels.

## Practical Rule

Если коротко:
- service discovery отвечает на вопрос “какие targets существуют?”;
- relabeling отвечает на вопрос “какие из них реально скрейпить и какие labels у них будут?”;
- именно relabeling превращает сырые infra labels в usable target model для Prometheus и Grafana.
