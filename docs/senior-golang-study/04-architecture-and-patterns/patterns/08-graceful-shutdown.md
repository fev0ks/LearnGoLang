# Graceful Shutdown в Go

## Содержание

- [Зачем это важно](#зачем-это-важно)
- [Какие сигналы важны для сервиса](#какие-сигналы-важны-для-сервиса)
- [Перехват сигналов OS](#перехват-сигналов-os)
- [HTTP-сервер: srv.Shutdown](#http-сервер-srvshutdown)
- [gRPC-сервер: GracefulStop и его нюансы](#grpc-сервер-gracefulstop-и-его-нюансы)
- [Горутины, сбежавшие из запроса](#горутины-сбежавшие-из-запроса)
- [Фоновый воркер / пул горутин](#фоновый-воркер--пул-горутин)
- [Оркестрация нескольких компонентов](#оркестрация-нескольких-компонентов)
- [Таймауты shutdown](#таймауты-shutdown)
- [Частые ошибки](#частые-ошибки)
- [Проверка корректности в тестах](#проверка-корректности-в-тестах)
- [Graceful shutdown vs Force kill](#graceful-shutdown-vs-force-kill)
- [Interview-ready answer](#interview-ready-answer)

Graceful shutdown — управляемое завершение процесса: сервис перестаёт принимать новую работу, даёт текущей работе ограниченное время завершиться и закрывает ресурсы. Это уменьшает число оборванных запросов и частично выполненных задач, но не заменяет идемпотентность и надёжное хранилище (`durable storage`): SIGKILL, crash или исчерпанный deadline всё равно могут оборвать процесс.

---

## Зачем это важно

```
Без graceful shutdown:
  SIGTERM → процесс убит мгновенно
  → Запросы в flight обрываются с 502
  → Воркеры бросают задачу на полпути
  → Незавершённая транзакция в БД откатится при разрыве соединения,
    но внешний побочный эффект (side effect) или работа вне транзакции
    могут остаться частичными
  → Клиент retry → дубли → нарушение идемпотентности

С graceful shutdown:
  SIGTERM → принят сигнал
  → Перестать принимать новые запросы
  → Дождаться завершения текущих
  → Закрыть соединения и сбросить буферы
  → Вернуть управление в main с результатом shutdown
```

**Когда приходит сигнал остановки:**

- при rollout или удалении пода kubelet через container runtime отправляет процессу SIGTERM;
- `docker stop` на Linux по умолчанию оставляет процессу 10 секунд до SIGKILL, если таймаут не переопределён;
- systemd отправляет сигнал при остановке сервиса на VM;
- `Ctrl+C` в терминале отправляет SIGINT.

---

## Какие сигналы важны для сервиса

Сигнал — это асинхронное уведомление процессу от ядра ОС. У каждого сигнала есть действие по умолчанию (default disposition); часть из них процесс может перехватить и обработать по-своему, часть — нет.

| Сигнал | № | Кто и когда шлёт | Default | Ловится? |
|---|---|---|---|---|
| SIGTERM | 15 | kubelet/container runtime, `docker stop`, systemd, `kill <pid>` | Завершить | Да — сюда вешают graceful shutdown |
| SIGINT | 2 | `Ctrl+C` в терминале | Завершить | Да — обычно так же, как SIGTERM |
| SIGKILL | 9 | ядро при OOM, `kill -9`, K8s после grace period | Убить немедленно | Нет — процесс не получает управление |
| SIGQUIT | 3 | `Ctrl+\` | Завершить с дампом | Да — Go печатает стеки всех горутин и завершается |
| SIGHUP | 1 | закрытие терминала; по конвенции — «перечитать конфиг» | Завершить | Да — многие демоны используют для reload без рестарта |
| SIGABRT | 6 | `abort()` или native-библиотека | Завершить с дампом | Да, но обычно не перехватывают |

Что из этого следует для graceful shutdown:

- **SIGTERM — основной сигнал остановки.** Именно его шлёт Kubernetes и `docker stop`. Ловить нужно в первую очередь его; SIGINT добавляют для удобства локального `Ctrl+C`.
- **SIGKILL перехватить невозможно** — ни один язык этого не может, сигнал обрабатывается ядром, процесс не получает управления и не выполняет ни defer'ы, ни shutdown-логику. Поэтому весь смысл grace period: успеть завершиться по SIGTERM до того, как прилетит SIGKILL.
- **SIGSTOP** (пауза процесса) тоже неперехватываемый, как и SIGKILL, — их два таких.
- **Go-рантайм сам обрабатывает часть сигналов** раньше прикладного кода: SIGQUIT печатает дамп всех горутин, что удобно для диагностики зависаний через `kill -QUIT <pid>`. Для SIGPIPE поведение зависит от файлового дескриптора: запись в закрытый pipe через stdout/stderr обычно завершает процесс, а для других дескрипторов операция возвращает `EPIPE`. После подписки через `signal.Notify` поведение по умолчанию для зарегистрированного сигнала отключается, и реагировать обязан уже прикладной код.

```
kubectl delete pod / rollout:
  1. Начинается terminationGracePeriodSeconds (по умолчанию 30s)
  2. Выполняется preStop, если он настроен
  3. SIGTERM  ──► приложению доступен оставшийся grace period
  4. ...ждём graceful shutdown...
  5. SIGKILL  ──► если не уложился — убивают принудительно, defer'ы не выполнятся
```

---

## Перехват сигналов OS

### `signal.NotifyContext` (Go 1.16+, рекомендуемый способ)

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGTERM, // K8s, systemd
        syscall.SIGINT,  // Ctrl+C
    )
    defer stop()

    // Первый сигнал отменяет ctx. Сразу после этого снимаем подписку,
    // чтобы второй SIGINT/SIGTERM сработал по правилу ОС и завершил процесс.
    go func() {
        <-ctx.Done()
        stop()
    }()

    // ctx — сигнал начать shutdown, а не контекст каждой текущей операции.
    if err := run(ctx); err != nil {
        slog.Error("service stopped", "err", err)
        os.Exit(1)
    }
}
```

`ctx.Done()` закрывается при получении первого сигнала. Вызов `stop()` снимает подписку и восстанавливает стандартное поведение SIGINT/SIGTERM. Поэтому следующий сигнал уже завершает процесс немедленно. Сам по себе повторный вызов `stop()` никакой сигнал не отправляет.

Контекст сигнала нельзя бездумно передавать каждой текущей операции. Если HTTP-запрос или обработчик сообщения получит этот же контекст, первый SIGTERM отменит работу вместо graceful drain. Компоненту обычно нужны два события:

1. первый сигнал — перестать принимать новую работу и дождаться текущей;
2. истечение shutdown deadline — отменить оставшуюся работу принудительно.

### Старый способ через канал (для понимания)

```go
quit := make(chan os.Signal, 1) // буфер обязателен, см. ниже
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit
// начать shutdown...
signal.Stop(quit) // перестать получать сигналы
```

**Почему канал буферизованный (`cap 1`).** `signal.Notify` шлёт в канал неблокирующе: если получатель в этот момент не читает и буфера нет, сигнал будет потерян. Буфер на 1 гарантирует, что сигнал, пришедший до `<-quit`, не пропадёт. Больше 1 обычно не нужно — по одному сигналу на остановку достаточно. `signal.NotifyContext` эту деталь берёт на себя.

`syscall.SIGINT`/`syscall.SIGTERM` можно писать и как `os.Interrupt` (SIGINT) — кроссплатформенный алиас; но `os.Interrupt` покрывает только SIGINT, а SIGTERM константы в `os` нет, поэтому для сервисов явно используют `syscall.SIGTERM`.

---

## HTTP-сервер: `srv.Shutdown`

`http.Server.Shutdown(ctx)` закрывает listener, перестаёт принимать новые соединения, закрывает idle keep-alive соединения и ждёт завершения активных запросов.

```go
func serveHTTP(shutdownSignal context.Context, srv *http.Server) error {
    serveErr := make(chan error, 1)
    go func() {
        err := srv.ListenAndServe()
        if errors.Is(err, http.ErrServerClosed) {
            err = nil
        }
        serveErr <- err
    }()

    select {
    case err := <-serveErr:
        return err
    case <-shutdownSignal.Done():
    }

    drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(drainCtx); err != nil {
        // Deadline Shutdown только прекращает ожидание. Активные соединения
        // он сам не рвёт, поэтому жёсткий fallback вызывается явно.
        closeErr := srv.Close()
        return errors.Join(fmt.Errorf("http drain: %w", err), closeErr)
    }

    return <-serveErr
}
```

**Ключевой момент.** `http.Server` сам ведёт учёт активных соединений через `ConnState` (`new → active → idle → closed`). Незавершённые HTTP-запросы считать вручную не нужно.

Контекст, переданный в `Shutdown`, ограничивает только ожидание самого метода. Он не становится `r.Context()` активных запросов и не отменяет handlers при истечении deadline. Длинные операции должны иметь собственные прикладные таймауты и реагировать на `r.Context()`, а после исчерпания общего бюджета сервер вызывает `Close()` как жёсткий fallback.

**Слепая зона `Shutdown`: hijacked-соединения (WebSocket).** Официальная документация прямо говорит: «Shutdown does not attempt to close nor wait for hijacked connections such as WebSockets». После `Hijack()` соединение выходит из-под учёта `ConnState`, и `Shutdown` про него не знает — ни закрыть, ни дождаться. Для таких соединений есть `srv.RegisterOnShutdown(f)`: колбэк вызывается при старте `Shutdown` и должен инициировать закрытие long-lived соединений — послать close frame и отменить их контекст, — но не ждать его. Ожидание выполняется отдельно через собственный `WaitGroup`:

```go
wsConns := NewConnRegistry() // свой учёт активных WebSocket-соединений

srv.RegisterOnShutdown(func() {
    wsConns.CloseAll() // инициировать закрытие, не блокируясь
})

// В shutdown-последовательности один deadline ограничивает обе части:
httpErr := srv.Shutdown(sdCtx)                // обычные HTTP-запросы
if httpErr != nil {
    _ = srv.Close()                            // жёстко закрыть остаток HTTP
}
wsErr := waitWithContext(sdCtx, wsConns.Wait) // hijacked-соединения
if wsErr != nil {
    wsConns.ForceCloseAll()
}
```

`waitWithContext` запускает переданный `Wait` в горутине и возвращает `ctx.Err()`, если deadline закончился. Сам реестр должен уметь принудительно закрыть оставшиеся соединения: одного `WaitGroup` для bounded shutdown недостаточно.

```go
func waitWithContext(ctx context.Context, wait func()) error {
    done := make(chan struct{})
    go func() {
        wait()
        close(done)
    }()

    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

Отсюда общее правило: горутину, порождённую сервером (HTTP-запрос, gRPC RPC), считает сам сервер; горутину, порождённую прикладным кодом, приходится считать вручную. Ниже — про второй случай.

---

## gRPC-сервер: `GracefulStop` и его нюансы

`grpc.Server.GracefulStop()` решает ту же задачу, что и `http.Server.Shutdown`: перестать принимать новые вызовы, дождаться завершения тех, что в полёте. Но API и гарантии заметно отличаются, и это ловит многих.

| | `http.Server.Shutdown(ctx)` | `grpc.Server.GracefulStop()` |
|---|---|---|
| Принимает контекст/таймаут | Да — `ctx` ограничивает ожидание | Нет — ждёт бесконечно |
| Что делает | закрывает listener, ждёт возврата хендлеров, закрывает idle keep-alive | закрывает listener, ждёт завершения всех in-flight RPC (включая стримы), закрывает соединения |
| «Жёсткая» версия | `srv.Close()` — рвёт всё немедленно | `srv.Stop()` — отменяет RPC и рвёт соединения |
| Поведение при зависшем вызове | вернёт `context.DeadlineExceeded` по дедлайну | зависнет вместе с вызовом |

### Главная ловушка: у `GracefulStop()` нет таймаута

`http.Server.Shutdown` ограничивают через `context.WithTimeout`. У `GracefulStop()` параметра-контекста нет вообще — он будет ждать зависший RPC ровно столько, сколько тот висит, то есть потенциально бесконечно.

В Kubernetes это выглядит так: под получил SIGTERM, застрял в `GracefulStop()`, выработал `terminationGracePeriodSeconds` — и прилетел SIGKILL, который оборвёт всё жёстко, включая закрытие БД и flush буферов. Формально был graceful-вызов, а по факту — force kill.

Каноничное решение — ограничить `GracefulStop()` таймаутом самостоятельно, с фолбэком на `Stop()`:

```go
func stopGRPC(ctx context.Context, log *slog.Logger, srv *grpc.Server) error {
    stopped := make(chan struct{})
    go func() {
        srv.GracefulStop() // ждёт завершения in-flight RPC
        close(stopped)
    }()

    select {
    case <-stopped:
        return nil
    case <-ctx.Done():
        log.Warn("grpc graceful stop timeout, forcing")
        srv.Stop()  // отменяет контексты RPC и рвёт соединения
        <-stopped   // после Stop() GracefulStop в горутине разблокируется
        return ctx.Err()
    }
}
```

`Stop()` отменяет контексты активных RPC и закрывает соединения — после него `GracefulStop()` в горутине разблокируется, поэтому `<-stopped` в конце безопасен. Таймаут держат меньше `terminationGracePeriodSeconds`, чтобы после форса ещё успели отработать закрытие ресурсов до SIGKILL.

### Стримы не закрываются сами

`GracefulStop()` ждёт завершения и серверных стримов. Обычный unary-RPC обычно возвращается быстро, а долгоживущий server-stream может не закрыться сам. При этом `GracefulStop()` не отменяет контекст RPC.

Если стрим должен завершаться мягко раньше deadline, приложение передаёт ему отдельный shutdown-сигнал, например через сервисный контекст или закрываемый канал. Контекст самого RPC остаётся полезен для отмены со стороны клиента. По истечении общего бюджета `Stop()` принудительно отменяет контексты оставшихся RPC.

### Health check: NOT_SERVING до GracefulStop

Если сервис отдаёт стандартный `grpc_health_v1`, перед `GracefulStop()` статус переключают в `NOT_SERVING`. Клиенты с health-aware балансировкой перестают слать новые вызовы ещё до того, как сервер разошлёт GOAWAY, — drain получается мягче:

```go
healthServer.SetServingStatus("readiness", healthpb.HealthCheckResponse_NOT_SERVING)

grpcCtx, cancel := context.WithTimeout(shutdownCtx, 20*time.Second)
defer cancel()
_ = stopGRPC(grpcCtx, log, grpcSrv)
```

Это влияет на Kubernetes только тогда, когда `grpc` readiness probe проверяет то же имя сервиса, здесь `readiness`. Liveness и readiness лучше разделять: намеренно проваливать liveness во время обычного drain не нужно, иначе kubelet может воспринять остановку как неисправность и перезапустить контейнер.

### In-flight RPC не знают, что идёт shutdown

`GracefulStop()` дожидается хендлеров, но не сообщает им, что пора закругляться: у каждого RPC свой контекст, привязанный к вызову, а не к жизни сервера. Это симметрично HTTP: как `srv.Shutdown` не отменяет `r.Context()` до дедлайна, так и `GracefulStop()` не отменяет контексты активных RPC (это делает уже `Stop()`). И так же, как в HTTP, фоновые горутины, запущенные из RPC-хендлера и не дождавшиеся завершения, сервер не посчитает — их нужно трекать вручную (см. [Горутины, сбежавшие из запроса](#горутины-сбежавшие-из-запроса)).

---

## Горутины, сбежавшие из запроса

Самая частая дыра. Хендлер отвечает `202 Accepted` и доделывает работу в фоне:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    go sendEmail(r.Context(), user) // ← сразу две ошибки
    w.WriteHeader(http.StatusAccepted)
}
```

Что здесь не так:

1. `srv.Shutdown` считает соединение завершённым, как только хендлер вернулся. Горутина `sendEmail` для сервера невидима — её оборвёт `SIGKILL`.
2. Для входящего HTTP-запроса `r.Context()` отменяется, когда `ServeHTTP` возвращает, клиент закрывает соединение или отменяет запрос. Фоновая работа с этим контекстом может завершиться сразу после возврата handler, ещё до всякого shutdown.

Такие «сбежавшие» горутины приходится считать самостоятельно. Трекер должен не только хранить `WaitGroup`, но и атомарно запрещать новые задачи перед `Wait`: иначе конкурентный `Add` может пересечься с ожиданием.

```go
type Tasks struct {
    mu        sync.Mutex
    accepting bool
    wg        sync.WaitGroup
    base      context.Context
    cancel    context.CancelFunc
}

func NewTasks() *Tasks {
    base, cancel := context.WithCancel(context.Background())
    return &Tasks{accepting: true, base: base, cancel: cancel}
}

// Go возвращает false, если shutdown уже запретил новые задачи.
func (t *Tasks) Go(fn func(ctx context.Context)) bool {
    t.mu.Lock()
    if !t.accepting {
        t.mu.Unlock()
        return false
    }
    t.wg.Add(1) // Add защищён тем же mutex, что переход в closing
    t.mu.Unlock()

    go func() {
        defer t.wg.Done()
        fn(t.base)
    }()

    return true
}

func (t *Tasks) Shutdown(drainCtx, hardCtx context.Context) error {
    t.mu.Lock()
    t.accepting = false
    t.mu.Unlock()

    done := make(chan struct{})
    go func() {
        t.wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        t.cancel() // освобождаем ресурсы базового контекста
        return nil
    case <-drainCtx.Done():
        t.cancel() // жёстко отменяем задачи, которые ещё работают
    }

    select {
    case <-done:
        return drainCtx.Err()
    case <-hardCtx.Done():
        return errors.Join(drainCtx.Err(), hardCtx.Err())
    }
}
```

В хендлере вместо голого `go` — регистрация в трекере:

```go
func handler(tasks *Tasks) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user := parse(r)
        if !tasks.Go(func(ctx context.Context) { sendEmail(ctx, user) }) {
            http.Error(w, "shutting down", http.StatusServiceUnavailable)
            return
        }
        w.WriteHeader(http.StatusAccepted)
    }
}
```

Порядок в shutdown строгий: сначала `srv.Shutdown`, чтобы HTTP-handlers перестали создавать задачи, затем `tasks.Shutdown` запрещает другие источники и ждёт уже зарегистрированную работу:

```go
_ = srv.Shutdown(httpCtx)                      // 1. handlers больше не создают задачи
_ = tasks.Shutdown(taskCtx, globalShutdownCtx) // 2. drain, затем force-cancel
```

Такой трекер делает плановый shutdown аккуратнее, но не превращает память процесса в надёжную очередь. После ответа `202 Accepted` процесс всё ещё может получить SIGKILL, упасть или потерять питание. Если задача обязана сохраниться, до ответа клиенту её записывают в durable queue либо в outbox внутри транзакции.

`taskCtx` ограничивает мягкое ожидание. По его deadline трекер отменяет базовый контекст задач, а `globalShutdownCtx` оставляет им небольшой срок обработать отмену и одновременно ограничивает весь shutdown.

---

## Фоновый воркер / пул горутин

Второй тип собственных горутин — постоянный цикл обработки: consumer брокера или периодическая задача. Для graceful drain одного контекста недостаточно. Отмена контекста чтения должна прекратить получение новых сообщений, но не отменить уже выполняющийся `handle`. Отдельный рабочий контекст отменяется только после deadline.

```go
type Worker struct {
    broker MessageBroker
    log    *slog.Logger
    handle func(context.Context, Message) error

    receiveCtx     context.Context
    stopReceiving context.CancelFunc
    workCtx        context.Context
    forceWork      context.CancelFunc

    stopOnce  sync.Once
    forceOnce sync.Once
    wg        sync.WaitGroup
}

func NewWorker(broker MessageBroker, log *slog.Logger,
    handle func(context.Context, Message) error,
) *Worker {
    receiveCtx, stopReceiving := context.WithCancel(context.Background())
    workCtx, forceWork := context.WithCancel(context.Background())

    return &Worker{
        broker: broker, log: log, handle: handle,
        receiveCtx: receiveCtx, stopReceiving: stopReceiving,
        workCtx: workCtx, forceWork: forceWork,
    }
}

func (w *Worker) Start(size int) {
    for i := 0; i < size; i++ {
        w.wg.Add(1) // Add перед go
        go func() {
            defer w.wg.Done()
            w.processLoop()
        }()
    }
}

func (w *Worker) processLoop() {
    for {
        msg, err := w.broker.Receive(w.receiveCtx)
        if err != nil {
            if w.receiveCtx.Err() != nil {
                w.log.Info("worker stopping")
                return
            }
            w.log.Error("receive error", "err", err)

            select {
            case <-time.After(time.Second):
            case <-w.receiveCtx.Done():
                return
            }
            continue
        }

        // Первый SIGTERM не отменяет workCtx: полученная задача может завершиться.
        if err := w.handle(w.workCtx, msg); err != nil {
            w.broker.Nack(msg)
        } else {
            w.broker.Ack(msg)
        }
    }
}

func (w *Worker) Shutdown(drainCtx, hardCtx context.Context) error {
    w.stopOnce.Do(w.stopReceiving) // больше не получаем сообщения

    done := make(chan struct{})
    go func() {
        w.wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        w.forceOnce.Do(w.forceWork)
        return nil
    case <-drainCtx.Done():
        w.forceOnce.Do(w.forceWork) // отменяем оставшиеся handle
    }

    select {
    case <-done:
        return drainCtx.Err()
    case <-hardCtx.Done():
        return errors.Join(drainCtx.Err(), hardCtx.Err())
    }
}
```

Разница с трекером `Tasks`: у воркера фиксированное число циклов, а число сообщений меняется. `stopReceiving` прекращает intake, `WaitGroup` считает циклы вместе с их текущими задачами, а `forceWork` ограничивает drain общим deadline. Метод `Receive` обязан реагировать на отмену переданного контекста, иначе остановить intake этим способом нельзя.

`drainCtx` задаёт время на нормальное завершение задачи. После его deadline текущая работа получает отмену, а `hardCtx` ограничивает ожидание реакции на неё. Если обработчик игнорирует рабочий контекст, метод вернёт ошибку по общему deadline: гарантировать graceful completion такого кода приложение уже не может. Пример предполагает один вызов `Start` и один lifecycle; повторный запуск требует отдельной защиты состояния.

---

## Оркестрация нескольких компонентов

Реальный сервис содержит HTTP-сервер, gRPC-сервер, фоновые задачи и consumer брокера. Серверы создают работу, а работа использует БД, поэтому выключать компоненты нужно в обратном порядке зависимостей.

```go
func run(shutdownSignal context.Context) error {
    // --- Инициализация ---
    db, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
    if err != nil {
        return fmt.Errorf("create db pool: %w", err)
    }
    tasks := NewTasks()
    worker := NewWorker(broker, logger, handleMessage(db))
    worker.Start(5)

    httpSrv := newHTTPServer(router(tasks, db))
    grpcSrv := newGRPCServer()

    // Ошибка любого server loop тоже запускает полную остановку.
    serveErr := make(chan error, 2)
    go func() {
        err := httpSrv.ListenAndServe()
        if errors.Is(err, http.ErrServerClosed) {
            err = nil
        }
        if err != nil {
            err = fmt.Errorf("http serve: %w", err)
        }
        serveErr <- err
    }()
    go func() {
        err := grpcSrv.Serve(lis)
        if errors.Is(err, grpc.ErrServerStopped) {
            err = nil
        }
        if err != nil {
            err = fmt.Errorf("grpc serve: %w", err)
        }
        serveErr <- err
    }()

    var serveCause error
    select {
    case <-shutdownSignal.Done():
        logger.Info("shutdown signal received")
    case serveCause = <-serveErr:
        if serveCause != nil {
            logger.Error("server stopped unexpectedly", "err", serveCause)
        } else {
            logger.Warn("server stopped unexpectedly")
        }
    }

    // 50 секунд — общий бюджет приложения внутри 60-секундного K8s grace period.
    shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 50*time.Second)
    defer cancelShutdown()

    var shutdownErr error

    // Фаза 1: оба источника перестают принимать работу и drain-ятся параллельно.
    healthServer.SetServingStatus("readiness", healthpb.HealthCheckResponse_NOT_SERVING)
    sourcePhaseCtx, cancelSources := context.WithTimeout(shutdownCtx, 20*time.Second)
    sourceGroup, sourceCtx := errgroup.WithContext(sourcePhaseCtx)
    sourceGroup.Go(func() error {
        if err := httpSrv.Shutdown(sourceCtx); err != nil {
            return errors.Join(err, httpSrv.Close())
        }
        return nil
    })
    sourceGroup.Go(func() error {
        return stopGRPC(sourceCtx, logger, grpcSrv)
    })
    shutdownErr = errors.Join(shutdownErr, sourceGroup.Wait())
    cancelSources()

    // Фаза 2: handlers завершены, новых прикладных задач больше нет.
    taskCtx, cancelTasks := context.WithTimeout(shutdownCtx, 10*time.Second)
    shutdownErr = errors.Join(shutdownErr, tasks.Shutdown(taskCtx, shutdownCtx))
    cancelTasks()

    // Фаза 3: прекращаем intake брокера и ждём текущие сообщения.
    workerCtx, cancelWorkers := context.WithTimeout(shutdownCtx, 15*time.Second)
    shutdownErr = errors.Join(shutdownErr, worker.Shutdown(workerCtx, shutdownCtx))
    cancelWorkers()

    // БД закрывается последней и тоже ограничена оставшимся глобальным бюджетом.
    shutdownErr = errors.Join(shutdownErr, waitWithContext(shutdownCtx, db.Close))
    return errors.Join(serveCause, shutdownErr)
}
```

HTTP и gRPC независимы друг от друга как источники, поэтому они drain-ятся параллельно. `Tasks`, worker и БД зависят от предыдущих фаз и останавливаются последовательно. Ошибки `ListenAndServe` и `grpc.Serve` не теряются: неожиданное завершение любого сервера запускает тот же shutdown flow.

```
Порядок остановки при зависимости «HTTP ставит задачи воркерам»:

  1. HTTP/gRPC drain       — новые handlers больше не появляются
  2. tasks.Shutdown(ctx)  — ждём прикладные горутины из handlers
  3. worker.Shutdown(ctx) — прекращаем intake и ждём текущие сообщения
  4. db.Close()           — соединения закрываются последними
```

Общее правило порядка: останавливать в направлении, обратном потоку данных, — сначала источники, затем обработчики, в конце хранилища и соединения.

---

## Таймауты shutdown

Структура таймаутов должна соответствовать зависимостям между компонентами. Параллельные независимые операции дают максимум своих длительностей, а последовательные фазы складываются.

```
K8s terminationGracePeriodSeconds: 60s
  ├── preStop для распространения endpoint update: до 5s
  ├── общий бюджет приложения после SIGTERM: 50s
  │   ├── max(HTTP drain, gRPC drain): до 20s — параллельно
  │   ├── фоновые Tasks: до 10s          — после handlers
  │   ├── worker drain: до 15s           — после Tasks
  │   └── DB/telemetry cleanup: до 5s
  └── резерв до SIGKILL: 5s

Проверка: 5s + 20s + 10s + 15s + 5s + 5s = 60s
```

Это верхние границы фаз, а не обещание тратить весь бюджет каждый раз: если HTTP завершился за секунду, следующая фаза начинается сразу. Один глобальный контекст не даёт сумме фаз выйти за 50 секунд, а локальные контексты ограничивают отдельные этапы. Если мягкий deadline фазы исчерпан, обработка принудительной отмены использует остаток общего бюджета; следующие фазы получают меньше времени.

```go
// shutdownSignal уже отменён, поэтому shutdownCtx нельзя наследовать от него:
// дочерний контекст тоже оказался бы отменён немедленно.
shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
defer cancel()

// Так делать нельзя:
<-shutdownSignal.Done()
os.Exit(0) // горутины и defer не успеют завершиться
```

### K8s: SIGTERM и удаление из endpoints идут параллельно

При завершении пода control plane начинает помечать endpoint как terminating и `ready=false`, а kubelet запускает `preStop` и затем отправляет SIGTERM процессу. Эти ветки termination flow не дают атомарной синхронизации между EndpointSlice, kube-proxy, ingress и внешним load balancer. Некоторое время новые соединения ещё могут направляться в завершающийся под.

Один из практических способов оставить инфраструктуре время на распространение изменений — `preStop` со sleep. Хук выполняется раньше SIGTERM, поэтому приложение пока не закрыло listener:

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep", "5"]
```

Фиксированные пять секунд не являются гарантией и не нужны автоматически каждому сервису. Задержку выбирают по наблюдаемому времени обновления конкретных ingress/LB; иногда достаточно стандартного endpoint termination flow. Время `preStop` уже входит в `terminationGracePeriodSeconds`, поэтому оно уменьшает бюджет приложения.

---

## Частые ошибки

| Ошибка | Последствие | Правило |
|---|---|---|
| `os.Exit(0)` после сигнала | defer'ы не выполняются, горутины брошены | Выполнить shutdown flow и вернуться из `main` |
| Не вызвать `cancel()` у timeout context | Timer и связанные ресурсы живут до deadline | Вызывать `cancel`, когда контекст больше не нужен |
| Shutdown context без таймаута | Зависает если воркер застрял | Всегда `WithTimeout` |
| Передать signal context в текущую работу | Первый SIGTERM обрывает handlers и задачи вместо drain | Разделять stop-accepting и force-cancel |
| Закрыть канал, пока producers ещё отправляют | `panic: send on closed channel` | Остановить producers, дождаться активных `Send`, затем закрыть очередь |
| WaitGroup.Add внутри горутины | Race: Wait может вернуться раньше | Add перед go |
| `go fn(r.Context())` в хендлере | Задача умирает при возврате ответа + не считается Shutdown | Свой базовый контекст + трекер на WaitGroup |
| Ответить `202`, сохранив задачу только в памяти | Работа теряется при crash/SIGKILL | Durable queue или transactional outbox до ответа |
| `grpcSrv.GracefulStop()` без таймаута | Зависший RPC/стрим блокирует shutdown до SIGKILL | Обернуть в таймаут + `Stop()`-фолбэк |
| Игнорировать результат `ListenAndServe`/`grpc.Serve` | Процесс остаётся живым без одного из серверов | Ошибка server loop запускает общий shutdown |
| Ждать WebSocket через `srv.Shutdown` | Shutdown не видит hijacked-соединения | `RegisterOnShutdown` + свой учёт |
| Считать `preStop: sleep 5` универсальной гарантией | Лишняя задержка или недостаточный drain при другом LB | Измерять propagation delay и включать его в общий бюджет |
| Игнорировать SIGTERM, ловить только SIGINT | Сервис не деплоится корректно в K8s | Ловить оба |

---

## Проверка корректности в тестах

```go
func TestWorkerShutdownDrainsInflightMessage(t *testing.T) {
    broker := newFakeBroker(Message{ID: "msg-1"})
    started := make(chan struct{})
    release := make(chan struct{})

    worker := NewWorker(broker, testLogger, func(ctx context.Context, msg Message) error {
        close(started)
        select {
        case <-release:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    })
    worker.Start(1)

    <-started // handler уже обрабатывает конкретное сообщение

    shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    done := make(chan error, 1)
    go func() {
        done <- worker.Shutdown(shutdownCtx, shutdownCtx)
    }()

    // Shutdown остановил intake, но не должен отменить текущий handler.
    select {
    case err := <-done:
        t.Fatalf("shutdown returned before in-flight task completed: %v", err)
    case <-time.After(50 * time.Millisecond):
    }

    close(release)
    if err := <-done; err != nil {
        t.Fatalf("shutdown worker: %v", err)
    }
    if !broker.Acked("msg-1") {
        t.Fatal("completed message was not acknowledged")
    }
}
```

Fake broker в этом тесте должен выдать одно сообщение, а следующий `Receive` держать до отмены `receiveCtx`. Отдельными тестами проверяют жёсткую отмену по deadline, ошибку цикла сервера и завершение активного HTTP-запроса. Тесты конкурентности полезно запускать с `go test -race`.

---

## Graceful shutdown vs Force kill

`signal.NotifyContext` не превращает второй сигнал в force-exit автоматически. Пока подписка активна, runtime направляет зарегистрированные сигналы в неё, а уже отменённый контекст не может отмениться ещё раз.

В примере из начала статьи после первого сигнала отдельная горутина вызывает `stop()`. Это восстанавливает стандартное поведение ОС:

```text
Первый Ctrl+C  → ctx отменён → начинается graceful shutdown → stop() снимает подписку
Второй Ctrl+C  → стандартный SIGINT → процесс завершается немедленно
```

Второй сигнал — осознанный аварийный выход: текущая работа и `defer` могут быть оборваны. Такой механизм удобен для локального запуска и ручного вмешательства, но корректность данных всё равно должна опираться на транзакции, идемпотентность, подтверждения брокера и durable storage.

---

## Interview-ready answer

**1. Как реализовать graceful shutdown Go-сервиса в Kubernetes?**

- Точка входа — `signal.NotifyContext` на SIGTERM и SIGINT. Этот контекст запускает shutdown flow, но не отменяет каждую текущую операцию немедленно.
- HTTP — `srv.Shutdown(ctx)` закрывает listener и ждёт handlers; по deadline вызывается `srv.Close()` как жёсткий fallback.
- gRPC — `GracefulStop()` запускается в горутине под `select`; по deadline вызывается `Stop()`.
- Воркеры — сначала отменяют blocking `Receive`, но сохраняют рабочий контекст текущей задачи. По deadline отменяют и его; `sync.WaitGroup` фиксирует завершение.
- Порядок — обратный потоку данных: сначала источники запросов, потом обработчики, в конце соединения с базой.

**2. Как таймауты связаны с настройками Kubernetes?**

- `terminationGracePeriodSeconds` включает `preStop`, работу приложения и резерв до SIGKILL.
- Длительности последовательных фаз складываются; для независимых параллельных фаз берётся максимум.
- Один глобальный deadline ограничивает весь shutdown, локальные deadline — отдельные фазы.
- `preStop` со sleep нужен только при измеренной задержке распространения endpoint update через конкретный ingress/LB; его длительность вычитается из бюджета приложения.

**3. Кто отслеживает незавершённые горутины при shutdown?**

- Ответ зависит от того, кто их породил.
- Сервер сам считает активные HTTP-запросы по внутреннему состоянию соединений и in-flight gRPC-вызовы; их закрывают `Shutdown` и `GracefulStop`.
- Вручную считать приходится — всё, что запустил прикладной код: фоновую работу после `202 Accepted`, воркеров, потребителей брокера.
- Такой горутине нельзя передавать `r.Context()`: он отменяется после возврата handler; нужен базовый контекст приложения.
- `sync.WaitGroup` требует `Add` до `go`, а переход в состояние «новые задачи запрещены» должен быть атомарным относительно `Add`.
- Трекер в памяти защищает только плановый shutdown. Для гарантии после `202 Accepted` нужна durable queue или transactional outbox.

**4. Почему `GracefulStop()` в gRPC — отдельная проблема?**

- Причина — у метода нет параметра-контекста, ограничить ожидание снаружи нечем.
- Последствие — зависший RPC или незакрытый серверный стрим держит процесс до SIGKILL, и graceful-остановка оказывается формальной.
- Решение — вызов в отдельной горутине, `select` с таймаутом и `Stop()` в фолбэке; после `Stop()` заблокированный `GracefulStop()` возвращается.
- Дополнительно — переключить readiness service в `NOT_SERVING`, не проваливая liveness намеренно.

**5. Какие соединения `srv.Shutdown` не закрывает?**

- Главное исключение — hijacked-соединения, прежде всего WebSocket: после `Hijack()` соединение выходит из учёта `ConnState`.
- Что об этом говорит документация — `Shutdown` такие соединения не закрывает и не дожидается.
- Решение — `srv.RegisterOnShutdown` инициирует закрытие (close frame, отмена контекста), не блокируясь.
- Ожидание — через собственный реестр соединений под deadline; после его истечения реестр принудительно закрывает остаток.
