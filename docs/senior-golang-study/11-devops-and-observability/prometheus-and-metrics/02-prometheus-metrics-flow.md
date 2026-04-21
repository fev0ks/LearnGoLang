# Prometheus Metrics Flow

Эта заметка нужна, чтобы понимать `Prometheus` не как "место, где лежат графики", а как конкретный pipeline:

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Почему pull model важна](#почему-pull-model-важна)
- [Что происходит внутри приложения](#что-происходит-внутри-приложения)
- [Как выглядит `/metrics`](#как-выглядит-metrics)
- [Что делает Prometheus на scrape](#что-делает-prometheus-на-scrape)
- [Как строится flow end-to-end](#как-строится-flow-end-to-end)
- [Как думать о метриках правильно](#как-думать-о-метриках-правильно)
- [Основные классы метрик в backend-сервисе](#основные-классы-метрик-в-backend-сервисе)
- [Что такое good metric contract](#что-такое-good-metric-contract)
- [Где чаще всего ломаются](#где-чаще-всего-ломаются)
- [Practical Rule](#practical-rule)

## Самая короткая интуиция

`Prometheus` почти всегда работает так:
- приложение само считает метрики в памяти;
- приложение отдает их на `/metrics`;
- `Prometheus` сам приходит и забирает эти данные;
- дальше он хранит их как time series и дает язык запросов `PromQL`.

Главная идея:
- приложение не "пушит графики";
- `Prometheus` сам регулярно делает `pull`.

## Почему pull model важна

Это один из ключевых моментов.

`Prometheus` предпочитает:
- `scrape` приложения по `HTTP`;
- видеть текущее состояние прямо в момент запроса;
- иметь единый контроль над интервалом опроса.

Плюсы такого подхода:
- проще централизованно конфигурировать, кого и как собирать;
- проще понимать, что target пропал;
- удобнее дебажить, потому что `/metrics` можно открыть руками;
- не надо заставлять каждое приложение знать, куда пушить данные.

Минусы:
- target должен быть reachable для Prometheus;
- для batch jobs и короткоживущих процессов часто нужен `Pushgateway` или иной path, но это отдельный special case.

## Что происходит внутри приложения

Обычно в Go это выглядит так:

```go
var httpRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "my_service",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total number of HTTP requests.",
	},
	[]string{"method", "route", "status_code"},
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
}
```

Идея здесь простая:
- метрика создается в коде;
- регистрируется в in-process registry;
- потом `promhttp.Handler()` или аналог отдает все это через `/metrics`.

## Как выглядит `/metrics`

Обычно endpoint возвращает текстовый exposition format:

```text
# HELP my_service_http_requests_total Total number of HTTP requests.
# TYPE my_service_http_requests_total counter
my_service_http_requests_total{method="GET",route="/health",status_code="200"} 42
my_service_http_requests_total{method="POST",route="/api/v1/links",status_code="201"} 15
```

Что тут важно:
- metric name: `my_service_http_requests_total`
- labels: `method`, `route`, `status_code`
- value: числовое значение

То есть `Prometheus` видит не "одну метрику", а набор series:

```text
metric_name + label set = отдельная time series
```

## Что делает Prometheus на scrape

Во время scrape:
- `Prometheus` идет на target, например `http://shortener:8080/metrics`;
- читает все текущие значения;
- пишет sample в свою time-series базу;
- помечает, жив ли target;
- использует timestamp scrape-момента.

То есть каждое значение живет во времени:

```text
my_service_http_requests_total{route="/api"} @ t1 = 10
my_service_http_requests_total{route="/api"} @ t2 = 14
my_service_http_requests_total{route="/api"} @ t3 = 19
```

Из этого уже можно считать:
- скорость роста;
- изменения за окно;
- latency percentiles;
- error rate.

## Как строится flow end-to-end

### 1. Код инструментария

Ты вшиваешь счетчики, гейджи, гистограммы в:
- HTTP middleware;
- service layer;
- repository layer;
- Kafka consumer;
- Redis/Postgres/ClickHouse path.

### 2. Export endpoint

Сервис публикует `/metrics`.

Типичный вопрос:
- нужно ли отдавать metrics отдельным портом?

Ответ:
- локально и в небольших сервисах часто достаточно того же HTTP server;
- в больших системах иногда делают отдельный admin/metrics port.

### 3. Prometheus scrape config

В конфиге задаются targets:

```yaml
scrape_configs:
  - job_name: shortener
    metrics_path: /metrics
    static_configs:
      - targets: ["shortener:8080"]
```

Именно это связывает приложение и Prometheus.

### 4. Query layer

Дальше ты спрашиваешь уже не raw value, а time-based вопрос:

```promql
rate(my_service_http_requests_total[5m])
```

или

```promql
histogram_quantile(0.95, sum by (le) (rate(my_service_http_request_duration_seconds_bucket[5m])))
```

### 5. Visualization and alerting

После этого:
- `Grafana` рисует dashboards;
- alert rules смотрят на те же series;
- recording rules считают pre-aggregated views.

## Как думать о метриках правильно

Нельзя начинать с вопроса:
- "какие метрики обычно ставят?"

Надо начинать с вопроса:
- "на какой operational question эта метрика отвечает?"

Примеры нормальных вопросов:
- растет ли ошибка на create endpoint;
- падает ли cache hit ratio;
- растет ли latency Postgres path;
- успевает ли consumer обрабатывать сообщения;
- сколько времени занимает ClickHouse insert;
- есть ли backpressure на event pipeline.

## Основные классы метрик в backend-сервисе

### HTTP / API

Почти всегда нужны:
- `requests_total`
- `request_duration_seconds`
- иногда `in_flight_requests`

Обычно labels:
- `method`
- `route`
- `status_code`

### Domain / business

Это не инфраструктура, а продуктовые события:
- `link_creates_total`
- `redirect_resolves_total`
- `link_visit_publish_total`

Их сила в том, что они отвечают на вопросы продукта, а не только платформы.

### Storage

Очень полезны:
- `postgres_operations_total`
- `postgres_operation_duration_seconds`
- `redis_operations_total`
- `redis_operation_duration_seconds`

Обычно labels:
- `operation`
- `result`

Никогда не надо пихать туда:
- raw SQL
- Redis key
- full URL
- user ID

### Async / worker

Для consumer'ов и jobs обычно нужны:
- `events_total`
- `operation_duration_seconds`
- queue lag
- retry / DLQ counters

## Что такое good metric contract

Хорошая метрика:
- стабильна по имени;
- имеет мало labels;
- labels имеют ограниченный набор значений;
- соответствует конкретному вопросу;
- не тянет high-cardinality поля.

Плохая метрика:
- зависит от raw path, `request_id`, user id, SQL text или short code;
- меняется именем от случая к случаю;
- дублирует то, что уже лучше видно в traces или logs.

## Где чаще всего ломаются

### 1. Слишком много labels

Если добавить label вроде:
- `user_id`
- `email`
- `trace_id`
- `request_id`
- `short_code`

то можно получить взрыв cardinality.

Это опасно, потому что:
- растет память;
- тяжелее queries;
- дороже storage;
- Prometheus/Grafana становятся медленными.

### 2. Неправильное понимание counter

Counter не надо читать как "текущее число".

Его надо читать через:
- `rate()`
- `increase()`

То есть не:

```promql
my_service_http_requests_total
```

а чаще:

```promql
rate(my_service_http_requests_total[5m])
```

### 3. Смешивание ownership

Нельзя свалить все метрики в один абстрактный giant file без структуры.

Лучше группировать по смыслу:
- `http`
- `domain`
- `storage`
- `eventing`
- `analytics`

### 4. Считать probe traffic как user traffic

Очень частая ошибка:
- `/metrics`
- `/health/live`
- `/health/ready`

попадают в те же HTTP metrics, что и реальные пользовательские запросы.

Итог:
- RPS и latency dashboards искажаются.

## Practical Rule

Если коротко:

- приложение считает метрики локально;
- `Prometheus` scrapes `/metrics`;
- каждая комбинация `metric + labels` — отдельная series;
- дальше `PromQL` отвечает уже не на вопрос "какое число сейчас", а на вопрос "как signal вел себя во времени".

Это и есть правильная mental model для Prometheus.
