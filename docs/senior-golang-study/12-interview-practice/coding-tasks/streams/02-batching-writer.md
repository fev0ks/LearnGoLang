# Задача 2: Batching Writer

Накапливать события из потока и flush'ить **пачками** — либо при достижении size, либо по timeout, либо при graceful shutdown. Стандартный паттерн: DB bulk insert, S3 multipart upload, Datadog/Sentry batch send, Kafka producer batching.

## Формулировка

> "Дан непрерывный поток событий. Реализуй компонент, который накапливает их и flush'ит batch'ами: либо при достижении `batchSize`, либо каждые `flushInterval`. При shutdown — flush'ить pending без потерь."

Use cases:
- Bulk INSERT в БД (1 транзакция на 1000 events vs 1000 транзакций)
- S3 multipart upload — отправить chunks по 5MB
- Metrics batching — отправить statsd packet раз в секунду
- Audit log shipping — пачка в Kafka producer

---

## Уточняющие вопросы

1. **Triggers — size, time, или оба?**
   "Обычно оба: flush при `size >= N` ИЛИ `now - lastFlush > interval`."

2. **При flush — sync или async?**
   "Sync — flush блокирует Add. Async — отдельная goroutine. Зависит от throughput."

3. **Что при shutdown — drain или drop?**
   "Critical workloads — drain. Metrics — drop OK."

4. **Backpressure — что если flush медленный, а Add быстрый?**
   "Block, drop oldest, drop newest — три варианта. Зависит от criticality."

5. **Failure handling при flush?**
   "Retry, dead letter, drop? Persistent buffer?"

6. **Concurrent Add'ы?**
   "Обязательно safe — несколько producers."

---

## Базовое решение

```go
package batcher

import (
    "context"
    "errors"
    "sync"
    "time"
)

type Item interface{}  // или generic с Go 1.18+

type FlushFunc[T any] func(ctx context.Context, items []T) error

type Batcher[T any] struct {
    flush       FlushFunc[T]
    maxSize     int
    flushInterval time.Duration

    mu      sync.Mutex
    buffer  []T

    done       chan struct{}
    flushSignal chan struct{}
    closed     bool
}

func New[T any](maxSize int, interval time.Duration, flush FlushFunc[T]) *Batcher[T] {
    b := &Batcher[T]{
        flush:         flush,
        maxSize:       maxSize,
        flushInterval: interval,
        buffer:        make([]T, 0, maxSize),
        done:          make(chan struct{}),
        flushSignal:   make(chan struct{}, 1),
    }
    go b.loop()
    return b
}

// Add добавляет item. Если buffer заполнен — флашит.
func (b *Batcher[T]) Add(item T) error {
    b.mu.Lock()
    if b.closed {
        b.mu.Unlock()
        return errors.New("batcher closed")
    }

    b.buffer = append(b.buffer, item)
    shouldFlush := len(b.buffer) >= b.maxSize
    b.mu.Unlock()

    if shouldFlush {
        // Non-blocking signal — если flusher уже знает, не дублируем
        select {
        case b.flushSignal <- struct{}{}:
        default:
        }
    }
    return nil
}

func (b *Batcher[T]) loop() {
    ticker := time.NewTicker(b.flushInterval)
    defer ticker.Stop()

    for {
        select {
        case <-b.done:
            b.flushNow(context.Background())
            return
        case <-ticker.C:
            b.flushNow(context.Background())
        case <-b.flushSignal:
            b.flushNow(context.Background())
        }
    }
}

func (b *Batcher[T]) flushNow(ctx context.Context) {
    b.mu.Lock()
    if len(b.buffer) == 0 {
        b.mu.Unlock()
        return
    }
    batch := b.buffer
    b.buffer = make([]T, 0, b.maxSize)
    b.mu.Unlock()

    // Flush вне lock — Add'ы могут продолжаться
    if err := b.flush(ctx, batch); err != nil {
        // TODO: retry, dead letter
    }
}

// Close gracefully shutdown — flush pending и выйти.
func (b *Batcher[T]) Close() {
    b.mu.Lock()
    b.closed = true
    b.mu.Unlock()

    close(b.done)
}
```

**Использование:**

```go
batcher := batcher.New(1000, time.Second, func(ctx context.Context, items []Event) error {
    return db.BulkInsert(ctx, items)
})

for event := range stream {
    batcher.Add(event)
}

batcher.Close()
```

**Ключевые моменты:**
- **Triple trigger:** size limit, time interval, manual signal (через `flushSignal`)
- **`flushSignal` buffered 1** — drop duplicate signals (no-op если flusher already знает)
- **Flush вне lock** — Add'ы могут продолжаться пока flush'имся
- **Close → done channel → final flush** — graceful shutdown

---

## Production-grade: с retry, async, backpressure

```go
package batcher

import (
    "context"
    "errors"
    "log/slog"
    "sync"
    "sync/atomic"
    "time"
)

type Config struct {
    MaxBatchSize  int
    FlushInterval time.Duration
    MaxBufferSize int           // hard cap, после — block или drop
    OnFullPolicy  FullPolicy
    FlushTimeout  time.Duration
    MaxRetries    int
}

type FullPolicy int

const (
    BlockOnFull FullPolicy = iota
    DropOldest
    DropNewest
)

type Batcher[T any] struct {
    cfg      Config
    flush    FlushFunc[T]
    onError  func(items []T, err error)  // dead letter callback
    log      *slog.Logger

    mu       sync.Mutex
    cond     *sync.Cond
    buffer   []T
    flushing bool

    flushCh  chan []T
    done     chan struct{}
    wg       sync.WaitGroup
    closed   atomic.Bool

    // Metrics
    accepted  atomic.Int64
    flushed   atomic.Int64
    failed    atomic.Int64
    dropped   atomic.Int64
}

func New[T any](cfg Config, flush FlushFunc[T], onError func([]T, error), log *slog.Logger) *Batcher[T] {
    if log == nil {
        log = slog.Default()
    }
    b := &Batcher[T]{
        cfg:     cfg,
        flush:   flush,
        onError: onError,
        log:     log,
        buffer:  make([]T, 0, cfg.MaxBatchSize),
        flushCh: make(chan []T, 4),  // small buffer для async pipeline
        done:    make(chan struct{}),
    }
    b.cond = sync.NewCond(&b.mu)

    b.wg.Add(2)
    go b.timerLoop()
    go b.flusherLoop()
    return b
}

// Add вставляет item. Может block, drop, или return error в зависимости от config.
func (b *Batcher[T]) Add(item T) error {
    if b.closed.Load() {
        return errors.New("batcher closed")
    }

    b.mu.Lock()
    defer b.mu.Unlock()

    // Проверить hard cap
    if len(b.buffer) >= b.cfg.MaxBufferSize {
        switch b.cfg.OnFullPolicy {
        case BlockOnFull:
            for len(b.buffer) >= b.cfg.MaxBufferSize && !b.closed.Load() {
                b.cond.Wait()
            }
        case DropOldest:
            // Удалить первый
            b.buffer = b.buffer[1:]
            b.dropped.Add(1)
        case DropNewest:
            b.dropped.Add(1)
            return errors.New("buffer full")
        }
    }

    b.buffer = append(b.buffer, item)
    b.accepted.Add(1)

    // Sync trigger когда полный batch готов
    if len(b.buffer) >= b.cfg.MaxBatchSize && !b.flushing {
        b.kickFlush()
    }
    return nil
}

func (b *Batcher[T]) kickFlush() {
    // Called under lock
    if len(b.buffer) == 0 {
        return
    }
    batch := b.buffer
    b.buffer = make([]T, 0, b.cfg.MaxBatchSize)
    b.flushing = true

    select {
    case b.flushCh <- batch:
    default:
        // Channel full → revert (rare with small buffer)
        b.buffer = append(batch, b.buffer...)
        b.flushing = false
    }
    b.cond.Broadcast()  // разбудить блокированных Add'ы
}

func (b *Batcher[T]) timerLoop() {
    defer b.wg.Done()
    ticker := time.NewTicker(b.cfg.FlushInterval)
    defer ticker.Stop()

    for {
        select {
        case <-b.done:
            return
        case <-ticker.C:
            b.mu.Lock()
            if !b.flushing {
                b.kickFlush()
            }
            b.mu.Unlock()
        }
    }
}

func (b *Batcher[T]) flusherLoop() {
    defer b.wg.Done()

    for batch := range b.flushCh {
        b.flushBatchWithRetry(batch)

        b.mu.Lock()
        b.flushing = false
        // Если за время flush'а накопилось ещё — повторить
        if len(b.buffer) >= b.cfg.MaxBatchSize {
            b.kickFlush()
        }
        b.mu.Unlock()
    }
}

func (b *Batcher[T]) flushBatchWithRetry(batch []T) {
    ctx, cancel := context.WithTimeout(context.Background(), b.cfg.FlushTimeout)
    defer cancel()

    var err error
    for attempt := 1; attempt <= b.cfg.MaxRetries+1; attempt++ {
        err = b.flush(ctx, batch)
        if err == nil {
            b.flushed.Add(int64(len(batch)))
            return
        }
        b.log.Warn("flush failed",
            "attempt", attempt,
            "batch_size", len(batch),
            "err", err,
        )
        if attempt < b.cfg.MaxRetries+1 {
            backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
            time.Sleep(backoff)
        }
    }

    // Все retry истощены — dead letter
    b.failed.Add(int64(len(batch)))
    if b.onError != nil {
        b.onError(batch, err)
    } else {
        b.log.Error("batch dropped after all retries",
            "batch_size", len(batch),
            "err", err,
        )
    }
}

// Close дренирует buffer и закрывает batcher.
func (b *Batcher[T]) Close() error {
    if !b.closed.CompareAndSwap(false, true) {
        return errors.New("already closed")
    }

    close(b.done)

    // Final flush из buffer
    b.mu.Lock()
    if len(b.buffer) > 0 {
        batch := b.buffer
        b.buffer = nil
        b.mu.Unlock()
        b.flushCh <- batch
    } else {
        b.mu.Unlock()
    }

    close(b.flushCh)
    b.wg.Wait()
    return nil
}

type Stats struct {
    Accepted int64
    Flushed  int64
    Failed   int64
    Dropped  int64
}

func (b *Batcher[T]) Stats() Stats {
    return Stats{
        Accepted: b.accepted.Load(),
        Flushed:  b.flushed.Load(),
        Failed:   b.failed.Load(),
        Dropped:  b.dropped.Load(),
    }
}
```

**Что улучшено:**

### A. Separate flusher goroutine

Add → buffer (быстро). Flusher → реальный flush с retry. Add не блокирует пока flush идёт.

### B. Buffer hard cap + policy

```go
MaxBufferSize: 10000  // если 10x batch size накопилось — backpressure
```

Когда producer быстрее flusher'а — три варианта:
- **BlockOnFull** — Add блокирует. Хороший backpressure, но может deadlock'нуть caller.
- **DropOldest** — losing oldest events. Для metrics OK.
- **DropNewest** — return error. Caller сам решает что делать.

### C. Retry with backoff

Failed flush retry'ится. После max attempts — dead letter callback.

### D. Per-flush timeout

```go
ctx, cancel := context.WithTimeout(..., b.cfg.FlushTimeout)
```

Slow downstream не залипает flusher навсегда.

### E. Re-flush после long flush

Пока flush идёт, накапливаются новые items. После flush — проверить и снова flush если набралось.

### F. Метрики

Accepted, flushed, failed, dropped — для observability.

---

## Тесты

```go
func TestBatcher_FlushOnSize(t *testing.T) {
    var flushed [][]int
    var mu sync.Mutex

    flush := func(ctx context.Context, items []int) error {
        mu.Lock()
        flushed = append(flushed, append([]int{}, items...))
        mu.Unlock()
        return nil
    }

    b := New(Config{
        MaxBatchSize:  3,
        FlushInterval: time.Second,
        MaxBufferSize: 100,
        FlushTimeout:  time.Second,
        MaxRetries:    0,
    }, flush, nil, nil)
    defer b.Close()

    for i := 1; i <= 6; i++ {
        b.Add(i)
    }

    time.Sleep(100 * time.Millisecond)

    mu.Lock()
    defer mu.Unlock()
    if len(flushed) != 2 {
        t.Errorf("got %d batches, want 2", len(flushed))
    }
    if len(flushed[0]) != 3 || len(flushed[1]) != 3 {
        t.Errorf("batches sizes wrong: %v", flushed)
    }
}

func TestBatcher_FlushOnTimer(t *testing.T) {
    var flushed atomic.Int32
    flush := func(ctx context.Context, items []int) error {
        flushed.Add(int32(len(items)))
        return nil
    }

    b := New(Config{
        MaxBatchSize:  100,
        FlushInterval: 50 * time.Millisecond,
        MaxBufferSize: 1000,
        FlushTimeout:  time.Second,
    }, flush, nil, nil)
    defer b.Close()

    for i := 0; i < 5; i++ {
        b.Add(i)
    }

    time.Sleep(100 * time.Millisecond)

    if flushed.Load() != 5 {
        t.Errorf("flushed %d, want 5 (timer should flush partial)", flushed.Load())
    }
}

func TestBatcher_FlushOnClose(t *testing.T) {
    var flushed atomic.Int32
    flush := func(ctx context.Context, items []int) error {
        flushed.Add(int32(len(items)))
        return nil
    }

    b := New(Config{
        MaxBatchSize:  100,
        FlushInterval: 10 * time.Second,  // long interval
        MaxBufferSize: 1000,
        FlushTimeout:  time.Second,
    }, flush, nil, nil)

    for i := 0; i < 5; i++ {
        b.Add(i)
    }

    b.Close()

    if flushed.Load() != 5 {
        t.Errorf("after Close flushed %d, want 5", flushed.Load())
    }
}

func TestBatcher_Retry(t *testing.T) {
    var attempts atomic.Int32
    flush := func(ctx context.Context, items []int) error {
        n := attempts.Add(1)
        if n < 3 {
            return errors.New("transient")
        }
        return nil
    }

    b := New(Config{
        MaxBatchSize:  10,
        FlushInterval: time.Hour,
        MaxBufferSize: 100,
        FlushTimeout:  time.Second,
        MaxRetries:    5,
    }, flush, nil, nil)
    defer b.Close()

    for i := 0; i < 10; i++ {
        b.Add(i)
    }

    time.Sleep(500 * time.Millisecond)

    stats := b.Stats()
    if stats.Flushed != 10 {
        t.Errorf("flushed %d, want 10 (eventual success)", stats.Flushed)
    }
}

func TestBatcher_DeadLetterAfterRetries(t *testing.T) {
    var deadLettered atomic.Int32

    flush := func(ctx context.Context, items []int) error {
        return errors.New("permanent")
    }
    onError := func(items []int, err error) {
        deadLettered.Add(int32(len(items)))
    }

    b := New(Config{
        MaxBatchSize:  3,
        FlushInterval: time.Hour,
        MaxBufferSize: 100,
        FlushTimeout:  time.Second,
        MaxRetries:    2,
    }, flush, onError, nil)
    defer b.Close()

    for i := 0; i < 3; i++ {
        b.Add(i)
    }

    time.Sleep(500 * time.Millisecond)

    if deadLettered.Load() != 3 {
        t.Errorf("dead lettered %d, want 3", deadLettered.Load())
    }
}
```

---

## Подводные камни

### 1. Flush блокирует Add (нет separate flusher)

```go
func (b *Batcher) Add(x int) {
    b.mu.Lock()
    b.buffer = append(b.buffer, x)
    if len(b.buffer) >= max {
        b.flushNow()  // ← блокирует под lock'ом → Add'ы ждут
    }
    b.mu.Unlock()
}
```

Под нагрузкой Add'ы выстраиваются в очередь. Separate goroutine + buffered channel.

### 2. Flush медленный → buffer растёт неограниченно

Без `MaxBufferSize` — OOM при slow downstream.

### 3. Close не дренирует

```go
func (b *Batcher) Close() {
    close(b.done)  // ← но pending items в buffer!
}
```

Final flush обязателен. Или return error если can't flush.

### 4. Concurrent Close + Add

```go
go b.Close()
b.Add(x)  // ← после Close → send on closed → panic
```

`atomic.Bool` для closed flag + check в Add.

### 5. Timer Flush с пустым buffer

```go
case <-ticker.C:
    flushNow()  // ← если buffer пуст, делаем no-op flush
```

В целом OK (idempotent), но если flush делает network call — лишняя нагрузка. Skip if empty.

### 6. Lost items при panic в flush

```go
batch := b.buffer
b.buffer = nil
flush(batch)  // ← panic → items потеряны (уже не в buffer)
```

Recover в flusher или транзакционная семантика (revert при panic).

### 7. Concurrent flush race

```go
go flush(batch1)
// add'ы продолжают, buffer наполняется
go flush(batch2)  // ← concurrent flush!
```

Если flush — DB write — concurrent OK. Если есть ordering или resource limits — sequential.

### 8. Backpressure через block — deadlock risk

```go
// Caller блокирован на Add
b.Add(x)  // ← waits forever если flusher upstream'е блокирован
```

Используй Add с context.

### 9. Слишком маленький batch size

```go
MaxBatchSize: 10  // ← 10-item batch'и
```

Каждый batch — overhead (transaction, network round-trip). 100-1000 обычно лучше для DB inserts.

### 10. Слишком большой batch size

```go
MaxBatchSize: 100000
```

Big batch = long transaction = lock holds = блок других writes. Также memory spike. Balance.

---

## Возможные расширения

### 1. Multi-destination batching

Один batcher → router → разные destinations (per-tenant, per-region).

### 2. Adaptive batch size

Подстраивать размер под latency. Если flush медленный → меньше batches.

### 3. Persistent buffer

Если падает pod — items в buffer теряются. Решение: persistent queue (BadgerDB, kafka, файл).

### 4. Priority batching

Critical events flush'ятся сразу, обычные batch'ятся.

### 5. Compression в batch

Сжимать целый batch перед send (gzip, snappy). Bigger throughput.

### 6. Async ack to producer

Producer ждёт подтверждения что item обработан (batch flushed). Per-item Promise.

---

## Что важно показать на собеседовании

1. **Triple trigger:** size + time + manual signal
2. **Separate flusher goroutine** — Add не блокирует на flush
3. **Buffer hard cap + policy** (block/drop)
4. **Retry with backoff + dead letter**
5. **Per-flush timeout** через ctx
6. **Final flush в Close** — graceful drain
7. **Concurrent Add safe** — atomic flag, condition variable для backpressure
8. **Metrics** — accepted/flushed/failed/dropped

## Связки

- [Background Workers](../../../04-architecture-and-patterns/patterns/04-background-workers.md) — basis pattern
- [Retry with Backoff](../system-primitives/02-retry-with-backoff.md) — flush retry
- [Outbox pattern](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md) — batching outbox events
- [Kafka producer batching](../../../07-message-brokers-and-streaming/01-kafka.md)
