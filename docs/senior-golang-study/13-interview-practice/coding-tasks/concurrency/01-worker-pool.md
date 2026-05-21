# Задача 1: Worker Pool

Самая частая concurrency-задача на Go-собеседовании. Спрашивают почти везде, потому что показывает понимание базовых примитивов: goroutines, channels, sync, context.

## Формулировка

> "Реализуй worker pool, который обрабатывает задачи параллельно с фиксированным количеством worker'ов. Поддержи graceful shutdown."

Вариации формулировки:
- "Скачать N URL'ов с лимитом параллельности K"
- "Обработать batch задач с N workers"
- "Сделать функцию `ProcessAll(items, fn, concurrency)`"

---

## Уточняющие вопросы

Перед тем как писать код — обязательно спроси:

1. **Ограничение параллельности — фиксированное число или динамическое?**
   "Фиксированное N — самый простой случай. Динамическое (auto-scaling) — сложнее, нужен другой паттерн."

2. **Сколько задач — известно заранее или поток?**
   "Известно — close channel после всех. Поток — нужен отдельный сигнал завершения."

3. **Что делать с ошибкой задачи — стопать всё или продолжать?**
   "Fail-fast (errgroup) или собирать все ошибки (sync.WaitGroup + error channel)."

4. **Нужна возможность отмены извне?**
   "Да — context.Context. Это обязательно для production."

5. **Что делать если worker упал (panic)?**
   "Recover внутри worker'а — pool должен пережить отдельный сбой."

6. **Результаты собирать или fire-and-forget?**
   "Если результаты нужны — channel для них или отдельный output. Если fire-and-forget — проще."

---

## Базовое решение

Минимальное MVP для случая "обработать массив задач параллельно":

```go
package workerpool

import (
    "context"
    "sync"
)

type Task func(ctx context.Context) error

// Run обрабатывает все задачи с лимитом параллельности.
// Возвращает первую ошибку, если она была.
func Run(ctx context.Context, tasks []Task, concurrency int) error {
    if concurrency <= 0 {
        concurrency = 1
    }

    taskCh := make(chan Task)
    errCh := make(chan error, 1)  // буфер 1 — для первой ошибки

    var wg sync.WaitGroup

    // Запускаем worker'ов
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for task := range taskCh {
                if err := task(ctx); err != nil {
                    select {
                    case errCh <- err:  // первая ошибка
                    default:           // буфер заполнен — игнорируем
                    }
                    return  // worker останавливается на ошибке
                }
            }
        }()
    }

    // Отправляем задачи
    for _, task := range tasks {
        select {
        case taskCh <- task:
        case <-ctx.Done():
            close(taskCh)
            wg.Wait()
            return ctx.Err()
        }
    }
    close(taskCh)
    wg.Wait()

    select {
    case err := <-errCh:
        return err
    default:
        return nil
    }
}
```

**Что показано:**
- Фиксированное число worker'ов через `concurrency`
- `taskCh` для передачи задач — каждый worker сам берёт следующую
- `close(taskCh)` сигнализирует worker'ам что задач больше нет
- `WaitGroup` для ожидания завершения всех worker'ов
- Context cancellation через select при отправке
- Первая ошибка возвращается через `errCh`

**Использование:**

```go
tasks := []Task{
    func(ctx context.Context) error { return downloadFile(ctx, url1) },
    func(ctx context.Context) error { return downloadFile(ctx, url2) },
    // ...
}

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := workerpool.Run(ctx, tasks, 10); err != nil {
    log.Fatal(err)
}
```

---

## Production-grade решение

Расширенная версия с:
- `errgroup` для управления ошибками
- Результатами через output channel
- Backpressure (limited buffer)
- Panic recovery
- Метриками (TaskDuration)

```go
package workerpool

import (
    "context"
    "fmt"
    "sync"
    "time"

    "golang.org/x/sync/errgroup"
)

// Pool — переиспользуемый worker pool с persistent workers.
type Pool[T any, R any] struct {
    workerFunc  func(ctx context.Context, in T) (R, error)
    concurrency int
}

func New[T any, R any](workerFunc func(context.Context, T) (R, error), concurrency int) *Pool[T, R] {
    if concurrency <= 0 {
        concurrency = 1
    }
    return &Pool[T, R]{
        workerFunc:  workerFunc,
        concurrency: concurrency,
    }
}

// Result содержит результат обработки одного входного элемента.
type Result[T any, R any] struct {
    Input    T
    Output   R
    Err      error
    Duration time.Duration
}

// Process обрабатывает все inputs с лимитом параллельности.
// Возвращает results в том же порядке что и inputs.
// При ошибке возвращает первый error (через errgroup).
func (p *Pool[T, R]) Process(ctx context.Context, inputs []T) ([]Result[T, R], error) {
    results := make([]Result[T, R], len(inputs))

    // Канал для индексов задач — workers берут отсюда
    indices := make(chan int)

    g, gctx := errgroup.WithContext(ctx)

    // Запускаем worker'ов
    for w := 0; w < p.concurrency; w++ {
        g.Go(func() error {
            for {
                select {
                case <-gctx.Done():
                    return gctx.Err()
                case idx, ok := <-indices:
                    if !ok {
                        return nil  // канал закрыт — задач больше нет
                    }
                    if err := p.processOne(gctx, inputs[idx], &results[idx]); err != nil {
                        return err  // errgroup отменит остальные
                    }
                }
            }
        })
    }

    // Producer: посылает индексы
    g.Go(func() error {
        defer close(indices)
        for i := range inputs {
            select {
            case <-gctx.Done():
                return gctx.Err()
            case indices <- i:
            }
        }
        return nil
    })

    if err := g.Wait(); err != nil {
        return results, err
    }
    return results, nil
}

func (p *Pool[T, R]) processOne(ctx context.Context, input T, result *Result[T, R]) error {
    start := time.Now()
    result.Input = input

    // Panic recovery — один сбойный task не должен валить весь pool
    defer func() {
        if r := recover(); r != nil {
            result.Err = fmt.Errorf("panic: %v", r)
            result.Duration = time.Since(start)
        }
    }()

    out, err := p.workerFunc(ctx, input)
    result.Output = out
    result.Err = err
    result.Duration = time.Since(start)

    if err != nil {
        return err  // если хочешь fail-fast
    }
    return nil
}
```

**Использование:**

```go
type URL string
type DownloadResult struct {
    Size int64
    Hash string
}

pool := workerpool.New(
    func(ctx context.Context, u URL) (DownloadResult, error) {
        return downloadFile(ctx, string(u))
    },
    10,  // concurrency
)

urls := []URL{"https://...", "https://...", ...}
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

results, err := pool.Process(ctx, urls)
if err != nil {
    log.Printf("pool failed: %v", err)
    // results содержит частичные результаты
}

for _, r := range results {
    if r.Err != nil {
        log.Printf("failed %s: %v", r.Input, r.Err)
        continue
    }
    log.Printf("downloaded %s: %d bytes (%v)", r.Input, r.Output.Size, r.Duration)
}
```

**Что добавлено:**
- **Generics** — type-safe input/output без `any`
- **errgroup** — встроенная отмена при первой ошибке, координация
- **Panic recovery** — изолирует сбои отдельных задач
- **Метрики** — Duration для каждой задачи
- **Order preservation** — результаты в том же порядке что и inputs (через индекс)
- **Backpressure** — unbuffered `indices` channel, producer ждёт пока worker'ы заберут

---

## Тесты

```go
package workerpool

import (
    "context"
    "errors"
    "sync/atomic"
    "testing"
    "time"
)

func TestPool_Process(t *testing.T) {
    var processed atomic.Int32

    pool := New(func(ctx context.Context, x int) (int, error) {
        processed.Add(1)
        return x * 2, nil
    }, 5)

    inputs := make([]int, 100)
    for i := range inputs {
        inputs[i] = i
    }

    results, err := pool.Process(context.Background(), inputs)
    if err != nil {
        t.Fatal(err)
    }
    if processed.Load() != 100 {
        t.Errorf("processed %d, want 100", processed.Load())
    }
    for i, r := range results {
        if r.Output != i*2 {
            t.Errorf("results[%d] = %d, want %d", i, r.Output, i*2)
        }
    }
}

func TestPool_ConcurrencyLimit(t *testing.T) {
    var active atomic.Int32
    var maxActive atomic.Int32

    pool := New(func(ctx context.Context, x int) (int, error) {
        current := active.Add(1)
        defer active.Add(-1)

        // Обновить max
        for {
            m := maxActive.Load()
            if current <= m || maxActive.CompareAndSwap(m, current) {
                break
            }
        }

        time.Sleep(10 * time.Millisecond)
        return x, nil
    }, 5)

    inputs := make([]int, 50)
    _, err := pool.Process(context.Background(), inputs)
    if err != nil {
        t.Fatal(err)
    }

    if maxActive.Load() > 5 {
        t.Errorf("max active = %d, expected ≤ 5", maxActive.Load())
    }
}

func TestPool_FailFast(t *testing.T) {
    expectedErr := errors.New("fail")

    pool := New(func(ctx context.Context, x int) (int, error) {
        if x == 10 {
            return 0, expectedErr
        }
        // Имитируем долгую работу — должны быть отменены через context
        select {
        case <-time.After(time.Second):
            return x, nil
        case <-ctx.Done():
            return 0, ctx.Err()
        }
    }, 5)

    inputs := make([]int, 100)
    for i := range inputs {
        inputs[i] = i
    }

    start := time.Now()
    _, err := pool.Process(context.Background(), inputs)
    elapsed := time.Since(start)

    if !errors.Is(err, expectedErr) {
        t.Errorf("got %v, want %v", err, expectedErr)
    }
    if elapsed > 500*time.Millisecond {
        t.Errorf("fail-fast took %v, expected < 500ms", elapsed)
    }
}

func TestPool_ContextCancel(t *testing.T) {
    pool := New(func(ctx context.Context, x int) (int, error) {
        select {
        case <-time.After(time.Second):
            return x, nil
        case <-ctx.Done():
            return 0, ctx.Err()
        }
    }, 5)

    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    inputs := make([]int, 100)
    _, err := pool.Process(ctx, inputs)

    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("got %v, want DeadlineExceeded", err)
    }
}
```

Если есть `go test -race ./...` — проверь что race detector не находит проблем.

---

## Подводные камни

### 1. Goroutine leak при early return

```go
// ❌ Если producer вернёт ошибку до отправки всех задач,
// worker'ы могут зависнуть ожидая задач из channel'а

func bad(inputs []int, concurrency int) {
    ch := make(chan int)

    for i := 0; i < concurrency; i++ {
        go func() {
            for v := range ch {  // ← блок если не close(ch)
                process(v)
            }
        }()
    }

    for _, v := range inputs {
        if shouldStop {
            return  // ← забыли close(ch)!  Worker'ы зависли.
        }
        ch <- v
    }
    close(ch)
}
```

**Решение:** всегда `defer close(ch)` или используй errgroup, который сам управляет cancellation.

### 2. Захват loop variable (до Go 1.22)

```go
// ❌ В Go 1.21 и раньше
for i, v := range items {
    go func() {
        process(i, v)  // i и v одинаковые для всех goroutines!
    }()
}

// Решение: явный shadowing
for i, v := range items {
    i, v := i, v  // shadow
    go func() {
        process(i, v)
    }()
}

// Или через параметры
for i, v := range items {
    go func(i int, v Item) {
        process(i, v)
    }(i, v)
}
```

В **Go 1.22+** это исправлено: каждая итерация имеет свои переменные. Но если работаешь со старым кодом — помни.

### 3. Buffered channel "на всякий случай"

```go
// ❌ Бессмысленный буфер
taskCh := make(chan Task, 1000)
```

Буфер должен быть **обоснован**. Без него — synchronous handoff, естественная backpressure. С буфером — producer может уйти далеко вперёд consumer'ов, тратя память.

Используй буфер если:
- Производитель имеет короткие "всплески" (smooth out)
- Producer и consumer имеют разную latency
- Известно конкретное "комфортное" число in-flight задач

### 4. Не возвращать ошибку

```go
// ❌ Errors теряются
for _, task := range tasks {
    go func(t Task) {
        if err := t(); err != nil {
            log.Println(err)  // ← только лог, caller не знает
        }
    }(task)
}
```

Используй `errgroup` или явный error channel.

### 5. Send в закрытый channel

```go
// ❌ Panic
close(ch)
ch <- value  // panic: send on closed channel
```

**Правило:** только **owner** канала (producer) закрывает его. Consumer'ы — нет.

### 6. Slow worker блокирует pool

```go
// ❌ Один worker тормозит → все остальные сидят без работы потому что
// pool ждёт пока медленный завершится перед close(ch)
```

В простом дизайне это нормально. Если важно — добавь per-task timeout через context.

### 7. Panic в worker валит весь pool

```go
// ❌ Без recover panic в task валит горутину → goroutine leak → возможно весь pool
go func() {
    for t := range ch {
        t()  // panic тут = goroutine dies
    }
}()

// ✓ С recover
go func() {
    for t := range ch {
        func() {
            defer func() {
                if r := recover(); r != nil {
                    log.Printf("worker panic: %v", r)
                }
            }()
            t()
        }()
    }
}()
```

---

## Возможные расширения

Интервьюер может попросить:

### 1. Streaming input (не известное число задач)

```go
type Submitter[T, R any] struct {
    in  chan T
    out chan Result[T, R]
}

func (s *Submitter[T, R]) Submit(task T) {
    s.in <- task
}

func (s *Submitter[T, R]) Results() <-chan Result[T, R] {
    return s.out
}

func (s *Submitter[T, R]) Close() {
    close(s.in)
}
```

Pool работает пока не вызовут `Close()`. Подходит для long-running сервиса.

### 2. Priority queue вместо FIFO

Заменить channel на heap. Worker'ы берут самую приоритетную задачу.

### 3. Retry на ошибке

Обернуть `workerFunc` в retry-логику (exponential backoff).

### 4. Dynamic concurrency

Scale up/down worker'ов в зависимости от нагрузки. Сложнее — нужен механизм запуска/остановки worker'ов и balance.

### 5. Rate limit на pool

Не более N задач в секунду через rate.Limiter (см. [02-rate-limiter.md](./02-rate-limiter.md)).

### 6. Метрики (Prometheus)

Counter в обработанных задач, histogram latency, gauge in-flight.

### 7. Идемпотентность

Если task может выполниться дважды (retry) — нужен idempotency key.

### 8. Graceful shutdown с дренажем

При получении SIGTERM — закрыть прием новых задач, дождаться завершения текущих, отчитаться о незавершённых.

---

## Что важно показать на собеседовании

1. **Уточняющие вопросы** перед кодом — что важно, что нет
2. **Использование context** — обязательно
3. **errgroup или явный error handling** — не игнорировать errors
4. **Понимание `close(channel)`** — кто закрывает, когда, почему
5. **`go test -race`** — упомянуть и желательно показать тест
6. **Знание `golang.org/x/sync/errgroup`** — это standard для большинства задач
7. **Trade-offs** — почему такой buffer size, почему такой concurrency

## Связки

- [Background workers](../../../04-architecture-and-patterns/patterns/04-background-workers.md) — паттерны production worker'ов
- [Worker pool patterns](../../../09-concurrency-and-performance/03-worker-pool.md) — больше вариантов
- [Context patterns](../../../09-concurrency-and-performance/04-context-patterns.md) — детально про context
- [Graceful shutdown](../../../04-architecture-and-patterns/patterns/08-graceful-shutdown.md) — как останавливать pool в k8s
