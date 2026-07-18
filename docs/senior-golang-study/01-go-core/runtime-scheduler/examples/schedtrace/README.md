# schedtrace demo

Небольшая программа для наблюдения за scheduler через `GODEBUG=schedtrace`. Основная теория — в [Scheduler и preemption](../../01-scheduler-and-preemption.md).

## Что моделирует программа

- `-cpu` — CPU-bound goroutines, которые конкурируют за P;
- `-io` — timer-waiting goroutines: название flag историческое, реального network I/O здесь нет;
- `-spin` — tight loop для наблюдения async preemption;
- `-dur` — продолжительность эксперимента.
- `-trace` — записать execution trace в указанный файл.

## Запуск

Лучше сначала собрать binary: так `GODEBUG` применяется только к target process, а output не смешивается с parent process `go run`.

```bash
cd docs/senior-golang-study/01-go-core/runtime-scheduler/examples/schedtrace
go build -o ./sched .

GODEBUG=schedtrace=1000 ./sched
GOMAXPROCS=1 GODEBUG=schedtrace=1000 ./sched
GODEBUG=schedtrace=1000,scheddetail=1 ./sched
GODEBUG=schedtrace=1000 ./sched -cpu=16 -io=5000 -dur=15s
GOMAXPROCS=1 GODEBUG=schedtrace=1000 ./sched -spin
./sched -cpu=16 -io=5000 -dur=5s -trace=trace.out
go tool trace trace.out
```

При `go run` можно увидеть schedtrace и от Go tool process, и от target program. Это не два scheduler внутри приложения — это разные processes, унаследовавшие `GODEBUG`.

## Как читать SCHED

Типичная summary line:

```text
SCHED 1007ms: gomaxprocs=2 idleprocs=0 threads=4 \
spinningthreads=0 needspinning=1 idlethreads=1 runqueue=132 [61 74]
```

| Поле | Что показывает |
| --- | --- |
| `gomaxprocs` | число P |
| `idleprocs` | P без runnable work в момент snapshot |
| `threads` | созданные OS threads |
| `spinningthreads` | M, которые активно ищут work |
| `idlethreads` | parked M |
| `runqueue` | global runnable queue |
| `[61 74]` | local queue lengths для P |

Это snapshot implementation state. Формат и дополнительные поля могут меняться между Go versions; не стройте alert по парсингу stderr без version control.

## Эксперименты

1. **Concurrency без parallelism:** `GOMAXPROCS=1`; goroutines много, но Go-код исполняет один P.
2. **Queue pressure:** увеличьте `-cpu` и смотрите, как меняются global/local queues.
3. **Waiting goroutines:** `-cpu=0 -io=5000`; goroutine count большой, а CPU queues почти пусты.
4. **Async preemption:** `GOMAXPROCS=1 ... -spin`; timer-based status output продолжает получать CPU.
5. **More detail:** включите `scheddetail=1`, но используйте короткий run — output очень большой.

Work stealing происходит быстро, поэтому один snapshot редко показывает причинно-следственную связь. Для timeline используйте `runtime/trace` и `go tool trace`.

<details>
<summary>Какие snapshots ожидать</summary>

Значения ниже иллюстративные: точный output зависит от Go version, машины и момента snapshot.

```text
# CPU pressure: все P заняты, в queues остаётся runnable work
SCHED ... gomaxprocs=2 idleprocs=0 ... runqueue=14 [31 28]

# Mostly waiting: P простаивают, runnable queues пусты
SCHED ... gomaxprocs=2 idleprocs=2 ... runqueue=0 [0 0]
```

Во втором случае `-io=5000` всё ещё создаёт тысячи goroutines, но они ждут timers. Summary schedtrace не показывает их полное число — его печатает сама программа через `runtime.NumGoroutine`, а подробные states можно увидеть с `scheddetail=1` или goroutine dump.

</details>

## Полезные команды

```bash
GODEBUG=schedtrace=1000,gctrace=1 ./sched
kill -QUIT <pid> # goroutine dump и завершение process на Unix
```

Для GC есть отдельный пример: [memory-internals/examples/gctrace](../../../memory-internals/examples/gctrace/).
