# Worker Pool

Worker pool — один из самых частых паттернов в Go-коде: bounded concurrency для CPU-bound или I/O-bound задач. Здесь — разбор реального баг-репорта с собеседования и правильная реализация.

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

## Interview-ready answer

**Q: Какие баги в этом коде? (task_before.go)**

Пять независимых проблем:
1. `var out chan Result` и `var jobs chan int` — nil channels. Отправка в nil канал блокируется вечно (goroutine leak), close nil — паника.
2. Race condition на `f.cache` — несколько горутин читают и пишут map без mutex. `go test -race` выловит. Нужен `sync.RWMutex`.
3. Нет `sync.WaitGroup` — `out` никогда не закрывается, `range out` в main зависнет (deadlock).
4. Context создан, но не передан в `FetchAll` — timeout не работает.
5. Следствие #1: `close(jobs)` в producer паникует (close nil channel).

**Q: Зачем `errCh chan error, 1` вместо `chan error`?**

Unbuffered канал заблокирует горутину-отправитель, если читателя нет в этот момент → goroutine leak. Буфер 1 позволяет первой ошибке записаться без блокировки, остальные дропаются через `select { default }`. В итоге горутина завершается чисто в любом случае.
