# Sync Primitives

Пакет `sync` и `sync/atomic` — низкоуровневые примитивы синхронизации. Каналы хороши для передачи данных; `sync` — для защиты состояния.

---

## `sync.Mutex` vs `sync.RWMutex`

### `sync.Mutex` — исключительный доступ

```go
type Counter struct {
    mu    sync.Mutex
    value int
}

func (c *Counter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.value++
}

func (c *Counter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.value
}
```

### `sync.RWMutex` — разделение read/write

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]string
}

func (c *Cache) Get(key string) (string, bool) {
    c.mu.RLock()         // несколько горутин могут читать одновременно
    defer c.mu.RUnlock()
    v, ok := c.items[key]
    return v, ok
}

func (c *Cache) Set(key, value string) {
    c.mu.Lock()          // исключительный доступ для записи
    defer c.mu.Unlock()
    c.items[key] = value
}
```

### Когда RWMutex выгоден

RWMutex полезен когда:
- Операций чтения **значительно больше**, чем записи (например, 95% reads)
- Критическая секция чтения **не тривиальная** (занимает хоть сколько-то времени)

RWMutex **невыгоден** когда:
- Writes и reads примерно поровну — overhead от самого RWMutex компенсирует выгоду
- Критическая секция очень короткая (1–2 инструкции) — Mutex быстрее
- Высокое write-давление: каждый write ждёт всех активных reads → starvation

```
Эмпирическое правило: >80% reads → RWMutex; иначе → Mutex
```

---

## Типичные ошибки с Mutex

### 1. Копирование Mutex (go vet ловит)

```go
type Safe struct {
    mu sync.Mutex
    v  int
}

// Плохо — копируем структуру вместе с mutex
s1 := Safe{}
s2 := s1 // mu скопирован в неопределённом состоянии!

// Хорошо — передавай указатель
s := &Safe{}
process(s)
```

`go vet` и `go vet ./...` поймает: `assignment copies lock value`.

### 2. Lock без Unlock (deadlock)

```go
func bad(mu *sync.Mutex) {
    mu.Lock()
    if someCondition {
        return // УТЕЧКА: mutex остался locked
    }
    mu.Unlock()
}

// Хорошо — defer гарантирует unlock при любом выходе
func good(mu *sync.Mutex) {
    mu.Lock()
    defer mu.Unlock()
    if someCondition {
        return // OK: defer выполнится
    }
}
```

### 3. Разблокировка без блокировки

```go
var mu sync.Mutex
mu.Unlock() // panic: sync: unlock of unlocked mutex
```

### 4. Рекурсивный Lock (deadlock)

```go
func (s *Safe) A() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.B() // DEADLOCK: B попытается взять уже захваченный mutex
}

func (s *Safe) B() {
    s.mu.Lock()
    defer s.mu.Unlock()
    // ...
}

// Решение: unexported helper без lock
func (s *Safe) b() { /* логика без lock */ }
func (s *Safe) B() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.b()
}
```

---

## `sync.WaitGroup`

Ждёт завершения группы горутин.

```go
var wg sync.WaitGroup

for i, task := range tasks {
    wg.Add(1)               // Add ПЕРЕД запуском горутины
    go func(t Task) {
        defer wg.Done()     // Done при выходе
        process(t)
    }(task)
}

wg.Wait()                   // блокируется пока счётчик не стал 0
```

**Почему `Add` перед `go`?**

```go
// Плохо: race condition
for _, task := range tasks {
    go func(t Task) {
        wg.Add(1)   // если main достигнет Wait() до этой строки — не дождётся горутины
        defer wg.Done()
        process(t)
    }(task)
}

// Правильно: Add в calling goroutine, перед запуском
for _, task := range tasks {
    wg.Add(1)
    go func(t Task) {
        defer wg.Done()
        process(t)
    }(task)
}
```

### WaitGroup + closer goroutine

```go
out := make(chan Result)
var wg sync.WaitGroup

for _, id := range ids {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        out <- fetch(id)
    }(id)
}

// Закрываем out когда все workers завершились
go func() {
    wg.Wait()
    close(out)
}()

for r := range out {
    process(r)
}
```

---

## `sync.Once`

Выполняет функцию ровно один раз, независимо от количества вызывающих горутин.

```go
var (
    instance *Config
    once     sync.Once
)

func GetConfig() *Config {
    once.Do(func() {
        instance = loadConfig() // вызовется только один раз
    })
    return instance
}
```

### Паника внутри Once

Если переданная функция паникует, `once.Do` считается **выполненным**. Повторный вызов `Do` не будет вызван.

```go
var once sync.Once
var initialized bool

once.Do(func() {
    panic("init failed") // паника!
})

once.Do(func() {
    initialized = true // НИКОГДА не выполнится
})
// initialized остаётся false
```

Следствие: если инициализация может провалиться, используй другой паттерн:

```go
var (
    instance *DB
    initErr  error
    once     sync.Once
)

func getDB() (*DB, error) {
    once.Do(func() {
        instance, initErr = connectDB()
    })
    return instance, initErr
}
```

---

## `sync.Cond`

Условная переменная: горутина ждёт пока **условие** не станет истинным.

```go
type Queue struct {
    mu    sync.Mutex
    cond  *sync.Cond
    items []int
}

func NewQueue() *Queue {
    q := &Queue{}
    q.cond = sync.NewCond(&q.mu)
    return q
}

func (q *Queue) Push(v int) {
    q.mu.Lock()
    q.items = append(q.items, v)
    q.mu.Unlock()
    q.cond.Signal() // разбудить одну ожидающую горутину
}

func (q *Queue) Pop() int {
    q.mu.Lock()
    defer q.mu.Unlock()
    for len(q.items) == 0 {
        q.cond.Wait() // атомарно: unlock mu + ждёт сигнала + lock mu
    }
    v := q.items[0]
    q.items = q.items[1:]
    return v
}
```

### `Signal` vs `Broadcast`

- `Signal()` — будит **одну** горутину (если несколько ждут — случайную)
- `Broadcast()` — будит **все** ожидающие горутины

### Когда `sync.Cond`, когда channel

| | `sync.Cond` | Channel |
|---|---|---|
| Broadcast ("все проснитесь") | ✅ `Broadcast()` | ✅ `close(done)` |
| Передача данных | ❌ (только сигнал) | ✅ |
| Сложное условие ожидания | ✅ for-loop с проверкой | ⚠️ сложнее |
| Производительность | быстрее при высокой конкуренции | overhead на channel ops |

**Практически** — `sync.Cond` используется редко. В большинстве случаев channel понятнее. `Cond` нужен когда нужно Broadcast + состояние защищено тем же mutex.

---

## `sync.Pool`

Пул объектов для переиспользования, снижает аллокации.

```go
var bufPool = sync.Pool{
    New: func() any {
        return make([]byte, 0, 4096) // создаётся только при пустом pool
    },
}

func encode(data []byte) string {
    buf := bufPool.Get().([]byte)
    defer func() {
        buf = buf[:0]       // сбросить длину, сохранить capacity
        bufPool.Put(buf)    // вернуть в pool
    }()
    
    buf = append(buf, data...)
    // ... encode
    return string(buf)
}
```

### Поведение при GC

`sync.Pool` — **не постоянное хранилище**. Объекты могут быть удалены GC в любой момент (обычно на каждом GC цикле). Не используй Pool для хранения state между запросами.

### Когда Pool полезен

- Шорт-лайфовые объекты с высокой аллокацией (буферы, временные структуры)
- HTTP/RPC handlers: пул буферов для encode/decode
- `fmt.Fprintf` в stdlib использует `sync.Pool` для буферов

### Когда Pool вреден

- Объект требует инициализации состояния (риск "dirty" объектов)
- Объект хранит ресурсы (connections, file handles) — используй явный pool с close
- Редкие аллокации — overhead Pool выше чем прямая аллокация

---

## `sync.Map`

Потокобезопасный map, оптимизированный для двух конкретных use case.

```go
var m sync.Map

// Store
m.Store("key", "value")

// Load
v, ok := m.Load("key")
if ok {
    fmt.Println(v.(string))
}

// LoadOrStore — atomic get-or-set
actual, loaded := m.LoadOrStore("key", "default")

// Delete
m.Delete("key")

// Range — итерация (не safe для concurrent modification)
m.Range(func(k, v any) bool {
    fmt.Println(k, v)
    return true // false = stop iteration
})
```

### Когда `sync.Map`, когда `map + mutex`

`sync.Map` оптимален **только** для:
1. Ключ пишется один раз, потом только читается (append-only)
2. Разные горутины читают/пишут **непересекающиеся** наборы ключей

Во всех остальных случаях — `map + sync.RWMutex` быстрее:

```go
// Типичная read-heavy cache — RWMutex быстрее
type Cache struct {
    mu sync.RWMutex
    m  map[string]Entry
}
```

**Почему**: `sync.Map` использует два map (read/dirty) + атомарные операции + mutex для dirty. При частых записях dirty map постоянно промотируется → хуже, чем прямой RWMutex.

---

## `atomic` — операции без mutex

Атомарные операции на примитивных типах без lock overhead.

```go
import "sync/atomic"

var counter int64

atomic.AddInt64(&counter, 1)              // increment
val := atomic.LoadInt64(&counter)         // read
atomic.StoreInt64(&counter, 0)            // write
old := atomic.SwapInt64(&counter, 100)    // swap
swapped := atomic.CompareAndSwapInt64(&counter, old, old+1) // CAS
```

### `atomic.Value` для произвольных типов

```go
var config atomic.Value

// Store (всегда один и тот же конкретный тип!)
config.Store(&Config{MaxConn: 100})

// Load
cfg := config.Load().(*Config)
```

**Главное ограничение**: всегда сохраняй один и тот же конкретный тип — паника если типы разные.

### Когда atomic, когда mutex

| | `atomic` | `mutex` |
|---|---|---|
| Простые счётчики, флаги | ✅ | ❌ overhead |
| Произвольные структуры | ❌ | ✅ |
| CAS-循环 (оптимистичные обновления) | ✅ | ❌ |
| Несколько полей атомарно | ❌ | ✅ |
| Lock-free алгоритмы | ✅ | ❌ |

**Практическое правило**: atomic для int-счётчиков и флагов (isRunning, requestCount). Для структур с несколькими полями — mutex.

```go
// Хороший use case: feature flag check (часто читается, редко пишется)
var featureEnabled atomic.Bool

// check (hot path — миллионы раз в секунду)
if featureEnabled.Load() {
    doNewBehavior()
}

// update (редко)
featureEnabled.Store(true)
```

### `atomic.Bool`, `atomic.Int64`, etc. (Go 1.19+)

```go
var running atomic.Bool
running.Store(true)
if running.Load() { ... }
running.CompareAndSwap(true, false) // атомарный CAS
```

---

## Шпаргалка: выбор примитива

```
Нужно передать данные между goroutines?
  → channel

Нужно защитить shared state?
  Reads >> Writes (>80% reads)?
    → sync.RWMutex
  Иначе?
    → sync.Mutex

Нужна ленивая инициализация?
  → sync.Once

Нужен pool временных объектов?
  → sync.Pool

Нужны счётчики/флаги без lock?
  → sync/atomic

Нужен thread-safe map (read-heavy, разные ключи)?
  → sync.Map
  Иначе:
  → map + sync.RWMutex
```

---

## Interview-ready answer

**Q: Когда RWMutex лучше Mutex?**

RWMutex выгоден когда операций чтения значительно больше (>80%), а сами критические секции занимают хоть какое-то время. При высоком write-давлении RWMutex хуже — каждый write ждёт всех активных reads, что создаёт голодание.

**Q: Как работает sync.Once и что будет при панике внутри?**

`sync.Once.Do` выполняет функцию ровно один раз — атомарно проверяет флаг, при необходимости берёт lock и вызывает функцию. Если функция паникует, `Do` всё равно считается вызванным — повторный вызов игнорируется. Поэтому в инициализаторе, который может упасть, нужно возвращать ошибку через замыкание, а не паниковать.

**Q: sync.Map vs map + RWMutex?**

`sync.Map` быстрее только в двух сценариях: ключ пишется один раз и только читается, или горутины работают с непересекающимися наборами ключей. В общем случае `map + RWMutex` быстрее, потому что `sync.Map` поддерживает два внутренних map и при частых writes постоянно их промотирует.
