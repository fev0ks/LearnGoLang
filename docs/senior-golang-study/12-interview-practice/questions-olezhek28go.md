# 20 задач с Go-собесов (Олег Козырев / olezhek28go)

Разбор тренажёра к ролику «Worker Pool в Go»: 20 вопросов с реальных Go-собеседований — concurrency, runtime, channels, interfaces, http. По каждому вопросу — краткий ответ по существу, типичные ошибки и ссылки на разделы конспекта с глубоким разбором.

## Содержание

1. [Исправить код (race + loop var + нет ожидания)](#01--исправить-код)
2. [Что такое GOMAXPROCS](#02--что-такое-gomaxprocs)
3. [Поиск на 3 сервера (first success)](#03--поиск-на-3-сервера)
4. [Утечка памяти и OOM (swap on/off)](#04--утечка-памяти-и-oom)
5. [Async-ручка создания заказа](#05--async-ручка-создания-заказа)
6. [Pipeline на N стадий](#06--pipeline-на-n-стадий)
7. [select по каналам — что выведет](#07--select-по-каналам)
8. [Процесс vs системный поток](#08--процесс-vs-системный-поток)
9. [Дедлок на двух мьютексах](#09--дедлок-на-двух-мьютексах)
10. [LRU cache за O(1)](#10--lru-cache) · [потокобезопасность](#потокобезопасность) · [бонус: LFU](#бонус-lfu-least-frequently-used)
11. [Что с закрытым каналом (4 случая)](#11--что-с-закрытым-каналом)
12. [Ревью кода кеша](#12--ревью-кода-кеша)
13. [Broadcast канал](#13--broadcast-канал)
14. [Mutex vs RWMutex](#14--mutex-vs-rwmutex)
15. [Semaphore через канал](#15--semaphore-через-канал)
16. [context.WithValue — когда валидно](#16--contextwithvalue)
17. [Найди goroutine leak](#17--найди-goroutine-leak)
18. [Rate limiter (token bucket)](#18--rate-limiter)
19. [Errgroup vs Worker pool](#19--errgroup-vs-worker-pool)
20. [Nil interface — почему false](#20--nil-interface)

---

## 01 · Исправить код

> В этом коде есть как минимум 3 проблемы. Найди их и почини.

```go
func main() {
    var max int
    for i := 1000; i > 0; i-- {
        go func() {
            if i%2 == 0 && i > max {
                max = i
            }
        }()
    }
    fmt.Printf("Maximum is %d", max)
}
```

Три (фактически четыре) проблемы:

- **Гонка данных на `max`** — все горутины читают и пишут общую переменную без синхронизации. `go run -race` это ловит. Поведение не определено.
- **`main` не дожидается горутин** — `fmt.Printf` выполняется сразу после запуска цикла, горутины ещё не отработали. Скорее всего напечатает `Maximum is 0`.
- **Захват переменной цикла `i`** (до Go 1.22) — все горутины делят одну `i`, к моменту их запуска она уже равна 0. В Go 1.22+ переменная цикла своя на каждой итерации, но гонка на `max` и отсутствие ожидания остаются.
- **Бонус: горутины вообще не нужны** — поиск максимума чётного числа от 1000 — это просто 1000, считается тривиально и последовательно.

Как чинить: дожидаться горутин через `sync.WaitGroup`, защищать `max` мьютексом (или `atomic`), передавать `i` аргументом. Но правильный senior-ответ — *указать, что параллелизм тут лишний*, и предложить либо корректную параллельную версию, либо честно последовательную.

<details>
<summary>Решение</summary>

```go
// Вариант 1: корректная параллельная версия (как «просили»)
func main() {
    var (
        mu  sync.Mutex
        max int
        wg  sync.WaitGroup
    )
    for i := 1000; i > 0; i-- {
        wg.Add(1)
        go func(i int) { // i передаём аргументом — на случай Go < 1.22
            defer wg.Done()
            if i%2 == 0 {
                mu.Lock()
                if i > max {
                    max = i
                }
                mu.Unlock()
            }
        }(i)
    }
    wg.Wait()
    fmt.Printf("Maximum is %d\n", max) // 1000
}

// Вариант 2: честный ответ — горутины не нужны
func main() {
    max := 0
    for i := 1000; i > 0; i-- {
        if i%2 == 0 && i > max {
            max = i
        }
    }
    fmt.Printf("Maximum is %d\n", max) // 1000
}
```

Через `atomic` максимум считать неудобно (нужен CAS-цикл), поэтому здесь мьютекс уместнее.
</details>

Глубже: [Memory model и happens-before](../01-go-core/concurrency-and-performance/01-memory-model.md), [Goroutines и channels](../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md), [Sync-примитивы](../01-go-core/concurrency-and-performance/03-sync-primitives.md). Про захват loop variable — раздел «Захват переменной цикла» в [worker-pool](coding-tasks/concurrency/01-worker-pool.md).

---

## 02 · Что такое GOMAXPROCS

> Что такое GOMAXPROCS и зачем оно нужно? Может ли приложение с `GOMAXPROCS=4` потреблять больше 4 ядер CPU?

- **GOMAXPROCS — число `P` (processor)** в планировщике Go: сколько горутин могут *одновременно выполнять Go-код* на ОС-потоках (`M`). Это верхняя граница параллелизма user-level кода, а не числа потоков.
- По умолчанию равно числу логических CPU (`runtime.NumCPU()`). С **Go 1.25** при работе в контейнере значение по умолчанию учитывает CPU-лимит из cgroup (округление вверх), чтобы не плодить лишние `P` на ограниченном по квоте поде.
- Зачем менять: ограничить нагрузку на shared-хосте, уменьшить контеншн на малом числе ядер, воспроизводимость бенчмарков (`GOMAXPROCS=1`).
- **Да, приложение может использовать больше 4 ядер CPU при `GOMAXPROCS=4`.** GOMAXPROCS ограничивает параллелизм *горутин, выполняющих Go-код*, но не общее число потоков процесса:
  - **Блокирующие syscall'ы** — поток в syscall отвязывается от `P`, рантайм создаёт/берёт новый `M` под другой `P`. Потоков (и активных ядер) может быть много больше 4.
  - **cgo / C-код** — вызовы в C идут на отдельных потоках вне контроля `P`.
  - **GC и рантайм** — фоновые worker'ы, sysmon-поток.
  - На уровне ОС видно `N` потоков; сколько из них реально на CPU — решает планировщик ОС, а не GOMAXPROCS.

Глубже: [Scheduler и preemption (G-M-P, syscalls)](../01-go-core/runtime-scheduler/01-scheduler-and-preemption.md), [Syscall handling](../01-go-core/runtime-scheduler/02-syscall.md), [Go 1.25 — cgroup-aware GOMAXPROCS](../15-go-version-differences/go1.25.md).

---

## 03 · Поиск на 3 сервера

> Дана `func (s *Search) Search(server string, query string) ([]string, error)`. Напиши обёртку: параллельно запрашиваем 3 сервера, возвращаем первый успешный результат. Если все три вернули ошибку — вернуть ошибку.

Ключевые моменты, которые проверяют:

- Запустить 3 горутины, собрать результаты через канал.
- Вернуть **первый успешный** — не ждать остальных.
- **Отменить остальные** через `context` после первого успеха (иначе висящие запросы = трата ресурсов).
- Не словить goroutine leak: канал результатов **буферизованный** на 3, чтобы отстающие горутины не блокировались на отправке после того, как обёртка уже вернулась.
- **Реагировать на отмену родительского контекста**: пока идут запросы, `ctx` мог отмениться извне. Сбор результатов оборачивается в `select` с `ctx.Done()`, чтобы выйти сразу, а не ждать, пока все «отменённые» `Search` доедут до канала. Буфер на 3 при этом не даёт отстающим горутинам зависнуть на отправке.
- Если все три — ошибки, собрать их (`errors.Join`) и вернуть.

<details>
<summary>Решение</summary>

```go
func (s *Search) SearchAll(ctx context.Context, query string, servers []string) ([]string, error) {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel() // отменяем «проигравшие» запросы при выходе

    type res struct {
        data []string
        err  error
    }
    // Буфер = len(servers): отстающие горутины не зависнут на отправке.
    ch := make(chan res, len(servers))

    for _, server := range servers {
        go func(server string) {
            data, err := s.Search(ctx, server, query)
            ch <- res{data, err}
        }(server)
    }

    var errs []error
    for range servers {
        select {
        case r := <-ch:
            if r.err == nil {
                return r.data, nil // первый успех — cancel() отменит остальных
            }
            errs = append(errs, r.err)
        case <-ctx.Done():
            // родительский контекст отменился, пока шли запросы —
            // выходим сразу, не дожидаясь отстающих горутин (буфер не даст им зависнуть)
            return nil, ctx.Err()
        }
    }
    return nil, errors.Join(errs...) // все упали
}
```

Если `Search` не принимает `ctx`, отмена «проигравших» невозможна — стоит проговорить это и предложить добавить `ctx` в сигнатуру. С `errgroup` лаконичнее, но «первый успех» там выражается неестественно (errgroup заточен под «первая ошибка»), поэтому ручной сбор через канал здесь чище.
</details>

Глубже: [Fan-in / fan-out](coding-tasks/concurrency/03-fan-in-fan-out.md), [Context patterns (отмена, propagation)](../01-go-core/concurrency-and-performance/04-context-patterns.md).

---

## 04 · Утечка памяти и OOM

> На голом сервере (без cgroup-лимитов) сервис течёт по памяти. Что произойдёт, когда память кончится? Разбери два сценария: со свопом и без.

**Без свопа:**

- Память (RAM) заполняется, страничный кэш вытесняется, аллокации упираются в предел.
- Ядро Linux запускает **OOM killer**: по `oom_score` (учитывает размер процесса, `oom_score_adj`) выбирает жертву и шлёт ей `SIGKILL`. Обычно жертва — сам жирный сервис, но может прилететь и соседу.
- Процесс убивается мгновенно, без graceful shutdown (`SIGKILL` не перехватывается). В dmesg — запись `Out of memory: Killed process ...`.

**Со свопом:**

- Перед OOM ядро вытесняет «холодные» страницы на диск (swap). Процесс продолжает жить, но...
- Начинается **thrashing**: горячие страницы постоянно гоняются RAM↔диск, латентность растёт на порядки, CPU уходит в iowait. Сервис формально жив, но фактически не отвечает (хуже, чем быстрый рестарт).
- Когда исчерпан и swap — приходит тот же OOM killer.

Нюансы для Go-сервиса:

- Go отдаёт память ОС лениво (`MADV_FREE`/`MADV_DONTNEED`); RSS может держаться высоким даже после освобождения, что приближает OOM.
- `GOMEMLIMIT` (soft limit) заставляет GC работать агрессивнее у границы — способ оттянуть OOM, но не лечит саму утечку.
- На голом сервере без cgroup нет «мягкого» лимита на процесс — упирается весь хост; в Kubernetes сначала сработал бы cgroup-лимит пода (OOM внутри cgroup).

Глубже: [Virtual memory и paging](../10-devops-and-observability/hardware-and-os/05-virtual-memory-and-paging.md), [Linux virtual memory](../10-devops-and-observability/linux/01-virtual-memory.md), [Garbage collector (GOMEMLIMIT, возврат памяти ОС)](../01-go-core/memory-internals/04-garbage-collector.md), [Memory profiling](../01-go-core/profiling/03-memory-profiling.md), [Symptom-driven troubleshooting](../10-devops-and-observability/incident-response-and-investigation/02-symptom-driven-troubleshooting.md).

---

## 05 · Async-ручка создания заказа

> Создание заказа занимает от 10 мс до 40 с. Ручка долгая в синхронном режиме. Нужно, чтобы она *всегда* отвечала в течение 1 секунды: либо результатом, либо сообщением «заказ создаётся». Можно править структуры и сигнатуры.

Шаблон:

```go
type usecase interface {
    CreateOrder(order Order) (Result, error)
}
type Handler struct{ usecase usecase }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req Request
    _ = json.NewDecoder(r.Body).Decode(&req)
    _, err := h.usecase.CreateOrder(Order{Payload: req.Payload})
    // ...
}
```

Идея решения — **«гонка» с таймаутом 1 секунда**:

- Запустить создание заказа в горутине, результат — в буферизованный канал (буфер 1, чтобы горутина не зависла, если ручка уже ответила по таймауту).
- `select` между «результат пришёл» и «прошла 1 секунда».
- Если успел за 1 с → отдать `200` с результатом. Если нет → отдать `202 Accepted` с `orderID` и статусом «создаётся», а создание **продолжает идти в фоне** и доводится до конца.
- Чтобы клиент потом узнал результат — заранее сгенерировать `orderID`, по нему отдать статус через polling (`GET /orders/{id}`) или вебхук/SSE. То есть это полноценный async-паттерн: ручка лишь инициирует.

Важные детали, которые проверяют:

- **Буфер 1 у канала результата** — иначе фоновая горутина залипнет на отправке (никто не читает после таймаута) → goroutine leak.
- **Нельзя завязывать фоновую работу на `r.Context()`** — он отменяется по завершении HTTP-запроса, и фоновое создание оборвётся. Нужен отдельный (background) контекст с собственным таймаутом ≈40 с.
- **Идемпотентность** — клиент по таймауту может ретраить; нужен idempotency-key, чтобы не создать заказ дважды.
- Для production — не «горутина на запрос», а постановка задачи в очередь/outbox + воркеры; ручка лишь принимает и возвращает `orderID`.

<details>
<summary>Решение</summary>

```go
type Result struct {
    OrderID string
    Status  string // "created" | "processing"
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req Request
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        return
    }

    orderID := newOrderID() // знаем ID заранее — по нему клиент опросит статус
    done := make(chan Result, 1) // буфер 1: фоновая горутина не зависнет

    go func() {
        // ВАЖНО: НЕ r.Context() — он умрёт вместе с HTTP-ответом.
        ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
        defer cancel()
        res, err := h.usecase.CreateOrder(ctx, Order{ID: orderID, Payload: req.Payload})
        if err != nil {
            // зафиксировать статус "failed" в хранилище по orderID
            return
        }
        done <- res
    }()

    select {
    case res := <-done:
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(res) // успели за 1 с
    case <-time.After(time.Second):
        w.WriteHeader(http.StatusAccepted) // 202
        _ = json.NewEncoder(w).Encode(Result{OrderID: orderID, Status: "processing"})
    }
}
```

Клиент при `202` опрашивает `GET /orders/{orderID}` или подписывается на вебхук/SSE.
</details>

Глубже: [Background workers (постановка в очередь, outbox)](../04-architecture-and-patterns/patterns/04-background-workers.md), [Идемпотентность](../05-system-design/reliability-patterns/06-idempotency.md), [Таймауты и deadlines](../05-system-design/reliability-patterns/01-timeouts-and-deadlines.md), [Write-request с очередью и async-обработкой](../05-system-design/external-request-flows/03-write-request-with-queue-and-async-processing.md).

---

## 06 · Pipeline на N стадий

> Реализуй `generator → stage1 → stage2 → sink` через каналы. Между стадиями небуферизованные каналы. Отмена через `context.Context` на любом этапе. Без утечек горутин при отмене. Кто закрывает каналы и в какой момент?

- **Каждая стадия — функция `func(ctx, in <-chan T) <-chan R`**: читает из входного канала, пишет в свой выходной, который сама же и закрывает.
- **Правило закрытия: канал закрывает только его писатель (owner), при выходе из своей горутины** (`defer close(out)`). Закрытие распространяется вниз по пайплайну: закрылся вход стадии → её цикл `for range` завершился → она закрыла свой выход.
- **Отмена**: каждая стадия в `select` слушает и `ctx.Done()`, и попытку записи в `out`. При отмене стадия выходит, закрывает свой `out` → следующая стадия видит закрытый вход и тоже завершается. Так горутины не виснут на отправке в канал, который никто не читает.
- **Почему нет утечек**: отправка в `out` обёрнута в `select` с `ctx.Done()` — если читатель ушёл (отмена), писатель не блокируется навечно, а выходит.

<details>
<summary>Решение</summary>

```go
func generator(ctx context.Context, nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            select {
            case out <- n:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

// stage — обобщённая стадия: применяет f к каждому элементу.
func stage[T, R any](ctx context.Context, in <-chan T, f func(T) R) <-chan R {
    out := make(chan R)
    go func() {
        defer close(out)
        for v := range in { // завершится, когда вход закрыт
            select {
            case out <- f(v):
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    gen := generator(ctx, 1, 2, 3, 4, 5)
    s1 := stage(ctx, gen, func(x int) int { return x * 2 })
    s2 := stage(ctx, s1, func(x int) int { return x + 1 })

    for v := range s2 { // sink
        fmt.Println(v)
        // cancel() где-нибудь здесь корректно остановит весь пайплайн
    }
}
```

При `cancel()` все стадии выйдут по `ctx.Done()`, каждая закроет свой `out`, цепочка `for range` свернётся сверху вниз — горутины не утекут.
</details>

Глубже: подробный разбор — [Pipeline (coding-task)](coding-tasks/concurrency/04-pipeline.md), [Context patterns](../01-go-core/concurrency-and-performance/04-context-patterns.md), [Go Concurrency Patterns: Pipelines](https://go.dev/blog/pipelines).

---

## 07 · select по каналам

> Что выведет код? Что изменится, если канал сделать небуферизованным (`make(chan int)`)?

```go
func main() {
    ch := make(chan int, 1)
    for i := 0; i < 10; i++ {
        select {
        case x := <-ch:
            print(x)
        case ch <- i:
        }
    }
}
```

**С буфером 1 → выведет `02468`.** Разбор по итерациям (в `select` выполняется единственный готовый case):

- `i=0`: буфер пуст → читать нельзя, писать можно → пишем `0`, буфер `[0]`.
- `i=1`: буфер полон → читать можно, писать нельзя → читаем, печатаем `0`, буфер пуст.
- `i=2`: пишем `2` → `[2]`.
- `i=3`: читаем, печатаем `2`.
- ... чередование «пишем чётное / печатаем предыдущее чётное» → печатается `0 2 4 6 8`.

**Без буфера (`make(chan int)`) → deadlock на первой же итерации.** В одной горутине у небуферизованного канала ни send, ни receive не готовы: send требует ждущего получателя, receive — ждущего отправителя, а другой горутины нет. `select` без `default` блокируется навсегда, рантайм печатает `fatal error: all goroutines are asleep - deadlock!`. Ничего не выводится.

Мораль: готовность case у небуферизованного канала определяется наличием *парной* горутины, у буферизованного — наличием места/данных в буфере.

Глубже: [Goroutines и channels (буферизация, select, deadlock)](../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md).

---

## 08 · Процесс vs системный поток

> Чем отличается процесс от системного потока с точки зрения ОС? Как процессы обмениваются информацией?

**Процесс vs поток:**

| | Процесс | Поток (thread) |
|---|---|---|
| Адресное пространство | своё, изолированное | общее с другими потоками процесса |
| Что приватно | весь heap/data/code, FD-таблица | свой стек, регистры, TLS |
| Создание | дорого (`fork`, копия таблиц страниц) | дёшево (общая память) |
| Переключение | дорого (смена адресного пространства, сброс TLB) | дешевле (адресное пространство то же) |
| Изоляция при крахе | падение одного не роняет другие | паника/segfault в потоке роняет весь процесс |
| Планирование (Linux) | через `task_struct` | тоже `task_struct` — ядру это «задачи» с разной общей памятью |

Суть: процесс — единица *изоляции и владения ресурсами*; поток — единица *планирования/исполнения*. Потоки одного процесса делят память, поэтому общаются через неё напрямую (и нуждаются в синхронизации).

> **Про строку «Переключение».** Имеется в виду **context switch** — планировщик ОС снимает с ядра одну задачу и ставит другую (само переключение всегда между потоками; «между процессами» = между потоками разных процессов). Сохранение/восстановление регистров есть всегда. Дорогим переключение делает именно смена адресного пространства: загрузка нового корня таблиц страниц (`CR3` на x86), что обычно **инвалидирует TLB** — кэш переводов «виртуальный → физический адрес». После этого первые обращения к памяти промахиваются мимо TLB и идут через page-table walk, плюс остывают L1/L2-кэши. При переключении потоков **одного** процесса адресное пространство то же → `CR3` не меняется → TLB цел, поэтому дешевле. Современные CPU смягчают сброс через **PCID/ASID** (тэгирование записей TLB идентификатором процесса), но затраты не убирают полностью. Переключение **горутин** внутри одного потока ещё дешевле — оно в user-space, ядро и `CR3` не вовлечены (см. [Context switching и scheduling](../10-devops-and-observability/hardware-and-os/07-context-switching-and-scheduling.md)).

**IPC между процессами** (память не общая, нужны явные механизмы):

- **Pipes / named pipes (FIFO)** — байтовый поток между процессами.
- **Unix domain sockets / TCP-сокеты** — локально или по сети.
- **Shared memory** (`shmget`/`mmap`) — самый быстрый: общий регион памяти + синхронизация семафорами/мьютексами.
- **Message queues** (POSIX/System V).
- **Signals** — примитивное уведомление (`SIGTERM` и т.п.), без полезной нагрузки.
- **Файлы / memory-mapped files**, **eventfd/pipe** для нотификаций.

Связь с Go: горутина — не поток ОС, а user-level «задача», которую рантайм мультиплексирует на потоки (`M`) — см. [Scheduler](../01-go-core/runtime-scheduler/01-scheduler-and-preemption.md).

Глубже: [Процессы и потоки](../10-devops-and-observability/hardware-and-os/06-processes-and-threads.md), [Context switching и scheduling](../10-devops-and-observability/hardware-and-os/07-context-switching-and-scheduling.md), [Сигналы и процессы](../10-devops-and-observability/linux/04-signals-and-processes.md).

---

## 09 · Дедлок на двух мьютексах

> Найди дедлок и предложи два способа починить.

```go
var (mu1 sync.Mutex; mu2 sync.Mutex)

func work1() { mu1.Lock(); defer mu1.Unlock(); time.Sleep(10*time.Millisecond); mu2.Lock(); defer mu2.Unlock() }
func work2() { mu2.Lock(); defer mu2.Unlock(); time.Sleep(10*time.Millisecond); mu1.Lock(); defer mu1.Unlock() }
```

- **Причина — нарушение порядка блокировок (lock-ordering deadlock).** `work1` берёт `mu1`, потом `mu2`; `work2` берёт `mu2`, потом `mu1`. Если вызвать их параллельно: `work1` держит `mu1` и ждёт `mu2`, `work2` держит `mu2` и ждёт `mu1` — circular wait, оба висят навсегда. `time.Sleep` лишь гарантированно подгоняет окно гонки.
- Это классический deadlock по условиям Коффмана (mutual exclusion + hold-and-wait + no preemption + circular wait); ломаем любое условие.

Способы починить:

1. **Единый порядок захвата** (ломаем circular wait) — обе функции берут мьютексы всегда в одном порядке: сначала `mu1`, потом `mu2`. Самый правильный и масштабируемый способ.
2. **Один мьютекс вместо двух** — если ресурсы всегда берутся вместе, два мьютекса не нужны; ломаем hold-and-wait, защищая всё одним замком.
3. (Бонус) **`TryLock` с откатом** — если не удалось взять второй замок, отпустить первый, подождать и повторить. Ломает hold-and-wait, но риск livelock; для собеса — упомянуть как вариант, не как основной.

<details>
<summary>Решение (единый порядок)</summary>

```go
// Оба берут mu1, затем mu2 — циклического ожидания больше нет.
func work1() {
    mu1.Lock(); defer mu1.Unlock()
    mu2.Lock(); defer mu2.Unlock()
    // ...
}
func work2() {
    mu1.Lock(); defer mu1.Unlock()
    mu2.Lock(); defer mu2.Unlock()
    // ...
}
```

Если по логике одна из функций обязана брать `mu2` первым — выделить общий helper, инкапсулирующий *единый* порядок, и звать только его.
</details>

Глубже: [Sync-примитивы (Mutex, TryLock, deadlock)](../01-go-core/concurrency-and-performance/03-sync-primitives.md).

---

## 10 · LRU cache

> Реализуй LRU-кеш фиксированного размера. `Get`/`Put` за O(1). При переполнении вытесняется least-recently-used. `Get` обновляет «свежесть».

- **Структура — `map` + двусвязный список.** `map[key]*node` даёт O(1) поиск, двусвязный список держит порядок использования: голова — самый свежий, хвост — кандидат на вытеснение.
- `Get`: нашли в map → переносим узел в голову списка → возвращаем значение. O(1).
- `Put`: если ключ есть — обновили значение, в голову. Если нет — создали узел в голове, добавили в map; если размер превысил `capacity` — удалили хвост (и из списка, и из map). O(1).
- Двусвязный (а не односвязный) список нужен, чтобы удалять/перемещать узел за O(1), зная только сам узел.
- В Go удобно взять `container/list`, но на собесе ценят ручную реализацию узлов — показывает понимание указателей.
- Для потокобезопасности — обернуть мьютексом (RWMutex здесь не помогает: `Get` тоже мутирует список).

<details>
<summary>Решение</summary>

```go
type entry struct {
    key        string
    value      any
    prev, next *entry
}

type LRU struct {
    capacity   int
    items      map[string]*entry
    head, tail *entry // head — свежий, tail — старый (sentinel'ы)
}

func New(capacity int) *LRU {
    head, tail := &entry{}, &entry{}
    head.next, tail.prev = tail, head
    return &LRU{capacity: capacity, items: make(map[string]*entry), head: head, tail: tail}
}

func (c *LRU) remove(e *entry) {
    e.prev.next, e.next.prev = e.next, e.prev
}
func (c *LRU) pushFront(e *entry) {
    e.prev, e.next = c.head, c.head.next
    c.head.next.prev, c.head.next = e, e
}

func (c *LRU) Get(key string) (any, bool) {
    e, ok := c.items[key]
    if !ok {
        return nil, false
    }
    c.remove(e)
    c.pushFront(e) // обновляем свежесть
    return e.value, true
}

func (c *LRU) Put(key string, value any) {
    if e, ok := c.items[key]; ok {
        e.value = value
        c.remove(e)
        c.pushFront(e)
        return
    }
    e := &entry{key: key, value: value}
    c.items[key] = e
    c.pushFront(e)
    if len(c.items) > c.capacity {
        lru := c.tail.prev // вытесняем самый старый
        c.remove(lru)
        delete(c.items, lru.key)
    }
}
```

Sentinel-узлы `head`/`tail` убирают спецслучаи с `nil` на краях списка.
</details>

### Потокобезопасность

Реализация выше — однопоточный baseline (так и стоит начинать на собесе: сперва логика за O(1), thread-safety — отдельным шагом). При конкурентном доступе она ломается: гонка на `map` (`fatal error: concurrent map read and map write`) и порча двусвязного списка.

- **`Mutex`, а не `RWMutex`.** И `Put`, и **`Get` мутируют структуру**: `Get` переносит узел в голову списка, обновляя «свежесть». То есть «читающая» операция фактически пишет. Под `RLock` два «читателя» одновременно начнут перелинковывать один список → гонка. Нужен эксклюзивный `Lock` на обе операции — `RWMutex` тут не даёт выигрыша.
- **Шардирование** — если один глобальный `Mutex` становится бутылочным горлышком под высокой нагрузкой: N независимых LRU по `hash(key) % N`, каждый со своим замком. Снимает контеншн, ценой того, что capacity делится между шардами и вытеснение становится «пошардовым».

```go
type LRU struct {
    mu         sync.Mutex
    capacity   int
    items      map[string]*entry
    head, tail *entry
}

func (c *LRU) Get(key string) (any, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()
    // ... та же логика, что и выше
}

func (c *LRU) Put(key string, value any) {
    c.mu.Lock()
    defer c.mu.Unlock()
    // ... та же логика, что и выше
}
```

### Бонус: LFU (Least Frequently Used)

Частый follow-up — «а если вытеснять не самый давний, а самый редко используемый?». Это **LFU**: у каждого элемента счётчик обращений, при переполнении выкидывается элемент с **наименьшей частотой**.

- **Отличие от LRU**: LRU смотрит на *давность* (recency) последнего обращения, LFU — на *частоту* (frequency) за всё время. Пример, где LFU лучше: элемент, к которому обращались 1000 раз, не должен вытесняться из-за одной свежей вставки нового ключа, как было бы в LRU.
- **Наивная реализация** (min-heap по частоте) даёт `Get`/`Put` за `O(log n)` — на собесе этого часто достаточно, если проговорить trade-off.
- **O(1)-схема (классическая, O(1) LFU)**: два уровня списков.
  - `map[key]*node` — узел хранит значение и текущую частоту `freq`.
  - **Список частотных «корзин» (frequency list)**: каждая корзина соответствует значению `freq` и держит двусвязный список своих узлов в порядке давности (LRU **внутри** одной частоты — для разрыва ничьих).
  - `Get`/`Put` существующего ключа: `freq++`, узел переезжает из корзины `f` в корзину `f+1` (создаётся, если её нет). O(1).
  - Вытеснение: берём корзину с **минимальной** частотой (держим указатель `minFreq`), удаляем из неё самый старый узел. O(1).
  - При вставке нового ключа его `freq=1`, `minFreq=1`.
- **Тонкость с ничьей**: когда несколько элементов с одинаковой минимальной частотой — вытесняется **наименее недавний** из них (внутри корзины порядок LRU). Это и есть причина двух уровней.
- **Потокобезопасность — как у LRU**: `Get`/`touch` тоже мутируют структуру (переезд узла между корзинами, сдвиг `minFreq`), поэтому нужен эксклюзивный `Mutex` на `Get` и `Put`, `RWMutex` не подходит. Под нагрузкой — шардирование.
- **Минусы LFU на практике**: «застаревание частот» — элемент, бывший популярным давно, держит высокий счётчик и не вытесняется, хотя уже не нужен. Лечится **decay** счётчиков по времени или гибридами (**LFU with aging**, **W-TinyLFU** в Caffeine/Ristretto — частотный фильтр на Count-Min Sketch поверх LRU-окна).

<details>
<summary>Решение O(1) LFU</summary>

```go
type node struct {
    key, value any
    freq       int
    prev, next *node
}

// dlist — двусвязный список узлов одной частоты (с sentinel'ами).
type dlist struct {
    head, tail *node
    size       int
}

func newList() *dlist {
    h, t := &node{}, &node{}
    h.next, t.prev = t, h
    return &dlist{head: h, tail: t}
}
func (l *dlist) pushFront(n *node) {
    n.prev, n.next = l.head, l.head.next
    l.head.next.prev, l.head.next = n, n
    l.size++
}
func (l *dlist) remove(n *node) {
    n.prev.next, n.next.prev = n.next, n.prev
    l.size--
}
func (l *dlist) back() *node { return l.tail.prev } // самый старый в корзине

type LFU struct {
    capacity int
    minFreq  int
    items    map[any]*node
    freqs    map[int]*dlist // freq -> список узлов этой частоты
}

func NewLFU(capacity int) *LFU {
    return &LFU{capacity: capacity, items: map[any]*node{}, freqs: map[int]*dlist{}}
}

// touch — повысить частоту узла на 1, переложив его в следующую корзину.
func (c *LFU) touch(n *node) {
    old := c.freqs[n.freq]
    old.remove(n)
    if old.size == 0 {
        delete(c.freqs, n.freq)
        if c.minFreq == n.freq {
            c.minFreq++
        }
    }
    n.freq++
    if c.freqs[n.freq] == nil {
        c.freqs[n.freq] = newList()
    }
    c.freqs[n.freq].pushFront(n)
}

func (c *LFU) Get(key any) (any, bool) {
    n, ok := c.items[key]
    if !ok {
        return nil, false
    }
    c.touch(n)
    return n.value, true
}

func (c *LFU) Put(key, value any) {
    if c.capacity <= 0 {
        return
    }
    if n, ok := c.items[key]; ok {
        n.value = value
        c.touch(n)
        return
    }
    if len(c.items) >= c.capacity {
        lst := c.freqs[c.minFreq]
        victim := lst.back() // наименее недавний среди наименее частых
        lst.remove(victim)
        delete(c.items, victim.key)
    }
    n := &node{key: key, value: value, freq: 1}
    c.items[key] = n
    if c.freqs[1] == nil {
        c.freqs[1] = newList()
    }
    c.freqs[1].pushFront(n)
    c.minFreq = 1 // новый элемент всегда сбрасывает минимум до 1
}
```

Ключевая идея: частота меняется на ±1 за раз, поэтому «переезд» узла — всегда в соседнюю корзину, а `minFreq` либо растёт на 1 при опустошении текущей корзины, либо сбрасывается в 1 при вставке. Это и держит все операции в O(1).
</details>

| | LRU | LFU |
|---|---|---|
| Критерий вытеснения | давность обращения (recency) | частота обращений (frequency) |
| Что выкидывает | самый давно используемый | самый редко используемый (ничья → самый давний) |
| Структура O(1) | хэш + 1 двусвязный список | хэш + список частотных корзин (списки внутри) |
| Слабое место | «скан» из новых ключей вымывает горячие | застаревание частот, нужен decay/aging |
| Где уместен | временна́я локальность (recent = вероятно нужно снова) | устойчивая популярность ключей |

Глубже: полный разбор LRU с тестами и thread-safe вариантом — [LRU cache (coding-task)](coding-tasks/data-structures/01-lru-cache.md).

---

## 11 · Что с закрытым каналом

> Что произойдёт в каждом случае? Если паника — какая именно?

- **(а)** `close(ch); close(ch)` → **паника** `panic: close of closed channel`. Повторное закрытие запрещено.
- **(б)** `close(ch); v, ok := <-ch` → **без паники**: `v` — нулевое значение типа, `ok == false`. Чтение из закрытого канала всегда успешно отдаёт zero value и `ok=false` (так и работает `for range` по каналу — выходит при закрытии).
- **(в)** `close(ch); ch <- 1` → **паника** `panic: send on closed channel`. Отправка в закрытый канал запрещена.
- **(г)** `var ch chan int; <-ch` (nil-канал) → **не паника, а вечная блокировка**. Операции с nil-каналом (и send, и receive) блокируются навсегда. Если это единственная горутина — `fatal error: all goroutines are asleep - deadlock!`.

Сводка правил:

| Операция | Закрытый канал | Nil-канал |
|---|---|---|
| Receive `<-ch` | zero value, `ok=false` | блок навсегда |
| Send `ch<-x` | паника | блок навсегда |
| Close | паника | паника |

Практический трюк: присваивание каналу `nil` в `select` **отключает** его case (он никогда не сработает) — используют, чтобы исключить уже завершённый источник из `select`.

Глубже: [Goroutines и channels (закрытие, nil-каналы, select)](../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md).

---

## 12 · Ревью кода кеша

> Кеш под высокой нагрузкой, чтение/запись = 80/20. Какие проблемы видишь?

```go
var cache = make(map[string]string)

func GetOrCreate(key, value string) string {
    var m sync.Mutex   // (!) локальный мьютекс
    m.Lock()
    value = cache[key]
    m.Unlock()
    if value != "" {
        return value
    }
    m.Lock()
    cache[key] = value
    m.Unlock()
    return value
}
```

Проблемы:

- **Мьютекс локальный (`var m sync.Mutex` внутри функции)** — у каждого вызова свой замок. Он **ничего не синхронизирует**: разные горутины блокируют *разные* мьютексы. Это фатальный баг — фактически защиты нет.
- **Гонка данных на `map`** — следствие предыдущего: конкурентные чтение и запись `map` без реальной синхронизации → `fatal error: concurrent map read and map write` (рантайм специально это детектит) или порча структуры.
- **Логическая ошибка `GetOrCreate`** — после «промаха» в кеш кладётся `value`, который к этому моменту уже перезаписан результатом `cache[key]` (пустой строкой). То есть всегда пишется `""`. Параметр и локальная переменная конфликтуют по имени.
- **`value != ""` как признак наличия** — невозможно закешировать легитимное пустое значение; нет различия «нет ключа» vs «значение пустое». Нужен `v, ok := cache[key]`.
- **Race window (TOCTOU)** — проверка «есть ли» и запись разнесены на две критические секции и не атомарны: между ними другая горутина уже могла вставить значение для того же ключа. Тогда «первый записавший побеждает» не соблюдается — второй вызов перезатрёт чужое значение. Нужна **одна** критическая секция (проверка + запись под одним замком) или повторная проверка под write-lock.
- **Под нагрузкой 80/20 один общий `Mutex` сериализует и чтения** — можно `sync.RWMutex` (читатели параллельно, запись — под `Lock` с повторной проверкой) или шардирование. Если бы значение не передавалось, а строилось дорогой функцией — тогда уместен `singleflight`, чтобы при промахе строить его один раз на ключ.

<details>
<summary>Корректная версия</summary>

Сигнатура та же, что в исходнике (`value` приходит аргументом) — чинятся только баги: общий мьютекс, одна критическая секция, `v, ok`.

```go
type Cache struct {
    mu    sync.Mutex
    items map[string]string
}

func New() *Cache {
    return &Cache{items: make(map[string]string)}
}

// GetOrCreate возвращает уже закешированное значение, иначе сохраняет
// переданное value и возвращает его. Первый записавший побеждает.
func (c *Cache) GetOrCreate(key, value string) string {
    c.mu.Lock()
    defer c.mu.Unlock()
    if v, ok := c.items[key]; ok { // проверка и запись в одной критической секции
        return v
    }
    c.items[key] = value
    return value
}
```

Под read-heavy нагрузкой (80/20) — `RWMutex` с double-checked locking, чтобы читатели шли параллельно:

```go
func (c *Cache) GetOrCreate(key, value string) string {
    c.mu.RLock()
    v, ok := c.items[key]
    c.mu.RUnlock()
    if ok {
        return v
    }
    c.mu.Lock()
    defer c.mu.Unlock()
    if v, ok := c.items[key]; ok { // повторная проверка: кто-то мог вставить между RUnlock и Lock
        return v
    }
    c.items[key] = value
    return value
}
```

Под очень высокой нагрузкой один замок (даже `RWMutex`) становится точкой контеншена — тогда **шардирование**: N независимых сегментов, ключ маршрутизируется по хэшу, каждый сегмент со своим мьютексом и map. Горутины, попавшие в разные шарды, не мешают друг другу — пропускная способность растёт ~линейно с числом шардов.

```go
const shardCount = 256 // степень двойки → дешёвый & вместо %

type shard struct {
    mu    sync.RWMutex
    items map[string]string
}

type ShardedCache struct {
    shards [shardCount]*shard
}

func NewSharded() *ShardedCache {
    c := &ShardedCache{}
    for i := range c.shards {
        c.shards[i] = &shard{items: make(map[string]string)}
    }
    return c
}

// shardFor — выбор сегмента по хэшу ключа (FNV-1a, быстрый и без аллокаций).
func (c *ShardedCache) shardFor(key string) *shard {
    h := fnv.New32a()
    _, _ = h.Write([]byte(key))
    return c.shards[h.Sum32()&(shardCount-1)] // & вместо % т.к. shardCount — степень двойки
}

func (c *ShardedCache) GetOrCreate(key, value string) string {
    s := c.shardFor(key)

    s.mu.RLock()
    v, ok := s.items[key]
    s.mu.RUnlock()
    if ok {
        return v
    }

    s.mu.Lock()
    defer s.mu.Unlock()
    if v, ok := s.items[key]; ok { // повторная проверка под write-lock
        return v
    }
    s.items[key] = value
    return value
}
```

Нюансы шардирования, которые стоит проговорить:

- **Число шардов** — степень двойки, тогда выбор сегмента — дешёвый `hash & (N-1)` вместо `% N`. Ориентир: ≥ числа ядер (часто 256), чтобы хватало параллелизма.
- **Хэш должен распределять равномерно** — перекос (hot key или плохой хэш) сводит выгоду на нет: весь трафик идёт в один шард. FNV/maphash подходят; `len(key)` — нет.
- **Capacity и вытеснение становятся пошардовыми** — если поверх навесить LRU/LFU с лимитом, лимит делится между шардами; глобальное «ровно N элементов» уже не гарантируется.
- **Готовая альтернатива** — не изобретать: `sync.Map` (read-mostly) или библиотеки вроде `dgraph-io/ristretto`/`puzpuzpuz/xsync`, где шардирование уже внутри.
</details>

Глубже: [Sync-примитивы (Mutex/RWMutex/singleflight)](../01-go-core/concurrency-and-performance/03-sync-primitives.md), [sync.Map](../01-go-core/map-internals/sync-map/README.md), [Fetcher with cache (code-review)](coding-tasks/code-review/01-fetcher-with-cache.md).

---

## 13 · Broadcast канал

> Один писатель пушит значения, N подписчиков получают каждое. Подписчик может отписаться в любой момент. Писатель не должен блокироваться на медленном подписчике. После `Close()` все каналы подписчиков закрываются.

- **Хранилище подписчиков — `map[<-chan T]chan T`** под мьютексом; `Subscribe` создаёт канал, кладёт в map и отдаёт его receive-only «вид», `Unsubscribe` удаляет и закрывает. Ключ — receive-only `<-chan T` (его же отдают в `Unsubscribe`) → отписка O(1); двунаправленный `chan T` лежит в значении, т.к. `close()`/send по receive-only каналу запрещены.
- **Не блокироваться на медленном подписчике** — каналы подписчиков буферизованные, а в `Send` отправка через `select` с `default`: если у подписчика буфер забит, сообщение для него **дропается** (или копится в его очереди — по политике), но писатель идёт дальше. Это ключевое требование: один тормоз не стопорит всех.
- **`Close()`** — закрыть все каналы подписчиков и пометить broadcast закрытым; повторные `Send` после `Close` — no-op или паника по контракту.
- Аккуратно с гонкой «`Unsubscribe` во время `Send`»: всё под одним мьютексом, либо канал закрывает строго владелец (сам broadcast), а подписчик только сигналит об отписке.

<details>
<summary>Решение</summary>

```go
type Broadcast[T any] struct {
    mu     sync.Mutex
    // Ключ — receive-only канал (его отдаём подписчику и его же он
    // передаёт в Unsubscribe) → поиск O(1).
    // Значение — двунаправленный канал: нужен для send и close,
    // т.к. по <-chan T закрыть/отправить нельзя.
    subs   map[<-chan T]chan T
    closed bool
}

func New[T any]() *Broadcast[T] {
    return &Broadcast[T]{subs: make(map[<-chan T]chan T)}
}

func (b *Broadcast[T]) Subscribe() <-chan T {
    b.mu.Lock()
    defer b.mu.Unlock()
    ch := make(chan T, 16) // буфер сглаживает кратковременные всплески
    var recv <-chan T = ch // тот же канал, но receive-only — отдаём наружу
    if b.closed {
        close(ch)
    } else {
        b.subs[recv] = ch
    }
    return recv
}

func (b *Broadcast[T]) Unsubscribe(ch <-chan T) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if c, ok := b.subs[ch]; ok { // O(1): прямой поиск по ключу
        delete(b.subs, ch)
        close(c) // закрываем по двунаправленному
    }
}

func (b *Broadcast[T]) Send(v T) {
    b.mu.Lock()
    defer b.mu.Unlock()
    for _, ch := range b.subs {
        select {
        case ch <- v: // успели — хорошо
        default:      // медленный подписчик — дропаем, не блокируемся
        }
    }
}

func (b *Broadcast[T]) Close() {
    b.mu.Lock()
    defer b.mu.Unlock()
    if b.closed {
        return
    }
    b.closed = true
    for recv, ch := range b.subs {
        close(ch)
        delete(b.subs, recv)
    }
}
```

Почему map ключуется по `<-chan T`, а двунаправленный канал лежит в значении: `Subscribe` отдаёт наружу receive-only `<-chan T` (чтобы подписчик не мог писать/закрывать), и именно его получает `Unsubscribe` — значит он и должен быть ключом, тогда отписка O(1). Но `close()` и отправка по receive-only каналу запрещены, поэтому сам `chan T` храним как значение. Если бы ключом был `chan T` (как в наивном варианте), пришлось бы линейно перебирать map и сравнивать с приведением типа — O(n).

Политика «дропать при переполнении» — выбор; альтернативы: блокироваться (нарушает условие), отписывать медленного, расширять буфер. На собесе важно проговорить компромисс.
</details>

Глубже: [Pub/Sub (coding-task)](coding-tasks/concurrency/05-pubsub.md), [Redis Pub/Sub](../07-message-brokers-and-streaming/05-redis-pubsub.md).

---

## 14 · Mutex vs RWMutex

> Когда `sync.Mutex`, когда `sync.RWMutex`? Когда RWMutex медленнее обычного Mutex?

- **`Mutex`** — по умолчанию. Простой, дешёвый, когда критическая секция короткая или запись/чтение примерно поровну.
- **`RWMutex`** — когда **read-mostly** нагрузка (много `RLock`, мало `Lock`) и критическая секция чтения **достаточно длинная**, чтобы выигрыш от параллельных читателей перекрыл накладные расходы. Несколько читателей под `RLock` работают одновременно.
- **RWMutex медленнее Mutex, когда:**
  - **Критическая секция очень короткая** (читается одно поле). `RWMutex` внутри дороже: больше атомиков и состояния (счётчик читателей, обработка ожидающего писателя), и эти накладные расходы превышают экономию.
  - **Высокий contention на запись** — писатель ждёт всех читателей, а новые `RLock` блокируются, пока ждёт писатель (защита от writer starvation). Пропускная способность проседает.
  - **Много ядер и частые `RLock`** — счётчик читателей становится точкой контеншена по кэш-линии (cache-line bouncing между ядрами); иногда обычный `Mutex` или шардирование быстрее.
- Практика: не угадывать, а **мерить** (`go test -bench`, `-race`, профиль contention `mutex`/`block`). Часто для коротких секций `Mutex` или `atomic`/шардирование обгоняют `RWMutex`.

Глубже: [Sync-примитивы (Mutex vs RWMutex, бенчмарки, semtable)](../01-go-core/concurrency-and-performance/03-sync-primitives.md).

---

## 15 · Semaphore через канал

> Семафор на N одновременных операций через буферизованный канал. `Acquire(ctx)` блокирует до слота или возвращает `ctx.Err()`. `Release` отдаёт слот. Что будет, если `Release` без парного `Acquire`?

- **Буферизованный канал ёмкости N как набор «слотов».** `Acquire` = отправить в канал (занять слот), `Release` = прочитать из канала (освободить). Когда буфер полон — `Acquire` блокируется, пока кто-то не сделает `Release`.
- **Отмена**: `Acquire` делает `select` между «слот занят» (`ch <- struct{}{}`) и `ctx.Done()` → при отмене возвращает `ctx.Err()`, не заняв слот.
- **`Release` без парного `Acquire`**: при такой реализации (`Release` читает из канала) лишний `Release` на **пустом** канале **заблокируется** навсегда (нечего читать) — баг. Если бы делали наоборот (буфер предзаполнен, `Acquire`=receive, `Release`=send), то лишний `Release` запаниковал бы/переполнил счётчик. В любом случае непарный `Release` — ошибка использования: он либо виснет, либо повышает реальный лимit параллельности выше N. Защита — паниковать/возвращать ошибку при попытке вернуть слот, который не занимали.

<details>
<summary>Решение</summary>

```go
type Semaphore struct {
    slots chan struct{}
}

func New(n int) *Semaphore {
    return &Semaphore{slots: make(chan struct{}, n)}
}

func (s *Semaphore) Acquire(ctx context.Context) error {
    select {
    case s.slots <- struct{}{}: // занял слот
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Semaphore) Release() {
    select {
    case <-s.slots: // освободил слот
    default:
        // непарный Release — программная ошибка; лучше упасть явно
        panic("semaphore: Release without Acquire")
    }
}
```

`default` в `Release` превращает «тихий вечный блок» в явную панику — непарный вызов виден сразу.
</details>

Глубже: [Sync-примитивы](../01-go-core/concurrency-and-performance/03-sync-primitives.md), [Rate limiter (coding-task)](coding-tasks/concurrency/02-rate-limiter.md), `golang.org/x/sync/semaphore` (weighted).

---

## 16 · context.WithValue

> Когда `context.WithValue` валиден, а когда антипаттерн? Почему нельзя использовать его для DI?

**Валидно** — для **request-scoped метаданных**, которые сквозняком проходят через слои и не влияют на бизнес-логику напрямую:

- `request_id` / `trace_id` / `correlation_id` для логов и трейсинга.
- Аутентификация: ID пользователя, права (после middleware).
- Дедлайны/таймауты — но это `WithCancel`/`WithDeadline`, не `WithValue`.

**Антипаттерн** — когда через контекст протаскивают то, что должно быть явной зависимостью или параметром:

- **Обязательные параметры функции** — их место в сигнатуре, а не в `ctx` (иначе компилятор не проверит, что они переданы).
- **Опциональная конфигурация** — через структуры/options.
- **Mutable state** — контекст иммутабелен по дизайну; класть туда изменяемое — путь к гонкам и сюрпризам.

**Почему `WithValue` нельзя для DI:**

- **Нет типобезопасности** — `ctx.Value(key)` возвращает `any`; зависимость не видна в сигнатуре, ошибки всплывают в рантайме (`nil`/паника при type assertion), а не на компиляции.
- **Скрытые зависимости** — функция «тайно» требует что-то из контекста; невозможно понять контракт по сигнатуре, тяжело тестировать и мокать.
- **Линейный поиск по цепочке** — `Value` идёт вверх по родителям; не для горячих путей и не для постоянных зависимостей.
- **Жизненный цикл не тот** — контекст живёт на время запроса; зависимости (репозитории, клиенты, логгер) живут на время приложения. DI делается через конструкторы/структуры/wire, а не через request-scoped контейнер.

Правило: «context передаёт *что про этот запрос*, а не *что умеет сервис*». Зависимости — явными полями/аргументами.

Глубже: [Context patterns (WithValue, антипаттерны, ключи)](../01-go-core/concurrency-and-performance/04-context-patterns.md), [Dependencies и composition (DI в Go)](../04-architecture-and-patterns/patterns/go-code-patterns/01-dependencies-and-composition.md).

---

## 17 · Найди goroutine leak

> Найди утечку горутины и предложи фикс.

```go
func process(ctx context.Context, jobs []Job) ([]Result, error) {
    results := make(chan Result) // небуферизованный!

    for _, job := range jobs {
        go func(j Job) { results <- doWork(j) }(job)
    }

    out := make([]Result, 0, len(jobs))
    for i := 0; i < len(jobs); i++ {
        select {
        case r := <-results:
            out = append(out, r)
        case <-ctx.Done():
            return nil, ctx.Err() // ранний выход!
        }
    }
    return out, nil
}
```

- **Утечка**: канал `results` небуферизованный. При `ctx.Done()` функция возвращается раньше, чем прочитаны все результаты. Оставшиеся горутины навсегда виснут на `results <- doWork(j)` — получателя больше нет. Горутины (и их память/ресурсы) утекают на всё время жизни процесса.
Тут три **независимые** проблемы, которые легко смешать:

- **(A) Send виснет навсегда** — это и есть leak в задаче. После раннего `return` по `ctx` читателя нет, а отправители на небуферизованном канале блокируются на `results <- ...` вечно. Лечится одним из двух **эквивалентных по эффекту** способов: буферизованный канал **или** `select` на отправке. Оба гарантируют, что горутина не зависнет на send.
- **(B) Сам `doWork` виснет, игнорируя `ctx`** — к исходной задаче не относится (`doWork` предполагается завершимым), но это частая путаница: **никакой `select`/буфер снаружи не прерывает `doWork`**. Аргумент send вычисляется до `select`, так что горутина блокируется внутри `doWork`, и `ctx.Done()` в обёртке тут бессилен. Отменяемость долгой работы — **только** внутри `doWork` (проверять `ctx.Done()`, прокидывать `ctx` в сетевые вызовы). `errgroup`/`gctx` это не меняет — `doWork` всё равно должен сам слушать контекст.
- **(C) `doWork` паникует → результат не отправлен → цикл недосчитается.** Тут две развилки. **Без `recover`** незахваченная паника в горутине крашит **весь процесс** (это не зависание — рантайм валит все горутины). **С `recover`, но без отправки** — горутина выживает, но в `results` ничего не пишет; цикл ждёт ровно `len(jobs)` значений, получает меньше и **виснет навсегда** (без дедлайна у `ctx` — `all goroutines are asleep — deadlock`). Фикс: `recover`, который **всё равно шлёт `Result` с ошибкой**, чтобы инвариант «ровно один send на горутину» соблюдался всегда.

Для (A) дефолт — **буферизованный канал**: проще всего и работает независимо от поведения `doWork`.

<details>
<summary>Фикс (A): буферизованный канал</summary>

```go
func process(ctx context.Context, jobs []Job) ([]Result, error) {
    results := make(chan Result, len(jobs)) // буфер = числу задач → send никогда не блокируется
    for _, job := range jobs {
        go func(j Job) { results <- doWork(j) }(job)
    }
    out := make([]Result, 0, len(jobs))
    for i := 0; i < len(jobs); i++ {
        select {
        case r := <-results:
            out = append(out, r)
        case <-ctx.Done():
            return nil, ctx.Err() // оставшиеся горутины допишут в буфер и завершатся, не зависнув
        }
    }
    return out, nil
}
```

Эквивалентная альтернатива — оставить канал небуферизованным, но обернуть **отправку** в `select`:

```go
go func(j Job) {
    r := doWork(j)
    select {
    case results <- r: // читатель забрал
    case <-ctx.Done():  // читатель ушёл — не виснем на send, просто выходим
    }
}(job)
```

Обе версии решают ровно (A) — «send не виснет». На `doWork` (проблему (B)) ни одна не влияет: чтобы прервать долгую работу, `doWork` сам должен уважать `ctx`.
</details>

<details>
<summary>Фикс (C): гарантированная доставка результата при панике</summary>

`recover` в горутине обязан **всё равно отправить `Result`** (с ошибкой), иначе цикл недосчитается и зависнет:

```go
type Result struct {
    Value any
    Err   error
}

func process(ctx context.Context, jobs []Job) ([]Result, error) {
    results := make(chan Result, len(jobs))
    for _, job := range jobs {
        go func(j Job) {
            defer func() {
                if r := recover(); r != nil {
                    // паника — всё равно шлём результат, чтобы цикл добрал счётчик
                    results <- Result{Err: fmt.Errorf("panic in doWork: %v", r)}
                }
            }()
            results <- doWork(j) // если doWork паникует — сюда не дойдём, отправит defer
        }(job)
    }

    out := make([]Result, 0, len(jobs))
    for i := 0; i < len(jobs); i++ {
        select {
        case r := <-results:
            out = append(out, r)
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    return out, nil
}
```

Инвариант «**ровно один send на горутину**»: при нормальном завершении send делает `doWork`, а `recover()` возвращает `nil` (второго send нет); при панике основной send не выполняется — отправляет `defer`. В любом случае цикл получает ровно `len(jobs)` значений и не виснет. Буфер `len(jobs)` гарантирует, что и «аварийный» send не заблокируется. Без `recover` паника обрушила бы весь процесс.
</details>

Глубже: [Worker pool debug (классы goroutine leak)](coding-tasks/concurrency/07-worker-pool-debug.md), [Symptom-driven troubleshooting](../10-devops-and-observability/incident-response-and-investigation/02-symptom-driven-troubleshooting.md), [Goroutine/concurrency profiling](../01-go-core/profiling/04-goroutine-concurrency-profiling.md).

---

## 18 · Rate limiter

> Token bucket. `Allow()` — non-blocking. `Wait(ctx)` — блокирует до токена с отменой. `burst` — макс. накопленных токенов. `x/time/rate` нельзя — пиши с нуля.

- **Token bucket**: токены капают со скоростью `rps`, копятся до `burst`. Каждый запрос забирает 1 токен; нет токена — `Allow()=false` / `Wait` ждёт.
- **Реализация без таймера на каждый токен** — ленивый пересчёт: при каждом запросе считаем, сколько токенов «накапало» с прошлого обращения (`elapsed * rps`), но не больше `burst`. Это точнее и дешевле, чем тикер.
- `Allow()`: пересчитать токены, если `>=1` — забрать, вернуть `true`, иначе `false`. Под мьютексом.
- `Wait(ctx)`: если токена нет — вычислить время до следующего токена, `select` между `time.After(delay)` и `ctx.Done()`.
- `burst` ограничивает «всплеск»: при простое можно накопить максимум `burst` токенов, не больше — иначе после долгой паузы прорвало бы лимит.

<details>
<summary>Решение</summary>

```go
type Limiter struct {
    mu       sync.Mutex
    rps      float64
    burst    float64
    tokens   float64
    lastSeen time.Time
}

func New(rps int, burst int) *Limiter {
    return &Limiter{
        rps:      float64(rps),
        burst:    float64(burst),
        tokens:   float64(burst), // стартуем «полными»
        lastSeen: time.Now(),
    }
}

// refill — ленивое пополнение: учитываем прошедшее время. Вызывать под mu.
func (l *Limiter) refill(now time.Time) {
    elapsed := now.Sub(l.lastSeen).Seconds()
    l.tokens = math.Min(l.burst, l.tokens+elapsed*l.rps)
    l.lastSeen = now
}

func (l *Limiter) Allow() bool {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.refill(time.Now())
    if l.tokens >= 1 {
        l.tokens--
        return true
    }
    return false
}

func (l *Limiter) Wait(ctx context.Context) error {
    for {
        l.mu.Lock()
        l.refill(time.Now())
        if l.tokens >= 1 {
            l.tokens--
            l.mu.Unlock()
            return nil
        }
        // сколько ждать до 1 токена
        deficit := 1 - l.tokens
        wait := time.Duration(deficit / l.rps * float64(time.Second))
        l.mu.Unlock()

        timer := time.NewTimer(wait)
        select {
        case <-timer.C: // повторим попытку
        case <-ctx.Done():
            timer.Stop()
            return ctx.Err()
        }
    }
}
```

Ленивый пересчёт точнее и не держит фоновых горутин. Альтернатива — «канал + тикер» — ниже.
</details>

<details>
<summary>Альтернатива: канал-бакет + тикер</summary>

Идея: токены — это места в буферизованном канале (`burst`), а фоновая горутина-тикер периодически «доливает» по одному токену с частотой `rps`. `Allow` — неблокирующее чтение из канала, `Wait` — чтение с `select` и `ctx`.

```go
type Limiter struct {
    tokens chan struct{}
    stop   chan struct{}
}

func New(rps int, burst int) *Limiter {
    l := &Limiter{
        tokens: make(chan struct{}, burst),
        stop:   make(chan struct{}),
    }
    for i := 0; i < burst; i++ { // стартуем «полными»
        l.tokens <- struct{}{}
    }
    go l.refill(rps)
    return l
}

func (l *Limiter) refill(rps int) {
    ticker := time.NewTicker(time.Second / time.Duration(rps))
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            select {
            case l.tokens <- struct{}{}: // долили токен
            default:                     // бакет полон (burst достигнут) — пропускаем
            }
        case <-l.stop:
            return
        }
    }
}

func (l *Limiter) Allow() bool {
    select {
    case <-l.tokens:
        return true
    default:
        return false
    }
}

func (l *Limiter) Wait(ctx context.Context) error {
    select {
    case <-l.tokens:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (l *Limiter) Close() { close(l.stop) } // остановить горутину-тикер
```

Плюсы: код проще, `Allow`/`Wait` тривиальны (одно чтение из канала), backpressure «бесплатная». Минусы:

- **Точность ограничена гранулярностью тикера** — при высоком `rps` интервал `time.Second/rps` становится крошечным, тикер не успевает/дрожит (на практике точность таймеров ~1–10 мс). Ленивый пересчёт от `time.Now()` такой проблемы не имеет.
- **Нужен явный `Close()`** — иначе горутина-тикер живёт вечно (leak). У версии с ленивым пересчётом фоновых горутин нет вовсе.
- **При большом `rps` тикер «крутится» вхолостую**, даже когда лимитер не используется.

Поэтому для production обычно берут ленивый пересчёт (как в `golang.org/x/time/rate`), а «канал + тикер» хорош, когда `rps` невелик и важна простота.
</details>

Глубже: [Rate limiter (coding-task, варианты)](coding-tasks/concurrency/02-rate-limiter.md), [Rate limiting (протоколы)](../08-networking-and-api/protocols/15-rate-limiting.md), [Rate limiting (reliability)](../05-system-design/reliability-patterns/04-rate-limiting.md), [Redis rate limiters](../06-databases/database-systems-catalog/08b-redis-rate-limiters.md).

---

## 19 · Errgroup vs Worker pool

> Когда `errgroup.Group`, когда worker pool с фиксированным числом воркеров? Почему для I/O-bound задач worker pool часто избыточен?

- **`errgroup`** — когда число задач **известно и ограничено**, нужна координация «первая ошибка отменяет остальных» и сбор результатов. По горутине на задачу. Идеален для «сходить в 5 сервисов параллельно», fan-out фиксированного набора. С Go 1.20+ у `errgroup` есть `SetLimit` — можно ограничить параллелизм, фактически получив worker pool «бесплатно».
- **Worker pool (фиксированное N воркеров)** — когда задач **много или это поток** (тысячи/неограниченно), и важно **ограничить параллелизм/ресурсы**: не плодить миллион горутин, держать стабильное число коннектов к БД, ровную нагрузку. Воркеры переиспользуются, читая задачи из канала.
- Грубое правило: *конечный известный набор + координация ошибок* → `errgroup`; *поток/большой объём + контроль ресурсов и backpressure* → worker pool (или `errgroup.SetLimit`).

**Почему для I/O-bound worker pool часто избыточен:**

- Горутины **дёшевы** (~2–8 КБ стартового стека) и при I/O-блокировке **не занимают поток ОС**: на блокирующем syscall/сетевом ожидании рантайм через netpoller паркует горутину и освобождает `M` под другие. Поэтому 10 000 горутин, ждущих сеть, почти не стоят CPU.
- Пул воркеров нужен прежде всего чтобы **не превысить лимит внешнего ресурса** (коннекты к БД, квоты API, файловые дескрипторы) — но это ограничение проще выразить **семафором** (`chan struct{}`/`errgroup.SetLimit`), чем полноценным пулом.
- Для **CPU-bound** задач, наоборот, пул на `GOMAXPROCS` воркеров оправдан: больше параллелизма, чем ядер, лишь добавляет переключений.

Итог: для I/O чаще достаточно «горутина на задачу + семафор на лимит», а тяжёлый worker pool — когда нужен переиспользуемый долгоживущий конвейер или строгий контроль числа исполнителей.

Глубже: [Worker pool (coding-task)](coding-tasks/concurrency/01-worker-pool.md), [Worker pool debug](coding-tasks/concurrency/07-worker-pool-debug.md), [Background workers](../04-architecture-and-patterns/patterns/04-background-workers.md), [Netpoller](../01-go-core/runtime-scheduler/03-netpoller.md).

---

## 20 · Nil interface

> Почему выводит `false`, хотя кажется, что должен `true`?

```go
type MyError struct{}
func (e *MyError) Error() string { return "boom" }

func doWork() error {
    var err *MyError = nil
    return err
}
func main() {
    err := doWork()
    fmt.Println(err == nil) // false — почему?
}
```

- **Интерфейс в Go — пара `(тип, значение)`**: `(itab, data)`. Интерфейс равен `nil`, только если **обе** части нулевые (тип не задан И значение `nil`).
- `doWork` возвращает `*MyError(nil)`: значение-указатель нулевое, **но тип задан** — `*MyError`. При присваивании в `error` интерфейс получает пару `(*MyError, nil)`. Тип непустой → интерфейс **не равен** `nil` → `false`. Это «typed nil».
- Классическая ловушка: функция вернула «нулевой» конкретный указатель как `error`, и `if err != nil` срабатывает, хотя ошибки фактически нет.

Как защититься:

- **Возвращать `nil` буквально** при отсутствии ошибки, а не типизированный `nil`-указатель:
  ```go
  func doWork() error {
      if somethingFailed {
          return &MyError{}
      }
      return nil // именно nil, не var err *MyError
  }
  ```
- **Не объявлять** промежуточную `var err *MyError` и не возвращать её в ветке «успех».
- В обёртках над ошибками проверять конкретный указатель **до** возврата как `error`.
- Ловить такое: `go vet` (есть проверки на nil-interface-сравнения в ряде линтеров), `errcheck`/`nilness` в `golangci-lint`; в тестах сравнивать через `errors.Is`/конкретный тип.

<details>
<summary>Демонстрация механики</summary>

```go
var p *MyError = nil
var err error = p

fmt.Println(p == nil)   // true  — голый указатель nil
fmt.Println(err == nil) // false — интерфейс (тип=*MyError, значение=nil)

// Под капотом: err = (itab для *MyError, data=nil)
// nil-интерфейс — это (nil, nil), а тут тип не nil → не равно.
```
</details>

Глубже: [Interfaces, method sets и nil (typed nil, itab)](../01-go-core/03-interfaces-method-sets-and-nil.md).

---

## Что есть рядом

- [Interview Practice — обзор](README.md)
- [Concurrency coding-tasks](coding-tasks/concurrency/README.md) — worker pool, rate limiter, fan-in/out, pipeline, pub/sub, singleflight
- [Concurrency and Performance (теория)](../01-go-core/concurrency-and-performance/README.md) — memory model, channels, sync, context
- [Runtime scheduler](../01-go-core/runtime-scheduler/README.md) — G-M-P, syscalls, netpoller, таймеры
- [System Design Interview Cases](../05-system-design/interview-cases/README.md)
