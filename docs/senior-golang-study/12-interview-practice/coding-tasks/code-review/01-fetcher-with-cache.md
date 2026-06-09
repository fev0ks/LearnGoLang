# Задача 1: Fetcher с кэшем — bug hunt

Концентрированная senior-задача: дан 50-строчный fetcher с **дюжиной** проблем (deadlock, race, nil panic, неправильный lifecycle, cache stampede). Кандидат должен найти все, объяснить consequences, переписать.

Это **один из самых сильных** форматов для оценки production-thinking за 30-40 минут.

## Формулировка

> "Вот код, который должен параллельно делать запросы по списку ID, кэшировать результаты, возвращать их через channel. Запусти, посмотри что не так. Опиши все проблемы и предложи fix."

---

## Изначальный код

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
    ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
    defer cancel()

    f := NewFetcher()

    ids := []int{1, 2, 3, 2, 4, 1, 5, 6, 7, 8, 9}

    start := time.Now()
    defer fmt.Println("duration", time.Since(start))

    for r := range f.FetchAll(ids) {
        fmt.Println(r)
    }
}

func NewFetcher() *Fetcher {
    return &Fetcher{}
}

func (f *Fetcher) doRequest(id int) Result {
    time.Sleep(50 * time.Millisecond)
    return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}
}

func (f *Fetcher) FetchAll(ids []int) chan Result {
    var out chan Result
    var jobs chan int

    go func() {
        defer close(jobs)
        for _, id := range ids {
            jobs <- id
        }
    }()

    for i := 0; i < 4; i++ {
        go func(worker int) {
            for id := range jobs {
                r, ok := f.cache[id]
                if ok {
                    out <- r
                    continue
                }
                r = f.doRequest(id)
                f.cache[id] = r
                out <- r
            }
        }(i)
    }

    return out
}
```

Запусти. Получишь что-то типа:
```
panic: send on nil channel
or
fatal error: all goroutines are asleep - deadlock!
```

И не одно из этого.

---

## Уточняющие вопросы перед reviewing

Senior сначала **задаст вопросы**, чтобы понять что от него ждут:

1. **Какая семантика результатов — один на каждый input id или dedup?**
   "В ids дубль `1` и `2`. Должны ли результаты тоже дублироваться?"

2. **Порядок результатов важен?**
   "Если важен — нужны индексы и сборка. Если нет — async OK."

3. **Что делать при ошибке `doRequest`?**
   "Игнорировать, retry, fail-fast?"

4. **Допустима ли отмена через context?**
   "Должен ли cancel прерывать запросы?"

5. **Lifecycle кэша — навсегда или TTL?**
   "Если навсегда — memory leak. Если TTL — какой?"

Вопросы — это **сигнал seniority** сами по себе. Junior сразу пишет код.

---

## Список проблем (12 штук)

### Critical (упадёт сразу)

#### 1. `var out chan Result` — nil channel

```go
var out chan Result  // ← nil
// ...
out <- r  // ← блокирует НАВСЕГДА
```

**Что произойдёт:** worker пытается записать в nil channel → блок навсегда. После того как все воркеры заблочились — deadlock detector Go panic'нет с "all goroutines are asleep".

**Fix:**
```go
out := make(chan Result, len(ids))  // или unbuffered, но с правильным lifecycle
```

#### 2. `var jobs chan int` — nil channel

```go
var jobs chan int  // ← nil
// ...
jobs <- id  // ← блокирует навсегда
for id := range jobs {  // ← никогда не получит
```

То же самое — nil channel. Producer и workers оба зависают.

**Fix:**
```go
jobs := make(chan int)
```

#### 3. `cache` не инициализирован → panic

```go
return &Fetcher{}  // cache == nil
// ...
f.cache[id] = r  // ← panic: assignment to entry in nil map
```

**Fix:**
```go
return &Fetcher{cache: make(map[int]Result)}
```

#### 4. Race condition на cache

Несколько worker'ов одновременно читают и пишут map:
```go
r, ok := f.cache[id]  // read
f.cache[id] = r       // write — concurrent → data race
```

**Что произойдёт:** `go test -race` найдёт; в худшем случае — corrupted map → panic. На проде — нестабильное поведение.

**Fix:**
```go
type Fetcher struct {
    mu    sync.RWMutex
    cache map[int]Result
}

f.mu.RLock()
r, ok := f.cache[id]
f.mu.RUnlock()

f.mu.Lock()
f.cache[id] = r
f.mu.Unlock()
```

### High (фундаментальные проблемы дизайна)

#### 5. `out` никогда не закрывается

```go
for r := range f.FetchAll(ids) {  // ← никогда не выйдет
```

`range` exits только на `close(ch)`. Никто не close'ит `out`. Даже если бы все воркеры завершились, `main` продолжал бы ждать.

**Fix:**
```go
// Отдельная "closer" goroutine
go func() {
    wg.Wait()       // дождаться всех workers
    close(out)      // только тогда close
}()
```

#### 6. Workers пишут в `out` без context-aware send

```go
out <- r  // ← блокирует если consumer не читает
```

При cancellation context'а — workers зависают на этой записи. WaitGroup никогда не закончится.

**Fix:**
```go
select {
case out <- r:
case <-ctx.Done():
    return
}
```

#### 7. `doRequest` игнорирует context

```go
func (f *Fetcher) doRequest(id int) Result {
    time.Sleep(50 * time.Millisecond)  // ← не отменяется
    return ...
}
```

Context cancelled → worker продолжает sleep'ить → не отменяется.

**Fix:**
```go
func (f *Fetcher) doRequest(ctx context.Context, id int) (Result, error) {
    select {
    case <-time.After(50 * time.Millisecond):
        return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()
    }
}
```

#### 8. Producer не учитывает context

```go
go func() {
    defer close(jobs)
    for _, id := range ids {
        jobs <- id  // ← блокирует если workers down или нет места
    }
}()
```

Если все workers умерли по cancel — producer навечно блокируется на write в jobs.

**Fix:**
```go
for _, id := range ids {
    select {
    case jobs <- id:
    case <-ctx.Done():
        return
    }
}
```

### Medium (cache stampede + design)

#### 9. Cache stampede на дубликатах

```go
ids := []int{1, 2, 3, 2, 4, 1, ...}
```

Два worker'а одновременно видят что `1` нет в кэше → оба делают `doRequest(1)` → дублирующий API call.

С увеличением concurrency проблема только усугубляется. На "горячих" ключах — ддос на downstream.

**Fix:** `golang.org/x/sync/singleflight`.

#### 10. Семантика дубликатов в input не определена

Что значит `ids = [1, 2, 1]`?
- Вернуть 3 результата (как сейчас пытается)?
- Вернуть 2 уникальных?

Это нужно **зафиксировать в API**. Сейчас неявно.

#### 11. Порядок результатов не сохранён

Worker'ы пишут в `out` по мере готовности → порядок зависит от тайминга. Если caller ожидает same order as ids — баг.

### Low (нет на смерть, но плохой дизайн)

#### 12. Нет error channel

```go
func (f *Fetcher) FetchAll(ids []int) chan Result
```

Если `doRequest` упадёт — как сообщить caller'у? Сейчас никак.

#### 13. Магические числа

```go
for i := 0; i < 4; i++ {  // 4 workers
time.Sleep(50 * time.Millisecond)  // 50ms latency
ctx := context.WithTimeout(..., 75*time.Millisecond)  // 75ms
```

Что менять при изменении нагрузки? Параметризовать.

#### 14. `worker` параметр не используется

```go
go func(worker int) {  // worker не используется внутри
```

Удалить или использовать для логов.

#### 15. Cache без limits/TTL

Map растёт бесконечно. Через месяц работы — OOM.

#### 16. Cache не вынесен в отдельную абстракцию

Cache в Fetcher жёстко зашит. Тестировать без cache невозможно. Replace на Redis тоже.

---

## Решение Level 1 — Quick fix

Минимально работающий код. Без stampede protection, без LRU, без metrics — но **не падает** и **корректен**:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"
)

type Result struct {
    ID   int
    Data string
}

type Fetcher struct {
    mu    sync.RWMutex
    cache map[int]Result
}

func NewFetcher() *Fetcher {
    return &Fetcher{cache: make(map[int]Result)}
}

func (f *Fetcher) doRequest(ctx context.Context, id int) (Result, error) {
    select {
    case <-time.After(50 * time.Millisecond):
        return Result{ID: id, Data: fmt.Sprintf("value-%d", id)}, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()
    }
}

func (f *Fetcher) FetchAll(ctx context.Context, ids []int) <-chan Result {
    out := make(chan Result)
    jobs := make(chan int)

    var wg sync.WaitGroup

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

    // Workers
    for i := 0; i < 4; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for id := range jobs {
                // Cache lookup
                f.mu.RLock()
                r, ok := f.cache[id]
                f.mu.RUnlock()

                if !ok {
                    // Miss
                    var err error
                    r, err = f.doRequest(ctx, id)
                    if err != nil {
                        return  // или error channel
                    }
                    f.mu.Lock()
                    f.cache[id] = r
                    f.mu.Unlock()
                }

                select {
                case out <- r:
                case <-ctx.Done():
                    return
                }
            }
        }()
    }

    // Closer
    go func() {
        wg.Wait()
        close(out)
    }()

    return out
}

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
    defer cancel()

    f := NewFetcher()
    ids := []int{1, 2, 3, 2, 4, 1, 5, 6, 7, 8, 9}

    for r := range f.FetchAll(ctx, ids) {
        fmt.Println(r)
    }
}
```

Это **полный минимум**. Каждая критическая проблема решена.

---

## Решение Level 2 — Production-grade

Здесь добавляются senior-level расширения:
- **Singleflight** против stampede
- **LRU cache с TTL** вместо unbounded map
- **Error channel** для возврата ошибок
- **Parametrized config**
- **Метрики** (cache hit rate)

```go
package fetcher

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "sync/atomic"
    "time"

    "github.com/hashicorp/golang-lru/v2/expirable"
    "golang.org/x/sync/singleflight"
)

type Result struct {
    ID   int
    Data string
}

// Source — абстракция над real fetch operation.
type Source interface {
    Fetch(ctx context.Context, id int) (Result, error)
}

type Config struct {
    Workers       int
    CacheCapacity int
    CacheTTL      time.Duration
}

func DefaultConfig() Config {
    return Config{
        Workers:       4,
        CacheCapacity: 10_000,
        CacheTTL:      5 * time.Minute,
    }
}

type Fetcher struct {
    source Source
    cache  *expirable.LRU[int, Result]
    sf     singleflight.Group
    cfg    Config

    // Metrics
    cacheHits   atomic.Int64
    cacheMisses atomic.Int64
    sfShared    atomic.Int64
    errors      atomic.Int64
}

func New(source Source, cfg Config) *Fetcher {
    cache := expirable.NewLRU[int, Result](cfg.CacheCapacity, nil, cfg.CacheTTL)
    return &Fetcher{
        source: source,
        cache:  cache,
        cfg:    cfg,
    }
}

// Get для одиночного запроса с stampede protection и кэшем.
func (f *Fetcher) Get(ctx context.Context, id int) (Result, error) {
    // Cache lookup
    if r, ok := f.cache.Get(id); ok {
        f.cacheHits.Add(1)
        return r, nil
    }
    f.cacheMisses.Add(1)

    // Singleflight: одинаковый id одновременно → один запрос
    v, err, shared := f.sf.Do(fmt.Sprintf("%d", id), func() (any, error) {
        return f.source.Fetch(ctx, id)
    })
    if shared {
        f.sfShared.Add(1)
    }
    if err != nil {
        f.errors.Add(1)
        return Result{}, err
    }

    r := v.(Result)
    f.cache.Add(id, r)
    return r, nil
}

// FetchAll — streaming для batch.
// Возвращает results и errors через отдельные каналы.
// Порядок — undefined (документировано).
func (f *Fetcher) FetchAll(ctx context.Context, ids []int) (<-chan Result, <-chan error) {
    out := make(chan Result)
    errCh := make(chan error, 1)  // буфер 1 — для первой ошибки

    jobs := make(chan int)

    var wg sync.WaitGroup

    // Producer
    wg.Add(1)
    go func() {
        defer wg.Done()
        defer close(jobs)
        for _, id := range ids {
            select {
            case jobs <- id:
            case <-ctx.Done():
                return
            }
        }
    }()

    // Workers
    var workerWG sync.WaitGroup
    for i := 0; i < f.cfg.Workers; i++ {
        workerWG.Add(1)
        go func() {
            defer workerWG.Done()
            for id := range jobs {
                r, err := f.Get(ctx, id)
                if err != nil {
                    // Context errors — quiet exit
                    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
                        return
                    }
                    // Other errors — report (non-blocking, only first)
                    select {
                    case errCh <- err:
                    default:
                    }
                    // Continue — другие IDs могут success'нуть
                    continue
                }

                select {
                case out <- r:
                case <-ctx.Done():
                    return
                }
            }
        }()
    }

    // Closer
    go func() {
        workerWG.Wait()
        wg.Wait()
        close(out)
        close(errCh)
    }()

    return out, errCh
}

type Stats struct {
    CacheHits   int64
    CacheMisses int64
    SfShared    int64
    Errors      int64
}

func (f *Fetcher) Stats() Stats {
    return Stats{
        CacheHits:   f.cacheHits.Load(),
        CacheMisses: f.cacheMisses.Load(),
        SfShared:    f.sfShared.Load(),
        Errors:      f.errors.Load(),
    }
}
```

**Ключевые изменения относительно Level 1:**

### 1. `Source` interface

Cache, fetcher и **источник данных** разделены. Тесты легко мокать. На production source — HTTP client, БД, что угодно.

### 2. `expirable.LRU` вместо map

- **Bounded size** — `CacheCapacity` = 10k. Старое выбрасывается.
- **TTL** — записи expire через 5 минут.
- **Thread-safe** под капотом.

### 3. `singleflight.Group` против stampede

```go
v, err, shared := f.sf.Do(fmt.Sprintf("%d", id), func() (any, error) {
    return f.source.Fetch(ctx, id)
})
```

Параллельные запросы по тому же `id` → **один реальный fetch**, остальные получают тот же result. См. [concurrency/06-singleflight.md](../concurrency/06-singleflight.md).

### 4. Error channel — non-fatal errors

В отличие от твоего решения, где `cancel()` валит **всё** при первой ошибке, здесь:
- Context errors → silent exit
- Business errors → record в errCh (non-blocking), continue по другим IDs

**Why:** в batch processing часто хочется получить максимум results, не прерывать всё на одной ошибке. Если 99 IDs success и 1 fail — лучше получить 99 + знать про fail, чем 0 + ошибка.

Если **fail-fast** реально нужен (платежи?) — это другой режим, например `FetchAllStrict`.

### 5. Метрики

- `CacheHits`/`CacheMisses` — hit rate cache
- `SfShared` — сколько concurrent dedup'ов (показывает stampede pressure)
- `Errors` — error count

В production экспортится в Prometheus.

### 6. Get как public API

```go
func (f *Fetcher) Get(ctx context.Context, id int) (Result, error)
```

Helpful для случаев когда нужен один результат — не нужно batch. Использует тот же путь cache + singleflight.

---

## Сравнение с твоим решением (after/task_after.go)

Что в твоём решении правильно:
- ✅ Context propagation в `fetch`
- ✅ Mutex на cache (RWMutex — отличный выбор)
- ✅ Closer goroutine для close(out)
- ✅ Select с `ctx.Done()` при отправке
- ✅ Initial sense что `len(ids)` для buffer'а может быть проще (комментарий)

Что можно улучшить:

### A. `cancel()` при первой ошибке — слишком агрессивно

```go
if errors.Is(err, errSome) {
    cancel()  // ← валит весь pipeline
}
```

В реальности: один ID упал → почему все остальные тоже отменяем? Лучше — report error через channel, continue.

**Исключение:** если ошибка означает "downstream полностью down" — да, cancel разумен. Но обычно ты не знаешь это из обычной error.

### B. `select default` для errCh = теряем все ошибки кроме первой

```go
select {
case errCh <- err:
default:  // ← если буфер занят — error потерян
}
```

С `errCh := make(chan error, 1)` — fит первая ошибка, остальные теряются. Это OK для **fail-fast** mode (нам нужна only one), но не для **collect all errors**.

**Альтернатива:** `errCh chan error` без буфера + reader goroutine, или slice errors под mutex.

### C. Cache без LRU/TTL — leak

В комментарии ты сам отметил:
```
//утечки памяти //нет eviction //нет TTL //рост latency // OOM
```

Решения:
- `hashicorp/golang-lru/v2/expirable` (выше)
- Свой LRU с TTL (см. [data-structures/01-lru-cache.md](../data-structures/01-lru-cache.md))

### D. Stampede не предотвращён

При `ids = [1, 2, 1, 2, 1]` — два worker'а одновременно увидят что `1` нет в кэше → два запроса. Добавь singleflight.

### E. Order — твоё решение тоже не сохраняет

Возможно это **OK**, но нужно **документировать**. Senior отмечает контракт явно.

### F. `out := make(chan Result)` unbuffered vs `len(ids)`

Твой комментарий правильно отмечает trade-off:
```
//? Буфер len(ids) здесь — не оптимизация, а средство безопасности
```

Но на самом деле unbuffered **тоже OK** если:
- Closer goroutine правильно работает
- Workers используют select с `ctx.Done()`

Это **естественная backpressure** — workers не убегут далеко вперёд consumer'а. Для memory-sensitive операций (большие Results) — unbuffered лучше.

С `len(ids)` буфером — workers могут сделать всю работу даже если consumer медленный, и сидеть с готовыми results в памяти. Иногда это нужно, иногда — memory leak.

### G. `worker int` параметр

```go
go func(worker int) {
    // worker не используется
}(i)
```

Удалить или использовать (например, в логах).

### H. Параметры hardcoded

`for i := 0; i < 5; i++` — magic 5. Сделай `Config`.

---

## Тесты

```go
package fetcher

import (
    "context"
    "errors"
    "sync"
    "sync/atomic"
    "testing"
    "time"
)

type mockSource struct {
    mu       sync.Mutex
    calls    map[int]int  // count calls per ID
    delay    time.Duration
    failOnce map[int]bool  // ID → fail first call
}

func (m *mockSource) Fetch(ctx context.Context, id int) (Result, error) {
    m.mu.Lock()
    m.calls[id]++
    callNum := m.calls[id]
    shouldFail := m.failOnce[id] && callNum == 1
    m.mu.Unlock()

    select {
    case <-time.After(m.delay):
        if shouldFail {
            return Result{}, errors.New("transient")
        }
        return Result{ID: id, Data: "data"}, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()
    }
}

func TestFetcher_Basic(t *testing.T) {
    src := &mockSource{calls: make(map[int]int), delay: 10 * time.Millisecond}
    f := New(src, Config{Workers: 4, CacheCapacity: 100, CacheTTL: time.Minute})

    ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
    defer cancel()

    out, errCh := f.FetchAll(ctx, []int{1, 2, 3, 4, 5})

    var count int
    for range out {
        count++
    }

    if err := <-errCh; err != nil {
        t.Fatal(err)
    }
    if count != 5 {
        t.Errorf("got %d results, want 5", count)
    }
}

func TestFetcher_DedupViaSingleflight(t *testing.T) {
    src := &mockSource{calls: make(map[int]int), delay: 50 * time.Millisecond}
    f := New(src, DefaultConfig())

    ctx := context.Background()

    // 10 параллельных запросов одного ID
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            f.Get(ctx, 42)
        }()
    }
    wg.Wait()

    src.mu.Lock()
    calls := src.calls[42]
    src.mu.Unlock()

    if calls != 1 {
        t.Errorf("source called %d times, want 1 (singleflight should dedup)", calls)
    }
}

func TestFetcher_CacheReuse(t *testing.T) {
    src := &mockSource{calls: make(map[int]int), delay: 10 * time.Millisecond}
    f := New(src, DefaultConfig())

    ctx := context.Background()

    // Fetch первый раз
    f.Get(ctx, 1)
    f.Get(ctx, 1)  // должен прийти из cache

    src.mu.Lock()
    calls := src.calls[1]
    src.mu.Unlock()

    if calls != 1 {
        t.Errorf("source called %d times, want 1 (cache should reuse)", calls)
    }

    stats := f.Stats()
    if stats.CacheHits != 1 {
        t.Errorf("cache hits %d, want 1", stats.CacheHits)
    }
}

func TestFetcher_ContextCancel(t *testing.T) {
    src := &mockSource{calls: make(map[int]int), delay: 100 * time.Millisecond}
    f := New(src, DefaultConfig())

    ctx, cancel := context.WithCancel(context.Background())

    var processed atomic.Int32
    done := make(chan bool)
    go func() {
        out, _ := f.FetchAll(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
        for range out {
            processed.Add(1)
        }
        done <- true
    }()

    time.Sleep(50 * time.Millisecond)
    cancel()

    <-done

    // Some должны успеть, но не все
    p := processed.Load()
    if p == 10 {
        t.Error("expected not all to complete after cancel")
    }
}

func TestFetcher_RaceDetector(t *testing.T) {
    src := &mockSource{calls: make(map[int]int), delay: time.Millisecond}
    f := New(src, Config{Workers: 10, CacheCapacity: 100, CacheTTL: time.Minute})

    ctx := context.Background()

    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            f.Get(ctx, id%10)  // hot keys
        }(i)
    }
    wg.Wait()
    // go test -race ./... — должно проходить без warnings
}
```

`go test -race ./...` обязательно.

---

## Чек-лист ответа на собеседовании (30 минут)

| Минута | Что делать |
|---|---|
| 0-2 | Прочитать код, задать уточняющие вопросы (см. выше) |
| 2-5 | Перечислить **критические** проблемы (nil channel, nil map, race) — то что упадёт сразу |
| 5-10 | Назвать **design** проблемы (lifecycle close, context, stampede) |
| 10-25 | Написать **Level 1 решение** — рабочий код без advanced features |
| 25-30 | Обсудить **расширения** — singleflight, LRU, метрики, errgroup — указать на trade-offs |

Если успел сделать всё с Level 2 + объяснил trade-offs — это **strong senior**.

---

## Уровни ответа

### Junior:
"Тут гонка на map, и канал nil. Надо mutex и `make`."

### Middle:
Видит ~5 проблем (nil channels, nil map, race, no close, no context). Пишет рабочий код. Не упоминает stampede/lifecycle nuances.

### Senior:
Все 12+ проблем. Объясняет **что именно произойдёт** в каждой. Знает singleflight, LRU, errgroup. Пишет parameterized config. Упоминает метрики. Делает тест с race detector. Документирует семантику (order, dedup).

### Strong Senior / Tech Lead:
Плюс:
- Source interface abstraction
- Trade-off обсуждения (unbuffered vs buffered, fail-fast vs collect)
- Где этот код будет жить и как его эксплуатировать
- Когда `Get` лучше `FetchAll` API
- Что мониторить (latency p99, hit rate, error rate)
- Когда менять архитектуру (e.g., on 1M ids per call)

---

## Senior-level ответы (reference)

Краткие "хорошие" ответы на типичные follow-ups:

**"Как ты обычно реализуешь ретраи?"**
> "Ретраи делаю только для transient ошибок, с экспоненциальным backoff и jitter, ограничиваю количеством попыток и обязательно учитываю deadline контекста. Для небезопасных операций — только с идемпотентностью."

**"Можно ли ретраить любой запрос?"**
> "Только идемпотентные. POST без идемпотентного ключа — нет. GET/PUT/DELETE — обычно да, но проверяя 5xx/timeout, не 4xx."

**"singleflight vs errgroup?"**
> "Разные задачи. singleflight — дедупликация одинаковой работы. errgroup — параллельное выполнение независимых задач со сбором ошибок. Я использовал бы оба здесь: errgroup для координации worker'ов, singleflight для дедупликации внутри `Get(id)`."

**"LRU vs LFU?"**
> "LRU — про недавность, LFU — про частоту. LRU проще и лучше для динамической нагрузки, LFU — для стабильных hot keys, но требует aging. В продакшне часто используют гибриды вроде ARC или TinyLFU."

**"Сервис зависает раз в несколько дней. -race чистый, panic'ов нет, CPU/memory в норме. С чего начнёшь?"**
> "Первое — `goroutine pprof` — посмотреть нет ли утечки. Если она есть, обычно растёт счёт горутин со временем. Дальше — block profile (`pprof?block`) — где блокируется. Подозрительные места: channels без select+ctx, mutex contention, slow downstream. Параллельно — `runtime/trace` чтобы видеть execution patterns. Если ничего — расследовать timeouts на downstream, может GC pause или page faults."

**"Планировщик Go?"**
> "G-M-P модель: goroutine выполняются поверх OS threads через logical processors. GOMAXPROCS задаёт число P. Work stealing балансирует нагрузку между P. Блокирующие syscalls не останавливают другие goroutines — runtime создаёт новый M или забирает P. С Go 1.14 — async preemption, до этого был cooperative."

---

## Что показать на собеседовании

1. **Перечисли проблемы в правильном порядке приоритета** — сначала critical (упадёт), потом design
2. **Объясни consequences каждой** — "nil channel → deadlock через 50ms" лучше чем "nil channel"
3. **Не сразу пиши код** — задай вопросы, объяви assumption'ы
4. **Помни про context** — везде, не только в fetch
5. **Singleflight для stampede** — называется, не реализуется руками
6. **LRU bound для cache** — упомяни `hashicorp/golang-lru` или равноценный
7. **Тесты с -race** — обязательно
8. **API контракт** — порядок, dedup, errors, lifecycle — задокументируй

## Связки

- [Worker Pool](../concurrency/01-worker-pool.md) — базовый паттерн
- [Singleflight](../concurrency/06-singleflight.md) — против stampede
- [LRU Cache](../data-structures/01-lru-cache.md) — bounded cache
- [Retry с Backoff](../system-primitives/02-retry-with-backoff.md) — расширение
- [Context patterns](../../../01-go-core/concurrency-and-performance/05-context-patterns.md)
