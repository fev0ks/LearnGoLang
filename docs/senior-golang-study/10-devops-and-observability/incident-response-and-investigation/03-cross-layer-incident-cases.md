# Межслойные кейсы инцидентов

## Содержание

- [Как работать с кейсами](#как-работать-с-кейсами)
- [Кейс 1: p99 вырос после выкладки, CPU нормальный](#кейс-1-p99-вырос-после-выкладки-cpu-нормальный)
- [Кейс 2: ошибки только на одном pod](#кейс-2-ошибки-только-на-одном-pod)
- [Кейс 3: растёт очередь consumer](#кейс-3-растёт-очередь-consumer)
- [Кейс 4: OOMKilled при стабильном Go heap](#кейс-4-oomkilled-при-стабильном-go-heap)
- [Кейс 5: отказ внешней зависимости превратился в retry storm](#кейс-5-отказ-внешней-зависимости-превратился-в-retry-storm)
- [Кейс 6: CPU throttling без видимых 100 процентов CPU](#кейс-6-cpu-throttling-без-видимых-100-процентов-cpu)
- [Кейс 7: истёк кэш и перегрузил источник данных](#кейс-7-истёк-кэш-и-перегрузил-источник-данных)
- [Сводная таблица](#сводная-таблица)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Эти кейсы тренируют не запоминание «симптом → причина», а построение
расследования. В каждом сценарии похожий внешний сигнал может возникнуть на
разных слоях, поэтому решение строится через охват проблемы, разделяющие
метрики, проверяемую гипотезу и безопасное ограничение ущерба.

---

## Как работать с кейсами

Перед чтением решения полезно ответить:

1. Как звучит пользовательское влияние?
2. Какие измерения ограничат охват проблемы?
3. Какие две-три причины пока совместимы с фактами?
4. Какой сигнал разделит эти причины?
5. Что можно сделать для ограничения ущерба до полного анализа первопричины?
6. Как доказать восстановление?

Правильный ответ не обязан с первой минуты назвать компонент. Сильный
инженерный ответ показывает последовательность уменьшения неопределённости.

---

## Кейс 1: p99 вырос после выкладки, CPU нормальный

### Симптом

После выкладки `booking-v43`:

```text
RPS:          800 -> 820
error rate:   0.2% -> 4%
p50:          180 ms -> 220 ms
p99:          700 ms -> 6 s
CPU:          45% -> 48%
```

Рост p99 почти не сопровождается ростом CPU.

### Плохая гипотеза

> Новый код медленный, нужно снять CPU profile.

CPU profile может быть полезен позже, но текущие факты скорее указывают на
ожидание: tail вырос значительно сильнее median, а процесс не потребляет больше
CPU.

### Локализация

Разрез по версии:

```text
v42 p99 = 750 ms
v43 p99 = 6 s
```

Distributed traces версии `v43`:

```text
POST /booking             5.9 s
  validation               8 ms
  acquire DB connection    4.8 s
  SELECT                   35 ms
  provider call           600 ms
```

Pool metrics:

```text
max_open = 50
in_use  = 50
waiters = 420
wait p99 = 4.7 s
```

SQL выполняется быстро. Очередь находится до базы — на приобретении соединения.

### Проверяемая гипотеза

В `v43` одна новая ветка перестала закрывать `*sql.Rows` и не дочитывает их до
конца. Пока rows не закрыты, не исчерпаны или не отменены через context,
соединение может оставаться занятым; рассчитывать на своевременную cleanup
через runtime нельзя.

Предсказания:

- проблема есть только на пути новой ветки;
- число in-use постепенно доходит до max;
- SQL latency остаётся нормальным;
- rollback уменьшит pool wait после замены pod.

### Mitigation

Если предыдущая версия совместима:

1. остановить rollout;
2. откатить `v43`;
3. ограничить входящую concurrency, пока старые pod заменяются;
4. не увеличивать pool вслепую.

### Проверка восстановления

- pool wait падает;
- in-use перестаёт постоянно упираться в max;
- p99 и error rate возвращаются;
- business flow создаёт корректный заказ;
- через несколько минут saturation не возвращается.

### Долговечное исправление

- закрывать rows/body во всех путях;
- тестировать connection lifecycle;
- добавить pool wait/in-use metrics;
- алертировать до исчерпания pool;
- ограничить concurrency перед БД;
- canary проверять по p99 и saturation.

### Урок

«После deploy» помогло найти scope, но причина обнаружилась не через CPU.
Latency состояла преимущественно из ожидания pool.

---

## Кейс 2: ошибки только на одном pod

### Симптом

Общий error rate равен 3%. Load balancer распределяет трафик на десять pod, но
разрез по instance показывает:

```text
9 pod: < 0.2% errors
1 pod: 31% errors
```

### Возможные причины

- другая версия или конфигурация;
- pod на проблемной node;
- локальный cache/state;
- истёкший credential;
- повреждённый volume;
- потерянное соединение;
- memory/FD leak;
- clock skew;
- неравномерный тип трафика.

### Сравнение healthy и unhealthy

| Свойство | Healthy pod | Unhealthy pod |
| --- | --- | --- |
| Image digest | `sha256:a1` | `sha256:a1` |
| Config revision | `cfg-18` | `cfg-18` |
| Node | `node-b` | `node-a` |
| Restart count | 0 | 0 |
| RSS | 600 MiB | 620 MiB |
| DNS errors | 0 | растут |
| Zone | `eu-1b` | `eu-1a` |

Версия и память одинаковы. Отличаются node/zone и DNS errors.

### Безопасная проверка

Создать новый pod той же версии на другой node и постепенно перевести малую
часть трафика.

Предсказание:

- на новой node ошибки исчезнут;
- приложение и конфигурация останутся прежними;
- проблема сохранится у других workloads на `node-a`, если причина node-level.

### Mitigation

- убрать unhealthy pod из readiness/traffic;
- cordon/drain проблемную node по процедуре;
- сохранить events, node metrics и DNS telemetry;
- убедиться, что оставшаяся capacity выдерживает трафик.

### Что нельзя заключить

Успешный restart pod не доказывает application bug. Новый pod мог оказаться на
другой node и тем самым скрыть инфраструктурную причину.

### Долговечное исправление

В зависимости от подтверждённой причины:

- исправить node-local DNS;
- добавить per-node/zone dashboards;
- alert по распределению ошибок между instances;
- настроить topology spread;
- автоматически исключать unhealthy endpoint через readiness.

### Урок

Агрегированная метрика скрывает локальный отказ. Разрез по instance превращает
«3% случайных ошибок» в воспроизводимую проблему одного failure domain.

---

## Кейс 3: растёт очередь consumer

### Симптом

```text
ingest rate:       1000 msg/s
success rate:       850 msg/s
retry rate:         140 msg/s
dead-letter rate:    10 msg/s
oldest message age:  25 min
CPU consumers:       35%
```

Backlog растёт, хотя CPU свободен.

### Конкурирующие гипотезы

1. Workers недостаточно.
2. Downstream медленный.
3. Poison messages постоянно retry.
4. Consumer ownership/rebalance сломан.
5. Ordering key блокирует часть partitions.

### Разделяющие сигналы

```text
processing p50 = 80 ms
processing p99 = 12 s

payment-provider span p99 = 11.5 s
local processing = 30 ms

85% retries относятся к одному provider error kind.
```

CPU свободен, потому что workers ждут provider.

### Почему scale out опасен

Если увеличить consumers с 20 до 40, число параллельных provider calls может
удвоиться. Деградировавший provider получит ещё больше трафика и начнёт
отвечать медленнее.

### Mitigation

- включить circuit breaker или временно отключить необязательную ветку;
- ограничить concurrency к provider;
- добавить exponential backoff с jitter;
- не retry non-retryable errors;
- отправить poison messages в DLQ;
- сохранить ordering для связанных событий;
- при допустимости обслуживать degraded result.

### Расчёт восстановления

После mitigation:

```text
ingest = 1000 msg/s
success = 1400 msg/s
backlog = 480 000

drain rate = 1400 - 1000 = 400 msg/s
drain time = 480 000 / 400 = 1200 s = 20 min
```

Пользовательская freshness восстановится не сразу после падения error rate.

### Долговечное исправление

- dependency-specific bulkhead;
- bounded retries и retry budget;
- backlog count + oldest age alerts;
- DLQ workflow;
- idempotent consumer;
- load test при деградировавшем provider;
- runbook для остановки retry amplification.

### Урок

Низкий CPU не означает, что нужно больше workers. Система ждёт внешний ресурс,
а дополнительная concurrency усиливает отказ.

---

## Кейс 4: OOMKilled при стабильном Go heap

### Симптом

Pod периодически получает `OOMKilled`.

```text
container memory limit = 2 GiB
RSS перед OOM          = 2 GiB
Go heap in-use         = 700 MiB
goroutines             = стабильно 500
```

Heap profile не показывает рост, достаточный для OOM.

### Ошибочный вывод

> Heap profile ничего не показал, значит метрика памяти неверная.

Container memory включает больше, чем Go heap.

### Возможные владельцы памяти

- CGO/native library;
- `mmap`;
- goroutine stacks;
- runtime metadata;
- большие network/file buffers;
- page cache, учитываемый cgroup;
- child process или sidecar в общей границе;
- allocator fragmentation и медленный возврат страниц ОС.

### Локализация

Сравниваются:

- cgroup memory breakdown;
- RSS/PSS;
- `/proc/<pid>/smaps`;
- Go heap/stacks;
- native allocator metrics;
- page cache;
- sidecar consumption;
- размер и lifecycle mmap/file operations.

Предположим:

```text
Go heap       = 700 MiB
goroutine stk = 40 MiB
anonymous RSS = 1.7 GiB
file-backed   = 150 MiB

native image decoder удерживает buffers после каждого batch.
```

### Mitigation

- временно уменьшить batch/concurrency;
- ограничить размер входных файлов;
- выключить проблемную native-ветку;
- увеличить limit только при наличии node capacity и понятного временного риска;
- сохранить native/system evidence до restart.

### Долговечное исправление

- исправить lifecycle native allocations;
- добавить метрику RSS minus Go managed memory;
- нагрузочный тест с representative files;
- bounded batch;
- alert по приближению к cgroup limit;
- установить `GOMEMLIMIT` ниже container limit для Go-части, понимая, что он не
  ограничивает native memory.

### Урок

`OOMKilled` описывает решение cgroup/kernel, а heap profile — только Go-managed
memory samples. Один инструмент не видит весь footprint.

---

## Кейс 5: отказ внешней зависимости превратился в retry storm

### Симптом

Внешний API начал отвечать `503`. Через минуту:

```text
business RPS            = 500
outbound attempts RPS   = 4500
application CPU         = 90%
downstream latency      = 8 s
local queue             = растёт
```

### Механизм

Пусть один пользовательский запрос делает до трёх service-level retries, а
HTTP-клиент внутри каждой попытки выполняет ещё до трёх transport retries:

```text
максимум attempts = 3 * 3 = 9
500 requests/s * 9 = 4500 attempts/s
```

Retry на нескольких слоях создаёт multiplicative amplification.

### Дополнительный ущерб

- goroutines и connections заняты ожиданием;
- deadlines пользователей истекают;
- retries продолжают работу после потери полезности;
- downstream восстанавливается под искусственно увеличенной нагрузкой;
- локальный CPU тратится на сериализацию, TLS и логи ошибок.

### Разделяющие сигналы

- business request rate стабилен;
- outbound attempt rate вырос;
- один `trace_id` содержит повторяющиеся spans;
- retry labels показывают несколько уровней;
- cancellation/deadline не останавливает вложенные операции.

### Mitigation

- отключить retries на одном уровне;
- включить circuit breaker;
- уменьшить retry budget;
- exponential backoff + jitter;
- load shedding;
- degraded response или stale cache;
- отменять работу после request deadline.

### Долговечное исправление

- один владелец retry policy;
- общий deadline budget;
- retries только для идемпотентных и retryable операций;
- метрики business calls отдельно от attempts;
- тест отказа downstream;
- per-dependency bulkhead.

### Урок

Высокий outbound RPS не всегда является ростом пользователей. Нужно разделять
логическую операцию, попытку и retry.

---

## Кейс 6: CPU throttling без видимых 100 процентов CPU

### Симптом

Сервис имеет:

```text
CPU request = 100m
CPU limit   = 500m
usage       = 0.48 core
node имеет 32 cores
```

Dashboard показывает «CPU 48%», но p99 периодически растёт.

### Почему интерпретация неверна

Если процент построен относительно одного core:

```text
0.48 / 1.0 = 48%
```

Но относительно quota контейнера:

```text
0.48 / 0.5 = 96%
```

Контейнер почти исчерпал доступный CPU. При burst cgroup throttles процесс,
хотя node в целом свободна.

### Сигналы

- `container_cpu_cfs_throttled_*` растёт;
- p99 коррелирует с throttled periods;
- runnable goroutines растут;
- CPU profile показывает обычную работу без нового hotspot;
- перенос на pod без жёсткого limit устраняет spikes.

### Mitigation

- увеличить CPU limit/request при наличии capacity;
- снизить concurrency или дорогую необязательную обработку;
- временно масштабировать replicas, если downstream выдержит;
- убрать pathological workload.

### Долговечное исправление

- dashboard относительно quota;
- alert по throttling, а не только usage;
- load test с реальными cgroup limits;
- адекватный `GOMAXPROCS`;
- requests/limits из измеренного профиля;
- проверка noisy-neighbor/node pressure.

### Урок

Процент без знаменателя вводит в заблуждение. Нужно знать единицы и resource
boundary.

---

## Кейс 7: истёк кэш и перегрузил источник данных

### Симптом

Популярные ключи Redis имеют одинаковый TTL 10 минут. После deploy все replicas
прогреваются одновременно.

```text
cache hit ratio: 98% -> 5%
DB QPS:           2k -> 35k
DB pool wait:      0 -> 4 s
API p99:       300ms -> 8 s
```

Redis работает и отвечает быстро, но база перегружена.

### Механизм

```text
одинаковый cold start
  -> одинаковое время заполнения
  -> синхронное истечение TTL
  -> множество cache misses
  -> stampede в БД
  -> pool saturation
  -> timeouts и retries
  -> ещё больше нагрузки
```

### Почему «масштабировать API» опасно

Новые replicas также холодные и создают ещё больше конкурентных DB queries.

### Разделяющие сигналы

- Redis command latency нормальна;
- hit ratio резко падает;
- DB QPS и wait растут;
- одинаковые keys загружаются многими instances;
- проблема совпадает с deploy или TTL boundary.

### Mitigation

- остановить autoscaling surge;
- ограничить DB fallback concurrency;
- временно отдавать stale values, если домен допускает;
- прогреть ограниченный набор горячих ключей;
- увеличить TTL только осознанно;
- load shed для дорогих misses.

### Долговечное исправление

- TTL jitter;
- local `singleflight`;
- cross-instance request coalescing для действительно горячих ключей;
- stale-while-revalidate;
- soft/hard TTL;
- warm-up с ограничением скорости;
- cache hit/miss и source QPS metrics;
- bulkhead на fallback;
- capacity plan для работы без cache.

### Урок

Cache может скрывать недостаточную capacity source of truth. Его доступность не
гарантирует высокий hit ratio.

---

## Сводная таблица

| Кейс | Обманчивый первый сигнал | Разделяющий сигнал | Главный mitigation |
| --- | --- | --- | --- |
| p99 после deploy | «нужен CPU profile» | DB pool wait | rollback + admission control |
| Один плохой pod | «случайные 3% ошибок» | per-instance/node split | убрать endpoint из traffic |
| Backlog | «добавить consumers» | downstream span + retry rate | limit concurrency/circuit breaker |
| OOMKilled | «Go heap leak» | RSS minus managed heap | ограничить native/batch path |
| Retry storm | «вырос пользовательский RPS» | business calls vs attempts | общий retry budget/breaker |
| CPU throttling | «CPU только 48%» | usage относительно quota | изменить limit/concurrency |
| Cache stampede | «Redis недоступен» | hit ratio + DB QPS | fallback bulkhead/stale |

---

## Interview-ready answer

**1. Почему нельзя диагностировать production по одному графику?**

Один симптом совместим с несколькими механизмами. Например, высокий p99 может
быть CPU work, pool wait или downstream. Нужен второй сигнал, который разделяет
эти гипотезы.

**2. Что вы проверяете при проблеме только на одном pod?**

Версию, конфигурацию, node/zone, restarts, limits, traffic, локальный cache,
connections и системные события. Healthy pod служит контрольной группой.

**3. Почему scale out может ухудшить backlog?**

Если workers ждут деградировавший downstream, дополнительные workers увеличат
concurrency и retries, а не полезный throughput.

**4. Почему Go heap может быть стабильным перед OOMKilled?**

Container memory включает native allocations, mmap, stacks, runtime metadata и
page cache. Heap profile наблюдает только управляемую Go память.

**5. Как обнаружить retry amplification?**

Сравнить business operation rate с outbound attempt rate и посмотреть
повторяющиеся spans. Retry policy должна иметь одного владельца и общий budget.

**6. Как cache может ухудшить доступность?**

Синхронные misses направляют множество запросов в source of truth. Без jitter,
coalescing и ограничения fallback cache stampede перегружает базу.

---

## Связанные материалы

- [Incident Response Workflow](./01-incident-response-workflow.md)
- [Symptom-Driven Troubleshooting](./02-symptom-driven-troubleshooting.md)
- [Go Profiling Case Studies](../../01-go-core/profiling/07-case-studies.md)
- [Timeouts And Deadlines](../../05-system-design/reliability-patterns/01-timeouts-and-deadlines.md)
- [Retries And Backoff](../../05-system-design/reliability-patterns/02-retries-and-backoff.md)
- [Backpressure And Load Shedding](../../05-system-design/reliability-patterns/05-backpressure-and-shedding.md)
- [Bulkhead](../../05-system-design/reliability-patterns/07-bulkhead.md)
- [Kubernetes: отказ узла, выселение и бюджеты доступности](../kubernetes/08-node-failure-and-disruptions.md)
- [Kubernetes: стратегии обновления и безопасный rollout](../kubernetes/09-update-strategies.md)
