# HTTP Request Rate Counters

Эта заметка про самый частый operational signal: сколько запросов реально идет в сервис.

## Содержание

- [Что это за метрика](#что-это-за-метрика)
- [Как это выглядит в Grafana](#как-это-выглядит-в-grafana)
- [Как это читать](#как-это-читать)
- [Что считать нормой](#что-считать-нормой)
- [Что считать плохим сигналом](#что-считать-плохим-сигналом)
- [Какие панели полезны](#какие-панели-полезны)
- [Что важно не перепутать](#что-важно-не-перепутать)
- [Practical Rule](#practical-rule)

## Что это за метрика

Обычно это `Counter`, например:

```text
http_requests_total{service="shortener",route="POST /api/v1/links",status_code="201"}
```

Сам по себе counter только растет.

Это значит:
- raw число почти ничего не говорит;
- важно не "сколько накопилось всего", а "с какой скоростью растет".

## Как это выглядит в Grafana

Чаще всего строят line chart:
- по оси `Y` — requests per second;
- по оси `X` — время.

Типичный query:

```promql
sum(rate(http_requests_total[5m]))
```

По route:

```promql
sum by (route) (
  rate(http_requests_total[5m])
)
```

## Как это читать

Если линия идет:
- ровно и стабильно — трафик стабилен;
- резко вверх — traffic spike;
- резко вниз — либо нагрузка упала, либо сервис перестал обслуживать запросы;
- в ноль — либо нет трафика, либо проблема с ingress/router/app.

## Что считать нормой

Норма не бывает универсальной.

Нормой считается:
- signal совпадает с обычным профилем сервиса;
- дневной/ночной ритм предсказуем;
- нет unexplained spikes или drops.

То есть `50 rps` может быть:
- прекрасно для одного сервиса;
- катастрофой для другого;
- слишком мало для третьего.

Нужно знать baseline.

## Что считать плохим сигналом

### Резкий рост

Может означать:
- всплеск реального трафика;
- retry storm;
- bad client behavior;
- loop в consumer/webhook system.

### Резкий спад

Может означать:
- сервис частично недоступен;
- traffic routing сломан;
- readiness/ingress issue;
- upstream перестал слать трафик.

## Какие панели полезны

### Общий throughput

```promql
sum(rate(http_requests_total[5m]))
```

Это панель “сколько трафика у сервиса вообще”.

### Throughput по route

```promql
sum by (route) (
  rate(http_requests_total[5m])
)
```

Это панель “какие endpoint-ы самые горячие”.

### Throughput по статусам

```promql
sum by (status_code) (
  rate(http_requests_total[5m])
)
```

Это панель “что происходит с ответами”.

## Что важно не перепутать

Плохо:

```promql
http_requests_total
```

Хорошо:

```promql
rate(http_requests_total[5m])
```

или

```promql
increase(http_requests_total[15m])
```

## Practical Rule

Если видишь панель по request counter:
- сначала спроси, это raw counter или `rate/increase`;
- потом смотри на baseline;
- потом проверяй, совпадает ли изменение с бизнес-событием или это unexplained anomaly.
