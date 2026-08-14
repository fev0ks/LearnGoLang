# HTTP error rate: абсолютные ошибки, ratio и SLO

## Содержание

- [Что считать ошибкой](#что-считать-ошибкой)
- [Абсолютная скорость и доля ошибок](#абсолютная-скорость-и-доля-ошибок)
- [4xx и 5xx: полезное, но неполное разделение](#4xx-и-5xx-полезное-но-неполное-разделение)
- [Ошибки по route](#ошибки-по-route)
- [Низкий трафик и деление на ноль](#низкий-трафик-и-деление-на-ноль)
- [От error ratio к SLI и burn rate](#от-error-ratio-к-sli-и-burn-rate)
- [Как расследовать рост ошибок](#как-расследовать-рост-ошибок)
- [Полезные panels и alerts](#полезные-panels-и-alerts)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Error rate не появляется как «магическая метрика». Обычно это производный
сигнал из counter завершённых запросов. Сначала команда определяет, какие
результаты считаются успешными для пользователя, затем строит числитель и
знаменатель по одному множеству eligible requests.

---

## Что считать ошибкой

Транспортный статус и пользовательский результат связаны, но не тождественны.

Примеры:

- `500` почти всегда является ошибкой сервиса;
- `503` от контролируемого load shedding всё равно означает неуспех пользователя;
- `429` является `4xx`, но может нарушать availability SLO;
- `404` для поиска несуществующего объекта может быть ожидаемым результатом;
- `404` для статического asset после deploy может быть инцидентом;
- разрыв соединения или client cancellation может не превратиться в обычный
  status code приложения.

Поэтому нужны два уровня:

1. Общий protocol breakdown `2xx/4xx/5xx` для диагностики.
2. SLI-классификация eligible/success по контракту продукта.

Не стоит добавлять raw error message в label `error`. Ограниченный `result`
вроде `success|invalid|not_found|timeout|unavailable|internal_error` даёт
стабильный operational breakdown, а детали остаются в traces и logs.

---

## Абсолютная скорость и доля ошибок

### 5xx в секунду

```promql
sum(
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
```

Ответ: сколько 5xx сейчас происходит в среднем за секунду.

### Доля 5xx

```promql
sum(
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
/
sum(
  rate(shortener_http_requests_total[5m])
)
```

Ответ: какая часть завершённых запросов получает 5xx.

Оба сигнала нужны одновременно:

| Трафик | Ошибки | Error ratio | Интерпретация |
| ---: | ---: | ---: | --- |
| 100 000 req/s | 100 err/s | 0.1% | Большое число событий, малая доля |
| 20 req/s | 5 err/s | 25% | Малое число событий, сильный impact |
| 0 req/s | 0 err/s | `NaN` | Нет наблюдений, а не 0% ошибок |

Ratio от `0` до `1` можно отображать в Grafana как percent. Значение `0.01`
равно `1%`, а не `0.01%`.

---

## 4xx и 5xx: полезное, но неполное разделение

### Breakdown по классу

Если есть label `status_class`:

```promql
sum by (status_class) (
  rate(shortener_http_requests_total[5m])
)
```

Если экспортируется точный `status_code`:

```promql
sum by (status_code) (
  rate(shortener_http_requests_total[5m])
)
```

5xx обычно указывает на server-side failure, но его причина может быть в
downstream, timeout budget или намеренном overload protection. 4xx часто
описывает запрос клиента, но рост `401`, `403` или `429` после deploy способен
быть настоящей пользовательской деградацией.

Правильный вопрос: «получил ли пользователь допустимый результат для этой
операции?», а не «какая первая цифра HTTP status?».

### Отдельные outcomes

Для route `/links/{id}` ожидаемый `404` можно исключить из availability SLI, но
сохранить в diagnostic breakdown:

```promql
sum by (result) (
  rate(shortener_http_requests_total{route="/links/{id}"}[5m])
)
```

Такой запрос требует заранее определённого label `result`. Генерировать его в
PromQL из десятков status/route combinations можно, но recording rule быстро
становится второй реализацией бизнес-контракта. Лучше иметь единое определение и
тестировать его рядом с инструментацией или SLO rules.

---

## Ошибки по route

### 5xx rate по route

```promql
sum by (route) (
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
```

### 5xx ratio по route

```promql
sum by (route) (
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
/
sum by (route) (
  rate(shortener_http_requests_total[5m])
)
```

Обе стороны агрегированы до одного label set `route`, поэтому vector matching
однозначен.

Global ratio может скрыть небольшой критичный route:

```text
GET /health       10 000 req/s, 0 errors
POST /payments        10 req/s, 10 errors
global ratio ≈ 10 / 10 010 ≈ 0.1%
payments ratio = 10 / 10 = 100%
```

Сервисная панель и route-level SLO отвечают на разные вопросы. Нельзя считать,
что малый вклад route в общий RPS делает его неважным.

---

## Низкий трафик и деление на ноль

При отсутствии запросов числитель и знаменатель равны нулю, а ratio — `NaN`.
Это корректно: данных о качестве нет.

Механическое исправление:

```promql
errors / clamp_min(requests, 1)
```

искажает ratio при малом трафике. Вместо этого разделяют display и alert
semantics.

Для alert можно потребовать минимальный request rate:

```promql
(
  sum(rate(shortener_http_requests_total{status_code=~"5.."}[5m]))
  /
  sum(rate(shortener_http_requests_total[5m]))
  > 0.01
)
and
(
  sum(rate(shortener_http_requests_total[5m])) > 1
)
```

Порог объёма `1 req/s` здесь является допущением примера. Он может скрыть
полный отказ низкочастотного, но критичного endpoint. Для такого route лучше
использовать longer window, synthetic probe или alert на число ошибок:

```promql
sum(increase(shortener_http_requests_total{
  route="/admin/report",
  status_code=~"5.."
}[30m])) > 2
```

`> 2` тоже должно следовать из допустимого error budget, а не из универсального
правила.

---

## От error ratio к SLI и burn rate

Availability SLI обычно определяют как:

```text
success ratio = successful eligible requests / all eligible requests
error ratio   = 1 - success ratio
```

Если SLO равен `99.9%`, error budget ratio:

```text
1 - 0.999 = 0.001 = 0.1%
```

При наблюдаемом error ratio `1%` скорость расходования бюджета:

```text
burn rate = 0.01 / 0.001 = 10
```

Это значит, что бюджет расходуется в десять раз быстрее равномерно допустимого
темпа. Burn-rate alert связывает ошибку с SLO и использует несколько окон, чтобы
различать быстрый крупный инцидент и медленную устойчивую деградацию.

Полная конфигурация multi-window multi-burn-rate зависит от SLO period,
evaluation interval и error budget policy. Здесь важна модель: порог alert
выводится из SLO, а не задаётся как одинаковые `5% ошибок` для всех сервисов.

---

## Как расследовать рост ошибок

Порядок проверки:

1. Сравнить absolute error rate, ratio и общий traffic volume.
2. Разделить status classes и продуктовые `result`.
3. Найти route, где появился рост.
4. Сравнить pods/version и deploy annotations.
5. Проверить p50/p95/p99 и in-flight/saturation.
6. Сопоставить client-side dependency errors и latency.
7. Перейти в traces по затронутому route и временному окну.
8. Использовать logs для конкретного exception/error detail.

Корреляция не равна причинности. Одновременный рост DB latency и HTTP 5xx
указывает на сильную гипотезу, которую подтверждают client error type, trace и
состояние пула.

---

## Полезные panels и alerts

Минимальный error view:

- 5xx или SLI error ratio;
- absolute errors/s;
- traffic volume рядом с ratio;
- breakdown по route/result;
- deploy annotations;
- связанная latency panel.

Alert на ratio обычно требует `for`, чтобы не реагировать на один scrape:

```yaml
- alert: ShortenerHighErrorRatio
  expr: |
    (
      sum(rate(shortener_http_requests_total{status_code=~"5.."}[5m]))
      /
      sum(rate(shortener_http_requests_total[5m]))
    ) > 0.01
    and
    sum(rate(shortener_http_requests_total[5m])) > 1
  for: 10m
```

Это демонстрация механики, а не готовые production thresholds. При окне `[5m]`
и `for: 10m` условие должно оставаться истинным на последовательных evaluations
десять минут; `for` не превращает запрос в «ошибки за 15 минут».

---

## Типичные ошибки

1. Любой 4xx исключается из пользовательских ошибок без проверки семантики.
2. 5xx/s показывается без общего RPS, поэтому impact неясен.
3. Ratio показывается без absolute count и маскирует единичный шум или большой
   объём событий.
4. Global ratio скрывает критичный малотрафиковый route.
5. Числитель и знаменатель считают разные множества started/completed requests.
6. `clamp_min(..., 1)` меняет смысл low-traffic ratio.
7. Отсутствие ряда превращается через `or vector(0)` в «ошибок нет», хотя target
   исчез.
8. Alert threshold не связан с SLO/error budget и одинаков для всех сервисов.

---

## Interview-ready answer

**1. Чем error rate отличается от error ratio?**

- Error rate — абсолютная скорость ошибок, например 5xx в секунду.
- Error ratio — доля ошибок среди eligible requests.
- Совместное чтение — rate показывает число затронутых событий, ratio — тяжесть
  относительно объёма трафика.

**2. Все ли 4xx являются нормой, а 5xx — ошибкой?**

- Протокол — status class полезен как первый diagnostic breakdown.
- Семантика — `429`, неожиданный `401` или `404` могут нарушать пользовательский
  SLO, а expected `404` может быть допустимым результатом.
- Контракт — SLI строят по eligible/successful outcomes конкретной операции.

**3. Что делать с error ratio при нулевом трафике?**

- Математика — `0 / 0` даёт `NaN`, потому что наблюдений о качестве нет.
- Dashboard — можно показывать no data и отдельно volume.
- Alert — использовать minimum-volume guard, longer window или synthetic probe
  в зависимости от критичности route.

**4. Что такое burn rate?**

- Определение — наблюдаемый error ratio, делённый на допустимый error budget
  ratio.
- Пример — при SLO `99.9%` бюджет равен `0.1%`; error ratio `1%` даёт burn rate
  `10`.
- Применение — multi-window burn-rate alerts связывают скорость деградации с SLO,
  а не с произвольным порогом ошибок.
