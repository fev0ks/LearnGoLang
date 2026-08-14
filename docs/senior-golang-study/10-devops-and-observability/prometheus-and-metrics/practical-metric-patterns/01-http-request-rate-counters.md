# Request rate: сколько трафика получает HTTP API

## Содержание

- [Что измеряет counter запросов](#что-измеряет-counter-запросов)
- [В какой момент считать запрос](#в-какой-момент-считать-запрос)
- [Как получить скорость и число запросов](#как-получить-скорость-и-число-запросов)
- [Какие разрезы полезны](#какие-разрезы-полезны)
- [Как читать рост и падение трафика](#как-читать-рост-и-падение-трафика)
- [Редкий трафик](#редкий-трафик)
- [Counter resets и rollout](#counter-resets-и-rollout)
- [Полезные panels и alerts](#полезные-panels-и-alerts)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Request rate отвечает на вопрос «с какой скоростью сервис завершает HTTP-
запросы?». Это первый сигнал RED, но сам по себе он не различает рост полезной
нагрузки, retry storm и перераспределение трафика после отказа реплики.

---

## Что измеряет counter запросов

Базовая метрика:

```text
shortener_http_requests_total{
  method="GET",
  route="/links/{id}",
  status_code="200"
}
```

Counter накапливает число событий внутри процесса. Он растёт до рестарта и не
является готовым RPS. Набор labels должен быть ограниченным:

- `method` — HTTP method;
- `route` — шаблон route, а не raw path;
- `status_code` или `status_class` — нормализованный результат;
- при необходимости `protocol` или другой небольшой стабильный enum.

`service`, `namespace`, `pod` и `instance` обычно добавляются как target labels,
а не записываются HTTP middleware вручную.

---

## В какой момент считать запрос

Counter можно увеличить в начале или в конце запроса, но контракт должен быть
последовательным. Для HTTP API обычно удобнее считать завершённые запросы:

```text
request starts
  -> in_flight +1
  -> handler executes
  -> response status known
  -> requests_total +1
  -> duration Observe(...)
  -> in_flight -1
```

Так request counter, result label и duration описывают одно множество
завершённых запросов. Запрос, который завис навсегда, ещё не попадёт в rate, но
останется в `in_flight` и повлияет на saturation/timeout signals.

Если middleware считает запросы в начале, rate ближе к arrival rate, но код
ответа ещё неизвестен. Тогда нужны отдельные метрики started/completed или иной
явный контракт. Нельзя молча назвать arrival counter `requests_total`, а error
ratio делить на completed errors: числитель и знаменатель будут описывать разные
события.

Служебные endpoints `/metrics`, `/health/live` и `/health/ready` либо исключают,
либо сохраняют как отдельные routes. Решение должно быть одинаковым между
сервисами, иначе service RPS несопоставим.

---

## Как получить скорость и число запросов

### RPS одной series

```promql
rate(shortener_http_requests_total[5m])
```

### RPS всего сервиса

```promql
sum(
  rate(shortener_http_requests_total{job="shortener"}[5m])
)
```

### Число запросов за 15 минут

```promql
sum(
  increase(shortener_http_requests_total{job="shortener"}[15m])
)
```

`rate()` даёт среднюю скорость в секунду. `increase()` оценивает рост за окно и
может вернуть дробное число из-за экстраполяции к границам диапазона.

Сначала вызывают `rate()` или `increase()` на каждом counter, затем суммируют.
Так resets каждой реплики учитываются до aggregation.

---

## Какие разрезы полезны

### По route

```promql
sum by (route) (
  rate(shortener_http_requests_total[5m])
)
```

Показывает, какие пользовательские операции создают нагрузку.

### По status code

```promql
sum by (status_code) (
  rate(shortener_http_requests_total[5m])
)
```

Это breakdown outcomes, но не замена error ratio: 100 ошибок в секунду имеют
разный impact при 200 и 100 000 requests/s.

### По pod

```promql
sum by (pod) (
  rate(shortener_http_requests_total{job="shortener"}[5m])
)
```

Используется для проверки балансировки и частичного rollout. Верхний service
panel не должен группировать по pod, потому что его identity меняется.

### По method и route

```promql
sum by (method, route) (
  rate(shortener_http_requests_total[5m])
)
```

Полезно, если один route template обслуживает несколько методов с разным
смыслом. Если `route` уже хранит строку вроде `GET /links/{id}`, отдельный
`method` дублирует информацию; схему нужно выбрать один раз.

---

## Как читать рост и падение трафика

### Резкий рост

Возможные объяснения:

- реальная пользовательская или маркетинговая нагрузка;
- retry storm клиента, gateway или worker;
- bot/abuse traffic;
- цикл webhook;
- возврат трафика после outage;
- новый route label объединил несколько операций.

Проверяют одновременно route/status breakdown, уникальные client signals в
logs/traces, ingress metrics, latency и saturation. Рост RPS при стабильных
errors и latency может быть штатным, но capacity headroom всё равно меняется.

### Резкое падение

Возможные объяснения:

- упал реальный спрос;
- ingress, DNS или routing перестал направлять запросы;
- upstream деградировал раньше текущего сервиса;
- часть targets исчезла;
- dashboard filter больше не совпадает после deploy;
- counters существуют, но окно `rate()` слишком короткое после rollout.

Нулевой RPS приложения и `up=1` не доказывает здоровье: scraper доступен, а
пользовательский путь может быть сломан до приложения.

---

## Редкий трафик

Для route с одним запросом в несколько минут короткий `rate()` будет нулевым или
неровным. Вопрос «сколько событий произошло» лучше выражает `increase()`:

```promql
sum by (route) (
  increase(shortener_http_requests_total[1h])
)
```

Длинное окно улучшает устойчивость, но скрывает точное время события. Для
расследования единичного запроса нужны logs или traces; metrics предназначены
для агрегированного поведения.

Alert «RPS равен нулю» корректен только для сервиса, который действительно
обязан непрерывно получать трафик. Для периодического API ноль является нормой.
Вместо универсального alert можно контролировать synthetic probe, свежесть
бизнес-события или отклонение от сезонного baseline.

---

## Counter resets и rollout

После рестарта pod локальный counter начинает новый жизненный цикл. Raw lines:

```text
pod-a: 1200 -> 1210 -> pod removed
pod-b:                    0 -> 8 -> 19
```

Это не потеря requests в monitoring model. `rate()` учитывает reset в одном
ряду, а новый `pod` обычно создаёт отдельный label set. Service query суммирует
rates живых series:

```promql
sum(rate(shortener_http_requests_total{job="shortener"}[5m]))
```

Суммировать raw counters между pods бессмысленно для operational view: результат
зависит от возраста процессов. Сохранение counters в общей базе ради графика
создаёт contention и отказоустойчивость хуже, чем локальная инструментация и
PromQL.

---

## Полезные panels и alerts

Минимальный набор panels:

1. Общий RPS сервиса.
2. RPS по ключевым routes.
3. Breakdown по status class/code.
4. RPS по pod как drill-down.
5. Сопоставленные error ratio, p95 latency и in-flight.

Alert только на высокий RPS редко отражает пользовательский impact. Он уместен,
когда есть известный capacity boundary, подтверждённый load test:

```promql
sum(rate(shortener_http_requests_total[5m])) > 12000
```

Число `12000` имеет смысл лишь вместе с условием: при каком route mix, числе
replicas и SLO оно измерено. Если autoscaling должен срабатывать раньше, alert
связывают с saturation или исчерпанием headroom, а не с произвольной красивой
границей.

---

## Типичные ошибки

1. Raw counter подписан `RPS`.
2. Counters pods суммируются до `rate()`, поэтому resets обрабатываются неверно.
3. `route` содержит raw path и создаёт ряд на каждый ID.
4. Started requests являются знаменателем для completed errors без явного
   контракта.
5. Probes и `/metrics` незаметно доминируют в трафике малого сервиса.
6. Падение RPS автоматически трактуется как проблема текущего сервиса без
   проверки ingress и upstream.
7. Нулевой RPS alert применяется к периодическому или новому route.
8. RPS по pod используется как главный service panel и ломается при rollout.

---

## Interview-ready answer

**1. Как получить RPS из request counter?**

- Шаг 1 — выбрать counters нужного сервиса.
- Шаг 2 — применить `rate(...[window])` к каждой series.
- Шаг 3 — сложить реплики через `sum` или сохранить нужный разрез через
  `sum by`.
- Порядок — `rate` выполняется до aggregation ради корректной обработки resets.

**2. Когда увеличивать HTTP request counter?**

- Предпочтение — после завершения запроса, когда известны status и duration.
- Согласованность — тогда counter, errors и histogram описывают одно множество
  операций.
- Активная работа — незавершённые запросы отдельно показывает `in_flight` gauge.

**3. Что означает резкое падение RPS?**

- Неоднозначность — это может быть падение спроса, routing failure, upstream
  outage, исчезновение targets или ошибка dashboard filter.
- Проверка — сравнить ingress, `up`, routes, status codes, deploy timeline и
  expected traffic profile.
- Вывод — request rate показывает симптом трафика, а не готовую причину.
