# Задача 2: Background Task Processor

## Содержание

- [Формулировка](#формулировка)
- [Изначальный код](#изначальный-код)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Разбор проблем](#разбор-проблем)
- [Решение уровня 1: drop-mode](#решение-уровня-1-drop-mode)
- [Reference implementation с graceful drain](#reference-implementation-с-graceful-drain)
- [Как работает lifecycle](#как-работает-lifecycle)
- [Границы in-process очереди](#границы-in-process-очереди)
- [Тесты](#тесты)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

В отличие от конечного [fetcher](./01-fetcher-with-cache.md), processor живёт
долго: принимает работу через `Submit`, обрабатывает её в фоне и когда-нибудь
получает `Stop`. Главная задача code review — определить точный момент принятия
работы и доказать, что конкурентный shutdown не теряет уже принятые задачи.

---

## Формулировка

Нужно проверить in-process processor со следующими ожиданиями:

- число одновременно работающих обработчиков ограничено;
- временная ошибка приводит к retry;
- permanent-ошибка после лимита попадает в dead letter;
- `Stop` больше не принимает новые задачи и завершает уже принятые;
- deadline shutdown ограничивает ожидание;
- panic обработчика не разрушает процесс.

Не все эти гарантии обязательно нужны одному продукту. Кандидат должен сначала
уточнить контракт, а затем проверить, соответствует ли ему код.

---

## Изначальный код

```go
package main

import (
    "errors"
    "fmt"
    "math/rand"
    "sync"
    "time"
)

type Task struct {
    ID       int
    Payload  string
    attempts int
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
    for workerID := 0; workerID < p.workers; workerID++ {
        go func() {
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
        }()
    }
}

func (p *Processor) processTask(task *Task) {
    err := doWork(task.Payload)
    if err != nil {
        task.attempts++
        fmt.Printf(
            "task %d failed (attempt %d), retrying\n",
            task.ID,
            task.attempts,
        )
        p.queue <- task
        return
    }
    fmt.Printf("task %d done\n", task.ID)
}

func (p *Processor) Submit(task *Task) {
    p.queue <- task
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
```

Код компилируется, но результат зависит от interleaving. Возможны ранний выход
`Stop`, потеря очереди, panic при конкурентном `Submit`, бесконечные повторы и
deadlock, когда все worker’ы пытаются вернуть retry в заполненную очередь.

---

## Уточняющие вопросы

1. `Stop` должен доделать очередь или отбросить её?
2. В какой момент `Submit` считается принятым: до отправки, после помещения в
   память или после durable-записи?
3. Blocking `Submit` допустим, нужен ли `TrySubmit`, и как caller отменяет
   ожидание свободного места?
4. Какие ошибки являются retryable, а какие permanent?
5. Сколько попыток включает `MaxAttempts`: только повторы или первую попытку
   тоже?
6. Что происходит с task после исчерпания попыток?
7. Обязан ли обработчик соблюдать контекст и каков timeout одной попытки?
8. Допустима ли потеря очереди при рестарте процесса?

В reference implementation `MaxAttempts` включает первую попытку. Принятая
задача либо успешно завершается, либо становится terminal failure, либо
отбрасывается при forced shutdown. Durability между рестартами не обещается.

---

## Разбор проблем

### 1. `WaitGroup.Add` вызывается слишком поздно

`Stop` может выполнить `Wait` раньше, чем новая goroutine увеличит счётчик.
Тогда `Wait` вернётся немедленно, очередь будет закрыта, а worker продолжит
работу. Положительный `Add` при нулевом счётчике должен произойти раньше `Wait`.

```go
p.wg.Add(1)
go func() {
    defer p.wg.Done()
    // ...
}()
```

### 2. `Submit` соревнуется с `close(queue)`

Проверка atomic-флага перед отправкой не решает проблему сама по себе: Stop
может закрыть канал между проверкой и `queue <- task`. Нужен единый
линеаризуемый переход состояния либо правило «очередь не закрывается, остановка
передаётся отдельным сигналом».

### 3. Stop теряет очередь

Когда `queue` и `stop` одновременно готовы, `select` выбирает одну готовую ветку
псевдослучайно. Worker может завершиться, хотя в очереди ещё есть задачи. Это
потеря данных, а не memory leak.

Текущая in-flight задача при этом не прерывается: worker проверит `stop` только
после возврата из `processTask`, а `Stop` ждёт `wg`.

### 4. `attempts` имеет неясное ownership

Показанный retry передаёт один указатель последовательно: отправка и получение
через канал синхронизируют владение, поэтому сама эта последовательность не
создаёт data race. Race появится, если caller повторно отправит тот же `*Task`
или изменяет его после `Submit`.

Processor должен принять задачу по значению либо скопировать её изменяемые поля.
Внутренний счётчик попыток лучше хранить в закрытом `workItem`, недоступном
caller’у.

### 5. Любая ошибка повторяется бесконечно

Permanent-ошибка занимает capacity без шанса на успех. Это не обязательно
deadlock: worker может продолжать крутить задачу и создавать starvation для
свежей работы. Нужны классификация ошибок, `MaxAttempts` и terminal policy.

### 6. Между попытками нет backoff

Немедленный retry увеличивает нагрузку именно тогда, когда downstream уже
испытывает проблему. Обычно используется exponential backoff с jitter и верхней
границей, ограниченной deadline задачи.

### 7. Retry может заблокировать все worker’ы

Если основная очередь заполнена, каждый worker может остановиться на
`p.queue <- task`. Получать следующие элементы больше некому — это настоящий
deadlock.

Простой корректный вариант повторяет задачу внутри того же worker’а. Он удерживает
worker во время backoff, но не требует повторной отправки. Для большой retry-
нагрузки нужна отдельная delayed priority queue или внешний broker.

### 8. Обработчик не получает context

`Stop` не может ограничить текущую работу. Даже правильно переданный context не
может принудительно остановить функцию: обработчик обязан проверять `Done` и
прерывать блокирующие операции через context-aware API.

### 9. Blocking `Submit` уже является backpressure

Проблема исходного API не в отсутствии backpressure, а в неограниченном и
неотменяемом ожидании. Полезно предоставить две операции:

- `Submit(ctx, task)` — ждёт место с ограничением контекста;
- `TrySubmit(task)` — сразу возвращает `ErrQueueFull`.

### 10. Неперехваченная panic завершает процесс

Panic в любой goroutine после выполнения defer поднимается до её верхней
функции, после чего runtime завершает весь процесс. Она не «убивает только один
worker». Если граница задачи должна изолировать panic пользовательского
обработчика, `recover` ставится именно вокруг одного вызова и сохраняет stack.

Recover не следует ставить вокруг всего worker loop: повреждённое состояние
одной задачи не должно продолжить произвольное выполнение внутри неё.

### 11. Нет lifecycle state machine

Не определены повторный `Start`, `Stop` до `Start`, конкурентные вызовы `Stop` и
`Submit` во время drain. Одного `atomic.Bool` недостаточно для переходов с
несколькими действиями.

### 12. Конфигурация не валидируется

При `workers <= 0` задачи никто не получает. Нулевой размер очереди меняет
семантику, нулевой timeout может немедленно отменять каждую попытку, а
`MaxAttempts <= 0` делает контракт бессмысленным.

### 13. Нет durable delivery и idempotency

Задача исчезает при падении процесса. Если side effect успел выполниться, а
процесс упал раньше фиксации успеха, после восстановления возможен дубль.
In-memory worker pool не заменяет durable queue, idempotency key или outbox.

### 14. Нет наблюдаемости

Нужны как минимум количество принятых, успешных, повторённых, terminal failed и
forced-dropped задач, глубина очереди и latency попытки. ID задачи полезен для
логов, но не должен становиться label метрики с высокой cardinality.

---

## Решение уровня 1: drop-mode

Минимальное решение ниже выбирает простой контракт: `Stop` прекращает приём,
дожидается текущих вызовов обработчика и может отбросить очередь. Канал `queue`
не закрывается. `TrySubmit` и переход в `stopping` синхронизированы одним mutex,
поэтому задача не может быть принята после точки остановки.

```go
package processor

import (
    "context"
    "errors"
    "log/slog"
    "runtime/debug"
    "sync"
)

var (
    ErrInvalidConfig    = errors.New("invalid processor config")
    ErrProcessorStopped = errors.New("processor stopped")
    ErrQueueFull        = errors.New("queue full")
)

type Task struct {
    ID      string
    Payload string
}

type workItem struct {
    task     Task
    attempts int
}

type state uint8

const (
    stateNew state = iota
    stateRunning
    stateStopping
    stateStopped
)

type ProcessFunc func(ctx context.Context, task Task) error

type Processor struct {
    mu          sync.Mutex
    state       state
    queue       chan workItem
    workers     int
    maxAttempts int
    process     ProcessFunc
    stop        chan struct{}
    done        chan struct{}
    workerWG    sync.WaitGroup
}

func New(
    workers int,
    queueSize int,
    maxAttempts int,
    process ProcessFunc,
) (*Processor, error) {
    if workers <= 0 || queueSize <= 0 || maxAttempts <= 0 || process == nil {
        return nil, ErrInvalidConfig
    }
    return &Processor{
        state:       stateNew,
        queue:       make(chan workItem, queueSize),
        workers:     workers,
        maxAttempts: maxAttempts,
        process:     process,
        stop:        make(chan struct{}),
        done:        make(chan struct{}),
    }, nil
}

func (p *Processor) Start() error {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.state != stateNew {
        return ErrProcessorStopped
    }
    p.state = stateRunning

    p.workerWG.Add(p.workers)
    for workerID := 0; workerID < p.workers; workerID++ {
        go func(id int) {
            defer p.workerWG.Done()
            p.workerLoop(id)
        }(workerID)
    }
    return nil
}

func (p *Processor) workerLoop(workerID int) {
    for {
        select {
        case <-p.stop:
            return
        case item := <-p.queue:
            p.handle(workerID, item)
        }
    }
}

func (p *Processor) handle(workerID int, item workItem) {
    defer func() {
        if recovered := recover(); recovered != nil {
            slog.Error(
                "task panic",
                "worker_id", workerID,
                "task_id", item.task.ID,
                "panic", recovered,
                "stack", string(debug.Stack()),
            )
        }
    }()

    err := p.process(context.Background(), item.task)
    if err == nil {
        return
    }

    item.attempts++
    if item.attempts >= p.maxAttempts {
        slog.Warn("task failed", "task_id", item.task.ID, "err", err)
        return
    }

    // Drop-mode: очередь не должна заблокировать всех worker’ов.
    select {
    case p.queue <- item:
    default:
        slog.Warn("retry dropped: queue full", "task_id", item.task.ID)
    }
}

func (p *Processor) TrySubmit(task Task) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.state != stateRunning {
        return ErrProcessorStopped
    }
    select {
    case p.queue <- workItem{task: task}:
        return nil
    default:
        return ErrQueueFull
    }
}

func (p *Processor) Stop() error {
    p.mu.Lock()
    switch p.state {
    case stateNew:
        p.mu.Unlock()
        return ErrProcessorStopped
    case stateStopped:
        p.mu.Unlock()
        return nil
    case stateStopping:
        done := p.done
        p.mu.Unlock()
        <-done
        return nil
    case stateRunning:
        p.state = stateStopping
        close(p.stop)
    }
    p.mu.Unlock()

    p.workerWG.Wait()

    p.mu.Lock()
    p.state = stateStopped
    close(p.done)
    p.mu.Unlock()
    return nil
}
```

Drop-mode подходит только тогда, когда потеря очереди явно разрешена. Для webhook,
платежей или изменения бизнес-состояния такой контракт обычно недостаточен.

---

## Reference implementation с graceful drain

В этом варианте `Submit` сначала резервирует ответственность processor’а под
mutex. С этого момента задача входит в `pending`, даже если goroutine ещё ждёт
место в канале. `Stop` переводит state в `draining`, запрещает новые резервации и
ждёт закрытия `drained`.

Retry выполняется внутри worker’а. Это менее эффективно при большом backoff, но
делает пример проверяемым: задача не исчезает во внутренней очереди scheduler’а,
а `pending` уменьшается ровно один раз после terminal outcome.

```go
package processor

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "math/rand"
    "runtime/debug"
    "sync"
    "sync/atomic"
    "time"
)

var (
    ErrInvalidConfig    = errors.New("invalid processor config")
    ErrNotStarted       = errors.New("processor not started")
    ErrProcessorStopped = errors.New("processor stopped")
    ErrQueueFull        = errors.New("queue full")
)

type Task struct {
    ID      string
    Payload []byte
}

type Config struct {
    Workers      int
    QueueSize    int
    MaxAttempts  int
    BaseBackoff  time.Duration
    MaxBackoff   time.Duration
    TaskTimeout  time.Duration
    Retryable    func(error) bool
}

type ProcessFunc func(ctx context.Context, task Task) error
type DeadLetterFunc func(task Task, err error, attempts int)

type lifecycleState uint8

const (
    lifecycleNew lifecycleState = iota
    lifecycleRunning
    lifecycleDraining
    lifecycleStopped
)

type workItem struct {
    task Task
}

type Processor struct {
    cfg        Config
    process    ProcessFunc
    deadLetter DeadLetterFunc
    log        *slog.Logger

    mu      sync.Mutex
    state   lifecycleState
    pending int
    drained chan struct{}

    queue       chan workItem
    runCtx      context.Context
    cancel      context.CancelFunc
    workerWG    sync.WaitGroup
    workersDone chan struct{}

    accepted atomic.Int64
    processed atomic.Int64
    failed   atomic.Int64
    retried  atomic.Int64
    dropped  atomic.Int64
}

func closedSignal() chan struct{} {
    signal := make(chan struct{})
    close(signal)
    return signal
}

func New(
    cfg Config,
    process ProcessFunc,
    deadLetter DeadLetterFunc,
    logger *slog.Logger,
) (*Processor, error) {
    if cfg.Workers <= 0 || cfg.QueueSize <= 0 || cfg.MaxAttempts <= 0 ||
        cfg.BaseBackoff <= 0 || cfg.MaxBackoff < cfg.BaseBackoff ||
        cfg.TaskTimeout <= 0 || cfg.Retryable == nil || process == nil {
        return nil, ErrInvalidConfig
    }
    if logger == nil {
        logger = slog.Default()
    }

    return &Processor{
        cfg:         cfg,
        process:     process,
        deadLetter:  deadLetter,
        log:         logger,
        state:       lifecycleNew,
        drained:     closedSignal(),
        queue:       make(chan workItem, cfg.QueueSize),
        workersDone: make(chan struct{}),
    }, nil
}

func (p *Processor) Start() error {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.state != lifecycleNew {
        return ErrProcessorStopped
    }

    p.runCtx, p.cancel = context.WithCancel(context.Background())
    p.state = lifecycleRunning
    p.workerWG.Add(p.cfg.Workers)
    for workerID := 0; workerID < p.cfg.Workers; workerID++ {
        go func(id int) {
            defer p.workerWG.Done()
            p.workerLoop(id)
        }(workerID)
    }

    go func() {
        p.workerWG.Wait()
        p.mu.Lock()
        p.state = lifecycleStopped
        p.mu.Unlock()
        close(p.workersDone)
    }()
    return nil
}

func (p *Processor) reserve(task Task) (workItem, error) {
    copied := Task{
        ID:      task.ID,
        Payload: append([]byte(nil), task.Payload...),
    }

    p.mu.Lock()
    defer p.mu.Unlock()
    if p.state != lifecycleRunning {
        return workItem{}, ErrProcessorStopped
    }
    if p.pending == 0 {
        p.drained = make(chan struct{})
    }
    p.pending++
    return workItem{task: copied}, nil
}

func (p *Processor) taskDone() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.pending--
    if p.pending < 0 {
        panic("processor: negative pending counter")
    }
    if p.pending == 0 {
        close(p.drained)
    }
}

func (p *Processor) Submit(ctx context.Context, task Task) error {
    if ctx == nil {
        return ErrInvalidConfig
    }
    item, err := p.reserve(task)
    if err != nil {
        return err
    }

    select {
    case p.queue <- item:
        p.accepted.Add(1)
        return nil
    case <-ctx.Done():
        p.taskDone()
        return ctx.Err()
    case <-p.runCtx.Done():
        p.taskDone()
        return ErrProcessorStopped
    }
}

func (p *Processor) TrySubmit(task Task) error {
    item, err := p.reserve(task)
    if err != nil {
        return err
    }
    select {
    case p.queue <- item:
        p.accepted.Add(1)
        return nil
    default:
        p.taskDone()
        return ErrQueueFull
    }
}

func (p *Processor) workerLoop(workerID int) {
    for {
        select {
        case <-p.runCtx.Done():
            return
        case item := <-p.queue:
            p.executeSafely(workerID, item)
        }
    }
}

func (p *Processor) executeSafely(workerID int, item workItem) {
    attempts := 0
    defer p.taskDone()
    defer func() {
        if recovered := recover(); recovered != nil {
            if attempts == 0 {
                attempts = 1
            }
            err := fmt.Errorf("panic: %v", recovered)
            p.failed.Add(1)
            p.log.Error(
                "task panic",
                "worker_id", workerID,
                "task_id", item.task.ID,
                "panic", recovered,
                "stack", string(debug.Stack()),
            )
            p.callDeadLetter(item.task, err, attempts)
        }
    }()

    for attempts < p.cfg.MaxAttempts {
        attempts++
        attemptCtx, cancel := context.WithTimeout(
            p.runCtx,
            p.cfg.TaskTimeout,
        )
        err := p.process(attemptCtx, item.task)
        cancel()

        if err == nil {
            p.processed.Add(1)
            return
        }
        if p.runCtx.Err() != nil {
            p.dropped.Add(1)
            return
        }
        if !p.cfg.Retryable(err) || attempts == p.cfg.MaxAttempts {
            p.failed.Add(1)
            p.callDeadLetter(item.task, err, attempts)
            return
        }

        p.retried.Add(1)
        timer := time.NewTimer(p.backoff(attempts))
        select {
        case <-timer.C:
        case <-p.runCtx.Done():
            if !timer.Stop() {
                select {
                case <-timer.C:
                default:
                }
            }
            p.dropped.Add(1)
            return
        }
    }
}

func (p *Processor) backoff(failedAttempt int) time.Duration {
    delay := p.cfg.BaseBackoff
    for step := 1; step < failedAttempt; step++ {
        if delay >= p.cfg.MaxBackoff/2 {
            delay = p.cfg.MaxBackoff
            break
        }
        delay *= 2
    }
    if delay > p.cfg.MaxBackoff {
        delay = p.cfg.MaxBackoff
    }
    if delay <= 1 {
        return 0
    }
    return time.Duration(rand.Int63n(int64(delay)))
}

func (p *Processor) callDeadLetter(
    task Task,
    err error,
    attempts int,
) {
    if p.deadLetter == nil {
        return
    }
    defer func() {
        if recovered := recover(); recovered != nil {
            p.log.Error(
                "dead letter callback panic",
                "task_id", task.ID,
                "panic", recovered,
                "stack", string(debug.Stack()),
            )
        }
    }()
    p.deadLetter(task, err, attempts)
}

func (p *Processor) Stop(ctx context.Context) error {
    if ctx == nil {
        return ErrInvalidConfig
    }

    p.mu.Lock()
    switch p.state {
    case lifecycleNew:
        p.mu.Unlock()
        return ErrNotStarted
    case lifecycleStopped:
        p.mu.Unlock()
        return nil
    case lifecycleDraining:
        workersDone := p.workersDone
        p.mu.Unlock()
        select {
        case <-workersDone:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    case lifecycleRunning:
        p.state = lifecycleDraining
    }
    drained := p.drained
    p.mu.Unlock()

    select {
    case <-drained:
        p.cancel()
        select {
        case <-p.workersDone:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    case <-ctx.Done():
        // Forced shutdown: queued tasks могут быть потеряны. In-flight
        // обработчики должны соблюдать p.runCtx, иначе их нельзя остановить.
        p.cancel()
        return ctx.Err()
    }
}

type Stats struct {
    Accepted  int64
    Processed int64
    Failed    int64
    Retried   int64
    Dropped   int64
    Pending   int
    Queue     int
}

func (p *Processor) Stats() Stats {
    p.mu.Lock()
    pending := p.pending
    p.mu.Unlock()
    return Stats{
        Accepted:  p.accepted.Load(),
        Processed: p.processed.Load(),
        Failed:    p.failed.Load(),
        Retried:   p.retried.Load(),
        Dropped:   p.dropped.Load(),
        Pending:   pending,
        Queue:     len(p.queue),
    }
}
```

---

## Как работает lifecycle

```text
New --Start--> running --Stop--> draining --cancel workers--> stopped
```

Последовательность graceful shutdown:

1. `Stop` под mutex меняет `running` на `draining`.
2. Новые `Submit` больше не могут увеличить `pending`.
3. Ранее зарезервированные Submit либо кладут задачу в очередь, либо уменьшают
   `pending` при отмене.
4. Worker уменьшает `pending` один раз после success, terminal error или panic.
5. Последняя terminal-задача закрывает `drained`.
6. `Stop` отменяет worker context и ждёт `workersDone`.

Проверка `len(queue) == 0` не заменяет этот счётчик. Пустой канал может означать,
что все задачи уже находятся in-flight или внутри retry scheduler.

При истечении shutdown context начинается forced shutdown. Метод возвращает
ошибку контекста, не обещая обработать оставшуюся очередь. Если `ProcessFunc`
игнорирует переданный context, Go не предоставляет безопасного способа
принудительно завершить эту goroutine.

---

## Границы in-process очереди

- **Durability —** после падения процесса очередь исчезает. Для гарантированной
  доставки нужна БД или broker с подтверждением обработки.
- **Idempotency —** внешний side effect может завершиться раньше записи успеха.
  Повтор после восстановления должен использовать idempotency key.
- **Backoff —** retry внутри worker’а прост и корректен, но уменьшает доступную
  concurrency. Для больших задержек нужна priority queue или broker-delayed
  delivery.
- **Dead letter —** callback вызывается конкурентно из worker’ов и должен быть
  thread-safe. Медленная durable-запись увеличивает время drain.
- **Forced stop —** `Dropped` показывает только задачи, которые уже взял worker.
  Оставшиеся в канале после отмены можно оценить по `Pending`, но для точного
  аудита нужна durable queue.
- **Повторный Stop —** конкурентный caller ждёт общий `workersDone`; его context
  ограничивает только собственное ожидание и не меняет deadline первого Stop.

---

## Тесты

Тесты ниже проверяют не только отсутствие panic, но и гарантию: каждая принятая
до graceful Stop задача достигает terminal outcome.

```go
package processor

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "sync/atomic"
    "testing"
    "time"
)

func testConfig() Config {
    return Config{
        Workers:     4,
        QueueSize:   128,
        MaxAttempts: 3,
        BaseBackoff: time.Millisecond,
        MaxBackoff:  5 * time.Millisecond,
        TaskTimeout: time.Second,
        Retryable:   func(error) bool { return true },
    }
}

func TestStopDrainsAcceptedTasks(t *testing.T) {
    var processed atomic.Int64
    processor, err := New(
        testConfig(),
        func(context.Context, Task) error {
            processed.Add(1)
            return nil
        },
        nil,
        nil,
    )
    if err != nil {
        t.Fatal(err)
    }
    if err := processor.Start(); err != nil {
        t.Fatal(err)
    }

    const tasks = 100
    for id := 0; id < tasks; id++ {
        task := Task{ID: fmt.Sprint(id)}
        if err := processor.Submit(context.Background(), task); err != nil {
            t.Fatal(err)
        }
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    if err := processor.Stop(ctx); err != nil {
        t.Fatal(err)
    }
    if got := processed.Load(); got != tasks {
        t.Fatalf("processed = %d, want %d", got, tasks)
    }
}

func TestRetryThenSuccess(t *testing.T) {
    var attempts atomic.Int64
    processor, err := New(
        testConfig(),
        func(context.Context, Task) error {
            if attempts.Add(1) < 3 {
                return errors.New("temporary")
            }
            return nil
        },
        nil,
        nil,
    )
    if err != nil {
        t.Fatal(err)
    }
    if err := processor.Start(); err != nil {
        t.Fatal(err)
    }
    if err := processor.Submit(
        context.Background(),
        Task{ID: "retry"},
    ); err != nil {
        t.Fatal(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    if err := processor.Stop(ctx); err != nil {
        t.Fatal(err)
    }
    if got := attempts.Load(); got != 3 {
        t.Fatalf("attempts = %d, want 3", got)
    }
}

func TestPanicIsTaskBoundary(t *testing.T) {
    var deadLetters atomic.Int64
    processor, err := New(
        testConfig(),
        func(context.Context, Task) error {
            panic("boom")
        },
        func(Task, error, int) {
            deadLetters.Add(1)
        },
        nil,
    )
    if err != nil {
        t.Fatal(err)
    }
    if err := processor.Start(); err != nil {
        t.Fatal(err)
    }
    if err := processor.Submit(
        context.Background(),
        Task{ID: "panic"},
    ); err != nil {
        t.Fatal(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    if err := processor.Stop(ctx); err != nil {
        t.Fatal(err)
    }
    if got := deadLetters.Load(); got != 1 {
        t.Fatalf("dead letters = %d, want 1", got)
    }
}

func TestConcurrentSubmitAndStop(t *testing.T) {
    var processed atomic.Int64
    processor, err := New(
        testConfig(),
        func(context.Context, Task) error {
            processed.Add(1)
            return nil
        },
        nil,
        nil,
    )
    if err != nil {
        t.Fatal(err)
    }
    if err := processor.Start(); err != nil {
        t.Fatal(err)
    }

    start := make(chan struct{})
    var accepted atomic.Int64
    var submitWG sync.WaitGroup
    for id := 0; id < 100; id++ {
        submitWG.Add(1)
        go func(current int) {
            defer submitWG.Done()
            <-start
            err := processor.Submit(
                context.Background(),
                Task{ID: fmt.Sprint(current)},
            )
            if err == nil {
                accepted.Add(1)
                return
            }
            if !errors.Is(err, ErrProcessorStopped) {
                t.Errorf("submit error = %v", err)
            }
        }(id)
    }
    close(start)

    stopCtx, cancelStop := context.WithTimeout(
        context.Background(),
        time.Second,
    )
    defer cancelStop()
    stopErr := processor.Stop(stopCtx)
    submitWG.Wait()
    if stopErr != nil {
        t.Fatal(stopErr)
    }
    if got, want := processed.Load(), accepted.Load(); got != want {
        t.Fatalf("processed = %d, accepted = %d", got, want)
    }
}

func TestForcedStopRespectsCallerDeadline(t *testing.T) {
    started := make(chan struct{}, 1)
    processor, err := New(
        testConfig(),
        func(ctx context.Context, task Task) error {
            started <- struct{}{}
            <-ctx.Done()
            return ctx.Err()
        },
        nil,
        nil,
    )
    if err != nil {
        t.Fatal(err)
    }
    if err := processor.Start(); err != nil {
        t.Fatal(err)
    }
    if err := processor.Submit(
        context.Background(),
        Task{ID: "blocked"},
    ); err != nil {
        t.Fatal(err)
    }
    <-started

    stopCtx, cancelStop := context.WithTimeout(
        context.Background(),
        20*time.Millisecond,
    )
    defer cancelStop()
    if err := processor.Stop(stopCtx); !errors.Is(err, context.DeadlineExceeded) {
        t.Fatalf("stop error = %v, want deadline exceeded", err)
    }
}
```

Эти тесты следует запускать с `go test -race ./...`. Для проверки forced shutdown
нужен отдельный тест с обработчиком, который намеренно игнорирует context: Stop
обязан вернуть deadline error, но завершить такую goroutine безопасно не сможет.

---

## Interview-ready answer

**1. Почему `WaitGroup.Add` вызывают до запуска goroutine?**

- Гарантия — положительный `Add` при нулевом счётчике должен произойти раньше
  конкурентного `Wait`.
- Ошибка — `Wait` может увидеть ноль и вернуться до регистрации worker’а.
- Вариант — в современных версиях Go можно использовать `WaitGroup.Go`, если
  выполняется его контракт для запускаемой функции.

**2. Почему atomic-флаг не решает race между Submit и Stop?**

- Разрыв — состояние проверяется отдельно от отправки в очередь.
- Interleaving — Stop может вмешаться между `Load` и send.
- Решение — резервирование принятой задачи и переход shutdown должны иметь одну
  точку линеаризации под mutex или у одного event-loop owner’а.

**3. Как доказать graceful drain?**

- Приём — после перехода в `draining` новые задачи не увеличивают `pending`.
- Учёт — каждая ранее принятая задача уменьшает `pending` ровно один раз после
  terminal outcome.
- Завершение — `pending == 0` закрывает `drained`, после чего останавливаются
  worker’ы.

**4. Где должна находиться граница recover?**

- Область — recover окружает один вызов пользовательского обработчика.
- Результат — panic превращается в terminal failure с логом и stack trace.
- Ограничение — panic инфраструктурного кода нельзя молча скрывать общим recover
  вокруг бесконечного worker loop.

**5. Когда in-process worker pool недостаточен?**

- Рестарт — очередь и её задачи должны переживать падение процесса.
- Масштабирование — несколько инстансов должны согласованно владеть задачами.
- Доставка — нужны acknowledgement, redelivery, dead letter и наблюдаемая
  семантика at-least-once.
- Альтернатива — durable broker или таблица задач вместе с idempotent handler.

---

## Связанные материалы

- [Fetcher с кэшем](./01-fetcher-with-cache.md) — конечный batch вместо
  долгоживущего processor’а.
- [Worker Pool](../concurrency/01-worker-pool.md) — базовая организация worker’ов.
- [Retry с backoff](../system-primitives/02-retry-with-backoff.md) — классификация
  ошибок, jitter и лимиты.
- [Graceful Shutdown](../../../04-architecture-and-patterns/patterns/08-graceful-shutdown.md) — общий lifecycle остановки.
- [Background Workers](../../../04-architecture-and-patterns/patterns/04-background-workers.md) — durability и operational trade-offs.
