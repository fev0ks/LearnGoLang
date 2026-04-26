# Backpressure и Load Shedding

Когда сервис получает больше нагрузки, чем может обработать, он должен **управлять перегрузкой**, а не падать. Два главных инструмента: backpressure (замедлить входящий поток) и load shedding (отклонить часть запросов).

## Содержание

- [Что происходит без управления перегрузкой](#что-происходит-без-управления-перегрузкой)
- [Backpressure](#backpressure)
- [Load Shedding](#load-shedding)
- [Bounded concurrency через semaphore](#bounded-concurrency-через-semaphore)
- [Worker pool как естественный backpressure](#worker-pool-как-естественный-backpressure)
- [Adaptive concurrency](#adaptive-concurrency)
- [Антипаттерны](#антипаттерны)

---

## Что происходит без управления перегрузкой

```
Сервис рассчитан на 1000 req/s, latency 50ms

Нагрузка: 2000 req/s
→ Очередь растёт
→ Goroutines накапливаются (каждый запрос ждёт)
→ Память заканчивается
→ GC давление растёт → паузы → latency растёт → очередь растёт быстрее
→ OOM kill или полная деградация

Итог: 0 req/s вместо 1000 req/s — система работала хуже без нагрузки
```

Цель: при перегрузке обрабатывать 1000 req/s с нормальной latency и явно отклонять остальные, вместо попытки обработать 2000 с деградацией для всех.

---

## Backpressure

Backpressure — сигнал upstream "замедлись, я не успеваю". В Go это реализуется через блокирующие каналы или semaphore.

```go
// Bounded channel как backpressure
requests := make(chan Request, 100)  // буфер = max очередь

// Producer (HTTP handler)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    select {
    case h.requests <- parseRequest(r):
        w.WriteHeader(http.StatusAccepted)  // принято в очередь
    default:
        // Очередь полна — backpressure → отклоняем
        w.Header().Set("Retry-After", "1")
        http.Error(w, "service overloaded", http.StatusServiceUnavailable)
    }
}

// Consumer (worker)
func (h *Handler) worker() {
    for req := range h.requests {
        h.process(req)
    }
}
```

В gRPC backpressure встроен в протокол: HTTP/2 flow control ограничивает отправителя если получатель не успевает читать.

---

## Load Shedding

Load shedding — намеренный отказ части запросов при перегрузке. В отличие от backpressure, не замедляет upstream, а явно отклоняет с `503`.

### По размеру очереди

```go
type LoadShedder struct {
    inFlight int64
    maxLoad  int64
}

func (ls *LoadShedder) Allow() bool {
    current := atomic.LoadInt64(&ls.inFlight)
    return current < ls.maxLoad
}

func (ls *LoadShedder) Acquire() bool {
    if !ls.Allow() {
        return false
    }
    atomic.AddInt64(&ls.inFlight, 1)
    return true
}

func (ls *LoadShedder) Release() {
    atomic.AddInt64(&ls.inFlight, -1)
}

// Middleware
func LoadShedMiddleware(shedder *LoadShedder) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !shedder.Acquire() {
                w.Header().Set("Retry-After", "1")
                http.Error(w, "server overloaded", http.StatusServiceUnavailable)
                return
            }
            defer shedder.Release()
            next.ServeHTTP(w, r)
        })
    }
}
```

### По CPU/Memory utilization

```go
import "runtime"

func isCPUOverloaded() bool {
    var stats runtime.MemStats
    runtime.ReadMemStats(&stats)
    // GCCPUFraction — доля времени процессора занятая GC
    // Если > 20% — система под давлением
    return stats.GCCPUFraction > 0.2
}
```

### Priority shedding

При перегрузке отклоняй низкоприоритетные запросы первыми, пропускай высокоприоритетные:

```go
type Priority int

const (
    PriorityHigh   Priority = 3
    PriorityNormal Priority = 2
    PriorityLow    Priority = 1
)

type PriorityLoadShedder struct {
    inFlight  int64
    capacity  int64  // 100% capacity
    highWater int64  // 80% — начать шедить Low
    lowWater  int64  // 60% — начать шедить Normal
}

func (s *PriorityLoadShedder) Allow(p Priority) bool {
    current := atomic.LoadInt64(&s.inFlight)
    switch p {
    case PriorityHigh:
        return current < s.capacity
    case PriorityNormal:
        return current < s.highWater
    case PriorityLow:
        return current < s.lowWater
    }
    return false
}
```

---

## Bounded concurrency через semaphore

Semaphore — самый прямой способ ограничить параллелизм для конкретного ресурса:

```go
// Semaphore через buffered channel
type Semaphore chan struct{}

func NewSemaphore(n int) Semaphore {
    return make(Semaphore, n)
}

func (s Semaphore) Acquire(ctx context.Context) error {
    select {
    case s <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s Semaphore) Release() {
    <-s
}

// Использование — не более 10 параллельных запросов к DB
dbSem := NewSemaphore(10)

func (r *Repo) Query(ctx context.Context, id string) (*User, error) {
    if err := dbSem.Acquire(ctx); err != nil {
        return nil, fmt.Errorf("db semaphore: %w", err)
    }
    defer dbSem.Release()

    return r.pool.QueryRow(ctx, "SELECT * FROM users WHERE id=$1", id)
}
```

Или через `golang.org/x/sync/semaphore`:
```go
import "golang.org/x/sync/semaphore"

sem := semaphore.NewWeighted(10)

func (r *Repo) Query(ctx context.Context, id string) (*User, error) {
    if err := sem.Acquire(ctx, 1); err != nil {
        return nil, err
    }
    defer sem.Release(1)
    // ...
}
```

---

## Worker pool как естественный backpressure

Worker pool с bounded input channel автоматически создаёт backpressure:

```go
type WorkerPool struct {
    jobs    chan Job
    results chan Result
    wg      sync.WaitGroup
}

func NewWorkerPool(workers, queueSize int) *WorkerPool {
    p := &WorkerPool{
        jobs:    make(chan Job, queueSize),
        results: make(chan Result, queueSize),
    }
    for i := 0; i < workers; i++ {
        p.wg.Add(1)
        go func() {
            defer p.wg.Done()
            for job := range p.jobs {
                p.results <- process(job)
            }
        }()
    }
    return p
}

func (p *WorkerPool) Submit(ctx context.Context, job Job) error {
    select {
    case p.jobs <- job:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Очередь полна — load shed
        return ErrPoolOverloaded
    }
}
```

`workers` — максимальная параллельность. `queueSize` — максимальный накоп. При превышении — явный `ErrPoolOverloaded`.

---

## Adaptive concurrency

Вместо фиксированного лимита — динамически подстраивать concurrency под latency. Алгоритм AIMD (Additive Increase / Multiplicative Decrease):

```go
type AdaptiveLimiter struct {
    mu        sync.Mutex
    limit     int64
    inFlight  int64
    minLimit  int64
    maxLimit  int64
    // AIMD: при успехе +1, при timeout *0.9
}

func (l *AdaptiveLimiter) onSuccess(latency time.Duration) {
    l.mu.Lock()
    defer l.mu.Unlock()
    if latency < l.targetLatency {
        l.limit = min(l.limit+1, l.maxLimit)  // additive increase
    }
}

func (l *AdaptiveLimiter) onError() {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.limit = max(int64(float64(l.limit)*0.9), l.minLimit)  // multiplicative decrease
}
```

Готовые реализации: `netflix/concurrency-limits` (Java, с Go портами), `platinummonkey/go-concurrency-limits`.

---

## Антипаттерны

**Unbounded goroutines** — `go processRequest(req)` в HTTP handler без любых ограничений. При spike нагрузки — миллион goroutines, OOM.

**Очередь без верхнего предела** — `make(chan Job)` с большим буфером или динамическим ростом. Очередь накапливает работу, latency растёт, клиент уже получил timeout, а сервис продолжает обрабатывать.

```go
// Плохо — обрабатываем запросы которые клиент уже не ждёт
jobs <- expiredJob  // клиент отключился 30s назад, context.Canceled

// Хорошо — проверять ctx в worker
for job := range jobs {
    if job.ctx.Err() != nil {
        continue  // клиент уже не ждёт — пропустить
    }
    process(job)
}
```

**Отклонять все запросы при любой перегрузке** — нужен headroom. Держать 80% capacity как нормальный режим, shed только при > 80-90%.

**Нет метрик на queue depth и in-flight** — без этого невозможно понять приближается ли система к пределу.
