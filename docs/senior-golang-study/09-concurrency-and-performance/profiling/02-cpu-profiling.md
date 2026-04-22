# CPU Profiling

CPU профиль — первое, за что берутся при "сервис потребляет много CPU" или "запросы медленные". Главная задача: найти функции, которые съедают процессорное время, и понять почему.

## Содержание

- [Как работает CPU семплер](#как-работает-cpu-семплер)
- [Сбор CPU профиля](#сбор-cpu-профиля)
- [Чтение flamegraph](#чтение-flamegraph)
- [Типичные hotspot паттерны](#типичные-hotspot-паттерны)
- [Оптимизации: что делать найдя hotspot](#оптимизации-что-делать-найдя-hotspot)
- [Ошибки при CPU профилировании](#ошибки-при-cpu-профилировании)
- [Interview-ready answer](#interview-ready-answer)

---

## Как работает CPU семплер

Go CPU профилировщик работает через **SIGPROF** — сигнал, который OS посылает процессу с заданной частотой.

```
100 раз в секунду (10ms интервал):
  SIGPROF → runtime прерывает выполнение
           → записывает stack trace текущей горутины
           → добавляет в счётчики профиля

итог: функция X набрала N семплов → ~N * 10ms CPU времени
```

**Что это означает на практике:**
- Функция с 500 семплами за 30 секунд → ~5 секунд CPU
- Функции быстрее ~1ms могут быть **недостаточно** представлены
- Для коротких быстрых функций — нужно больше итераций (бенчмарки + cpuprofile)
- Функции в runtime (gc, mallocgc, schedule) тоже видны в профиле

**Параллелизм**: семплируется только **одна** горутина из тех, что реально выполняются в момент SIGPROF. Если есть 4P и все заняты — за 30 секунд будет ~3000 семплов суммарно (не 300).

---

## Сбор CPU профиля

### Из production (рекомендуется)

```bash
# 30 секунд — хорошее время для продакшн сервиса под нагрузкой
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/profile?seconds=30"
```

**Важно:** собирать профиль **под нагрузкой** — если сервис idle, профиль будет пустым или покажет только runtime overhead.

### Из бенчмарка

```bash
go test -bench=BenchmarkProcessRequest -cpuprofile=cpu.prof -run='^$' ./...
go tool pprof -http=:6061 cpu.prof
```

### Из кода (для конкретного участка)

```go
import (
    "os"
    "runtime/pprof"
)

func profileSection() {
    f, err := os.Create("section.prof")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()

    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()

    doExpensiveWork()  // только этот участок попадёт в профиль
}
```

### Из теста для конкретного сценария

```go
func TestMyFunction(t *testing.T) {
    if testing.Short() {
        t.Skip()
    }
    f, _ := os.Create("test.prof")
    pprof.StartCPUProfile(f)
    defer func() {
        pprof.StopCPUProfile()
        f.Close()
    }()

    for i := 0; i < 100000; i++ {
        myExpensiveFunction()
    }
}
```

---

## Чтение flamegraph

```bash
go tool pprof -http=:6061 cpu.prof
# → http://localhost:6061 → View → Flame Graph
```

### Анатомия flamegraph

```
┌────────────────────────────────────────────────────────┐  ← main.main (широкий = много времени)
│                    main.handleRequest                   │
├──────────────────────────┬─────────────────────────────┤
│     json.Marshal (25%)   │   processItems (72%)        │
├──────────────────────────┼──────┬──────────────────────┤
│  encoding/json internals │      │ regexp.FindString    │
│          ...             │      │    (65% of total!)   │
└──────────────────────────┴──────┴──────────────────────┘
```

**Что искать:**
1. **Широкие плато** на верхних уровнях — функции, потребляющие больше всего CPU
2. **Широкий прямоугольник без дочерних** — "leaf" функция, сама по себе медленная
3. **Длинная тонкая башня** — глубокая рекурсия или цепочка оберток, смотри на вершину
4. **runtime.mallocgc** или **runtime.GC** — GC overhead, много аллокаций

### top в терминале

```bash
$ go tool pprof cpu.prof
(pprof) top 10

      flat  flat%   sum%        cum   cum%
     4.12s 41.20% 41.20%      4.12s 41.20%  regexp.(*Regexp).FindString
     1.83s 18.30% 59.50%      1.83s 18.30%  runtime.mallocgc
     0.91s  9.10% 68.60%      9.54s 95.40%  main.processItems
     0.44s  4.40% 73.00%      0.44s  4.40%  runtime.memmove
     ...
```

**Алгоритм чтения:**
1. Посмотреть топ по flat — самые "горячие" листовые функции
2. Посмотреть топ по cum (`top -cum`) — найти точки входа с наибольшим влиянием
3. `list <func>` — посмотреть конкретные строки

```
(pprof) list processItems
...
     0.91s      9.54s (flat, cum) 95.40% of Total
         .          .     23:func processItems(items []Item) {
         .          .     24:    pattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)  // ← компиляция ВНУТРИ цикла!
     0.91s      4.12s     25:    for _, item := range items {
         .      4.12s     26:        if pattern.FindString(item.Date) != "" {
         .      5.42s     27:            processMatch(item)
         .          .     28:        }
         .          .     29:    }
         .          .     30:}
```

---

## Типичные hotspot паттерны

### 1. Компиляция regexp в цикле

```go
// ❌ regexp.MustCompile КАЖДЫЙ вызов — очень дорого
func validate(s string) bool {
    return regexp.MustCompile(`^\d+$`).MatchString(s)
}

// ✅ Компилировать один раз
var digitRegexp = regexp.MustCompile(`^\d+$`)

func validate(s string) bool {
    return digitRegexp.MatchString(s)
}
```

В профиле: `regexp.Compile` с высоким flat% в вызываемой из цикла функции.

### 2. fmt.Sprintf для конкатенации строк

```go
// ❌ fmt.Sprintf аллоцирует, медленно для горячего пути
key := fmt.Sprintf("%s:%d", prefix, id)

// ✅ strings.Builder или strconv
var b strings.Builder
b.WriteString(prefix)
b.WriteByte(':')
b.WriteString(strconv.Itoa(id))
key := b.String()

// Или для простых случаев:
key := prefix + ":" + strconv.Itoa(id)
```

В профиле: `fmt.(*pp).doPrintf` или `fmt.Sprintf` с заметным flat%.

### 3. JSON marshal/unmarshal на hot path

```go
// ❌ encoding/json медленно из-за reflect
data, _ := json.Marshal(obj)

// ✅ Вариант 1: jsoniter (дропин-замена, в 2-3x быстрее)
import jsoniter "github.com/json-iterator/go"
var json = jsoniter.ConfigCompatibleWithStandardLibrary
data, _ := json.Marshal(obj)

// ✅ Вариант 2: easyjson/sonic — кодогенерация, самый быстрый
// go generate ./...  +  easyjson model.go

// ✅ Вариант 3: pooling буферов если формат фиксированный
var bufPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
```

В профиле: `encoding/json.Marshal`, `reflect.Value.*` с высоким cum%.

### 4. Interface boxing (неожиданные аллокации)

```go
// ❌ каждый вызов создаёт interface{} wrapper — аллокация
var cache = make(map[string]interface{})
cache[key] = value  // value боксируется

// ✅ типизированная map
var cache = make(map[string]*MyType)
cache[key] = value
```

В профиле: `runtime.mallocgc` с высоким flat% при, казалось бы, "ничего не делающем" коде.

### 5. String ↔ []byte конверсии

```go
// ❌ конверсия string → []byte всегда копирует
func process(data string) {
    b := []byte(data)  // копия!
    processBytes(b)
}

// ✅ если функция только читает, передавать строку напрямую
// ✅ если нужно []byte часто — хранить уже как []byte
// ✅ unsafe трюк (читай осторожно):
func stringToBytes(s string) []byte {
    return unsafe.Slice(unsafe.StringData(s), len(s))  // Go 1.20+
}
```

### 6. Sync.Mutex на горячем пути без нужды

```go
// ❌ global mutex на каждом обращении к counter
var mu sync.Mutex
var counter int64
func increment() {
    mu.Lock()
    counter++
    mu.Unlock()
}

// ✅ atomic для простых счётчиков
var counter atomic.Int64
func increment() {
    counter.Add(1)
}
```

В профиле: `sync.(*Mutex).Lock` / `runtime.semacquire1` с заметным flat%.

---

## Оптимизации: что делать найдя hotspot

### До оптимизации: измерить baseline

```bash
# Всегда делать baseline бенчмарк перед изменениями
go test -bench=BenchmarkMyFunc -benchmem -count=5 ./... > before.txt
```

### После оптимизации: сравнить

```bash
go test -bench=BenchmarkMyFunc -benchmem -count=5 ./... > after.txt
benchstat before.txt after.txt
```

```
name        old time/op    new time/op    delta
MyFunc-8      1.23ms ± 2%    0.31ms ± 3%  -74.80%  (p=0.008)

name        old allocs/op  new allocs/op  delta
MyFunc-8       234 ± 0%       12 ± 0%    -94.87%
```

### Общие принципы оптимизации CPU

```
1. Сначала алгоритм — O(n²) → O(n log n) лучше любого micro-opt
2. Потом аллокации — меньше GC pressure = меньше пауз
3. Потом cache locality — struct of arrays vs array of structs
4. Потом SIMD/assembly (редко нужно в Go)
```

---

## Ошибки при CPU профилировании

### Ошибка 1: профилировать idle сервис

```bash
# Бесполезно: профиль покажет только runtime scheduler
go tool pprof "http://localhost:6060/debug/pprof/profile?seconds=30"
# (сервис без нагрузки)

# Правильно: запустить нагрузку ДО и ВО ВРЕМЯ сбора
# wrk -t4 -c100 -d35s http://localhost:8080/api/endpoint &
# go tool pprof "http://localhost:6060/debug/pprof/profile?seconds=30"
```

### Ошибка 2: слишком короткий профиль

```bash
# 5 секунд — мало семплов, шумный результат
?seconds=5

# 30 секунд — хорошо для большинства случаев
?seconds=30

# Для редких событий — 60-120 секунд
?seconds=60
```

### Ошибка 3: оптимизировать не то

```
Правило: не оптимизируй то, что показывает < 5% от total.
Сначала закрой самый большой flat.
```

### Ошибка 4: inline-expanded функции

Маленькие функции компилятор inline-ит → они исчезают из стека → их время приписывается вызывающей функции. Это нормально — `list` покажет правильные строки.

```bash
# Посмотреть что инлайнилось при компиляции
go build -gcflags='-m' ./... 2>&1 | grep "inlining call"
```

---

## Interview-ready answer

**"Высокий CPU на сервисе. С чего начнёшь?"**

Сначала смотрю metrics: рост CPU постепенный или резкий? Коррелирует с нагрузкой (RPS) или нет? Если пропорционален нагрузке — скорее всего алгоритмическая проблема или дорогие операции. Если рос постепенно без роста нагрузки — подозреваю утечку горутин или накопление состояния.

Собираю CPU профиль **под нагрузкой**: `go tool pprof -http=:6061 "http://svc:6060/debug/pprof/profile?seconds=30"`. Открываю flamegraph в браузере. Ищу широкие плато.

Типичные находки: regexp компиляция в цикле, fmt.Sprintf на горячем пути, JSON marshal через reflect, string-to-bytes копии, или много маленьких аллокаций → высокий `runtime.mallocgc` → нужен heap профиль.

После нахождения hotspot — `list <func>` смотрю конкретные строки. Делаю baseline бенчмарк, фиксирую, оптимизирую, сравниваю через `benchstat`.

**Главная ошибка**: оптимизировать без измерения до и после — можно "улучшить" функцию с 2% flat и потерять неделю.
