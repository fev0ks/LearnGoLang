# Goroutine и Concurrency профилирование

Горутин может быть много, и каждая где-то "висит". Goroutine профиль показывает что именно — channel, mutex, syscall, или ничего не делает. Block и mutex профили показывают где горутины теряют время в ожидании.

## Содержание

- [Goroutine профиль](#goroutine-профиль)
- [Читаем goroutine dump](#читаем-goroutine-dump)
- [Типичные горутина-ловушки](#типичные-горутина-ловушки)
- [Block профиль: где горутины ждали](#block-профиль-где-горутины-ждали)
- [Mutex профиль: contention на локах](#mutex-профиль-contention-на-локах)
- [ThreadCreate профиль](#threadcreate-профиль)
- [Как найти goroutine leak](#как-найти-goroutine-leak)
- [Interview-ready answer](#interview-ready-answer)

---

## Goroutine профиль

Goroutine профиль — **снимок всех горутин прямо сейчас**: сколько их, и где каждая остановлена.

```bash
# Краткий: агрегированные stack traces (сколько горутин на каждом стеке)
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/goroutine"

# Полный текстовый дамп всех горутин
curl "http://localhost:6060/debug/pprof/goroutine?debug=2"

# Или в браузере:
# http://localhost:6060/debug/pprof/goroutine?debug=2
```

`debug=1` → каждый уникальный стек с количеством горутин  
`debug=2` → полный дамп всех горутин по одной (огромный для больших сервисов)

### Быстрая проверка количества

```go
// В метриках или логах:
n := runtime.NumGoroutine()
fmt.Printf("goroutines: %d\n", n)

// Нормально для web-сервиса под нагрузкой: len(workers) + несколько десятков служебных
// Подозрительно: тысячи при маленьком RPS или постоянный рост
```

---

## Читаем goroutine dump

```
goroutine 42 [IO wait]:               ← номер горутины, состояние
internal/poll.runtime_pollWait(...)
    /usr/local/go/src/runtime/netpoll.go:351
internal/poll.(*pollDesc).waitRead(...)
net.(*conn).Read(0xc000124120, ...)
main.handleConn(0xc000124000)
    /app/server.go:87 +0x1a4
created by main.acceptLoop
    /app/server.go:45 +0x68
```

### Состояния горутин

| Состояние | Что происходит |
|---|---|
| `IO wait` | ждёт сетевого I/O (нормально для сетевых горутин) |
| `chan receive` | блокирована на `<-ch` — никто не пишет |
| `chan send` | блокирована на `ch <-` — никто не читает (буфер полный) |
| `select` | ждёт одного из случаев в select |
| `semacquire` | ждёт mutex, semaphore или WaitGroup |
| `sleep` | time.Sleep |
| `syscall` | в системном вызове (file I/O) |
| `runnable` | готова к выполнению, ждёт P |
| `running` | выполняется прямо сейчас |
| `GC sweep wait` | ждёт GC sweep |

### Сигналы проблем

```
Много горутин в "chan receive" на одном и том же месте:
→ производитель перестал писать, но горутины ждут
→ утечка или deadlock

Много горутин в "IO wait" при маленьком числе соединений:
→ соединения не закрываются (нет deadline)

Много горутин в "semacquire" на одном mutex:
→ lock contention

Монотонный рост числа горутин:
→ goroutine leak — горутины создаются, но не завершаются
```

---

## Типичные горутина-ловушки

### 1. Channel без close и без context

```go
// ❌ Утечка: горутина висит в chan receive вечно
func process(jobs <-chan Job) {
    for job := range jobs {  // если jobs никогда не закрыть — висит
        handle(job)
    }
}

// ✅ С context — горутина завершится при отмене
func process(ctx context.Context, jobs <-chan Job) {
    for {
        select {
        case job, ok := <-jobs:
            if !ok { return }
            handle(job)
        case <-ctx.Done():
            return
        }
    }
}
```

В дампе: `goroutine N [chan receive, X minutes]` с большим X — подозрительно.

### 2. Горутина-зомби: результат игнорируется

```go
// ❌ Горутина висит, ожидая записи в ch, но никто не читает
func startWorker() {
    ch := make(chan Result)
    go func() {
        result := compute()
        ch <- result  // ВИСИТ если никто не читает ch
    }()
    // ch нигде не используется дальше — горутина застряла
}

// ✅ Буферизованный канал или явная отмена
func startWorker(ctx context.Context) <-chan Result {
    ch := make(chan Result, 1)  // буфер = горутина не блокируется
    go func() {
        defer close(ch)
        result := compute()
        select {
        case ch <- result:
        case <-ctx.Done():
        }
    }()
    return ch
}
```

### 3. Timer и Ticker без Stop

```go
// ❌ Ticker горутина никогда не завершится
func startPolling() {
    ticker := time.NewTicker(5 * time.Second)
    go func() {
        for range ticker.C {
            poll()
        }
        // ← никогда не выйдет из цикла
    }()
}

// ✅ Stop + drain
func startPolling(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                poll()
            case <-ctx.Done():
                return
            }
        }
    }()
}
```

В дампе: много горутин в `sleep` (time.Sleep или ticker.C).

### 4. WaitGroup без завершения

```go
// ❌ Если одна из горутин паникует — Done не вызовется, Wait зависнет
var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    go func(item Item) {
        processItem(item)  // паника тут?
        wg.Done()          // не вызовется при панике
    }(item)
}
wg.Wait()  // может зависнуть

// ✅ defer всегда
go func(item Item) {
    defer wg.Done()
    processItem(item)
}(item)
```

---

## Block профиль: где горутины ждали

Block профиль записывает, **сколько времени горутины провели в блокировке** на:
- `sync.Mutex.Lock()`
- `sync.RWMutex.RLock()` / `Lock()`
- Channel send/receive (блокирующие)
- `sync.WaitGroup.Wait()`
- `time.Sleep` (не всегда)

### Включение и сбор

```go
// В main или init — ДО начала обработки запросов
runtime.SetBlockProfileRate(1)  // записывать каждую блокировку (дорого!)
// Для production: SetBlockProfileRate(10000) — каждую 10000-ю наносекунду ожидания
```

```bash
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/block"
```

### Чтение

```
(pprof) top
      flat  flat%   sum%        cum   cum%
    12.34s 45.67% 45.67%     12.34s 45.67%  sync.(*RWMutex).RLock
     8.91s 32.99% 78.66%      8.91s 32.99%  main.(*Cache).Get
```

Высокий flat в `RLock` при большом числе читателей → много конкуренции за блокировку чтения.

**Важно:** block профиль не покажет CPU время — только время ожидания. Функция с 10 секундами в block профиле не тратила 10 секунд CPU, она 10 секунд ждала.

---

## Mutex профиль: contention на локах

Mutex профиль фокусируется на конкретно `sync.Mutex` и `sync.RWMutex` contention — где лок уже занят и горутина вынуждена ждать.

### Включение

```go
runtime.SetMutexProfileFraction(1)  // записывать каждый конфликтный lock (дорого)
// Для production: 10 (каждый 10-й конфликт)
```

```bash
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/mutex"
```

### Пример находки: map под RWMutex на горячем пути

```go
// ❌ Глобальный RWMutex на каждую операцию — высокое contention при большом числе горутин
type Counter struct {
    mu    sync.RWMutex
    counts map[string]int64
}

func (c *Counter) Inc(key string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.counts[key]++
}

func (c *Counter) Get(key string) int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.counts[key]
}
```

Mutex профиль покажет: `sync.(*Mutex).Lock` с большим flat от `Counter.Inc`.

```go
// ✅ Вариант 1: sync/atomic для простых счётчиков
type Counter struct {
    counts sync.Map  // или sharded map
}

// ✅ Вариант 2: sharding — N независимых счётчиков, ключ → шард
type ShardedCounter struct {
    shards [256]struct {
        mu    sync.Mutex
        count int64
        _     [56]byte  // cache line padding
    }
}
func (c *ShardedCounter) Inc(key string) {
    shard := fnv32(key) % 256
    c.shards[shard].mu.Lock()
    c.shards[shard].count++
    c.shards[shard].mu.Unlock()
}
```

### Block vs Mutex: в чём разница

| | Block профиль | Mutex профиль |
|---|---|---|
| Что считает | Время ожидания (channel, mutex, semaphore) | Только mutex lock contention |
| Включение | `SetBlockProfileRate` | `SetMutexProfileFraction` |
| Overhead | Высокий | Средний |
| Когда использовать | "где горутины вообще ждут" | "где конкретно локи мешают" |

---

## ThreadCreate профиль

Показывает стеки, которые создали OS threads. Полезен при неожиданно большом числе threads.

```bash
go tool pprof "http://localhost:6060/debug/pprof/threadcreate"
```

```bash
# Посмотреть количество OS threads сейчас
cat /proc/$(pgrep myapp)/status | grep Threads
```

Много threads (> GOMAXPROCS * 4) → подозрение на:
- Много blocking file I/O без ограничения параллелизма
- CGo блокирующие вызовы
- `runtime.LockOSThread` без парного `UnlockOSThread`

---

## Как найти goroutine leak

### Шаг 1: Мониторинг роста

```go
// Экспортировать в Prometheus
func metricsHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "go_goroutines %d\n", runtime.NumGoroutine())
}
```

Если горутин постоянно больше чем (workers + connections + небольшой baseline) → утечка.

### Шаг 2: Сравнить дампы

```bash
# До нагрузки
curl "http://localhost:6060/debug/pprof/goroutine?debug=1" > goroutines_before.txt

# После нагрузки и небольшой паузы (нормальные горутины завершились)
curl "http://localhost:6060/debug/pprof/goroutine?debug=1" > goroutines_after.txt

# Сравнить
diff goroutines_before.txt goroutines_after.txt
```

Что выросло — то и течёт.

### Шаг 3: Читать стек

```
goroutine 1847 [chan receive, 47 minutes]:   ← 47 минут! очевидная утечка
main.(*Worker).run(0xc000198900)
    /app/worker.go:54 +0x8c
created by main.(*Pool).Start
    /app/pool.go:23 +0x44
```

Горутина ждёт 47 минут на chan receive в `worker.go:54`. Смотрим код — где этот worker должен был завершиться.

### Шаг 4: goleak для тестов

```go
import "go.uber.org/goleak"

func TestMyComponent(t *testing.T) {
    defer goleak.VerifyNone(t)  // тест упадёт если после него остались горутины

    component := NewMyComponent()
    component.Start()
    // ... тест ...
    component.Stop()
}
```

`goleak` незаменим для unit/integration тестов.

---

## Interview-ready answer

**"Горутины растут и не уменьшаются. Как найдёшь причину?"**

Сначала подтверждаю по метрикам: `runtime.NumGoroutine()` растёт и не возвращается в baseline после спада нагрузки.

Собираю два дампа: до и после нагрузки, жду немного — нормальные горутины завершатся.

```bash
curl "http://localhost:6060/debug/pprof/goroutine?debug=1" > before.txt
# ...нагрузка...
curl "http://localhost:6060/debug/pprof/goroutine?debug=1" > after.txt
diff before.txt after.txt
```

В дампе ищу горутины с большим временем ожидания (`chan receive, N minutes`) или неожиданно большим количеством на одном стеке. Смотрю строку `created by` — это место, где создаётся утечка.

Самые частые причины:
1. **Channel без close** — горутина ждёт `range ch`, но channel никто не закрывает. Fix: `close(ch)` или context.
2. **Goroutine не слушает context** — нет `case <-ctx.Done()`. Fix: добавить ветку отмены.
3. **Ticker без Stop** — горутина читает `ticker.C` но Stop никогда не вызывается. Fix: `defer ticker.Stop()`.

Для тестов использую `goleak.VerifyNone(t)` — сразу ловит утечки в тестовой среде.

**Lock contention:** если throughput падает при нормальном CPU — включаю mutex профиль `runtime.SetMutexProfileFraction(1)`, собираю, смотрю где больше всего ожидания. Fix: sharding, sync.Map, atomic или пересмотр scope блокировки.
