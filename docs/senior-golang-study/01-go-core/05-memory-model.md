# Go Memory Model

Memory model отвечает на вопрос: когда запись в одной горутине гарантированно видна другой? Без понимания этого невозможно рассуждать о корректности любого конкурентного кода.

## Содержание

- [Проблема видимости памяти](#проблема-видимости-памяти)
- [Happens-Before](#happens-before)
- [Ключевые HB-правила Go](#ключевые-hb-правила-go)
- [Data race: определение и последствия](#data-race-определение-и-последствия)
- [Паттерны синхронизации](#паттерны-синхронизации)
- [Race detector](#race-detector)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

## Проблема видимости памяти

Без синхронизации нет гарантий о порядке видимости записей между горутинами.

```go
var x int
var ready bool

// Горутина 1
x = 42
ready = true

// Горутина 2
for !ready {} // active wait
fmt.Println(x) // может напечатать 0, а не 42
```

Почему: компилятор и процессор имеют право **переупорядочивать** инструкции внутри одной горутины (и делают это для оптимизации), и CPU кэши не синхронизируются автоматически между ядрами.

Одна горутина может видеть запись в `ready=true` **раньше**, чем запись в `x=42` — даже если в коде `x=42` написано первым.

## Happens-Before

**Happens-before (HB)** — частичный порядок событий в программе.

> Если A happens-before B, то все эффекты A (записи в память) гарантированно видны при выполнении B.

Свойства:
- В одной горутине: все операции упорядочены по HB в порядке кода;
- Между горутинами: HB устанавливается только через явные **synchronization edges**.

```
Горутина 1:  A → B → C
Горутина 2:  X → Y → Z

Без синхронизации: нет никакого HB между {A,B,C} и {X,Y,Z}
С каналом:   C → [channel send] → X  →  C HB X HB Y HB Z
```

Если между двумя операциями из разных горутин нет HB — поведение **не определено** (для записей).

## Ключевые HB-правила Go

### 1. Запуск горутины

`go` statement happens-before первой инструкции горутины:

```go
var data int

func main() {
    data = 42
    go func() {
        fmt.Println(data) // гарантированно видит 42
    }()
    // НО: main может завершиться до горутины, нужен WaitGroup
}
```

**Завершение горутины** НЕ устанавливает HB автоматически — нужен `WaitGroup.Wait()`:

```go
var wg sync.WaitGroup
var result int

wg.Add(1)
go func() {
    defer wg.Done()
    result = compute()
}()

wg.Wait()
fmt.Println(result) // OK: wg.Done() HB wg.Wait() → result виден
```

### 2. Каналы

**Unbuffered channel:**
```go
var data int
ch := make(chan struct{})

go func() {
    data = 42
    ch <- struct{}{} // send HB соответствующий receive
}()

<-ch                 // receive возвращает только после send
fmt.Println(data)    // гарантированно 42

// Дополнительно для unbuffered:
// receive happens-before completion of send
// (send в отправителе не завершится, пока получатель не принял)
```

**Buffered channel (capacity C):**
```go
// kth send HB (k + cap)th receive
ch := make(chan int, 3)  // capacity = 3
// 1-й send HB 4-й receive
// 2-й send HB 5-й receive
// и т.д.
```

**Close канала:**
```go
var data int
ch := make(chan struct{})

go func() {
    data = 42
    close(ch) // close HB receive нулевого значения
}()

<-ch                   // получаем нулевое значение из закрытого канала
fmt.Println(data)      // гарантированно 42
```

### 3. sync.Mutex / sync.RWMutex

`n-й Unlock` happens-before `(n+1)-й Lock`:

```go
var mu sync.Mutex
var shared int

// Горутина 1:
mu.Lock()
shared = 42
mu.Unlock() // этот Unlock HB следующий Lock в горутине 2

// Горутина 2:
mu.Lock()
fmt.Println(shared) // гарантированно 42
mu.Unlock()
```

`RWMutex`: `RUnlock` HB `Lock` (write lock блокирует читателей), `Unlock` HB `RLock`.

### 4. sync.Once

Функция f, переданная в `Do`, завершается HB любой последующий вызов `Do`:

```go
var once sync.Once
var config *Config

func getConfig() *Config {
    once.Do(func() {
        config = loadConfig() // загружается ровно один раз
    })
    return config // гарантированно видит результат loadConfig()
}
```

**Без Once — классический race:**
```go
// НЕПРАВИЛЬНО: double-checked locking без правильной синхронизации
var mu sync.Mutex
var config *Config

func getConfig() *Config {
    if config == nil {  // BUG: читаем без lock
        mu.Lock()
        if config == nil {
            config = loadConfig()
        }
        mu.Unlock()
    }
    return config // может вернуть partially initialized config
}
```

### 5. sync/atomic

Начиная с **Go 1.19**: atomic операции на одной переменной имеют **sequentially consistent** семантику — если `Load` наблюдает значение от `Store`, то `Store` HB `Load`:

```go
var flag atomic.Bool
var data int

// Горутина 1:
data = 42
flag.Store(true) // HB

// Горутина 2:
if flag.Load() { // если видим true
    fmt.Println(data) // гарантированно видим 42
}
```

До Go 1.19: только sequentially consistent для отдельных операций, но не было гарантий между разными переменными.

## Data race: определение и последствия

**Data race** возникает когда:
1. Два concurrent доступа к одной переменной;
2. Хотя бы один из них — **запись**;
3. Между ними **нет HB** отношения.

Последствия data race в Go: **undefined behavior** на уровне Go specification.

```go
// DATA RACE примеры:

// 1. Counter без синхронизации
var counter int
for i := 0; i < 1000; i++ {
    go func() { counter++ }() // READ+WRITE без синхронизации
}

// 2. Map из нескольких горутин
m := map[string]int{}
go func() { m["a"] = 1 }() // WRITE
go func() { _ = m["a"] }() // READ — map не thread-safe!

// 3. Slice append
var s []int
go func() { s = append(s, 1) }() // READ+WRITE slice header
go func() { s = append(s, 2) }() // READ+WRITE slice header
```

**Правильные версии:**
```go
// 1. Counter — atomic или mutex
var counter atomic.Int64
for i := 0; i < 1000; i++ {
    go func() { counter.Add(1) }()
}

// 2. Map — sync.Map или mutex
var mu sync.RWMutex
m := map[string]int{}
go func() {
    mu.Lock()
    m["a"] = 1
    mu.Unlock()
}()

// 3. Slice — через channel или mutex
var mu sync.Mutex
var s []int
go func() {
    mu.Lock()
    s = append(s, 1)
    mu.Unlock()
}()
```

## Паттерны синхронизации

**Channel для передачи владения (ownership transfer):**
```go
// Горутина 1 создает данные и передает их горутине 2
ch := make(chan []byte, 1)

go func() {
    buf := make([]byte, 1024)
    // заполняем buf
    ch <- buf // передаем владение; после этого не трогаем buf
}()

go func() {
    buf := <-ch // принимаем владение; безопасно работать с buf
    process(buf)
}()
```

**Mutex для защиты shared mutable state:**
```go
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *SafeCounter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}
```

**sync.Once для ленивой инициализации:**
```go
var (
    instance *DB
    once     sync.Once
)

func GetDB() *DB {
    once.Do(func() {
        instance = openDB()
    })
    return instance
}
```

**atomic.Value для read-heavy snapshot:**
```go
type config struct {
    timeout time.Duration
    maxConn int
}

var cfg atomic.Value // хранит *config

// Запись (редко): background goroutine
func updateConfig(newCfg *config) {
    cfg.Store(newCfg) // atomic store — HB для последующих Load
}

// Чтение (часто): zero allocation, no lock
func getTimeout() time.Duration {
    return cfg.Load().(*config).timeout
}
```

## Race detector

```bash
# Запуск с race detector (добавить ко всем тестам в CI)
go test -race ./...
go build -race ./...
go run -race main.go
```

Race detector:
- инструментирует **каждый** доступ к памяти (запись и чтение);
- отслеживает HB-граф через vector clocks;
- репортирует при обнаружении concurrent доступа без HB.

```
==================
WARNING: DATA RACE
Write at 0x00c0000b4010 by goroutine 7:
  main.main.func1()
      /app/main.go:12 +0x2c

Previous read at 0x00c0000b4010 by goroutine 8:
  main.main.func2()
      /app/main.go:17 +0x24
==================
```

Ограничения:
- находит только те гонки, которые **реально произошли** во время запуска;
- overhead ~5–20× CPU, ~5–10× memory;
- нет false positives, но есть false negatives (гонка есть, но не сработала).

## Типичные ошибки

**Closure захватывает переменную цикла (исправлено в Go 1.22):**
```go
// До Go 1.22: все горутины видят последнее значение i
for i := 0; i < 5; i++ {
    go func() {
        fmt.Println(i) // RACE + все печатают 5
    }()
}

// Фикс (до Go 1.22): передавать как аргумент
for i := 0; i < 5; i++ {
    go func(i int) {
        fmt.Println(i) // OK
    }(i)
}

// Go 1.22+: переменная цикла имеет per-iteration scope, гонки нет
```

**WaitGroup counter race:**
```go
// ПЛОХО: Add вызван внутри горутины — race с Wait
var wg sync.WaitGroup
for _, item := range items {
    go func(item Item) {
        wg.Add(1)        // может быть вызван после Wait()
        defer wg.Done()
        process(item)
    }(item)
}
wg.Wait()

// ПРАВИЛЬНО: Add вызывать до запуска горутины
for _, item := range items {
    wg.Add(1)
    go func(item Item) {
        defer wg.Done()
        process(item)
    }(item)
}
wg.Wait()
```

**Возврат указателя на локальный slice из горутины:**
```go
// ПЛОХО
var result []int
go func() {
    result = []int{1, 2, 3} // запись без синхронизации
}()
fmt.Println(result) // чтение без синхронизации → race

// ПРАВИЛЬНО: через channel
ch := make(chan []int, 1)
go func() {
    ch <- []int{1, 2, 3}
}()
result := <-ch
fmt.Println(result)
```

## Interview-ready answer

**"Что такое happens-before?"**

Happens-before — частичный порядок событий в программе. Если A HB B, то все записи, сделанные до и в A, гарантированно видны при B. В одной горутине все операции HB по порядку кода. Между горутинами HB устанавливается только через явные synchronization edges: channel send/receive, mutex Lock/Unlock, sync.Once, WaitGroup, atomic операции.

**"Что такое data race?"**

Data race — два concurrent доступа к одной переменной, хотя бы один из которых запись, и между ними нет HB. В Go это undefined behavior по спецификации: значение может быть старым, несогласованным или вообще garbage из-за torn reads. Race detector (`-race`) находит фактически произошедшие гонки с overhead ~10× по скорости.

**"Когда использовать channel, а когда mutex?"**

Channel — для передачи владения данными между горутинами, pipeline координации, сигналов (done/cancel). Mutex — для защиты shared mutable state, особенно когда несколько полей должны меняться атомарно. На hot path с высоким RPS и simple shared state — `atomic.Value` или `sync/atomic` операции (zero allocation, no lock overhead). Сложная multi-field логика → mutex, потому что reasoning о корректности проще.
