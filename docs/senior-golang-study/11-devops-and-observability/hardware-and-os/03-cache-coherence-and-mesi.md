# Cache coherence, MESI и false sharing

В системе с несколькими ядрами каждое ядро имеет **свой** L1 и L2 cache. Когда два ядра работают с одной и той же ячейкой памяти, у них в кэше — **разные копии** одних и тех же данных. Что если одно ядро её меняет?

Это — проблема **cache coherence**. Без её решения многопоточный код был бы невозможен — каждое ядро видело бы свою устаревшую версию данных. Hardware решает эту проблему автоматически через **MESI protocol**. Но "автоматически" — не "бесплатно": cache coherence — один из главных источников скрытых тормозов в concurrent коде.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Cache line — единица работы](#cache-line-единица-работы)
- [Проблема coherence](#проблема-coherence)
- [MESI protocol](#mesi-protocol)
- [MESIF, MOESI и другие варианты](#mesif-moesi-и-другие-варианты)
- [Cache coherence traffic — невидимая стоимость](#cache-coherence-traffic-невидимая-стоимость)
- [False sharing](#false-sharing)
- [Демо false sharing в Go](#демо-false-sharing-в-go)
- [Padding для защиты от false sharing](#padding-для-защиты-от-false-sharing)
- [True sharing — когда оно неизбежно](#true-sharing-когда-оно-неизбежно)
- [NUMA](#numa)
- [Практические выводы](#практические-выводы)

---

## Простая аналогия

Представь что у каждого сотрудника офиса есть **личный блокнот** с копией расписания совещаний. Когда секретарь меняет расписание в главном журнале, что делать с копиями в блокнотах? Варианты:

- Звонить каждому и сказать "вычеркни старое" (invalidation)
- Отправить новую версию всем (update)
- Каждый сам перепроверяет журнал перед своим совещанием (но это много походов)

Cache coherence — это именно тот протокол, по которому ядра CPU "договариваются" о согласованности своих копий. И как в офисе — обновление 100 блокнотов отнимает время.

---

## Cache line — единица работы

CPU не работает с отдельными байтами. Минимальная единица — **cache line**, обычно 64 байта (на x86 и большинстве ARM).

Когда программа читает один байт по адресу `X`, CPU вытаскивает **всю** cache line [X aligned to 64, X+63] в L1.

**Последствия:**
- Соседние данные — "бесплатно" в кэше (spatial locality)
- Изменение **любого** байта в линии — это изменение **всей** линии с точки зрения coherence
- Две независимые переменные в одной cache line — становятся "связанными" для cache coherence (см. false sharing)

```go
// Все эти int8 живут в одной 64-байтной cache line
type Hot struct {
    a, b, c, d, e, f, g, h int8  // 8 байт
    // ... ещё много полей
}
```

---

## Проблема coherence

Сценарий с двумя ядрами и переменной `x`:

```
Время  Ядро 0                  Ядро 1
-----  -------------------    -------------------
 t1    read x → cache: x=0
 t2                            read x → cache: x=0
 t3    write x=1
        local cache: x=1
 t4                            read x → cache: x=0 (!!)
```

В момент `t4` ядро 1 читает из **своего** кэша и видит старое значение, хотя ядро 0 уже изменило `x`. Это **catastrophic** — даже простые алгоритмы перестают работать.

Без cache coherence пришлось бы либо **отказаться от per-core cache** (потеря производительности), либо **запрещать программисту полагаться на видимость записей** (что невозможно для concurrent кода).

Решение — hardware-протокол, который **автоматически** держит копии когерентными.

---

## MESI protocol

MESI — самый распространённый протокол cache coherence (Intel, AMD, ARM в основном). Каждая cache line в кэше каждого ядра имеет одно из 4 состояний:

| Состояние | Значит |
|---|---|
| **M (Modified)** | Линия изменена, копия только у меня, в RAM — устаревшая версия |
| **E (Exclusive)** | Копия только у меня, совпадает с RAM, но я её не менял |
| **S (Shared)** | Копия у меня и потенциально у других, совпадает с RAM, только read |
| **I (Invalid)** | Эта запись недействительна, надо перечитать |

### Переходы

```
                          (загрузил, никто больше не имеет)
                   ┌───────────────────────────────┐
                   ↓                               │
                ┌─────┐                       ┌────┴────┐
   write   ┌───→│  E  │──────read─by_other───→│    S    │
       ┌───┘    └─────┘                       └─────────┘
       ↓                                            │
   ┌─────┐                                          │write
   │  M  │←──────────write─────────────────────────┘
   └─────┘                                          ↓
       │                                       (invalidate
       │       other_writes / cache evict        others)
       └─────────────────────────────────────────────┘
                                                     ↓
                                                  ┌─────┐
                                                  │  I  │
                                                  └─────┘
```

### Сценарий пошагово

```
Ядро 0 читает x:
  Ядро 0: x → E (никто больше не имеет, RAM совпадает)

Ядро 1 читает x:
  Ядро 0: x → S
  Ядро 1: x → S
  (оба видят одно и то же, никто не пишет)

Ядро 0 пишет x = 1:
  Ядро 0: посылает invalidate message по шине
  Ядро 1: получает, помечает x → I (мой кэш устарел!)
  Ядро 0: x → M (модифицировано, я единственный владелец)

Ядро 1 читает x:
  Ядро 1: cache miss (I = invalid)
  Ядро 1: запрашивает x по шине
  Ядро 0: видит запрос, отдаёт свою modified копию
  Ядро 0: x → S
  Ядро 1: x → S
  RAM обновляется до x = 1
```

**Ключевой момент:** запись — это **не просто запись в свой cache**. Это сообщение всем остальным ядрам "у меня новая версия, ваши копии invalid". Это требует **inter-core communication** через шину или специальную сеть (на современных CPU — ring bus, mesh).

---

## MESIF, MOESI и другие варианты

Базовая MESI имеет inefficiency: когда ядро A пишет, а потом B читает — данные должны пройти через память. MOESI добавляет состояние **Owned**: ядро держит модифицированную копию, но **другие** тоже могут иметь read-only копии. Делает многоядерный read-heavy workload быстрее.

**Intel:** MESIF (адаптация MESI с состоянием Forward).
**AMD:** MOESI.
**ARM:** разные варианты в зависимости от поколения.

Для практики senior backend это **детали**. Главное — есть какой-то coherence protocol, он работает, но имеет **стоимость**.

---

## Cache coherence traffic — невидимая стоимость

Каждая запись в shared линии вызывает invalidate-сообщение по шине. На современных серверах с 64+ ядрами — это **много трафика** между ядрами.

### Что дорого

| Операция | ~Время на современном x86 |
|---|---|
| Local cache hit (L1) | 1 нс |
| Local cache hit (L2) | 3-10 нс |
| **Cache-to-cache transfer** (L3 same socket) | 20-40 нс |
| **Cross-socket cache transfer** | 100-300 нс |
| Memory access | 100 нс |

То есть **частая запись в shared line** между ядрами — это десятки наносекунд **на каждую запись**. Если 16 ядер бомбят одну shared переменную (например, общий счётчик) — это десятки миллионов раундтрипов в секунду = огромное замедление.

### Lock prefix в x86 / atomic операции

Atomic операции (CAS, fetch-and-add) на x86 — это специальные инструкции с префиксом `LOCK`, которые гарантируют atomic чтение-модификация-запись. Они:

1. Захватывают cache line в состояние M (выбрасывая чужие копии в I)
2. Делают операцию
3. Освобождают (другие ядра могут запросить копию через cache-to-cache)

Это работает быстро (~20-50 нс) **если контеншн низкий**. Если 16 ядер бомбят одну atomic переменную — производительность падает катастрофически из-за coherence trafficа.

См. [04-atomics-and-memory-ordering.md](./04-atomics-and-memory-ordering.md) для детального разбора как `sync/atomic` работает на CPU-уровне.

---

## False sharing

**False sharing** — самый коварный эффект cache coherence. Происходит когда два потока **независимо** работают с **разными** переменными, но эти переменные лежат на **одной cache line**.

С точки зрения программиста — нет shared state, нет contention.
С точки зрения железа — каждая запись в одну переменную **invalidate'ит** копии cache line на ядре, работающем с другой переменной. Огромное coherence traffic. Программа тормозит в разы.

```
┌─────── 64-byte cache line ───────┐
│ counter_a │ counter_b │ ... │ ...│
└──────────────────────────────────┘
      ↑           ↑
   Goroutine 1   Goroutine 2
   обновляет     обновляет
   только это    только это
```

Они "не shared" логически — каждая работает со своей переменной. Но физически они в одной линии, и каждая запись `counter_a++` принудительно делает копию у Goroutine 2 invalid → её следующая запись `counter_b++` начнётся с cache miss → подтянет линию обратно → теперь у Goroutine 1 invalid → и так в цикле.

---

## Демо false sharing в Go

```go
package main

import (
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

const N = 100_000_000

type Counters struct {
    a int64
    b int64
    c int64
    d int64
}

func benchmarkClose() time.Duration {
    var c Counters
    var wg sync.WaitGroup
    wg.Add(4)

    start := time.Now()

    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.a, 1) } }()
    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.b, 1) } }()
    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.c, 1) } }()
    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.d, 1) } }()

    wg.Wait()
    return time.Since(start)
}

type PaddedCounter struct {
    v int64
    _ [56]byte  // 64-8 = 56 байт padding до конца cache line
}

type CountersPadded struct {
    a PaddedCounter
    b PaddedCounter
    c PaddedCounter
    d PaddedCounter
}

func benchmarkPadded() time.Duration {
    var c CountersPadded
    var wg sync.WaitGroup
    wg.Add(4)

    start := time.Now()

    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.a.v, 1) } }()
    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.b.v, 1) } }()
    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.c.v, 1) } }()
    go func() { defer wg.Done(); for i := 0; i < N; i++ { atomic.AddInt64(&c.d.v, 1) } }()

    wg.Wait()
    return time.Since(start)
}

func main() {
    fmt.Println("False sharing (4 counters in one cache line):")
    fmt.Println("  ", benchmarkClose())

    fmt.Println("Padded (each counter on its own cache line):")
    fmt.Println("  ", benchmarkPadded())
}
```

Типичный результат на 4-ядерном x86:

```
False sharing (4 counters in one cache line):
   3.2s
Padded (each counter on its own cache line):
   0.8s
```

**В 4 раза быстрее** — без изменения логики, только за счёт padding'а. Это и есть стоимость cache coherence в чистом виде.

---

## Padding для защиты от false sharing

### Ручной padding

```go
type Counter struct {
    v int64
    _ [56]byte  // 8 байт v + 56 байт padding = 64 байта cache line
}
```

### sync.Mutex и Go runtime

В Go runtime есть internal padding в горячих структурах. Например, в `runtime.mheap` некоторые поля разнесены по cache line специально.

### CacheLinePad (типичный паттерн)

```go
// Готовая структура для padding
type cacheLinePad struct {
    _ [64]byte
}

type Counter struct {
    pad1 cacheLinePad   // защита от соседей слева
    v    int64
    pad2 cacheLinePad   // защита от соседей справа
}
```

Двойной padding защищает от соседей с обеих сторон в массивах счётчиков.

### Когда padding нужен

- **Hot counters** в highly concurrent коде (`atomic.Add` из многих goroutine)
- **Lock-free структуры** (lock-free queue, ring buffer)
- **Per-CPU данные** (метрики, статистика, обновляемые потоками с привязкой к ядру)

### Когда не нужен

- Если не concurrent (один пишет, остальные не трогают)
- Если редкие обновления (false sharing виден только при высокой частоте)
- Если данные **уже** разнесены по разным аллокациям (heap-pointer — обычно ≥64 байта между объектами)

**Не padd'ить превентивно!** Каждый padding = +56 байт памяти. Для горячих данных оправдано, для остальных — bloat.

---

## True sharing — когда оно неизбежно

Иногда несколько ядер **должны** работать с одним значением. Например — глобальный счётчик RPS. Нельзя сделать "у каждого ядра свой" — нужна сумма.

**Решения:**

**1. Mutex-protected counter.**
```go
var mu sync.Mutex
var count int64

mu.Lock()
count++
mu.Unlock()
```
Атомарная блокировка, но при высоком contention — узкое место.

**2. Atomic counter.**
```go
atomic.AddInt64(&count, 1)
```
Быстрее mutex'а, но всё равно cache coherence traffic.

**3. Per-CPU counter с агрегацией.**
```go
// Каждое ядро имеет свой счётчик
counters := make([]int64, runtime.NumCPU())

// Worker увеличивает свой
counters[procID]++   // нет contention

// Когда нужна сумма — итерируем
var total int64
for _, c := range counters {
    total += atomic.LoadInt64(&c)
}
```
Идеально для метрик: счётчик per-CPU обновляется без contention, периодически собираем для отчёта.

**4. Sharding.**
```go
// shard'им по hash от запроса
shard := hashKey(key) % len(shards)
shards[shard].counter++
```
Распределяем contention по shard'ам.

**5. Hierarchical aggregation.**
"Per-CPU → per-NUMA-node → global". Используется в Linux kernel.

---

## NUMA

На больших серверах (2+ CPU socket'а) каждый CPU имеет свою локальную RAM. Доступ к чужой RAM — через **межсокетный link** (Intel UPI, AMD Infinity Fabric).

```
┌── Socket 0 ──┐         ┌── Socket 1 ──┐
│ Cores 0-31   │ ←──────→│ Cores 32-63  │
│  Local RAM   │  cross  │  Local RAM   │
│   (DDR5)     │  socket │   (DDR5)     │
└──────────────┘         └──────────────┘
```

**Latency:**
- Local memory: ~80 нс
- Remote (через socket link): ~150-300 нс

**Cache coherence через сокеты — ещё дороже.** Cache line, "путешествующая" между сокетами при contention — это сотни наносекунд за каждое обращение.

### Практика для backend

- Большинство Go-сервисов работают на одном сокете (8-32 ядра) — NUMA не критично
- Highload-сервисы (Redis, PostgreSQL на больших серверах) часто **pinning'уются** к одному NUMA-узлу:
  ```bash
  numactl --cpunodebind=0 --membind=0 ./service
  ```
- Kubernetes имеет `topology-manager` для NUMA-aware планирования

См. [04-atomics-and-memory-ordering.md](./04-atomics-and-memory-ordering.md) — там обсуждается как cache coherence и memory ordering взаимодействуют.

---

## Практические выводы

**1. Cache coherence — невидимая, но реальная стоимость.**
Если concurrent код тормозит больше чем "сумма работы goroutines" — почти наверняка cache contention. pprof покажет где, но не почему. Анализируй паттерны записи в shared data.

**2. False sharing — частая проблема в hot counters.**
Если несколько goroutine агрессивно обновляют `atomic.AddInt64` на соседних полях структуры — добавь padding. Эффект может быть × 3-5.

**3. Atomic — это не "бесплатно".**
Один atomic на ядро в секунду — норма. Миллион atomic'ов в секунду от множества ядер на одну переменную — серьёзная нагрузка. Думай про per-CPU или sharding.

**4. Cache line aware structure layout.**
Размер структуры в Go выравнивается; учитывай порядок полей. Hot поля — в начало (часто в одной cache line). Cold — в конец.

**5. NUMA pinning — только если знаешь зачем.**
Для большинства сервисов даёт +0%. Для специальных (in-memory БД, ML inference) — заметный выигрыш.

**6. Изучай через bench.**
`go test -bench` с разным числом goroutine, разным padding'ом, разным `GOMAXPROCS`. Реальные числа > интуиции. Cache effects плохо предсказуемы без замеров.

**7. Профилирование cache misses.**
На Linux `perf stat -e cache-misses,cache-references ./binary` покажет долю cache miss'ов. Высокая доля (>10%) на горячем участке — сигнал к оптимизации.

**8. Шаблоны параллельного кода:**
- Read-mostly данные — `atomic.Value`, RWMutex
- Counter под нагрузкой — per-CPU + агрегация
- Hot path — minimal sharing, lock-free где можно
- Cold path — mutex без проблем
