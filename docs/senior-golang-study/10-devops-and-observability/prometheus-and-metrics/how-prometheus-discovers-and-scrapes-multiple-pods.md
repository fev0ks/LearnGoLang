# How Prometheus Discovers And Scrapes Multiple Pods

Эта заметка нужна, чтобы понимать, как `Prometheus` работает в реальной распределенной среде, где у одного сервиса много pod'ов.

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Откуда Prometheus вообще узнает про pod'ы](#откуда-prometheus-вообще-узнает-про-podы)
- [Как происходит scrape](#как-происходит-scrape)
- [Что видит Prometheus после scrape](#что-видит-prometheus-после-scrape)
- [Почему это нормально](#почему-это-нормально)
- [Как потом получить картину по сервису](#как-потом-получить-картину-по-сервису)
- [Как посмотреть один pod отдельно](#как-посмотреть-один-pod-отдельно)
- [Что происходит при рестарте pod'а](#что-происходит-при-рестарте-podа)
- [Что обычно путают](#что-обычно-путают)
- [Где тут важен relabeling](#где-тут-важен-relabeling)
- [Практический mental model](#практический-mental-model)
- [Practical Rule](#practical-rule)

## Самая короткая интуиция

Если у тебя 5 pod'ов одного сервиса:
- нет одного общего `/metrics` на весь сервис;
- у каждого pod свой `/metrics`;
- `Prometheus` знает про все 5 target'ов;
- ходит в каждый отдельно;
- потом хранит их как отдельные series.

То есть:
- приложение считает локальные метрики;
- `Prometheus` делает global view только на уровне query/aggregation.

## Откуда Prometheus вообще узнает про pod'ы

Есть несколько способов.

### 1. Static configs

Самый простой вариант:

```yaml
scrape_configs:
  - job_name: shortener
    static_configs:
      - targets:
          - shortener-1:8080
          - shortener-2:8080
          - shortener-3:8080
```

Это удобно:
- локально;
- в docker-compose;
- в маленьких стендах.

Но это плохо масштабируется, потому что target list надо поддерживать руками.

### 2. Kubernetes service discovery

Это самый частый real-world вариант.

Например:

```yaml
scrape_configs:
  - job_name: kubernetes-pods
    kubernetes_sd_configs:
      - role: pod
```

Тогда `Prometheus` сам спрашивает у Kubernetes API:
- какие pod'ы сейчас существуют;
- какие у них labels;
- какие IP/ports;
- какие annotations;
- какие namespace.

И на основе этого строит target list.

То есть в Kubernetes `Prometheus` не "угадывает" pod'ы — он получает их из service discovery.

### 3. Service discovery в других системах

Кроме Kubernetes, бывают:
- Consul
- EC2
- GCE
- file-based SD
- DNS-based discovery

Принцип всегда один:
- есть источник правды о target'ах;
- `Prometheus` периодически синхронизирует target list;
- потом scrapes их по `HTTP`.

## Как происходит scrape

Когда target list уже известен, `Prometheus` делает обычный `HTTP GET` на `/metrics`.

Например:

```text
GET http://10.42.1.15:8080/metrics
GET http://10.42.1.16:8080/metrics
GET http://10.42.1.17:8080/metrics
```

То есть каждый pod опрашивается отдельно.

Обычно для каждого target есть:
- `job`
- `instance`
- и еще k8s labels вроде `pod`, `namespace`, `service`

## Что видит Prometheus после scrape

Допустим, каждый pod отдает:

```text
http_requests_total{route="/api/v1/links",status_code="201"} ...
```

`Prometheus` добавит target labels и получит разные series:

```text
http_requests_total{job="shortener",instance="10.42.1.15:8080",pod="shortener-abc",route="/api/v1/links",status_code="201"}
http_requests_total{job="shortener",instance="10.42.1.16:8080",pod="shortener-def",route="/api/v1/links",status_code="201"}
http_requests_total{job="shortener",instance="10.42.1.17:8080",pod="shortener-ghi",route="/api/v1/links",status_code="201"}
```

То есть одна логическая метрика превращается в несколько time series — по одной на target/label set.

## Почему это нормально

Это и есть правильная модель `Prometheus`.

Она дает:
- независимость pod'ов друг от друга;
- отсутствие shared metrics state между pod'ами;
- простое горизонтальное масштабирование;
- возможность смотреть и per-pod, и service-level signals.

## Как потом получить картину по сервису

Через aggregation в `PromQL`.

Например суммарный RPS по всем pod'ам сервиса:

```promql
sum(rate(http_requests_total{job="shortener"}[5m]))
```

Или убрать target noise:

```promql
sum without (instance, pod) (
  rate(http_requests_total{job="shortener"}[5m])
)
```

То есть:
- `/metrics` pod-level;
- `PromQL` service-level.

## Как посмотреть один pod отдельно

Например:

```promql
rate(http_requests_total{pod="shortener-abc"}[5m])
```

Или p95 по pod:

```promql
histogram_quantile(
  0.95,
  sum by (pod, le) (
    rate(http_request_duration_seconds_bucket{job="shortener"}[5m])
  )
)
```

Это полезно, если:
- один pod деградировал;
- rollout сломал только часть replicas;
- есть hotspot / imbalance.

## Что происходит при рестарте pod'а

Очень важный момент:
- локальные метрики в pod memory сбрасываются;
- новый pod начинает counters заново.

Но это нормально, потому что `Prometheus` обычно работает с:
- `rate()`
- `increase()`

И умеет жить с reset counters.

Поэтому raw absolute counter на одном pod:
- редко useful;
- и почти никогда не главный operational signal.

## Что обычно путают

### 1. "Сервис сам знает свои глобальные метрики"

Нет.

Обычно сервис знает только локальные метрики текущего процесса.

Глобальная картина появляется только в `Prometheus`.

### 2. "Один Service в Kubernetes = один target"

Не обязательно.

Логический сервис и scrape target — разные вещи.

`Prometheus` часто scrapes:
- pod endpoints;
- service endpoints;
- sidecars;
- exporters.

### 3. "Если pod'ов 10, надо как-то вручную суммировать в приложении"

Нет.

Именно этого делать и не надо.

Сумма строится в query layer:
- `sum(...)`
- `sum by (...)`
- `sum without (...)`

## Где тут важен relabeling

В Kubernetes service discovery часто получают много служебных labels.

Их потом:
- фильтруют;
- переименовывают;
- превращают в cleaner target labels.

Это делает `relabel_configs`.

То есть discovery отвечает на вопрос:
- "какие targets вообще существуют?"

А relabeling отвечает на вопрос:
- "какие labels и endpoints мы реально будем использовать?"

## Практический mental model

Если коротко:

- `Prometheus` не смотрит на “сервис” как на одну коробку;
- он смотрит на набор target'ов;
- каждый target отдает свои локальные метрики;
- дальше query layer собирает service-level answer.

## Practical Rule

Запомнить стоит так:

- много pod'ов = много `/metrics` endpoints;
- `Prometheus` discovers targets через static config или service discovery;
- каждый pod scrapes separately;
- каждая target+label combination становится своей time series;
- сервисный график — это уже результат `PromQL` aggregation, а не того, что приложение считает глобально.
