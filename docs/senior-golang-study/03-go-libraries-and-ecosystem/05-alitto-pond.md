# alitto/pond

`github.com/alitto/pond/v2` — worker pool с динамическим числом воркеров, типизированным результатом (generics) и встроенными метриками. Цель — дать backpressure (ограничение очереди) и удобный сбор результатов без ручного управления каналами и `sync.WaitGroup`.

Требует Go 1.18+ (generics). Ноль внешних зависимостей, лицензия MIT.

## Содержание

- [Зачем](#зачем)
- [Базовый пул](#базовый-пул)
- [Задачи с ошибкой и с результатом](#задачи-с-ошибкой-и-с-результатом)
- [Группы задач: fan-out и ожидание](#группы-задач-fan-out-и-ожидание)
- [Backpressure: размер очереди](#backpressure-размер-очереди)
- [Динамический resize и subpools](#динамический-resize-и-subpools)
- [Метрики](#метрики)
- [Panic recovery](#panic-recovery)
- [Остановка](#остановка)
- [Типичные ошибки](#типичные-ошибки)
- [Когда использовать](#когда-использовать)
- [pond vs ants vs errgroup](#pond-vs-ants-vs-errgroup)
- [Interview-ready answer](#interview-ready-answer)

## Зачем

Базовый паттерн «N воркеров читают из канала задач» (см. [topics/02-concurrency/workerpool](../../../topics/02-concurrency/workerpool/README.md)) приходится писать руками каждый раз, и в нём легко допустить классические ошибки: race на закрытии канала, отсутствие panic recovery, отсутствие graceful shutdown, сбор результатов через ещё один канал.

```go
// Руками: канал задач + WaitGroup + recover + сбор результатов
tasks := make(chan func(), 100)
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for t := range tasks {
            func() {
                defer func() { _ = recover() }() // иначе паника убивает воркер
                t()
            }()
        }
    }()
}
// ...раздача задач, close(tasks), wg.Wait()

// С pond
pool := pond.NewPool(10)
pool.Submit(func() { doWork() })
pool.StopAndWait()
```

`pond` закрывает три вещи, которые в ручной версии всегда забывают: **panic recovery по умолчанию**, **ограничение очереди (backpressure)** и **сбор результата/ошибки на каждую задачу**.

## Базовый пул

```go
import "github.com/alitto/pond/v2"

// MaxConcurrency = 10: максимум 10 задач выполняется одновременно.
// Воркеры создаются лениво по мере поступления задач и завершаются,
// когда работы нет (динамический размер, не фиксированный пул горутин).
pool := pond.NewPool(10)

pool.Submit(func() {
    process(item)
})

// Дождаться завершения всех задач и остановить пул.
pool.StopAndWait()
```

Ключевое отличие от `ants`: **число воркеров динамическое**. `pond` держит до `MaxConcurrency` одновременных горутин, но не аллоцирует их заранее и отпускает простаивающие. Это «ограничитель параллелизма», а не «переиспользуемый набор горутин».

## Задачи с ошибкой и с результатом

```go
// Задача возвращает error — ошибка (и паника) доступны через Wait().
task := pool.SubmitErr(func() error {
    return doWork()
})
if err := task.Wait(); err != nil {
    log.Printf("task failed: %v", err)
}

// Типизированный результат через ResultPool[T].
pool := pond.NewResultPool[string](10)
task := pool.Submit(func() string {
    return fetch(url)
})
result, err := task.Wait() // err != nil если задача паниковала
```

`ResultPool[T]` — то, ради чего часто и берут `pond`: типобезопасный возврат значений без отдельного канала результатов и без `interface{}`.

## Группы задач: fan-out и ожидание

```go
// Группа связанных задач: один Wait() на всю пачку, первая ошибка возвращается.
group := pool.NewGroup()
for _, url := range urls {
    group.Submit(func() {
        crawl(url)
    })
}
err := group.Wait() // блокирует до завершения всех задач группы

// С контекстом: отмена/таймаут останавливает выдачу новых задач группы.
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
group := pool.NewGroupContext(ctx)
```

Семантически `Group` близок к `errgroup.Group`, но работает поверх общего пула с ограничением параллелизма (у `errgroup` лимит задаётся отдельно через `SetLimit`).

## Backpressure: размер очереди

```go
// Неограниченная очередь (по умолчанию): Submit никогда не блокирует,
// но память растёт неограниченно, если producer быстрее consumer'ов.
pond.NewPool(8) // == WithQueueSize(pond.Unbounded)

// Ограниченная очередь: при заполнении Submit БЛОКИРУЕТ producer'а — backpressure.
pond.NewPool(8, pond.WithQueueSize(1000))

// Без очереди: задача либо сразу берётся воркером, либо Submit ждёт свободного.
pond.NewPool(4, pond.WithQueueSize(0))

// Non-blocking: при заполнении очереди задача отбрасывается, а не ждёт.
pool := pond.NewPool(8, pond.WithQueueSize(1000), pond.WithNonBlocking(true))
task, ok := pool.TrySubmit(func() { work() })
if !ok {
    // очередь полна — задача отброшена (учтётся в DroppedTasks)
}
```

Это центральный вопрос на собесе: **что происходит, когда задачи приходят быстрее, чем обрабатываются**. Три стратегии — блокировать producer'а (bounded queue), копить в памяти (unbounded), отбрасывать (non-blocking). `pond` даёт выбрать явно.

## Динамический resize и subpools

```go
// Менять лимит параллелизма на лету (например, по сигналу нагрузки).
pool.Resize(20) // увеличить
pool.Resize(5)  // уменьшить

// Subpool — дочерний пул, делящий ресурсы родителя, со своим лимитом.
// Удобно ограничить «тяжёлую» категорию задач внутри общего пула.
subpool := pool.NewSubpool(5)
subpool.Submit(func() { heavyJob() })
subpool.StopAndWait()
```

## Метрики

```go
pool.RunningWorkers()   // активных воркеров сейчас
pool.WaitingTasks()     // в очереди
pool.SubmittedTasks()   // всего отправлено
pool.SuccessfulTasks()  // успешно завершено
pool.FailedTasks()      // завершено с ошибкой/паникой
pool.DroppedTasks()     // отброшено из-за полной очереди (non-blocking)
pool.CompletedTasks()   // всего завершено
```

Метрики идут «из коробки» — их легко прокинуть в Prometheus-gauge без дополнительной обвязки.

## Panic recovery

```go
// По умолчанию паника в задаче ловится и возвращается как error через Wait().
task := pool.SubmitErr(func() error {
    panic("boom")
})
err := task.Wait() // err != nil, воркер НЕ умирает, пул живёт дальше

// Отключить (если хочется fail-fast на панике).
pool := pond.NewPool(10, pond.WithoutPanicRecovery())
```

В ручном воркерпуле забытый `recover` — причина деградации: упавший воркер не восстанавливается, и пул постепенно «тает». `pond` ловит панику по умолчанию.

## Остановка

```go
pool.StopAndWait()       // дождаться текущих задач и остановить
pool.Stop().Wait()       // то же, но Stop() возвращает «future» для ожидания
```

После `Stop()` новые `Submit` отклоняются. Graceful shutdown — встроенный, не нужно вручную закрывать канал задач.

## Типичные ошибки

```go
// 1. Unbounded-очередь под нагрузкой → рост памяти и OOM.
//    Если producer быстрее воркеров, всегда задавай WithQueueSize.
pool := pond.NewPool(8) // опасно для долгоживущего сервиса с пиками

// 2. Забыть Wait() у задачи с результатом → паника проглатывается молча.
pool.SubmitErr(func() error { return doWork() }) // ошибку никто не прочитал

// 3. Слишком высокий MaxConcurrency для CPU-bound работы.
//    Для CPU-bound оптимум ≈ runtime.GOMAXPROCS(0), а не сотни воркеров.
pool := pond.NewPool(1000) // для IO-bound ок, для CPU-bound — лишние переключения
```

## Когда использовать

- нужен **сбор результатов** типобезопасно (`ResultPool[T]`) — главный плюс над `ants`
- нужен **backpressure** через ограниченную очередь без ручных каналов
- нужны **группы задач** с одним `Wait()` (альтернатива `errgroup` поверх общего лимита)
- нужны **метрики пула** из коробки
- нужен **динамический resize** под меняющуюся нагрузку

**Не использовать:**
- когда хватает `errgroup.Group` + `SetLimit` (одна пачка задач, без долгоживущего пула)
- ради экстремального throughput на миллионах коротких задач — там профильнее `ants` (переиспользование горутин, меньше аллокаций)

## pond vs ants vs errgroup

| | `alitto/pond` | `panjf2000/ants` | `errgroup` + `SetLimit` |
|---|---|---|---|
| Модель | динамический лимит параллелизма | фиксированный пул переиспользуемых горутин | лимит на разовую пачку |
| Результаты | `ResultPool[T]`, типизированно | руками (через замыкания/каналы) | руками |
| Backpressure | bounded/unbounded/non-blocking | `WithMaxBlockingTasks`, `WithNonblocking` | через лимит |
| Метрики | встроенные | `Running/Free/Cap` | нет |
| Panic recovery | по умолчанию | `WithPanicHandler` | паника всплывает |
| Фокус | удобство, результаты, backpressure | throughput, экономия памяти | простота, разовый fan-out |

## Interview-ready answer

**1. Что такое `pond` и какую модель пула он реализует?**

- Worker pool с **динамическим** числом воркеров: это ограничитель параллелизма (`MaxConcurrency`), а не фиксированный набор горутин. Воркеры создаются лениво под нагрузку и отпускаются, когда работы нет. Закрывает три вещи, которые забывают в ручном воркерпуле: panic recovery, backpressure и сбор результата на задачу.

**2. Как `pond` собирает результаты задач?**

- Через `ResultPool[T]`: `task := pool.Submit(...)`, затем `result, err := task.Wait()` — типобезопасно, без отдельного канала результатов и `interface{}`. Для задач с ошибкой — `SubmitErr(func() error)`. Это главное преимущество над `ants`, где результаты собираются вручную.

**3. Как настраивается backpressure?**

- `WithQueueSize`: **unbounded** (по умолчанию — `Submit` не блокирует, но память растёт), **bounded** (`Submit` блокирует producer'а при заполнении), **0** (без очереди). Плюс `WithNonBlocking(true)` + `TrySubmit` — при полной очереди задача отбрасывается (учитывается в `DroppedTasks`). Это прямой ответ на вопрос «что будет, если задачи приходят быстрее, чем обрабатываются».

**4. Что с паникой в задаче?**

- По умолчанию паника **ловится** и возвращается как `error` через `Wait()`, воркер не умирает и пул живёт дальше. Отключается через `WithoutPanicRecovery()` (если нужен fail-fast).

**5. Когда `pond`, а когда `ants` или `errgroup`?**

- `pond` — когда нужен типизированный сбор результатов, backpressure и метрики из коробки. `ants` — когда важен максимальный throughput и экономия памяти на миллионах коротких задач (переиспользование горутин). `errgroup` + `SetLimit` — для разового fan-out без долгоживущего пула. См. [panjf2000/ants](./06-panjf2000-ants.md).

**6. Главный риск в production?**

- Unbounded-очередь под пиковой нагрузкой → рост памяти и OOM, если producer быстрее воркеров. В сервисе всегда задаю `WithQueueSize` явно. Второй частый промах — забыть `Wait()` у задачи с результатом, тогда ошибка/паника проглатывается молча.
