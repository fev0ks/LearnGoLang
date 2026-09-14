# Метрики Go runtime и процесса в production

## Содержание

- [Три уровня наблюдения](#три-уровня-наблюдения)
- [Как подключить стандартные collectors](#как-подключить-стандартные-collectors)
- [Минимальный набор метрик](#минимальный-набор-метрик)
- [Память: heap, RSS и container memory](#память-heap-rss-и-container-memory)
- [CPU процесса](#cpu-процесса)
- [Goroutines, OS threads и scheduler](#goroutines-os-threads-и-scheduler)
- [GC и скорость аллокаций](#gc-и-скорость-аллокаций)
- [Файловые дескрипторы и время жизни процесса](#файловые-дескрипторы-и-время-жизни-процесса)
- [Как собрать dashboard](#как-собрать-dashboard)
- [Диагностические сценарии](#диагностические-сценарии)
- [Alerts без ложной точности](#alerts-без-ложной-точности)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

HTTP-метрики показывают пользовательский симптом: изменились RPS, error ratio
или latency. Метрики Go runtime и процесса помогают найти ресурсную причину:
растёт ли heap, успевает ли GC, копятся ли goroutines, занят ли CPU и не
заканчиваются ли файловые дескрипторы.

Этот слой не заменяет [RED-метрики HTTP](./01-http-request-rate-counters.md) и
метрики контейнера. Один и тот же симптом нужно проверять на нескольких уровнях.

---

## Три уровня наблюдения

У Go-сервиса нет одной метрики «здоровье процесса». Наблюдение состоит из трёх
слоёв:

| Уровень | Примеры | На какой вопрос отвечает |
| --- | --- | --- |
| Приложение | `http_requests_total`, latency, errors, queue depth | Что видит пользователь и где копится работа? |
| Go runtime и процесс | heap, allocations, GC, goroutines, RSS, CPU, file descriptors | Как процесс тратит ресурсы? |
| Контейнер и узел | memory working set, CPU throttling, limits, OOM kills | Как процесс ограничен средой исполнения? |

Например, рост latency вместе с ростом `go_goroutines` ещё не доказывает
goroutine leak. Причиной может быть медленная база: запросы дольше живут, поэтому
по закону Литтла одновременно существует больше goroutines. Утечка становится
вероятнее, если после спада RPS и завершения запросов число goroutines не
возвращается к обычному уровню.

Практический порядок проверки:

```text
RED symptom
    -> RPS и in-flight
    -> CPU, heap, allocation rate и GC
    -> goroutines, scheduler и file descriptors
    -> container limit, throttling, restart или OOMKill
    -> profile, trace и logs для локализации причины
```

---

## Как подключить стандартные collectors

Пакет `github.com/prometheus/client_golang/prometheus` создаёт default registry
с уже зарегистрированными Go collector и process collector. Если приложение
использует этот registry, достаточно открыть endpoint:

```go
import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func registerMetricsEndpoint(mux *http.ServeMux) {
    mux.Handle("/metrics", promhttp.Handler())
}
```

`promhttp.Handler()` читает `prometheus.DefaultGatherer`. Повторная регистрация
тех же collectors в default registry приведёт к ошибке duplicate registration.

### Отдельный registry

`prometheus.NewRegistry()` создаёт пустой registry. В него стандартные
collectors нужно добавить явно:

```go
import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/collectors"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func registerMetricsEndpoint(mux *http.ServeMux) {
    registry := prometheus.NewRegistry()

    registry.MustRegister(
        collectors.NewGoCollector(),
        collectors.NewProcessCollector(
            collectors.ProcessCollectorOpts{},
        ),
    )

    mux.Handle(
        "/metrics",
        promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
    )
}
```

Наличие конкретных process-метрик зависит от операционной системы и версии
`client_golang`. Контракт проверяют на собранном бинарнике, а не по памяти:

```bash
curl -s http://localhost:8080/metrics \
  | grep -E '^(go_|process_)'
```

### Расширенные runtime-метрики

Go collector экспортирует стабильный базовый набор и совместимые
`go_memstats_*` метрики. Дополнительные семейства из стандартного пакета
`runtime/metrics` включаются явно. Например, метрики scheduler:

```go
registry.MustRegister(
    collectors.NewGoCollector(
        collectors.WithGoCollectorRuntimeMetrics(
            collectors.MetricsScheduler,
        ),
    ),
)
```

Не нужно одновременно регистрировать ещё один `NewGoCollector()`. Точный набор
и имена расширенных метрик зависят от версий Go и `client_golang`. Включать
`collectors.MetricsAll` без конкретного operational question не стоит:
гистограммы создают дополнительные bucket series, увеличивают размер scrape и
усложняют поддержку dashboards.

---

## Минимальный набор метрик

Для большинства Go API и workers полезен следующий стартовый набор:

| Метрика | Тип | Что показывает |
| --- | --- | --- |
| `process_resident_memory_bytes` | Gauge | RSS процесса |
| `process_cpu_seconds_total` | Counter | накопленное user + system CPU time |
| `go_goroutines` | Gauge | текущее число goroutines |
| `go_threads` | Gauge | число OS threads, сообщаемое runtime |
| `go_memstats_heap_alloc_bytes` | Gauge | память выделенных и ещё не освобождённых heap-объектов |
| `go_memstats_heap_inuse_bytes` | Gauge | память spans, выделенных heap |
| `go_memstats_alloc_bytes_total` | Counter | все выделенные heap bytes с запуска |
| `go_memstats_heap_objects` | Gauge | текущее число heap-объектов |
| `go_gc_duration_seconds` | Summary | stop-the-world паузы GC |
| `go_gc_gogc_percent` | Gauge | текущее значение `GOGC` |
| `go_gc_gomemlimit_bytes` | Gauge | soft memory limit Go runtime |
| `go_sched_gomaxprocs_threads` | Gauge | текущее значение `GOMAXPROCS` |
| `process_open_fds` | Gauge | открытые файловые дескрипторы |
| `process_max_fds` | Gauge | лимит файловых дескрипторов |
| `process_start_time_seconds` | Gauge | Unix timestamp запуска процесса |

Названия `go_memstats_*` поддерживаются `client_golang` как совместимый набор,
хотя внутри collector современных версий Go читает большую часть значений
через `runtime/metrics`. Не нужно одновременно экспортировать те же данные под
собственными именами.

`go_gc_gomemlimit_bytes` описывает настройку `GOMEMLIMIT`, а не memory limit
контейнера. Если soft limit runtime явно не настроен, значение может означать
практически неограниченный лимит. `GOMEMLIMIT` также не является жёстким cap на
RSS: он управляет памятью, известной Go runtime, но не всей памятью cgo, `mmap`
и других процессов контейнера.

---

## Память: heap, RSS и container memory

Слово «память» скрывает несколько разных величин:

```text
Go objects
    -> go_memstats_heap_alloc_bytes
    -> go_memstats_heap_inuse_bytes

Весь процесс в физической памяти
    -> process_resident_memory_bytes

Всё, что учитывает cgroup контейнера
    -> container_memory_working_set_bytes
    -> memory limit и OOM policy
```

Эти значения не обязаны совпадать.

### Основные Go memory metrics

| Метрика | Практический смысл |
| --- | --- |
| `go_memstats_heap_alloc_bytes` | bytes выделенных и ещё не освобождённых heap-объектов; между циклами GC сюда может входить ещё не собранный мусор |
| `go_memstats_heap_inuse_bytes` | spans, назначенные под heap-объекты; обычно не меньше `heap_alloc` |
| `go_memstats_heap_idle_bytes` | heap spans, которые сейчас не используются |
| `go_memstats_heap_released_bytes` | часть idle heap, возвращённая операционной системе |
| `go_memstats_stack_inuse_bytes` | память stack spans goroutines |
| `go_memstats_sys_bytes` | память, полученная или зарезервированная runtime у ОС; это не RSS |
| `go_memstats_next_gc_bytes` | целевой размер heap для следующего цикла GC |
| `go_memstats_heap_objects` | текущее число heap-объектов |

Текущий размер выделенных heap-объектов одного pod:

```promql
max by (namespace, pod) (
  go_memstats_heap_alloc_bytes{job="shortener"}
)
```

RSS одного pod:

```promql
max by (namespace, pod) (
  process_resident_memory_bytes{job="shortener"}
)
```

Суммарный RSS всех реплик:

```promql
sum(process_resident_memory_bytes{job="shortener"})
```

`sum()` отвечает на вопрос о полном footprint сервиса, но скрывает одну
аномальную реплику. Для расследования рядом нужен per-pod panel или `topk()`:

```promql
topk(5, process_resident_memory_bytes{job="shortener"})
```

### Почему heap уменьшается, а RSS остаётся высоким

GC освобождает недостижимые Go-объекты, но это не означает немедленный возврат
тех же страниц операционной системе. Runtime может оставить свободные spans для
следующих аллокаций. Кроме heap в RSS входят стеки, код, runtime metadata,
память cgo и memory mappings.

Полезная оценка idle heap, ещё не возвращённого ОС:

```promql
go_memstats_heap_idle_bytes
-
go_memstats_heap_released_bytes
```

Это не «утечка в байтах». Значение показывает пространство для гипотезы о
retained pages и фрагментации, которую проверяют вместе с RSS, heap profile и
нагрузкой.

### Container memory и memory limit

`process_resident_memory_bytes` описывает один процесс. Метрика
`container_memory_working_set_bytes` приходит из инфраструктурного collector и
описывает cgroup контейнера: туда могут попасть другие процессы и память,
которой нет в Go heap. Она не является точным синонимом RSS.

Если метрики kube-state-metrics имеют современные generic resource names,
долю от memory limit можно оценить так:

```promql
100 *
container_memory_working_set_bytes{
  namespace="prod",
  container="shortener"
}
/
on (namespace, pod, container)
kube_pod_container_resource_limits{
  namespace="prod",
  container="shortener",
  resource="memory",
  unit="byte"
}
```

Конкретные имена и лишние labels зависят от версии cAdvisor,
kube-state-metrics и конфигурации monitoring stack. Перед alert нужно проверить
реальные series и убедиться, что справа существует ровно один limit на
контейнер.

Для OOM-риска главным сигналом служит память cgroup относительно limit, а не
один `go_memstats_heap_alloc_bytes`. Подробная механика heap, RSS и `GOMEMLIMIT`
разобрана в [материале про stack и heap](../../../01-go-core/memory-internals/01-stack-and-heap.md).

---

## CPU процесса

`process_cpu_seconds_total` накапливает процессорное время процесса. Разность
counter за минуту показывает, сколько CPU seconds процесс израсходовал за эту
минуту. `rate()` нормализует результат к секунде:

```promql
rate(process_cpu_seconds_total{job="shortener"}[5m])
```

Результат измеряется в ядрах CPU:

```text
rate = 0.25  -> в среднем занята четверть одного ядра CPU
rate = 1.00  -> полностью занято одно ядро CPU
rate = 2.40  -> процесс использует в среднем 2.4 ядра CPU
```

Проценты одного core:

```promql
100 * rate(process_cpu_seconds_total{job="shortener"}[5m])
```

Поэтому значение `240%` означает примерно `2.4` ядра CPU, а не ошибку запроса.
Процент от лимита CPU контейнера вычисляют отдельно, разделив использование
контейнера на его лимит.

Загрузка относительно доступного Go-параллелизма:

```promql
100 *
rate(process_cpu_seconds_total{job="shortener"}[5m])
/
go_sched_gomaxprocs_threads{job="shortener"}
```

Это доля от текущего `GOMAXPROCS`, а не от cgroup CPU quota. Эти ограничения
могут совпадать, но имеют разные контракты и на старых версиях Go или при ручной
настройке легко расходятся.

Суммарное использование сервиса:

```promql
sum(
  rate(process_cpu_seconds_total{job="shortener"}[5m])
)
```

Высокий CPU не равен saturation. Процесс может использовать несколько ядер и
сохранять нормальную latency. Для вывода о насыщении нужны CPU limit,
throttling, очередь runnable goroutines и пользовательские RED-сигналы.

---

## Goroutines, OS threads и scheduler

Текущее число goroutines:

```promql
go_goroutines{job="shortener"}
```

Универсального порога «слишком много» нет. HTTP-сервис с долгими соединениями,
worker с фиксированным пулом и batch job имеют разные baseline. Важны три
наблюдения:

1. как число меняется вместе с RPS и `in_flight`;
2. возвращается ли оно к baseline после спада нагрузки;
3. повторяются ли в goroutine profile одинаковые зависшие stacks.

Для визуализации долгосрочного направления можно использовать линейный наклон:

```promql
deriv(go_goroutines{job="shortener"}[30m])
```

Результат измеряется в goroutines в секунду и шумит при нормальных всплесках.
Его нельзя превращать в универсальный alert без service-specific baseline и
минимальной длительности условия.

`go_threads` показывает число OS threads, сообщаемое Go runtime. Рост может
сопровождать блокирующие syscall, cgo или частое применение
`runtime.LockOSThread`, но по одной метрике причина не определяется.

Расширенный scheduler collector может добавить метрики runnable latency и
состояний goroutines. Они помогают отличить «goroutines ждут сеть» от
«goroutines готовы исполняться, но долго не получают CPU». Набор зависит от
версии Go; сначала включают только нужную группу и проверяют `/metrics`.

Переход от растущего графика к goroutine profile разобран в
[профилировании concurrency](../../../01-go-core/profiling/04-goroutine-concurrency-profiling.md).

---

## GC и скорость аллокаций

Размер heap и скорость аллокаций отвечают на разные вопросы:

- heap size показывает, сколько памяти удерживается сейчас;
- allocation rate показывает, сколько новых bytes создаётся в секунду, включая
  короткоживущие объекты;
- число GC cycles показывает, как часто runtime вынужден очищать heap;
- GC pause показывает stop-the-world часть работы сборщика.

### Allocation rate

```promql
sum by (namespace, pod) (
  rate(go_memstats_alloc_bytes_total{job="shortener"}[5m])
)
```

Результат — bytes, выделенные в heap за секунду. Heap может оставаться на уровне
`200 MiB`, а сервис при этом выделять гигабайты короткоживущих объектов в минуту
и тратить CPU на частый GC. Это allocation churn, а не memory leak.

Новые heap-объекты в секунду:

```promql
sum by (namespace, pod) (
  rate(go_memstats_mallocs_total{job="shortener"}[5m])
)
```

### Частота GC и средняя пауза

Число GC cycles в секунду:

```promql
sum(
  rate(go_gc_duration_seconds_count{job="shortener"}[5m])
)
```

Средняя stop-the-world пауза по собранным cycles:

```promql
sum(rate(go_gc_duration_seconds_sum{job="shortener"}[5m]))
/
sum(rate(go_gc_duration_seconds_count{job="shortener"}[5m]))
```

`go_gc_duration_seconds` является Summary. Его готовые series с label
`quantile` нельзя усреднять или суммировать между pods: локальные квантили не
восстанавливают глобальное распределение. `_sum` и `_count` можно складывать,
поэтому среднее выше корректно, но оно не заменяет tail latency.

Для детального анализа scheduler и GC можно включить нужные histogram metrics
из `runtime/metrics`. Их buckets и имена зависят от версии Go, поэтому dashboard
должен соответствовать версии runtime, на которой действительно работает
сервис.

### Как читать сочетания сигналов

| Наблюдение | Вероятная гипотеза | Следующая проверка |
| --- | --- | --- |
| Heap растёт после каждого цикла нагрузки | объекты удерживаются | heap profile и dominators |
| Heap стабилен, allocation rate и CPU высокие | allocation churn | allocs profile и benchmark |
| GC cycles участились после rollout | выросла скорость аллокаций или уменьшился target heap | сравнить allocation rate, `GOGC`, `GOMEMLIMIT` |
| RSS растёт, heap стабилен | non-heap memory, cgo, mmap, stacks или retained pages | container memory, goroutines, mappings, process profile |

---

## Файловые дескрипторы и время жизни процесса

Сокеты, файлы и pipes используют файловые дескрипторы. Их текущую долю от
лимита можно оценить так:

```promql
100 *
process_open_fds{job="shortener"}
/
process_max_fds{job="shortener"}
```

Постоянный рост при стабильной нагрузке может означать незакрытый response body,
файл, соединение или listener. Но всплеск `open_fds` во время роста параллелизма
нормален, поэтому график сравнивают с RPS, `in_flight`, connection pool и
числом активных сетевых соединений.

Uptime процесса:

```promql
time() - process_start_time_seconds{job="shortener"}
```

Низкий uptime помогает заметить недавний restart, но в Kubernetes причину и
число рестартов надёжнее брать из kube-state-metrics и статуса pod. Restart из-за
rollout, crash и OOMKill имеют разный operational смысл.

Process collector поддерживает не одинаковый набор метрик на всех платформах.
Если `process_open_fds` отсутствует, запрос не должен подменять отсутствие
нулём: сначала проверяют поддержку и содержимое `/metrics`.

---

## Как собрать dashboard

Runtime dashboard не должен жить отдельно от поведения сервиса. Практичная
структура состоит из трёх строк.

### Строка 1: пользовательский симптом

- RPS всего сервиса и по route;
- error ratio и абсолютные errors/s;
- p50, p95, p99 и доля запросов в пределах SLO;
- `in_flight` и saturation.

### Строка 2: CPU и память

- CPU cores по pod и сумма по сервису;
- RSS процесса по pod;
- container working set относительно memory limit;
- `heap_alloc`, `heap_inuse` и idle minus released;
- allocation bytes/s.

### Строка 3: runtime и process limits

- goroutines и OS threads по pod;
- GC cycles/s и средняя GC pause;
- heap objects;
- open/max file descriptors;
- uptime, restarts и OOM kills.

Для capacity нужен service-level `sum()`, а для поиска плохой реплики —
per-pod линии, `max by (pod)` или `topk()`. Если оставить только сумму, один
leaking pod может потеряться среди здоровых реплик. Если оставить только
среднее, rollout с двумя разными версиями будет выглядеть стабильнее, чем есть.

Полезные переменные dashboard: `cluster`, `namespace`, `service`, `version` и
`pod`. В PromQL для графиков вместо фиксированного `[5m]` можно использовать
Grafana `$__rate_interval`, если datasource и dashboard настроены согласованно.

---

## Диагностические сценарии

### RSS приближается к memory limit

1. Проверить container working set и расстояние до limit.
2. Сравнить RSS процесса и Go heap.
3. Если heap растёт, снять heap profile и найти удерживающие объекты.
4. Если heap стабилен, проверить goroutine stacks, cgo, mmap, page cache и
   дополнительные процессы контейнера.
5. Проверить `GOMEMLIMIT`, `GOGC`, OOM events и изменения после rollout.

### Число goroutines постоянно растёт

1. Сравнить график с RPS, `in_flight`, queue depth и latency dependencies.
2. Дождаться спада нагрузки и проверить возврат к baseline.
3. Снять goroutine profile до и после нагрузки.
4. Сгруппировать одинаковые stacks и искать ожидание channel, mutex, network I/O
   или забытый `context.CancelFunc`.

### CPU вырос после rollout

1. Сравнить RPS и состав routes между версиями.
2. Проверить allocation rate и частоту GC.
3. Проверить throttling и runnable scheduler latency.
4. Снять CPU profile на проблемной версии.
5. Сравнить профиль, latency и allocations с предыдущей версией на одинаковой
   нагрузке.

### File descriptors приближаются к лимиту

1. Проверить, растёт ли значение при стабильном `in_flight`.
2. Разделить sockets, files и pipes инструментами ОС.
3. Проверить закрытие HTTP response body, rows, files и соединений.
4. Не считать увеличение `ulimit` исправлением, пока причина роста неизвестна.

---

## Alerts без ложной точности

Alerts должны описывать риск для сервиса, а не необычный вид одного графика.

Хорошие кандидаты:

- container memory устойчиво близка к limit;
- процесс длительно использует большую долю CPU limit одновременно с ростом
  latency или throttling;
- файловые дескрипторы устойчиво занимают значимую долю лимита;
- процесс неожиданно перезапускается или получает OOMKill;
- allocation rate или число goroutines отклоняется от проверенного baseline
  конкретного сервиса после нормализации по нагрузке.

Порог `go_goroutines > 1000` без знания workload обычно бесполезен. Более
надёжный alert использует service-specific baseline, длительность условия и
контекст нагрузки. Например, retained growth подозрителен, если RPS вернулся к
обычному уровню, `in_flight` мал, а минимальное число goroutines в каждом
последующем окне продолжает расти.

Для memory alert оставляют запас до limit: scrape, alert evaluation и доставка
уведомления занимают время, а workload может расти скачком. Конкретный процент
и окно выбирают по скорости роста памяти, размеру всплесков и времени реакции
команды, а не копируют как универсальные `80% за 5 минут`.

---

## Типичные ошибки

1. **Смотреть только на Go heap.** OOM принимает решение по памяти cgroup, куда
   входят не только живые Go-объекты.
2. **Называть `go_memstats_sys_bytes` RSS.** Runtime reservation и физически
   resident pages имеют разные контракты.
3. **Считать высокий RSS доказательством утечки.** Нужен retained growth после
   спада нагрузки и подтверждение profile или другим независимым сигналом.
4. **Применять `rate()` к текущему heap или goroutines.** Это gauges; для них
   используют raw value, `*_over_time`, `delta()` или `deriv()` согласно вопросу.
5. **Суммировать Summary quantiles между pods.** Локальные p95 нельзя превратить
   в service p95 через `sum()` или `avg()`.
6. **Считать `rate(process_cpu_seconds_total) * 100` процентом container limit.**
   Это проценты одного core; limit может быть `0.5`, `2` или отсутствовать.
7. **Ставить общий alert на число goroutines.** Нормальный baseline зависит от
   числа соединений, workers и модели конкурентности.
8. **Регистрировать Go/process collectors дважды.** Default registry уже
   содержит их; пустой custom registry — нет.
9. **Суммировать раньше `rate()`.** Counter resets отдельных pods должны быть
   обработаны до агрегации.
10. **Включать все `runtime/metrics` без потребности.** Версионность и histogram
    buckets увеличивают число series и стоимость сопровождения.

---

## Interview-ready answer

**1. Какие системные метрики обычно экспортирует Go-сервис?**

- Базовый набор — RSS и CPU процесса, heap и allocation rate, goroutines, OS
  threads, GC pauses/cycles, file descriptors и start time.
- Граница — runtime/process metrics дополняют HTTP RED и container metrics, но
  не заменяют их.
- Подключение — default `client_golang` registry уже содержит Go и process
  collectors; в custom registry их регистрируют явно.

**2. Чем Go heap отличается от RSS и container memory?**

- Heap — память Go-объектов и spans, которыми управляет runtime.
- RSS — физически resident pages одного процесса, включая не только Go heap.
- Container memory — учёт cgroup для всех процессов и других видов памяти
  контейнера; именно её сопоставляют с container limit и OOM-риском.

**3. Как по метрикам заподозрить goroutine или memory leak?**

- Тренд — значение растёт через повторяющиеся циклы нагрузки и не возвращается
  к baseline после её спада.
- Корреляция — рост проверяют вместе с RPS, `in_flight`, heap, RSS, dependencies
  и restart events.
- Подтверждение — метрика формирует гипотезу, а причину ищут через heap или
  goroutine profile, traces и данные ОС.

**4. Как интерпретировать CPU и GC?**

- CPU — `rate(process_cpu_seconds_total[5m])` показывает среднее число занятых
  cores; умножение на `100` даёт проценты одного core.
- Аллокации — `rate(go_memstats_alloc_bytes_total[5m])` показывает bytes/s и
  обнаруживает churn даже при стабильном heap.
- GC — `_sum / _count` даёт среднюю паузу, но Summary quantiles нельзя
  агрегировать между pods.

---

## Официальная документация

- [client_golang: collectors](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus/collectors)
- [client_golang: registry](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Registry)
- [Go runtime metrics](https://pkg.go.dev/runtime/metrics)
- [Go GC guide](https://go.dev/doc/gc-guide)
- [Prometheus instrumentation](https://prometheus.io/docs/practices/instrumentation/)
- [PromQL functions](https://prometheus.io/docs/prometheus/latest/querying/functions/)
