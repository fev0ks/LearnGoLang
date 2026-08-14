# Задача 2: Batching Writer

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Контракт](#контракт-компонента)
- [Реализация](#реализация)
- [Как работает реализация](#как-работает-реализация)
- [Тесты](#тесты)
- [Надёжность](#надёжность-и-delivery-semantics)
- [Типичные ошибки](#подводные-камни)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Batching writer объединяет отдельные элементы в более крупные запросы. Он
уменьшает число network round trips и транзакций, но добавляет задержку,
состояние в памяти и отдельный failure path при shutdown.

---

## Формулировка

> Дан непрерывный поток событий. Реализуй компонент, который отправляет batch
> при достижении `MaxBatchSize` или по `FlushInterval`. Ограничь память, задай
> backpressure policy и дренируй принятые элементы при graceful shutdown.

Типичные применения:

- bulk insert в БД;
- отправка telemetry и audit events;
- запись пачек в object storage;
- объединение запросов к внешнему API, если API поддерживает bulk operation.

S3 multipart upload — не просто «batching любых chunks»: у него есть собственные
ограничения на part size, число parts и завершение upload. Их нужно учитывать
отдельно от общей механики batcher.

---

## Уточняющие вопросы

1. **Что важнее: throughput или максимальная latency?**
   Маленький интервал снижает latency, но создаёт больше неполных batches.
2. **Можно ли терять данные?**
   Для metrics иногда допустим drop, для billing/audit чаще нужны block и
   durable queue.
3. **Что означает успешный `Add`?**
   Только приём в RAM или уже durable acceptance? В примере ниже — только RAM.
4. **Нужен ли порядок?**
   Один flusher сохраняет порядок принятых элементов; несколько flushers могут
   его нарушить.
5. **Какие ошибки retryable?**
   Timeout/temporary overload часто retryable, validation error — обычно нет.
6. **Как ограничить shutdown?**
   `Close(ctx)` должен позволять вызывающему коду прекратить ожидание, даже если
   downstream завис.

---

## Контракт компонента

Реализация ниже использует такие правила:

- `Add(ctx, item)` безопасен для concurrent вызовов;
- одновременно выполняется не более одного `flush`, поэтому порядок batches
  сохраняется;
- waiting buffer ограничен `MaxBufferSize`; ещё до `MaxBatchSize` элементов
  может находиться в выполняющемся flush;
- partial batch уходит по timer, полный — сразу;
- `Close(ctx)` запрещает новые `Add`, дренирует принятые элементы и идемпотентен;
- если context в `Close` завершился, drain продолжает выполняться в фоне, а
  повторный `Close` может дождаться результата;
- terminal flush error возвращается из `Close` и учитывается в метриках.

Это lifecycle-гарантии, а не durability-гарантии: crash процесса уничтожит
буфер в памяти.

---

## Реализация

```go
package batcher

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

var (
    ErrClosed      = errors.New("batcher: closed")
    ErrBufferFull  = errors.New("batcher: buffer full")
    ErrInvalidConfig = errors.New("batcher: invalid configuration")
)

type FlushFunc[T any] func(ctx context.Context, items []T) error

type FullPolicy int

const (
    BlockOnFull FullPolicy = iota
    DropOldest
    DropNewest
)

type Config struct {
    MaxBatchSize  int
    FlushInterval time.Duration
    MaxBufferSize int
    OnFullPolicy  FullPolicy
    FlushTimeout  time.Duration
    MaxRetries    int // retry после первой попытки
    RetryBackoff  time.Duration
}

type Batcher[T any] struct {
    cfg     Config
    flush   FlushFunc[T]
    onError func(items []T, err error) // notification, не durable DLQ

    mu             sync.Mutex
    cond           *sync.Cond
    buffer         []T
    flushing       bool
    inFlight       int
    flushRequested bool
    closed         bool

    flushCh    chan []T
    timerStop  chan struct{}
    timerDone  chan struct{}
    flusherDone chan struct{}

    closeOnce sync.Once
    closeDone chan struct{}
    errMu     sync.Mutex
    closeErr  error

    accepted atomic.Int64
    flushed  atomic.Int64
    failed   atomic.Int64
    dropped  atomic.Int64
}

func New[T any](
    cfg Config,
    flush FlushFunc[T],
    onError func([]T, error),
) (*Batcher[T], error) {
    validPolicy := cfg.OnFullPolicy >= BlockOnFull && cfg.OnFullPolicy <= DropNewest
    if flush == nil || cfg.MaxBatchSize <= 0 ||
        cfg.MaxBufferSize < cfg.MaxBatchSize ||
        cfg.FlushInterval <= 0 || cfg.FlushTimeout <= 0 ||
        cfg.MaxRetries < 0 || (cfg.MaxRetries > 0 && cfg.RetryBackoff <= 0) ||
        !validPolicy {
        return nil, ErrInvalidConfig
    }

    b := &Batcher[T]{
        cfg:         cfg,
        flush:       flush,
        onError:     onError,
        buffer:      make([]T, 0, cfg.MaxBatchSize),
        flushCh:     make(chan []T, 1),
        timerStop:   make(chan struct{}),
        timerDone:   make(chan struct{}),
        flusherDone: make(chan struct{}),
        closeDone:   make(chan struct{}),
    }
    b.cond = sync.NewCond(&b.mu)

    go b.timerLoop()
    go b.flusherLoop()
    return b, nil
}

func (b *Batcher[T]) Add(ctx context.Context, item T) error {
    if ctx == nil {
        return ErrInvalidConfig
    }

    b.mu.Lock()
    defer b.mu.Unlock()

    for len(b.buffer) >= b.cfg.MaxBufferSize {
        if b.closed {
            return ErrClosed
        }

        switch b.cfg.OnFullPolicy {
        case DropNewest:
            b.dropped.Add(1)
            return ErrBufferFull

        case DropOldest:
            var zero T
            copy(b.buffer, b.buffer[1:])
            b.buffer[len(b.buffer)-1] = zero
            b.buffer = b.buffer[:len(b.buffer)-1]
            b.dropped.Add(1)

        case BlockOnFull:
            if err := ctx.Err(); err != nil {
                return err
            }
            stopWakeup := context.AfterFunc(ctx, func() {
                b.mu.Lock()
                b.cond.Broadcast()
                b.mu.Unlock()
            })
            b.cond.Wait()
            stopWakeup()
        }
    }

    if b.closed {
        return ErrClosed
    }
    if err := ctx.Err(); err != nil {
        return err
    }

    b.buffer = append(b.buffer, item)
    b.accepted.Add(1)
    if len(b.buffer) >= b.cfg.MaxBatchSize {
        b.flushRequested = true
        b.kickLocked()
    }
    return nil
}

// kickLocked передаёт не более MaxBatchSize элементов единственному flusher.
// Метод вызывается только под b.mu.
func (b *Batcher[T]) kickLocked() {
    if b.flushing || len(b.buffer) == 0 {
        return
    }

    n := min(len(b.buffer), b.cfg.MaxBatchSize)
    batch := append([]T(nil), b.buffer[:n]...)

    copy(b.buffer, b.buffer[n:])
    clear(b.buffer[len(b.buffer)-n:])
    b.buffer = b.buffer[:len(b.buffer)-n]

    b.flushing = true
    b.inFlight = len(batch)
    b.flushRequested = false
    b.flushCh <- batch // capacity=1 и invariant !flushing делают send неблокирующим
    b.cond.Broadcast()
}

func (b *Batcher[T]) timerLoop() {
    ticker := time.NewTicker(b.cfg.FlushInterval)
    defer ticker.Stop()
    defer close(b.timerDone)

    for {
        select {
        case <-b.timerStop:
            return
        case <-ticker.C:
            b.mu.Lock()
            if !b.closed && len(b.buffer) > 0 {
                b.flushRequested = true
                b.kickLocked()
            }
            b.mu.Unlock()
        }
    }
}

func (b *Batcher[T]) flusherLoop() {
    defer close(b.flusherDone)

    for batch := range b.flushCh {
        if err := b.flushWithRetry(batch); err != nil {
            b.recordTerminalError(err)
            b.notifyFailure(batch, err)
        }

        b.mu.Lock()
        b.flushing = false
        b.inFlight = 0
        if len(b.buffer) > 0 &&
            (b.closed || b.flushRequested || len(b.buffer) >= b.cfg.MaxBatchSize) {
            b.kickLocked()
        }
        b.cond.Broadcast()
        b.mu.Unlock()
    }
}

func (b *Batcher[T]) flushWithRetry(batch []T) error {
    var lastErr error
    for attempt := 0; attempt <= b.cfg.MaxRetries; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), b.cfg.FlushTimeout)
        lastErr = callFlush(ctx, b.flush, batch)
        cancel()

        if lastErr == nil {
            b.flushed.Add(int64(len(batch)))
            return nil
        }
        if attempt < b.cfg.MaxRetries {
            // В production обычно добавляют jitter и классификацию retryable errors.
            time.Sleep(b.cfg.RetryBackoff * time.Duration(attempt+1))
        }
    }

    b.failed.Add(int64(len(batch)))
    return fmt.Errorf("flush %d items after retries: %w", len(batch), lastErr)
}

func callFlush[T any](
    ctx context.Context,
    flush FlushFunc[T],
    batch []T,
) (err error) {
    defer func() {
        if recovered := recover(); recovered != nil {
            err = fmt.Errorf("flush panic: %v", recovered)
        }
    }()
    return flush(ctx, batch)
}

func (b *Batcher[T]) notifyFailure(batch []T, flushErr error) {
    if b.onError == nil {
        return
    }
    // Callback не должен завершить служебную goroutine своим panic.
    defer func() {
        if recovered := recover(); recovered != nil {
            b.recordTerminalError(fmt.Errorf("onError panic: %v", recovered))
        }
    }()
    b.onError(batch, flushErr)
}

func (b *Batcher[T]) recordTerminalError(err error) {
    b.errMu.Lock()
    b.closeErr = errors.Join(b.closeErr, err)
    b.errMu.Unlock()
}

func (b *Batcher[T]) Close(ctx context.Context) error {
    if ctx == nil {
        return ErrInvalidConfig
    }
    b.closeOnce.Do(func() { go b.shutdown() })

    select {
    case <-b.closeDone:
        b.errMu.Lock()
        defer b.errMu.Unlock()
        return b.closeErr
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (b *Batcher[T]) shutdown() {
    b.mu.Lock()
    b.closed = true
    b.cond.Broadcast()
    b.mu.Unlock()

    close(b.timerStop)
    <-b.timerDone

    b.mu.Lock()
    if len(b.buffer) > 0 {
        b.flushRequested = true
        b.kickLocked()
    }
    for b.flushing || len(b.buffer) > 0 {
        if !b.flushing {
            b.kickLocked()
        }
        b.cond.Wait()
    }
    b.mu.Unlock()

    close(b.flushCh)
    <-b.flusherDone
    close(b.closeDone)
}

type Stats struct {
    Accepted int64
    Flushed  int64
    Failed   int64
    Dropped  int64
    Buffered int
    InFlight int
}

func (b *Batcher[T]) Stats() Stats {
    b.mu.Lock()
    defer b.mu.Unlock()
    return Stats{
        Accepted: b.accepted.Load(),
        Flushed:  b.flushed.Load(),
        Failed:   b.failed.Load(),
        Dropped:  b.dropped.Load(),
        Buffered: len(b.buffer),
        InFlight: b.inFlight,
    }
}
```

---

## Как работает реализация

### Один batch в полёте

`flushing` означает, что batch уже лежит в `flushCh` или выполняется. Следующий
batch не запускается параллельно. Это упрощает ordering и не позволяет медленному
downstream породить неограниченное число goroutines.

### Size и time triggers

Полный batch отправляется из `Add`. Timer отправляет partial batch. Если timer
срабатывает во время долгого flush, `flushRequested` запоминает trigger и
partial batch уходит сразу после текущего, а не ждёт ещё один интервал.

### Backpressure

- `BlockOnFull` ждёт свободное место и реагирует на cancellation context;
- `DropNewest` отклоняет новый элемент;
- `DropOldest` принимает новый элемент ценой самого старого waiting item.

Удаление первого элемента из slice стоит `O(n)`. Если `DropOldest` — основной
режим под высокой нагрузкой, waiting buffer лучше реализовать кольцом.

### Shutdown

Сначала `closed=true` под тем же mutex, который использует `Add`. Затем
останавливается timer, остаток делится на batches не больше `MaxBatchSize`, и
канал закрывается только после drain. Поэтому concurrent `Add`/`Close` не может
привести к `send on closed channel`.

---

## Тесты

Вместо `Sleep` для size/retry/shutdown тесты ждут явный сигнал. Только timer-тест
зависит от реального времени и имеет большой запас по deadline.

```go
func testConfig() Config {
    return Config{
        MaxBatchSize:  3,
        FlushInterval: time.Hour,
        MaxBufferSize: 9,
        OnFullPolicy:  BlockOnFull,
        FlushTimeout:  time.Second,
        MaxRetries:    0,
    }
}

func TestBatcher_FlushOnSize(t *testing.T) {
    got := make(chan []int, 1)
    b, err := New(testConfig(), func(_ context.Context, items []int) error {
        got <- append([]int(nil), items...)
        return nil
    }, nil)
    if err != nil {
        t.Fatal(err)
    }

    for _, item := range []int{1, 2, 3} {
        if err := b.Add(context.Background(), item); err != nil {
            t.Fatal(err)
        }
    }

    select {
    case batch := <-got:
        if !slices.Equal(batch, []int{1, 2, 3}) {
            t.Fatalf("batch = %v", batch)
        }
    case <-time.After(time.Second):
        t.Fatal("size flush did not happen")
    }

    if err := b.Close(context.Background()); err != nil {
        t.Fatal(err)
    }
}

func TestBatcher_FlushOnTimer(t *testing.T) {
    cfg := testConfig()
    cfg.FlushInterval = 20 * time.Millisecond
    got := make(chan []int, 1)

    b, err := New(cfg, func(_ context.Context, items []int) error {
        got <- append([]int(nil), items...)
        return nil
    }, nil)
    if err != nil {
        t.Fatal(err)
    }
    if err := b.Add(context.Background(), 1); err != nil {
        t.Fatal(err)
    }

    select {
    case batch := <-got:
        if !slices.Equal(batch, []int{1}) {
            t.Fatalf("batch = %v", batch)
        }
    case <-time.After(time.Second):
        t.Fatal("timer flush did not happen")
    }
    if err := b.Close(context.Background()); err != nil {
        t.Fatal(err)
    }
}

func TestBatcher_CloseDrainsPartialBatch(t *testing.T) {
    var flushed atomic.Int64
    b, err := New(testConfig(), func(_ context.Context, items []int) error {
        flushed.Add(int64(len(items)))
        return nil
    }, nil)
    if err != nil {
        t.Fatal(err)
    }

    for _, item := range []int{1, 2} {
        if err := b.Add(context.Background(), item); err != nil {
            t.Fatal(err)
        }
    }
    if err := b.Close(context.Background()); err != nil {
        t.Fatal(err)
    }
    if got := flushed.Load(); got != 2 {
        t.Fatalf("flushed = %d, want 2", got)
    }
    if err := b.Add(context.Background(), 3); !errors.Is(err, ErrClosed) {
        t.Fatalf("Add after Close error = %v", err)
    }
}

func TestBatcher_RetryAndCloseError(t *testing.T) {
    cfg := testConfig()
    cfg.MaxRetries = 2
    cfg.RetryBackoff = time.Millisecond

    var attempts atomic.Int64
    b, err := New(cfg, func(_ context.Context, _ []int) error {
        if attempts.Add(1) < 3 {
            return errors.New("temporary")
        }
        return nil
    }, nil)
    if err != nil {
        t.Fatal(err)
    }
    for _, item := range []int{1, 2, 3} {
        if err := b.Add(context.Background(), item); err != nil {
            t.Fatal(err)
        }
    }
    if err := b.Close(context.Background()); err != nil {
        t.Fatal(err)
    }
    if attempts.Load() != 3 || b.Stats().Flushed != 3 {
        t.Fatalf("attempts=%d stats=%+v", attempts.Load(), b.Stats())
    }
}
```

В реальном пакете дополнительно нужен stress-test concurrent `Add`/`Close` под
`go test -race`, а также тесты всех трёх full policies и terminal error.

---

## Надёжность и delivery semantics

Успешный `Add` означает только «элемент принят в память». Возможны такие исходы:

| Событие | Результат |
|---|---|
| Graceful `Close` и успешный downstream | pending элементы отправлены |
| Terminal flush error | `Close` возвращает ошибку, `onError` получает batch |
| `Close` timeout | caller перестаёт ждать, drain продолжается |
| Process crash / OOM / `SIGKILL` | элементы в RAM теряются |
| Неидемпотентный retry | downstream может получить дубль |

Если требуется at-least-once после crash, перед подтверждением producer нужен
durable log/queue. Если retry может повторить уже применённый запрос, downstream
должен принимать idempotency key либо выполнять idempotent upsert. Callback
`onError` в примере — observability hook, а не durable dead-letter queue.

---

## Подводные камни

### 1. Flush под mutex

Network/DB call под lock блокирует все `Add`. Под lock нужно только отделить
batch, а I/O выполнять снаружи.

### 2. Неограниченный async flush

`go flush(batch)` на каждый trigger скрывает backpressure, нарушает ordering и
может создать тысячи goroutines. Число параллельных flushers должно быть явно
ограничено.

### 3. Закрытие канала одновременно с `Add`

Atomic `closed` check сам по себе не решает race: `Add` может пройти проверку, а
`Close` успеет закрыть канал перед send. Переход в closed и приём элемента должны
сериализоваться одним протоколом — здесь это общий mutex.

### 4. Final flush без ожидания

Если `Close` только посылает сигнал и возвращает, caller может завершить процесс
раньше I/O. Нужен completion signal и результат drain.

### 5. Один timeout на все retry

Context, истёкший на первой попытке, делает все последующие попытки бесполезными.
В примере каждая попытка получает отдельный timeout. Если нужен общий deadline,
это должен быть отдельный явно описанный budget.

### 6. Retry любой ошибки

Retry validation/permission error увеличивает нагрузку без шанса на успех.
Production-реализация классифицирует ошибки и добавляет exponential backoff с
jitter.

### 7. Удержание backing array

После удаления элементов ссылки нужно обнулить, иначе GC может удерживать
крупные объекты. Код использует `clear` перед уменьшением slice.

### 8. Неверная оценка capacity

Размер waiting buffer выбирают не как «10 batches на глаз». Приближённо:

```text
backlog ≈ max(0, arrival_rate - flush_rate) × tolerated_degradation_time
```

Если средний arrival rate стабильно выше flush rate, конечный buffer только
откладывает drop/block — систему нужно масштабировать или снижать входной поток.

---

## Interview-ready answer

**1. Какие triggers нужны batcher?**

- Size — обеспечивает throughput при высокой нагрузке.
- Time — ограничивает latency при редком потоке.
- Shutdown — дренирует partial batch при штатном завершении.

**2. Как не получить race между `Add` и `Close`?**

- Сериализация — одним lock защищать closed state и приём в waiting buffer.
- Timer — остановить до закрытия внутреннего канала.
- Завершение — закрывать канал только после drain и ждать flusher completion.

**3. Как выбрать full policy?**

- Block — когда потери запрещены и upstream умеет пережить backpressure.
- Drop newest/oldest — только когда потеря явно входит в контракт.
- Durable queue — когда принятые данные должны переживать process crash.

**4. Что проверить у retry?**

- Идемпотентность — проверить downstream и классификацию ошибок.
- Budget — задать timeout каждой попытки, общий предел, backoff и jitter.
- Observability — считать attempts/failures/dropped и проверять dead-letter path.

---

## Связанные материалы

- [Background Workers](../../../04-architecture-and-patterns/patterns/04-background-workers.md)
- [Retry with Backoff](../system-primitives/02-retry-with-backoff.md)
- [Outbox pattern](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md)
- [Kafka](../../../07-message-brokers-and-streaming/01-kafka.md)
