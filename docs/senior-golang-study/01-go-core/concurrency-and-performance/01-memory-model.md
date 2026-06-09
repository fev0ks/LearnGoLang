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

Каждый практический приём синхронизации — это применение конкретного HB-правила выше. Сводка «какой edge зачем»:

| Приём | Какой HB-edge использует | Когда |
|---|---|---|
| Передача владения через канал | send HB receive | отдать данные другой горутине и больше их не трогать |
| Mutex вокруг shared state | `Unlock` HB следующий `Lock` | несколько полей менять согласованно |
| `sync.Once` для ленивой инициализации | завершение `f` HB любой последующий `Do` | один раз построить синглтон/конфиг |
| `atomic.Value` / `atomic.Pointer[T]` snapshot | `Store` HB наблюдающий `Load` (Go 1.19+ seq-consistent) | read-heavy hot-reload без локов |

Реализации и подводные камни этих примитивов — в соседних файлах:

- каналы, ownership transfer, fan-in/out, goroutine leak → [02-goroutines-and-channels](./02-goroutines-and-channels.md);
- Mutex/RWMutex, `sync.Once`, `sync.Pool`, `sync.Map`, `atomic`, CAS-loop, `atomic.Pointer` vs `atomic.Value` → [03-sync-primitives](./03-sync-primitives.md).

Здесь важно лишь: **каждый из них создаёт synchronization edge**, превращая «нет HB → undefined» в «есть HB → запись видна».

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

Все они — частный случай одного: **общая память без synchronization edge между записью и чтением**.

**Незащищённый результат из горутины** — нет HB между записью в горутине и чтением в main:
```go
// ПЛОХО: запись в горутине и чтение в main без HB → data race
var result []int
go func() { result = []int{1, 2, 3} }()
fmt.Println(result)

// ПРАВИЛЬНО: канал создаёт edge send HB receive
ch := make(chan []int, 1)
go func() { ch <- []int{1, 2, 3} }()
fmt.Println(<-ch)
```

Ещё две классические гонки разобраны как загадки в соседних файлах (тоже «нет HB»):

- **захват переменной цикла горутиной** (до Go 1.22) → [02-goroutines-and-channels, Загадка 1](./02-goroutines-and-channels.md#разбор-примеров-загадок);
- **`WaitGroup.Add` внутри горутины** (гонка с `Wait`) → [03-sync-primitives](./03-sync-primitives.md#syncwaitgroup).

## Interview-ready answer

**1. Что такое happens-before?**
Частичный порядок событий: если A HB B, все записи до и в A гарантированно видны при B. В одной горутине операции упорядочены по коду. Между горутинами HB возникает **только** через synchronization edges: `go`-statement, channel send/receive, `Unlock`/`Lock`, `close`, `sync.Once`, `WaitGroup.Wait`, atomic. Нет edge → порядок и видимость не определены.

**2. Что такое data race и чем он опасен?**
Два конкурентных доступа к одной переменной, хотя бы один — запись, и между ними нет HB. По спецификации Go это **undefined behavior**: можно прочитать старое, несогласованное или рваное (torn) значение, а не «просто иногда неверное». Поэтому гонку нельзя «оставить, если редко».

**3. Какие HB-гарантии у каналов?**
Send HB соответствующий receive; `close` HB receive нулевого значения; у buffered-канала ёмкости C — `k`-й send HB `(k+C)`-й receive. У unbuffered дополнительно: receive HB завершение send. Завершение горутины само по себе HB **не** создаёт — нужен `WaitGroup`/канал.

**4. Что гарантирует sync/atomic по памяти?**
С Go 1.19 атомарные операции **sequentially consistent**: если `Load` увидел значение от `Store`, то `Store` HB этот `Load` (можно публиковать данные через atomic-флаг). Смешивать atomic и обычный доступ к одной переменной нельзя — это гонка.

**5. Как работает race detector и каковы его пределы?**
`-race` инструментирует каждый доступ к памяти и строит HB-граф через vector clocks; репортит доступ без HB. Overhead ~5–20× CPU, ~5–10× память. Нет false positives, но есть **false negatives** — находит лишь гонки, которые реально произошли на этом прогоне. Поэтому `-race` гоняют в CI на представительной нагрузке.

**6. Channel или mutex?**
Channel — передача владения, pipeline-координация, сигналы (done/cancel). Mutex — защита shared mutable state, особенно когда несколько полей меняются согласованно. Hot path с простым состоянием — `atomic`/`atomic.Pointer` (без локов). Сложная multi-field логика → mutex (проще рассуждать о корректности).
