# Sync Primitives

Пакет `sync` и `sync/atomic` — низкоуровневые примитивы синхронизации. Каналы хороши для передачи данных; `sync` — для защиты состояния.

## Содержание

- [`sync.Mutex` vs `sync.RWMutex`](#syncmutex-vs-syncrwmutex)
- [Типичные ошибки с Mutex](#типичные-ошибки-с-mutex)
- [`sync.WaitGroup`](#syncwaitgroup)
- [`sync.Once`](#synconce)
- [`sync.Cond`](#synccond)
- [`sync.Pool`](#syncpool)
- [`sync.Map`](#syncmap)
- [`atomic` — операции без mutex](#atomic--операции-без-mutex)
- [`singleflight` — дедупликация одинаковых запросов](#singleflight--дедупликация-одинаковых-запросов)
- [Шпаргалка: выбор примитива](#шпаргалка-выбор-примитива)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

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

Атомарные операции на примитивных типах без lock overhead. Реализованы аппаратными инструкциями CPU (`LOCK XADD`, `CMPXCHG` на x86), а не блокировкой.

### Пять механик (плюс And/Or в 1.23)

Все атомарные операции сводятся к нескольким примитивам:

| Операция | Что делает атомарно | Возвращает |
|---|---|---|
| `Load` | прочитать значение | значение |
| `Store(new)` | записать значение | — |
| `Swap(new)` | записать new | **старое** значение |
| `Add(delta)` | прибавить delta (можно отрицательный) | **новое** значение |
| `CompareAndSwap(old, new)` | если текущее == old → записать new | `bool` (удалось ли) |
| `And(mask)` / `Or(mask)` ¹ | побитовые `&` / `\|` | старое значение |

¹ — `And`/`Or` добавлены в Go 1.23 (`atomic.AndInt64`, метод `.And()` и т.п.).

**CAS** (Compare-And-Swap) — фундамент всех lock-free алгоритмов: «поменяй, только если значение не изменилось с тех пор, как я его прочитал».

### Два API: функции vs типизированные типы

```go
// Старый API — функции на *T (есть всегда):
var counter int64
atomic.AddInt64(&counter, 1)
val := atomic.LoadInt64(&counter)
atomic.CompareAndSwapInt64(&counter, val, val+1)

// Новый API — типизированные обёртки (Go 1.19+), предпочтительнее:
var counter2 atomic.Int64
counter2.Add(1)
val2 := counter2.Load()
counter2.CompareAndSwap(val2, val2+1)
```

Типизированный API лучше: нельзя случайно обратиться к переменной **не**атомарно (поле приватное), и не нужно следить за выравниванием 64-бит на 32-bit платформах (старый `atomic.*Int64` требовал 8-байтного выравнивания вручную).

### Типизированные типы (Go 1.19+)

| Тип | Для чего | `Add` | `CAS` | `And/Or` (1.23) |
|---|---|:---:|:---:|:---:|
| `atomic.Int32` / `Int64` | знаковые счётчики | ✅ | ✅ | ✅ |
| `atomic.Uint32` / `Uint64` / `Uintptr` | беззнаковые, битмаски | ✅ | ✅ | ✅ |
| `atomic.Bool` | флаги | ❌ | ✅ | ❌ |
| `atomic.Pointer[T]` | указатели, **типобезопасно** | ❌ | ✅ | ❌ |
| `atomic.Value` | произвольный тип (один!) | ❌ | ✅ | ❌ |

### CAS-loop: оптимистичные обновления

Когда нужной операции нет (нет `atomic.Mul`, атомарного max и т.п.) — делают через **CAS в цикле**: прочитать, посчитать, попытаться записать; если кто-то опередил — повторить.

```go
var n atomic.Int64

// атомарно: n = max(n, x)
func atomicMax(n *atomic.Int64, x int64) {
    for {
        old := n.Load()
        if x <= old {
            return // уже не меньше
        }
        if n.CompareAndSwap(old, x) {
            return // успех
        }
        // кто-то изменил n между Load и CAS → повторяем с новым old
    }
}
```

Это и есть **lock-free**: без блокировок, прогресс гарантирован (хотя отдельная горутина может «крутиться» при высокой конкуренции).

### `atomic.Pointer[T]` и `atomic.Value`

Для атомарной подмены целого объекта (hot-reload конфига, lock-free снапшот):

```go
// Современно (Go 1.19+): типобезопасно, без приведений и паник
var cfg atomic.Pointer[Config]
cfg.Store(&Config{MaxConn: 100})
c := cfg.Load() // *Config, без .(...)

// Старое: atomic.Value — хранит any, ПАНИКА если Store другого типа
var cfgv atomic.Value
cfgv.Store(&Config{MaxConn: 100})
c2 := cfgv.Load().(*Config) // нужно приведение
```

`atomic.Pointer[T]` почти всегда лучше `atomic.Value`: компилятор не даст положить другой тип. Паттерн чтения-без-локов: писатель готовит новый `*Config` и `Store`-ит его одним атомарным действием; читатели делают `Load()` без блокировки.

### Когда atomic, когда mutex

| | `atomic` | `mutex` |
|---|---|---|
| Простые счётчики, флаги | ✅ | ❌ overhead |
| Произвольные структуры | ❌ | ✅ |
| CAS-loop (оптимистичные обновления) | ✅ | ❌ |
| Несколько полей **согласованно** | ❌ | ✅ |
| Атомарная подмена одного указателя | ✅ `Pointer[T]` | ⚠️ можно, но дороже |
| Lock-free алгоритмы | ✅ | ❌ |

**Правило:** atomic — для одиночных счётчиков/флагов/указателей. Как только нужно поменять **несколько полей согласованно** — это уже инвариант, его держит mutex.

```go
// Хороший use case: feature flag (часто читается, редко пишется)
var featureEnabled atomic.Bool
if featureEnabled.Load() { // hot path, миллионы раз/сек
    doNewBehavior()
}
featureEnabled.Store(true) // редко
```

---

## `singleflight` — дедупликация одинаковых запросов

`golang.org/x/sync/singleflight` — не из stdlib, но де-факто стандарт. Решает **cache stampede / thundering herd**: когда N горутин одновременно промахиваются по кэшу и идут за **одним и тем же** ключом, реально выполняется **один** вызов, остальные ждут и получают тот же результат.

```go
import "golang.org/x/sync/singleflight"

type Service struct {
    sf    singleflight.Group
    cache *Cache
}

func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
    if u, ok := s.cache.Get(id); ok {
        return u, nil
    }
    // Do: для одного id реально выполнится ЛИШЬ у первой горутины,
    // остальные с тем же id заблокируются и получат тот же (v, err)
    v, err, shared := s.sf.Do(id, func() (any, error) {
        u, err := loadUserFromDB(ctx, id) // дорогой запрос — один на всех
        if err == nil {
            s.cache.Set(id, u)
        }
        return u, err
    })
    _ = shared // true, если результат разделён с другими вызовами
    if err != nil {
        return nil, err
    }
    return v.(*User), nil
}
```

### Методы

| Метод | Поведение |
|---|---|
| `Do(key, fn)` | блокирующий; дедуплицирует по key; `(v, err, shared)` |
| `DoChan(key, fn)` | возвращает `<-chan Result` — для `select` с `ctx.Done()` |
| `Forget(key)` | убрать in-flight ключ (например, чтобы не «залипла» ошибка) |

### Подводные камни

- **Общий результат на всех.** Все ждущие получают **один и тот же** `v` и `err`. Если `fn` вернула ошибку — её получат все. Если `v` — мутабельный объект, его нельзя менять у получателей (это общий указатель).
- **Отмена контекста.** `ctx` замыкается внутри `fn` от **первого** вызывающего. Если он отменится, упадут все ждущие. Когда у каждого свой дедлайн — используй `DoChan` + `select { case r := <-ch: case <-ctx.Done(): }`, чтобы конкретный вызывающий мог уйти по своему ctx.
- **Head-of-line blocking.** Зависший `fn` держит всех ждущих этого ключа. `DoChan` + таймаут на стороне вызывающего спасает.
- **Кэширование ошибок.** `Do` не кэширует результат сам — он лишь дедуплицирует *одновременные* вызовы. Между «волнами» кэшировать надо самому (как выше). `Forget(key)` помогает сбросить, если первый вызов завершился ошибкой и не нужно её «расшаривать» следующей волне.

Разбор реализации и задача — в [coding-tasks/concurrency/06-singleflight](../../12-interview-practice/coding-tasks/concurrency/06-singleflight.md).

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

Нужна атомарная подмена целого объекта (hot-reload)?
  → atomic.Pointer[T]

N горутин запрашивают один ключ (cache stampede)?
  → singleflight

Нужен thread-safe map (read-heavy, разные ключи)?
  → sync.Map
  Иначе:
  → map + sync.RWMutex
```

---

## Разбор примеров-загадок

### Загадка 1: RLock → Lock в одной горутине

```go
var mu sync.RWMutex
mu.RLock()
mu.Lock()   // ?
```

<details>
<summary>Ответ</summary>

```
deadlock — fatal error: all goroutines are asleep
```

`Lock()` ждёт освобождения **всех** read-локов, но `RUnlock()` в этой же горутине так и не наступит — она застряла на `Lock()`. RWMutex **не поддерживает upgrade** RLock→Lock.

Коварнее: даже рекурсивный `RLock()` может зависнуть. Если между двумя `RLock()` одной горутины другой поток вызвал `Lock()`, второй `RLock()` встанет в очередь **за** ожидающим писателем (чтобы не морить его голодом) → взаимная блокировка. Вывод: не бери RWMutex повторно/рекурсивно.
</details>

---

### Загадка 2: лишний Done → паника

```go
var wg sync.WaitGroup
wg.Add(1)
go func() {
    wg.Done()
    wg.Done()  // ?
}()
wg.Wait()
```

<details>
<summary>Ответ</summary>

```
panic: sync: negative WaitGroup counter
```

Счётчик ушёл ниже нуля → паника. Частые причины: лишний `Done()`, `Add` внутри горутины (гонка с `Wait`), повторное использование `wg` до полного `Wait`. Правило: число `Add` строго равно числу `Done`, `Add` — в вызывающей горутине **до** `go`.
</details>

---

### Загадка 3: sync.Once + паника «съедает» инициализацию

```go
var once sync.Once
func tryInit() {
    defer func() { recover() }()
    once.Do(func() { panic("boom") })
}

func main() {
    tryInit()
    once.Do(func() { fmt.Println("init") })  // ?
    fmt.Println("end")
}
```

<details>
<summary>Ответ</summary>

```
end
```

`"init"` **не печатается**. Первый `Do` запаниковал, но Once уже пометил себя выполненным — любой следующий `Do` игнорируется навсегда. Поэтому в инициализаторе, который может упасть, нельзя паниковать «вникуда»: возвращай ошибку через замыкание (`instance, err = ...`) и решай повторять или нет на уровне выше.
</details>

---

### Загадка 4: atomic + обычный доступ к одной переменной

```go
var counter int64
go func() { atomic.AddInt64(&counter, 1) }()
go func() { counter++ }()   // ?
```

<details>
<summary>Ответ</summary>

`go test -race` репортит **data race**, даже что один из доступов «атомарный». Атомарность работает, только если **все** обращения к переменной идут через `atomic`. Смешивание `atomic.AddInt64` и обычного `counter++` = гонка (компилятор/CPU может переупорядочить, прочитать рваное значение).

Правило: переменную под `atomic` трогают **исключительно** через `atomic.*` (или новый типизированный `atomic.Int64` — он не даёт случайно обратиться напрямую).
</details>

---

### Загадка 5: atomic.Value и смена типа

```go
var v atomic.Value
v.Store(1)
v.Store("hello")  // ?
```

<details>
<summary>Ответ</summary>

```
panic: sync/atomic: store of inconsistently typed value into Value
```

`atomic.Value` требует, чтобы во всех `Store` лежал **один конкретный тип**. Частая ловушка — хранить интерфейс с разными динамическими типами или забыть, что `nil` тоже типизирован. Для конфигов кладут всегда `*Config` (один тип), для гибкости — оборачивают в структуру-холдер.
</details>

---

### Загадка 6: defer Unlock и долгая работа под локом

```go
func (c *Cache) GetOrLoad(key string) Value {
    c.mu.Lock()
    defer c.mu.Unlock()
    if v, ok := c.m[key]; ok {
        return v
    }
    v := slowLoadFromDB(key)  // ?  держим Lock всё это время
    c.m[key] = v
    return v
}
```

<details>
<summary>Ответ</summary>

Баг производительности: `defer c.mu.Unlock()` держит **эксклюзивный** лок на всё время `slowLoadFromDB` — остальные горутины (даже читатели) стоят. Под глобальным локом нельзя делать I/O / медленные вызовы.

Фиксы: загружать вне лока (double-checked), либо `singleflight` чтобы не грузить один ключ N раз. `defer` удобен, но «растягивает» критическую секцию до конца функции — иногда нужен ручной `Unlock()` раньше.
</details>

---

## Interview-ready answer

**1. Когда RWMutex лучше Mutex?**
Когда чтений ≫ записей (ориентир >80%) и критическая секция не однострочная. При частых записях RWMutex хуже: писатель ждёт всех читателей, плюс свой overhead. Короткая секция → обычный Mutex быстрее.

**2. RWMutex реентерабельный?**
Нет. RLock→Lock в одной горутине — дедлок (нет upgrade). Рекурсивный RLock тоже опасен: второй RLock может встать за ожидающим писателем. Не бери лок повторно.

**3. Как работает sync.Once и что при панике?**
`Do` выполняет функцию ровно раз (быстрый atomic-флаг + lock на медленном пути). Паника внутри всё равно помечает Once выполненным — следующий `Do` не запустится. Для падающей инициализации возвращай ошибку, не паникуй.

**4. atomic или mutex?**
atomic — для одиночных счётчиков/флагов (lock-free, дёшево). Mutex — для нескольких полей «атомарно» или произвольных структур. Нельзя смешивать atomic и прямой доступ к одной переменной — это гонка. С Go 1.19 — типизированные `atomic.Int64/Bool/Pointer`.

**5. sync.Map vs map + RWMutex?**
`sync.Map` выигрывает в двух сценариях: append-only (ключ пишется раз, дальше читается) или непересекающиеся наборы ключей у горутин. Иначе `map + RWMutex` быстрее — `sync.Map` держит два внутренних map и при частых записях постоянно их промотирует.

**6. Почему нельзя копировать sync-типы?**
`Mutex`, `WaitGroup`, `Once`, `Cond` содержат внутреннее состояние; копия = рассинхрон (lock на копии не блокирует оригинал). `go vet -copylocks` ловит «passes lock by value» — всегда передавай указателем.

**7. Что такое CAS и зачем CAS-loop?**
CAS (`CompareAndSwap`) — «запиши new, только если значение всё ещё == old». Это база lock-free алгоритмов. Когда нужной атомарной операции нет (max, умножение, обновление структуры по указателю), её делают через CAS в цикле: Load → посчитать → CompareAndSwap → при неудаче повторить. Прогресс гарантирован без блокировок.

**8. atomic.Pointer vs atomic.Value?**
Оба атомарно подменяют объект целиком (hot-reload конфига, lock-free снапшот). `atomic.Pointer[T]` (1.19+) типобезопасен — компилятор не даст положить другой тип. `atomic.Value` хранит `any` и **паникует**, если `Store` другого динамического типа. Предпочитай `Pointer[T]`.

**9. Что такое singleflight и зачем?**
`golang.org/x/sync/singleflight` дедуплицирует одновременные запросы по ключу: при cache stampede (N горутин промахнулись по одному ключу) реально выполняется один вызов, остальные ждут и получают тот же результат. Подводные камни: общий `err` на всех, отмена ctx первого вызывающего валит всех (нужен `DoChan` + свой `select`), head-of-line blocking.
