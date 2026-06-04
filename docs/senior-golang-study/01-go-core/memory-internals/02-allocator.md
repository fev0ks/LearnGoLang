# Go Memory Allocator

Go использует собственный аллокатор поверх OS-страниц. Его архитектура — трёхуровневая иерархия — позволяет аллоцировать объекты почти без contention на многопроцессорной системе.

## Содержание

- [Трёхуровневая иерархия](#трёхуровневая-иерархия)
- [Size classes](#size-classes)
- [mcache — per-P кэш](#mcache--per-p-кэш)
- [mcentral — per-size-class список](#mcentral--per-size-class-список)
- [mheap — глобальный менеджер](#mheap--глобальный-менеджер)
- [Tiny allocator](#tiny-allocator)
- [Large objects (> 32 KB)](#large-objects--32-kb)
- [noscan: объекты без указателей](#noscan-объекты-без-указателей)
- [Путь аллокации: пошагово](#путь-аллокации-пошагово)
- [Zero initialization](#zero-initialization)
- [Инструменты диагностики](#инструменты-диагностики)
- [Interview-ready answer](#interview-ready-answer)

---

## Трёхуровневая иерархия

Аллокатор Go устроен по тому же принципу, что и кэши процессора (L1 → L2 → RAM) или TCMalloc, от которого он и произошёл: **горячий путь — локальный и без блокировок, а за общими ресурсами идём всё реже и всё дороже**. Чем выше уровень, тем больше contention.

```mermaid
flowchart TD
    A["mcache (per-P)<br/>БЕЗ блокировок"] -->|"span пуст"| B["mcentral (per-size-class)<br/>fine-grained lock"]
    B -->|"нет свободных span"| C["mheap (глобальный)<br/>coarse lock на весь heap"]
    C -->|"нет страниц"| D["OS<br/>mmap"]

    style A fill:#e8f5e8,stroke:#4caf50,color:#1b5e20
    style B fill:#fff8e1,stroke:#f9a825,color:#6d4c00
    style C fill:#ffebee,stroke:#e53935,color:#b71c1c
    style D fill:#eceff1,stroke:#607d8b,color:#263238
```

| Уровень | Сколько штук | Кто владеет | Общий? | Блокировка | Как часто доходим |
|---|---|---|---|---|---|
| **mcache** | `GOMAXPROCS` (по одному на P) | каждый **P** | нет, локальный | нет (lock-free) | почти всегда |
| **mcentral** | ~136 (size classes × 2) | живут внутри mheap | да, но лок на каждый class | mutex на этот class | когда span в mcache кончился |
| **mheap** | **1 на процесс** | весь процесс | да, глобальный | глобальный `mheap.lock` | когда у mcentral нет span; large objects (> 32 KB) — сразу сюда |
| **OS (mmap)** | — | OS-ядро | — | syscall | когда heap надо расширить |

Большинство аллокаций разрешается на уровне **mcache** — без блокировок, потому что каждый P владеет своим кэшем, и только один M (OS-поток) работает с P в каждый момент. Это и есть причина, почему аллокация в Go при `GOMAXPROCS=N` масштабируется почти линейно: N процессоров аллоцируют параллельно, не мешая друг другу, пока хватает локальных span.

> **Важный нюанс: mcache — это per-P, а не per-goroutine.** Своего кэша аллокатора у горутины нет. Горутина исполняется на каком-то P (через M) и аллоцирует из mcache *этого* P. Поскольку на одном P в каждый момент работает только одна горутина — доступ к mcache не нужно защищать блокировкой. Тысячи горутин делят `GOMAXPROCS` кэшей, а самый верхний уровень — `mheap` — **один на весь процесс** (отсюда и contention на больших аллокациях, которые идут прямо в него).

---

## Size classes

Объекты размером ≤ 32 KB распределяются по **size classes** — заранее известным категориям размеров.

Go имеет **~70 size classes** (точное число меняется между версиями):

```
class  bytes     class  bytes     class  bytes
  1      8         11    128         21    1792
  2     16         12    144         22    2048
  3     24         13    160         23    2304
  4     32         14    176         24    2688
  5     48         15    192         25    3072
  6     64         16    208         26    3200
  7     80         17    224         27    3456
  8     96         18    240         28    4096
  9    112         19    256         ...
 10    128         20    288         67   32768
```

При аллокации запрос **округляется вверх** до ближайшего size class. Объект 100 байт попадёт в class 9 (112 байт) → 12 байт пропадают. Это **внутренняя фрагментация** (internal fragmentation) — плата за то, что не нужен заголовок перед каждым объектом и за O(1) аллокацию.

Несколько примеров, сколько теряется (значения классов — реальные из Go):

| Запрос | Округлено до | Потеря | % overhead |
|---:|---:|---:|---:|
| 8 B | 8 B | 0 B | 0% |
| 17 B | 24 B | 7 B | 41% |
| 100 B | 112 B | 12 B | 12% |
| 129 B | 144 B | 15 B | 12% |
| 769 B | 896 B | 127 B | 17% |
| 3100 B | 3200 B | 100 B | 3% |

Worst case — чуть-чуть «перевалить» за границу класса (как 17 B → 24 B, +41%). Поэтому если в hot path много объектов одного размера, иногда стоит подогнать структуру/буфер под границу size class. Проверить, в какой класс попадёт размер, можно по таблице в `runtime/sizeclasses.go` или экспериментом с `-benchmem`.

```go
// Структура весит 33 байта → округлится до 48 B, теряем 15 B на объект.
// Уберём/упакуем поля до 32 B → ровно в класс 32 B, потеря 0 B.
// На 10M объектов это ~150 MB сэкономленного heap.
```

**Зачем size classes:**
- Нет external fragmentation: span содержит объекты только одного размера, дырки заполняются объектами того же размера;
- Allocation = bump pointer внутри span (O(1));
- GC sweep: всю span можно переиспользовать или вернуть целиком.

```go
// runtime/sizeclasses.go (упрощённо)
var class_to_size = [_NumSizeClasses]uint16{
    0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, ...
}
```

### Сколько страниц в span (span ≠ 8 KB!)

Частая путаница: «`mspan` — это 8 KB, как же тогда хранятся объекты в 12 или 32 KB?». **8 KB — это размер страницы (page), а не span.** `mspan` — это **группа из N подряд идущих страниц**, и N **разное для каждого size class**.

Для каждого класса runtime заранее (в `sizeclasses.go`) подбирает число страниц так, чтобы в span влезало **целое число** объектов с минимальным хвостовым остатком (tail waste):

| Объект | span = N страниц | размер span | объектов в span |
|---:|---:|---:|---:|
| 8 B | 1 (8 KB) | 8 KB | 1024 |
| 1 KB | 1 (8 KB) | 8 KB | 8 |
| **12 KB** | **3 (24 KB)** | 24 KB | 2 |
| **32 KB** | **4 (32 KB)** | 32 KB | 1 |

Ключевой момент: объект 12 KB **физически не влезает в одну 8 KB страницу**, поэтому его span = 3 страницы (24 KB) и держит 2 объекта без потерь (`24 / 12 = 2`). Самый большой small-class, 32 KB, использует span из 4 страниц и хранит ровно 1 объект.

```
size class 12 KB → span из 3 страниц (24 KB):
┌────────┬────────┬────────┐
│ page 0 │ page 1 │ page 2 │   ← 3 смежные страницы = один mspan
├────────┴───┬────┴────────┤
│  объект 0  │  объект 1   │   ← 2 объекта по 12 KB, хвоста нет
└────────────┴─────────────┘
```

Итого: мелкие классы → span в 1 страницу с кучей объектов; крупные (до 32 KB) → span в несколько страниц с малым числом объектов. Поэтому `alloc [numSpanClasses]*mspan` в mcache — это «один span на класс», но **сам span у каждого класса своей длины** в страницах.

---

## mcache — per-P кэш

Каждый **P** (logical processor) имеет объект `mcache`:

```
mcache {
    alloc [numSpanClasses]*mspan   // по одному span на каждый size class
                                   // × 2: scan + noscan варианты
    tiny       uintptr             // tiny allocator: текущий блок
    tinyoffset uintptr
    tinyAllocs uintptr
}
```

`numSpanClasses = _NumSizeClasses × 2` — для каждого size class есть два span: один для объектов с указателями (scan), один без (noscan).

В `alloc` — целочисленный индекс `spanClass`, склеенный из номера размера и одного бита scan/noscan:

```
spanClass = sizeclass << 1 | noscanBit
                                 └─ 0 = есть указатели (scan)
                                    1 = указателей нет (noscan)
```

Поэтому каждый size class занимает **две подряд идущие ячейки** массива (чётная — scan, нечётная — noscan):

```
индекс (spanClass)  size class   scan/noscan
   4                   2 (16 B)    scan      ← struct{ V int; Next *Node }
   5                   2 (16 B)    noscan    ← struct{ X, Y float64 }
   6                   3 (24 B)    scan
   7                   3 (24 B)    noscan
```

Обратное извлечение: `sizeclass = spanClass >> 1`, `noscan = spanClass & 1`.

**Аллокация из mcache:**

```
1. size class по размеру + бит scan/noscan по типу → spanClass
2. Взять mspan из mcache.alloc[spanClass]
3. Если span не пуст: взять следующий свободный слот (freeindex)
4. Обновить freeindex, вернуть указатель
```

Конкретно: пусть нужен объект 100 B (size class → 112 B). Span для этого класса — это, скажем, 8 KB страница, нарезанная на `8192 / 112 ≈ 73` слота. `freeindex` указывает на первый свободный слот:

```
span (112-байтные слоты),  freeindex = 3
┌──────┬──────┬──────┬──────┬──────┬─────┐
│ slot0│ slot1│ slot2│ slot3│ slot4│ ... │
│ заня│ заня│ заня│ ↑своб│ своб │     │
└──────┴──────┴──────┴──────┴──────┴─────┘
                       │
 аллокация: addr = span.base + freeindex×112
            freeindex++   (теперь 4)
```

Аллокация = арифметика над `freeindex` + возврат адреса. Никаких блокировок, никакого поиска — отсюда и скорость. Когда `freeindex` дойдёт до конца span (все 73 слота заняты), mcache идёт за новым span в mcentral.

> На самом деле freeindex работает в паре с `allocCache` — битовой маской свободных слотов (для переиспользования дырок после GC sweep), но идея та же: найти свободный слот почти бесплатно.

Это быстро: нет блокировок, нет обращений к общим структурам. При context switch P переходит к другому M, но mcache переносится вместе с P.

---

## mcentral — per-size-class список

Когда mcache span исчерпан, запрашивается новый span из `mcentral`:

```
mheap.central[sizeclass] = mcentral {
    lock   mutex
    partial [2]spanSet   // spans с свободными слотами
    full    [2]spanSet   // spans без свободных слотов
}
```

`[2]` — два набора: swept (после GC) и unswept (до GC). GC lazy sweep происходит при аллокации.

Взять span из mcentral = захватить lock, вернуть span в mcache (spin-free в большинстве случаев).

---

## mheap — глобальный менеджер

Если mcentral не может дать span — обращение к `mheap`:

```
mheap {
    lock      mutex
    pages     pageAlloc     // bitmap всех heap-страниц
    arenas    [arenaL1]*[arenaL2]*heapArena
    central   [numSpanClasses]mcentral
    ...
}
```

mheap находит подходящий contiguous range страниц через `pageAlloc` (radix tree), нарезает их в новый `mspan` и передаёт в mcentral.

Если heap-страниц не хватает — mheap запрашивает у OS через `mmap`.

---

## Tiny allocator

Отдельная оптимизация для очень маленьких объектов (≤ 16 bytes) **без указателей**:

```go
// runtime/malloc.go
if size <= maxTinySize && noscan {
    // Попытаться упаковать в текущий tiny block
    off := c.tinyoffset
    if off+size <= maxTinySize && c.tiny != 0 {
        x = unsafe.Pointer(c.tiny + off)
        c.tinyoffset = off + size
        return x
    }
    // Аллоцировать новый tiny block (16 bytes из size class 2)
    ...
}
```

**Tiny block = 16 байт** из size class 2 (16 bytes). В один такой блок может упасть несколько маленьких объектов:

```
Tiny block (16 bytes):
[  int32(4B)  ][  bool(1B)  ][  pad(3B)  ][  int64(8B)  ]
```

Типичные кандидаты: `bool`, `byte`, `int8/16/32`, `float32`, маленькие structs без pointer полей.

Если объект содержит указатели — tiny allocator не применяется (GC должен знать границы объектов для сканирования).

---

## Large objects (> 32 KB)

Объекты > 32 KB обходят mcache и mcentral, идут напрямую в mheap:

```go
if size > maxSmallSize {
    span = mheap_.allocLarge(npages)
    // каждый большой объект получает свой собственный span
}
```

Каждый большой объект = свой `mspan` с `sizeclass = 0`. GC сканирует его как единое целое.

**Важно:** большие аллокации всегда идут через global lock `mheap.lock`. Это дороже, чем small allocations. Паттерн частых больших аллокаций на hot path — потенциальная точка contention.

---

## noscan: объекты без указателей

Каждый size class существует в двух вариантах: scan и noscan.

Если объект не содержит указателей — он попадает в **noscan** span:

```go
type Point struct {
    X, Y float64  // нет указателей → noscan
}

type User struct {
    ID    int64
    Name  string  // string содержит указатель на данные → scan
}
```

GC **не сканирует** noscan объекты в phase mark — внутри них гарантированно нет указателей, обходить нечего. Это значительно ускоряет GC при большом количестве числовых объектов.

Span-тип кодируется в `spanClass` одним битом: `spanClass = sizeclass<<1 | noscanBit`. Поэтому span-классов вдвое больше, чем size-классов (`numSpanClasses = _NumSizeClasses × 2`).

Компилятор знает о наличии указателей статически (по типу) и выбирает span-тип при аллокации.

**Почему это важно — на практике.** Сравним два слайса по 10M элементов: один из значений без указателей, другой с указателем внутри.

```go
type NoPtr struct{ A, B, C int64 }   // 24 B, нет указателей → noscan
type WithPtr struct{ A int64; S *int } // есть указатель → scan

// держим 10M живых объектов и меряем паузу GC (runtime.GC())
a := make([]NoPtr, 10_000_000)    // noscan span: GC пройдёт мимо
b := make([]*WithPtr, 10_000_000) // scan: GC обойдёт каждый указатель
```

Для `NoPtr` mark-фаза почти не тратит времени на этот массив — span помечается целиком. Для слайса указателей GC обязан пройти по каждому из 10M указателей. Отсюда практическое правило: **в больших долгоживущих структурах избегай лишних указателей** — `[]Value` дешевле для GC, чем `[]*Value`; массив `int` дешевле, чем массив `interface{}`. Это один из главных способов снизить GC pause на больших heap.

---

## Путь аллокации: пошагово

```mermaid
flowchart TD
    Start["new(T) · make([]T, n) · &T{}"] --> EA{"escape analysis"}
    EA -->|"не убегает"| Stack["СТЕК<br/>бесплатно, без GC"]
    EA -->|"убегает в heap"| MG["mallocgc(size, type, zero)"]

    MG --> Sz{"размер объекта"}
    Sz -->|"< 16 B<br/>без указателей"| Tiny["tiny allocator<br/>упаковка в общий 16 B блок"]
    Sz -->|"≤ 32 KB"| MC["mcache.alloc[spanClass]<br/>per-P · lock-free"]
    Sz -->|"> 32 KB"| Large["mheap.allocLarge(npages)<br/>отдельный span"]

    Tiny -->|"блок полон → нужен новый"| MC

    MC -->|"span пуст"| MCe["mcentral.cacheSpan()<br/>лок на size class"]
    MCe -->|"нет свободных span"| MH["mheap.alloc(npages)<br/>global mheap.lock"]
    Large --> MH
    MH -->|"нет страниц"| OS["mmap → запрос к OS"]

    style Stack fill:#e8f5e8,stroke:#4caf50,color:#1b5e20
    style Tiny fill:#e8f5e8,stroke:#4caf50,color:#1b5e20
    style MC fill:#e8f5e8,stroke:#4caf50,color:#1b5e20
    style MCe fill:#fff8e1,stroke:#f9a825,color:#6d4c00
    style MH fill:#ffebee,stroke:#e53935,color:#b71c1c
    style Large fill:#ffebee,stroke:#e53935,color:#b71c1c
    style OS fill:#eceff1,stroke:#607d8b,color:#263238
```

Зелёные узлы — дёшево (стек или lock-free mcache, сюда попадает большинство аллокаций); жёлтый/красные — с блокировками, доходим реже.

**Сквозной пример.** Пусть в HTTP-хендлере `buf := make([]byte, 100)`, и буфер не убегает (escape analysis оставил бы на стеке — но представим, что убегает в горутину):

```
make([]byte, 100), buf уходит в heap
  → mallocgc(100, []byte, zero=true)
  → []byte без указателей → noscan
  → 100 ≤ 32 KB → small object, size class = 112 B (потеря 12 B)
  → mcache.alloc[spanClass(112, noscan)]: span есть, freeindex=5
      addr = span.base + 5×112;  freeindex → 6
  → вернуть addr, память уже занулена
```

Весь горячий путь — **без единой блокировки**. К mcentral обратимся только когда этот span заполнится, к mheap — когда у mcentral кончатся span, к OS — когда heap надо физически расширить. Поэтому «дешёвая аллокация в Go» — это про попадание в mcache; всё остальное гораздо дороже.

---

## Zero initialization

**Что это.** Когда ты создаёшь что-либо в Go, выданная память **всегда заполнена нулями**. Никакого «мусора» из прошлого, как в C. Поэтому и существуют zero values:

```go
var x int          // 0,   а не случайное число
var s string       // "",  а не мусор
var p *T           // nil
b := make([]byte, 4) // [0 0 0 0], гарантированно
u := new(User)     // все поля обнулены
```

В C `int x;` или `malloc()` отдают тебе то, что лежало в этой памяти раньше (остатки чужих данных). В Go так не бывает — это и удобно (zero value сразу валиден), и безопасно (не увидишь чужие данные).

**Откуда берётся обнуление — и почему это почти бесплатно.** Зависит от того, **свежая** это память или **переиспользованная**:

```mermaid
flowchart TD
    A["нужна память под объект"] --> B{"память свежая<br/>(только что от OS) или<br/>переиспользованная?"}
    B -->|"свежая страница<br/>(mmap)"| C["уже нулевая —<br/>OS отдаёт страницы<br/>зануленными<br/>→ обнулять НЕ надо"]
    B -->|"слот, освобождённый<br/>прошлым объектом"| D["там старые данные →<br/>аллокатор зануляет сам<br/>(memclr — быстрая<br/>очистка памяти)"]
```

- **Свежие страницы от OS** (через `mmap`) уже нулевые — OS-ядро *обязано* отдавать обнулённые страницы (иначе бы процессы видели чужую память). Здесь Go не делает вообще ничего.
- **Переиспользованный слот** (объект здесь жил, GC его освободил, теперь слот выдаётся заново) может содержать старые байты — вот его аллокатор зануляет сам, вызывая `memclr` (быстрая машинная очистка диапазона памяти).

**Деталь про `mallocgc(size, type, zero)`.** У аллокатора есть флаг `zero`. Обычно `zero=true`. Но компилятор иногда ставит `zero=false` как оптимизацию — если он видит, что код **сразу же целиком перезапишет** эту память (например, `make([]byte, n)`, который тут же заполняется через `copy`). Зачем зануляять то, что через миллисекунду затрётся.

Итог: гарантия «всё обнулено» — на уровне рантайма, но платим за неё только когда реально надо (переиспользование), а в массе случаев (свежие страницы / немедленная перезапись) обнуление бесплатно.

---

## Инструменты диагностики

```bash
# Посмотреть аллокации в бенчмарке
go test -bench=. -benchmem
# BenchmarkFoo-8   1000000   1234 ns/op   256 B/op   3 allocs/op

# Escape analysis: что улетает в heap
go build -gcflags="-m=1" ./...
go build -gcflags="-m=2" ./...  # подробнее

# Heap profile через pprof
go tool pprof http://localhost:6060/debug/pprof/heap
# Команды внутри: top, list FuncName, web
# -alloc_space: все аллокации за всё время
# -inuse_space: живые объекты сейчас

# MemStats
var ms runtime.MemStats
runtime.ReadMemStats(&ms)
fmt.Printf("HeapAlloc:   %d MB\n", ms.HeapAlloc>>20)
fmt.Printf("HeapSys:     %d MB\n", ms.HeapSys>>20)
fmt.Printf("HeapIdle:    %d MB\n", ms.HeapIdle>>20)   // свободно, но не вернули OS
fmt.Printf("HeapReleased:%d MB\n", ms.HeapReleased>>20) // возвращено OS
fmt.Printf("Mallocs:     %d\n", ms.Mallocs)
fmt.Printf("Frees:       %d\n", ms.Frees)
fmt.Printf("NumGC:       %d\n", ms.NumGC)
```

**Важные поля MemStats:**

| Поле | Что значит |
|---|---|
| `HeapAlloc` | живые объекты на heap сейчас |
| `HeapSys` | зарезервировано у OS всего |
| `HeapIdle` | span-ы свободны, но не возвращены OS |
| `HeapReleased` | возвращено OS через madvise |
| `HeapInuse` | span-ы с живыми объектами |
| `StackInuse` | стеки горутин |
| `MCacheInuse` | mcache объекты |
| `TotalAlloc` | всё что было аллоцировано (cumulative) |
| `Mallocs - Frees` | живых объектов на heap |

---

## Interview-ready answer

**"Как устроен аллокатор памяти в Go?"**

Go использует трёхуровневый аллокатор, вдохновлённый TCMalloc. Объекты размером ≤ 32 KB классифицируются по ~70 size classes. Аллокация проходит через три уровня:

1. **mcache** (per-P, без блокировок) — каждый логический процессор имеет свой кэш `mspan` для каждого size class. Аллокация = bump pointer, O(1), без contention.

2. **mcentral** (per-size-class, fine-grained lock) — когда span в mcache исчерпан, берётся новый span из mcentral. Один lock на size class.

3. **mheap** (global lock) — когда в mcentral нет spans, mheap нарезает новые spans из страниц. Если страниц нет — запрашивает у OS через `mmap`.

Дополнительно: **tiny allocator** упаковывает несколько маленьких (≤ 16 B) noscan объектов в один 16-байтный блок. Объекты без указателей (noscan) не сканируются GC — это ускоряет mark-фазу. Большие объекты (> 32 KB) идут напрямую в mheap.

**Практические выводы для senior:**
- дешёвая аллокация = попадание в mcache (lock-free); всё, что дальше (mcentral/mheap/mmap), дороже и с блокировками;
- запрос округляется вверх до size class → есть внутренняя фрагментация; подгонка размера структуры под границу класса экономит heap на больших объёмах;
- `[]Value` дешевле для GC, чем `[]*Value`, а числовой массив — чем `[]interface{}`: меньше указателей → noscan → GC проходит мимо;
- частые большие (> 32 KB) аллокации на hot path бьют по `mheap.lock` — кандидат на `sync.Pool` или переиспользование буфера.
