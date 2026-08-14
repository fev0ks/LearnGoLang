# PromQL: практическая шпаргалка

## Содержание

- [Ментальная модель запроса](#ментальная-модель-запроса)
- [Типы выражений](#типы-выражений)
- [Selectors и matchers](#selectors-и-matchers)
- [Counter: rate, increase и irate](#counter-rate-increase-и-irate)
- [Gauge и функции по времени](#gauge-и-функции-по-времени)
- [Агрегация: by и without](#агрегация-by-и-without)
- [Binary operators и vector matching](#binary-operators-и-vector-matching)
- [Classic и native histograms](#classic-и-native-histograms)
- [Рабочие запросы для HTTP](#рабочие-запросы-для-http)
- [Рабочие запросы для workers и хранилища](#рабочие-запросы-для-workers-и-хранилища)
- [Отсутствующие данные и нулевой трафик](#отсутствующие-данные-и-нулевой-трафик)
- [Offset, @ и сравнение периодов](#offset--и-сравнение-периодов)
- [Topk и ranking](#topk-и-ranking)
- [Как выбрать окно](#как-выбрать-окно)
- [Recording rules](#recording-rules)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

PromQL удобнее читать как pipeline над временными рядами:

```text
выбрать series -> взять samples за окно -> вычислить изменение
-> агрегировать labels -> получить ratio/quantile -> сравнить с условием
```

Порядок операций влияет на корректность. Для counter сначала вызывают `rate()`
на каждом исходном ряду и только потом суммируют, иначе reset отдельной реплики
может потеряться внутри общей суммы.

---

## Ментальная модель запроса

Пример сервисного RPS:

```promql
sum by (route) (
  rate(shortener_http_requests_total{job="shortener"}[5m])
)
```

Его можно разобрать слоями:

1. `shortener_http_requests_total{job="shortener"}` выбирает ряды.
2. `[5m]` берёт samples каждого ряда за пять минут.
3. `rate()` вычисляет средний рост в секунду и учитывает counter resets.
4. `sum by (route)` складывает реплики и коды ответа, сохраняя `route`.

Проверять выражение лучше в том же порядке: selector, затем функция, затем
aggregation. Сложный запрос, написанный сразу целиком, скрывает, на каком этапе
исчезли данные или labels.

---

## Типы выражений

### Instant vector

Набор временных рядов с одним sample на момент вычисления:

```promql
up{job="shortener"}
```

Это не обязательно «последний sample прямо сейчас». Prometheus выбирает самый
свежий допустимый sample не позже момента вычисления с учётом lookback и
staleness.

### Range vector

Набор рядов с диапазоном samples для каждого:

```promql
shortener_http_requests_total[5m]
```

Range vector обычно является аргументом функции `rate`, `increase`,
`max_over_time` и других функций по окну.

### Scalar

Обычное число:

```promql
0.01
```

### String

PromQL имеет строковый тип, но он почти не используется как результат
практических monitoring queries.

Native histogram sample отличается от обычного float sample: он содержит
count, sum и buckets как составное значение. Из-за этого не каждая функция или
binary operation имеет одинаковый смысл для обоих видов samples.

---

## Selectors и matchers

### Точное совпадение

```promql
shortener_http_requests_total{job="shortener",method="GET"}
```

### Неравенство

```promql
shortener_http_requests_total{status_code!="200"}
```

Matcher `!=` также совпадает с рядами, где label отсутствует. Если отсутствие
label нужно отличить, контракт метрики лучше сделать явным, а не полагаться на
сложные отрицательные filters.

### Регулярное выражение

```promql
shortener_http_requests_total{status_code=~"5.."}
```

Prometheus использует RE2, а regex matcher полностью anchored. Выражение
`5..` уже означает совпадение всей строки из трёх символов.

### Отрицательное регулярное выражение

```promql
shortener_http_requests_total{route!~"/health/(live|ready)"}
```

Фильтрация служебных routes в каждом dashboard хрупка. Если probes не являются
пользовательским трафиком, лучше единообразно исключить их в инструментации или
создать recording rule с зафиксированным контрактом.

### Пустое значение

```promql
some_metric{zone=""}
```

Такой matcher совпадает и с пустым `zone`, и с рядом без label `zone`. Эту
семантику важно помнить при миграции схемы labels.

---

## Counter: rate, increase и irate

### `rate()`

Средняя скорость роста в секунду за окно:

```promql
rate(shortener_http_requests_total[5m])
```

`rate()`:

- предназначена для counters и counter histograms;
- учитывает resets;
- экстраполирует результат к границам окна;
- подходит для dashboards, recording rules и alerts.

Правильный порядок:

```promql
sum by (route) (
  rate(shortener_http_requests_total[5m])
)
```

Опасный порядок:

```promql
rate(
  sum by (route) (shortener_http_requests_total)[5m:]
)
```

Во втором варианте counter разных реплик сначала объединён. Reset одной реплики
может быть скрыт ростом другой, и `rate()` больше не видит исходные resets.

### `increase()`

Оценка роста counter за окно:

```promql
sum(increase(shortener_worker_retries_total[15m]))
```

`increase(v[15m])` концептуально равно `rate(v[15m]) * 15 минут`. Из-за
экстраполяции к границам окна результат может быть дробным даже для счётчика
целых событий.

Для recording rules обычно сохраняют per-second `rate()`, а `increase()`
вычисляют при чтении под нужное окно. Тогда один записанный ряд остаётся полезен
для разных диапазонов.

### `irate()`

Мгновенная скорость по двум последним samples:

```promql
irate(process_cpu_seconds_total[5m])
```

Она быстрее реагирует и сильнее шумит. `irate()` полезна для исследования
резких движений высокочастотного counter, но для alerts обычно выбирают
`rate()`: устойчивое среднее меньше зависит от одного scrape.

---

## Gauge и функции по времени

Текущее состояние:

```promql
shortener_worker_queue_depth
```

Максимум за окно:

```promql
max_over_time(shortener_worker_queue_depth[15m])
```

Среднее за окно:

```promql
avg_over_time(shortener_http_in_flight_requests[5m])
```

Изменение между началом и концом окна:

```promql
delta(shortener_worker_queue_depth[15m])
```

Линейный тренд gauge в единицах в секунду:

```promql
deriv(shortener_worker_queue_depth[30m])
```

`delta()` и `deriv()` не превращают queue depth в надёжный прогноз. Они лишь
описывают выбранное окно; периодические задачи и сезонность могут менять знак
тренда без инцидента.

Время с последнего события лучше хранить как timestamp gauge:

```promql
time() - shortener_batch_last_success_timestamp_seconds
```

Приложение обновляет timestamp только при успехе. Если оно экспортировало бы
«секунд с последнего успеха», зависший код перестал бы обновлять gauge и мог
замаскировать проблему.

---

## Агрегация: by и without

### Удалить все labels

```promql
sum(rate(shortener_http_requests_total[5m]))
```

### Сохранить только выбранные labels

```promql
sum by (route, status_code) (
  rate(shortener_http_requests_total[5m])
)
```

### Удалить выбранные labels и сохранить остальные

```promql
sum without (instance, pod) (
  rate(shortener_http_requests_total[5m])
)
```

`by` задаёт стабильную схему результата. `without` удобен в ad-hoc анализе, но
новый label исходной метрики автоматически появится в результате. Для dashboard
и recording rule это может неожиданно размножить ряды.

Другие агрегаторы:

```promql
max by (namespace) (shortener_worker_queue_depth)
```

```promql
min by (namespace) (up{job="shortener"})
```

```promql
avg by (version) (process_resident_memory_bytes)
```

```promql
count by (job) (up)
```

Агрегатор должен соответствовать владельцу величины. Например, `sum` локальных
in-flight даёт общий параллелизм, но `sum` одинакового внешнего queue depth,
экспортированного каждой репликой, умножает значение на число targets.

---

## Binary operators и vector matching

Binary operation между vectors сопоставляет ряды по labels. Если labels слева и
справа не совпадают, элементы могут исчезнуть из результата.

### Ratio с одинаковыми labels

```promql
sum by (route) (
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
/
sum by (route) (
  rate(shortener_http_requests_total[5m])
)
```

Обе стороны имеют только `route`, поэтому сопоставление выполняется напрямую.

### Игнорировать лишний label

```promql
queue_depth{source="broker"}
/
ignoring (source)
queue_capacity
```

Такое выражение корректно только если после игнорирования `source` остаётся
однозначное соответствие по остальным labels. Если слева несколько sources на
один ряд справа, нужен group modifier, но для ratios обычно проще сначала
агрегировать обе стороны до одинаковой схемы labels.

### `on(...)`

Явно перечисляет labels для matching:

```promql
queue_depth
/
on (queue)
queue_capacity
```

Перед делением нужно подтвердить, что на каждую `queue` есть ровно один depth и
одна capacity. `group_left`/`group_right` разрешают many-to-one matching, но их
не следует добавлять только для подавления ошибки: сначала нужно понять
кардинальность обеих сторон.

### Comparison operators

Без модификатора `bool` сравнение фильтрует vector:

```promql
shortener_worker_queue_depth > 1000
```

Остаются только ряды, где условие истинно, с исходными значениями depth.

С `bool` результат равен `0` или `1`:

```promql
shortener_worker_queue_depth > bool 1000
```

В alert expression обычно нужен фильтрующий вариант, а `bool` полезен для
дальнейшей арифметики или явного бинарного сигнала.

---

## Classic и native histograms

### p95 classic histogram

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(shortener_http_request_duration_seconds_bucket[5m])
  )
)
```

Label `le` обязателен: он задаёт верхнюю границу bucket. Без него
`histogram_quantile()` не может восстановить classic histogram.

### p95 classic histogram по route

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(shortener_http_request_duration_seconds_bucket[5m])
  )
)
```

### p95 native histogram

```promql
histogram_quantile(
  0.95,
  sum(rate(shortener_http_request_duration_seconds[5m]))
)
```

Native histogram является одним составным sample, поэтому `_bucket` и `le` в
запросе отсутствуют.

### p95 native histogram по route

```promql
histogram_quantile(
  0.95,
  sum by (route) (
    rate(shortener_http_request_duration_seconds[5m])
  )
)
```

### Среднее classic histogram

```promql
sum(rate(shortener_http_request_duration_seconds_sum[5m]))
/
sum(rate(shortener_http_request_duration_seconds_count[5m]))
```

### Среднее native histogram

```promql
histogram_avg(
  sum(rate(shortener_http_request_duration_seconds[5m]))
)
```

### Доля запросов быстрее SLO для classic histogram

```promql
sum(
  rate(shortener_http_request_duration_seconds_bucket{le="0.3"}[5m])
)
/
sum(
  rate(shortener_http_request_duration_seconds_count[5m])
)
```

Если `0.3` является точной bucket boundary, этот ratio точнее проверки
`p95 <= 0.3`: quantile интерполируется внутри bucket, а SLO ratio использует
непосредственный count на границе.

Всегда выполнять `rate()` до `sum()` нужно и для buckets. Это сохраняет
способность обнаружить reset каждой реплики.

---

## Рабочие запросы для HTTP

### Общий RPS

```promql
sum(rate(shortener_http_requests_total[5m]))
```

### RPS по route

```promql
sum by (route) (
  rate(shortener_http_requests_total[5m])
)
```

### 5xx в секунду

```promql
sum(
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
```

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

При полном отсутствии трафика получается `0 / 0 = NaN`. Для alert условие
ratio сочетают с минимальным объёмом трафика:

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

Порог `1 req/s` здесь только пример. Реальный minimum traffic выводят из
чувствительности SLO и допустимого количества событий, а не копируют между
сервисами.

### Доля 5xx по route

```promql
sum by (route) (
  rate(shortener_http_requests_total{status_code=~"5.."}[5m])
)
/
sum by (route) (
  rate(shortener_http_requests_total[5m])
)
```

### Активные запросы по сервису

```promql
sum(shortener_http_in_flight_requests)
```

### Ряды, появившиеся после deploy

```promql
sum by (version) (
  rate(shortener_http_requests_total[5m])
)
```

Label `version` должен иметь ограниченное число одновременно живых значений и
удаляться вместе со старыми targets; commit SHA на каждом историческом ряду
увеличивает churn, хотя одновременно активная cardinality может быть небольшой.

---

## Рабочие запросы для workers и хранилища

### Скорость обработки по результату

```promql
sum by (result) (
  rate(shortener_worker_events_total{phase="process"}[5m])
)
```

### Число DLQ-событий за час

```promql
sum(increase(shortener_worker_dlq_messages_total[1h]))
```

### Queue depth и возраст старейшего сообщения

```promql
max(shortener_worker_queue_depth)
```

```promql
max(shortener_worker_oldest_message_age_seconds)
```

`max` предполагает, что реплики наблюдают одну внешнюю очередь. Для локальных
partition-specific depths корректнее `sum by (consumer_group)` или другой
контракт владельца.

### Операции Postgres по operation и result

```promql
sum by (operation, result) (
  rate(shortener_postgres_operations_total[5m])
)
```

### p95 Postgres latency по operation

```promql
histogram_quantile(
  0.95,
  sum by (operation, le) (
    rate(shortener_postgres_operation_duration_seconds_bucket[5m])
  )
)
```

### Cache hit ratio

```promql
sum(rate(shortener_cache_requests_total{result="hit"}[5m]))
/
sum(rate(shortener_cache_requests_total{result=~"hit|miss"}[5m]))
```

Ошибки cache исключены из знаменателя hit ratio, потому что он измеряет
эффективность найденных ответов, а не availability cache. Error ratio строят
отдельно по всем attempts. Это решение является частью metric contract и может
отличаться для другого продукта.

### Использование пула соединений

```promql
sum(shortener_db_connections{state="in_use"})
/
sum(shortener_db_connections{state="max"})
```

Этот запрос корректен только если обе серии аддитивны между репликами и label
`state` действительно отделяет используемую и настроенную capacity. Более
прозрачный контракт часто использует две metric families:
`db_connections_in_use` и `db_connections_max`.

---

## Отсутствующие данные и нулевой трафик

### Target отсутствует

```promql
absent(up{job="shortener"})
```

`absent()` возвращает элемент, когда входной vector пуст. Это отличается от
`up=0`: во втором случае target существует, но scrape не проходит.

### Ряд отсутствовал всё окно

```promql
absent_over_time(shortener_batch_last_success_timestamp_seconds[30m])
```

### Подставить ноль для отсутствующего ряда

Распространённая конструкция:

```promql
sum(rate(shortener_worker_errors_total[5m]))
or on() vector(0)
```

Она подходит только когда отсутствие ряда действительно означает ноль для
одного глобального результата. Для результата по `route` `vector(0)` не создаст
нужные labels, а исчезновение exporter может быть ошибочно замаскировано как
«ошибок нет».

Лучше заранее инициализировать ожидаемые label combinations нулём и отдельно
контролировать `up`/presence.

### Деление на ноль

`0 / 0` даёт `NaN`, а положительное число, делённое на ноль, — `+Inf`. Вместо
механического `clamp_min(denominator, 1)` нужно определить семантику низкого
трафика: скрыть panel, показать no data или добавить minimum-volume guard в
alert. Подмена знаменателя единицей искажает ratio.

---

## Offset, @ и сравнение периодов

### Значение со сдвигом

```promql
sum(rate(shortener_http_requests_total[5m] offset 1h))
```

### Текущее и час назад

```promql
sum(rate(shortener_http_requests_total[5m]))
-
sum(rate(shortener_http_requests_total[5m] offset 1h))
```

Сравнение полезно только при сопоставимой сезонности. Час назад может быть
другой traffic phase; для суточного цикла логичнее `offset 1d` и всё равно
нужно учитывать праздники, deploy и изменения продукта.

Модификатор `@` фиксирует время selector:

```promql
shortener_build_info @ 1754323200
```

Он полезен для воспроизводимого расследования и сравнения с границей диапазона,
но Unix timestamp в произвольном dashboard быстро устаревает.

---

## Topk и ranking

Пять routes с самым высоким RPS на момент вычисления:

```promql
topk(
  5,
  sum by (route) (
    rate(shortener_http_requests_total[5m])
  )
)
```

Самые маленькие текущие значения:

```promql
bottomk(5, shortener_worker_partition_lag)
```

На range graph `topk(5, ...)` вычисляется независимо в каждый шаг. Состав top-5
может меняться, поэтому за весь диапазон появится больше пяти линий. Для
фиксированного ranking сначала определяют список на одном времени или используют
table, а затем отдельно исследуют выбранные series.

---

## Как выбрать окно

Для `rate()` нужно как минимум два samples, но окно из двух точек крайне
чувствительно к пропущенному или задержанному scrape. Практическая начальная
эвристика — окно не меньше примерно четырёх scrape intervals.

При `scrape_interval: 15s`:

```text
4 × 15s = 60s
```

Окно `[1m]` является нижней отправной точкой, а `[5m]` даёт более сглаженный
operational signal. Выбор зависит от вопроса:

- короткое окно быстрее обнаруживает изменение, но сильнее шумит;
- длинное окно устойчивее, но медленнее показывает и скрывает короткий spike;
- редкие события лучше смотреть через `increase()` на более длинном окне;
- alert `for` не заменяет достаточное окно функции: это разные механизмы.

В Grafana используют `$__rate_interval`, если datasource его предоставляет. В
recording и alert rules окно задают явно и документируют рядом с целью правила.

---

## Recording rules

Тяжёлое или многократно используемое выражение можно вычислять заранее:

```yaml
groups:
  - name: shortener-http
    rules:
      - record: job_route:http_requests:rate5m
        expr: |
          sum by (job, route) (
            rate(shortener_http_requests_total[5m])
          )
```

Плюсы:

- dashboard читает меньше исходных series;
- одинаковая логика используется в panels и alerts;
- сложное выражение получает стабильное имя и reviewable contract.

Минусы:

- новый ряд занимает место в хранилище;
- фиксированное окно `5m` нельзя превратить в точный `rate1h`;
- ошибочная агрегация заранее удаляет labels, которые позже понадобятся;
- изменение правила требует миграции потребителей.

Recording rule не исправляет высокую ingestion cardinality исходной метрики:
дорогие series уже были собраны и сохранены.

---

## Типичные ошибки

1. Raw counter показывают как скорость.
2. `sum()` выполняют до `rate()` и теряют корректную обработку resets реплик.
3. К gauge применяют `rate()` вместо функций, соответствующих состоянию.
4. В classic histogram удаляют `le` до `histogram_quantile()`.
5. Готовые quantiles `Summary` суммируют или усредняют между instances.
6. Ratio строят из vectors с разной схемой labels, и часть рядов исчезает.
7. `clamp_min` используют для маскировки `0 / 0`, меняя смысл ratio.
8. `or vector(0)` скрывает отсутствие target как здоровый ноль.
9. Окно `rate()` почти равно scrape interval и не переживает пропущенный scrape.
10. `topk` на range graph ожидают увидеть как ровно N фиксированных series.
11. `sum without (...)` в recording rule случайно сохраняет новый label.
12. Дорогой query превращают в recording rule, не исправив исходную
    cardinality.

---

## Interview-ready answer

**1. Почему нужно делать `rate()` до `sum()`?**

- Суть — `rate()` должен увидеть samples каждого исходного counter.
- Reset — тогда функция отдельно учитывает перезапуск каждой реплики.
- Ошибка — после предварительного `sum()` рост других реплик может скрыть reset
  одной из них.

**2. Чем `rate()` отличается от `increase()`?**

- `rate()` — средняя скорость роста counter в секунду.
- `increase()` — экстраполированная оценка роста за указанное окно.
- Связь — `increase` концептуально равен `rate`, умноженному на длительность
  окна; результат может быть дробным.
- Применение — recording rules обычно сохраняют `rate`, а `increase` удобен для
  вопроса «сколько событий за период».

**3. Как вычислить p95 classic histogram по route?**

- Шаг 1 — применить `rate()` к `_bucket` каждого ряда.
- Шаг 2 — сложить реплики через `sum by (route, le)`.
- Шаг 3 — вызвать `histogram_quantile(0.95, ...)`.
- Ограничение — результат интерполируется внутри bucket и зависит от выбранных
  границ.

**4. Почему PromQL ratio иногда пустой?**

- Matching — labels числителя и знаменателя могут не совпасть.
- Данные — одна сторона может отсутствовать или стать stale.
- Нулевой трафик — `0 / 0` даёт `NaN`.
- Диагностика — отдельно проверить обе стороны, затем привести их к одинаковой
  агрегации и определить семантику низкого трафика.

**5. Как выбрать окно `rate()`?**

- Нижняя граница — нужно больше одного scrape; начальная эвристика составляет
  около четырёх scrape intervals.
- Trade-off — короткое окно быстрее и шумнее, длинное устойчивее и медленнее.
- Контекст — окно выбирают по частоте событий, чувствительности alert и
  допустимой задержке обнаружения.
