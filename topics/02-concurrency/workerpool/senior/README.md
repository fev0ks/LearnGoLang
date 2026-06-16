# Senior: backpressure + dynamic resize (~190 LOC)

Уровень Avito/VK. Backpressure становится явным выбором, число воркеров
адаптируется к нагрузке. Middle-фундамент не ломается - расширяется.

## Запуск

```bash
go run ./cmd/workerpool/
go test -v -count=1 ./cmd/workerpool/
go test -race -count=1 ./cmd/workerpool/
go test -count=10 ./cmd/workerpool/   # flaky check
```

## Что добавлено к middle

| Middle | Senior |
|---|---|
| `Submit` всегда блокируется на полной очереди | `Config.OnFull`: `Block` (default) / `FailFast` / `DropNewest` |
| Фиксированное число воркеров | `Resize(n) (int, error)` через команд-канал |
| `DroppedErrors` counter | + `DroppedTasks` counter (для lossy-стратегий) |
| 3 case'а в worker `select` | + `quitOne` (4-й case) для кооперативного shrink |
| - | `WorkerID(ctx)` - `worker_id` в задаче для structured-логов |

Все 9 middle-тестов копируются в senior без изменений и остаются
зелёными.

## Public API (дельта к middle)

```go
type OnFullPolicy int

const (
    OnFullBlock      OnFullPolicy = iota // ждать слот (default; поведение middle)
    OnFullFailFast                        // вернуть ErrQueueFull сразу
    OnFullDropNewest                      // дропнуть + DroppedTasks++
)

type Config struct {
    Workers   int
    QueueSize int
    ErrBuf    int
    OnFull    OnFullPolicy // NEW; zero-value = OnFullBlock
}

// Меняет число воркеров. Синхронный, под mutex.
//   (n, nil)                  - изменение применено
//   (current, ErrPoolClosed)  - Shutdown уже шёл
//   (0, ErrResizeNonPositive) - n <= 0
func (p *Pool) Resize(n int) (int, error)

func (p *Pool) DroppedTasks() uint64
func WorkerID(ctx context.Context) (int64, bool)

var (
    ErrQueueFull         = errors.New("workerpool: очередь заполнена")
    ErrResizeNonPositive = errors.New("workerpool: Resize требует n > 0")
)
```

**Backward-compat upgrade:** zero-value `OnFull = OnFullBlock`. Middle-конфиг
скопированный в senior работает идентично.

## Ключевые решения

### 1. Backpressure - поле `Config.OnFull`

Enum-поле в `Config`, читается с полпинка. Zero-value совпадает с
поведением middle - middle-конфиг копируется без изменений.

### 2. `OnFullDropNewest` возвращает `nil` + counter

Семантика fire-and-forget: "попробовал, не вышло - забуду". Вызывающий
получает `nil` и думает что всё ок - но `DroppedTasks()` подскочил. **Это
ловушка backpressure'а:** без мониторинга counter'а деградация невидима.
В production превращается в Prometheus counter.

### 3. `Resize` - синхронный, под `sync.Mutex`, возвращает фактическое число

Mutex закрывает race: два concurrent `Resize` оба читают `current=10`,
оба считают `delta=+5` - стартуют 10 воркеров вместо 5.

`(int, error)` вместо `error` - диагностика: "просил 10, получил 7,
потому что Shutdown пошёл".

### 4. `quitOne` - N send'ов вместо `close`

`close(quitOne)` дал бы broadcast и погасил **всех**. Нам надо ровно N.
Канал unbuffered, `send` блокирует до receiver'а - N send'ов = N exit'ов.
Какой именно из живых воркеров получит сигнал - undefined (Go random
select), и это **фича**: worker affinity мы не поддерживаем.

### 5. `workersRunning` - eager-decrement в shrink

Counter обновляется под `resizeMu` **сразу после** успешного
`quitOne <- struct{}{}`, а не в `defer` воркера. Иначе два concurrent
shrink-Resize'а оба прочитают одинаковый stale, оба пошлют одинаковое
число сигналов - пул усохнет вдвое сильнее. `TestResize_ConcurrentResize`
ловит этот баг.

### 6. `Resize(0)` ≠ `Shutdown`

`n <= 0` → `ErrResizeNonPositive`. Shutdown drain'ит очередь, Resize -
нет. Смешивать = терять задачи без drain'а. Явная ошибка лучше тихого
дедлока из 0 воркеров на не пустой очереди.

В `New(cfg)` `Workers <= 0` - паника (programmer error на старте). В
`Resize(0)` - ошибка (runtime command в долгоживущем процессе).

## Lifecycle

```
                          New(cfg)
                              │
                              ▼
              ┌───────────────────────────────┐
              │   ACTIVE                      │
              │   Submit → tasks              │
              │   Resize → grow / shrink      │
              │   воркеры:                    │
              │     select tasks/done/        │
              │            quitOne/stop       │
              └───────────────┬───────────────┘
                              │ Shutdown(ctx) → close(stop)
                              │ Submit → ErrPoolClosed
                              │ Resize → ErrPoolClosed
                              ▼
              ┌───────────────────────────────┐
              │   DRAINING                    │
              │   воркеры дорезают tasks      │
              └───────┬───────────────┬───────┘
                drained               ctx.Done()
                      ▼               ▼
              ┌─────────────┐   ┌──────────────────────┐
              │   CLOSED    │   │   FORCE-STOP         │
              │   return nil│   │   close(done)        │
              └─────────────┘   │   return wrapped err │
                                └──────────────────────┘
```

## Тесты

| Слой | Тесты |
|---|---|
| middle (без изменений) | `TestJuniorBugIsGone`, `TestPanicRecovery`, `TestGracefulDrain`, `TestShutdownTimeout`, `TestPerTaskCtxCancellation`, `TestSubmitAfterShutdown`, `TestDoubleShutdown`, `TestDroppedErrors`, `TestSubmitCtxAlreadyDone` |
| якорные senior | `TestOnFullBlock_DefaultMatchesMiddle`, `TestOnFullFailFast`, `TestOnFullDropNewest`, `TestResize_GrowAndShrink` |
| покрытие senior | `TestResize_NonPositive`, `TestResize_NoOp`, `TestResize_AfterShutdown`, `TestResize_ConcurrentResize`, `TestResize_RaceWithShutdown` |

Зелёные под `-race` и при `-count=10`.

## Q&A - что не в коде, но надо знать на собесе

Полный разбор - в `../senior-design.md`. Кратко:

1. **Rate limiter** - `golang.org/x/time/rate`, token bucket vs leaky bucket; `Wait(ctx)` в `Submit`/`runTask`. Rate limiter ≠ backpressure.
2. **Observability** - 5 метрик (`queue_length`, `tasks_in_flight`, `task_duration_seconds`, `task_errors_total`, `dropped_tasks_total`), structured `slog`, OTel через ctx.
3. **Когда WP не нужен** - для I/O-bound лучше `errgroup.Group` + `semaphore.Weighted`. WP оправдан, когда **реюз воркера** что-то даёт.
4. **Готовые либы** - `sourcegraph/conc`, `alitto/pond`. В проде - готовое; свой пишут на собес "понимаешь ли механику".
5. **Priority queue** - `container/heap` (~80 LOC) или два канала high/low с nested select (~10 LOC). Сначала вопрос: нужна ли вообще?
6. **Pool of pools** - диспетчер + N маленьких пулов по типу задачи. Изоляция SLO (latency-sensitive vs background batch). Bulkhead из Hystrix.

## Чего здесь нет (явно вне scope)

- Rate limiting в коде (Q&A #1)
- Prometheus / OpenTelemetry / structured logs wiring (Q&A #2)
- Priority queue в коде (Q&A #5)
- Pool of pools / bulkhead (Q&A #6)
- Worker affinity / sticky sessions
- Spillover backpressure (overflow на диск)
- `OnFullDropOldest` (atomicity дороже выгоды)
- Auto-resize по метрикам (без явного `Resize(n)`)
- Hot-reload `Config` (изменение `QueueSize` после старта)

Каждая фича - на отдельный ролик. Закодить "вскользь" = размыть нарратив.
