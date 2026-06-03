# schedtrace demo

Синтетическая программа, чтобы «вживую» посмотреть на планировщик Go через `GODEBUG=schedtrace`. Связана с [../../01-scheduler-and-preemption.md](../../01-scheduler-and-preemption.md).

## Что внутри

`main.go` поднимает три вида нагрузки (управляются флагами):

- **CPU-bound** горутины (`-cpu`) — бесконечно считают порциями, заполняют локальные очереди P → видно work stealing и preemption;
- **I/O-bound** горутины (`-io`) — спят на тикере (`Gwaiting`), почти не грузят CPU, но «висят» как горутины;
- **spin** (`-spin`) — tight loop без точек уступки, демо async preemption (Go 1.14+ вытеснит через `SIGURG`).

## Как запустить

```bash
cd docs/senior-golang-study/01-go-core/runtime-scheduler/examples/schedtrace

# Базово: печать состояния планировщика раз в 1000 мс
GODEBUG=schedtrace=1000 go run .

# Один P — нагляднее видно чередование горутин (concurrency без parallelism)
GOMAXPROCS=1 GODEBUG=schedtrace=1000 go run .

# Подробно по каждому P/M/G
GODEBUG=schedtrace=1000,scheddetail=1 go run .

# Поиграть с нагрузкой
go run . -cpu=16 -io=5000 -dur=15s
GODEBUG=schedtrace=1000 go run . -spin
```

> Эта программа специально почти **не аллоцирует** — она про планировщик. Чтобы посмотреть на **GC** (`gctrace=1`), есть отдельный пример: [memory-internals/examples/gctrace](../../../memory-internals/examples/gctrace/).

> **Видишь ДВЕ строки `SCHED` на интервал? Это нормально — и важно понять почему.** При `go run` живут **два процесса**, и оба наследуют `GODEBUG`:
> 1. **родитель `go run`** — он не просто компилирует, а **остаётся жить весь прогон** (ждёт дочерний процесс). Почти простаивает → его строки выглядят как `idleprocs=4 ... runqueue=0 [0 0 0 0]`;
> 2. **твоя программа** (дочерний процесс) — реальная нагрузка: `idleprocs=0 ... runqueue=519 [200 113 130 1]`.
>
> У каждого свой таймер schedtrace → время чуть разное (`1008ms` vs `1003ms`), а изредка строки **перемешиваются** в stdout (два процесса пишут разом). Чтобы видеть **только свою программу** — собери бинарник и запусти его напрямую:
> ```bash
>   go build -o ./sched .
> или с выводом алокатора
>   go build -gcflags=-m -o sched . 
> потом 
>   GOMAXPROCS=2 GODEBUG=schedtrace=1000,gctrace=1 ./sched 
> ```
> Тогда строка `SCHED` будет одна на интервал.

## Как читать строку SCHED

```
SCHED 1007ms: gomaxprocs=2 idleprocs=0 threads=4 spinningthreads=0 \
              needspinning=1 idlethreads=1 runqueue=132 [ 61 74 ] schedticks=[ 3356 3112 ]
```

| Поле | Что значит |
|---|---|
| `1007ms` | время с запуска (интервал ≈ заданному в schedtrace) |
| `gomaxprocs=2` | число P |
| `idleprocs=0` | простаивающих P (под нагрузкой 0 — все заняты) |
| `threads=4` | всего OS-потоков (включая sysmon, netpoller) |
| `spinningthreads` | число **M (с привязанным P)**, которые остались без работы и активно ищут её (крадут из чужих LRQ, смотрят GRQ/netpoll) прежде чем припарковаться. Это состояние M, но work stealing перекладывает горутины между очередями **P** |
| `idlethreads` | припаркованные M (резерв в idle-пуле) |
| `runqueue=132` | горутин в **глобальной** очереди (GRQ) |
| `[ 61 74 ]` | длины **LRQ** каждого P — дисбаланс → сейчас будет work stealing |
| `schedticks` | счётчики решений планировщика по P (тот самый `schedtick`, кратность 61) |

## Что попробовать (мини-эксперименты)

1. **Work stealing и GRQ.** Запусти `go run . -cpu=8 -io=2000` и смотри на `runqueue` и `[..]`: в момент старта тысячи горутин не влезают в LRQ (по 256) → излишек уходит в GRQ (`runqueue` подскакивает), потом разгребается.
2. **Concurrency без parallelism.** `GOMAXPROCS=1 GODEBUG=schedtrace=1000 go run .` — `gomaxprocs=1`, одна LRQ, всё чередуется на одном P. Сравни `[app] NumGoroutine` (живо много) с тем, что исполняется одна.
3. **I/O-bound почти не грузит P.** Подними `-io=5000 -cpu=0` — горутин тысячи (`NumGoroutine` большой), но LRQ почти пустые: они спят в `Gwaiting`, а не на CPU.
4. **Async preemption.** `GOMAXPROCS=1 GODEBUG=schedtrace=1000 go run . -spin` — spin-горутина крутит tight loop, но остальные (тикеры, печать `NumGoroutine`) **продолжают работать**: sysmon вытесняет spin через `SIGURG`. До Go 1.14 на 1 P это бы всё заморозило.
5. **Число потоков.** Добавь много блокирующих файловых операций (или `-io` с реальным I/O) и смотри, как растёт `threads` (Go поднимает новые M под застрявшие syscall).

## Полезные соседние GODEBUG

```bash
GODEBUG=schedtrace=1000              # состояние планировщика
GODEBUG=schedtrace=1000,scheddetail=1 # + детально по каждому P/M/G
GODEBUG=gctrace=1                    # циклы GC (см. memory-internals/04-garbage-collector.md)
GODEBUG=schedtrace=1000,gctrace=1    # вместе
```

Дамп всех горутин со стеками (включая системные `forcegc`/`bgsweep`/`bgscavenge`):

```bash
# во время работы программы в другом терминале:
kill -QUIT <pid>      # SIGQUIT печатает стеки всех горутин и падает
```
