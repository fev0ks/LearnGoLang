# Задача 4: Pipeline

Pipeline — фундаментальный паттерн Go concurrency: цепочка stages, каждый = goroutine + channel. Данные текут через stages, обрабатываются на каждом, в конце — финальный output. Похоже на UNIX pipes (`cat | grep | sort`).

## Формулировка

> "Реализуй pipeline для обработки данных в несколько этапов параллельно. Например: чтение URL'ов → загрузка → парсинг → сохранение."

Вариации:
- "Image processing: resize → watermark → save"
- "ETL: extract → transform → load"
- "Streaming aggregation"

---

## Уточняющие вопросы

1. **Stages — фиксированное число или динамические?**
   "Обычно фиксированное. Динамические — сложнее, нужна dispatcher логика."

2. **Каждый stage — одна goroutine или N?**
   "Зависит от bottleneck. CPU-bound stage может быть N. I/O-bound — одна или N с лимитом параллельности."

3. **Backpressure?**
   "Unbuffered channels дают естественную backpressure (slow stage блокирует upstream). Buffered — сглаживают spike'и."

4. **Что делать с ошибкой на stage?**
   "Fail-fast (cancel всё через ctx) или skip плохой элемент?"

5. **Streaming или batch?**
   "Streaming — данные текут бесконечно. Batch — фиксированный input, закрытие cascading."

---

## Базовое решение: 3-stage pipeline

```go
package pipeline

import "context"

// Stage 1: генерация URL'ов
func generate(ctx context.Context, urls []string) <-chan string {
    out := make(chan string)
    go func() {
        defer close(out)
        for _, u := range urls {
            select {
            case <-ctx.Done():
                return
            case out <- u:
            }
        }
    }()
    return out
}

// Stage 2: загрузка
func download(ctx context.Context, in <-chan string) <-chan []byte {
    out := make(chan []byte)
    go func() {
        defer close(out)
        for url := range in {
            data, err := downloadOne(ctx, url)
            if err != nil {
                continue  // skip errors (или handle иначе)
            }
            select {
            case <-ctx.Done():
                return
            case out <- data:
            }
        }
    }()
    return out
}

// Stage 3: парсинг
func parse(ctx context.Context, in <-chan []byte) <-chan Result {
    out := make(chan Result)
    go func() {
        defer close(out)
        for data := range in {
            r, err := parseOne(data)
            if err != nil {
                continue
            }
            select {
            case <-ctx.Done():
                return
            case out <- r:
            }
        }
    }()
    return out
}

// Использование
func runPipeline(ctx context.Context, urls []string) {
    stage1 := generate(ctx, urls)
    stage2 := download(ctx, stage1)
    stage3 := parse(ctx, stage2)

    for result := range stage3 {
        handleResult(result)
    }
}
```

**Ключевые свойства:**
- Каждый stage — функция `<-chan In → <-chan Out`
- Stages соединяются "цепью"
- `close(out)` cascading'ит вниз — когда upstream закроет, текущий stage завершает range и закрывает свой output
- `<-ctx.Done()` в `select` — graceful shutdown в любой момент

---

## Production-grade: с параллельностью stages и errors

```go
package pipeline

import (
    "context"
    "sync"

    "golang.org/x/sync/errgroup"
)

// PipelineStage представляет один этап с возможной параллельностью.
type PipelineStage[In, Out any] struct {
    Workers int                                          // сколько goroutines в этом stage
    Process func(ctx context.Context, in In) (Out, error)
}

func runStage[In, Out any](
    ctx context.Context,
    g *errgroup.Group,
    in <-chan In,
    stage PipelineStage[In, Out],
) <-chan Out {
    out := make(chan Out)

    workers := stage.Workers
    if workers <= 0 {
        workers = 1
    }

    var wg sync.WaitGroup
    for i := 0; i < workers; i++ {
        wg.Add(1)
        g.Go(func() error {
            defer wg.Done()
            for v := range in {
                result, err := stage.Process(ctx, v)
                if err != nil {
                    return err  // errgroup отменит остальные
                }
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case out <- result:
                }
            }
            return nil
        })
    }

    // Closer
    go func() {
        wg.Wait()
        close(out)
    }()

    return out
}

// Пример: 3-stage pipeline с параллельностью
func runDownloadPipeline(ctx context.Context, urls []string) error {
    g, gctx := errgroup.WithContext(ctx)

    // Stage 1: producer (один)
    stage1 := make(chan string)
    g.Go(func() error {
        defer close(stage1)
        for _, u := range urls {
            select {
            case <-gctx.Done():
                return gctx.Err()
            case stage1 <- u:
            }
        }
        return nil
    })

    // Stage 2: download (10 параллельных I/O)
    stage2 := runStage(gctx, g, stage1, PipelineStage[string, []byte]{
        Workers: 10,
        Process: func(ctx context.Context, url string) ([]byte, error) {
            return downloadOne(ctx, url)
        },
    })

    // Stage 3: parse (4 параллельных CPU)
    stage3 := runStage(gctx, g, stage2, PipelineStage[[]byte, Result]{
        Workers: 4,
        Process: func(ctx context.Context, data []byte) (Result, error) {
            return parseOne(data)
        },
    })

    // Stage 4: save (один — пишем в БД sequential)
    g.Go(func() error {
        for r := range stage3 {
            if err := saveOne(gctx, r); err != nil {
                return err
            }
        }
        return nil
    })

    return g.Wait()
}
```

**Ключевые моменты:**
- Каждый stage может иметь разный `Workers` — баланс по типу работы (I/O vs CPU)
- `errgroup` координирует — первая ошибка отменяет всех
- Closer goroutine на каждый stage закрывает output после всех workers
- Final stage не имеет output — он "консьюмер"

---

## Pipeline с явной структурой Pipe

Для переиспользуемости — wrapper:

```go
package pipeline

import (
    "context"
    "sync"
)

// Pipe — обобщённая stage функция.
type Pipe[In, Out any] func(ctx context.Context, in <-chan In) <-chan Out

// Chain соединяет два pipe'а
func Chain[A, B, C any](p1 Pipe[A, B], p2 Pipe[B, C]) Pipe[A, C] {
    return func(ctx context.Context, in <-chan A) <-chan C {
        return p2(ctx, p1(ctx, in))
    }
}

// Parallel создаёт stage с N параллельных workers.
func Parallel[In, Out any](workers int, fn func(context.Context, In) (Out, error)) Pipe[In, Out] {
    return func(ctx context.Context, in <-chan In) <-chan Out {
        out := make(chan Out)
        var wg sync.WaitGroup

        for i := 0; i < workers; i++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                for v := range in {
                    if r, err := fn(ctx, v); err == nil {
                        select {
                        case <-ctx.Done():
                            return
                        case out <- r:
                        }
                    }
                }
            }()
        }

        go func() {
            wg.Wait()
            close(out)
        }()

        return out
    }
}

// Использование
download := Parallel(10, func(ctx context.Context, url string) ([]byte, error) {
    return fetch(ctx, url)
})
parse := Parallel(4, func(ctx context.Context, data []byte) (Result, error) {
    return parseHTML(data)
})

// Цепочка
process := Chain(download, parse)

// Запуск
in := generate(ctx, urls)
results := process(ctx, in)
for r := range results {
    handle(r)
}
```

Композиция через типизированные generics — clean API.

---

## Backpressure и буферизация

```go
// Buffered channel — стейдж может убежать вперёд consumer'а
ch := make(chan T, 100)

// Unbuffered — естественная backpressure
ch := make(chan T)
```

**Когда buffered:**
- Stage с разной latency (e.g., download = 100-500ms variable)
- Сглаживание burst'ов
- Stage делает batches (накапливает 10 → отправляет 10)

**Когда unbuffered:**
- Хочешь чтобы slow consumer **тормозил** producer'а
- Минимальная задержка (нет очереди)
- Малые объёмы

**Anti-pattern:** буфер "на всякий случай" с размером 1000+. Memory + hides slow consumer issues.

---

## Тесты

```go
func TestPipeline_Basic(t *testing.T) {
    ctx := context.Background()

    in := make(chan int)
    go func() {
        defer close(in)
        for i := 1; i <= 10; i++ {
            in <- i
        }
    }()

    doubler := func(ctx context.Context, x int) (int, error) {
        return x * 2, nil
    }
    squarer := func(ctx context.Context, x int) (int, error) {
        return x * x, nil
    }

    stage1 := Parallel(2, doubler)(ctx, in)
    stage2 := Parallel(2, squarer)(ctx, stage1)

    var results []int
    for r := range stage2 {
        results = append(results, r)
    }

    sort.Ints(results)
    // (1*2)^2, (2*2)^2, ..., (10*2)^2 = 4, 16, 36, ..., 400
    expected := []int{4, 16, 36, 64, 100, 144, 196, 256, 324, 400}
    if !reflect.DeepEqual(results, expected) {
        t.Errorf("got %v, want %v", results, expected)
    }
}

func TestPipeline_ContextCancel(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    in := make(chan int)
    go func() {
        defer close(in)
        for i := 0; i < 1000; i++ {
            in <- i
        }
    }()

    out := Parallel(2, func(ctx context.Context, x int) (int, error) {
        time.Sleep(10 * time.Millisecond)
        return x, nil
    })(ctx, in)

    var processed int
    go func() {
        for range out {
            processed++
            if processed == 5 {
                cancel()  // отменяем после 5
            }
        }
    }()

    // Дать pipeline время остановиться
    time.Sleep(100 * time.Millisecond)

    // Не все 1000 элементов обработаны
    if processed > 50 {
        t.Errorf("processed %d items after cancel, expected ≤ 50", processed)
    }
}

func TestPipeline_NoLeak(t *testing.T) {
    before := runtime.NumGoroutine()

    for i := 0; i < 10; i++ {
        ctx := context.Background()
        in := make(chan int)
        go func() {
            defer close(in)
            for j := 0; j < 100; j++ {
                in <- j
            }
        }()

        out := Parallel(3, func(ctx context.Context, x int) (int, error) {
            return x, nil
        })(ctx, in)

        for range out {
        }
    }

    time.Sleep(50 * time.Millisecond)
    after := runtime.NumGoroutine()
    if after > before+5 {
        t.Errorf("goroutine leak: before %d, after %d", before, after)
    }
}
```

---

## Подводные камни

### 1. Не закрытый upstream → leak горутин

```go
func badPipeline(ctx context.Context) {
    in := make(chan int)
    go func() {
        for {
            in <- generate()  // ← бесконечный producer без close(in)
        }
    }()

    out := stage2(ctx, in)
    for v := range out {
        // ...
    }
    // out не закроется потому что in не закроется
    // → горутина в stage2 зависла навсегда
}
```

**Правило:** producer обязан закрывать свой output (через `defer close(out)`). И сам должен реагировать на cancel сигнал.

### 2. Wrong order закрытия channels

```go
// ❌ Closer goroutine стартует ДО workers — закрывает out пока кто-то ещё пишет
go func() { wg.Wait(); close(out) }()  // ← OK
for i := 0; i < workers; i++ {
    wg.Add(1)
    go worker(out)  // ← каждый worker пишет в out
}
// ✓ Это OK потому что wg.Add(1) перед go worker()
// wg.Wait() не вернётся пока все воркеры не сделают wg.Done() в defer
```

Главное — `wg.Add()` **перед** `go`, не внутри.

### 3. Send в закрытый channel

```go
// Stage пишет в out, но кто-то ещё его закрыл
go func() {
    for v := range in {
        out <- transform(v)  // ← panic если out закрылся параллельно
    }
}()
```

В стандартном паттерне (один closer goroutine после wg.Wait) этого не происходит — workers пишут пока активны, после wg.Done() — closer закрывает. Но если разделить ответственность — careful.

### 4. Buffer ослепляет проблему

```go
ch := make(chan T, 100000)
```

Если consumer медленнее producer'а, буфер заполнится памятью. Под нагрузкой OOM. Лучше — unbuffered + наблюдай где затыкается → решай root cause (больше worker'ов? rate limit на producer'а?).

### 5. Errors теряются в pipeline

```go
go func() {
    for v := range in {
        result, err := transform(v)
        if err != nil {
            continue  // ← ошибка потерялась
        }
        out <- result
    }
}()
```

Решения:
- Использовать errgroup — первая ошибка cancels всё
- Возвращать `Result{Value, Err}` струкуру через output
- Отдельный error channel

### 6. Pipeline без context

```go
func bad(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        for v := range in {
            time.Sleep(time.Hour)  // ← не отменяется
            out <- transform(v)
        }
    }()
    return out
}
```

Всегда `ctx context.Context` первым аргументом.

### 7. Workers стейджа спорят за ресурс

Если stage с 100 workers и каждый делает DB-запрос → 100 параллельных DB connections. Может exhaust pool.

Workers per stage = think about downstream resources.

### 8. Tight coupling между stages

Меняешь тип в stage 1 → нужно менять stage 2, 3, 4. Думай про domain-types (`URL`, `Page`, `Result`) — стабильнее.

---

## Возможные расширения

### 1. Batching stage

Накапливать N элементов, отправлять batch'ем (для DB writes, API calls):

```go
func Batcher[T any](size int, timeout time.Duration) Pipe[T, []T] {
    return func(ctx context.Context, in <-chan T) <-chan []T {
        out := make(chan []T)
        go func() {
            defer close(out)
            batch := make([]T, 0, size)
            ticker := time.NewTicker(timeout)
            defer ticker.Stop()

            flush := func() {
                if len(batch) > 0 {
                    out <- batch
                    batch = make([]T, 0, size)
                }
            }

            for {
                select {
                case v, ok := <-in:
                    if !ok {
                        flush()
                        return
                    }
                    batch = append(batch, v)
                    if len(batch) >= size {
                        flush()
                    }
                case <-ticker.C:
                    flush()  // flush по timeout даже если меньше size
                case <-ctx.Done():
                    return
                }
            }
        }()
        return out
    }
}
```

### 2. Filter stage

```go
func Filter[T any](pred func(T) bool) Pipe[T, T] {
    return func(ctx context.Context, in <-chan T) <-chan T {
        out := make(chan T)
        go func() {
            defer close(out)
            for v := range in {
                if pred(v) {
                    select {
                    case <-ctx.Done():
                        return
                    case out <- v:
                    }
                }
            }
        }()
        return out
    }
}
```

### 3. Throttling stage

Limit rate через rate.Limiter (см. [02-rate-limiter.md](./02-rate-limiter.md)).

### 4. Tee — отвести копию

Один input → два output (e.g., main path + metrics path).

### 5. Merge stage

Несколько inputs → один output (fan-in pattern, см. [03-fan-in-fan-out.md](./03-fan-in-fan-out.md)).

### 6. Persistent stage (с retry)

При ошибке — retry с backoff (вместо continue или fail).

### 7. Pipeline metrics

Per-stage histogram latency, throughput, error rate. Prometheus integration.

### 8. Pipeline visualization

Логирование с stage ID → видишь где затыкается. Или Jaeger trace.

---

## Что важно показать на собеседовании

1. **Понимание cascading close** — close upstream → range exits → close downstream
2. **Context propagation** — в каждый stage
3. **`select` с `<-ctx.Done()`** в каждом send/receive
4. **Wait + close pattern** для closing output после всех workers
5. **Trade-off buffered vs unbuffered** — почему такой выбор
6. **Backpressure** — slow consumer тормозит producer
7. **errgroup** для координации ошибок
8. **Ссылка на Sameer Ajmani's blog** — официальный гайд

## Связки

- [Go Concurrency Patterns: Pipelines](https://go.dev/blog/pipelines) — Sameer Ajmani, must-read
- [Fan-in/fan-out](./03-fan-in-fan-out.md) — базовые паттерны pipeline'а
- [Worker pool](./01-worker-pool.md) — частный случай pipeline
- [Background workers](../../../04-architecture-and-patterns/patterns/04-background-workers.md) — production pipelines
- [Streaming в Kafka](../../../07-message-brokers-and-streaming/01-kafka.md) — pipeline через Kafka между сервисами
