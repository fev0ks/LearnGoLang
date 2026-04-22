# Go Profiling

Детальный разбор инструментов профилирования Go-приложений: от механики pprof до разбора реальных кейсов.

## Материалы

- [01. pprof: инструменты и workflow](./01-pprof-tools-and-workflow.md) — типы профилей, подключение, go tool pprof команды, flat vs cum, pprof vs trace
- [02. CPU Profiling](./02-cpu-profiling.md) — семплер 100 Hz, flamegraph, типичные hotspots (regexp, fmt, json), baseline и benchstat
- [03. Memory Profiling](./03-memory-profiling.md) — inuse_space vs alloc_space, `-diff_base`, GC pressure, GOGC/GOMEMLIMIT
- [04. Goroutine & Concurrency](./04-goroutine-concurrency-profiling.md) — goroutine dump, состояния горутин, block/mutex профили, поиск утечек, goleak
- [05. Execution Tracer](./05-execution-tracer.md) — runtime/trace vs pprof, go tool trace, GC паузы, STW, scheduling gaps, user annotations
- [06. Benchmarks](./06-benchmarks.md) — testing.B, dead code elimination, -benchmem, b.RunParallel, benchstat, PGO
- [07. Case Studies](./07-case-studies.md) — 5 сценариев: CPU/regexp, memory leak, goroutine leak, lock contention, GC pressure

## Порядок чтения

1. `01` — основы, без этого остальное не читать
2. `02` + `03` — самые частые проблемы на практике
3. `04` — goroutine leak (очень частая тема на интервью)
4. `05` — для понимания latency spikes и GC
5. `06` — если пишешь оптимизации и нужно их измерять
6. `07` — закрепление через реальные сценарии, хорошо перед интервью

## Вопросы senior-уровня

- Чем pprof отличается от runtime/trace? Когда каждый из них нужен?
- Что такое flat vs cum в pprof? Какой из них смотреть при высоком CPU?
- Как бы ты нашёл goroutine leak на production сервисе?
- Как найти источник GC pressure? Какой профиль для этого?
- Почему CPU профиль может не показывать проблему при latency spikes?
- Что такое block profile и зачем он нужен, если уже есть mutex profile?
- Как сравнить производительность до и после оптимизации надёжно?
- Что такое dead code elimination в бенчмарках и как от неё защититься?
- Как sync.Pool снижает GC pressure?
- Что показывает пустой P в execution tracer?

## Инструменты

```bash
# Открыть flamegraph
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/profile?seconds=30"

# Все типы профилей
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/heap"
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/goroutine"
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/block"
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/mutex"

# Execution tracer
curl -o trace.out "http://localhost:6060/debug/pprof/trace?seconds=5"
go tool trace trace.out

# Benchmarks
go test -bench=. -benchmem -count=10 ./... > before.txt
benchstat before.txt after.txt

# GC трейс
GODEBUG=gctrace=1 ./myapp
```

## Перекрёстные ссылки

- [Memory Internals: Allocator](../../01-go-core/memory-internals/02-allocator.md) — mcache/mcentral/mheap, size classes
- [Memory Internals: GC](../../01-go-core/memory-internals/04-garbage-collector.md) — tri-color, write barrier, GOGC
- [Scheduler](../../01-go-core/07-scheduler-and-preemption.md) — GMP модель, P handoff, work stealing
- [Syscall](../../01-go-core/08-syscall.md) — почему file I/O блокирует M
- [Netpoller](../../01-go-core/09-netpoller.md) — почему network I/O не блокирует M
- [Incident Investigation](../../11-devops-and-observability/incident-investigation-and-profiling/01-how-to-investigate-production-issues.md) — как профилирование вписывается в общую диагностику
