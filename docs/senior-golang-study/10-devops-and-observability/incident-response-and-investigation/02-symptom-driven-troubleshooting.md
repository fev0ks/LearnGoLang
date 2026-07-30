# Диагностика по симптомам

## Содержание

- [Ментальная модель](#ментальная-модель)
- [Симптом не равен причине](#симптом-не-равен-причине)
- [Быстрая матрица выбора](#быстрая-матрица-выбора)
- [Какие измерения проверить сначала](#какие-измерения-проверить-сначала)
- [Рост ошибок](#рост-ошибок)
- [Рост задержки](#рост-задержки)
- [Падение пропускной способности](#падение-пропускной-способности)
- [Рост очереди](#рост-очереди)
- [Высокий CPU](#высокий-cpu)
- [Рост памяти и OOMKilled](#рост-памяти-и-oomkilled)
- [Рост числа горутин](#рост-числа-горутин)
- [Исчерпание пулов и файловых дескрипторов](#исчерпание-пулов-и-файловых-дескрипторов)
- [Сетевые ошибки и медленные соединения](#сетевые-ошибки-и-медленные-соединения)
- [Проблема только на части инстансов](#проблема-только-на-части-инстансов)
- [Как выбрать инструмент](#как-выбрать-инструмент)
- [Три вида трассировки](#три-вида-трассировки)
- [Безопасная диагностика в production](#безопасная-диагностика-в-production)
- [Как строить проверяемую гипотезу](#как-строить-проверяемую-гипотезу)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)
- [Официальные материалы](#официальные-материалы)

Один симптом может иметь причины на разных слоях. Высокий `p99` возникает из-за
CPU, ожидания блокировки, исчерпанного пула, очереди, DNS, ограничения CPU
(throttling) или медленной внешней зависимости. Поэтому расследование начинается не с любимого инструмента, а с
границы проблемы и сигнала, который разделяет конкурирующие гипотезы.

---

## Ментальная модель

Полезная последовательность:

```text
симптом
  -> масштаб и временное окно
  -> проблемная граница
  -> конкурирующие гипотезы
  -> сигнал, который их разделяет
  -> минимальная безопасная проверка
  -> mitigation
  -> повторное измерение
```

Пример:

```text
Симптом:
  p99 API вырос с 400 ms до 4 s.

Гипотеза A:
  процесс тратит время на CPU.

Гипотеза B:
  запросы ждут свободное DB connection.

Разделяющий сигнал:
  CPU utilization + DB pool wait.

Результат:
  CPU = 35%, pool wait p99 = 3.5 s.

Следующий инструмент:
  pool metrics и traces, а не CPU profile.
```

---

## Симптом не равен причине

| Наблюдение | Возможные причины |
| --- | --- |
| Высокий HTTP latency | CPU hotspot, lock, очередь, пул, DNS, сеть, downstream |
| Высокий CPU | полезная нагрузка, retry storm, GC pressure, busy loop, сериализация |
| Высокий RSS | Go heap, goroutine stacks, `mmap`, CGO, page cache |
| Низкий CPU и низкий throughput | I/O wait, lock contention, throttling, пустая очередь |
| `OOMKilled` | container limit, heap spike, native memory, слишком маленький limit |
| Рост goroutines | leak, нормальный рост concurrency, зависший downstream |
| Backlog растёт | мало workers, downstream медленный, poison message, rebalance |
| `connection refused` | процесс не слушает, Service routing, firewall, exhausted backlog |

Инструмент выбирается по вопросу:

- «Насколько широко?» — метрики.
- «Что изменилось и какая ошибка?» — логи и change timeline.
- «На каком межсервисном шаге потеряно время?» — distributed trace.
- «Где процесс тратит CPU или удерживает память?» — `pprof`.
- «Почему goroutines не исполняются вовремя?» — `runtime/trace`.
- «Что делает ОС, сеть или контейнер?» — Linux/Kubernetes/cloud telemetry.

---

## Быстрая матрица выбора

| Симптом | Сначала проверить | Развилка | Следующий инструмент |
| --- | --- | --- | --- |
| Ошибки растут | status/error kind, scope, deploy timeline | app error или dependency/edge | logs + traces |
| p99 растёт | RPS, error rate, saturation | CPU или ожидание | CPU profile либо pool/trace |
| Throughput падает | вход/выход, concurrency, queue | нет работы или работа ждёт | metrics + goroutine/block profile |
| Backlog растёт | ingest vs process rate, oldest age | capacity или poison/retry | consumer metrics + logs/traces |
| CPU высокий | per-pod CPU, throttling, GC | user code или runtime/OS | CPU profile + runtime metrics |
| RSS растёт | RSS vs heap, container limit | Go heap или вне heap | heap diff + `/proc`/container metrics |
| Goroutines растут | состояния и stack groups | нормальная concurrency или leak | goroutine dumps |
| Pool wait растёт | in-use/idle/max/wait | утечка или медленный dependency | metrics + traces + connection lifecycle |
| Network errors | DNS/connect/TLS/TTFB | путь до процесса или внутри upstream | `curl`, `ss`, DNS, traces |
| Один pod плохой | version/node/config/restarts | локальное состояние или node | pod diff + events + profile |

Эта таблица выбирает следующий вопрос, а не готовый диагноз.

---

## Какие измерения проверить сначала

### Временное окно

Все сигналы сравниваются на одном интервале и в одной timezone:

- начало пользовательского impact;
- момент alert;
- deploy/config/flag changes;
- начало роста saturation;
- время mitigation;
- восстановление.

Графики с разными окнами создают ложную причинность.

### RED для request path

RED:

- **Rate:** сколько запросов поступает;
- **Errors:** какая доля завершается ошибкой;
- **Duration:** распределение latency, особенно tail.

Нужно разделять как минимум:

- route/operation;
- status class или стабильный error kind;
- service/version/region;
- иногда dependency.

Не следует помещать `user_id`, `order_id`, URL с идентификатором или полный
текст ошибки в metric labels: это создаёт высокую cardinality.

### USE для ресурсов

USE:

- **Utilization:** насколько ресурс занят;
- **Saturation:** сколько работы ждёт ресурс;
- **Errors:** какие ошибки выдаёт ресурс.

Примеры saturation:

- run queue CPU;
- CPU throttling;
- DB pool waiters;
- disk queue;
- socket accept backlog;
- goroutines, ожидающие lock;
- broker backlog.

Utilization без saturation недостаточен. CPU может показывать 50% от одного
ядра, но контейнер с quota 0.5 CPU уже полностью насыщен и throttled.

### Сравнение с нормальным состоянием

Само значение редко достаточно:

- этот сервис всегда держит 20 000 goroutines или обычно 200?
- RSS 2 GiB — это рост или стабильный рабочий набор?
- p99 2 s нарушает SLO или соответствует batch operation?
- backlog 100 000 сообщений уменьшается или растёт?

Нужны baseline, предыдущая версия и аналогичные healthy instances.

---

## Рост ошибок

### Первый разрез

1. Какая доля запросов?
2. Какие status/error kinds?
3. Одна операция или весь сервис?
4. Одна версия, зона, tenant или payload class?
5. Ошибка создаётся приложением, proxy или downstream?

### Полезные различия

| Сигнал | Возможная область |
| --- | --- |
| Только `5xx` новой версии | regression приложения или конфигурации |
| `502/504` на edge, app не видит запрос | routing, proxy, pod availability |
| `429` от downstream | rate limit или retry amplification |
| `context deadline exceeded` | нужно найти, на каком ожидании исчерпан budget |
| `connection refused` | endpoint достижим, но никто не принимает соединение |
| `connection reset` | соединение оборвано peer/proxy/network |
| Ошибки одного business kind | данные или конкретная доменная ветка |

### Следующий инструмент

- error ratio и scope — метрики;
- новый error kind — структурированные логи;
- ошибка конкретного запроса — trace по `trace_id`;
- panic — stack trace и версия binary;
- ошибки только до приложения — load balancer, ingress, Service/endpoints;
- ошибки после вызова зависимости — dependency metrics и span.

Не следует начинать с grep случайного текста по всем логам. Сначала задаётся
временное окно, сервис, версия и стабильное поле ошибки.

---

## Рост задержки

Latency нужно разделить на **работу** и **ожидание**.

```text
request latency =
  queue wait
  + CPU work
  + lock/pool wait
  + database/cache calls
  + network/downstream calls
  + serialization
```

### Развилка CPU или ожидание

| Наблюдение | Следующий шаг |
| --- | --- |
| CPU высокий, goroutines runnable | CPU profile |
| CPU низкий, DB pool wait высокий | pool metrics и SQL spans |
| CPU низкий, mutex/block wait высокий | mutex/block profiles |
| Длинный downstream span | upstream latency, timeouts, retries |
| Trace быстрый, но client latency высокий | edge, queue до span, network |
| p50 стабилен, p99 растёт | saturation, редкая ветка, GC, noisy neighbor |
| Все перцентили растут одинаково | общий dependency или постоянное удорожание |

### Почему average мешает

Пусть 99 запросов занимают `100 ms`, а один — `10 s`.

```text
average =
  (99 * 0.1 s + 1 * 10 s) / 100
  = 19.9 s / 100
  = 0.199 s
```

Среднее `199 ms` скрывает пользователя с десятисекундным ожиданием. Поэтому
смотрятся histogram и p95/p99, но вместе с числом запросов и bucket design.

### Инструменты

- distributed trace локализует длинный межсервисный шаг;
- CPU profile показывает активное потребление CPU;
- mutex/block profile показывает ожидание синхронизации;
- `runtime/trace` показывает scheduler delay, GC и состояние goroutines;
- Linux/Kubernetes metrics показывают throttling и node pressure.

---

## Падение пропускной способности

Throughput может падать при высоком или низком CPU.

### Высокий CPU

Вероятны:

- более дорогой payload;
- regression алгоритма;
- сериализация;
- GC pressure;
- retry storm;
- слишком много логирования;
- compression/encryption.

### Низкий CPU

Вероятны:

- работа ждёт DB connection;
- lock contention;
- внешняя зависимость зависла;
- consumers не получают partitions;
- rate limiter ограничивает обработку;
- context cancellations обрывают работу;
- процесс throttled относительно маленькой quota.

### Закон сохранения потока

Для очереди:

```text
backlog_delta = ingest_rate - process_rate
```

Если ingest `1200 msg/s`, а process `900 msg/s`:

```text
backlog растёт на 300 msg/s
за 10 минут:
300 * 600 = 180 000 сообщений
```

Throughput нужно сравнивать со входным потоком: «consumer обрабатывает мало»
может быть нормой, если сообщений не поступает.

---

## Рост очереди

Сначала нужны четыре числа:

- ingest rate;
- successful processing rate;
- retry/error rate;
- age самого старого сообщения.

Количество сообщений без age неоднозначно. Большой backlog может быть новым, но
быстро перерабатываться; маленький backlog может содержать одно критичное
сообщение, застрявшее на сутки.

### Основные классы причин

1. **Недостаточная capacity:** каждый consumer работает корректно, но медленно.
2. **Медленный downstream:** workers заняты ожиданием.
3. **Poison message:** одно сообщение постоянно retry и блокирует partition.
4. **Rebalance/ownership:** часть partitions не назначена или consumers
   постоянно перезапускаются.
5. **Backpressure:** система сознательно замедляет intake.
6. **Duplicate/retry amplification:** одна бизнес-операция создаёт много работы.

### Проверки

- lag по partition/subscription;
- processing duration;
- concurrency и in-flight;
- доля success/retry/dead-letter;
- downstream latency;
- частота restarts/rebalances;
- ключи ordering;
- повторяется ли один message ID или business key.

Scale out помогает только если broker распределит работу и downstream выдержит
дополнительную concurrency.

---

## Высокий CPU

### Сначала исключить измерительную ловушку

- CPU измеряется на pod, process или node?
- значение указано в cores или процентах?
- какая CPU quota?
- есть ли throttling?
- проблема на всех replicas?
- RPS и payload изменились?

Контейнер с limit `500m` получает половину CPU core. Использование `0.5 core`
может выглядеть как 50% одного ядра, но для контейнера это полное насыщение.

### Разделить user code, runtime и ядро

| Сигнал | Вероятная область |
| --- | --- |
| CPU profile показывает business functions | алгоритм, JSON, compression |
| `runtime.gcBgMarkWorker` заметен, allocations растут | GC pressure |
| CPU profile приложения пустой, system CPU высокий | syscalls, kernel, network/disk |
| throttling растёт | cgroup quota |
| много retries при том же business rate | amplification |

Подробный workflow находится в
[CPU Profiling](../../01-go-core/profiling/02-cpu-profiling.md).

---

## Рост памяти и OOMKilled

Нужно различать:

- Go heap;
- goroutine stacks;
- runtime metadata;
- native/CGO allocations;
- memory-mapped regions;
- page cache и shared memory;
- RSS контейнера.

### Основная развилка

```text
RSS растёт, Go heap растёт
  -> heap retention, unbounded cache, goroutine leak, allocation pressure.

RSS растёт, Go heap стабилен
  -> native memory, mmap, stacks, fragmentation, page cache.

Heap растёт и затем падает после GC
  -> возможно, высокий churn, а не leak.

Heap после нескольких GC продолжает расти
  -> искать retained objects и владельца ссылок.
```

### Что сохранить до restart

- heap profile;
- allocs profile;
- goroutine dump;
- `runtime.MemStats`/Go runtime metrics;
- RSS и cgroup usage/limit;
- `/proc/<pid>/smaps` или эквивалент;
- OOM event и container termination reason;
- version/config/payload class.

Сравнение двух heap profiles обычно полезнее одного снимка. Детали находятся в
[Memory Profiling](../../01-go-core/profiling/03-memory-profiling.md).

---

## Рост числа горутин

Рост не всегда является leak. При росте RPS закономерно увеличивается число
одновременных запросов.

### Проверка

1. Сравнить goroutine count с RPS и in-flight.
2. Убрать нагрузку или дождаться завершения burst.
3. Снять два goroutine dump.
4. Сгруппировать одинаковые stacks.
5. Проверить, исчезают ли старые группы.

Типичные состояния:

- ожидание channel без cancellation;
- зависший network call без timeout;
- worker loop без stop;
- `WaitGroup` без `Done`;
- timer/ticker без остановки;
- goroutines, удерживающие connection или большой объект.

Если 10 000 goroutines стоят на одном stack ожидания DB connection, это может
быть не goroutine leak, а отсутствие admission control перед маленьким пулом.

Подробности:
[Goroutine And Concurrency Profiling](../../01-go-core/profiling/04-goroutine-concurrency-profiling.md).

---

## Исчерпание пулов и файловых дескрипторов

### Пулы

Для DB/HTTP/Redis pool полезны:

- max;
- in-use;
- idle;
- waiters;
- wait duration;
- acquisition timeout;
- connection lifetime;
- ошибки создания соединений.

```text
in-use = max
waiters растут
dependency latency растёт
```

Это может означать медленную зависимость, а не слишком маленький pool. Простое
увеличение max переносит очередь в базу и способно ухудшить её saturation.

### File descriptors

Симптомы:

- `too many open files`;
- новые соединения не создаются;
- число sockets/files постоянно растёт;
- много `CLOSE_WAIT`.

Проверяются:

- process limit и container limit;
- количество открытых FD;
- типы FD;
- lifecycle response bodies/connections/files;
- connection reuse;
- соответствие pool budget числу replicas.

Команды собраны в
[Linux: команды для диагностики production](../linux/06-linux-commands.md).

---

## Сетевые ошибки и медленные соединения

«Проблема сети» нужно разложить на этапы:

```text
DNS
  -> TCP connect
  -> TLS handshake
  -> request write
  -> server queue/work
  -> time to first byte
  -> response body
```

| Симптом | Возможная граница |
| --- | --- |
| DNS timeout/NXDOMAIN | resolver, record, search domain |
| Connect timeout | routing, firewall, SYN loss, backlog |
| Connection refused | endpoint доступен, listener отсутствует |
| TLS handshake error | сертификат, SNI, protocol/cipher |
| Высокий TTFB | server queue или обработка |
| Медленная загрузка body | bandwidth, packet loss, большой response |
| `CLOSE_WAIT` | приложение не закрывает peer-closed connection |

Distributed trace показывает длительность client span, но не всегда разделяет
DNS, connect и TLS. Для этого полезны HTTP client metrics, `curl` timings,
socket statistics и packet capture с соблюдением политики безопасности.

---

## Проблема только на части инстансов

Один плохой pod — очень полезная контрольная группа.

Сравнить:

- image digest и commit;
- конфигурацию, secrets и flags;
- node, zone и instance type;
- CPU/memory requests и limits;
- restart count и termination reason;
- traffic distribution;
- connections/pools;
- локальный cache;
- clock/timezone;
- mounted volumes;
- sidecars;
- kernel/cgroup pressure.

Если новый pod сразу healthy, а старый деградирует со временем, вероятны:

- leak;
- локальное накопленное состояние;
- expiring credentials/connections;
- unbounded cache;
- fragmentation;
- проблема конкретной node.

Если все pod одной версии деградируют сразу, версия/конфигурация становится
сильнее гипотезы локального состояния.

---

## Как выбрать инструмент

| Инструмент | Лучший вопрос | Плохое применение |
| --- | --- | --- |
| Метрики | где и когда изменилось поведение | искать stack конкретной ошибки |
| Логи | какое событие/ошибка произошло | считать latency distribution по тексту |
| Distributed tracing | какой компонент задержал запрос | искать fleet-wide memory leak |
| CPU profile | где процесс активно тратит CPU | анализировать network wait |
| Heap profile | что удерживает/аллоцирует память | объяснять RSS вне Go heap |
| Goroutine profile | где goroutines находятся сейчас | измерять длительность всей истории |
| Mutex/block profile | где ожидают синхронизацию | искать медленный внешний API без локов |
| `runtime/trace` | scheduler, GC, runnable/blocking timeline | заменять межсервисный trace |
| Linux tools | процесс, sockets, disk, kernel | доказывать business impact |
| Kubernetes events | lifecycle pod/node и scheduling | анализировать функцию Go |

---

## Три вида трассировки

Название «trace» используется для разных сущностей.

### Distributed tracing

OpenTelemetry trace связывает spans нескольких сервисов и показывает путь
запроса или сообщения:

```text
gateway -> booking -> PostgreSQL -> payment provider
```

Он отвечает: **на какой межкомпонентной операции потеряно время?**

### Go execution trace

`runtime/trace` записывает события Go runtime в одном процессе:

- выполнение goroutines;
- scheduler delays;
- GC;
- syscalls;
- network blocking;
- user tasks/regions.

Он отвечает: **почему goroutine не исполнялась или где возникла runtime-пауза?**

### Системная трассировка

`strace`, `perf`, eBPF и packet capture наблюдают syscalls, kernel, scheduling и
network path. Они отвечают на вопросы ниже уровня Go runtime.

Эти инструменты не заменяют друг друга. В типичном flow:

```text
метрика показывает p99
  -> distributed trace локализует booking
  -> mutex profile показывает hot lock
  -> runtime/trace подтверждает scheduler/blocking pattern
```

---

## Безопасная диагностика в production

### Ограничить доступ

`/debug/pprof` и другие debug endpoints не должны быть публичными. Используются:

- отдельный внутренний listener;
- network policy/firewall;
- аутентификация и аудит доступа;
- port-forward или approved debug path;
- запрет попадания endpoints в public ingress.

Profiles и goroutine dumps могут раскрывать function names, URL, параметры и
часть данных из stack.

### Учитывать overhead

- CPU profiling добавляет стоимость;
- mutex/block profiling требует sampling configuration;
- `runtime/trace` может создавать заметный объём данных;
- несколько профилей одновременно искажают измерения;
- heap dump и debug actions расходуют CPU/память/I/O.

Go documentation рекомендует заранее измерить overhead и собирать один профиль
за раз. Для replicated service безопаснее выбрать representative instance, а не
профилировать весь fleet одновременно.

### Собирать representative данные

Профиль idle pod не объясняет CPU spike. Нужно записать:

- точную версию binary;
- временное окно;
- RPS и тип нагрузки;
- pod/region;
- duration и profile type;
- baseline для сравнения.

### Начинать с наименее опасного действия

```text
metrics/read-only queries
  -> logs/traces
  -> bounded profile
  -> debug container/read-only inspection
  -> config/traffic mutation
  -> restart/rollback
```

Это не абсолютный порядок: при серьёзном impact rollback может быть нужен
раньше profile. Риск действия сравнивается с продолжающимся ущербом.

---

## Как строить проверяемую гипотезу

Хорошая гипотеза содержит:

1. наблюдение;
2. предполагаемый механизм;
3. предсказание;
4. проверку;
5. критерий опровержения.

```text
Наблюдение:
  только pod на node-a имеют высокий p99 и CPU throttling.

Механизм:
  node-a испытывает CPU pressure, cgroup не получает quota вовремя.

Предсказание:
  перенос pod на другую node уберёт throttling;
  версия приложения и трафик останутся теми же.

Проверка:
  безопасно drain/replace одного pod и сравнить SLI.

Опровержение:
  новый pod на другой node показывает тот же throttling и p99.
```

---

## Типичные ошибки

- использовать один snapshot вместо временного ряда;
- сравнивать графики в разных timezone;
- считать корреляцию с deploy доказанной причиной;
- смотреть CPU profile при ожидании I/O;
- называть любой рост RSS memory leak;
- увеличивать pool без проверки dependency capacity;
- профилировать все replicas одновременно;
- открывать debug endpoint наружу;
- перезапускать единственный проблемный pod до сохранения evidence;
- оптимизировать p99 по average;
- менять несколько переменных без timeline;
- считать HTTP 200 доказательством корректного бизнес-результата.

---

## Interview-ready answer

**1. Как выбрать инструмент для production-проблемы?**

Сначала определяю symptom, scope и проблемную границу. Метрики показывают
масштаб, логи — конкретные события, distributed trace — медленный компонент,
`pprof` — потребление ресурсов внутри Go-процесса, а системные инструменты —
контейнер, сеть и ядро.

**2. Что делать, если p99 высокий, а CPU низкий?**

Проверить ожидания: DB/HTTP pools, downstream spans, mutex/block profiles,
queueing, throttling и I/O. Низкий CPU часто означает, что процесс ждёт, а не
работает.

**3. Чем distributed trace отличается от `runtime/trace`?**

Distributed trace показывает путь запроса через сервисы. `runtime/trace`
показывает scheduler, goroutines, GC и runtime-события внутри одного процесса.

**4. Как искать memory leak в Go?**

Сравнить RSS и Go heap, снять heap profiles в разные моменты, посмотреть retained
delta и goroutine dumps. Если RSS растёт при стабильном heap, искать native
memory, mmap, stacks или page cache.

**5. Почему нельзя сразу увеличить connection pool?**

Pool может быть очередью перед уже насыщенной БД. Увеличение переносит больше
concurrency в зависимость и способно ухудшить latency и отказ.

---

## Связанные материалы

- [Incident Response Workflow](./01-incident-response-workflow.md)
- [Go Profiling](../../01-go-core/profiling/README.md)
- [Practical Metric Patterns](../prometheus-and-metrics/practical-metric-patterns/README.md)
- [Tempo And Trace Investigation](../tracing-and-opentelemetry/03-tempo-and-trace-investigation.md)
- [Linux: команды для диагностики production](../linux/06-linux-commands.md)
- [Kubernetes: kubectl commands](../kubernetes/05-kubectl-commands.md)
- [Cross-Layer Incident Cases](./03-cross-layer-incident-cases.md)

---

## Официальные материалы

- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [OpenTelemetry Observability Primer](https://opentelemetry.io/docs/concepts/observability-primer/)
- [Kubernetes Troubleshooting Applications](https://kubernetes.io/docs/tasks/debug/debug-application/)
- [kubectl debug](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_debug/)
