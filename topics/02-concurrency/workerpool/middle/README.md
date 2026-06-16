# Middle: production-ready Worker Pool (~150 LOC)

Уровень Lamoda/WB. Архитектурно закрыты все 5 косяков junior'а - не
заплатками, а сменой модели синхронизации.

## Запуск

```bash
go run ./cmd/workerpool/
go test -v -count=1 ./...
go test -race -count=1 ./...
go test -count=10 ./...           # flaky check
```

## Что добавлено к junior

| Junior | Middle |
|---|---|
| Mutex + флаг `closed` рядом с каналом | `stop`/`done` каналы - единая модель синхронизации |
| Магическое 100 в буфере | `Config{Workers, QueueSize, ErrBuf}` |
| Нет panic recovery | `defer recover` в `runTask`, `*PanicError` в `Errors()` |
| TOCTOU `send on closed channel` | Канал `tasks` **никогда** не закрывается |
| Нет ctx, нет shutdown timeout | `Submit(ctx, fn(ctx))` + `Shutdown(ctx)` с force-stop |
| `AddTask(f func())` | `Submit(ctx, fn) error` (3 класса ошибок) |

`TestJuniorBugIsGone` - прямая копия сценария, который ронял junior.
На middle он зелёный.

## Public API

```go
type Config struct {
    Workers   int  // > 0; New паникует на нарушении
    QueueSize int  // 0 = unbuffered
    ErrBuf    int  // 0 = все ошибки идут в DroppedErrors
}

func New(cfg Config) *Pool
func (p *Pool) Submit(ctx context.Context, fn func(ctx context.Context) error) error
func (p *Pool) Shutdown(ctx context.Context) error
func (p *Pool) Errors() <-chan error
func (p *Pool) DroppedErrors() uint64

type PanicError struct { Recovered any; Stack []byte }

var (
    ErrPoolClosed      = errors.New("workerpool: пул останавливается")
    ErrAlreadyShutdown = errors.New("workerpool: shutdown уже был вызван")
    ErrNilTask         = errors.New("workerpool: пустая функция задачи")
)
```

`Submit` возвращает `nil`, `ctx.Err()`, `ErrPoolClosed` или `ErrNilTask`.
`Shutdown` оборачивает `ctx.Err()` через `%w` - клиент пишет
`errors.Is(err, context.DeadlineExceeded)`. Паники приходят в `Errors()`
как `*PanicError`, извлекаются через `errors.As`.

## Ключевые решения

### 1. Канал `tasks` не закрывается никогда

Главный архитектурный ход. Закрывать канал - единственный способ получить
`send on closed`, поэтому мы его не закрываем. Воркеры выходят по сигналам:
`stop` (graceful) и `done` (force-stop).

### 2. Два сигнальных канала вместо одного

- `stop` (закрывается в начале Shutdown) - "новые `Submit` отбиваем".
- `done` (закрывается, если `Shutdown(ctx)` не уложился в дедлайн) - "бросаем drain".

Одним каналом две фазы shutdown'а не разделить: graceful drain и
принудительная остановка - это разные сигналы.

### 3. Почему воркеры не выходят сразу по `<-stop`

После `close(stop)` воркер не может просто `return`. Иначе сценарий: в
буфере лежит N задач, `Shutdown` закрывает `stop`, все воркеры
одновременно его видят и выходят. `wg.Wait()` мгновенно разблокируется,
`Shutdown` возвращает `nil` - "всё ок". А N задач так и остались в
буфере, никто их не выполнит.

Поэтому воркер уходит в `drainAndExit` и дочёрпывает буфер вместе с
остальными воркерами:

```go
func (p *Pool) drainAndExit() {
    for {
        select {
        case <-p.done:        return            // force-stop по таймауту
        case t := <-p.tasks:  p.runTask(t)      // ещё есть задача - выполнить
        default:              return            // буфер пуст - выходим
        }
    }
}
```

Каждый воркер берёт по одной задаче из буфера, пока буфер не опустеет
(`default`-кейс) или не сработает force-stop по таймауту. Контракт:
**никто не уходит, пока в очереди что-то есть**.

### 4. Per-task ctx, не pool-level

`Submit(ctx, fn(ctx))` - отмена одной задачи не трогает остальные. Pool-level
ctx эмулируется передачей одного и того же `ctx` во все `Submit`.

### 5. Panic - типизированная ошибка через общий канал

Один `Errors()` для всего: и обычные ошибки, и паники. Потребитель
разделяет стандартным паттерном:

```go
for err := range pool.Errors() {
    var pe *PanicError
    if errors.As(err, &pe) {
        // alert + pe.Stack
    } else {
        // обычная ошибка задачи
    }
}
```

### 6. `Submit` fast-path - детерминизм против random select

Перед основным `select` стоит non-blocking проверка `stop` и `ctx`:

```go
select {
case <-p.stop:   return ErrPoolClosed
case <-ctx.Done(): return ctx.Err()
default:
}
```

Без него Go при равновозможных case'ах выбирает рандомно - `Submit` после
вернувшегося `Shutdown` мог бы протолкнуть задачу в буфер, где её уже
никто не прочитает.

> ⚠️ **Известное ограничение.** Fast-path не убирает гонку до конца.
> Если `Submit` уже вошёл в основной `select`, и в этот момент
> срабатывает `close(stop)` - все три case'а ready, выбор рандомный.
> Если runtime выберет `tasks <- t`, задача попадёт в буфер уже после
> начала Shutdown'а. `drainAndExit` её подхватит и выполнит **в обычном
> случае**. Потеряется она только если `Shutdown(ctx)` истечёт по
> таймауту до того, как воркеры до неё доедут - тогда сработает
> force-stop и хвост буфера дропается молча.
>
> Это принятое ограничение middle. Закрыть его насмерть можно только
> линеаризацией `Submit` против `Shutdown` (refcount in-flight Submit'ов,
> Shutdown ждёт их) - это добавляет sync на горячий путь. Альтернатива -
> щедрый дедлайн на `Shutdown`.

### 7. `AddTask` → `Submit`

`AddTask` - junior-naming, как для slice. Java `ExecutorService`, Python
`concurrent.futures`, Go `pond`/`ants` - везде `Submit`. Контракт:
"сдал задачу executor'у, может отказать с ошибкой".

## Тесты

**Якорные** (для нарратива):

| Тест | Что проверяет |
|---|---|
| `TestJuniorBugIsGone` | Сценарий, ронявший junior, - паники нет |
| `TestPanicRecovery` | Паника в задаче ловится, остальные живут, в `Errors()` приходит `*PanicError` |
| `TestGracefulDrain` | 100 задач + `Shutdown(Background)` - все выполняются |
| `TestShutdownTimeout` | Дедлайн истёк → `errors.Is(err, DeadlineExceeded)`, горутины не текут |
| `TestPerTaskCtxCancellation` | Задача уважает `ctx.Done()`, отмена не трогает пул |

**Покрытие:**

| Тест | Что проверяет |
|---|---|
| `TestSubmitAfterShutdown` | `Submit` после `Shutdown` → `ErrPoolClosed` |
| `TestDoubleShutdown` | Повторный `Shutdown` → `ErrAlreadyShutdown` |
| `TestDroppedErrors` | Переполнение `Errors()` → `DroppedErrors()` инкрементится |
| `TestSubmitCtxAlreadyDone` | `Submit` с уже отменённым ctx → `ctx.Err()`, не висит |

Все тесты должны быть зелёными под `-race` и при `-count=10`.

## Чего здесь нет (senior-блок)

- Backpressure-стратегии (`OnFullBlock` / `FailFast` / `DropNewest`)
- Dynamic resize воркеров
