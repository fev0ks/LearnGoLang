# Задача 2: Broken Background Task Processor

Дан долгоживущий **task processor** — другой сценарий чем fetcher из [01-fetcher-with-cache.md](./01-fetcher-with-cache.md). Там был batch (fetched список и завершился). Здесь — **long-running worker pool**: принимает задачи через `Submit()`, делает их асинхронно, retry'ит при ошибках, поддерживает graceful `Stop()`.

Такой код встречается в **email senders, image processors, background jobs, webhook delivery, notification dispatchers**. Куча проблем которые легко сделать.

## Формулировка

> "Дан BackgroundProcessor — pool воркеров для асинхронной обработки задач с retry. Запусти, посмотри что ломается. Найди все проблемы, предложи fix."

---

## Изначальный код

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "math/rand"
    "sync"
    "time"
)

type Task struct {
    ID      int
    Payload string
    attempts int  // счётчик попыток
}

type Processor struct {
    queue   chan *Task
    workers int
    stop    chan struct{}
    wg      sync.WaitGroup
}

func NewProcessor(workers int) *Processor {
    return &Processor{
        queue:   make(chan *Task, 100),
        workers: workers,
        stop:    make(chan struct{}),
    }
}

func (p *Processor) Start() {
    for i := 0; i < p.workers; i++ {
        go func(id int) {
            p.wg.Add(1)
            defer p.wg.Done()

            for {
                select {
                case task := <-p.queue:
                    p.processTask(task)
                case <-p.stop:
                    return
                }
            }
        }(i)
    }
}

func (p *Processor) processTask(t *Task) {
    err := doWork(t.Payload)
    if err != nil {
        t.attempts++
        fmt.Printf("task %d failed (attempt %d), retrying\n", t.ID, t.attempts)
        p.queue <- t  // re-enqueue для retry
    } else {
        fmt.Printf("task %d done\n", t.ID)
    }
}

func (p *Processor) Submit(t *Task) {
    p.queue <- t
}

func (p *Processor) Stop() {
    close(p.stop)
    p.wg.Wait()
    close(p.queue)
}

func doWork(payload string) error {
    time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
    if rand.Intn(3) == 0 {
        return errors.New("transient failure")
    }
    return nil
}

func main() {
    p := NewProcessor(3)
    p.Start()

    for i := 0; i < 20; i++ {
        p.Submit(&Task{ID: i, Payload: fmt.Sprintf("task-%d", i)})
    }

    time.Sleep(time.Second)
    p.Stop()
}
```

Запусти. Получишь что-то из:
- `panic: send on closed channel` (при Stop()+Submit race)
- Бесконечный retry (если task всегда падает)
- Тест с `-race` ругается на `attempts`
- Stop() висит навсегда

И не одно одновременно.

---

## Уточняющие вопросы

1. **Что делать с tasks в queue при Stop?**
   "Drain (доделать) или drop (потерять)? Зависит от criticality."

2. **Maximum retry attempts?**
   "Без лимита — infinite loop. 3-5 типично, потом dead letter."

3. **Что считать success vs retryable error vs fatal?**
   "Сейчас всё считается retryable — даже permanent ошибки бесконечно крутятся."

4. **Должен ли Submit блокировать когда queue full?**
   "Block / drop / return error — три варианта."

5. **Timeout per task?**
   "Без timeout — slow task блокирует worker навсегда."

6. **Graceful shutdown timeout?**
   "Если workers in-flight — ждать сколько? Forever / 30s / cancel?"

---

## Список проблем (14 штук)

### Critical (ломают сразу)

#### 1. `wg.Add(1)` внутри goroutine — race с `wg.Wait()`

```go
go func(id int) {
    p.wg.Add(1)        // ← race: Wait может выполниться раньше Add
    defer p.wg.Done()
    ...
}(i)
```

**Что происходит:** `Stop()` вызывает `wg.Wait()`. Если он сработал до того как одна из goroutines выполнила `Add(1)` — Wait увидит counter=0 и завершится **немедленно**. Goroutine продолжает работать, обращается к `p.queue` или `p.stop` — может быть закрыто → panic.

**Правило:** `wg.Add()` **до** `go func()`, на стороне caller'а, не внутри.

**Fix:**
```go
for i := 0; i < p.workers; i++ {
    p.wg.Add(1)  // ← здесь
    go func(id int) {
        defer p.wg.Done()
        // ...
    }(i)
}
```

#### 2. Send в closed channel — panic

```go
func (p *Processor) Stop() {
    close(p.stop)
    p.wg.Wait()
    close(p.queue)  // ← закрываем
}

func (p *Processor) Submit(t *Task) {
    p.queue <- t  // ← если Stop уже выполнил close(queue) — panic
}
```

**Что происходит:** Caller вызывает Stop. Параллельно другая goroutine вызывает Submit. После `close(p.queue)` — `Submit` пишет в закрытый channel → `panic: send on closed channel`.

**Fix:** не закрывать queue вообще (let GC), или закрывать через atomic flag:
```go
func (p *Processor) Submit(t *Task) error {
    if p.stopped.Load() {
        return ErrProcessorStopped
    }
    select {
    case p.queue <- t:
        return nil
    case <-p.stop:
        return ErrProcessorStopped
    }
}
```

#### 3. `t.attempts++` без atomic — race condition

```go
type Task struct {
    attempts int  // ← shared между workers
}

// Worker 1
t.attempts++  // ← read-modify-write, не atomic

// Worker 2 (тот же *Task через retry)
t.attempts++  // ← race!
```

**Что происходит:** Task ре-энквью'ится после retry. Если несколько worker'ов обрабатывают один объект последовательно — race detector найдёт. Реально на одном CPU может работать "случайно", на multi-core — terror.

**Fix:** не shared mutable state в Task — использовать local counter:
```go
type Task struct {
    ID      int
    Payload string
    // attempts вынесены в WorkItem внутри pool
}

type workItem struct {
    task     *Task
    attempts int  // не shared, в одной goroutine за раз
}
```

#### 4. `range select` на queue — потеря данных при Stop

```go
for {
    select {
    case task := <-p.queue:  // ← может выбрать ЭТУ ветку
        p.processTask(task)
    case <-p.stop:           // ← или ЭТУ
        return
    }
}
```

Когда оба готовы — Go runtime выбирает **случайно**. При Stop в queue могут остаться tasks, они **никогда** не обработаются — leak. И current task в обработке может не finished'ниться.

**Fix:** двухфазный shutdown — сначала "не принимать новые", потом "доделать pending", потом "stop". См. Level 2.

### High (фундаментальные дизайн issues)

#### 5. Infinite retry — нет max attempts

```go
if err != nil {
    t.attempts++
    p.queue <- t  // ← retry forever
}
```

Если task всегда падает (permanent error, bad data) — он крутится **навсегда**, занимая worker. С 3 worker'ами и 3 permanent-fail tasks — pool полностью deadlocked.

**Fix:**
```go
if err != nil {
    if attempts >= maxAttempts {
        // Dead letter queue / log
        p.deadLetter(task, err)
        return
    }
    // Retry с backoff
    time.Sleep(backoff(attempts))
    p.queue <- task
}
```

#### 6. Нет backoff при retry — busy loop

```go
p.queue <- t  // ← сразу опять выполнить
```

Task с permanent error моментально reenqueue'ится → worker берёт, fail'ит, reenqueue'ит. CPU loop, downstream не успевает восстановиться. Cascade failure.

**Fix:** exponential backoff (`time.Sleep(backoff(attempts))` before re-queue).

#### 7. `processTask` блокирует worker если queue full

```go
p.queue <- t  // ← если buffer full, worker блокируется навсегда
```

С buffer 100 и 3 workers — если все 100 в queue + retry'ятся → workers пишут в queue → блок → 3 workers сидят на этом write → новые tasks никто не берёт → deadlock.

**Fix:** retry **не в той же queue**, а с задержкой через timer или отдельной retry queue. См. Level 2.

#### 8. `doWork` игнорирует context

```go
func doWork(payload string) error {
    time.Sleep(...)
    // ничего про cancellation
}
```

`Stop()` → workers ждут окончания текущего task. Если task занимает 60 секунд — Stop висит 60 секунд. Без context'а — никак не прервать.

**Fix:** `doWork(ctx, payload)` с `select { case <-ctx.Done(): }`.

### Medium

#### 9. Stop не drain'ит queue

```go
func (p *Processor) Stop() {
    close(p.stop)      // workers выходят сразу
    p.wg.Wait()
    close(p.queue)
}
```

Tasks в queue **теряются**. Для some сценариев (real-time notifications) — OK. Для critical (платежи) — недопустимо.

**Fix:** опция "drain mode" — Stop с deadline дождётся пока queue empty:
```go
func (p *Processor) Stop(ctx context.Context) error {
    p.stopAccepting()  // не принимать новые
    select {
    case <-p.allDone:
        return nil
    case <-ctx.Done():
        return ctx.Err()  // forced stop
    }
}
```

#### 10. Submit без backpressure — full queue блокирует caller

```go
func (p *Processor) Submit(t *Task) {
    p.queue <- t  // ← блокирует если queue full
}
```

Если producer быстрее consumers — Submit блокирует. Hot loops в caller'е → memory leak (goroutines накапливаются waiting на Submit).

**Fix:** non-blocking Submit с error:
```go
func (p *Processor) Submit(t *Task) error {
    select {
    case p.queue <- t:
        return nil
    default:
        return ErrQueueFull  // или ErrBusy
    }
}

// Или blocking with context
func (p *Processor) SubmitWait(ctx context.Context, t *Task) error {
    select {
    case p.queue <- t:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

#### 11. Нет panic recovery в worker

```go
go func(id int) {
    for {
        case task := <-p.queue:
            p.processTask(task)  // ← panic убивает goroutine
    }
}(i)
```

`doWork` panic'нет (bug, division by zero, nil deref) → worker мёртв. С 3 worker'ами и 3 bad tasks → pool deadlocked.

**Fix:**
```go
func (p *Processor) processTask(t *Task) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("task %d panic: %v", t.ID, r)
            // Можно либо record как failed, либо в dead letter
        }
    }()
    // ... работа
}
```

### Low (UX / observability)

#### 12. `fmt.Printf` для логирования

Не structured, нет уровней, не log shipping в production.

**Fix:** `slog` или зависимость от Logger:
```go
type Processor struct {
    log *slog.Logger
}
```

#### 13. Loop variable `id` корректно captured, но не используется

```go
go func(id int) {
    // id не используется внутри
}(i)
```

Если capture'нуть для логов — полезно. Иначе — удалить параметр.

(В коде это правильно — `id` параметр функции, не loop variable. Так что захвата нет. Но pre-Go 1.22 если бы было `go func() { ... use id ... }()` — был бы баг.)

#### 14. Нет метрик

Сколько tasks в очереди? Сколько обработали успешно? Сколько fail'ов? Cardinality dead letter? — ничего.

**Fix:** Prometheus counters/gauges.

---

## Решение Level 1 — Quick fix

Минимальный fix всех **critical** проблем. Без backoff, без dead letter, без drain mode — но **не падает**.

```go
package processor

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "sync"
    "sync/atomic"
)

var (
    ErrQueueFull         = errors.New("queue full")
    ErrProcessorStopped  = errors.New("processor stopped")
    ErrMaxAttemptsReached = errors.New("max attempts reached")
)

type Task struct {
    ID      int
    Payload string
}

type workItem struct {
    task     *Task
    attempts int
}

type Processor struct {
    queue       chan *workItem
    workers     int
    maxAttempts int

    stop        chan struct{}
    wg          sync.WaitGroup
    stopped     atomic.Bool
    process     func(ctx context.Context, t *Task) error
}

func New(workers, maxAttempts int, process func(context.Context, *Task) error) *Processor {
    return &Processor{
        queue:       make(chan *workItem, 100),
        workers:     workers,
        maxAttempts: maxAttempts,
        stop:        make(chan struct{}),
        process:     process,
    }
}

func (p *Processor) Start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)  // Add ДО goroutine
        go func(id int) {
            defer p.wg.Done()
            p.workerLoop(id)
        }(i)
    }
}

func (p *Processor) workerLoop(id int) {
    for {
        select {
        case <-p.stop:
            return
        case wi := <-p.queue:
            if wi == nil {
                return  // safety
            }
            p.handle(wi)
        }
    }
}

func (p *Processor) handle(wi *workItem) {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("task panic", "task_id", wi.task.ID, "panic", r)
        }
    }()

    err := p.process(context.Background(), wi.task)
    if err == nil {
        return  // success
    }

    wi.attempts++
    if wi.attempts >= p.maxAttempts {
        slog.Warn("task gave up",
            "task_id", wi.task.ID,
            "attempts", wi.attempts,
            "err", err,
        )
        return
    }

    // Re-enqueue (non-blocking чтобы не deadlock)
    select {
    case p.queue <- wi:
    default:
        slog.Warn("retry queue full, dropping", "task_id", wi.task.ID)
    }
}

func (p *Processor) Submit(t *Task) error {
    if p.stopped.Load() {
        return ErrProcessorStopped
    }
    wi := &workItem{task: t}
    select {
    case p.queue <- wi:
        return nil
    default:
        return ErrQueueFull
    }
}

func (p *Processor) Stop() {
    if !p.stopped.CompareAndSwap(false, true) {
        return  // уже stopped
    }
    close(p.stop)
    p.wg.Wait()
    // не закрываем p.queue — может быть Submit в полёте, лучше дать GC убрать
}
```

**Что исправлено:**
- ✅ `wg.Add()` до `go` — нет race
- ✅ `attempts` в локальном `workItem`, не в Task
- ✅ `stopped` atomic flag — Submit'ы прекращаются после Stop
- ✅ Recover в handle — panic не валит worker
- ✅ `maxAttempts` — нет infinite retry
- ✅ Non-blocking Submit — backpressure через `ErrQueueFull`
- ✅ Non-blocking re-enqueue — нет deadlock'а

Чего ещё нет (Level 2):
- Backoff между retry
- Drain mode при Stop
- Dead letter queue
- Per-task timeout

---

## Решение Level 2 — Production-grade

```go
package processor

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "math"
    "math/rand"
    "sync"
    "sync/atomic"
    "time"
)

type Config struct {
    Workers        int
    QueueSize      int
    MaxAttempts    int
    BaseBackoff    time.Duration
    MaxBackoff     time.Duration
    TaskTimeout    time.Duration
    DrainTimeout   time.Duration
}

type Task struct {
    ID      string
    Payload []byte
}

type workItem struct {
    task      *Task
    attempts  int
    nextRetry time.Time
}

type ProcessFunc func(ctx context.Context, t *Task) error
type DeadLetterFunc func(t *Task, err error, attempts int)

type Processor struct {
    cfg         Config
    process     ProcessFunc
    deadLetter  DeadLetterFunc
    log         *slog.Logger

    queue       chan *workItem
    delayedQ    chan *workItem  // отдельная queue для retry с backoff

    stopAccept  atomic.Bool
    stopWorkers chan struct{}
    workerWG    sync.WaitGroup
    done        chan struct{}

    // Metrics
    processed atomic.Int64
    failed    atomic.Int64
    retried   atomic.Int64
    dropped   atomic.Int64
}

func New(cfg Config, process ProcessFunc, deadLetter DeadLetterFunc, log *slog.Logger) *Processor {
    if log == nil {
        log = slog.Default()
    }
    return &Processor{
        cfg:         cfg,
        process:     process,
        deadLetter:  deadLetter,
        log:         log,
        queue:       make(chan *workItem, cfg.QueueSize),
        delayedQ:    make(chan *workItem, cfg.QueueSize),
        stopWorkers: make(chan struct{}),
        done:        make(chan struct{}),
    }
}

func (p *Processor) Start() {
    // Workers
    for i := 0; i < p.cfg.Workers; i++ {
        p.workerWG.Add(1)
        go func(id int) {
            defer p.workerWG.Done()
            p.workerLoop(id)
        }(i)
    }

    // Delay dispatcher — берёт из delayedQ когда время retry'я наступило
    p.workerWG.Add(1)
    go func() {
        defer p.workerWG.Done()
        p.delayDispatcher()
    }()

    // Done signaller
    go func() {
        p.workerWG.Wait()
        close(p.done)
    }()
}

func (p *Processor) Submit(ctx context.Context, t *Task) error {
    if p.stopAccept.Load() {
        return ErrProcessorStopped
    }

    wi := &workItem{task: t}
    select {
    case p.queue <- wi:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    case <-p.stopWorkers:
        return ErrProcessorStopped
    }
}

func (p *Processor) TrySubmit(t *Task) error {
    if p.stopAccept.Load() {
        return ErrProcessorStopped
    }
    select {
    case p.queue <- &workItem{task: t}:
        return nil
    default:
        return ErrQueueFull
    }
}

// Stop инициирует graceful shutdown. Ждёт пока queue drained или ctx истечёт.
func (p *Processor) Stop(ctx context.Context) error {
    if !p.stopAccept.CompareAndSwap(false, true) {
        return errors.New("already stopping")
    }

    // Wait for queue drain or context
    drained := make(chan struct{})
    go func() {
        // Periodic check
        ticker := time.NewTicker(50 * time.Millisecond)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                if len(p.queue) == 0 && len(p.delayedQ) == 0 {
                    close(drained)
                    return
                }
            case <-ctx.Done():
                return
            }
        }
    }()

    select {
    case <-drained:
        // Queue empty — теперь stop workers
    case <-ctx.Done():
        p.log.Warn("graceful drain timeout, forcing stop",
            "remaining", len(p.queue)+len(p.delayedQ))
    }

    close(p.stopWorkers)
    <-p.done  // подождать workers
    return nil
}

func (p *Processor) workerLoop(id int) {
    for {
        select {
        case <-p.stopWorkers:
            return
        case wi := <-p.queue:
            p.handle(wi)
        }
    }
}

func (p *Processor) delayDispatcher() {
    // Простая реализация — single dispatcher с timer'ом.
    // Production — может быть priority queue по nextRetry времени.
    ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()

    var pending []*workItem

    for {
        select {
        case <-p.stopWorkers:
            return
        case wi := <-p.delayedQ:
            pending = append(pending, wi)
        case <-ticker.C:
            now := time.Now()
            kept := pending[:0]
            for _, wi := range pending {
                if now.After(wi.nextRetry) {
                    // Move в основную queue
                    select {
                    case p.queue <- wi:
                    default:
                        // queue full — оставить в pending до next tick
                        kept = append(kept, wi)
                    }
                } else {
                    kept = append(kept, wi)
                }
            }
            pending = kept
        }
    }
}

func (p *Processor) handle(wi *workItem) {
    defer func() {
        if r := recover(); r != nil {
            p.log.Error("task panic",
                "task_id", wi.task.ID,
                "panic", fmt.Sprintf("%v", r),
            )
            p.failed.Add(1)
            if p.deadLetter != nil {
                p.deadLetter(wi.task, fmt.Errorf("panic: %v", r), wi.attempts)
            }
        }
    }()

    // Per-task timeout
    ctx, cancel := context.WithTimeout(context.Background(), p.cfg.TaskTimeout)
    defer cancel()

    err := p.process(ctx, wi.task)
    if err == nil {
        p.processed.Add(1)
        return
    }

    wi.attempts++
    if wi.attempts >= p.cfg.MaxAttempts {
        p.failed.Add(1)
        p.log.Warn("task max attempts reached",
            "task_id", wi.task.ID,
            "attempts", wi.attempts,
            "err", err,
        )
        if p.deadLetter != nil {
            p.deadLetter(wi.task, err, wi.attempts)
        }
        return
    }

    // Schedule retry с exponential backoff + jitter
    backoff := p.computeBackoff(wi.attempts)
    wi.nextRetry = time.Now().Add(backoff)

    p.retried.Add(1)
    select {
    case p.delayedQ <- wi:
    default:
        p.dropped.Add(1)
        p.log.Warn("delayed queue full, dropping retry",
            "task_id", wi.task.ID,
        )
    }
}

func (p *Processor) computeBackoff(attempt int) time.Duration {
    backoff := float64(p.cfg.BaseBackoff) * math.Pow(2, float64(attempt-1))
    if backoff > float64(p.cfg.MaxBackoff) {
        backoff = float64(p.cfg.MaxBackoff)
    }
    // Full jitter
    return time.Duration(rand.Float64() * backoff)
}

type Stats struct {
    Processed int64
    Failed    int64
    Retried   int64
    Dropped   int64
    Queue     int
    Delayed   int
}

func (p *Processor) Stats() Stats {
    return Stats{
        Processed: p.processed.Load(),
        Failed:    p.failed.Load(),
        Retried:   p.retried.Load(),
        Dropped:   p.dropped.Load(),
        Queue:     len(p.queue),
        Delayed:   len(p.delayedQ),
    }
}
```

**Использование:**

```go
p := processor.New(processor.Config{
    Workers:      10,
    QueueSize:    1000,
    MaxAttempts:  5,
    BaseBackoff:  100 * time.Millisecond,
    MaxBackoff:   30 * time.Second,
    TaskTimeout:  10 * time.Second,
    DrainTimeout: 30 * time.Second,
}, processSomething, deadLetterToS3, slog.Default())

p.Start()

// Submit tasks
for _, t := range tasks {
    if err := p.TrySubmit(t); err != nil {
        // Queue full — handle (drop, alert, retry submit)
    }
}

// Graceful shutdown
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
p.Stop(ctx)
```

**Что добавлено:**

### A. Separate delayed queue для retry

Главная идея — **не reenqueue в основную очередь сразу**. Retry items идут в `delayedQ`, dispatcher периодически проверяет время и перемещает в основную queue. Главные преимущества:
- Основная queue не загромождается infinite retry-loop'ом
- Failing tasks не блокируют свежие
- Backoff реально работает (не просто `time.Sleep` в worker'е)

### B. Per-task timeout через ctx.WithTimeout

```go
ctx, cancel := context.WithTimeout(context.Background(), p.cfg.TaskTimeout)
defer cancel()
err := p.process(ctx, wi.task)
```

Slow task не залипает worker'а навсегда.

### C. Graceful drain mode

`Stop(ctx)`:
1. Stop accepting new submits
2. Wait queue drain или ctx timeout
3. Force stop workers

Trade-off: либо tasks доделываются (даже если долго), либо ctx истекает (force stop).

### D. Dead letter callback

```go
deadLetter(task, err, attempts)
```

После max attempts — task в DLQ. Может быть: S3 storage, Postgres table, отдельная queue, alert.

### E. Metrics

Counters processed/failed/retried/dropped + gauges queue size — для Prometheus.

---

## Тесты

```go
func TestProcessor_BasicSuccess(t *testing.T) {
    var processed atomic.Int32

    p := New(Config{
        Workers:     2,
        QueueSize:   10,
        MaxAttempts: 3,
        BaseBackoff: time.Millisecond,
        MaxBackoff:  10 * time.Millisecond,
        TaskTimeout: time.Second,
    }, func(ctx context.Context, t *Task) error {
        processed.Add(1)
        return nil
    }, nil, nil)

    p.Start()
    defer p.Stop(context.Background())

    for i := 0; i < 10; i++ {
        if err := p.Submit(context.Background(), &Task{ID: fmt.Sprint(i)}); err != nil {
            t.Fatal(err)
        }
    }

    // Wait for processing
    deadline := time.Now().Add(2 * time.Second)
    for processed.Load() < 10 && time.Now().Before(deadline) {
        time.Sleep(10 * time.Millisecond)
    }

    if processed.Load() != 10 {
        t.Errorf("processed %d, want 10", processed.Load())
    }
}

func TestProcessor_RetryThenSuccess(t *testing.T) {
    var attempts atomic.Int32
    var deadLettered atomic.Int32

    p := New(Config{
        Workers:     1,
        QueueSize:   10,
        MaxAttempts: 5,
        BaseBackoff: time.Millisecond,
        MaxBackoff:  10 * time.Millisecond,
        TaskTimeout: time.Second,
    }, func(ctx context.Context, t *Task) error {
        n := attempts.Add(1)
        if n < 3 {
            return errors.New("transient")
        }
        return nil
    }, func(task *Task, err error, atts int) {
        deadLettered.Add(1)
    }, nil)

    p.Start()
    defer p.Stop(context.Background())

    p.Submit(context.Background(), &Task{ID: "1"})

    time.Sleep(100 * time.Millisecond)

    if attempts.Load() != 3 {
        t.Errorf("attempts %d, want 3", attempts.Load())
    }
    if deadLettered.Load() != 0 {
        t.Errorf("should not dead letter on eventual success")
    }
}

func TestProcessor_MaxAttemptsExceeded(t *testing.T) {
    var deadLettered atomic.Int32

    p := New(Config{
        Workers:     1,
        QueueSize:   10,
        MaxAttempts: 3,
        BaseBackoff: time.Millisecond,
        TaskTimeout: time.Second,
    }, func(ctx context.Context, t *Task) error {
        return errors.New("permanent fail")
    }, func(task *Task, err error, atts int) {
        deadLettered.Add(1)
    }, nil)

    p.Start()
    defer p.Stop(context.Background())

    p.Submit(context.Background(), &Task{ID: "1"})

    time.Sleep(100 * time.Millisecond)

    if deadLettered.Load() != 1 {
        t.Errorf("dead lettered %d, want 1", deadLettered.Load())
    }
}

func TestProcessor_PanicRecovery(t *testing.T) {
    var deadLettered atomic.Int32

    p := New(Config{
        Workers:     1,
        QueueSize:   10,
        MaxAttempts: 1,
        BaseBackoff: time.Millisecond,
        TaskTimeout: time.Second,
    }, func(ctx context.Context, t *Task) error {
        panic("boom")
    }, func(task *Task, err error, atts int) {
        deadLettered.Add(1)
    }, nil)

    p.Start()
    defer p.Stop(context.Background())

    p.Submit(context.Background(), &Task{ID: "1"})
    p.Submit(context.Background(), &Task{ID: "2"})

    time.Sleep(100 * time.Millisecond)

    // Pool ВСЁ ЕЩЁ работает после panic
    if deadLettered.Load() != 2 {
        t.Errorf("dead lettered %d, want 2 (panic shouldn't kill pool)", deadLettered.Load())
    }
}

func TestProcessor_StopDrains(t *testing.T) {
    var processed atomic.Int32

    p := New(Config{
        Workers:     2,
        QueueSize:   100,
        MaxAttempts: 1,
        BaseBackoff: time.Millisecond,
        TaskTimeout: time.Second,
    }, func(ctx context.Context, t *Task) error {
        time.Sleep(10 * time.Millisecond)
        processed.Add(1)
        return nil
    }, nil, nil)

    p.Start()

    for i := 0; i < 20; i++ {
        p.Submit(context.Background(), &Task{ID: fmt.Sprint(i)})
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    p.Stop(ctx)

    // Stop должен drain все 20
    if processed.Load() != 20 {
        t.Errorf("after Stop processed %d, want 20", processed.Load())
    }
}

func TestProcessor_ConcurrentSubmitStop(t *testing.T) {
    p := New(Config{
        Workers:     5,
        QueueSize:   100,
        MaxAttempts: 1,
        BaseBackoff: time.Millisecond,
        TaskTimeout: time.Second,
    }, func(ctx context.Context, t *Task) error {
        return nil
    }, nil, nil)

    p.Start()

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            p.Submit(context.Background(), &Task{ID: fmt.Sprint(id)})
        }(i)
    }

    // Stop в parallel — не должно быть panic
    time.Sleep(10 * time.Millisecond)
    p.Stop(context.Background())

    wg.Wait()
    // Должны дойти без panic
}
```

`go test -race ./...` — обязательно.

---

## Уровни ответа

### Junior:
"Сделал бы wg.Add правильно, поставил mutex на attempts, не закрывал queue."

### Middle:
~5-7 проблем. Видит wg.Add race, send on closed channel, infinite retry. Не идёт глубже про drain mode, dead letter, separate retry queue.

### Senior:
Все 14 проблем. Объясняет последствия. Знает про:
- WaitGroup add-before-goroutine правило
- atomic flag для idempotent Stop
- Per-task timeout через ctx
- Backoff + jitter
- Drain mode при Stop
- Panic recovery в worker'е
- Dead letter pattern
- Separate retry queue (не reenqueue в основную)

### Strong Senior:
Плюс:
- Discussion: blocking vs non-blocking Submit (trade-offs)
- Priority queue для retry items
- Distributed scenarios (несколько pod'ов делят queue → Redis Streams / Kafka)
- Observability: metrics + tracing + log correlation
- Idempotency: что если task обработан и pod упал?
- Кто отвечает за task ownership при partial failure?

---

## Связки с другими задачами

Эта задача комбинирует:
- [Worker Pool](../concurrency/01-worker-pool.md) — базовый паттерн (но эта — long-running, не batch)
- [Retry с Backoff](../system-primitives/02-retry-with-backoff.md) — exponential + jitter
- [Graceful Shutdown](../../../04-architecture-and-patterns/patterns/08-graceful-shutdown.md) — Stop с context

И отличается от:
- [01-fetcher-with-cache.md](./01-fetcher-with-cache.md) — там batch, здесь long-running. Там одна fetch-проблема (stampede + lifecycle), здесь — retry-проблема (max attempts, backoff, dead letter, drain).

---

## Что важно показать на собеседовании

1. **WaitGroup.Add() до goroutine** — классический проброс
2. **atomic flag для Stop** — idempotent, защита от concurrent Submit/Stop
3. **Recover в worker** — panic в одной task не валит pool
4. **Max attempts + backoff** — иначе infinite retry убивает pool
5. **Separate retry queue** — не реenqueue в основную (deadlock + busy loop)
6. **Drain mode при Stop** — important для critical workloads
7. **Per-task timeout** — slow task не блокирует worker'а
8. **Dead letter** — куда фейлы уходят
9. **Тесты с race detector** — concurrent Submit+Stop сценарий обязательно
10. **Metrics** — без них production-fail не виден

## Связки

- [01. Fetcher with cache](./01-fetcher-with-cache.md) — другой code-review с batch-pool проблемами
- [Worker Pool](../concurrency/01-worker-pool.md) — базовый pattern
- [Retry с Backoff](../system-primitives/02-retry-with-backoff.md) — экспоненциальный backoff
- [Background Workers](../../../04-architecture-and-patterns/patterns/04-background-workers.md) — production patterns
