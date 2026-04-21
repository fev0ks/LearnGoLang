# Stack And Heap

Go управляет двумя видами памяти: стеком (быстро, без GC) и heap (медленнее, GC отслеживает). Понимание того, как они устроены на уровне runtime, объясняет почему горутины дешевле потоков и как Go возвращает память OS.

## Содержание

- [Goroutine stack vs OS thread stack](#goroutine-stack-vs-os-thread-stack)
- [Устройство goroutine stack](#устройство-goroutine-stack)
- [Stack growth и shrink](#stack-growth-и-shrink)
- [Go heap: virtual memory и arenas](#go-heap-virtual-memory-и-arenas)
- [Возврат памяти OS: scavenger](#возврат-памяти-os-scavenger)
- [RSS vs VSZ: что видит Kubernetes](#rss-vs-vsz-что-видит-kubernetes)
- [Safe points и stack scanning](#safe-points-и-stack-scanning)
- [Interview-ready answer](#interview-ready-answer)

---

## Goroutine stack vs OS thread stack

```
OS thread stack:       фиксированный размер, обычно 8 MB (задаётся при создании)
Goroutine stack:       начинается с 2 KB, растёт по необходимости
```

Именно поэтому можно держать **миллион горутин**: 1 000 000 × 2 KB = 2 GB (адресного пространства), а не 1 000 000 × 8 MB = 8 TB.

OS-поток при создании резервирует виртуальный адрес сразу (или ядро выделяет физические страницы по demand), и этот размер фиксирован. У горутины стек аллоцируется явно через `malg` и растёт по мере необходимости.

---

## Устройство goroutine stack

Каждая горутина имеет объект `runtime.g`, в котором хранятся:

```
runtime.g {
    stack.lo  uintptr   // нижняя граница стека
    stack.hi  uintptr   // верхняя граница стека
    stackguard0 uintptr // порог для проверки переполнения
    ...
}
```

Стек растёт **вниз** (по убыванию адреса), как обычно для x86-64:

```
stack.hi  ┌──────────────────┐  ← высокий адрес
          │  frame функции A │
          │  (locals, args)  │
          ├──────────────────┤
          │  frame функции B │
          ├──────────────────┤
          │  frame функции C │
          ├──────────────────┤
          │   (свободно)     │
stack.lo  └──────────────────┘  ← stackguard0 чуть выше
```

**Stack frame** содержит:
- локальные переменные функции
- аргументы и возвращаемые значения (в Go передаются через регистры, начиная с Go 1.17, но frame всё равно нужен для spill)
- saved return address
- saved base pointer (для `GOEXPERIMENT=framepointer` или `-msan`)

---

## Stack growth и shrink

### Проверка переполнения (stack check)

Каждая неинлайненная функция начинается с **stack check prologue**:

```asm
; Псевдо-asm: function prologue
MOVQ (TLS), R14          ; загрузить *g (текущая горутина)
CMPQ SP, stackguard0(R14) ; SP < stackguard0 ?
JBE  morestack           ; да → вызвать runtime.morestack
```

`stackguard0 = stack.lo + StackGuard` (StackGuard = 928 bytes). Если SP опустился до guard — нужен рост стека.

### Рост: contiguous copy

До Go 1.4 стек был сегментированным (linked list of stack segments). Начиная с Go 1.4 — **contiguous stack**: весь стек копируется в новый, вдвое больший регион.

```
1. runtime.morestack вызывает newstack()
2. Аллоцируется новый стек: новый_размер = старый_размер × 2
3. Все frame'ы копируются в новую область
4. Все указатели на стековые переменные обновляются (pointer adjustment)
5. Старый стек освобождается
```

**Pointer adjustment** — сложная часть: GC и runtime знают, где в каждом frame лежат указатели (через stack maps), и обновляют их на новые адреса.

Начальный размер: `_StackMin = 2048` (2 KB). Максимум по умолчанию: 1 GB (64-bit), 250 MB (32-bit). Можно изменить через `debug.SetMaxStack`.

### Shrink: уменьшение стека

Стек уменьшается вдвое во время **GC** (в фазе concurrent mark):
- если горутина использует < 25% текущего стека → shrink
- реализация аналогична росту: копирование в меньший буфер

Это предотвращает ситуацию, когда горутина однократно росла до большого стека и больше его не освобождает.

---

## Go heap: virtual memory и arenas

Go не использует `malloc/free` из libc. Он напрямую запрашивает память у OS через `mmap`:

```go
// runtime/mem_linux.go
func sysAlloc(n uintptr, sysStat *sysMemStat) unsafe.Pointer {
    p, err := mmap(nil, n, _PROT_READ|_PROT_WRITE,
        _MAP_ANON|_MAP_PRIVATE, -1, 0)
    // ...
}
```

### heapArena

Heap организован в **arenas** — регионы по `heapArenaBytes`:

```
64-bit Linux:  heapArenaBytes = 64 MB
               heapArena содержит metadata: bitmap для указателей, span table
32-bit:        heapArenaBytes = 4 MB
```

Вся виртуальная память арен организована в `mheap.arenas` — двумерный массив (L1 × L2), что позволяет быстро находить метаданные по любому адресу heap.

### mspan: единица управления

Heap делится на **pages** (1 page = 8 KB). Группы страниц образуют **mspan**:

```
mspan {
    startAddr  uintptr    // начальный адрес
    npages     uintptr    // количество страниц
    sizeclass  uint8      // size class (0 = large object)
    allocBits  *gcBits    // bitmap: какие слоты заняты
    gcmarkBits *gcBits    // bitmap: живые объекты (после GC)
    ...
}
```

Один mspan обслуживает объекты **одного** size class. Это ключевое: нет external fragmentation внутри span.

---

## Возврат памяти OS: scavenger

Go держит освобождённую память у себя (не возвращает OS сразу) — для быстрого переиспользования. Но со временем **scavenger goroutine** возвращает idle страницы:

```
madvise(addr, size, MADV_DONTNEED)  // Linux ≥ Go 1.12 default
// или
madvise(addr, size, MADV_FREE)      // lazy decommit, быстрее
```

- `MADV_DONTNEED`: ядро немедленно освобождает физические страницы, RSS падает
- `MADV_FREE`: страницы помечаются как кандидаты, ядро освобождает при нехватке памяти

`GODEBUG=madvdontneed=1` форсирует `MADV_DONTNEED` если по умолчанию используется `MADV_FREE`.

Scavenger бюджетирует CPU: использует не более 1% CPU для возврата памяти, чтобы не мешать приложению.

**GOMEMLIMIT** (Go 1.19) влияет на scavenger: при приближении к лимиту scavenger становится агрессивным.

---

## RSS vs VSZ: что видит Kubernetes

```
VmSize (VSZ) = всё виртуальное адресное пространство (зарезервировано)
VmRSS (RSS)  = реально отображённые физические страницы
```

После `MADV_DONTNEED`: RSS падает, VSZ остаётся.  
После `MADV_FREE`: RSS не падает сразу (ядро решает сам), VSZ остаётся.

Kubernetes смотрит на RSS (`container_memory_working_set_bytes` в cgroup v2 = RSS + kernel). Поэтому:
- `GOMEMLIMIT` должен быть < cgroup memory.limit (обычно 90%)
- без `GOMEMLIMIT` Go может увеличивать RSS до OOM killer

```bash
# Посмотреть memory процесса
cat /proc/$(pgrep myapp)/status | grep -E 'VmRSS|VmSize|VmPeak'
```

---

## Safe points и stack scanning

GC должен сканировать goroutine stacks (найти указатели как GC roots). Это требует, чтобы горутина стояла в **safe point** — состоянии, когда известно где именно в стеке лежат указатели.

**Safe points** в Go:
- вызов функции (preamble генерирует stack map)
- `runtime.Gosched()` — явная уступка
- syscall (горутина паркуется)
- async preemption signal (SIGURG, Go 1.14+) → горутина переводится в safe point при следующей инструкции

Для каждой функции компилятор генерирует **stack map** — bitmap того, какие слоты стека содержат указатели. GC использует её при сканировании.

```
stack frame:
  [0]: int64    → не указатель → skip
  [1]: *User    → указатель   → добавить в grey set
  [2]: string   → содержит ptr внутри (data pointer)
  [3]: []int    → slice header содержит ptr
```

---

## Interview-ready answer

**"Чем goroutine stack отличается от OS thread stack?"**

OS thread имеет фиксированный стек при создании (обычно 8 MB), который не уменьшается. Goroutine начинается с 2 KB и использует **contiguous stack**: при переполнении весь стек копируется в новый регион вдвое большего размера, все указатели на стековые переменные обновляются. При GC стек может уменьшиться вдвое, если использовано < 25%. Это и позволяет держать миллионы горутин.

**"Как Go управляет памятью heap?"**

Go напрямую запрашивает память у OS через `mmap` анонимными маппингами. Heap делится на `heapArena` (64 MB на 64-bit), каждая с bitmap'ами для GC. Внутри arena память делится на `mspan` — группы страниц (1 page = 8 KB) для объектов одного size class. Освобождённая память возвращается OS через `madvise(MADV_DONTNEED)` scavenger-горутиной. Kubernetes видит RSS — поэтому нужен `GOMEMLIMIT = 90% × cgroup limit`.
