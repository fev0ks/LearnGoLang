# Timers: time.Sleep, Timer и Ticker

Timer park goroutine до deadline, не резервируя OS thread. Начиная с Go 1.23 channel-based timers получили новые GC и Stop/Reset guarantees, поэтому старые советы про обязательное drain канала часто больше не нужны.

## Содержание

- [Mental model](#mental-model)
- [Где хранятся timers](#где-хранятся-timers)
- [Связь с scheduler и netpoller](#связь-с-scheduler-и-netpoller)
- [Какой API выбирать](#какой-api-выбирать)
- [Что изменилось в Go 1.23](#что-изменилось-в-go-123)
- [Практические паттерны](#практические-паттерны)
- [Точность и Ticker](#точность-и-ticker)
- [Типичные ошибки](#типичные-ошибки)
- [Версионная карта runtime timers](#версионная-карта-runtime-timers)
- [Interview-ready answer](#interview-ready-answer)
- [Официальные источники](#официальные-источники)

## Mental model

```go
time.Sleep(500 * time.Millisecond)
```

Упрощённо runtime:

1. создаёт timer с deadline;
2. переводит current G в waiting;
3. M продолжает исполнять другие goroutines;
4. после deadline runtime делает sleeping G runnable;
5. scheduler позже снова запускает её на любом подходящем M.

`Sleep` гарантирует паузу **не меньше** duration, но не точный момент resume. После deadline goroutine ещё ждёт scheduler и OS CPU time.

## Где хранятся timers

В current runtime timers организованы как per-P sets с heap, ordered по ближайшему deadline. Это уменьшает contention по сравнению с одной global heap.

Операции в общем случае:

- add/modify timer поддерживает heap order;
- scheduler проверяет ближайшие timers;
- expired timer делает G runnable, отправляет value в channel или запускает callback;
- periodic timer вычисляет следующий deadline.

Конкретные fields и helper function names в `runtime/time.go` — implementation details. Стабильная идея: множество timers обслуживается runtime, а не отдельным OS thread на timer.

<details>
<summary>Упрощённые структуры timer и timers в Go 1.26</summary>

```go
// Один logical trigger.
type timer struct {
	mu     mutex
	state  uint8
	isChan bool

	when   int64
	period int64
	f      func(arg any, seq uintptr, delay int64)
	arg    any
	seq    uintptr

	ts *timers
}

// Per-P набор timers.
type timers struct {
	mu   mutex
	heap []timerWhen
	len  atomic.Uint32

	zombies     atomic.Int32
	minWhenHeap atomic.Int64
}
```

Ключевые поля:

- `when` — absolute monotonic deadline;
- `period == 0` — one-shot timer;
- `period > 0` — periodic timer/Ticker;
- `f/arg` — действие: разбудить G, подготовить channel send или запустить callback;
- `seq` — защита от callback старой configuration после Reset/deadline update;
- `heap[0]` — ближайший timer внутри per-P set.

Scheduler способен обращаться к timers другого P, поэтому «per-P» не означает «вообще без synchronization». Heap защищается lock, а atomics дают дешёвый fast check ближайшего deadline и количества entries.

</details>

## Связь с scheduler и netpoller

Когда runnable work есть, scheduler попутно проверяет expired timers. Когда work нет, runtime может block в netpoll с timeout до ближайшего timer.

```text
network event раньше deadline → poller просыпается из-за fd
deadline раньше network event → poller timeout, scheduler проверяет timers
```

Netpoller не исполняет timer callback. Он лишь помогает runtime эффективно спать до network event или ближайшего deadline. Callback/ready goroutine затем проходит через scheduler.

## Какой API выбирать

| API | Когда использовать | Можно управлять lifecycle |
| --- | --- | --- |
| `time.Sleep` | просто приостановить current G | нет |
| `time.After` | одноразовый timeout в простом `select` | channel only |
| `time.NewTimer` | timeout нужно Stop/Reset/reuse | да |
| `time.AfterFunc` | запустить callback после delay | Stop/Reset со специальной семантикой |
| `time.NewTicker` | periodic event | да, через `Stop` |
| `time.Tick` | короткий convenience case | channel only |

## Что изменилось в Go 1.23

Для channel-based timers (`NewTimer`, `After`, `NewTicker`, `Tick`) в Go 1.23:

- GC может собирать unreferenced timers/tickers, даже если code не вызывает `Stop`;
- timer channels работают как synchronous (`cap=0`);
- после return из `Stop` или `Reset` channel не получит stale value от прежней configuration.

Поэтому старый универсальный idiom больше не нужен для Go 1.23+:

```go
// Старый pre-1.23 pattern; в новом коде не копировать механически.
if !t.Stop() {
    <-t.C
}
t.Reset(d)
```

В module с `go` directive до 1.23 может действовать legacy timer behavior; им также управляет `GODEBUG=asynctimerchan`.

`Stop` всё ещё полезен, когда timer больше не нужен: это явный lifecycle и предотвращение ненужного wakeup/callback. Для Ticker `Stop` прекращает будущие ticks.

<details>
<summary>Current и legacy Reset рядом</summary>

Для module с Go 1.23+ channel timer можно reset без ручного drain:

```go
t.Reset(nextDelay)
```

Старый pattern для legacy semantics выглядел так:

```go
if !t.Stop() {
	select {
	case <-t.C:
	default:
	}
}
t.Reset(nextDelay)
```

Не смешивайте patterns механически. Поведение выбирается `go` directive module и может быть изменено `GODEBUG=asynctimerchan`; при upgrade полезно иметь test на Stop/Reset behavior, от которого зависит код.

</details>

## Практические паттерны

### Timeout с context

Если операция принимает context, обычно проще позволить higher-level API управлять timer:

```go
ctx, cancel := context.WithTimeout(parent, 2*time.Second)
defer cancel()

return call(ctx)
```

### Переиспользуемый Timer в hot loop

`time.After` на каждой итерации создаёт новый timer. В hot path можно reuse один:

```go
t := time.NewTimer(time.Second)
defer t.Stop()

for {
    select {
    case v := <-in:
        handle(v)
        t.Reset(time.Second) // Go 1.23+ semantics
    case <-t.C:
        onIdleTimeout()
        t.Reset(time.Second)
    }
}
```

Это пример для одного owner goroutine. Concurrent Stop/Reset/read одного timer требует отдельного synchronization design.

<details>
<summary>Debounce на одном reusable Timer</summary>

```go
func debounce[T any](
	ctx context.Context,
	in <-chan T,
	delay time.Duration,
	flush func(T),
) {
	timer := time.NewTimer(time.Hour)
	_ = timer.Stop() // пример рассчитан на Go 1.23+ semantics
	defer timer.Stop()

	var (
		latest T
		armed  bool
		timerC <-chan time.Time
	)

	for {
		select {
		case value, ok := <-in:
			if !ok {
				return
			}
			latest = value
			armed = true
			timer.Reset(delay) // Go 1.23+ channel timer semantics
			timerC = timer.C
		case <-timerC:
			if armed {
				flush(latest)
			}
			armed = false
			timerC = nil
		case <-ctx.Done():
			return
		}
	}
}
```

Одна goroutine владеет Timer, поэтому нет concurrent Reset/read. `nil` channel выключает timer branch после flush. В production нужно отдельно решить, следует ли flush последнее значение при закрытии input или cancellation.

</details>

### Ticker lifecycle

```go
ticker := time.NewTicker(time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        refresh()
    case <-ctx.Done():
        return
    }
}
```

## Точность и Ticker

Timers — scheduling mechanism, а не hard real-time clock:

- OS scheduling и load задерживают execution after deadline;
- long GC/runtime work может добавить latency;
- Ticker корректирует interval и может drop ticks, если receiver не успевает;
- wall clock может меняться, поэтому Go time values используют monotonic component для duration comparisons, когда он доступен.

Если нужно обработать «сколько периодов прошло», нельзя полагаться на количество полученных ticker events — вычисляйте состояние из current time/domain data.

<details>
<summary>Эксперимент: Ticker не является очередью всех ticks</summary>

```go
ticker := time.NewTicker(100 * time.Millisecond)
defer ticker.Stop()

previous := time.Now()
for range 5 {
	tickTime := <-ticker.C
	fmt.Println("tick delta:", tickTime.Sub(previous).Round(10*time.Millisecond))
	previous = tickTime

	// Receiver медленнее периода ticker.
	time.Sleep(350 * time.Millisecond)
}
```

Программа не получает по три события после каждого `Sleep`: Ticker корректирует schedule и может drop ticks для медленного receiver. Поэтому periodic reconciliation обычно вычисляет состояние заново, а не делает «один шаг на каждый tick».

</details>

`time.AfterFunc` запускает callback в отдельной goroutine. Callback может пересекаться с application work и требует обычной synchronization. Семантика `Reset` отличается в зависимости от того, active timer или callback уже запущен; её проверяют по package docs.

<details>
<summary>AfterFunc: Stop не ждёт уже начавшийся callback</summary>

```go
done := make(chan struct{})
timer := time.AfterFunc(100*time.Millisecond, func() {
	defer close(done)
	expensiveCleanup()
})

if !timer.Stop() {
	// В этом примере callback уже мог начаться; ждём его отдельно.
	<-done
}
```

Такое ожидание корректно здесь, потому что timer новый, `Stop` вызывается один раз, а callback гарантированно закрывает `done`. В общем случае повторные `Reset`/`Stop` требуют отдельной state machine: Timer API не заменяет `WaitGroup`, mutex или ownership protocol.

</details>

## Типичные ошибки

- считать `time.Sleep` blocking syscall для M;
- ожидать resume точно в deadline;
- создавать `time.After` в hot loop без оценки allocations;
- переносить pre-1.23 drain pattern в Go 1.23+;
- забывать `Ticker.Stop`, когда periodic work должен прекратиться;
- делать тяжёлый `AfterFunc` callback без контроля concurrency;
- считать каждый ticker event гарантированным;
- одновременно управлять одним Timer из нескольких goroutines без clear ownership.

## Версионная карта runtime timers

Этот раздел помогает читать старые статьи, не перенося их устройство на актуальный runtime:

| Версия | Модель |
| --- | --- |
| Ранние версии до Go 1.10 | Global timer heap и отдельная `timerproc` goroutine |
| Go 1.10–1.13 | Timer heaps sharded, но architecture ещё отличается от современной per-P integration |
| Go 1.14+ | Timers интегрированы с P/scheduler и timeout ожидания netpoll |
| Go 1.23+ | Channel timers получают synchronous-channel и новые GC/Stop/Reset guarantees |

Поэтому утверждение «в Go есть одна специальная goroutine, которая обслуживает все timers» описывает старый runtime, а не Go 1.26. В актуальной модели scheduler проверяет per-P timers, а blocking netpoll timeout помогает эффективно ждать ближайший deadline.

Версионная карта не обещает одинаковый internal layout во всех releases после Go 1.14. Проверять нужно стабильную API semantics package `time`, а runtime fields использовать только для понимания и диагностики.

## Interview-ready answer

**1. Блокирует ли time.Sleep OS thread?**

- Нет. Runtime регистрирует timer и park G. M с P запускает другую goroutine. После deadline G становится runnable и позже продолжает работу.

**2. Где живут timers?**

- В current runtime это per-P timer sets/heaps. Scheduler проверяет expired timers, а при idle может использовать netpoll timeout до ближайшего deadline. Отдельный thread на каждый timer не нужен.

**3. Зачем netpoller участвует в timers?**

- Blocking poll уже умеет ждать с timeout. Runtime выбирает timeout по ближайшему timer, поэтому один wait просыпается либо по network readiness, либо по времени. Timer work после wakeup делает scheduler.

**4. Что изменилось в Go 1.23?**

- Channel timers работают как synchronous, Stop/Reset гарантируют отсутствие stale values, а GC может собирать unreferenced unstopped timers/tickers. Старый обязательный drain channel больше не является правильным default для нового кода.

**5. Есть ли отдельная timerproc goroutine?**

- Это описание старых версий. В Go 1.26 timers организованы как per-P sets и интегрированы с scheduler/netpoll wait; отдельного OS thread или goroutine на каждый timer нет.

## Официальные источники

- [time package](https://pkg.go.dev/time)
- [runtime timers source](https://go.dev/src/runtime/time.go)
- [Go 1.23 timer channel changes](https://go.dev/wiki/Go123Timer)
- [Go 1.23 release notes](https://go.dev/doc/go1.23)
