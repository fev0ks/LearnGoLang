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

Graceful shutdown — корректное завершение процесса без потери данных и обрыва активных запросов. Тема выглядит второстепенной ровно до первого деплоя под нагрузкой: при каждом обновлении сервиса Kubernetes останавливает поды, и без корректной остановки каждый деплой стоит порции пятисотых у клиентов и брошенных на середине задач.

---

## Зачем это важно

```
Без graceful shutdown:
  SIGTERM → процесс убит мгновенно
  → Запросы в flight обрываются с 502
  → Воркеры бросают задачу на полпути
  → Транзакция в БД не откатилась (или не закомитилась)
  → Клиент retry → дубли → нарушение идемпотентности

С graceful shutdown:
  SIGTERM → принят сигнал
  → Перестать принимать новые запросы
  → Дождаться завершения текущих
  → Закрыть соединения и flush буферы
  → Выйти с кодом 0
```

**Когда приходит SIGTERM:**
- `kubectl rollout` — K8s посылает SIGTERM перед заменой пода
- `docker stop` — 10-секундный grace period перед SIGKILL
- Деплой на VM через systemd
- `Ctrl+C` в терминале (это SIGINT)

---

## Какие сигналы важны для сервиса

Сигнал — это асинхронное уведомление процессу от ядра ОС. У каждого сигнала есть действие по умолчанию (default disposition); часть из них процесс может перехватить и обработать по-своему, часть — нет.

| Сигнал | № | Кто и когда шлёт | Default | Ловится? |
|---|---|---|---|---|
| SIGTERM | 15 | `kubectl`, `docker stop`, systemd, `kill <pid>` | Завершить | Да — сюда вешают graceful shutdown |
| SIGINT | 2 | `Ctrl+C` в терминале | Завершить | Да — обычно так же, как SIGTERM |
| SIGKILL | 9 | ядро при OOM, `kill -9`, K8s после grace period | Убить немедленно | Нет — процесс не получает управление |
| SIGQUIT | 3 | `Ctrl+\` | Завершить + core dump | Да — Go печатает stack trace всех горутин и падает |
| SIGHUP | 1 | закрытие терминала; по конвенции — «перечитать конфиг» | Завершить | Да — многие демоны используют для reload без рестарта |
| SIGABRT | 6 | `abort()`, детектор дедлоков рантайма | Завершить + core dump | Да, но обычно не перехватывают |

Что из этого следует для graceful shutdown:

- **SIGTERM — основной сигнал остановки.** Именно его шлёт Kubernetes и `docker stop`. Ловить нужно в первую очередь его; SIGINT добавляют для удобства локального `Ctrl+C`.
- **SIGKILL перехватить невозможно** — ни один язык этого не может, сигнал обрабатывается ядром, процесс не получает управления и не выполняет ни defer'ы, ни shutdown-логику. Поэтому весь смысл grace period: успеть завершиться по SIGTERM до того, как прилетит SIGKILL.
- **SIGSTOP** (пауза процесса) тоже неперехватываемый, как и SIGKILL, — их два таких.
- **Go-рантайм сам обрабатывает часть сигналов** раньше прикладного кода: SIGQUIT печатает дамп всех горутин (удобно для диагностики зависаний — `kill -QUIT <pid>`), SIGPIPE при записи в закрытый сокет гасится внутри рантайма. После подписки через `signal.Notify` поведение по умолчанию для этого сигнала отключается, и реагировать обязан уже прикладной код.

```
kubectl delete pod / rollout:
  1. SIGTERM  ──► у процесса есть terminationGracePeriodSeconds (default 30s)
  2. ...ждём graceful shutdown...
  3. SIGKILL  ──► если не уложился — убивают принудительно, defer'ы не выполнятся
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
    defer stop() // освобождает ресурсы NotifyContext

    // Передаём ctx в весь сервис
    if err := run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

`ctx.Done()` закрывается при получении сигнала. `stop()` отменяет подписку (вторая `stop()` повторно отправит следующий сигнал напрямую процессу — полезно для force-kill).

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

`http.Server.Shutdown(ctx)` — закрывает listener (новые соединения не принимаются), ждёт завершения активных запросов, закрывает idle keep-alive соединения.

```go
func run(ctx context.Context) error {
    srv := &http.Server{
        Addr:         ":8080",
        Handler:      router,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    // Запускаем сервер в отдельной горутине
    errCh := make(chan error, 1)
    go func() {
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
    }()

    // Ждём сигнала или ошибки
    select {
    case err := <-errCh:
        return fmt.Errorf("server error: %w", err)
    case <-ctx.Done():
        // Сигнал получен, начинаем shutdown
    }

    // Даём активным запросам время завершиться
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("shutdown error: %w", err)
    }
    return nil
}
```

**Ключевой момент:** `http.Server` сам ведёт учёт активных соединений через `ConnState` (new → active → idle → closed). `srv.Shutdown` закрывает listener и ждёт, пока все хендлеры вернутся, — незавершённые запросы считать вручную не нужно. Единственное требование к коду: длинные операции внутри хендлера должны слушать `r.Context().Done()`, иначе они не впишутся в дедлайн shutdown-контекста и `Shutdown` вернёт ошибку по таймауту.

**Слепая зона `Shutdown`: hijacked-соединения (WebSocket).** Официальная документация прямо говорит: «Shutdown does not attempt to close nor wait for hijacked connections such as WebSockets». После `Hijack()` соединение выходит из-под учёта `ConnState`, и `Shutdown` про него не знает — ни закрыть, ни дождаться. Для таких соединений есть `srv.RegisterOnShutdown(f)`: колбэк вызывается при старте `Shutdown` и должен **инициировать** закрытие long-lived соединений (послать close frame, отменить их контекст), но не ждать его — ожидание делается отдельно, своим `WaitGroup`:

```go
wsConns := NewConnRegistry() // свой учёт активных WebSocket-соединений

srv.RegisterOnShutdown(func() {
    wsConns.CloseAll() // инициировать закрытие, не блокируясь
})

// в shutdown-последовательности:
_ = srv.Shutdown(sdCtx) // обычные HTTP-запросы
wsConns.Wait()          // hijacked-соединения — считаем сами
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
func stopGRPC(log *slog.Logger, srv *grpc.Server, timeout time.Duration) {
    stopped := make(chan struct{})
    go func() {
        srv.GracefulStop() // ждёт завершения in-flight RPC
        close(stopped)
    }()

    select {
    case <-stopped:
        // все RPC завершились сами
    case <-time.After(timeout):
        log.Warn("grpc graceful stop timeout, forcing")
        srv.Stop()  // отменяет контексты RPC и рвёт соединения
        <-stopped   // после Stop() GracefulStop в горутине разблокируется
    }
}
```

`Stop()` отменяет контексты активных RPC и закрывает соединения — после него `GracefulStop()` в горутине разблокируется, поэтому `<-stopped` в конце безопасен. Таймаут держат меньше `terminationGracePeriodSeconds`, чтобы после форса ещё успели отработать закрытие ресурсов до SIGKILL.

### Стримы не закрываются сами

`GracefulStop()` ждёт завершения и серверных стримов. Обычный unary-RPC вернётся быстро, а долгоживущий server-stream (подписка, наблюдение за событиями) сам не закроется — его хендлер должен реагировать на отмену контекста. Если стрим это игнорирует, `GracefulStop()` будет ждать его до бесконечности; именно такие вызовы и обрывает `Stop()` в фолбэке.

### Health check: NOT_SERVING до GracefulStop

Если сервис отдаёт стандартный `grpc_health_v1`, перед `GracefulStop()` статус переключают в `NOT_SERVING`. Клиенты с health-aware балансировкой перестают слать новые вызовы ещё до того, как сервер разошлёт GOAWAY, — drain получается мягче:

```go
healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
stopGRPC(log, grpcSrv, 20*time.Second)
```

Это же обновляет K8s `grpc` liveness/readiness probe — под быстрее выпадает из endpoints.

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
2. `r.Context()` отменяется, как только ответ записан и соединение закрыто. Фоновая работа с этим контекстом умрёт сразу, ещё до всякого shutdown.

Такие «сбежавшие» горутины приходится считать самостоятельно. Обычно это оформляют небольшим трекером уровня приложения на `sync.WaitGroup`, с собственным базовым контекстом вместо `r.Context()`:

```go
type Tasks struct {
    wg   sync.WaitGroup
    base context.Context // отменяется только при shutdown, живёт дольше запроса
}

// Go запускает фоновую задачу и регистрирует её в WaitGroup
func (t *Tasks) Go(fn func(ctx context.Context)) {
    t.wg.Add(1) // Add до go, а не внутри горутины
    go func() {
        defer t.wg.Done()
        fn(t.base)
    }()
}

func (t *Tasks) Wait() { t.wg.Wait() }
```

В хендлере вместо голого `go` — регистрация в трекере:

```go
func handler(tasks *Tasks) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user := parse(r)
        tasks.Go(func(ctx context.Context) { sendEmail(ctx, user) })
        w.WriteHeader(http.StatusAccepted)
    }
}
```

Порядок в shutdown строгий: сначала `srv.Shutdown` (чтобы перестали появляться новые фоновые задачи), потом `tasks.Wait()` (дождаться уже запущенных):

```go
_ = srv.Shutdown(sdCtx) // 1. дождались in-flight HTTP-запросов — их считает сервер
tasks.Wait()            // 2. дождались фоновых задач, что они породили — их считаем мы
```

`tasks.Wait()` тоже оборачивают в `select` с таймаутом (см. секцию с воркерами ниже), иначе одна зависшая задача не даст процессу выйти.

---

## Фоновый воркер / пул горутин

Второй тип собственных горутин — постоянный цикл обработки (consumer брокера, периодическая задача). Здесь `ctx.Done()` останавливает цикл, а `WaitGroup` фиксирует факт полного завершения:

```go
type Worker struct {
    broker MessageBroker
    log    *slog.Logger
    wg     sync.WaitGroup
}

func (w *Worker) Run(ctx context.Context) {
    for i := 0; i < 5; i++ {
        w.wg.Add(1) // Add перед go
        go func() {
            defer w.wg.Done()
            w.processLoop(ctx)
        }()
    }
}

func (w *Worker) processLoop(ctx context.Context) {
    for {
        msg, err := w.broker.Receive(ctx) // blocking; отменяется через ctx
        if err != nil {
            if errors.Is(err, context.Canceled) {
                w.log.Info("worker stopping")
                return
            }
            w.log.Error("receive error", "err", err)
            time.Sleep(time.Second)
            continue
        }

        if err := w.handle(ctx, msg); err != nil {
            w.broker.Nack(msg)
        } else {
            w.broker.Ack(msg)
        }
    }
}

func (w *Worker) Wait() { w.wg.Wait() }
```

Разница с трекером `Tasks`: у воркера фиксированное число горутин, которые сами читают `ctx`, — снаружи достаточно отменить контекст и дождаться `Wait()`. У `Tasks` число задач заранее неизвестно, поэтому `Add` вызывается на каждую.

---

## Оркестрация нескольких компонентов

Реальный сервис: HTTP-сервер + воркеры + gRPC-сервер + фоновые задачи. Все должны остановиться корректно.

```go
func run(ctx context.Context) error {
    // --- Инициализация ---
    db, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    defer db.Close()

    worker := NewWorker(db, broker, logger)
    httpSrv := newHTTPServer(router)
    grpcSrv := newGRPCServer()

    // --- Запуск компонентов ---
    g, gCtx := errgroup.WithContext(ctx)

    // HTTP-сервер
    g.Go(func() error {
        errCh := make(chan error, 1)
        go func() {
            if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
                errCh <- err
            }
        }()
        select {
        case err := <-errCh:
            return err
        case <-gCtx.Done():
            sdCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            return httpSrv.Shutdown(sdCtx)
        }
    })

    // gRPC-сервер
    g.Go(func() error {
        go grpcSrv.Serve(lis)
        <-gCtx.Done()
        stopGRPC(logger, grpcSrv, 30*time.Second) // GracefulStop с таймаутом + Stop-фолбэк
        return nil
    })

    // Воркеры
    g.Go(func() error {
        worker.Run(gCtx)
        <-gCtx.Done()

        // Даём воркерам 45 секунд завершить текущие задачи
        done := make(chan struct{})
        go func() {
            worker.Wait()
            close(done)
        }()

        select {
        case <-done:
            logger.Info("workers stopped gracefully")
        case <-time.After(45 * time.Second):
            logger.Warn("workers shutdown timeout")
        }
        return nil
    })

    return g.Wait()
}
```

`errgroup.WithContext` удобен тем, что при ошибке любого компонента `gCtx` отменяется — остальные тоже начинают shutdown.

У этой схемы есть свойство, которое легко упустить: все компоненты останавливаются одновременно. Для независимых частей это правильно и быстро, но между ними бывают зависимости. HTTP-обработчик, отвечающий `202 Accepted`, ставит задачу в очередь воркеров — значит воркеры должны пережить HTTP-сервер, а не останавливаться вместе с ним. Иначе задача, принятая за миг до сигнала, не будет ни выполнена, ни отклонена.

Там, где такая зависимость есть, порядок задаётся явно: сначала дождаться остановки принимающей стороны, потом останавливать обрабатывающую.

```
Порядок остановки при зависимости «HTTP ставит задачи воркерам»:

  1. srv.Shutdown(ctx)  — новых задач больше не появится
  2. tasks.Wait()       — фоновые задачи, порождённые запросами
  3. cancel(workerCtx)  — только теперь останавливаем воркеров
  4. worker.Wait()      — дожидаемся текущих задач
  5. db.Close()         — соединения закрываются последними
```

Общее правило порядка: останавливать в направлении, обратном потоку данных, — сначала источники, затем обработчики, в конце хранилища и соединения.

---

## Таймауты shutdown

Структура таймаутов должна соответствовать окружению:

```
K8s terminationGracePeriodSeconds: 60s
  └── SIGTERM → grace period начался
      ├── HTTP запросы: 30s (WriteTimeout сервера)
      ├── Воркеры: 45s (завершить текущую задачу)
      └── Итого: нужно уложиться в 60s до SIGKILL

Рекомендация:
  terminationGracePeriodSeconds = max(worker_timeout, http_timeout) + 10s buffer
```

```go
// Глобальный таймаут на весь процесс shutdown
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := run(ctx); err != nil {
        logger.Error("run failed", "err", err)
        os.Exit(1)
    }
}

// Так делать нельзя:
func main() {
    // ...
    <-ctx.Done()
    os.Exit(0) // ← горутины не успели завершиться
}
```

### K8s: SIGTERM и удаление из endpoints идут параллельно

Неочевидный момент: при удалении пода Kubernetes одновременно посылает SIGTERM и запускает удаление пода из Service endpoints. Эти процессы не синхронизированы — несколько секунд kube-proxy и ingress ещё шлют трафик в под, который уже начал shutdown и закрыл listener. Результат — всплеск 502/connection refused при каждом деплое, даже с идеальным graceful shutdown в коде.

Стандартное решение — хук `preStop` со sleep; он выполняется раньше отправки SIGTERM:

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep", "5"]
```

Последовательность становится правильной: под помечен на удаление → endpoints обновляются, трафик перестаёт приходить (пока идёт sleep) → SIGTERM → приложение спокойно дожидается уже принятых запросов. Время sleep входит в `terminationGracePeriodSeconds` — буфер нужно учитывать.

---

## Частые ошибки

| Ошибка | Последствие | Правило |
|---|---|---|
| `os.Exit(0)` после сигнала | defer'ы не выполняются, горутины брошены | Дождаться WaitGroup |
| Нет `defer cancel()` на shutdown context | Timer goroutine утекает | Всегда `defer cancel()` |
| Shutdown context без таймаута | Зависает если воркер застрял | Всегда `WithTimeout` |
| Закрыть канал до WaitGroup.Wait | Panic в воркерах | Сначала cancel, потом close |
| WaitGroup.Add внутри горутины | Race: Wait может вернуться раньше | Add перед go |
| `go fn(r.Context())` в хендлере | Задача умирает при возврате ответа + не считается Shutdown | Свой базовый контекст + трекер на WaitGroup |
| `grpcSrv.GracefulStop()` без таймаута | Зависший RPC/стрим блокирует shutdown до SIGKILL | Обернуть в таймаут + `Stop()`-фолбэк |
| Ждать WebSocket через `srv.Shutdown` | Shutdown не видит hijacked-соединения | `RegisterOnShutdown` + свой учёт |
| Нет `preStop: sleep` в K8s | Трафик летит в под после закрытия listener → 502 при деплое | `sleep 5` до SIGTERM |
| Игнорировать SIGTERM, ловить только SIGINT | Сервис не деплоится корректно в K8s | Ловить оба |

---

## Проверка корректности в тестах

```go
func TestGracefulShutdown(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    worker := NewWorker(...)
    started := make(chan struct{})

    go func() {
        close(started)
        worker.Run(ctx)
    }()

    <-started // убедились что воркер запущен

    cancel() // эмулируем SIGTERM

    done := make(chan struct{})
    go func() {
        worker.Wait()
        close(done)
    }()

    select {
    case <-done:
        // OK
    case <-time.After(5 * time.Second):
        t.Fatal("worker did not stop within timeout")
    }
}
```

---

## Graceful shutdown vs Force kill

```
signal.NotifyContext ловит второй SIGTERM/SIGINT:
  Первый Ctrl+C → ctx отменён, stop() вызван
  Второй Ctrl+C → сигнал доставлен напрямую процессу → immediate exit

Это стандартное поведение:
  - Один раз: graceful (ждём)
  - Два раза: force (убиваем немедленно)
```

---

## Interview-ready answer

**1. Как реализовать graceful shutdown Go-сервиса в Kubernetes?**

- Точка входа — `signal.NotifyContext` на SIGTERM и SIGINT; полученный контекст расходится по всем компонентам.
- HTTP — `srv.Shutdown(ctx)`: listener закрыт, активные запросы дожидаются ответа.
- gRPC — `GracefulStop()` в горутине под `select` с таймаутом и фолбэком на `Stop()`.
- Воркеры — читают `ctx.Done()`, дорабатывают текущую задачу, `sync.WaitGroup` фиксирует завершение.
- Порядок — обратный потоку данных: сначала источники запросов, потом обработчики, в конце соединения с базой.

**2. Как таймауты связаны с настройками Kubernetes?**

- Главное ограничение — суммарное время shutdown меньше `terminationGracePeriodSeconds`, иначе остаток работы обрывает SIGKILL.
- Запас — порядка десяти секунд: часть времени съедает сам `preStop`.
- Зачем `preStop: sleep 5` — SIGTERM и удаление пода из endpoints идут параллельно, и несколько секунд трафик ещё летит в под.
- Без этой паузы — 502 при каждом деплое даже при идеальном коде остановки.

**3. Кто отслеживает незавершённые горутины при shutdown?**

- Ответ зависит от того, кто их породил.
- Сервер сам считает — активные HTTP-запросы (через `ConnState`) и in-flight gRPC-вызовы; их закрывают `Shutdown` и `GracefulStop`.
- Вручную считать приходится — всё, что запустил прикладной код: фоновую работу после `202 Accepted`, воркеров, потребителей брокера.
- Отдельная ловушка — такой горутине нельзя передавать `r.Context()`: он отменяется сразу после записи ответа, нужен базовый контекст приложения.
- Оформление — `sync.WaitGroup` с `Add` перед `go`, а не внутри горутины.

**4. Почему `GracefulStop()` в gRPC — отдельная проблема?**

- Причина — у метода нет параметра-контекста, ограничить ожидание снаружи нечем.
- Последствие — зависший RPC или незакрытый серверный стрим держит процесс до SIGKILL, и graceful-остановка оказывается формальной.
- Решение — вызов в отдельной горутине, `select` с таймаутом и `Stop()` в фолбэке; после `Stop()` заблокированный `GracefulStop()` возвращается.
- Дополнительно — переключить health-статус в `NOT_SERVING` до остановки, чтобы балансировщик перестал слать новые вызовы заранее.

**5. Какие соединения `srv.Shutdown` не закрывает?**

- Главное исключение — hijacked-соединения, прежде всего WebSocket: после `Hijack()` соединение выходит из учёта `ConnState`.
- Что об этом говорит документация — `Shutdown` такие соединения не закрывает и не дожидается.
- Решение — `srv.RegisterOnShutdown` инициирует закрытие (close frame, отмена контекста), не блокируясь.
- Ожидание — через собственный реестр соединений с `WaitGroup`, уже после возврата `Shutdown`.
