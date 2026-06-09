# Worker Pool

Worker pool — один из самых частых паттернов в Go-коде: bounded concurrency для CPU-bound или I/O-bound задач. Здесь — разбор реального баг-репорта с собеседования и правильная реализация.

## Содержание

- [Разбор задачи с собеседования: task_before.go](#разбор-задачи-с-собеседования-task_beforego)
- [Правильная реализация: task_after_gpt.go](#правильная-реализация-task_after_gptgo)
- [Worker pool шаблон — универсальный](#worker-pool-шаблон--универсальный)
- [Паттерн errCh: `chan error, 1` — первая ошибка wins](#паттерн-errch-chan-error-1--первая-ошибка-wins)
- [Graceful shutdown: ctx.Done() в producer и workers](#graceful-shutdown-ctxdone-в-producer-и-workers)
- [Semaphore через buffered channel как альтернатива pool](#semaphore-через-buffered-channel-как-альтернатива-pool)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## Разбор задачи с собеседования: task_before.go

Вот код, который нужно было проверить на собеседовании:

```go
type Fetcher struct {
    cache map[int]Result
}

func (f *Fetcher) FetchAll(ids []int) chan Result {
    var out chan Result   // BUG 1
    var jobs chan int     // BUG 2

    go func() {
        defer close(jobs) // BUG 3: close nil channel → panic
        for _, id := range ids {
            jobs <- id    // отправка в nil channel → goroutine leak + panic
        }
    }()

    for i := 0; i < 4; i++ {
        go func(worker int) {
            for id := range jobs {       // range nil channel — блокируется вечно
                r, ok := f.cache[id]     // BUG 4: race condition
                if ok {
                    out <- r             // BUG 1: send to nil channel → panic
                    continue
                }
                r = f.doRequest(id)
                f.cache[id] = r          // BUG 4: race condition
                out <- r                 // BUG 1: send to nil channel → panic
            }
        }(i)
    }

    return out // возвращает nil
}
```

### Баг 1: nil channels — `var out chan Result` и `var jobs chan int`

```go
var out chan Result  // nil
var jobs chan int    // nil

// Отправка в nil channel → goroutine БЛОКИРУЕТСЯ ВЕЧНО (не panic)
jobs <- id  // горутина висит навсегда → leak

// Получение из nil channel → блокируется вечно
for id := range jobs { ... }  // никогда не получит данных

// close nil channel → PANIC
close(jobs) // panic: close of nil channel
```

**Правило**: `make(chan T)` или `make(chan T, n)` — всегда инициализируй каналы перед использованием. `var ch chan T` — это `nil`, не пустой канал.

```go
// Правильно
out := make(chan Result, workers)
jobs := make(chan int)
```

### Баг 2: Race condition на `f.cache`

```go
// 4 воркера читают и пишут в один map без mutex
r, ok := f.cache[id]  // concurrent read
f.cache[id] = r        // concurrent write — DATA RACE
```

Go race detector (`go test -race`) поймает это немедленно. Одновременная запись в map → **undefined behavior**, возможен crash рантайма.

```go
// Правильно: sync.RWMutex для cache
type Fetcher struct {
    mu    sync.RWMutex
    cache map[int]Result
}

// Read (fast path)
f.mu.RLock()
r, ok := f.cache[id]
f.mu.RUnlock()

// Write
f.mu.Lock()
f.cache[id] = r
f.mu.Unlock()
```

### Баг 3: Нет WaitGroup → out никогда не закрывается

```go
// Воркеры запущены, но FetchAll немедленно возвращает out (nil!)
// Никто не ждёт завершения воркеров
// out никогда не закрывается
// range out в вызывающем коде зависнет навсегда

for r := range f.FetchAll(ids) { // range nil channel → deadlock
    fmt.Println(r)
}
```

**Правило**: Кто открыл — тот и закрывает. Для нескольких writers нужен WaitGroup + отдельная горутина-closer.

### Баг 4: Context создан, но не передан в FetchAll

```go
// В main:
ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
defer cancel()

for r := range f.FetchAll(ids) { // ctx не передаётся!
```

Timeout создан, но не используется. `doRequest` не знает о таймауте и не может быть отменён.

### Итог: 5 независимых багов

| # | Баг | Симптом | Решение |
|---|---|---|---|
| 1a | `var out chan Result` — nil channel | range → deadlock, close → panic | `make(chan Result, workers)` |
| 1b | `var jobs chan int` — nil channel | goroutine leak, close → panic | `make(chan int)` |
| 2 | race condition на `f.cache` | crash/data race | `sync.RWMutex` |
| 3 | нет WaitGroup | `out` никогда не закрыт → range deadlock | `sync.WaitGroup` + closer |
| 4 | ctx создан, не передан | timeout игнорируется | `FetchAll(ctx, ids)` |

---

## Правильная реализация: task_after_gpt.go

```go
type Fetcher struct {
    mu    sync.RWMutex
    cache map[int]Result
}

func NewFetcher() *Fetcher {
    return &Fetcher{cache: make(map[int]Result)} // инициализируем map!
}

// fetch — отменяемый IO через context
func (f *Fetcher) fetch(ctx context.Context, id int) (Result, error) {
    select {
    case <-time.After(50 * time.Millisecond):
        return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()
    }
}

// FetchAll — безопасная worker-pool реализация
func (f *Fetcher) FetchAll(ctx context.Context, ids []int, workers int) (<-chan Result, <-chan error) {
    if workers <= 0 {
        workers = 1
    }

    out := make(chan Result, workers) // небольшой буфер — снижает coupling
    errCh := make(chan error, 1)      // первая ошибка wins

    jobs := make(chan int)

    // Producer: отправляет IDs или останавливается при отмене
    go func() {
        defer close(jobs)
        for _, id := range ids {
            select {
            case jobs <- id:
            case <-ctx.Done():
                return
            }
        }
    }()

    var wg sync.WaitGroup
    wg.Add(workers)

    workerFn := func() {
        defer wg.Done()
        for {
            select {
            case <-ctx.Done():
                return
            case id, ok := <-jobs:
                if !ok {
                    return // channel closed — jobs исчерпаны
                }

                // Fast path: проверяем кэш
                f.mu.RLock()
                r, ok := f.cache[id]
                f.mu.RUnlock()
                if ok {
                    select {
                    case out <- r:    // отправляем результат
                    case <-ctx.Done(): // или уходим при отмене
                        return
                    }
                    continue
                }

                // Slow path: реальный запрос
                r, err := f.fetch(ctx, id)
                if err != nil {
                    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
                        return // тихий выход при отмене ctx
                    }
                    select {
                    case errCh <- err: // первая ошибка в канал
                    default:           // остальные дропаются
                    }
                    return
                }

                // Записываем в кэш
                f.mu.Lock()
                f.cache[id] = r
                f.mu.Unlock()

                select {
                case out <- r:
                case <-ctx.Done():
                    return
                }
            }
        }
    }

    for i := 0; i < workers; i++ {
        go workerFn()
    }

    // Closer: закрывает каналы когда все воркеры завершились
    go func() {
        wg.Wait()
        close(out)
        close(errCh)
    }()

    return out, errCh
}

// Использование
func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
    defer cancel()

    f := NewFetcher()
    out, errCh := f.FetchAll(ctx, []int{1, 2, 3, 2, 4, 5, 6}, 4)

    for r := range out {
        fmt.Println("result:", r)
    }

    if err := <-errCh; err != nil {
        fmt.Println("error:", err)
    }
}
```

---

## Worker pool шаблон — универсальный

```go
// WorkerPool — параметризованный worker pool
func WorkerPool[T, R any](
    ctx context.Context,
    jobs []T,
    workers int,
    fn func(ctx context.Context, job T) (R, error),
) (<-chan R, <-chan error) {
    out := make(chan R, workers)
    errCh := make(chan error, 1)
    jobsCh := make(chan T)

    // Producer
    go func() {
        defer close(jobsCh)
        for _, j := range jobs {
            select {
            case jobsCh <- j:
            case <-ctx.Done():
                return
            }
        }
    }()

    var wg sync.WaitGroup
    wg.Add(workers)

    for range workers {
        go func() {
            defer wg.Done()
            for {
                select {
                case <-ctx.Done():
                    return
                case job, ok := <-jobsCh:
                    if !ok {
                        return
                    }
                    r, err := fn(ctx, job)
                    if err != nil {
                        select {
                        case errCh <- err:
                        default:
                        }
                        return
                    }
                    select {
                    case out <- r:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }()
    }

    go func() {
        wg.Wait()
        close(out)
        close(errCh)
    }()

    return out, errCh
}
```

---

## Паттерн errCh: `chan error, 1` — первая ошибка wins

```go
errCh := make(chan error, 1) // буфер = 1

// В горутине: неблокирующая отправка
select {
case errCh <- err:  // первая запишет
default:            // остальные дропаются — НЕ БЛОКИРУЮТСЯ
}

// После завершения горутин: проверка
if err := <-errCh; err != nil {
    return err
}
```

**Почему буфер именно 1?**
- `make(chan error)` — unbuffered: горутина заблокируется если никто не читает → **leak**
- `make(chan error, 1)` — первая ошибка записывается без блокировки, остальные дропаются через `default`
- `make(chan error, n)` — можно собрать несколько ошибок (но обычно нужна только первая)

---

## Graceful shutdown: ctx.Done() в producer и workers

```go
// Producer уважает ctx
go func() {
    defer close(jobs)
    for _, id := range ids {
        select {
        case jobs <- id:
        case <-ctx.Done():
            return // прекращаем подавать задачи
        }
    }
}()

// Worker уважает ctx в двух местах:
// 1. Перед чтением из jobs
// 2. Перед отправкой в out
for {
    select {
    case <-ctx.Done():
        return
    case id, ok := <-jobs:
        if !ok { return }
        // обработка...
        select {
        case out <- result:
        case <-ctx.Done():
            return
        }
    }
}
```

При отмене ctx:
1. Producer перестаёт отправлять в jobs и закрывает jobs
2. Workers получают `ctx.Done()` или читают из закрытого jobs → exit
3. Closer горутина видит `wg.Wait()` → закрывает out и errCh
4. Consumer читает оставшиеся значения из out и выходит из range

---

## Semaphore через buffered channel как альтернатива pool

```go
type Semaphore struct {
    ch chan struct{}
}

func NewSemaphore(n int) *Semaphore {
    return &Semaphore{ch: make(chan struct{}, n)}
}

func (s *Semaphore) Acquire(ctx context.Context) error {
    select {
    case s.ch <- struct{}{}: // берём токен
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Semaphore) Release() {
    <-s.ch // возвращаем токен
}

// Использование: не более 10 параллельных HTTP запросов
sem := NewSemaphore(10)
for _, url := range urls {
    if err := sem.Acquire(ctx); err != nil {
        break
    }
    go func(u string) {
        defer sem.Release()
        fetch(u)
    }(url)
}
```

**Semaphore vs Worker Pool:**

| | Worker Pool | Semaphore |
|---|---|---|
| Управление горутинами | фиксированный пул | новая горутина на задачу |
| Memory overhead | меньше (N горутин) | больше (M горутин) |
| Порядок завершения | предсказуемый | нет |
| Use case | долгие задачи | короткие burst задачи |

---

## Разбор примеров-загадок

### Загадка 1: closer не в отдельной горутине → deadlock

```go
func squares(nums []int) []int {
    out := make(chan int)        // unbuffered
    var wg sync.WaitGroup
    for _, n := range nums {
        wg.Add(1)
        go func(x int) {
            defer wg.Done()
            out <- x * x         // ?
        }(n)
    }
    wg.Wait()                    // ?
    close(out)

    var res []int
    for v := range out {
        res = append(res, v)
    }
    return res
}
```

<details>
<summary>Ответ</summary>

```
fatal error: all goroutines are asleep - deadlock!
```

Воркеры блокируются на `out <- x*x` (канал unbuffered, читателя ещё нет — `range out` идёт **после** `wg.Wait()`). Значит `wg.Done()` не вызывается → `wg.Wait()` висит вечно → дедлок.

Закрытие/ожидание должно идти **параллельно** чтению:
```go
go func() { wg.Wait(); close(out) }()  // closer в отдельной горутине
for v := range out { res = append(res, v) }  // читаем одновременно
```
Либо буферизировать `out` под все значения. Это ровно то, что делает «closer goroutine» в правильной реализации выше.
</details>

---

### Загадка 2: producer без ctx-select течёт при ранней отмене

```go
jobs := make(chan int)
go func() {
    defer close(jobs)
    for _, id := range ids {
        jobs <- id            // ?  нет select на ctx.Done()
    }
}()

for id := range jobs {
    if shouldStop() {
        break                 // consumer ушёл раньше
    }
    process(id)
}
```

<details>
<summary>Ответ</summary>

Если consumer вышел из `range` раньше (break, ctx cancel, ошибка), а `ids` ещё не исчерпан — producer навсегда виснет на `jobs <- id` (читателя нет). Горутина-producer + `defer close` не выполнятся → **goroutine leak**.

Producer обязан уважать отмену:
```go
select {
case jobs <- id:
case <-ctx.Done():
    return
}
```
</details>

---

### Загадка 3: fan-out не дублирует, а делит работу

```go
jobs := make(chan int)
go func() { defer close(jobs); for i := 1; i <= 6; i++ { jobs <- i } }()

var wg sync.WaitGroup
for w := 0; w < 3; w++ {           // 3 воркера читают ОДИН канал
    wg.Add(1)
    go func() { defer wg.Done(); for j := range jobs { _ = j } }()
}
wg.Wait()
```

<details>
<summary>Ответ</summary>

Каждое значение из `jobs` получает **ровно один** воркер — Go гарантирует, что отправленное значение примет только одна горутина. 6 задач делятся между 3 воркерами (≈по 2), а **не** обрабатываются трижды.

Частая путаница: «3 воркера читают один канал — значит каждый получит все 6». Нет — это распределение нагрузки (fan-out), не broadcast. Broadcast делается через `close(done)` или отдельные каналы на подписчика.
</details>

---

### Загадка 4: буфер out и «потерянные» результаты при отмене

```go
out := make(chan Result, workers)   // буфер = workers
// воркеры пишут в out; при ctx.Done() выходят, не дослав часть
// closer: go func(){ wg.Wait(); close(out) }()
for r := range out { use(r) }
```

<details>
<summary>Ответ</summary>

При отмене `ctx` воркеры выходят на `case <-ctx.Done(): return`, **не дослав** свои текущие результаты — это нормально (отмена = «результаты больше не нужны»). Но важно: если воркер пишет в `out` **без** `select { case out<-r: case <-ctx.Done(): }`, он зависнет, когда consumer перестал читать → leak.

Вывод: и producer, и worker должны делать отправку через `select` с `ctx.Done()`. Буфер `out` лишь сглаживает coupling, но не спасает от блокировки на полном канале без ушедшего читателя.
</details>

---

## Interview-ready answer

**1. Какие баги в task_before.go?**
Пять независимых: (1) `var out/jobs chan` — nil-каналы: send виснет вечно (leak), `close(nil)` — паника; (2) гонка на `f.cache` без mutex — `go test -race` ловит, нужен `RWMutex`; (3) нет `WaitGroup` — `out` не закрывается, `range out` дедлочит; (4) `ctx` создан, но не передан — timeout не работает; (5) следствие #1 — `close(jobs)` паникует.

**2. Зачем `errCh chan error, 1`?**
Unbuffered заблокирует отправителя, если читателя нет → leak. Буфер 1: первая ошибка пишется без блокировки, остальные дропаются через `select { default }`. Горутина завершается чисто при любом числе ошибок.

**3. Где в пуле обязателен `select` на `ctx.Done()`?**
В трёх местах: producer при `jobs <- id`, worker при чтении `<-jobs` и при отправке `out <- r`. Иначе при ранней отмене/уходе consumer горутины виснут на заблокированных каналах.

**4. Почему closer-горутина обязательно отдельная?**
`wg.Wait()` должен крутиться **параллельно** чтению `out`, иначе воркеры блокируются на отправке в непрочитанный канал, `Done` не зовётся, `Wait` висит — дедлок. Паттерн: `go func(){ wg.Wait(); close(out) }()` + `for range out` в вызывающем.

**5. Worker pool или semaphore?**
Пул (фиксированное N горутин, читающих общий `jobs`) — для долгих задач и предсказуемого потребления памяти. Semaphore (новая горутина на задачу, ограниченная buffered-каналом) — для коротких burst-задач. Пул экономит горутины, semaphore проще по коду.

**6. Несколько воркеров на одном канале — дублируют работу?**
Нет, делят: каждое значение примет ровно один воркер (fan-out/распределение). Broadcast («все получили») — это `close(done)` или канал на подписчика.
