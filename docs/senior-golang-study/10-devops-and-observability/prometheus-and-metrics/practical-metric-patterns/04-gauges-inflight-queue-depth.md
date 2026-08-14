# Gauges: in-flight, queue depth и текущее состояние

## Содержание

- [Что измеряет Gauge](#что-измеряет-gauge)
- [Владелец состояния определяет агрегацию](#владелец-состояния-определяет-агрегацию)
- [In-flight и закон Литтла](#in-flight-и-закон-литтла)
- [Queue depth, lag и age](#queue-depth-lag-и-age)
- [Connection pool и capacity](#connection-pool-и-capacity)
- [Функции PromQL для gauges](#функции-promql-для-gauges)
- [Как читать рост gauge](#как-читать-рост-gauge)
- [Полезные panels и alerts](#полезные-panels-и-alerts)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Gauge показывает состояние в момент наблюдения: активную работу, глубину
очереди, занятые соединения или timestamp последнего успеха. Его raw value уже
имеет смысл, но правильная агрегация зависит от того, кому принадлежит
состояние.

---

## Что измеряет Gauge

Gauge может произвольно расти и уменьшаться:

```text
shortener_http_in_flight_requests
shortener_worker_queue_depth
shortener_worker_oldest_message_age_seconds
shortener_db_connections_in_use
shortener_batch_last_success_timestamp_seconds
process_resident_memory_bytes
```

Counter отвечает «сколько событий накопилось», gauge — «каково значение
сейчас»:

```text
requests_total:  100 -> 120 -> 145 -> reset -> 3
in_flight:         4 ->  19 ->   7 ->         2
```

К gauge не применяют `rate()` как к counter. Уменьшение состояния штатно и не
является reset.

---

## Владелец состояния определяет агрегацию

Перед `sum`, `max` или `avg` нужно ответить, что именно экспортирует каждая
реплика.

### Локальное независимое состояние

Каждый pod экспортирует своё `in_flight`. Общий параллелизм аддитивен:

```promql
sum(shortener_http_in_flight_requests{job="shortener"})
```

Если pods показывают `4`, `7` и `3`, сервис выполняет:

```text
4 + 7 + 3 = 14 requests
```

### Одна общая внешняя величина

Три consumers читают depth одной broker queue и каждый экспортирует `100`.
Сумма `300` неверна: это три копии одного наблюдения. Временно можно взять:

```promql
max(shortener_worker_queue_depth{queue="links"})
```

Но лучший контракт — один authoritative exporter или partition-level series,
которые действительно можно суммировать.

### Разделённое состояние

Каждый consumer владеет своим partition lag. Тогда общий объём накопившейся
работы можно суммировать, а худшую пользовательскую свежесть лучше показывает
максимум:

```promql
sum by (consumer_group) (shortener_consumer_partition_lag)
```

```promql
max by (consumer_group) (shortener_consumer_partition_lag)
```

Обе панели корректны, но отвечают на разные вопросы: total backlog и worst
partition.

---

## In-flight и закон Литтла

In-flight — число операций, которые начались, но ещё не завершились. В
устойчивом состоянии его порядок можно проверить законом Литтла:

```text
concurrency ≈ throughput × average duration
```

Если сервис обслуживает `200 req/s`, а средняя длительность `0.05 s`:

```text
200 req/s × 0.05 s = 10 requests
```

Ожидаемый средний in-flight близок к `10`, если границы измерений совпадают и
система находится примерно в steady state.

Если duration выросла до `0.5 s` при том же throughput:

```text
200 req/s × 0.5 s = 100 requests
```

Рост in-flight тогда является следствием более долгой жизни запросов. Если
in-flight растёт, throughput падает и latency увеличивается, это сильный сигнал
saturation или зависшего downstream.

Закон Литтла — проверка порядка величины, а не точное равенство каждого sample:
rate и duration усредняются по окну, arrivals могут быть bursty, а retries и
streaming requests меняют границы системы.

---

## Queue depth, lag и age

Одна queue depth не показывает, сколько времени ждут задачи.

### Depth

```promql
max(shortener_worker_queue_depth{queue="links"})
```

Описывает число ожидающих items. Большая batch queue может иметь высокий depth,
но нормальную свежесть при высокой скорости обработки.

### Arrival и processing rates

```promql
sum(rate(shortener_worker_events_total{phase="received"}[5m]))
```

```promql
sum(rate(shortener_worker_events_total{phase="completed"}[5m]))
```

Если arrival устойчиво выше completion, backlog растёт. Например:

```text
arrival = 120 events/s
completion = 100 events/s
net growth = 120 - 100 = 20 events/s
```

За десять минут ожидаемое накопление:

```text
20 events/s × 10 × 60 s = 12 000 events
```

Расчёт предполагает устойчивые rates и отсутствие других входов/выходов. Его
сверяют с реальным изменением depth.

### Age старейшей задачи

```promql
max(shortener_worker_oldest_message_age_seconds{queue="links"})
```

Age прямо связывается с freshness SLO. Depth `10 000` может быть нормальным при
age `2 s`, а depth `20` — проблемой при age `30 min` из-за poison message или
зависшей partition.

### Broker lag

Lag измеряется в broker-specific единицах: offsets/messages, bytes или time.
Название и Help должны явно показывать единицу. Нельзя сравнивать
`consumer_lag` разных брокеров, не определив его смысл.

---

## Connection pool и capacity

Для пула полезны несколько связанных signals:

```text
shortener_db_connections_in_use
shortener_db_connections_idle
shortener_db_connections_max
shortener_db_connection_waiters
shortener_db_connection_wait_duration_seconds
shortener_db_connection_timeouts_total
```

Utilization:

```promql
sum(shortener_db_connections_in_use)
/
sum(shortener_db_connections_max)
```

Ratio около `1` ещё не доказывает incident. При высокой utilization и нулевом
wait time pool может справляться. Saturation подтверждают waiters, wait duration
и timeouts.

Суммирование capacity по pods корректно для service-level client pools, но не
показывает лимит самой database. Например, 20 pods × pool max 50 создают
потенциал:

```text
20 × 50 = 1000 connections
```

Если Postgres допускает 500 соединений приложений, локально безопасный pool
config глобально опасен. Метрики приложения сопоставляют с server-side capacity
и rollout surge.

---

## Функции PromQL для gauges

### Текущее значение

```promql
shortener_worker_queue_depth
```

### Максимум за окно

```promql
max_over_time(shortener_worker_queue_depth[15m])
```

### Среднее за окно

```promql
avg_over_time(shortener_http_in_flight_requests[5m])
```

`avg_over_time` усредняет samples, а не точный непрерывный сигнал. При регулярном
scrape это хорошая оценка; при больших пропусках каждый имеющийся sample всё
равно получает одинаковый вес.

### Изменение за окно

```promql
delta(shortener_worker_queue_depth[15m])
```

### Линейный тренд

```promql
deriv(shortener_worker_queue_depth[30m])
```

### Время с последнего успеха

```promql
time() - shortener_batch_last_success_timestamp_seconds
```

Для service-level `*_over_time` сначала нужно решить порядок агрегации. Например,
максимум общего in-flight за окно:

```promql
max_over_time(
  (sum(shortener_http_in_flight_requests))[15m:]
)
```

Subquery сначала вычисляет service sum на каждом шаге, затем берёт временной
максимум. `sum(max_over_time(per_pod[15m]))` сложил бы maxima pods, которые могли
произойти в разные моменты, и завысил бы реальный одновременный пик.

---

## Как читать рост gauge

| Наблюдение | Возможная гипотеза | Что проверить рядом |
| --- | --- | --- |
| In-flight растёт, RPS стабилен | Увеличилась duration | p50/p95, downstream latency |
| In-flight растёт, RPS падает | Saturation или зависшие запросы | CPU, pool wait, timeouts |
| Queue depth растёт линейно | Arrival выше processing | Оба rates, worker capacity |
| Depth стабилен, oldest age растёт | Зависла partition/item | Per-partition lag, errors |
| Pool in-use у max | Возможна saturation | Waiters, wait duration, DB limit |
| Gauge памяти растёт ступенями | Удержание, cache или утечка | GC, heap profile, нагрузка |

Gauge создаёт гипотезу, а не готовую причину. Correlation с rate, errors и
duration превращает её в operational diagnosis.

---

## Полезные panels и alerts

Для worker:

- queue depth и configured capacity;
- oldest message age против freshness SLO;
- arrival и completion rates на одной шкале;
- in-flight workers и concurrency limit;
- processing p95, retries и DLQ.

Для HTTP API:

- service sum in-flight;
- in-flight по pod для imbalance;
- RPS и latency рядом;
- pool wait и dependency latency.

Alert на queue depth полезен только при известной capacity semantics. Alert на
age часто ближе к пользовательскому impact:

```promql
max(shortener_worker_oldest_message_age_seconds{queue="links"}) > 300
```

Порог `300 seconds` должен следовать из freshness SLO. Для очереди, где задача
может законно ждать час, он неверен.

---

## Типичные ошибки

1. Одинаковый внешний queue depth суммируется по replicas.
2. Partition lag берётся только через sum и скрывает одну зависшую partition.
3. In-flight растёт, но его не сопоставляют с throughput и duration.
4. Pool utilization трактуется как saturation без waiters и wait duration.
5. К gauge применяют `rate()` как к counter.
6. `sum(max_over_time(per_pod))` называют общим одновременным максимумом.
7. Queue depth alert не учитывает processing rate и age.
8. Timestamp последнего успеха заменяют обновляемым «временем с последнего
   успеха», которое замирает вместе с зависшим updater.

---

## Interview-ready answer

**1. Когда Gauge нужно суммировать, а когда брать max?**

- Локальное состояние — независимые in-flight или pool connections реплик можно
  суммировать для service total.
- Общая копия — один внешний queue depth, увиденный всеми репликами, суммировать
  нельзя; используют authoritative source или `max` как временную меру.
- Разделённое состояние — partition lag полезно смотреть и через `sum`, и через
  `max`, потому что это total backlog и worst partition.

**2. Как связаны RPS, latency и in-flight?**

- Модель — в steady state закон Литтла даёт `concurrency ≈ throughput × average
  duration`.
- Следствие — при стабильном RPS рост duration увеличивает in-flight.
- Ограничение — bursty traffic, retries и несовпавшие границы измерений делают
  равенство приближённым.

**3. Почему queue depth недостаточно?**

- Объём — depth показывает число ожидающих items.
- Скорость — arrival и completion rates объясняют, растёт ли backlog.
- Свежесть — age старейшей задачи показывает пользовательский impact.
- Локализация — per-partition lag обнаруживает зависшую часть очереди.

**4. Как увидеть saturation connection pool?**

- Utilization — in-use приближается к max.
- Saturation — появляются waiters и растёт wait duration.
- Errors — увеличиваются acquisition timeouts.
- Capacity — service sum pool limits сопоставляется с server-side connection
  budget.
