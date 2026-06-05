# Goroutines and Channels

Конкурентность в Go строится на двух примитивах: горутины (дешёвые потоки, управляемые рантаймом) и каналы (безопасная передача данных между горутинами). Плюс `select` для мультиплексирования.

---

## Goroutine lifecycle

### Запуск и стек

```go
go func() {
    // тело горутины
}()
```

- Стек горутины начинается с **~2 KB** (до Go 1.4 — 8 KB, с 1.4 — 2 KB)
- Стек **растёт динамически** (до 1 GB по умолчанию) — при переполнении аллоцируется новый сегмент в 2× больший
- OS thread — 1–8 MB фиксированного стека; горутина — 2 KB, поэтому легко создать 100k горутин

### Завершение горутины

Горутина завершается когда:
1. Тело функции возвращает (`return`)
2. Рантайм завершает программу (`os.Exit`, `main` вернулась)
3. Паника без recover внутри горутины **крашит всю программу**

```go
// Всегда обрабатывай панику в long-running горутинах
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("goroutine panic: %v", r)
        }
    }()
    doWork()
}()
```

Горутина **не имеет handle** — нельзя её "убить" снаружи, только попросить завершиться через channel или context.

---

## Unbuffered vs buffered channel

### Unbuffered (синхронный)

```go
ch := make(chan int) // capacity = 0
```

- Отправитель блокируется **до тех пор, пока получатель не готов принять**
- Получатель блокируется **до тех пор, пока отправитель не отправит**
- Это **гарантия синхронизации**: когда `ch <- v` вернулся, получатель уже получил `v`

```go
done := make(chan struct{})
go func() {
    doWork()
    done <- struct{}{} // сигнализируем о завершении
}()
<-done // ждём
```

### Buffered (асинхронный)

```go
ch := make(chan int, 10) // capacity = 10
```

- Отправитель блокируется только когда буфер **полный**
- Получатель блокируется только когда буфер **пустой**
- Полезен для развязки producer и consumer по скорости

```go
// Semaphore через buffered channel
sem := make(chan struct{}, 5) // не более 5 параллельных операций

for _, item := range items {
    sem <- struct{}{}    // acquire
    go func(item Item) {
        defer func() { <-sem }() // release
        process(item)
    }(item)
}
// Ждём завершения всех (заполняем до capacity)
for i := 0; i < cap(sem); i++ {
    sem <- struct{}{}
}
```

### Когда что

| | Unbuffered | Buffered |
|---|---|---|
| Синхронизация | ✅ гарантирована | ❌ нет |
| Throughput | ниже (каждая передача — handshake) | выше (batch) |
| Обнаружение дедлоков | проще (блокировка сразу видна) | сложнее (маскируется буфером) |
| Use case | сигналы, done-channels, rendezvous | worker queues, rate limiting |

---

## Pipeline паттерн

Цепочка стадий: каждая стадия читает из входного канала, обрабатывает и пишет в выходной.

```go
// gen: []int → chan int
func gen(nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out) // всегда закрывай каналы на стороне отправителя
        for _, n := range nums {
            out <- n
        }
    }()
    return out
}

// square: chan int → chan int
func square(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in { // range по каналу — до close
            out <- n * n
        }
    }()
    return out
}

// Использование
c := gen(2, 3, 4)
out := square(c)
for n := range out {
    fmt.Println(n) // 4, 9, 16
}
```

**Правило pipeline**: каждая стадия закрывает свой выходной канал; никогда не закрывает входной.

---

## Fan-out / Fan-in

### Fan-out: один producer → несколько workers

```go
func fanOut(in <-chan int, workers int) []<-chan int {
    outs := make([]<-chan int, workers)
    for i := range workers {
        outs[i] = square(in) // несколько goroutines читают один канал
    }
    return outs
}
```

Все workers читают из **одного** входного канала — Go гарантирует, что каждое значение получит только одна горутина.

### Fan-in: несколько producers → один consumer

```go
func merge(cs ...<-chan int) <-chan int {
    var wg sync.WaitGroup
    merged := make(chan int)

    output := func(c <-chan int) {
        defer wg.Done()
        for n := range c {
            merged <- n
        }
    }

    wg.Add(len(cs))
    for _, c := range cs {
        go output(c)
    }

    // Закрываем merged когда все входы исчерпаны
    go func() {
        wg.Wait()
        close(merged)
    }()

    return merged
}
```

### Полный пример fan-out + fan-in с отменой

```go
func process(ctx context.Context, ids []int) <-chan Result {
    jobs := make(chan int)
    
    // Producer
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
    
    // Fan-out: 4 workers
    const numWorkers = 4
    results := make([]<-chan Result, numWorkers)
    for i := range numWorkers {
        results[i] = worker(ctx, jobs)
    }
    
    // Fan-in: merge results
    return merge(results...)
}
```

---

## Done-channel для отмены

```go
func generate(done <-chan struct{}, nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            select {
            case out <- n:       // отправить
            case <-done:         // или выйти при отмене
                return
            }
        }
    }()
    return out
}

done := make(chan struct{})
defer close(done) // сигнал отмены для всех downstream

nums := generate(done, 1, 2, 3, 4, 5)
```

**Done-channel vs context.Done()**:
- `context.Done()` предпочтительнее — стандарт, носит deadline и значения
- `done chan struct{}` — legacy паттерн, до context
- Всегда используй `close(done)` для broadcast, не `done <- struct{}{}` (send — только для одного получателя)

---

## Goroutine leak

### Причины

1. **Горутина заблокирована на receive**, а отправитель не отправит:
```go
// Leak: никто никогда не закроет ch
ch := make(chan int)
go func() {
    for v := range ch { // блокируется навсегда
        process(v)
    }
}()
```

2. **Горутина заблокирована на send**, а получатель не читает:
```go
out := make(chan Result) // unbuffered
go func() {
    out <- doWork() // блокируется если main уже вышла
}()
// main не читает из out
```

3. **Goroutine ждёт lock, который уже не будет released**

4. **Горутина ждёт ctx.Done(), а cancel не вызывается**:
```go
ctx, cancel := context.WithCancel(parent)
// забыли defer cancel()
go longRunning(ctx)
```

### Как ловить

```go
// goleak (uber-go/goleak) — в тестах
func TestFetch(t *testing.T) {
    defer goleak.VerifyNone(t) // проверит, нет ли утечек после теста
    
    // ... тест
}
```

```go
// pprof goroutine profile
resp, _ := http.Get("http://localhost:6060/debug/pprof/goroutine?debug=2")
// или через go tool pprof
```

```go
// runtime — счётчик горутин
fmt.Println(runtime.NumGoroutine())
```

### Правила предотвращения

1. Всегда закрывай канал на стороне отправителя
2. Используй `context` + `defer cancel()` для любой долгой горутины
3. При buffered channel — убедись, что кто-то дочитает буфер
4. `select { case <-done: return }` в каждом цикле, блокирующемся на канале

---

## `context` для отмены

```go
// Правильная передача context через цепочку вызовов
func (s *Service) Handle(ctx context.Context, req *Request) (*Response, error) {
    // Передаём ctx в каждый I/O вызов
    user, err := s.repo.Get(ctx, req.UserID)
    if err != nil {
        return nil, err
    }
    
    // Создаём дочерний ctx с timeout для внешнего вызова
    extCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
    defer cancel()
    
    data, err := s.external.Fetch(extCtx, user.ExternalID)
    if err != nil {
        return nil, fmt.Errorf("external fetch: %w", err)
    }
    
    return &Response{Data: data}, nil
}
```

```go
// Горутина с ctx.Done()
go func() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            doPeriodicWork()
        case <-ctx.Done():
            return // чистое завершение
        }
    }
}()
```

---

## `select` — мультиплексирование каналов

```go
select {
case msg := <-ch1:
    handle(msg)
case msg := <-ch2:
    handle(msg)
case <-time.After(5 * time.Second):
    // timeout
case <-ctx.Done():
    return ctx.Err()
default:
    // non-blocking: если ни один канал не готов
}
```

**Важно:**
- Если несколько case готовы — выбирается **случайно** (не FIFO)
- `default` делает select **неблокирующим**
- `select {}` — блокировка навсегда (иногда нужна в main)

### Неблокирующая отправка/получение

```go
// Неблокирующая отправка
select {
case ch <- val:
    // отправлено
default:
    // канал заполнен/нет получателя — не блокируемся
}

// Неблокирующее получение
select {
case val, ok := <-ch:
    if !ok {
        // канал закрыт
    }
    // обработать val
default:
    // нет данных
}
```

---

## Закрытие канала

```go
// Закрывать может только отправитель, не получатель
// Отправка в closed channel → panic
// Получение из closed channel → zero value + ok=false

ch := make(chan int, 3)
ch <- 1
ch <- 2
close(ch)

// Два способа читать закрытый канал:
for v := range ch {       // автоматически остановится при close
    fmt.Println(v)
}

v, ok := <-ch             // ok=false если канал закрыт
if !ok {
    fmt.Println("closed")
}
```

**Кто закрывает канал?**
- Закрывает тот, кто **пишет** (producer, не consumer)
- Если несколько producers — нужен WaitGroup + отдельная горутина-closer:

```go
var wg sync.WaitGroup
ch := make(chan int)

for i := 0; i < workers; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        ch <- produce()
    }()
}

go func() {
    wg.Wait()
    close(ch) // только когда все producers завершились
}()
```

---

## Interview-ready answer

**Q: Чем горутина отличается от OS thread?**

OS thread имеет фиксированный стек (1–8 MB), создаётся ОС — дорого (~1 мкс). Горутина имеет динамический стек (начиная с 2 KB), управляется рантаймом Go — дёшево (~200 нс, 0 системных вызовов). Планировщик Go (GMP) мультиплексирует тысячи горутин на десятки OS threads.

**Q: Когда использовать buffered channel?**

Buffered нужен когда producer и consumer работают с разной скоростью и небольшое накопление допустимо. Или как semaphore для ограничения параллелизма. Unbuffered — для строгой синхронизации: "я отправил → ты точно получил", done-channel сигналы.

**Q: Как обнаружить goroutine leak?**

`goleak.VerifyNone(t)` в тестах. В production — `runtime.NumGoroutine()` как метрика + `/debug/pprof/goroutine` для анализа стека. Основная причина — горутина заблокирована на канале или lock, которые уже не разблокируются. Решение — всегда передавать context, ставить `defer cancel()`, использовать `select { case <-ctx.Done(): return }`.
