# Background Workers и Job Orchestration

## Содержание

- [Типы фоновых задач](#типы-фоновых-задач)
- [Worker pool](#worker-pool)
- [Graceful shutdown](#graceful-shutdown)
- [Периодические задачи](#периодические-задачи)
- [Distributed lease: один воркер в кластере](#distributed-lease-один-воркер-в-кластере)
- [Идемпотентные воркеры](#идемпотентные-воркеры)
- [Backpressure и ограничение параллелизма](#backpressure-и-ограничение-параллелизма)
- [Наблюдаемость воркеров](#наблюдаемость-воркеров)
- [Interview-ready answer](#interview-ready-answer)

Фоновый воркер отличается от обработчика HTTP-запроса тем, что у него нет клиента, который заметит проблему. Никто не ждёт ответа, никто не увидит ошибку 500, и сбой обнаруживается по косвенным признакам: отчёт не пришёл, письмо не отправлено, статус заказа завис.

Отсюда четыре темы, которые в фоновой обработке приходится решать явно: корректная остановка при деплое, ограничение параллелизма, единственность периодической задачи в кластере из нескольких реплик и устойчивость к повторной обработке.

---

## Типы фоновых задач

| Тип | Описание | Пример |
|---|---|---|
| One-shot | Выполнить один раз и завершить | Миграция данных |
| Periodic | Запускаться по расписанию | Отчёт раз в сутки |
| Queue consumer | Читать задачи из очереди | Обработка заказов |
| Reconciler | Периодически сверять состояние | Синхронизация с внешним API |
| Event listener | Реагировать на события из брокера | Kafka consumer |
| Scheduled at-time | Запустить в определённое время | Напоминание пользователю |

Различие между reconciler и event listener важнее, чем кажется. Слушатель событий реагирует на факт изменения и ломается, если событие потеряно. Reconciler сравнивает желаемое состояние с фактическим и на следующем цикле исправляет расхождение сам — потеря события для него не событие вовсе. Подробнее — в [Architecture Patterns](./02-architecture-patterns.md).

---

## Worker pool

Ограниченный пул воркеров защищает сервис от собственной нагрузки: одновременно выполняется не больше `workers` задач, ещё `queueSize` задач помещаются в канал, а после заполнения канала `Submit` ждёт свободного места.

```go
var ErrPoolClosed = errors.New("worker pool is closed")

type Handler func(ctx context.Context, job Job)

type WorkerPool struct {
    jobs   chan Job
    handle Handler

    // Контекст выполнения отделён от контекста, которым Submit ограничивает
    // ожидание места в очереди. При обычном shutdown он остаётся активным,
    // чтобы воркеры успели обработать уже принятые задачи.
    workCtx    context.Context
    cancelWork context.CancelFunc

    // mu защищает только короткое изменение состояния жизненного цикла.
    // Во время блокирующей отправки в jobs этот mutex не удерживается.
    mu        sync.Mutex
    accepting bool
    submitWG  sync.WaitGroup

    stopping chan struct{} // закрытие разблокирует ожидающие Submit
    stopOnce sync.Once
    workerWG sync.WaitGroup
    done     chan struct{} // закрывается после завершения всех воркеров
}

func NewWorkerPool(workers, queueSize int, handle Handler) (*WorkerPool, error) {
    if workers <= 0 {
        return nil, fmt.Errorf("workers must be positive: %d", workers)
    }
    if queueSize < 0 {
        return nil, fmt.Errorf("queue size must not be negative: %d", queueSize)
    }
    if handle == nil {
        return nil, errors.New("worker pool handler is nil")
    }

    workCtx, cancelWork := context.WithCancel(context.Background())
    p := &WorkerPool{
        jobs:       make(chan Job, queueSize),
        handle:     handle,
        workCtx:    workCtx,
        cancelWork: cancelWork,
        accepting:  true,
        stopping:   make(chan struct{}),
        done:       make(chan struct{}),
    }

    p.workerWG.Add(workers)
    for i := 0; i < workers; i++ {
        go p.worker()
    }
    return p, nil
}

func (p *WorkerPool) worker() {
    defer p.workerWG.Done()

    // При обычном shutdown jobs закрывается только после завершения Submit.
    // range дочитывает буфер и выходит после его опустошения.
    for job := range p.jobs {
        // cancelWork вызывается, только если истёк бюджет graceful shutdown.
        // После этого оставшиеся в памяти задачи намеренно не запускаются.
        if p.workCtx.Err() != nil {
            return
        }
        p.handle(p.workCtx, job)
    }
}

func (p *WorkerPool) Submit(ctx context.Context, job Job) error {
    p.mu.Lock()
    if !p.accepting {
        p.mu.Unlock()
        return ErrPoolClosed
    }
    // Add выполняется под тем же mutex, под которым shutdown запрещает новые
    // Submit. Поэтому к моменту submitWG.Wait новые Add уже невозможны.
    p.submitWG.Add(1)
    p.mu.Unlock()
    defer p.submitWG.Done()

    select {
    case p.jobs <- job:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    case <-p.stopping:
        return ErrPoolClosed
    }
}

func (p *WorkerPool) beginShutdown() {
    p.stopOnce.Do(func() {
        p.mu.Lock()
        p.accepting = false
        close(p.stopping)
        p.mu.Unlock()

        go func() {
            // Сначала завершаются все Submit, зарегистрированные до остановки.
            // Лишь после этого jobs можно закрыть без send on closed channel.
            p.submitWG.Wait()
            close(p.jobs)

            p.workerWG.Wait()
            p.cancelWork() // освобождает ресурсы context после обычного drain
            close(p.done)
        }()
    })
}

func (p *WorkerPool) Shutdown(ctx context.Context) error {
    p.beginShutdown()

    select {
    case <-p.done:
        return nil
    case <-ctx.Done():
        // Graceful-период закончился: просим текущие handler завершиться и
        // больше не запускаем задачи, оставшиеся в очереди. Произвольно
        // зависшую функцию Go остановить принудительно нельзя, поэтому handler
        // обязан соблюдать отмену контекста и ставить timeout внешним вызовам.
        p.cancelWork()
        return fmt.Errorf("worker pool shutdown: %w", ctx.Err())
    }
}
```

Жизненный цикл остановки состоит из четырёх последовательных фаз:

1. `accepting = false` запрещает регистрировать новые отправки.
2. Закрытие `stopping` будит вызовы `Submit`, которые уже ждут места в заполненной очереди.
3. `submitWG.Wait()` дожидается всех ранее зарегистрированных отправок, после чего пул получает исключительное право закрыть `jobs`.
4. Воркеры дочитывают закрытый канал, завершаются, и общий `done` фиксирует окончание drain.

**Почему нельзя держать mutex во время отправки.** Если очередь заполнена, `p.jobs <- job` блокируется. Если в этот момент `Submit` удерживает `RLock`, а `Shutdown` ждёт `Lock`, shutdown не доберётся до проверки своего контекста и его timeout не сработает. Здесь mutex защищает только проверку `accepting` и регистрацию в `submitWG`, а сама отправка происходит уже без него.

**Почему контекст `Submit` не передаётся в `handle`.** У этих контекстов разное время жизни. Контекст `Submit` ограничивает только ожидание места в очереди: HTTP-клиент может отключиться сразу после успешной постановки задачи, но фоновая работа от этого не должна отменяться. Контекст выполнения принадлежит пулу; deadline конкретной задачи создаётся внутри `handle` от `workCtx`.

**Что происходит по deadline `Shutdown`.** До deadline пул пытается обработать всю принятую очередь. После deadline вызывается `cancelWork`: текущие обработчики получают отмену, а новые задачи из очереди больше не запускаются. Это уже принудительная остановка с возможной потерей задач из памяти, поэтому надёжные задания хранят во внешней очереди и подтверждают только после успешной обработки.

**Почему повторный `Shutdown` безопасен.** `stopOnce` запускает остановку ровно один раз, а каждый вызов ждёт один и тот же `done`. Флаг «больше не принимаем» не считается признаком полного завершения: `nil` возвращается только после выхода всех воркеров.

**Выбор размера пула:**

| Тип задачи | Рекомендация |
|---|---|
| CPU-bound | `runtime.NumCPU()` или `runtime.NumCPU() + 1` |
| IO-bound (БД, HTTP) | По ёмкости получателя, обычно десятки-сотни |
| Смешанная | Измерить; отправная точка — `runtime.NumCPU() * 4` |

Числа во второй строке берутся не из воздуха. Для задач, упирающихся в базу, потолок задаёт пул соединений: тридцать воркеров при `MaxOpenConns=10` не дают тройного ускорения — двадцать из них стоят в очереди за соединением, а нагрузка на планировщик и память растёт. Разумная отправная точка — размер пула соединений; дальше значение подбирается по замеру пропускной способности, а не по интуиции.

Размер буфера канала (`queueSize`) отвечает за другое — за то, сколько задач допустимо принять в память до того, как `Submit` начнёт блокироваться. Нулевой буфер означает, что отправитель ждёт свободного воркера; это самый честный вариант и хорошая настройка по умолчанию. Буфер сглаживает короткие всплески, но большой буфер лишь маскирует нехватку воркеров: очередь растёт, задачи стареют, а при падении процесса всё принятое в память теряется.

---

## Graceful shutdown

Корректная остановка проходит в две фазы. Контекст от `signal.NotifyContext` ловит SIGTERM/SIGINT и инициирует shutdown, но не передаётся напрямую в уже принятые задачи: иначе вся очередь сразу получит отмену вместо graceful drain. Для `Shutdown` создаётся отдельный контекст с timeout. До его истечения пул дочитывает очередь, а после истечения отменяет контекст выполнения; обработчики обязаны реагировать на эту отмену. `sync.WaitGroup` и общий канал `done` фиксируют полное завершение.

Подробно: паттерны, оркестрация нескольких компонентов (HTTP + gRPC + workers), таймауты, частые ошибки — в [08. Graceful Shutdown](./08-graceful-shutdown.md).

---

## Периодические задачи

```go
// Простой ticker-based job
func RunPeriodic(ctx context.Context, interval time.Duration, fn func(ctx context.Context) error, log *slog.Logger) {
    if interval <= 0 {
        log.Error("periodic job interval must be positive", "interval", interval)
        return
    }
    if ctx.Err() != nil {
        return // не запускаем первый вызов с уже отменённым контекстом
    }

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    // Запустить сразу при старте, не ждать первого тика
    if err := fn(ctx); err != nil {
        log.Error("job error", "err", err)
    }

    for {
        select {
        case <-ticker.C:
            if err := fn(ctx); err != nil {
                log.Error("job error", "err", err)
                // не прекращать — продолжить по расписанию
            }
        case <-ctx.Done():
            return
        }
    }
}

// Использование
go RunPeriodic(ctx, 5*time.Minute, reconciler.Reconcile, logger)
go RunPeriodic(ctx, 1*time.Hour, reporter.GenerateHourly, logger)
```

У такого цикла есть два свойства, о которых стоит знать заранее.

**Тики не накапливаются в неограниченную очередь.** Публичный контракт `time.Ticker` разрешает runtime скорректировать интервал или отбросить тики, если получатель читает канал слишком медленно. Поэтому отставание не превращается в лавину, но и рассчитывать на «ровно 12 запусков в час» нельзя: при `interval = 5 минут` и обработке в 7 минут следующий вызов начинается только после предыдущего, а пропущенные запуски не воспроизводятся один за другим.

Утверждение «канал ticker имеет буфер размером один» верно только для старой реализации. Начиная с Go 1.23 каналы таймеров имеют синхронную семантику и наблюдаемую ёмкость `0`. Код не должен зависеть ни от `cap(ticker.C)`, ни от `len(ticker.C)`; нужное поведение задаётся через `select`.

**Все реплики стартуют синхронно.** После общего деплоя поды поднимаются почти одновременно, немедленный первый вызов `fn(ctx)` происходит у всех сразу, и дальше тики совпадают. Получается регулярный всплеск нагрузки на базу или внешний API от всех реплик разом. Лечится случайной задержкой перед началом цикла:

```go
// Разводим реплики по времени: случайный сдвиг в диапазоне 0..interval.
// rand — это math/rand/v2 (Go 1.22+), глобальный источник в нём
// уже безопасен для одновременного использования и не требует Seed.
select {
case <-time.After(time.Duration(rand.Int64N(int64(interval)))):
case <-ctx.Done():
    return
}
```

Для задачи, которая и так защищена распределённой блокировкой (следующий раздел), сдвиг менее важен: лишние реплики просто не получат блокировку. Без блокировки jitter полезен, когда синхронный запуск реплик создаёт заметный всплеск; добавлять его автоматически к любой периодической задаче не требуется.

**Cron-библиотека для сложных расписаний:**

```go
// github.com/robfig/cron/v3
func StartCron(ctx context.Context, log *slog.Logger) (*cron.Cron, error) {
    c := cron.New(
        cron.WithSeconds(),
        cron.WithChain(
            // Не запускаем вторую копию той же job, если первая ещё работает.
            // DelayIfStillRunning — альтернатива, если запуск нельзя пропустить.
            cron.SkipIfStillRunning(cron.DefaultLogger),
            // По умолчанию panic внутри cron job завершает весь процесс.
            cron.Recover(cron.DefaultLogger),
        ),
    )

    // WithSeconds требует шесть полей: second, minute, hour, dom, month, dow.
    if _, err := c.AddFunc("0 0 * * * *", func() { // каждый час
        if err := generateReport(ctx); err != nil {
            log.Error("report failed", "err", err)
        }
    }); err != nil {
        return nil, fmt.Errorf("register hourly report: %w", err)
    }

    if _, err := c.AddFunc("*/30 * * * * *", func() { // каждые 30 секунд
        if err := reconcile(ctx); err != nil {
            log.Error("reconcile failed", "err", err)
        }
    }); err != nil {
        return nil, fmt.Errorf("register reconciler: %w", err)
    }

    c.Start()
    return c, nil
}

func StopCron(ctx context.Context, c *cron.Cron) error {
    // Stop запрещает новые запуски и возвращает context, который завершится
    // после выхода уже работающих job.
    stopped := c.Stop()
    select {
    case <-stopped.Done():
        return nil
    case <-ctx.Done():
        return fmt.Errorf("stop cron: %w", ctx.Err())
    }
}
```

По умолчанию `robfig/cron` запускает каждый вызов в отдельной goroutine, поэтому долгая job может пересечься со следующим запуском. В примере явно выбрана политика `SkipIfStillRunning`; для задач, где пропуск недопустим, нужен `DelayIfStillRunning` или внешняя надёжная очередь. Для расписаний, привязанных к локальному времени, также нужно явно выбрать часовой пояс через `cron.WithLocation` или префикс `CRON_TZ`.

---

## Distributed lease: один воркер в кластере

**Проблема:** у сервиса три реплики. Периодическая задача должна выполняться только на одной из них, иначе отчёт сформируется трижды, а письмо уйдёт клиенту три раза.

Готового «главного» узла в такой схеме нет: реплики равноправны и друг о друге не знают. Роль арбитра берёт на себя внешнее хранилище, у которого есть атомарная операция «занять, если свободно».

```go
var ErrLockLost = errors.New("distributed lock lost")

type DistributedLock struct {
    redis *redis.Client
    key   string
    ttl   time.Duration
}

// Lease описывает одно конкретное владение блокировкой.
// Новый TryAcquire создаёт новый token, даже если его вызывает та же реплика.
type Lease struct {
    lock  *DistributedLock
    token string
}

func NewDistributedLock(
    client *redis.Client,
    key string,
    ttl time.Duration,
) (*DistributedLock, error) {
    if client == nil {
        return nil, errors.New("redis client is nil")
    }
    if key == "" {
        return nil, errors.New("lock key is empty")
    }
    // Это механический минимум для показанной политики TTL/3 и TTL/6.
    // В production TTL выбирают с запасом относительно latency Redis,
    // длительности сетевых сбоев и пауз процесса.
    if ttl < time.Second {
        return nil, fmt.Errorf("lock TTL is too small: %s", ttl)
    }
    return &DistributedLock{redis: client, key: key, ttl: ttl}, nil
}

// TryAcquire занимает ключ, если он свободен.
// SET key token NX с TTL атомарен: одновременную попытку выигрывает
// ровно одна реплика. nil, nil означает, что ключ уже занят.
func (l *DistributedLock) TryAcquire(ctx context.Context) (*Lease, error) {
    var rawToken [16]byte
    if _, err := cryptorand.Read(rawToken[:]); err != nil {
        return nil, fmt.Errorf("generate lock token: %w", err)
    }
    token := hex.EncodeToString(rawToken[:])

    acquired, err := l.redis.SetNX(ctx, l.key, token, l.ttl).Result()
    if err != nil {
        return nil, fmt.Errorf("acquire lock: %w", err)
    }
    if !acquired {
        return nil, nil
    }
    return &Lease{lock: l, token: token}, nil
}

// Renew продлевает TTL, но только если ключ всё ещё наш.
// Проверка и продление обязаны быть одной операцией, поэтому Lua-скрипт:
// между отдельными GET и PEXPIRE ключ мог протухнуть и достаться другому,
// и тогда PEXPIRE продлил бы чужую блокировку.
func (l *Lease) Renew(ctx context.Context) error {
    const script = `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("PEXPIRE", KEYS[1], ARGV[2])
        end
        return 0
    `
    res, err := l.lock.redis.Eval(ctx, script,
        []string{l.lock.key}, l.token, l.lock.ttl.Milliseconds()).Int()
    if err != nil {
        return fmt.Errorf("renew lock: %w", err)
    }
    if res == 0 {
        return ErrLockLost
    }
    return nil
}

// Release снимает блокировку, тоже с проверкой владельца:
// иначе задача, затянувшаяся дольше TTL, удалит чужой ключ.
func (l *Lease) Release(ctx context.Context) error {
    const script = `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("DEL", KEYS[1])
        end
        return 0
    `
    res, err := l.lock.redis.Eval(ctx, script,
        []string{l.lock.key}, l.token).Int()
    if err != nil {
        return fmt.Errorf("release lock: %w", err)
    }
    if res == 0 {
        return ErrLockLost
    }
    return nil
}
```

Продление TTL нужно ровно на время выполнения задачи, поэтому цикл продления живёт внутри одного запуска, а не рядом с ним:

```go
func RunWithLease(
    ctx context.Context,
    lock *DistributedLock,
    interval time.Duration,
    fn func(ctx context.Context) error,
    log *slog.Logger,
) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            lease, err := lock.TryAcquire(ctx)
            if err != nil {
                log.Error("acquire lock", "err", err)
                continue
            }
            if lease == nil {
                continue // блокировку держит другая реплика
            }
            runOnce(ctx, lease, fn, log)

        case <-ctx.Done():
            return
        }
    }
}

func runOnce(
    ctx context.Context,
    lease *Lease,
    fn func(ctx context.Context) error,
    log *slog.Logger,
) {
    // Контекст задачи отменяется, если блокировка потеряна:
    // продолжать работу без неё нельзя — её уже мог взять кто-то другой.
    jobCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        // Первая попытка начинается через TTL/3, а её timeout равен TTL/6.
        // Если владение не удалось подтвердить, остаётся примерно половина TTL,
        // чтобы отмена дошла до основной работы до истечения lease.
        t := time.NewTicker(lease.lock.ttl / 3)
        defer t.Stop()
        for {
            select {
            case <-t.C:
                if jobCtx.Err() != nil {
                    return
                }
                renewCtx, renewCancel := context.WithTimeout(
                    jobCtx, lease.lock.ttl/6)
                err := lease.Renew(renewCtx)
                renewCancel()
                if err != nil {
                    if jobCtx.Err() != nil {
                        return
                    }
                    // Fail closed: timeout не позволяет отличить «renew не
                    // выполнился» от «выполнился, но ответ потерялся».
                    log.Warn("lock ownership is unconfirmed, cancelling job",
                        "err", err)
                    cancel()
                    return
                }
            case <-jobCtx.Done():
                return
            }
        }
    }()

    if err := fn(jobCtx); err != nil {
        log.Error("job error", "err", err)
    }
    cancel()
    wg.Wait()

    // Release с новым контекстом: jobCtx уже отменён,
    // а запрос в Redis всё ещё нужно выполнить.
    relCtx, relCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
    defer relCancel()
    if err := lease.Release(relCtx); err != nil {
        log.Warn("release lock", "err", err) // не критично: ключ протухнет по TTL
    }
}
```

В примере выбрана политика fail-closed: любая неподтверждённая попытка продления отменяет задачу. Это может остановить работу при кратком сетевом сбое, даже если Redis успел продлить TTL, зато код не продолжает работу, когда наличие lease неизвестно. Ограниченные повторные попытки тоже возможны, но они должны завершиться раньше локального срока безопасности, а не продолжаться бесконечно.

**Чего такая блокировка не гарантирует.** Redis-блокировка ограничивает время, а не доступ. Реплика может «зависнуть» — попасть под долгую паузу GC, потерять сеть, быть остановленной планировщиком — и очнуться уже после истечения TTL, когда ключ занят другой репликой. Формально блокировки у неё больше нет, но код об этом ещё не знает и продолжает писать в базу.

Отмена контекста при потере блокировки, как в примере, сокращает окно, но не закрывает его: между проверкой и записью всегда есть промежуток. Полностью проблему решает только защита на стороне получателя записи — сравнение номера владения (`fencing token`) или условное обновление по версии строки. Практический вывод простой: распределённая блокировка годится как средство от дублирования работы, но не как замена идемпотентности.

Есть и отдельный риск Redis failover. Запись ключа могла не успеть попасть с primary на replica; после повышения replica второй клиент успешно захватит ту же блокировку, пока первый ещё работает. Поэтому пример с одним экземпляром Redis подходит только там, где редкий параллельный запуск допустим и бизнес-операция дополнительно защищена идемпотентностью или fencing token. Если взаимное исключение требуется сохранять при отказах координатора, нужен механизм с более сильной моделью согласованности.

**Альтернатива через PostgreSQL Advisory Locks:**

```go
func withAdvisoryLock(
    ctx context.Context,
    db *pgxpool.Pool,
    lockID int64,
    fn func(ctx context.Context) error,
    log *slog.Logger,
) (retErr error) {
    // Блокировка живёт в рамках соединения, поэтому соединение
    // берётся из пула явно и удерживается до конца работы.
    conn, err := db.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("acquire conn: %w", err)
    }
    releaseToPool := true
    defer func() {
        if releaseToPool {
            conn.Release()
        }
    }()

    // Если состояние session-level lock неизвестно, соединение нельзя
    // возвращать в pool. Hijack удаляет его из pool, а Close завершает сессию
    // PostgreSQL и освобождает все оставшиеся session-level locks.
    discardConn := func() {
        if !releaseToPool {
            return
        }
        rawConn := conn.Hijack()
        releaseToPool = false

        closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := rawConn.Close(closeCtx); err != nil {
            log.Error("close connection with uncertain advisory lock",
                "err", err, "lock_id", lockID)
        }
    }

    var acquired bool
    if err := conn.QueryRow(ctx,
        "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
        // Сервер мог захватить lock, а клиент — не получить ответ.
        // Закрытие сессии снимает эту неопределённость.
        discardConn()
        return fmt.Errorf("try advisory lock: %w", err)
    }
    if !acquired {
        return nil // блокировку держит другая реплика, пропускаем цикл
    }

    // Снятие — с собственным контекстом: основной может быть уже отменён.
    // QueryRow нужен потому, что pg_advisory_unlock возвращает boolean.
    defer func() {
        unlockCtx, cancel := context.WithTimeout(
            context.WithoutCancel(ctx), 5*time.Second)
        defer cancel()

        var unlocked bool
        err := conn.QueryRow(unlockCtx,
            "SELECT pg_advisory_unlock($1)", lockID).Scan(&unlocked)
        if err != nil {
            discardConn()
            retErr = errors.Join(retErr,
                fmt.Errorf("advisory unlock %d: %w", lockID, err))
            return
        }
        if !unlocked {
            // Мы ожидаем ровно одно успешное получение и одно освобождение.
            // false означает нарушение этого инварианта.
            discardConn()
            retErr = errors.Join(retErr,
                fmt.Errorf("advisory lock %d was not held", lockID))
        }
    }()

    return fn(ctx)
}
```

Разница между двумя вариантами не в удобстве, а в модели отказа.

| | Redis `SET NX` с TTL | `pg_try_advisory_lock` |
|---|---|---|
| Что освобождает блокировку | Истечение TTL | Закрытие сессии PostgreSQL |
| Реплика зависла | Блокировка уйдёт другому через TTL | Блокировка держится, пока жива сессия |
| Реплика умерла | Ждём TTL | Освобождается сразу, как сервер заметит разрыв |
| Нужна ли отдельная система | Да, Redis | Нет, если база уже есть |
| Главный риск | Работа двоих после истечения TTL | Зависший процесс блокирует задачу надолго |

Advisory lock точнее в определении «узел умер» — за этим следит сам PostgreSQL. Но с пулом соединений с ним нужно обращаться аккуратно: блокировка привязана к сессии, а не к транзакции, поэтому соединение приходится удерживать явно и возвращать в пул только после подтверждённого снятия.

Session-level advisory locks реентерабельны: повторный `pg_try_advisory_lock` из той же сессии снова вернёт `true`, а для окончательного освобождения потребуется столько же вызовов `pg_advisory_unlock`. Поэтому соединение с неизвестным состоянием нельзя «просто вернуть» в pool — оно способно и блокировать другие сессии, и незаметно накапливать глубину блокировки при повторном использовании. В примере такое соединение удаляется из pool и закрывается. Транзакционный вариант `pg_advisory_xact_lock` этой проблемы лишён — блокировка снимается при завершении транзакции автоматически, — но требует держать транзакцию открытой всё время работы задачи.

---

## Идемпотентные воркеры

Повторная обработка одного и того же сообщения — не исключительная ситуация, а нормальный сценарий для consumer с at-least-once доставкой. Потребитель успел выполнить работу, но не успел подтвердить смещение; произошла перебалансировка группы; сработал повтор после таймаута — во всех случаях сообщение приходит второй раз. Поэтому защита от повторов становится обязанностью воркера.

Базовая идея: отметка об обработке хранится в той же базе, что и результат работы, и записывается той же транзакцией. Тогда «работа выполнена» и «сообщение отмечено» становятся одним неделимым фактом.

Ключ дедупликации должен включать область уникальности сообщения. Один и тот же `message_id` может встретиться у разных consumers или источников, поэтому безопасная схема использует составной первичный ключ:

```sql
CREATE TABLE processed_messages (
    consumer_name text        NOT NULL,
    message_id    text        NOT NULL,
    processed_at  timestamptz NOT NULL,
    PRIMARY KEY (consumer_name, message_id)
);
```

```go
var ErrPermanent = errors.New("permanent message error")

func (w *Worker) processOrder(ctx context.Context, msg Message) error {
    var event OrderCreatedEvent
    if err := json.Unmarshal(msg.Body, &event); err != nil {
        // Разобрать не удалось и не удастся — повтор не поможет.
        // Sentinel позволяет внешнему consumer отличить permanent error
        // от временного сбоя БД или внешнего сервиса.
        return fmt.Errorf("%w: unmarshal %s: %w", ErrPermanent, msg.ID, err)
    }

    tx, err := w.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback() //nolint:errcheck // no-op после успешного Commit

    // Отметка о сообщении. Если строка уже есть, INSERT не вставит ничего
    // и RowsAffected вернёт 0 — значит, сообщение уже обработано.
    res, err := tx.ExecContext(ctx, `
        INSERT INTO processed_messages (consumer_name, message_id, processed_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (consumer_name, message_id) DO NOTHING
    `, w.consumerName, msg.ID)
    if err != nil {
        return fmt.Errorf("mark message %s: %w", msg.ID, err)
    }
    affected, err := res.RowsAffected()
    if err != nil {
        return fmt.Errorf("rows affected: %w", err)
    }
    if affected == 0 {
        return nil // дубликат: работа уже выполнена в прошлый раз
    }

    // Основная работа — в той же транзакции, что и отметка.
    if err := w.fulfillOrder(ctx, tx, event); err != nil {
        return fmt.Errorf("fulfill order %s: %w", event.OrderID, err)
    }

    return tx.Commit()
}

func (w *Worker) consumeOrder(ctx context.Context, msg Message) error {
    err := w.processOrder(ctx, msg)
    if err == nil || !errors.Is(err, ErrPermanent) {
        return err // nil -> ack, временная ошибка -> retry
    }

    // Сначала надёжно публикуем проблемное сообщение в DLQ. Только nil
    // позволяет внешнему consumer подтвердить исходное сообщение.
    if dlqErr := w.dlq.Publish(ctx, msg, err); dlqErr != nil {
        return fmt.Errorf("publish message %s to DLQ: %w", msg.ID, dlqErr)
    }
    return nil
}
```

**Почему отметка и работа должны быть в одной транзакции.** Если вставить отметку отдельным запросом и закоммитить её до основной работы, то падение процесса между двумя операциями делает сообщение потерянным навсегда: при повторной доставке `INSERT` упрётся в конфликт, воркер решит, что всё уже сделано, и заказ останется несобранным. Обратный порядок — сначала работа, потом отметка — даёт зеркальную проблему: падение между ними приводит к повторному выполнению. Одна транзакция снимает оба случая.

**Где приём не работает.** Транзакция покрывает только собственную базу. Если работа воркера — вызов внешнего API (списать деньги, отправить письмо), откатить его нельзя, и одна транзакция ничего не гарантирует. Там нужна идемпотентность на стороне получателя: ключ идемпотентности в запросе к платёжному провайдеру, дедупликация по идентификатору у почтового сервиса. Подробнее — в [Saga и Outbox](./09-saga-and-outbox.md).

**Что делать со старыми записями.** Таблица `processed_messages` растёт линейно по объёму трафика. Если единственный источник повторов — исходный топик, записи можно хранить дольше его retention. Но replay из архива, backfill и повторная публикация старого `message_id` расширяют окно дедупликации; в такой системе срок очистки выбирают по максимальному окну replay, а не только по retention брокера.

---

## Backpressure и ограничение параллелизма

Если producer создаёт задачи быстрее, чем consumer успевает их выполнять, разница скоростей должна где-то накапливаться: во входной очереди, памяти процесса, ожидающих горутинах или пуле соединений. Без явной границы накопление продолжается до исчерпания памяти, таймаутов downstream или деградации всего сервиса.

Ограничение параллелизма и backpressure решают связанные, но разные задачи:

- **Ограничение параллелизма** задаёт число операций, одновременно выполняющихся в критичном downstream: например, не больше десяти запросов к БД или внешнему API.
- **Backpressure** передаёт заполненность обратно producer. Когда свободных слотов нет, producer блокируется, отклоняет новую задачу либо оставляет её в ограниченной или внешней очереди вместо бесконтрольного создания работы.

Одного семафора недостаточно, если захватывать его уже внутри новой горутины. Одновременных запросов к downstream действительно останется десять, но перед семафором смогут накопиться десять тысяч ожидающих горутин. Чтобы ограничить не только выполнение, но и число созданных горутин, слот захватывают до `go`.

**Почему ограничение необходимо:**

```text
Без ограничения:
  10000 задач -> 10000 горутин -> 10000 одновременных запросов в БД
  -> пул соединений исчерпан
  -> запросы ждут соединения и упираются в таймаут
  -> ошибки идут и по фоновым задачам, и по пользовательскому трафику

С ограничением:
  цикл запускает не больше 10 горутин одновременно
  -> при занятых 10 слотах цикл блокируется перед запуском следующей
  -> остальные элементы пока остаются во входной коллекции, а не в канале семафора
  -> БД работает в штатном режиме
  -> пропускная способность стабильна, задержка предсказуема
```

Политика при заполнении выбирается по требованиям к задаче:

| Политика | Что происходит при заполнении | Когда подходит |
|---|---|---|
| Блокировать producer | `Submit` или цикл ждёт свободный слот | Внутренний batch, где можно замедлить чтение |
| Отклонить задачу | Вызов сразу возвращает `ErrQueueFull` | Онлайн-запрос, которому лучше быстро ответить `429/503` |
| Ограниченный буфер | Короткий всплеск помещается в память; после заполнения нужна блокировка или отказ | Небольшие, предсказуемые всплески |
| Оставить во внешнем брокере | Consumer ограничивает получение: например, `prefetch` в RabbitMQ или `Pause` для partitions в Kafka | Задачи, которые нельзя терять при перезапуске процесса |

Следующий пример использует первую политику: `Acquire` вызывается перед созданием горутины, поэтому при десяти занятых слотах блокируется сам цикл producer.

Это backpressure только до текущего producer. Если `items` уже целиком загружен в память, семафор не уменьшит размер этой коллекции — он ограничит лишь число запущенных операций. При чтении из брокера или потока данных источник тоже должен иметь явную границу: через настройки получения брокера, паузу consumer или ограниченную очередь.

```go
// Semaphore для ограничения параллельных задач
type Semaphore struct {
    ch chan struct{}
}

func NewSemaphore(n int) (*Semaphore, error) {
    if n <= 0 {
        return nil, fmt.Errorf("semaphore size must be positive: %d", n)
    }
    return &Semaphore{ch: make(chan struct{}, n)}, nil
}

func (s *Semaphore) Acquire(ctx context.Context) error {
    // Быстрый путь не должен выигрывать у контекста, который уже отменён
    // до входа в Acquire.
    if err := ctx.Err(); err != nil {
        return err
    }
    select {
    case s.ch <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Semaphore) Release() {
    <-s.ch
}

// processItemsBestEffort выполняет не более 10 одновременных вызовов API.
// Ошибки отдельных items логируются, а наружу возвращается ошибка,
// из-за которой функция перестала запускать новую работу.
func processItemsBestEffort(
    ctx context.Context,
    items []Item,
    externalAPI ExternalAPI,
    log *slog.Logger,
) error {
    sem, err := NewSemaphore(10)
    if err != nil {
        return err
    }

    var wg sync.WaitGroup
    var startErr error

    for _, item := range items {
        if err := sem.Acquire(ctx); err != nil {
            startErr = err
            break // контекст отменён — новые задачи не запускаем
        }
        // Между первой проверкой ctx.Err и select контекст мог отмениться,
        // а свободная отправка в канал — победить в случайном выборе select.
        if err := ctx.Err(); err != nil {
            sem.Release()
            startErr = err
            break
        }

        wg.Add(1)
        go func() {
            defer wg.Done()
            defer sem.Release()
            if err := externalAPI.Process(ctx, item); err != nil {
                log.Error("process error", "err", err, "item", item.ID)
            }
        }()
    }

    wg.Wait() // без этого функция вернётся раньше, чем завершится работа
    return startErr
}
```

Семафор ограничивает число одновременных задач, но сам по себе не дожидается их окончания: последние десять горутин продолжают работать после выхода из цикла. Поэтому рядом всегда стоит `sync.WaitGroup` — иначе вызывающий код решит, что обработка закончена, а `main` может завершиться прямо посреди запросов.

Переменная цикла отдельной копией (`item := item`) больше не нужна: начиная с Go 1.22 у каждой итерации собственный экземпляр переменной. В коде под более старые версии эта строка обязательна, иначе все горутины увидят последний элемент.

Существенная деталь: страдает не только фоновая обработка. Пул соединений у процесса общий, поэтому воркер, запустивший десять тысяч запросов, отбирает соединения у HTTP-обработчиков того же сервиса. Отказ выглядит как деградация API, хотя причина — фоновая задача.

Предварительная и повторная проверки `ctx.Err()` уменьшают окно запуска после отмены, но не создают абсолютной атомарной границы: контекст может отмениться сразу после второй проверки. Поэтому уже запущенная операция всё равно обязана соблюдать переданный `ctx`.

В стандартной библиотеке аналога нет, но в `golang.org/x/sync/semaphore` есть готовая реализация с весами: `semaphore.NewWeighted(10)` и `Acquire(ctx, 1)`. Свой семафор на канале уместен, когда веса не нужны и не хочется тянуть зависимость.

Семафор ограничивает in-flight concurrency, а не скорость запросов во времени. Если внешний API разрешает, например, 100 запросов в секунду, дополнительно нужен rate limiter. Размер семафора выбирают по фактической ёмкости downstream: доступной доле DB connection pool, лимиту внешнего API и измеренной latency, а не по произвольному круглому числу.

---

## Наблюдаемость воркеров

У фоновой задачи нет клиента, который пожалуется, поэтому метрики здесь — основной способ узнать о проблеме до того, как о ней спросит бизнес. Логи помогают разобрать отдельную ошибку, traces — восстановить путь задачи, а метрики показывают систематическое отставание и рост доли отказов.

```go
type WorkerMetrics struct {
    succeeded  prometheus.Counter
    failed     prometheus.Counter
    duration   prometheus.Histogram
    queueDepth prometheus.GaugeFunc
    inFlight   prometheus.Gauge
}

func NewWorkerMetrics(jobs <-chan Job) *WorkerMetrics {
    return &WorkerMetrics{
        succeeded: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "worker_jobs_succeeded_total",
            Help: "Successfully processed jobs.",
        }),
        failed: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "worker_jobs_failed_total",
            Help: "Jobs completed with an error.",
        }),
        duration: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: "worker_job_duration_seconds",
            Help: "Job processing duration.",
        }),
        // GaugeFunc читает размер буфера непосредственно во время scrape.
        // Для unbuffered channel значение всегда 0; ожидающие Submit нужно
        // считать отдельной метрикой, если это важно для эксплуатации.
        queueDepth: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
            Name: "worker_queue_depth",
            Help: "Jobs currently buffered in the in-memory queue.",
        }, func() float64 {
            return float64(len(jobs))
        }),
        inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "worker_jobs_in_flight",
            Help: "Jobs currently being processed.",
        }),
    }
}

func (w *Worker) process(ctx context.Context, job Job) {
    w.metrics.inFlight.Inc()
    defer w.metrics.inFlight.Dec()

    start := time.Now()
    defer func() {
        w.metrics.duration.Observe(time.Since(start).Seconds())
    }()

    if err := w.handle(ctx, job); err != nil {
        w.metrics.failed.Inc()
        w.log.Error("job failed", "err", err, "job_id", job.ID, "type", job.Type)
    } else {
        w.metrics.succeeded.Inc()
    }
}
```

Все метрики из `WorkerMetrics` нужно зарегистрировать в одном `prometheus.Registry`. Для очереди во внешнем брокере `len(channel)` неприменим: там используют broker lag, число готовых сообщений и возраст самого старого сообщения.

**Алерты, которые должны быть:**

| Метрика | Условие алерта | Что означает |
|---|---|---|
| `worker_queue_depth` | долго близка к `queueSize` или растёт вместе с возрастом задач | Воркеры не успевают: приход задач выше пропускной способности |
| failure rate из `worker_jobs_failed_total` и `worker_jobs_succeeded_total` | доля ошибок заметно выше нормального уровня | Отказ получателя или проблемное сообщение в цикле повторов |
| `worker_job_duration_seconds` p99 | превышает нормальное значение в несколько раз | Задачи зависают: обычно нет таймаута у внешнего вызова |
| `worker_jobs_in_flight` | = 0 при непустой очереди в течение нескольких интервалов | Воркеры остановлены или не получают задачи |
| Перезапуски процесса | > 3 раз за час | Паника в обработчике или превышение лимита памяти |

Абсолютный порог по глубине очереди («больше 1000») плох тем, что зависит от сервиса: для одного это авария, для другого — обычный вечерний всплеск. Для ограниченного канала важно следить не только за ростом, но и за длительным насыщением около `queueSize`: после заполнения глубина больше не растёт, хотя отправители уже заблокированы. Второй важный сигнал — возраст самой старой задачи: он прямо отвечает на вопрос «насколько мы отстали», а глубина отвечает на него только вместе со скоростью обработки. Пороги доли ошибок и p99 также берутся из SLO и нормального уровня конкретного сервиса, а не копируются из примера.

---

## Interview-ready answer

**1. Что главное в фоновых воркерах на Go?**

- Главное — корректная остановка и ограниченный параллелизм; всё остальное вторично.
- Остановка — `signal.NotifyContext` ловит SIGTERM и инициирует `Shutdown`, а отдельный контекст выполнения позволяет сначала обработать принятую очередь и отменяется только после graceful deadline.
- Причина внимания к теме — у фоновой задачи нет клиента, который заметит сбой, поэтому ошибка обнаруживается по последствиям.

**2. Как безопасно остановить пул воркеров?**

- Фаза 1 — запретить новые отправки и закрыть `stopping`, чтобы разблокировать `Submit`, ожидающие места в очереди.
- Фаза 2 — дождаться зарегистрированных `Submit`, закрыть `jobs` и дать воркерам дочитать канал.
- Mutex — не удерживать во время отправки в канал, иначе заполненная очередь не даст `Shutdown` добраться до собственного timeout.
- Контексты — контекст `Submit` ограничивает постановку в очередь, а отдельный контекст пула управляет выполнением принятых задач.
- Deadline — после graceful-периода отменить контекст выполнения; обработчик должен соблюдать отмену, потому что Go не умеет принудительно остановить произвольную функцию.
- Повторный вызов — ждать общий `done`, а не возвращать успех только потому, что приём задач уже остановлен.

**3. Зачем ограничивать параллелизм?**

- Причина — пул соединений к базе конечен: десять тысяч горутин превращаются в очередь за соединением и таймауты.
- Побочный эффект — пул общий на процесс, поэтому фоновая задача отбирает соединения у пользовательского трафика.
- Инструмент — семафор на буферизованном канале или `golang.org/x/sync/semaphore`, рядом обязательно `WaitGroup`.

**4. Как сделать периодическую задачу единственной в кластере?**

- Механизм — распределённая блокировка: Redis `SET NX` с TTL и уникальным token на каждое владение или `pg_try_advisory_lock` на выделенной сессии PostgreSQL.
- Продление — атомарная проверка владельца плюс `PEXPIRE`; если продление не подтверждено за ограниченное время, задача отменяется по политике fail-closed.
- Ограничение — блокировка ограничивает время, а не доступ: пауза процесса дольше TTL или потеря ключа Redis при failover допускает работу двух реплик.
- Вывод — блокировка снимает дублирование работы, но не заменяет идемпотентность.

**5. Как добиться одного бизнес-эффекта при at-least-once доставке?**

- Настоящей exactly-once доставки здесь нет: брокер по-прежнему может доставить сообщение повторно.
- Для изменений в одной БД можно получить effectively-once эффект через таблицу `processed_messages` с ключом `(consumer_name, message_id)` и `ON CONFLICT DO NOTHING`; `RowsAffected() == 0` означает дубликат.
- Dedup-отметка и бизнес-изменение выполняются в одной транзакции, иначе падение между ними приводит к потере или повторному выполнению работы.
- Гарантия действует только в пределах этой БД и окна хранения dedup-записей. Повтор с другим `message_id` считается новой операцией.
- Внешний side effect транзакцией не покрывается: получатель должен поддерживать idempotency key. Transactional outbox делает публикацию надёжной, но сам по себе не устраняет повторный вызов получателя. Permanent errors отдельно маршрутизируются в DLQ.
