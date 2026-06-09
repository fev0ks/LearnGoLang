# Goroutines and Channels

Конкурентность в Go строится на двух примитивах: горутины (дешёвые потоки, управляемые рантаймом) и каналы (безопасная передача данных между горутинами). Плюс `select` для мультиплексирования.

## Содержание

- [Goroutine lifecycle](#goroutine-lifecycle)
- [Unbuffered vs buffered channel](#unbuffered-vs-buffered-channel)
- [Pipeline паттерн](#pipeline-паттерн)
- [Fan-out / Fan-in](#fan-out--fan-in)
- [Done-channel для отмены](#done-channel-для-отмены)
- [Goroutine leak](#goroutine-leak)
- [`context` для отмены](#context-для-отмены)
- [`select` — мультиплексирование каналов](#select--мультиплексирование-каналов)
- [nil-канал как выключатель ветки select](#nil-канал-как-выключатель-ветки-select)
- [Закрытие канала](#закрытие-канала)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

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

## nil-канал как выключатель ветки select

Поскольку операции с `nil`-каналом блокируются вечно (см. Загадку 3), присваивание каналу `nil` **динамически отключает** соответствующую ветку `select` — она больше никогда не выбирается. Это идиоматичный способ управлять `select` без флагов и вложенных `if`. Два главных применения:

### 1. Fan-in: отключить закрытый канал, чтобы не крутить busy-loop

Закрытый канал в `select` «готов» всегда и отдаёт `(zero, false)` — если его не убрать, цикл жжёт CPU и плодит нули (Загадка 5). При слиянии нескольких источников зануляем переменную канала, как только он закрылся:

```go
func merge(c1, c2 <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for c1 != nil || c2 != nil {     // пока жив хоть один вход
            select {
            case v, ok := <-c1:
                if !ok {
                    c1 = nil             // вход закрыт → выключаем ветку
                    continue
                }
                out <- v
            case v, ok := <-c2:
                if !ok {
                    c2 = nil             // теперь select ждёт только живой канал
                    continue
                }
                out <- v
            }
        }
    }()
    return out
}
```

`nil`-ветки не выбираются, цикл аккуратно завершается, когда оба входа стали `nil`. Без зануления закрытый `c1` срабатывал бы на каждой итерации.

### 2. Гейт направления: включать/выключать отправку по состоянию

Горутина, которая **и читает, и пишет** в одном `select`, буферизуя значение. Канал отправки держим `nil`, пока отправлять нечего:

```go
func relay(in <-chan int, out chan<- int) {
    var pending int
    var outC chan<- int          // nil → ветка отправки выключена
    for {
        select {
        case v, ok := <-in:
            if !ok {
                return
            }
            pending = v
            outC = out           // появилось значение → включаем отправку
        case outC <- pending:    // сработает только когда outC != nil
            outC = nil           // отправили → снова выключаем, ждём новое
        }
    }
}
```

Зануление решает две проблемы разом:
- **не отправить «пустое»**: пока значения нет, `outC == nil` и ветка `outC <- pending` не выберется (иначе постоянно слали бы старое/нулевое);
- **нет busy-loop**: в отличие от `default`, `select` спокойно блокируется ровно на актуальных сейчас каналах.

> Приём особенно полезен в fan-in нескольких источников, закрывающихся в разное время, и в конвейерах с обратным давлением (выключаешь чтение, когда буфер полон; выключаешь запись, когда буфер пуст).

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

## Разбор примеров-загадок

Каверзные задачи на каналы и горутины — частый формат «что выведет / где баг».

### Загадка 1: захват цикловой переменной в горутине

```go
var wg sync.WaitGroup
for i := 0; i < 3; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        fmt.Print(i, " ")
    }()
}
wg.Wait()  // ?
```

<details>
<summary>Ответ</summary>

```
Go ≤ 1.21:  3 3 3   (в каком-то порядке)
Go ≥ 1.22:  0 1 2   (в произвольном порядке)
```

До Go 1.22 `i` — **одна** переменная на весь цикл; к моменту запуска горутин цикл часто уже дошёл до `i == 3`. С Go 1.22 цикловая переменная — **новая на каждой итерации**, поэтому каждая горутина видит своё значение (но порядок вывода всё равно недетерминирован — планировщик).

Фикс под старые версии — передавать параметром: `go func(i int){...}(i)`.
</details>

---

### Загадка 2: send/receive на закрытом канале

```go
ch := make(chan int, 1)
ch <- 1
close(ch)

v1, ok1 := <-ch
v2, ok2 := <-ch
fmt.Println(v1, ok1, v2, ok2)  // ?
ch <- 2                        // ?
```

<details>
<summary>Ответ</summary>

```
1 true 0 false
panic: send on closed channel
```

Чтение из закрытого канала сначала отдаёт **оставшиеся в буфере** значения (`1, true`), затем — zero value + `ok=false` (`0, false`) сколько угодно раз. А вот **отправка** в закрытый канал — всегда `panic`. Поэтому закрывает канал только отправитель и только когда точно больше не пишет.
</details>

---

### Загадка 3: nil-канал выключает case в select

```go
var ch chan int  // nil
select {
case <-ch:
    fmt.Println("got")
case <-time.After(100 * time.Millisecond):
    fmt.Println("timeout")
}
```

<details>
<summary>Ответ</summary>

```
timeout
```

Операции с **nil-каналом блокируются вечно** — поэтому первый case никогда не готов, и select всегда уходит в timeout. Это не баг, а приём: присваивая каналу `nil`, можно **динамически отключать** ветку select (например, отключить чтение из канала, который исчерпан, не выходя из цикла).
</details>

---

### Загадка 4: range по каналу без close

```go
func main() {
    ch := make(chan int, 2)
    ch <- 1
    ch <- 2
    for v := range ch {
        fmt.Println(v)
    }
}
```

<details>
<summary>Ответ</summary>

```
1
2
fatal error: all goroutines are asleep - deadlock!
```

`range` по каналу читает **до close**, а не до опустошения буфера. После двух значений он блокируется, ожидая следующего или закрытия. Раз отправителей больше нет и канал не закрыт — рантайм видит, что все горутины спят → deadlock. Нужен `close(ch)` на стороне отправителя.
</details>

---

### Загадка 5: закрытый канал в select «срабатывает» всегда

```go
done := make(chan struct{})
close(done)

count := 0
for count < 1_000_000 {
    select {
    case <-done:
        count++          // срабатывает на каждой итерации
    default:
    }
}
fmt.Println("burned", count, "iterations")
```

<details>
<summary>Ответ</summary>

Цикл прокрутится мгновенно и сожжёт CPU: **закрытый канал в select готов всегда** (отдаёт zero value без блокировки). Частая ошибка — оставить уже закрытый `done` в `select` внутри `for`, превратив его в busy-loop на 100% CPU.

Правильно — после получения сигнала отмены **выходить** из цикла (`return`), а не продолжать опрашивать закрытый канал:
```go
case <-done:
    return
```
</details>

---

### Загадка 6: «первый результат wins» течёт горутинами

```go
func first(urls []string) string {
    ch := make(chan string)  // unbuffered!
    for _, u := range urls {
        go func(u string) {
            ch <- fetch(u)   // все пишут сюда
        }(u)
    }
    return <-ch              // берём только первый
}
```

<details>
<summary>Ответ</summary>

Берём один результат, а остальные `len(urls)-1` горутин **навсегда виснут** на `ch <- fetch(u)`: канал unbuffered, читатель ушёл после первого значения. Классический goroutine leak в паттерне «кто первый».

Фиксы: буферизировать канал под все ответы (`make(chan string, len(urls))`), либо отменять остальных через `context` (и в горутине `select { case ch<-v: case <-ctx.Done(): }`).
</details>

---

## Interview-ready answer

Концентрат по реально частым вопросам про горутины и каналы.

**1. Чем горутина отличается от OS-потока?**
Поток — фиксированный стек (1–8 MB), создаёт ОС (дорого, ~1 мкс). Горутина — динамический стек от 2 KB, управляется рантаймом (дёшево, ~сотни нс, без syscall). Планировщик GMP мультиплексирует тысячи горутин на десятки потоков.

**2. Buffered или unbuffered?**
Unbuffered = синхронизация «отправил → точно получили» (done-сигналы, rendezvous). Buffered = развязка producer/consumer по скорости или семафор для ограничения параллелизма. Буфер маскирует дедлоки — для строгой синхронизации бери unbuffered.

**3. Кто и когда закрывает канал?**
Только отправитель и только когда больше не пишет. Отправка в закрытый — паника; чтение — отдаёт остаток буфера, потом zero+`ok=false`. При нескольких отправителях — `WaitGroup` + отдельная горутина-closer.

**4. Что делает nil-канал?**
Любая операция блокируется вечно. Применяется, чтобы отключить ветку `select` динамически. `close(nil)` — паника.

**5. Как ведёт себя select при нескольких готовых case?**
Выбирает случайный (не FIFO). `default` делает неблокирующим. Закрытый канал «готов» всегда → не оставляй его в `select` внутри `for` без `return`.

**6. Как поймать goroutine leak?**
`goleak.VerifyNone(t)` в тестах; в проде — метрика `runtime.NumGoroutine()` + `/debug/pprof/goroutine`. Причина — блокировка на канале/локе, который не разблокируется. Профилактика: `context` + `defer cancel()`, `select { case <-ctx.Done(): return }`, буфер под все ответы в «first-wins».
