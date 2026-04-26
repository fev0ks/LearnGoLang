# Bulkhead

Bulkhead (переборка) — паттерн изоляции ресурсов по зависимостям. Называется по аналогии с водонепроницаемыми переборками в корабле: повреждение одного отсека не топит всё судно.

## Содержание

- [Проблема общего пула](#проблема-общего-пула)
- [Bulkhead через semaphore per dependency](#bulkhead-через-semaphore-per-dependency)
- [Connection pool как bulkhead](#connection-pool-как-bulkhead)
- [Thread pool isolation (goroutine pool)](#thread-pool-isolation-goroutine-pool)
- [Комбинация с circuit breaker](#комбинация-с-circuit-breaker)
- [Антипаттерны](#антипаттерны)

---

## Проблема общего пула

Без bulkhead все зависимости конкурируют за одни ресурсы:

```
Сервис: max 200 одновременных goroutines

PaymentService начал отвечать за 5s (вместо 200ms)

1. Запросы к PaymentService занимают goroutines, ждут 5s
2. Через несколько секунд все 200 goroutines заняты PaymentService
3. Запросы к UserService и NotificationService тоже не обрабатываются
4. Весь сервис деградирует из-за одной медленной зависимости
```

С bulkhead каждая зависимость имеет свой изолированный лимит:

```
PaymentService:      max 50 concurrent
UserService:         max 80 concurrent
NotificationService: max 30 concurrent
Прочие:              max 40 concurrent

Если PaymentService тормозит — его 50 goroutines заблокированы,
остальные 150 продолжают работать нормально
```

---

## Bulkhead через semaphore per dependency

```go
type Bulkheads struct {
    Payment      *semaphore.Weighted
    User         *semaphore.Weighted
    Notification *semaphore.Weighted
}

func NewBulkheads() *Bulkheads {
    return &Bulkheads{
        Payment:      semaphore.NewWeighted(50),
        User:         semaphore.NewWeighted(80),
        Notification: semaphore.NewWeighted(30),
    }
}

// Обёртка клиента с bulkhead
type PaymentClient struct {
    inner     PaymentServiceClient
    bulkhead  *semaphore.Weighted
    timeout   time.Duration
}

func (c *PaymentClient) Charge(ctx context.Context, req *ChargeRequest) (*ChargeResponse, error) {
    // Попытаться занять slot с таймаутом
    acquireCtx, cancel := context.WithTimeout(ctx, c.timeout)
    defer cancel()

    if err := c.bulkhead.Acquire(acquireCtx, 1); err != nil {
        // Все слоты заняты — быстрый отказ
        return nil, fmt.Errorf("payment bulkhead full: %w", ErrServiceOverloaded)
    }
    defer c.bulkhead.Release(1)

    return c.inner.Charge(ctx, req)
}
```

### С метриками

```go
func (c *PaymentClient) Charge(ctx context.Context, req *ChargeRequest) (*ChargeResponse, error) {
    // Текущая загрузка bulkhead
    current := c.bulkhead.TryAcquire(0)  // нет, TryAcquire(0) не работает так
    // Используем отдельный счётчик
    inFlight := atomic.AddInt64(&c.inFlight, 1)
    defer atomic.AddInt64(&c.inFlight, -1)

    metrics.BulkheadInFlight.WithLabelValues("payment").Set(float64(inFlight))

    if err := c.bulkhead.Acquire(ctx, 1); err != nil {
        metrics.BulkheadRejected.WithLabelValues("payment").Inc()
        return nil, ErrServiceOverloaded
    }
    defer c.bulkhead.Release(1)

    start := time.Now()
    resp, err := c.inner.Charge(ctx, req)
    metrics.BulkheadLatency.WithLabelValues("payment").Observe(time.Since(start).Seconds())
    return resp, err
}
```

---

## Connection pool как bulkhead

Database connection pool — встроенный bulkhead. `MaxConns` ограничивает параллелизм к БД:

```go
poolConfig, _ := pgxpool.ParseConfig(dsn)

// Bulkhead для PostgreSQL
poolConfig.MaxConns = 20          // max 20 одновременных запросов
poolConfig.MinConns = 5
poolConfig.MaxConnWaitingTime = 500 * time.Millisecond  // ждать свободный conn не более 500ms

// Если все 20 заняты — следующий запрос получит ошибку через 500ms
// вместо ожидания неопределённое время
pool, _ := pgxpool.NewWithConfig(ctx, poolConfig)
```

Отдельные пулы для разных операций:

```go
// Читальный пул — больше коннектов, идут на реплику
readPool, _ := pgxpool.New(ctx, replicaDSN+"?pool_max_conns=50")

// Писательный пул — меньше коннектов, идут на мастер
writePool, _ := pgxpool.New(ctx, masterDSN+"?pool_max_conns=10")
```

---

## Thread pool isolation (goroutine pool)

Для CPU-интенсивных задач или тяжёлых операций — отдельный goroutine pool per dependency:

```go
type BulkheadPool struct {
    jobs chan func()
    wg   sync.WaitGroup
}

func NewBulkheadPool(workers, queueSize int) *BulkheadPool {
    p := &BulkheadPool{
        jobs: make(chan func(), queueSize),
    }
    for i := 0; i < workers; i++ {
        p.wg.Add(1)
        go func() {
            defer p.wg.Done()
            for job := range p.jobs {
                job()
            }
        }()
    }
    return p
}

func (p *BulkheadPool) Submit(ctx context.Context, fn func()) error {
    select {
    case p.jobs <- fn:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        return ErrPoolFull
    }
}

func (p *BulkheadPool) Shutdown() {
    close(p.jobs)
    p.wg.Wait()
}

// Использование — тяжёлые PDF/image операции изолированы от бизнес-логики
var pdfPool = NewBulkheadPool(5, 20)   // max 5 параллельных PDF генераций
var reportPool = NewBulkheadPool(3, 10) // max 3 генерации отчётов

func GeneratePDF(ctx context.Context, data ReportData) error {
    resultCh := make(chan error, 1)
    err := pdfPool.Submit(ctx, func() {
        resultCh <- doGeneratePDF(data)
    })
    if err != nil {
        return fmt.Errorf("pdf pool full: %w", err)
    }
    select {
    case err := <-resultCh:
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

---

## Комбинация с circuit breaker

Bulkhead и circuit breaker дополняют друг друга:

```
Запрос → Circuit Breaker → Bulkhead → Downstream Service
           (open?)          (full?)
```

- **Circuit Breaker** отказывает быстро когда зависимость известно сломана
- **Bulkhead** ограничивает ресурсы когда зависимость медленная (но не сломана)

```go
type ResilientClient struct {
    inner    ServiceClient
    cb       *gobreaker.CircuitBreaker
    bulkhead *semaphore.Weighted
}

func (c *ResilientClient) Call(ctx context.Context, req *Request) (*Response, error) {
    result, err := c.cb.Execute(func() (interface{}, error) {
        // Внутри circuit breaker — проверить bulkhead
        if !c.bulkhead.TryAcquire(1) {
            // Bulkhead full — это тоже failure для circuit breaker
            return nil, ErrBulkheadFull
        }
        defer c.bulkhead.Release(1)

        return c.inner.Call(ctx, req)
    })
    if err != nil {
        if errors.Is(err, gobreaker.ErrOpenState) {
            return nil, ErrCircuitOpen
        }
        return nil, err
    }
    return result.(*Response), nil
}
```

---

## Антипаттерны

**Один глобальный semaphore на все зависимости** — это не bulkhead, это rate limiter. Медленный PaymentService всё равно вытеснит UserService.

**Слишком большой лимит** — если bulkhead на 1000 goroutines, а реальный bottleneck в 50 — bulkhead не защитит.

**Не мониторить заполненность** — bulkhead постоянно полный означает что либо лимит слишком низкий, либо downstream деградирует. Без метрики invisible.

**Таймаут на Acquire слишком большой** — если ждать слободный slot 10s, goroutines накапливаются в ожидании, и эффект как без bulkhead. Таймаут acquire должен быть короче expected latency + небольшой буфер.

```go
// Слишком долгий wait — goroutines накапливаются в очереди на acquire
acquireCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

// Разумный wait — быстрый fail если пул перегружен
acquireCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
```
