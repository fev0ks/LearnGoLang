# Задача 1: Fetcher с кэшем

## Содержание

- [Формулировка](#формулировка)
- [Изначальный код](#изначальный-код)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Разбор проблем](#разбор-проблем)
- [Решение уровня 1](#решение-уровня-1)
- [Reference implementation](#reference-implementation)
- [Trade-offs reference implementation](#trade-offs-reference-implementation)
- [Тесты](#тесты)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет разбор конечного конкурентного pipeline: один вызов получает
список идентификаторов, ограниченно параллельно загружает данные, использует кэш
и завершает выходной поток. Основная сложность находится не в worker pool, а в
контракте отмены, владении каналами и защите общей работы по одинаковому ключу.

---

## Формулировка

Нужно провести code review следующего кода. Ожидаемый API:

- обрабатывает идентификаторы с ограниченным параллелизмом;
- использует кэш между вызовами `FetchAll`;
- позволяет отменить batch через `context.Context`;
- завершает выходной канал после остановки всех worker’ов.

Сначала следует проверить компиляцию и runtime-поведение, затем восстановить
неявные части контракта: порядок результатов, дубликаты и ошибки.

---

## Изначальный код

Код компилируется, но при запуске заканчивается deadlock. Некоторые следующие
проблемы станут достижимы только после исправления `nil`-каналов — это нормальная
ситуация для bug hunt, где дефекты раскрываются слоями.

```go
package main

import (
    "context"
    "fmt"
    "time"
)

type Result struct {
    ID   int
    Data string
}

type Fetcher struct {
    cache map[int]Result
}

func main() {
    ctx, cancel := context.WithTimeout(
        context.Background(),
        75*time.Millisecond,
    )
    defer cancel()

    fetcher := NewFetcher()
    ids := []int{1, 2, 3, 2, 4, 1, 5, 6, 7, 8, 9}

    start := time.Now()
    defer fmt.Println("duration", time.Since(start))

    for result := range fetcher.FetchAll(ctx, ids) {
        fmt.Println(result)
    }
}

func NewFetcher() *Fetcher {
    return &Fetcher{}
}

func (f *Fetcher) doRequest(id int) Result {
    time.Sleep(50 * time.Millisecond)
    return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}
}

func (f *Fetcher) FetchAll(ctx context.Context, ids []int) <-chan Result {
    var out chan Result
    var jobs chan int

    go func() {
        defer close(jobs)
        for _, id := range ids {
            jobs <- id
        }
    }()

    for workerID := 0; workerID < 4; workerID++ {
        go func() {
            for id := range jobs {
                result, ok := f.cache[id]
                if ok {
                    out <- result
                    continue
                }

                result = f.doRequest(id)
                f.cache[id] = result
                out <- result
            }
        }()
    }

    return out
}
```

Отправка в `nil`-канал не вызывает `panic: send on nil channel`. Она блокируется
навсегда. В этом примере producer блокируется на первой отправке, worker’ы — на
чтении `jobs`, а `main` — на чтении `out`. Когда runtime не находит возможного
прогресса, процесс завершается сообщением о deadlock.

---

## Уточняющие вопросы

1. Должен ли каждый входной элемент породить отдельный результат, включая
   дубликаты?
2. Нужно ли сохранять порядок `ids` или достаточно вернуть результаты по мере
   готовности?
3. Возвращается ли ошибка отдельно для каждого ID или первая ошибка отменяет
   весь batch?
4. Что означает отмена: перестать запускать новые запросы, отменить in-flight
   запросы или сделать и то и другое?
5. Кэш принадлежит одному `Fetcher` и живёт вместе с ним или разделяется между
   процессами?
6. Как ограничиваются размер кэша, TTL и длительность обращения к источнику?

В решениях ниже каждый элемент `ids` считается отдельной работой. Результаты
приходят без гарантии порядка, но содержат исходный индекс. Ошибка одного ID не
отменяет остальные, а отмена контекста прекращает весь batch.

---

## Разбор проблем

### 1. Оба канала равны `nil`

`var jobs chan int` и `var out chan Result` создают нулевые значения каналов.
Любая отправка или получение вне `select` блокируется навсегда.

```go
jobs := make(chan int)
out := make(chan Result)
```

Буфер не является обязательным исправлением. Небуферизованный `out` создаёт
естественную backpressure: скорость worker’ов ограничивается скоростью reader’а.

### 2. Кэш не инициализирован

Чтение из `nil`-map допустимо и выглядит как cache miss. Запись запрещена и
вызывает `panic: assignment to entry in nil map`.

```go
return &Fetcher{cache: make(map[int]Result)}
```

### 3. Конкурентный доступ к `map`

После инициализации несколько worker’ов читают и изменяют одну map без
синхронизации. Это data race; runtime также может завершить процесс из-за
конкурентного чтения и записи map.

`sync.RWMutex` защищает целостность map, но не предотвращает cache stampede:
два worker’а всё ещё могут одновременно увидеть miss, отпустить lock и начать
одинаковые запросы.

### 4. Выходной канал никто не закрывает

`main` использует `range`, который завершается только после `close(out)`. Канал
должна закрывать отдельная goroutine после `WaitGroup.Wait`; worker не может
безопасно решить, был ли он последним.

### 5. Контекст передан, но полностью игнорируется

Параметры функции могут не использоваться, поэтому код компилируется. Однако
timeout не влияет ни на producer, ни на `doRequest`, ни на отправку результата.
Каждая потенциально блокирующая операция должна либо учитывать контекст, либо
иметь отдельно описанную верхнюю границу ожидания.

### 6. Producer может утечь

Если worker’ы завершились, producer способен навсегда остаться на `jobs <- id`.
Отправка должна выбирать между очередью и `ctx.Done()`.

### 7. Worker может утечь на результате

Если consumer перестал читать и не отменил контекст, worker блокируется на
`out <- result`. API должен потребовать одно из двух действий: прочитать поток
до закрытия или отменить переданный контекст.

### 8. Длительность измеряется в момент регистрации `defer`

Аргументы отложенного вызова вычисляются сразу:

```go
defer fmt.Println("duration", time.Since(start))
```

Поэтому будет напечатана длительность запуска, а не всей операции. Нужна
замыкающая функция:

```go
defer func() {
    fmt.Println("duration", time.Since(start))
}()
```

### 9. Дубликаты создают cache stampede

Mutex защищает структуру данных, но последовательность «проверить → загрузить →
записать» остаётся неатомарной. Для одинакового ID нужна дедупликация in-flight
работы, например `singleflight.Group`.

### 10. Не определены порядок и дубликаты

Worker’ы публикуют результаты по мере готовности. Если caller ожидает исходный
порядок, результат должен содержать индекс либо собираться в заранее выделенный
срез. Дубликаты также нельзя молча удалять: это меняет число результатов.

### 11. Ошибке негде появиться в API

`doRequest` возвращает только `Result`, поэтому невозможно отличить пустой ответ
от сбоя. Два параллельных канала `results` и `errors` неудобны: их нужно читать
одновременно, а закрывать согласованно. Один поток `Outcome` сохраняет связь ID,
результата и ошибки.

### 12. Нет конфигурации и lifecycle кэша

Число worker’ов, timeout и параметры кэша зашиты в код. Неограниченная map растёт
вместе с количеством уникальных ID. Bounded LRU ограничивает память, TTL —
устаревание, но конкретная библиотека может запускать собственную cleanup-
goroutine; её lifecycle нужно учитывать при выборе реализации.

---

## Решение уровня 1

Вариант ниже компилируется, корректно завершает каналы и возвращает ошибки рядом
с соответствующим ID. Он намеренно не решает stampede и ограничение размера
кэша: это следующий слой дизайна.

```go
package fetcher

import (
    "context"
    "fmt"
    "sync"
    "time"
)

type Result struct {
    ID   int
    Data string
}

type Outcome struct {
    Index  int
    ID     int
    Result Result
    Err    error
}

type job struct {
    index int
    id    int
}

type Fetcher struct {
    mu    sync.RWMutex
    cache map[int]Result
}

func NewFetcher() *Fetcher {
    return &Fetcher{cache: make(map[int]Result)}
}

func (f *Fetcher) doRequest(
    ctx context.Context,
    id int,
) (Result, error) {
    timer := time.NewTimer(50 * time.Millisecond)
    defer timer.Stop()

    select {
    case <-timer.C:
        return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()
    }
}

func (f *Fetcher) get(ctx context.Context, id int) (Result, error) {
    f.mu.RLock()
    result, ok := f.cache[id]
    f.mu.RUnlock()
    if ok {
        return result, nil
    }

    result, err := f.doRequest(ctx, id)
    if err != nil {
        return Result{}, err
    }

    f.mu.Lock()
    f.cache[id] = result
    f.mu.Unlock()
    return result, nil
}

func (f *Fetcher) FetchAll(
    ctx context.Context,
    ids []int,
    workers int,
) <-chan Outcome {
    out := make(chan Outcome)
    jobs := make(chan job)

    if workers <= 0 {
        close(out)
        return out
    }

    go func() {
        defer close(jobs)
        for index, id := range ids {
            select {
            case jobs <- job{index: index, id: id}:
            case <-ctx.Done():
                return
            }
        }
    }()

    var workerWG sync.WaitGroup
    workerWG.Add(workers)
    for workerID := 0; workerID < workers; workerID++ {
        go func() {
            defer workerWG.Done()
            for current := range jobs {
                result, err := f.get(ctx, current.id)
                outcome := Outcome{
                    Index:  current.index,
                    ID:     current.id,
                    Result: result,
                    Err:    err,
                }

                select {
                case out <- outcome:
                case <-ctx.Done():
                    return
                }
            }
        }()
    }

    go func() {
        workerWG.Wait()
        close(out)
    }()

    return out
}
```

При отмене число outcome может быть меньше `len(ids)`: producer перестаёт
выдавать новые jobs, а worker’ы не обязаны публиковать результат после закрытия
контекста. Этот контракт нужно документировать рядом с API.

---

## Reference implementation

Следующий вариант добавляет ограниченный TTL-кэш, валидацию конфигурации и
`singleflight`. Это reference implementation для одного процесса, а не готовая
универсальная библиотека.

`Get` разделяет контекст ожидающего caller’а и контекст общей загрузки. Первый
caller не должен отменить запрос для остальных участников singleflight. Поэтому
общая загрузка получает независимый timeout, а каждый caller может отдельно
прекратить ожидание.

```go
package fetcher

import (
    "context"
    "errors"
    "fmt"
    "strconv"
    "sync"
    "sync/atomic"
    "time"

    "github.com/hashicorp/golang-lru/v2/expirable"
    "golang.org/x/sync/singleflight"
)

var ErrInvalidConfig = errors.New("invalid fetcher config")

type Result struct {
    ID   int
    Data string
}

type Outcome struct {
    Index  int
    ID     int
    Result Result
    Err    error
}

type Source interface {
    Fetch(ctx context.Context, id int) (Result, error)
}

type Config struct {
    Workers       int
    CacheCapacity int
    CacheTTL      time.Duration
    FetchTimeout  time.Duration
}

func DefaultConfig() Config {
    return Config{
        Workers:       4,
        CacheCapacity: 10_000,
        CacheTTL:      5 * time.Minute,
        FetchTimeout:  2 * time.Second,
    }
}

type Fetcher struct {
    source Source
    cache  *expirable.LRU[int, Result]
    group  singleflight.Group
    cfg    Config

    cacheHits     atomic.Int64
    cacheMisses   atomic.Int64
    sourceFetches atomic.Int64
    sharedResults atomic.Int64
    errors        atomic.Int64
}

func New(source Source, cfg Config) (*Fetcher, error) {
    if source == nil || cfg.Workers <= 0 || cfg.CacheCapacity <= 0 ||
        cfg.CacheTTL <= 0 || cfg.FetchTimeout <= 0 {
        return nil, ErrInvalidConfig
    }

    cache := expirable.NewLRU[int, Result](
        cfg.CacheCapacity,
        nil,
        cfg.CacheTTL,
    )
    return &Fetcher{source: source, cache: cache, cfg: cfg}, nil
}

func (f *Fetcher) Get(ctx context.Context, id int) (Result, error) {
    if err := ctx.Err(); err != nil {
        return Result{}, err
    }
    if result, ok := f.cache.Get(id); ok {
        f.cacheHits.Add(1)
        return result, nil
    }
    f.cacheMisses.Add(1)

    resultCh := f.group.DoChan(strconv.Itoa(id), func() (any, error) {
        // Повторная проверка закрывает окно между внешним miss и запуском fn.
        if result, ok := f.cache.Get(id); ok {
            return result, nil
        }

        sharedCtx, cancel := context.WithTimeout(
            context.WithoutCancel(ctx),
            f.cfg.FetchTimeout,
        )
        defer cancel()

        f.sourceFetches.Add(1)
        result, err := f.source.Fetch(sharedCtx, id)
        if err != nil {
            return Result{}, err
        }
        f.cache.Add(id, result)
        return result, nil
    })

    select {
    case call := <-resultCh:
        if call.Shared {
            // Это число участников shared-результата, а не число запросов,
            // которые singleflight точно предотвратил.
            f.sharedResults.Add(1)
        }
        if call.Err != nil {
            f.errors.Add(1)
            return Result{}, call.Err
        }
        result, ok := call.Val.(Result)
        if !ok {
            f.errors.Add(1)
            return Result{}, fmt.Errorf("unexpected result type %T", call.Val)
        }
        return result, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()
    }
}

type batchJob struct {
    index int
    id    int
}

func (f *Fetcher) FetchAll(
    ctx context.Context,
    ids []int,
) <-chan Outcome {
    out := make(chan Outcome)
    jobs := make(chan batchJob)

    go func() {
        defer close(jobs)
        for index, id := range ids {
            select {
            case jobs <- batchJob{index: index, id: id}:
            case <-ctx.Done():
                return
            }
        }
    }()

    var workerWG sync.WaitGroup
    workerWG.Add(f.cfg.Workers)
    for workerID := 0; workerID < f.cfg.Workers; workerID++ {
        go func() {
            defer workerWG.Done()
            for current := range jobs {
                result, err := f.Get(ctx, current.id)
                outcome := Outcome{
                    Index:  current.index,
                    ID:     current.id,
                    Result: result,
                    Err:    err,
                }

                select {
                case out <- outcome:
                case <-ctx.Done():
                    return
                }
            }
        }()
    }

    go func() {
        workerWG.Wait()
        close(out)
    }()
    return out
}

type Stats struct {
    CacheHits     int64
    CacheMisses   int64
    SourceFetches int64
    SharedResults int64
    Errors        int64
}

func (f *Fetcher) Stats() Stats {
    return Stats{
        CacheHits:     f.cacheHits.Load(),
        CacheMisses:   f.cacheMisses.Load(),
        SourceFetches: f.sourceFetches.Load(),
        SharedResults: f.sharedResults.Load(),
        Errors:        f.errors.Load(),
    }
}
```

---

## Trade-offs reference implementation

### Один канал outcome

Один канал сохраняет связь между ID и ошибкой и не требует одновременно
дренировать два потока. `Index` позволяет вызывающему коду восстановить порядок,
не заставляя worker pool удерживать весь batch в памяти.

### Контекст singleflight

`singleflight.DoChan` позволяет caller’у прекратить ожидание. Сама общая работа
продолжается до `FetchTimeout`, даже если все callers уже ушли. Это осознанная
цена независимости от контекста первого участника. Для дорогой работы можно
реализовать reference counting и отменять источник после ухода последнего
caller’а, но такой lifecycle заметно сложнее.

### Ограниченный TTL-кэш

`expirable.LRU` является thread-safe. В используемой версии capacity `0`
отключает ограничение размера, а TTL `0` отключает expiration, поэтому `New`
отвергает оба значения. Библиотека также запускает cleanup-goroutine на весь
lifecycle кэша; `Fetcher` следует создавать как долгоживущий объект, а не на
каждый запрос.

### Потеря порядка

Публикация по мере готовности уменьшает задержку первых результатов. Если API
обязан вернуть срез в исходном порядке, caller создаёт `[]Outcome` длиной
`len(ids)` и записывает элементы по `Index`.

### Кэш и ошибки

Reference implementation не кэширует ошибки. Для устойчивой permanent-ошибки
это может создавать повторную нагрузку. Negative caching допустим только с
коротким TTL и точной классификацией ошибок: timeout и временный `5xx` нельзя
держать так же долго, как `404` для неизменяемого ресурса.

---

## Тесты

Тесты синхронизируют goroutine через каналы, а не угадывают момент конкурентного
выполнения с помощью `time.Sleep`.

```go
package fetcher

import (
    "context"
    "sync"
    "testing"
    "time"
)

type testSource struct {
    mu      sync.Mutex
    calls   map[int]int
    started chan struct{}
    release <-chan struct{}
}

func (s *testSource) Fetch(
    ctx context.Context,
    id int,
) (Result, error) {
    s.mu.Lock()
    s.calls[id]++
    s.mu.Unlock()

    if s.started != nil {
        select {
        case s.started <- struct{}{}:
        default:
        }
    }
    if s.release != nil {
        select {
        case <-s.release:
        case <-ctx.Done():
            return Result{}, ctx.Err()
        }
    }
    return Result{ID: id, Data: "data"}, nil
}

func TestFetcherSingleflight(t *testing.T) {
    release := make(chan struct{})
    started := make(chan struct{}, 1)
    source := &testSource{
        calls:   make(map[int]int),
        started: started,
        release: release,
    }
    fetcher, err := New(source, DefaultConfig())
    if err != nil {
        t.Fatal(err)
    }

    const callers = 10
    results := make(chan error, callers)
    var callerWG sync.WaitGroup
    callerWG.Add(callers)
    for i := 0; i < callers; i++ {
        go func() {
            defer callerWG.Done()
            _, callErr := fetcher.Get(context.Background(), 42)
            results <- callErr
        }()
    }

    <-started
    close(release)
    callerWG.Wait()
    close(results)

    for callErr := range results {
        if callErr != nil {
            t.Fatal(callErr)
        }
    }

    source.mu.Lock()
    calls := source.calls[42]
    source.mu.Unlock()
    if calls != 1 {
        t.Fatalf("source calls = %d, want 1", calls)
    }
}

func TestCallerCancellationDoesNotPoisonSharedFetch(t *testing.T) {
    release := make(chan struct{})
    started := make(chan struct{}, 1)
    source := &testSource{
        calls:   make(map[int]int),
        started: started,
        release: release,
    }
    fetcher, err := New(source, DefaultConfig())
    if err != nil {
        t.Fatal(err)
    }

    firstCtx, cancelFirst := context.WithCancel(context.Background())
    firstDone := make(chan error, 1)
    go func() {
        _, callErr := fetcher.Get(firstCtx, 7)
        firstDone <- callErr
    }()
    <-started

    secondDone := make(chan error, 1)
    go func() {
        _, callErr := fetcher.Get(context.Background(), 7)
        secondDone <- callErr
    }()

    cancelFirst()
    if callErr := <-firstDone; callErr != context.Canceled {
        t.Fatalf("first error = %v, want context.Canceled", callErr)
    }

    close(release)
    if callErr := <-secondDone; callErr != nil {
        t.Fatalf("second error = %v, want nil", callErr)
    }
}

func TestFetchAllClosesOutput(t *testing.T) {
    source := &testSource{calls: make(map[int]int)}
    cfg := DefaultConfig()
    cfg.FetchTimeout = time.Second
    fetcher, err := New(source, cfg)
    if err != nil {
        t.Fatal(err)
    }

    outcomes := fetcher.FetchAll(context.Background(), []int{1, 2, 1})
    count := 0
    for outcome := range outcomes {
        if outcome.Err != nil {
            t.Fatal(outcome.Err)
        }
        count++
    }
    if count != 3 {
        t.Fatalf("outcomes = %d, want 3", count)
    }
}
```

Запускать нужно как минимум `go test -race ./...`. Для cache stampede полезен
отдельный benchmark с горячими и равномерно распределёнными ключами.

---

## Interview-ready answer

**1. Чем отправка в `nil`-канал отличается от отправки в закрытый?**

- `nil`-канал — отправка и получение блокируются навсегда.
- Закрытый канал — отправка вызывает panic, а получение возвращает оставшиеся
  значения и затем zero value с `ok == false`.
- `select` — case с `nil`-каналом никогда не становится готовым и может
  использоваться для отключения ветки.

**2. Почему mutex не предотвращает cache stampede?**

- Защита map — mutex делает отдельные чтения и записи безопасными.
- Окно miss — lock отпускается раньше медленного запроса, поэтому несколько
  worker’ов начинают одинаковую работу.
- Дедупликация — `singleflight` объединяет только одновременно выполняющиеся
  запросы одного ключа.

**3. Кто должен закрывать выходной канал worker pool?**

- Владелец — канал закрывает goroutine, которая знает, что все senders завершены.
- Координация — worker’ы вызывают `Done`, а отдельный closer ждёт `Wait` и делает
  единственный `close(out)`.
- Запрет — consumer не закрывает канал, в который продолжают писать producer’ы.

**4. Как вернуть ошибки из конкурентного batch?**

- Связь — результат, ID, индекс и ошибка помещаются в один `Outcome`.
- Политика — API отдельно фиксирует fail-fast или продолжение после ошибки.
- Отмена — caller обязан дренировать канал до закрытия либо отменить контекст.

**5. Как context взаимодействует с singleflight?**

- Общая работа — все callers одного ключа получают один результат.
- Риск — контекст первого caller’а может преждевременно отменить работу для
  остальных, если напрямую передать его в `fn`.
- Компромисс — общая работа получает собственный timeout, а каждый caller
  отдельно отменяет ожидание результата.

---

## Связанные материалы

- [Worker Pool](../concurrency/01-worker-pool.md) — базовая координация worker’ов.
- [Singleflight](../concurrency/06-singleflight.md) — дедупликация in-flight
  запросов.
- [LRU Cache](../data-structures/01-lru-cache.md) — ограничение размера кэша.
- [Retry с backoff](../system-primitives/02-retry-with-backoff.md) — политика
  повторных запросов.
- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md) — отмена и deadline.
