# Memory Profiling

Heap профиль показывает, кто занимает память и кто её аллоцирует. Это инструмент номер один при росте RSS, частых GC и OOM.

## Содержание

- [Два вопроса — два режима](#два-вопроса--два-режима)
- [Сбор heap профиля](#сбор-heap-профиля)
- [inuse_space: что сейчас в памяти](#inuse_space-что-сейчас-в-памяти)
- [alloc_space: кто больше всего аллоцирует](#alloc_space-кто-больше-всего-аллоцирует)
- [Сравнение профилей: -diff_base](#сравнение-профилей--diff_base)
- [Типичные паттерны](#типичные-паттерны)
- [GOGC и GOMEMLIMIT](#gogc-и-gomemlimit)
- [Ошибки при анализе памяти](#ошибки-при-анализе-памяти)
- [Interview-ready answer](#interview-ready-answer)

---

## Два вопроса — два режима

Прежде чем собирать профиль, ответь на вопрос: что именно ты ищешь?

| Вопрос | Режим | Что показывает |
|---|---|---|
| "Что сейчас держит память?" | `inuse_space` | Объекты, живые прямо сейчас |
| "Где больше всего аллоцируется?" | `alloc_space` | Всё, что было аллоцировано с начала (включая уже освобождённое) |
| "Сколько объектов аллоцировано?" | `alloc_objects` | Количество объектов (а не байты) |
| "Сколько объектов живёт сейчас?" | `inuse_objects` | Количество живых объектов |

```
inuse_space  → "у нас утечка памяти — кто держит?"
alloc_space  → "GC слишком часто — кто создаёт мусор?"
```

---

## Сбор heap профиля

```bash
# Скачать профиль
curl -o heap.prof "http://localhost:6060/debug/pprof/heap"

# Открыть с конкретным режимом
go tool pprof -inuse_space  -http=:6061 heap.prof   # по умолчанию
go tool pprof -alloc_space  -http=:6061 heap.prof
go tool pprof -inuse_objects -http=:6061 heap.prof
go tool pprof -alloc_objects -http=:6061 heap.prof

# Или allocs профиль (только аллокации)
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/allocs"
```

### Точность: memprofilerate

По умолчанию heap профиль семплирует **каждые 512KB аллокаций**. Мелкие аллокации могут быть не видны.

```go
// В начале программы для точного профиля (дорого!)
runtime.MemProfileRate = 1  // каждый байт

// Или через GODEBUG
// GODEBUG=memprofilerate=1 ./myapp
```

Для production — оставить default (512KB), для диагностики конкретной проблемы — снизить.

---

## inuse_space: что сейчас в памяти

Используй когда: процесс занимает много RSS, память растёт со временем.

```bash
go tool pprof -inuse_space -http=:6061 heap.prof
```

```
(pprof) top 10
      flat  flat%   sum%        cum   cum%
  512.00MB 48.59% 48.59%   512.00MB 48.59%  main.loadCache
  256.00MB 24.30% 72.89%   256.00MB 24.30%  main.buildIndex
  128.00MB 12.15% 85.04%   128.00MB 12.15%  bytes.(*Buffer).ReadFrom
```

```
(pprof) list loadCache
...
   512.00MB  512.00MB (flat, cum) 48.59% of Total
         .          .     45:func loadCache(items []Item) {
   512.00MB  512.00MB     46:    cache = make(map[string]*Item, len(items))  // ← держит всё
         .          .     47:    for _, item := range items {
         .          .     48:        cache[item.ID] = &item  // ← указатель на копию!
         .          .     49:    }
         .          .     50:}
```

### Чтение inuse_space профиля

```
flat  = память, выделенная непосредственно этой функцией и ещё живая
cum   = включая память всего, что эта функция вызвала
```

Объект живёт в профиле пока на него есть хотя бы одна живая ссылка.

---

## alloc_space: кто больше всего аллоцирует

Используй когда: CPU профиль показывает высокий `runtime.mallocgc`, GC срабатывает слишком часто.

```bash
go tool pprof -alloc_space -http=:6061 heap.prof
```

```
(pprof) top 10
      flat  flat%   sum%        cum   cum%
    2.34GB 58.40% 58.40%     2.34GB 58.40%  encoding/json.Marshal    ← 2.34GB за время жизни!
    0.91GB 22.73% 81.13%     0.91GB 22.73%  fmt.Sprintf
    0.45GB 11.23% 92.36%     0.45GB 11.23%  strings.(*Builder).grow
```

Большой alloc_space при нормальном inuse_space означает: **объекты создаются и быстро умирают** — GC работает нормально, но часто. Это называется allocation pressure.

### Снижение allocation pressure

```go
// ❌ Каждый запрос создаёт новый буфер
func handler(w http.ResponseWriter, r *http.Request) {
    buf := make([]byte, 4096)
    // ...
}

// ✅ sync.Pool — переиспользование буферов
var bufPool = sync.Pool{
    New: func() interface{} { return make([]byte, 4096) },
}

func handler(w http.ResponseWriter, r *http.Request) {
    buf := bufPool.Get().([]byte)
    defer bufPool.Put(buf)
    // ...
}
```

---

## Сравнение профилей: -diff_base

Мощный инструмент для поиска утечек: собери профиль до и после нагрузки, сравни.

```bash
# Профиль 1: исходное состояние
curl -o heap_before.prof "http://localhost:6060/debug/pprof/heap"

# ... запустить нагрузку или подождать ...

# Профиль 2: через 5 минут под нагрузкой
curl -o heap_after.prof "http://localhost:6060/debug/pprof/heap"

# Сравнение: показывает только ДЕЛЬТУ (что выросло)
go tool pprof -inuse_space -diff_base heap_before.prof -http=:6061 heap_after.prof
```

В diff профиле:
- **Положительные значения** → это выросло (возможная утечка)
- **Отрицательные значения** → это уменьшилось (GC освободил)

```
(pprof) top
      flat  flat%   sum%        cum   cum%
   128.00MB 100.00% 100.00%  128.00MB 100.00%  main.appendToHistory  ← утечка здесь
    -64.00MB -50.00% 50.00%  -64.00MB -50.00%  main.processOld       ← это освободилось
```

---

## Типичные паттерны

### 1. Slice удерживает большой backing array

```go
// ❌ sub-slice держит весь исходный массив
func getRecent(all []Event) []Event {
    return all[len(all)-10:]  // 10 элементов, но весь массив в памяти
}

// ✅ явное копирование
func getRecent(all []Event) []Event {
    result := make([]Event, 10)
    copy(result, all[len(all)-10:])
    return result
}
```

В профиле: inuse_space у getRecent будет огромным relative to 10 elements.

### 2. Замыкание захватывает большой объект

```go
// ❌ bigData держится в памяти пока горутина жива
bigData := loadHugeData()  // 1 GB
go func() {
    time.Sleep(24 * time.Hour)
    process(bigData[0])  // нужен только первый элемент
}()

// ✅ скопировать только нужное ДО захвата
first := bigData[0]
bigData = nil  // явно освободить
go func() {
    time.Sleep(24 * time.Hour)
    process(first)
}()
```

### 3. Map без ограничения роста (in-memory cache без TTL/eviction)

```go
// ❌ растёт вечно
var cache = make(map[string]*Result)

func getOrCompute(key string) *Result {
    if r, ok := cache[key]; ok { return r }
    r := compute(key)
    cache[key] = r  // никогда не удаляется!
    return r
}

// ✅ LRU cache с ограничением
import "github.com/hashicorp/golang-lru/v2"
cache, _ := lru.New[string, *Result](10000)  // max 10k entries
```

В профиле: `runtime.mapassign_faststr` или указатель на map struct с огромным inuse.

### 4. string([]byte) конверсия: лишние копии

```go
// ❌ каждый вызов копирует
func process(data []byte) {
    s := string(data)  // копия
    if strings.Contains(s, "prefix") { ... }
}

// ✅ работать с []byte напрямую
func process(data []byte) {
    if bytes.Contains(data, []byte("prefix")) { ... }
}
```

### 5. Goroutine leak = memory leak

Каждая горутина держит минимум 2KB стека. 100k горутин = минимум 200MB.

```go
// ❌ горутина никогда не завершится если ch никто не закроет
go func() {
    for v := range ch {  // блокируется навсегда
        process(v)
    }
}()

// ✅ с context
go func() {
    for {
        select {
        case v, ok := <-ch:
            if !ok { return }
            process(v)
        case <-ctx.Done():
            return
        }
    }
}()
```

В профиле: `inuse_space` при goroutine profile покажет горутины в "chan receive" на одной строке.

---

## GOGC и GOMEMLIMIT

Иногда после оптимизации аллокаций нужно настроить сам GC.

```go
// GOGC=100 (по умолчанию): GC запускается когда heap вырос в 2x
// GOGC=200: GC реже → меньше CPU на GC, больше пиковая память
// GOGC=50:  GC чаще → меньше пиковая память, больше CPU на GC
os.Setenv("GOGC", "200")

// GOMEMLIMIT (Go 1.19+): жёсткий лимит heap, GC не даст выйти за него
// Лучше чем GOGC для контейнеров с ограниченной памятью
os.Setenv("GOMEMLIMIT", "512MiB")

// Или из кода:
import "runtime/debug"
debug.SetGCPercent(200)
debug.SetMemoryLimit(512 * 1024 * 1024)
```

```bash
# Посмотреть GC активность в реальном времени
GODEBUG=gctrace=1 ./myapp

# Вывод:
# gc 1 @0.005s 2%: 0.13+1.2+0.014 ms clock, 0.26+0.59/1.1/0+0.027 ms cpu, 8->8->1 MB, 9 MB goal, 0 MB stacks, 0 MB globals, 4 P
#                               ↑ wall time    ↑ STW  ↑ concurrent   ↑ heap stats
```

Значение `2%` в `@0.005s 2%` — процент времени программы, потраченный на GC. Норма < 5-10%.

---

## Ошибки при анализе памяти

### Ошибка 1: путать RSS и heap

```
RSS (Resident Set Size) = heap + stack горутин + runtime structures + mmap файлы
Heap профиль ≠ RSS

Если RSS растёт, а heap профиль не растёт — смотри goroutine stacks или mmap.
```

### Ошибка 2: смотреть inuse когда нужен alloc

```
inuse_space нормальный, но GC работает 30% времени
→ проблема в allocation rate, нужен alloc_space
→ ищем кто создаёт много мусора, а не кто держит память
```

### Ошибка 3: не учитывать scavenger

Go runtime может вернуть память OS через `MADV_DONTNEED`/`MADV_FREE` (scavenger). RSS может быть меньше heap capacity. `runtime.MemStats.HeapIdle` — выделенный, но пустой heap.

```go
var stats runtime.MemStats
runtime.ReadMemStats(&stats)

fmt.Printf("HeapAlloc:   %d MB (живые объекты)\n", stats.HeapAlloc/1024/1024)
fmt.Printf("HeapInuse:   %d MB (занятые spans)\n", stats.HeapInuse/1024/1024)
fmt.Printf("HeapIdle:    %d MB (свободные spans, возможно вернуть OS)\n", stats.HeapIdle/1024/1024)
fmt.Printf("HeapSys:     %d MB (запрошено у OS суммарно)\n", stats.HeapSys/1024/1024)
fmt.Printf("NumGC:       %d\n", stats.NumGC)
fmt.Printf("PauseTotalNs: %.2f ms\n", float64(stats.PauseTotalNs)/1e6)
```

---

## Interview-ready answer

**"Сервис постепенно растёт по памяти и не освобождает. Что делаешь?"**

Первый шаг — разобраться что именно растёт. Смотрю `runtime.MemStats`: HeapAlloc растёт = объекты не освобождаются. RSS растёт но HeapAlloc нет = возможно растут стеки горутин (goroutine leak) или mmap.

Для heap: собираю два профиля с разрывом под нагрузкой и сравниваю через `-diff_base`:
```bash
go tool pprof -inuse_space -diff_base heap1.prof -http=:6061 heap2.prof
```
Смотрю что выросло в `flat` — это подозреваемый.

Типичные причины:
1. **Unbounded cache** — map без eviction. Fix: LRU с лимитом.
2. **Slice удерживает большой backing array** через sub-slice. Fix: явный copy.
3. **Замыкание захватывает большой объект** в горутину. Fix: скопировать нужное, обнулить исходное.
4. **Goroutine leak** — каждая горутина минимум 2KB стека. Fix: context cancellation.

Для allocation pressure (GC часто, но память не растёт): `alloc_space` профиль — кто создаёт больше всего байт. Fix: sync.Pool для переиспользования, уменьшение escape на heap.
