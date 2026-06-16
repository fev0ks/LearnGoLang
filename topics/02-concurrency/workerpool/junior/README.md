# Junior: базовый Worker Pool (~40 LOC)

Уровень галеры/аутсорса. На демо работает, в проде падает с
`panic: send on closed channel`.

## Запуск

```bash
go run ./cmd/workerpool/
go test -v -count=1 ./...        # включая тест с воспроизведением бага
go test -race -count=1 ./...     # race detector ловит гонку на канале
```

## Public API

```go
type WorkerPool struct { /* tasks chan, wg, mu, closed bool */ }

func New(workers int) *WorkerPool
func (p *WorkerPool) AddTask(f func())
func (p *WorkerPool) Close()
```

Никакого `context`, никаких ошибок, никакой конфигурации.

## Косяки (по нарастанию серьёзности)

### 1. Mutex + канал на одной сущности

```go
type WorkerPool struct {
    tasks  chan func()
    mu     sync.Mutex
    closed bool
}
```

Канал - уже потокобезопасный примитив. Мьютекс с флагом `closed` рядом -
вторая система синхронизации поверх первой. На собесе спросят: "зачем оба
сразу?" - внятного ответа у джуна нет.

### 2. Магическое число 100

```go
tasks: make(chan func(), 100)
```

Размер буфера хардкодом. Не настраивается, не вычисляется. На любом ревью
попросят вынести в параметр.

### 3. Нет panic recovery в воркере

```go
func (p *WorkerPool) worker() {
    defer p.wg.Done()
    for task := range p.tasks {
        task() // паника пробивает range и убивает воркер
    }
}
```

Одна паника = минус воркер. Через несколько падений пул деградирует до
нуля, `AddTask` встаёт на полном буфере.

### 4. TOCTOU между AddTask и Close - главный баг

`AddTask` проверяет `p.closed` под мьютексом, отпускает мьютекс и идёт
делать `p.tasks <- f`. В этом окне `Close` успевает вызвать `close(p.tasks)`.
Заблокированный `send` падает с `panic: send on closed channel`.

```
AddTask                       Close
-------                       -----
mu.Lock(); closed==false; mu.Unlock()
                              mu.Lock(); closed=true; close(p.tasks); mu.Unlock()
p.tasks <- f                  💥 panic: send on closed channel
```

> **TOCTOU** - *Time-Of-Check to Time-Of-Use*. Между проверкой условия и
> действием по результату состояние успевает измениться. Мьютекс защищает
> запись флага, но не сам `send`.

Тест `TestAddTask_PanicOnBlockedSend` загоняет гонку в детерминированный
сценарий: занимаем воркера, забиваем буфер, ставим `AddTask` в `send`,
вызываем `Close` - паника гарантирована.

### 5. Нет `context.Context`, нет грациозного шатдауна

```go
func (p *WorkerPool) Close() {
    ...
    p.wg.Wait() // ждёт ВСЕ задачи в буфере, без таймаута
}
```

Зависшая задача в очереди → `Close` висит вместе с ней. Сказать "дай 5
секунд на drain, потом убей" нельзя.

## Тесты

| Тест | Что показывает |
|---|---|
| `TestAddTask_PanicOnBlockedSend` | Детерминированно роняет пул паникой `send on closed channel` (баг #4) |
| `TestAddTask_RaceUnderRaceDetector` | Под `-race` - гонка `close` vs `send` на канале |
