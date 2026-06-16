# Задача 3: Fan-In / Fan-Out

Базовые паттерны для распараллеливания: **fan-out** — раздать работу множеству worker'ов, **fan-in** — собрать результаты в один поток. Часто комбинируются.

## Формулировка

> "Реализуй функцию, которая получает поток входных данных, обрабатывает их параллельно N горутинами и возвращает поток результатов в одном канале."

Вариации:
- "Распиши классические паттерны Go concurrency"
- "Сделай функцию `Map(in <-chan T, fn func(T) R, workers int) <-chan R`"
- "Объедини N каналов в один"

---

## Уточняющие вопросы

1. **Порядок результатов важен или нет?**
   "Если важен — нужны индексы и пересборка в конце. Если нет — fan-in проще."

2. **Что делать с ошибками?**
   "Fail-fast (через errgroup) или собирать все?"

3. **Бесконечный поток или фиксированный набор?**
   "Streaming требует другого управления lifecycle."

4. **Buffered output или нет?**
   "Depends — если consumer медленнее, буфер сглаживает spike'и."

5. **Что делать если producer закончил?**
   "Закрыть output после того как все worker'ы завершили — корректная отмена."

---

## Fan-Out

Fan-out = разделить работу. Один input channel → N goroutines читают параллельно.

```go
// Один input, несколько workers
func fanOut[T any, R any](in <-chan T, fn func(T) R, workers int) []<-chan R {
    outs := make([]<-chan R, workers)
    for i := 0; i < workers; i++ {
        out := make(chan R)
        outs[i] = out
        go func() {
            defer close(out)
            for v := range in {
                out <- fn(v)
            }
        }()
    }
    return outs
}
```

Каждый worker берёт из общего `in` (Go runtime сам распределяет — кто первый успел, тот и взял). Возвращаем массив output channels.

---

## Fan-In

Fan-in = объединить N каналов в один.

```go
// Несколько inputs, один output
func fanIn[T any](inputs ...<-chan T) <-chan T {
    out := make(chan T)
    var wg sync.WaitGroup

    for _, in := range inputs {
        wg.Add(1)
        go func(ch <-chan T) {
            defer wg.Done()
            for v := range ch {
                out <- v
            }
        }(in)
    }

    // Goroutine которая закроет out когда все inputs закроются
    go func() {
        wg.Wait()
        close(out)
    }()

    return out
}
```

**Ключевые моменты:**
- Каждый input в своей goroutine → читаем параллельно, не блокируясь
- `wg.Wait() + close(out)` в отдельной goroutine — иначе нельзя `range` по out
- `close(out)` сигнализирует consumer'у что данных больше не будет

---

## Базовое решение: Fan-Out + Fan-In

Объединяем — это и есть классический `Map(in, fn, workers) out` паттерн:

```go
package fanout

import "sync"

// Map обрабатывает все элементы in через fn с N горутинами.
// Возвращает output channel который закроется когда in закроется и все workers закончат.
func Map[T any, R any](in <-chan T, fn func(T) R, workers int) <-chan R {
    out := make(chan R)
    var wg sync.WaitGroup

    // Запускаем N worker'ов, каждый читает из in и пишет в out
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for v := range in {
                out <- fn(v)
            }
        }()
    }

    // Closer goroutine: закрывает out после завершения всех workers
    go func() {
        wg.Wait()
        close(out)
    }()

    return out
}
```

**Использование:**

```go
in := make(chan int)
go func() {
    defer close(in)
    for i := 0; i < 100; i++ {
        in <- i
    }
}()

out := fanout.Map(in, func(x int) int {
    return x * x
}, 10)

for result := range out {
    fmt.Println(result)
}
```

**Что важно:**
- `defer wg.Done()` в worker'е
- Caller сам закрывает `in` (мы не можем — мы только читаем)
- Мы сами закрываем `out` (мы — owner output'а)
- Только closer-goroutine может вызвать `close(out)` (после `wg.Wait()`)

---

## Production-grade: с context, errors, order preservation

```go
package fanout

import (
    "context"
    "sync"
    "sync/atomic"

    "golang.org/x/sync/errgroup"
)

// MapResult содержит результат и индекс для восстановления порядка.
type MapResult[R any] struct {
    Index  int
    Output R
}

// MapOrdered обрабатывает inputs параллельно, возвращает results в правильном порядке.
// Если любая задача fail'ит — context cancelled, остальные останавливаются.
func MapOrdered[T any, R any](
    ctx context.Context,
    inputs []T,
    fn func(context.Context, T) (R, error),
    workers int,
) ([]R, error) {
    if workers <= 0 {
        workers = 1
    }

    results := make([]R, len(inputs))
    indices := make(chan int)

    g, gctx := errgroup.WithContext(ctx)

    // Producer
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

    // Workers
    var inFlight atomic.Int32
    for w := 0; w < workers; w++ {
        g.Go(func() error {
            for idx := range indices {
                if gctx.Err() != nil {
                    return gctx.Err()
                }
                inFlight.Add(1)
                out, err := fn(gctx, inputs[idx])
                inFlight.Add(-1)
                if err != nil {
                    return err
                }
                results[idx] = out
            }
            return nil
        })
    }

    if err := g.Wait(); err != nil {
        return results, err
    }
    return results, nil
}

// MapUnordered — то же самое, но результаты приходят по мере готовности через channel.
func MapUnordered[T any, R any](
    ctx context.Context,
    in <-chan T,
    fn func(context.Context, T) (R, error),
    workers int,
) (<-chan R, <-chan error) {
    out := make(chan R)
    errCh := make(chan error, 1)

    g, gctx := errgroup.WithContext(ctx)

    for w := 0; w < workers; w++ {
        g.Go(func() error {
            for {
                select {
                case <-gctx.Done():
                    return gctx.Err()
                case v, ok := <-in:
                    if !ok {
                        return nil
                    }
                    r, err := fn(gctx, v)
                    if err != nil {
                        return err
                    }
                    select {
                    case <-gctx.Done():
                        return gctx.Err()
                    case out <- r:
                    }
                }
            }
        })
    }

    go func() {
        if err := g.Wait(); err != nil {
            select {
            case errCh <- err:
            default:
            }
        }
        close(out)
        close(errCh)
    }()

    return out, errCh
}
```

**Использование:**

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

inputs := []string{"http://a.com", "http://b.com", "http://c.com"}

// Ordered: результаты в том же порядке
results, err := fanout.MapOrdered(ctx, inputs, func(ctx context.Context, url string) (Page, error) {
    return downloadPage(ctx, url)
}, 5)
if err != nil {
    log.Fatal(err)
}

// или unordered streaming
in := make(chan string)
go func() {
    defer close(in)
    for _, u := range urls {
        in <- u
    }
}()

results, errs := fanout.MapUnordered(ctx, in, downloadPage, 10)
for r := range results {
    handle(r)
}
if err := <-errs; err != nil {
    log.Println(err)
}
```

---

## Pattern: Multiplexing с select

Иногда нужно fan-in между **разными типами** каналов или с приоритетом:

```go
// Merge с приоритетом: high-priority channel читается первым
func mergeWithPriority(ctx context.Context, high, low <-chan Task) <-chan Task {
    out := make(chan Task)
    go func() {
        defer close(out)
        for {
            // Сначала проверить high — если есть, взять
            select {
            case t, ok := <-high:
                if !ok {
                    return
                }
                out <- t
                continue
            default:
            }

            // Если high пустой — берём из любого
            select {
            case <-ctx.Done():
                return
            case t, ok := <-high:
                if !ok {
                    return
                }
                out <- t
            case t, ok := <-low:
                if !ok {
                    return
                }
                out <- t
            }
        }
    }()
    return out
}
```

`default` в первом select — non-blocking check. Если high пустой — переходим ко второму select с обоими каналами.

---

## Тесты

```go
func TestMap_PreservesValues(t *testing.T) {
    in := make(chan int)
    go func() {
        defer close(in)
        for i := 1; i <= 10; i++ {
            in <- i
        }
    }()

    out := Map(in, func(x int) int { return x * 2 }, 3)

    var results []int
    for r := range out {
        results = append(results, r)
    }

    sort.Ints(results)
    expected := []int{2, 4, 6, 8, 10, 12, 14, 16, 18, 20}
    if !reflect.DeepEqual(results, expected) {
        t.Errorf("got %v, want %v", results, expected)
    }
}

func TestMapOrdered_KeepsOrder(t *testing.T) {
    inputs := make([]int, 100)
    for i := range inputs {
        inputs[i] = i
    }

    results, err := MapOrdered(context.Background(), inputs,
        func(ctx context.Context, x int) (int, error) {
            // Случайная задержка чтобы убедиться что order preservation работает
            time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
            return x * 2, nil
        }, 10)

    if err != nil {
        t.Fatal(err)
    }

    for i, r := range results {
        if r != i*2 {
            t.Errorf("results[%d] = %d, want %d", i, r, i*2)
        }
    }
}

func TestMap_Parallelism(t *testing.T) {
    var concurrent atomic.Int32
    var maxConcurrent atomic.Int32

    in := make(chan int, 100)
    for i := 0; i < 100; i++ {
        in <- i
    }
    close(in)

    out := Map(in, func(x int) int {
        n := concurrent.Add(1)
        defer concurrent.Add(-1)

        // Update max
        for {
            m := maxConcurrent.Load()
            if n <= m || maxConcurrent.CompareAndSwap(m, n) {
                break
            }
        }

        time.Sleep(10 * time.Millisecond)
        return x
    }, 5)

    for range out {
    }

    if maxConcurrent.Load() > 5 {
        t.Errorf("max concurrent = %d, expected ≤ 5", maxConcurrent.Load())
    }
}

func TestMap_NoGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()

    in := make(chan int)
    close(in)  // нет данных

    out := Map(in, func(x int) int { return x }, 5)
    for range out {
    }

    // Дать time runtime'у убрать горутины
    time.Sleep(50 * time.Millisecond)

    after := runtime.NumGoroutine()
    if after > before {
        t.Errorf("goroutine leak: before %d, after %d", before, after)
    }
}
```

---

## Подводные камни

### 1. Дважды закрытие output channel

```go
// ❌ Два worker'а одновременно закрывают out
for i := 0; i < workers; i++ {
    go func() {
        for v := range in {
            out <- fn(v)
        }
        close(out)  // ← все workers закроют → panic второй раз
    }()
}
```

**Только один** закрыватель. Стандартно — отдельная goroutine после `wg.Wait()`.

### 2. Goroutine leak когда consumer прекращает читать

```go
// ❌ Если caller перестал читать из out, workers заблокированы на out <- fn(v)
out := Map(in, fn, 10)
for r := range out {
    if shouldStop {
        break  // ← workers зависли пытаясь писать в out
    }
}
```

Решения:
- Передай context, проверяй `gctx.Done()` перед записью
- Используй buffered out (но это не решает фундаментально)
- Дренируй out до конца после break

### 3. Slow consumer = slow producers

Если worker'ы быстрые, а consumer медленный — workers блокируются на `out <-`. Возможно ты хочешь:
- Buffered out — сглаживает spike'и
- Drop on full — для metrics и telemetry где старое не важно
- Backpressure до producer'а — propagate slowness назад

### 4. Closure захват loop variable (до Go 1.22)

```go
for _, in := range inputs {
    go func() {
        for v := range in {  // ← in — общая переменная во всех итерациях
            ...
        }
    }()
}
```

В Go 1.22+ исправлено. В старых — `in := in` shadowing.

### 5. fanIn не имеет cancel mechanism

```go
// ❌ Если хотим прекратить ранее
func fanIn(ins ...<-chan T) <-chan T {
    out := make(chan T)
    for _, in := range ins {
        go func(c <-chan T) {
            for v := range c {
                out <- v  // ← если кто-то заблокирован, нет cancel
            }
        }(in)
    }
}
```

Production версия должна принимать context.

### 6. Order не сохраняется в стандартном fan-out

Worker A и B берут из общего channel — кто первый закончит, тот и пишет первым. **Порядок не сохраняется.**

Если нужен порядок — используй `MapOrdered` с индексами и итоговой пересборкой.

### 7. Buffered channel с большим буфером

```go
in := make(chan int, 1000000)
```

Если producer быстрее consumer'ов — буфер заполнится памятью. Лучше unbuffered и натуральная backpressure.

---

## Возможные расширения

### 1. Bounded parallelism через semaphore

Альтернативный подход — semaphore вместо worker pool:

```go
sem := make(chan struct{}, concurrency)
for _, task := range tasks {
    sem <- struct{}{}  // acquire
    go func(t Task) {
        defer func() { <-sem }()  // release
        t()
    }(task)
}
// Дождаться завершения всех
for i := 0; i < concurrency; i++ {
    sem <- struct{}{}
}
```

Не "pool" worker'ов, а "max N в полёте одновременно". Каждый task — своя горутина.

### 2. Streaming pipeline с back pressure

См. [04-pipeline.md](./04-pipeline.md) — следующая задача.

### 3. Round-robin distribution

Вместо случайного "кто первый взял", explicit распределение по worker'ам.

### 4. Sharding по ключу

Один key → всегда один worker. Полезно для stateful processing (per-user serialization).

### 5. Lazy evaluation через generator

```go
func generate(n int) <-chan int {
    ch := make(chan int)
    go func() {
        defer close(ch)
        for i := 0; i < n; i++ {
            ch <- i
        }
    }()
    return ch
}
```

### 6. Tee — один input, два output (мультиплексирование вверх)

```go
func tee[T any](in <-chan T) (<-chan T, <-chan T) {
    out1, out2 := make(chan T), make(chan T)
    go func() {
        defer close(out1)
        defer close(out2)
        for v := range in {
            out1 <- v
            out2 <- v  // оба должны успеть прочитать
        }
    }()
    return out1, out2
}
```

Полезно для "обработать + logging + persist" одновременно.

---

## Что важно показать на собеседовании

1. **Правильное закрытие channel** — owner закрывает, только один раз
2. **`sync.WaitGroup + close()` паттерн** для fan-in
3. **Понимание что общий channel = "first wins"** для fan-out
4. **Context cancellation** обязательно
5. **`errgroup`** как std-tool для координации
6. **Order preservation через индексы** если попросят
7. **Goroutine leak prevention** — все горутины должны иметь exit path

## Связки

- [Worker pool](./01-worker-pool.md) — частный случай fan-out + fan-in
- [Pipeline](./04-pipeline.md) — fan-out + fan-in в виде stage'ей
- [Concurrency patterns Go blog](https://go.dev/blog/pipelines) — официальный гайд от Sameer Ajmani
- [Worker pool patterns](07-worker-pool-debug.md)
