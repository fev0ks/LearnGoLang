# Grafana Overview And Functionality

Эта заметка нужна, чтобы понимать `Grafana` не как "инструмент для красивых графиков", а как центральный UI и platform layer для observability.

Коротко:
- `Grafana` это интерфейс и платформа для работы с метриками, логами, трейcами, алертами и другими observability-данными;
- она не обязана сама хранить все данные;
- чаще всего `Grafana` подключается к backend-системам и даёт единое место для investigation и dashboards.

## Самая короткая интуиция

Если сильно упростить:

- `Prometheus` хранит метрики
- `Loki` хранит логи
- `Tempo` хранит трейсы
- `Grafana` даёт единый интерфейс для работы со всем этим

То есть типичный grafana-centric stack выглядит так:

```text
metrics -> Prometheus
logs -> Loki
traces -> Tempo
UI -> Grafana
```

Но важно:
- `Grafana` умеет работать не только с этими backend'ами;
- она поддерживает много data sources, поэтому её сила именно в unified observability UI.

## Что такое Grafana по сути

`Grafana` решает несколько задач сразу:
- подключение к разным источникам данных;
- визуализация через dashboards и panels;
- исследование данных через interactive queries;
- корреляция между logs, metrics и traces;
- alerting;
- sharing и operational collaboration.

Это важно: `Grafana` чаще не storage system, а control plane и UI поверх storage backends.

## Главные сущности в Grafana

### Data Sources

`Data source` — это подключение к backend-системе.

Частые примеры:
- `Prometheus`
- `Loki`
- `Tempo`
- `Elasticsearch`
- `PostgreSQL`
- `ClickHouse`
- `CloudWatch`
- `Google Cloud Monitoring`

Практический смысл:
- одна `Grafana` может объединить несколько разных систем в одном интерфейсе.

### Dashboards

`Dashboard` — это экран из панелей, где собраны нужные графики, таблицы и статусы.

Зачем нужны dashboards:
- следить за состоянием системы;
- быстро видеть деградацию;
- собирать operational overview для сервиса, команды или платформы.

Типичный dashboard для backend-сервиса включает:
- RPS или throughput;
- latency;
- error rate;
- saturation signals;
- бизнес-метрики;
- ссылки на logs и traces.

### Panels

`Panel` — отдельный виджет внутри dashboard.

Это может быть:
- time series graph;
- table;
- stat panel;
- heatmap;
- logs panel;
- trace-related panel.

### Explore

`Explore` — это режим для интерактивного investigation, а не для статичного dashboard.

Здесь обычно:
- запускают ad-hoc queries;
- ищут конкретный `request_id`;
- смотрят логи за нужный интервал;
- переходят к trace;
- сравнивают сигналы рядом.

Практически `Explore` — это один из самых полезных режимов для on-call и incident response.

### Alerting

`Grafana` умеет строить alert rules и отправлять уведомления.

Обычно это используют для:
- high error rate;
- latency spikes;
- no data;
- saturation;
- SLO/SLA monitoring.

## Как Grafana работает с разными типами сигналов

### Метрики

Метрики — это агрегированные численные сигналы во времени.

Чаще всего в `Grafana` это:
- `Prometheus`
- `Mimir`
- `Cloud Monitoring`
- `CloudWatch`

Через них отвечают на вопросы:
- растёт ли latency;
- сколько ошибок в минуту;
- какой throughput;
- хватает ли CPU и памяти.

### Логи

Логи — это событийные записи.

Чаще всего в `Grafana` это:
- `Loki`
- иногда `Elasticsearch`
- иногда облачные backends

Через них отвечают на вопросы:
- что именно произошло;
- какой `request_id` упал;
- какой `error_kind` повторяется;
- какой сервис пишет ошибки.

### Трейсы

Трейсы показывают путь одного запроса через систему.

Обычно в `Grafana` это:
- `Tempo`
- иногда Jaeger/Zipkin-compatible backends

Через них отвечают на вопросы:
- где именно тормозит один запрос;
- в каком downstream вызове возникла проблема;
- какой span съел большую часть latency.

## Что такое Tempo внутри grafana-стека

`Tempo` — это backend для distributed tracing в экосистеме `Grafana`.

Если коротко:
- лог говорит "что произошло";
- метрика говорит "насколько часто и насколько сильно";
- trace говорит "где именно в цепочке это произошло".

`Tempo` хранит traces, а `Grafana` позволяет:
- открыть конкретный trace;
- смотреть дерево spans;
- видеть длительности по шагам;
- переходить от логов к trace и обратно, если есть `trace_id`.

Практическая цепочка выглядит так:

```text
user request -> trace spans -> Tempo
application logs with trace_id -> Loki
service metrics -> Prometheus
Grafana -> единая точка расследования
```

Это один из самых сильных сценариев `Grafana`:
- в одном UI можно увидеть метрику деградации;
- затем открыть логи;
- затем перейти к trace;
- затем найти конкретный медленный span.

## Почему Grafana часто любят platform teams

Потому что она хорошо решает задачу "единая observability-витрина":
- один UI;
- много разных backends;
- удобные dashboards;
- investigation workflows;
- correlation между сигналами;
- плагины и интеграции.

То есть `Grafana` особенно удобна, когда инфраструктура не монолитна и данные лежат не в одной системе.

## Где Grafana особенно сильна

`Grafana` особенно хороша, когда:
- нужны dashboards по метрикам;
- нужен единый UI для logs, metrics и traces;
- observability already built around `Prometheus/Loki/Tempo`;
- есть много разных data sources;
- команда хочет open and composable tooling.

## Где Grafana не стоит идеализировать

Важно помнить:
- `Grafana` не заменяет сама по себе backend для логов, метрик или трейсов;
- если underlying storage плохой, `Grafana` это не исправит;
- search-first workflows в `Elasticsearch + Kibana` могут быть естественнее, чем через `Grafana`;
- часть силы `Grafana` раскрывается только когда данные и labels/metadata организованы хорошо.

## Типичный workflow расследования через Grafana

1. Видим всплеск ошибки или latency на dashboard.
2. Переходим в `Explore`.
3. Смотрим логи нужного сервиса в `Loki`.
4. Находим `trace_id` или корреляционное поле.
5. Открываем trace из `Tempo`.
6. Видим, какой span был медленным или упал.
7. Возвращаемся к метрикам и проверяем масштаб проблемы.

Это и есть одна из главных идей `Grafana`: не просто визуализация, а быстрый переход между типами сигналов.

## Practical Rule

Если нужно быстро понять, что такое `Grafana`, полезно запомнить так:

- `Grafana` это не просто dashboard tool;
- это observability UI и platform layer;
- её главная сила в unified view поверх разных backends;
- в grafana-centric стеке `Prometheus` отвечает за metrics, `Loki` за logs, `Tempo` за traces.

## Что полезно уметь сказать на интервью

- `Grafana` это универсальный observability UI, а не только инструмент для графиков.
- Она обычно не хранит все данные сама, а подключается к backend-системам.
- Её сильная сторона — dashboards, `Explore`, alerting и correlation между logs, metrics и traces.
- `Tempo` в этой экосистеме — backend для distributed tracing.
- Типичный стек: `Prometheus + Loki + Tempo + Grafana`.
