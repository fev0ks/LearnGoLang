# Latency histograms: среднее, квантили и границы SLO

## Содержание

- [Что хранит histogram](#что-хранит-histogram)
- [Как читать cumulative buckets](#как-читать-cumulative-buckets)
- [Среднее, p50, p95 и p99](#среднее-p50-p95-и-p99)
- [Как агрегировать реплики](#как-агрегировать-реплики)
- [Как выбирать buckets для classic histogram](#как-выбирать-buckets-для-classic-histogram)
- [Native histograms](#native-histograms)
- [SLO по bucket boundary](#slo-по-bucket-boundary)
- [Как читать форму деградации](#как-читать-форму-деградации)
- [Полезные panels](#полезные-panels)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Latency — распределение, а не одно число. Среднее, p50, p95 и p99 являются
разными проекциями одной нагрузки и отвечают на разные вопросы. Histogram
сохраняет достаточно информации, чтобы выбирать окно и агрегировать реплики во
время PromQL-запроса.

---

## Что хранит histogram

Подходящие наблюдения:

- длительность HTTP-запроса;
- ожидание connection pool;
- время обработки сообщения;
- размер payload;
- длительность client call к Postgres, Redis или downstream API.

Единица времени по Prometheus naming convention — seconds:

```text
shortener_http_request_duration_seconds
shortener_postgres_operation_duration_seconds
shortener_db_connection_wait_duration_seconds
```

Для каждого завершённого действия код вызывает `Observe(duration.Seconds())`.
Важно определить границы: включает ли HTTP duration запись response body,
включает ли duration обращения к хранилищу ожидание pool connection и считаются
ли отменённые операции. Без этого два одинаковых имени могут измерять разные
paths.

---

## Как читать cumulative buckets

Classic histogram с buckets `0.1`, `0.3`, `1.0`, `+Inf` публикует:

```text
duration_seconds_bucket{le="0.1"}    82
duration_seconds_bucket{le="0.3"}    97
duration_seconds_bucket{le="1"}     100
duration_seconds_bucket{le="+Inf"}  100
duration_seconds_sum                 18.4
duration_seconds_count               100
```

Каждый bucket включает все наблюдения не больше своей границы:

```text
<= 0.1s                 82
<= 0.3s                 97
<= 1.0s                100
<= +Inf                100
```

Чтобы получить непересекающиеся диапазоны, соседние counts вычитают:

```text
(0.0s, 0.1s]          = 82
(0.1s, 0.3s]          = 97 - 82 = 15
(0.3s, 1.0s]          = 100 - 97 = 3
```

Bucket `+Inf` обязателен и совпадает с `_count`. Raw buckets полезны для
проверки экспорта и heatmap, но line chart каждого cumulative bucket трудно
читать как latency.

Classic histogram создаёт на каждую исходную label combination:

```text
number of bucket series + _sum + _count
```

Поэтому дополнительные labels особенно дороги: они перемножаются ещё и с числом
buckets.

---

## Среднее, p50, p95 и p99

### Среднее

```promql
sum(rate(shortener_http_request_duration_seconds_sum[5m]))
/
sum(rate(shortener_http_request_duration_seconds_count[5m]))
```

Среднее использует все наблюдения, но может скрыть хвост. Например:

```text
99 requests × 0.05s = 4.95s
 1 request  × 5.00s = 5.00s
sum                    9.95s
average = 9.95s / 100 = 0.0995s
```

Среднее около `100 ms` выглядит приемлемо, хотя один пользователь ждал `5 s`.

### p50

Медиана: примерно половина наблюдений не больше этого значения. Она описывает
типичный путь, но почти не видит небольшой медленный хвост.

### p95

Примерно 95% наблюдений не больше значения p95, а 5% — больше. Часто это
удобный operational tail signal, но его выбор должен быть связан с SLO и
traffic volume.

### p99

Показывает более редкий хвост и требует больше наблюдений для устойчивости. При
100 requests в окне p99 определяется примерно одним самым медленным запросом и
будет шумным; при очень низком трафике один квантиль без count вводит в
заблуждение.

Percentile не говорит, насколько медленны запросы за границей. Одинаковый p99
может скрывать один timeout `2 s` или серию зависших calls `30 s`; heatmap,
max-like traces и timeout counters дополняют картину.

---

## Как агрегировать реплики

### Правильный global p95 classic histogram

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(shortener_http_request_duration_seconds_bucket[5m])
  )
)
```

Порядок:

1. `rate()` учитывает reset bucket counters каждой реплики.
2. `sum by (le)` объединяет counts одинаковых boundaries.
3. `histogram_quantile()` вычисляет квантиль общего распределения.

### p95 по route

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(shortener_http_request_duration_seconds_bucket[5m])
  )
)
```

### Неправильное усреднение percentiles

```text
pod-a: 1000 requests, p95 = 100ms
pod-b:   10 requests, p95 = 2s

(100ms + 2s) / 2 = 1.05s
```

`1.05s` не является global p95: pods имеют разный объём и разные распределения.
Даже weighted average готовых percentiles не восстанавливает потерянную форму
распределения.

Нельзя складывать histograms с несовпадающими classic bucket boundaries. Для
одной metric family все реплики должны использовать одинаковую конфигурацию
buckets, особенно во время rollout.

---

## Как выбирать buckets для classic histogram

Prometheus оценивает quantile интерполяцией внутри bucket. Он знает только count
между границами, но не точные значения. Широкий bucket даёт широкий диапазон
ошибки.

Если SLO равен `300 ms`, полезны границы вокруг него:

```go
[]float64{0.05, 0.1, 0.2, 0.3, 0.5, 1, 2}
```

Это пример, а не универсальный набор. Для API с ожидаемой latency `10 ms` он
слишком грубый, для batch operation на минуты — слишком маленький.

Процесс выбора:

1. Зафиксировать единицу и границы измеряемой операции.
2. Взять ожидаемый диапазон из load test или production observations.
3. Добавить точную boundary на SLO и несколько границ вокруг неё.
4. Покрыть timeouts и допустимый хвост, сохранив `+Inf`.
5. Оценить series cost: labels × replicas × (`buckets + 2`).
6. Проверить queries и dashboards на canary до широкого rollout.

Больше buckets повышает разрешение, но линейно увеличивает series classic
histogram. Слишком мало buckets делает p95 дешёвым, но неточным. Этот trade-off
нельзя решить одним default для всех workloads.

---

## Native histograms

Native histogram хранит динамические buckets в одном составном sample. Запрос
global p95:

```promql
histogram_quantile(
  0.95,
  sum(rate(shortener_http_request_duration_seconds[5m]))
)
```

p95 по route:

```promql
histogram_quantile(
  0.95,
  sum by (route) (
    rate(shortener_http_request_duration_seconds[5m])
  )
)
```

Преимущества:

- не нужно вручную задавать список фиксированных boundaries;
- обычно можно получить более высокое разрешение при меньшей цене;
- aggregation не требует label `le`;
- buckets передаются атомарно как часть одного sample.

Ограничения:

- client library, Prometheus и remote path должны поддерживать native samples;
- в Prometheus 3.8+ feature стабильна, но scrape нужно явно включить через
  `scrape_native_histograms: true`;
- разрешение и limits всё равно влияют на объём хранения и CPU;
- миграция должна проверить dashboards, alerts и долгосрочное хранилище.

При несовместимой цепочке classic histogram остаётся корректным выбором. Summary
не является автоматической заменой: его готовые quantiles нельзя агрегировать
между репликами.

---

## SLO по bucket boundary

Если SLO формулируется как «95% eligible requests быстрее 300 ms», classic
histogram с boundary `0.3` позволяет считать прямой ratio:

```promql
sum(
  rate(shortener_http_request_duration_seconds_bucket{le="0.3"}[5m])
)
/
sum(
  rate(shortener_http_request_duration_seconds_count[5m])
)
```

В числителе количество запросов не дольше `0.3 s`, в знаменателе все
наблюдения. Результат `0.97` означает `97%`.

Проверка через p95:

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(shortener_http_request_duration_seconds_bucket[5m])
  )
) <= 0.3
```

менее точна около boundary, потому что quantile интерполируется. Прямой bucket
ratio совпадает с формулировкой SLO и использует фактический count.

Для Apdex или другого score можно использовать несколько boundaries, но сначала
нужно явно записать категории и проверить, что cumulative buckets не
посчитаны дважды.

---

## Как читать форму деградации

### Растут p95/p99, p50 стабилен

Гипотезы:

- contention только части запросов;
- cache miss path;
- одна медленная partition или shard;
- occasional GC pause;
- retry/timeout одного downstream;
- один pod или AZ деградирует.

### Растут p50 и p95

Гипотезы:

- системное замедление dependency;
- CPU saturation;
- общий pool wait;
- новый код добавил работу ко всем requests;
- изменение route mix в сторону тяжёлых операций.

### Global p95 стабилен, один route деградирует

Большой быстрый route доминирует в global distribution. Нужны route-level
panels или SLO по критичной операции.

### p99 растёт, count очень мал

Quantile статистически нестабилен. Смотрят request count за окно, отдельные
traces и число timeouts. Не следует увеличивать окно бесконечно: длинное окно
может смешать до- и после-инцидентное поведение.

### Average растёт, p95 почти нет

Возможны экстремальные значения выше p95, изменение результата/route mix или
ошибка в запросе. Проверяют p99, heatmap, `_sum/_count` и label aggregation.

---

## Полезные panels

Service latency view:

- p50/p95/p99 или p50+p95 с count рядом;
- SLO success ratio по точной boundary;
- heatmap распределения, если нужна форма;
- p95 по ключевым routes;
- p95 по pod как drill-down;
- error ratio и RPS на том же time range;
- dependency/client latency и pool wait.

Не нужно выводить десятки percentiles только потому, что функция это позволяет.
Каждая линия должна отвечать на отдельный вопрос.

Panel unit должна быть seconds или автоматически масштабируемая duration. Если
PromQL возвращает `0.3`, а Grafana подписывает `ms`, зритель прочитает 0.3 ms
вместо 300 ms.

---

## Типичные ошибки

1. Raw `_bucket` lines показываются как готовая latency.
2. Percentiles реплик усредняются вместо объединения buckets.
3. `sum()` выполняется до `rate()`, скрывая resets отдельных counters.
4. Label `le` удаляется до `histogram_quantile()` classic histogram.
5. Все сервисы используют одни default buckets без связи с SLO.
6. Boundary SLO отсутствует, но квантиль сравнивают с ней как с точным числом.
7. p99 показывается без count при редком трафике.
8. Среднее используется как единственный tail signal.
9. Во время rollout реплики экспортируют разные bucket layouts одной family.
10. Native histogram включён в приложении, но не принят или не передан дальше
    observability backend.

---

## Interview-ready answer

**1. Как classic histogram хранит latency?**

- Buckets — cumulative counters по верхним границам `le`.
- Дополнения — `_count` хранит число наблюдений, `_sum` — их сумму.
- Запрос — `rate` превращает counter growth в распределение за окно, а
  `histogram_quantile` оценивает квантиль.

**2. Как получить global p95 нескольких replicas?**

- Шаг 1 — применить `rate()` к bucket counters каждой реплики.
- Шаг 2 — сложить их через `sum by (le)` или `sum by (route, le)`.
- Шаг 3 — вызвать `histogram_quantile(0.95, ...)`.
- Запрет — готовые p95 replicas нельзя усреднять.

**3. Почему buckets выбирают рядом с SLO?**

- Ограничение — Prometheus знает только count внутри bucket и интерполирует
  quantile.
- Точность — широкая граница вокруг SLO даёт большую неопределённость.
- Цена — каждый дополнительный classic bucket создаёт series для каждой label
  combination и реплики.

**4. Чем native histogram отличается от classic?**

- Представление — native histogram хранит динамические buckets в одном
  составном sample, classic — в наборе `_bucket/_sum/_count` series.
- Запрос — native aggregation не требует `le` и `_bucket`.
- Trade-off — native обычно эффективнее и детальнее, но требует совместимости и
  явного включения по всей цепочке.

**5. Почему SLO ratio по bucket может быть лучше проверки p95?**

- Соответствие — bucket count прямо считает долю наблюдений быстрее заданной
  boundary.
- Точность — p95 является интерполированной оценкой внутри bucket.
- Условие — нужная SLO boundary должна быть явно настроена в classic histogram.
