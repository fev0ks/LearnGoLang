# Atomics и memory ordering

Когда несколько ядер CPU работают с одной памятью, всё что ты "знаешь" о порядке операций — **неправда**. CPU переупорядочивает инструкции для производительности. Компилятор переупорядочивает инструкции для производительности. И результат может удивить даже опытного разработчика.

Этот файл — про то как **на самом деле** работают atomic операции и memory ordering на уровне процессора. Поняв это, ты увидишь что `sync/atomic` в Go — не магия, а тонкая работа с конкретными hardware-инструкциями. И увидишь почему в коде ниже возможен результат `(0, 0)`.

## Содержание

- [Мотивирующий пример: невозможное (0, 0)](#мотивирующий-пример-невозможное-0-0)
- [Store buffer — корень зла](#store-buffer-корень-зла)
- [Как объясняется (0, 0)](#как-объясняется-0-0)
- [Виды memory reordering](#виды-memory-reordering)
- [Модели памяти: x86 TSO, ARM weak, RISC-V](#модели-памяти-x86-tso-arm-weak-risc-v)
- [Memory fences](#memory-fences)
- [Atomic операции на уровне CPU](#atomic-операции-на-уровне-cpu)
- [LOCK prefix на x86](#lock-prefix-на-x86)
- [Compare-and-Swap (CAS)](#compare-and-swap-cas)
- [Acquire/Release semantics](#acquirerelease-semantics)
- [sync/atomic в Go](#syncatomic-в-go)
- [Как Mutex использует atomics](#как-mutex-использует-atomics)
- [Go memory model](#go-memory-model)
- [Практические выводы](#практические-выводы)

---

## Мотивирующий пример: невозможное (0, 0)

```go
package main

import (
    "fmt"
    "sync"
)

func main() {
    results := make(map[[2]int]int)

    for i := 0; i < 10_000_000; i++ {
        var x, y, r1, r2 int64
        wg := new(sync.WaitGroup)
        wg.Add(2)

        go func() {
            defer wg.Done()
            x = 1
            r1 = y
        }()

        go func() {
            defer wg.Done()
            y = 1
            r2 = x
        }()

        wg.Wait()
        results[[2]int{int(r1), int(r2)}]++
    }

    for k, v := range results {
        fmt.Printf("(%d, %d): %d times\n", k[0], k[1], v)
    }
}
```

Что мы ожидаем увидеть? Поток 1 делает `x = 1` потом читает `y`. Поток 2 делает `y = 1` потом читает `x`. Возможные варианты:

- `(1, 1)` — оба потока выполнили запись до того как другой прочитал → оба видят 1
- `(0, 1)` — поток 2 прочитал x до того как поток 1 успел записать x=1
- `(1, 0)` — поток 1 прочитал y до того как поток 2 успел записать y=1

А `(0, 0)`? Это значит:
- `r1 == 0` → поток 1 прочитал y **раньше** чем поток 2 записал y. Но поток 1 ДО чтения y записал x=1. Значит к моменту когда поток 2 будет читать x — там должно быть 1.
- `r2 == 0` → но поток 2 прочитал x как 0. Значит он прочитал x **раньше** чем поток 1 записал x=1.

Получается противоречие: каждый поток прочитал ДО того как другой записал. Это **невозможно** в последовательной модели.

**Реальный результат** (на x86):

```
(1, 0): 9896564 times
(0, 1): 100218 times
(0, 0): 3217 times    ← вот оно
(1, 1): 1 times
```

`(0, 0)` встречается **3000 раз из 10 миллионов**. Это не баг компилятора, не баг Go, не баг твоего кода. Это **физика процессора**.

---

## Store buffer — корень зла

Современный CPU не пишет в L1 cache мгновенно. У каждого ядра есть **store buffer** — маленькая FIFO-очередь записей перед попаданием в cache:

```
Core 0:
   write x=1 ──→ [store buffer] ──→ L1 cache ──→ visible to Core 1
                  (несколько циклов задержки)
```

**Зачем:** запись в cache — медленнее чем выполнение инструкции. Store buffer позволяет CPU не ждать — он "отложил" запись и продолжил работать. Когда есть свободный цикл, контроллер flush'ит buffer в cache.

**Свой store buffer виден только своему ядру.** Если Core 0 положил `x=1` в свой store buffer и тут же читает `x` — он увидит 1 (CPU умеет читать из своего store buffer, это называется **store-to-load forwarding**). Но **Core 1 не видит** этой записи, пока она не дойдёт до cache и не отинвалидировано Core 1.

---

## Как объясняется (0, 0)

Возвращаемся к нашему примеру:

```
Поток 1:           Поток 2:
x = 1              y = 1
r1 = y             r2 = x
```

С учётом store buffer:

```
t=1: Core 0: write x=1 → store buffer Core 0 (не в cache!)
t=2: Core 1: write y=1 → store buffer Core 1 (не в cache!)
t=3: Core 0: read y → cache → 0 (Core 1 ещё не flush'нул свою запись)
t=4: Core 1: read x → cache → 0 (Core 0 ещё не flush'нул свою запись)
t=5: Core 0: store buffer flush, x=1 в cache, инвалидация в Core 1
t=6: Core 1: store buffer flush, y=1 в cache, инвалидация в Core 0
```

В моменты t=3 и t=4 обе записи "застряли" в store buffer'ах. Каждое ядро записало в свой buffer но прочитало через cache (где запись ещё не отразилась). Результат — оба прочитали 0.

Это называется **StoreLoad reordering** — операция Store "обогнала" операцию Load **с точки зрения других ядер**. Локально (внутри ядра) порядок сохранён — ядро 0 само видит свой `x=1` через forwarding. Но Core 1 видит сначала "ничего" (для x), и только потом 1.

---

## Виды memory reordering

Программа имеет 4 типа операций с памятью и для каждой пары — возможность переупорядочивания **со стороны других ядер**:

| Тип переупорядочивания | Что значит |
|---|---|
| **LoadLoad** | Load A; Load B — B видится первым |
| **LoadStore** | Load A; Store B — Store видится первым |
| **StoreLoad** | Store A; Load B — Load видится первым (тот самый случай) |
| **StoreStore** | Store A; Store B — Store B видится первым |

Разные процессоры разрешают разные виды переупорядочивания. Чем больше разрешено — тем агрессивнее можно оптимизировать, но тем сложнее писать correct concurrent code.

---

## Модели памяти: x86 TSO, ARM weak, RISC-V

### x86 — Total Store Order (TSO)

Strict-ish модель. Гарантии:
- ✅ **LoadLoad** — НЕ переупорядочивается
- ✅ **LoadStore** — НЕ переупорядочивается
- ❌ **StoreLoad** — **ПЕРЕУПОРЯДОЧИВАЕТСЯ** (это наш пример)
- ✅ **StoreStore** — НЕ переупорядочивается

x86 разрешает только один вид reordering — StoreLoad. Это делает его "относительно дружественным" процессором для concurrent кода: большинство интуиций работают. Но не все.

### ARM, PowerPC — weakly ordered

Разрешают **все** виды переупорядочивания. Без явных fence'ов даже простой счётчик может быть прочитан в неожиданном порядке между ядрами.

ARM-сервера (AWS Graviton, Apple silicon) — это значит код, который **работает** на x86 в multi-threaded режиме, на ARM **может сломаться**.

```go
// На x86 этот код может работать "случайно"
// На ARM почти гарантированно сломается без proper atomic
var ready bool
var data int

// Producer
data = 42
ready = true

// Consumer
if ready {
    fmt.Println(data)  // на ARM может прочитать 0!
}
```

Это потому что на ARM `data = 42` и `ready = true` могут "переехать" в любом порядке с точки зрения consumer'а.

### RISC-V — RVWMO

Конфигурируемая модель, чуть строже ARM. Зависит от платформы.

### Что это значит для Go разработчика

**Go runtime использует atomics с правильными fence'ами для своей платформы.** Если ты используешь `sync.Mutex`, `sync/atomic`, channels — Go скрывает разницу платформ. Твой код будет корректным и на x86, и на ARM.

**Но если ты делаешь ad-hoc concurrency без synchronisation** — например, "просто читаю переменную из другой goroutine без atomic" — ты получишь поведение в зависимости от модели памяти CPU. На x86 это часто **работает случайно**. На ARM — **скорее всего сломается**.

Поэтому золотое правило Go: **не делай data race**. Race detector (`-race`) — твой друг.

---

## Memory fences

**Fence (barrier)** — специальная CPU-инструкция, которая запрещает переупорядочивание через неё.

```
Store A
FENCE          ← все Store/Load до fence завершатся до того, что идёт после
Load B
```

На x86:

| Инструкция | Что блокирует |
|---|---|
| **LFENCE** | LoadLoad и LoadStore — все loads до завершатся до loads после |
| **SFENCE** | StoreStore — все stores до завершатся до stores после |
| **MFENCE** | Всё — full memory barrier, drain'ит store buffer |

`MFENCE` — самый дорогой (~30-50 нс на современных Intel). Drain'ит store buffer **до конца** — теперь все записи видны другим ядрам.

Если в наш пример вставить MFENCE между записью и чтением:

```
Поток 1: x = 1; MFENCE; r1 = y
Поток 2: y = 1; MFENCE; r2 = x
```

`(0, 0)` стало бы **невозможным**, потому что MFENCE дренирует store buffer перед чтением.

На ARM соответствующие инструкции — `DMB`, `DSB`, `ISB` с различными domain.

### Зачем это знать backend-разработчику

Прямые fence-инструкции — это редкость. Их можно встретить в:
- Lock-free структурах данных в стандартной библиотеке
- Реализациях `sync.Mutex`, `atomic.*`
- Низкоуровневых библиотеках (например, lock-free очереди типа `golang.org/x/sync`)

Понимание fence'ов нужно чтобы **читать** такой код. Писать его в обычном backend не нужно — используй `sync/atomic`, который инкапсулирует fence'ы.

---

## Atomic операции на уровне CPU

"Atomic" означает: **между чтением и записью никто не может вмешаться**. Операция выглядит "как точка" для других ядер.

Простой `counter++` на x86 — **не atomic**:

```assembly
MOV RAX, [counter]   ; ← чтение
ADD RAX, 1            ; ← инкремент в регистре
MOV [counter], RAX   ; ← запись
```

Три инструкции. Между чтением и записью другое ядро может изменить `counter` — твой результат запишет старое значение + 1, "потеряв" чужое изменение. Классический lost update.

Atomic версия должна сделать read-modify-write **за одну неделимую операцию**.

---

## LOCK prefix на x86

x86 имеет специальный префикс `LOCK` для instructions, делающий их atomic:

```assembly
LOCK ADD [counter], 1   ; atomic increment
LOCK XCHG [val], RAX    ; atomic swap
LOCK CMPXCHG [val], RBX ; atomic compare-and-swap
```

**Что делает LOCK на старых CPU (до Pentium Pro):** буквально блокировал шину памяти. Никто не мог пользоваться шиной во время операции. Это было дорого.

**Что делает на современных CPU:** через cache coherence захватывает cache line в Modified state (выкидывает копии у других ядер), делает операцию, освобождает. Намного эффективнее, но всё равно платно.

**Стоимость:** ~20-50 нс на современных Intel при низком contention. При высоком contention (много ядер бьются за одну линию) — до сотен наносекунд.

---

## Compare-and-Swap (CAS)

Самая важная atomic операция — **compare-and-swap**:

```
CAS(addr, expected, new):
   atomically:
      if *addr == expected:
         *addr = new
         return true
      else:
         return false
```

"Если значение по адресу всё ещё `expected` — замени на `new`. Если кто-то другой уже изменил — не трогай, верни fail".

Это фундамент **lock-free программирования**. Любую операцию можно построить:

```go
// Atomic increment через CAS
for {
    old := atomic.LoadInt64(&counter)
    new := old + 1
    if atomic.CompareAndSwapInt64(&counter, old, new) {
        break
    }
    // Если CAS не удался — кто-то другой изменил counter, пробуем заново
}
```

На x86 — инструкция `LOCK CMPXCHG`. Один из самых частых atomic'ов в lock-free коде.

### ABA problem

CAS гарантирует "значение не менялось" — но что если оно поменялось туда-обратно? Был A, стал B, потом снова A. CAS думает что ничего не менялось.

В реальности обычно проблема при работе с указателями: пойнтер на старый объект → удалён → создан новый объект с тем же адресом → CAS не замечает.

Решение — **versioned pointer** или **hazard pointers**. В Go-коде с GC проблема ABA встречается реже потому что GC не освобождает память пока есть ссылки.

---

## Acquire/Release semantics

Не все atomic нужно "полное" упорядочивание. Большинство хороши с **acquire/release** semantics:

**Acquire (для load):** "после этого load все последующие операции **остаются** после". Можно reorder инструкции **снизу вверх**, но не наоборот.

**Release (для store):** "перед этим store все предыдущие операции **остаются** до". Можно reorder инструкции **сверху вниз**, но не наоборот.

Типичный паттерн:

```
Producer:                     Consumer:
data = 42                     while !ready.Load() {}  ← acquire
ready.Store(true)  ← release    use(data)
```

Release-store гарантирует что `data = 42` **видно** до того как ready=true стало видно. Acquire-load гарантирует что `use(data)` **не пере-ordered** до проверки ready.

На x86 каждый load автоматически "acquire" и каждый store — "release" (потому что TSO). На ARM нужны явные `ldar`/`stlr` инструкции.

### Sequential consistency (seq_cst)

Самая сильная семантика — все операции выстраиваются в **глобальный** порядок, видимый всем ядрам одинаково. Для большинства случаев избыточно и дорого.

Это то что даёт **MFENCE** на x86 — full seq_cst.

---

## sync/atomic в Go

`sync/atomic` — мост между Go-кодом и hardware atomic.

```go
import "sync/atomic"

var counter atomic.Int64

// Atomic increment
counter.Add(1)         // на x86: LOCK XADD

// Atomic load
val := counter.Load()  // на x86: обычный MOV (load уже acquire)

// Atomic store
counter.Store(42)      // на x86: XCHG или MOV+MFENCE

// Compare-and-swap
swapped := counter.CompareAndSwap(old, new)  // LOCK CMPXCHG

// Atomic swap
prev := counter.Swap(0)
```

### Что НЕ atomic

```go
// НЕ atomic, может быть data race
var counter int64
counter++  // 3 инструкции, прерываемые
```

Race detector найдёт:
```bash
go test -race
```

### Типы в новом API (Go 1.19+)

```go
var counter atomic.Int64       // вместо atomic.AddInt64(&counter, 1)
var pointer atomic.Pointer[T]  // type-safe atomic pointer
var b atomic.Bool              // atomic boolean
```

Старый API (`atomic.AddInt64(&x, ...)`) тоже работает, но новый удобнее и типизированнее.

### Memory ordering в Go atomics

В Go все atomic операции **по умолчанию sequentially consistent**. То есть самая сильная гарантия. Это упрощает программирование (не надо думать про acquire/release), ценой небольшой потери производительности.

В отличие от C++/Rust, где atomic может быть с разными memory orders (relaxed, acquire, release, acq_rel, seq_cst), Go даёт только один — самый сильный.

Подробнее: [01-go-core/06-memory-model.md](../../01-go-core/06-memory-model.md).

---

## Как Mutex использует atomics

`sync.Mutex` в Go — построен на atomic CAS:

```go
// Упрощённая реализация
type Mutex struct {
    state int32
}

func (m *Mutex) Lock() {
    // Fast path — попытка через atomic CAS
    if atomic.CompareAndSwapInt32(&m.state, 0, 1) {
        return  // Lock acquired
    }
    // Slow path — кто-то держит lock, ждём в очереди (sema)
    m.lockSlow()
}

func (m *Mutex) Unlock() {
    // Atomic decrement
    new := atomic.AddInt32(&m.state, -1)
    if new != 0 {
        // Кто-то ждёт — разбудить
        m.unlockSlow(new)
    }
}
```

**Без contention** — Lock/Unlock работают за один CAS каждый = ~20-50 нс. Очень дёшево.

**При contention** — goroutine уходит в sleep через `runtime.semacquire`, runtime пробуждает её когда lock освобождается. Это дороже — десятки микросекунд.

Поэтому в Go "Mutex дёшев когда не contention". В горячем цикле с агрессивной конкуренцией mutex становится bottleneck.

Реальная реализация в Go — сложнее (`runtime/mutex.go`), включает spinning, fair queueing, и т.д. См. [09-concurrency-and-performance/02-sync-primitives.md](../../09-concurrency-and-performance/02-sync-primitives.md).

---

## Go memory model

Go memory model — это **контракт** между runtime'ом и тобой о том какие гарантии есть для concurrent code.

Главное правило: **если есть data race — поведение undefined**. Гарантии есть только для синхронизированных доступов.

Что считается synchronisation:
- `sync.Mutex`, `sync.RWMutex`
- `sync/atomic` операции
- Send/receive по channel
- `sync.Once`
- `sync.WaitGroup`
- Запуск/завершение goroutine

Если две goroutine читают/пишут общее без чего-то из этого — это data race, и даже на x86 результат может удивить (компилятор может переупорядочить, может удерживать в регистре, и т.д.).

### Happens-before

Формальная модель называется **happens-before**: если событие A "happens-before" события B, то B видит эффекты A. Иначе — может видеть, может не видеть.

```go
var x int
var done bool

// Goroutine A
x = 42        // (1)
done = true   // (2) — не synchronized

// Goroutine B
if done {     // (3) — может прочитать true но видеть x=0
    print(x)
}
```

Здесь (1) не happens-before (3) потому что нет synchronisation. Это data race.

Правильно:

```go
var x int
var done atomic.Bool

// Goroutine A
x = 42                  // (1)
done.Store(true)        // (2) — atomic store

// Goroutine B
if done.Load() {        // (3) — atomic load
    print(x)            // ✓ гарантированно увидит 42
}
```

Сейчас (1) happens-before (3) через atomic synchronisation.

Подробнее — [Go Memory Model](https://go.dev/ref/mem) и [01-go-core/06-memory-model.md](../../01-go-core/06-memory-model.md).

---

## Практические выводы

**1. Любой concurrent доступ к shared переменной — через synchronisation.**
Mutex, atomic, channel — выбирай по задаче, но НЕ делай "наивно".

**2. `go test -race` — must-have в CI.**
Большинство data race'ов на x86 кажутся работающими. Race detector найдёт их детерминированно. Без него код может годами работать "случайно" и сломаться на ARM-сервере.

**3. `sync.Mutex` — нормально для большинства случаев.**
"Mutex медленный" — миф. Без contention — 20-50 нс. Используй пока pprof не покажет contention bottleneck.

**4. Atomic — для simple shared state.**
Counters, флаги, single pointers. Не пытайся строить сложные структуры данных через atomics руками без серьёзного опыта lock-free programming.

**5. Channels — для передачи владения.**
Когда смысл "одна goroutine закончила работу, другая её забирает" — channel идиоматичнее mutex'а.

**6. Cross-platform behavior.**
Если разрабатываешь на Mac (ARM) или собираешься деплоить на Graviton — race detector обязателен. Тестируй на той же архитектуре что и production.

**7. Знание hardware помогает читать low-level код.**
`runtime` package, sync primitives, lock-free структуры — все используют memory ordering. Понимая acquire/release/seq_cst — читаешь их код, не магия.

**8. Не оптимизируй преждевременно через atomics.**
"Заменим mutex на atomic — будет быстрее" — часто **не быстрее**, и **некорректно**. Mutex использует atomic внутри. Прямые atomics нужны для специфических задач (counters, flags), не для замены mutex.

---

## Дополнительно: спецификации

- [Intel Software Developer's Manual, Volume 3A, Chapter 8](https://www.intel.com/sdm) — memory ordering на x86
- [ARM Architecture Reference Manual, Memory Model](https://developer.arm.com/documentation/102376/) — для ARM
- [Go Memory Model](https://go.dev/ref/mem) — что гарантирует Go
- ["Memory Barriers: a Hardware View for Software Hackers"](https://www.rdrop.com/users/paulmck/scalability/paper/whymb.2010.06.07c.pdf) — Paul McKenney, классика
- [C++ memory_order на cppreference](https://en.cppreference.com/w/cpp/atomic/memory_order) — детальное описание acquire/release/seq_cst (релевантно для понимания)
