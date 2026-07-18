# Go Core

Язык и runtime Go. Читать по порядку — каждый файл строится на предыдущем.

## Материалы

- [01. Primitive Types, Sizes And Overflow](./01-primitive-types-and-zero-values.md) — встроенные типы, zero values, поведение nil slice/map/chan, размеры, диапазоны числовых типов, overflow, конверсии
- [02. Value vs Pointer Semantics](./02-value-vs-pointer-semantics.md) — когда копировать, когда брать указатель; mutex copy bug; slice aliasing
- [03. Interfaces, Method Sets And Nil](./03-interfaces-method-sets-and-nil.md) — iface/eface layout, itab vtable, typed nil trap, method sets
- [04. Slices](./04-slices.md) — slice header (ptr/len/cap), shared backing array, append реаллокация, sub-slice, copy ловушки, nil vs empty, memory retention
- [05. Error Handling](./05-error-handling.md) — errors.Is/As, wrapping chain, sentinel vs typed errors, errgroup, errCh паттерн, errors.Join
- [06. Generics](./06-generics.md) — type parameters, constraints, any/comparable, ~underlying type, слайс-утилиты, stdlib slices/maps/cmp, подводные камни, производительность
- [07. Strings](./07-strings.md) — string header (ptr/len), immutability, byte vs rune, UTF-8, len=байты, range по рунам, конверсии и аллокации, substring retention, strings.Builder, string(int) ловушка, unsafe-конверсии
- [08. Unsafe And Low-Level](./08-unsafe-and-low-level.md) — unsafe.Pointer vs uintptr, Sizeof/Alignof/Offsetof, padding/alignment, zero-copy string↔[]byte, layout, где используется в рантайме
- [Runtime Scheduler](./runtime-scheduler/) — GMP scheduler, syscall handoff, netpoller (подраздел)
- [Map Internals](./map-internals/) — актуальные Swiss Tables, поведение map, `sync.Map` и короткая история hmap (подраздел)
- [Memory Internals](./memory-internals/) — стек и heap, аллокатор, escape analysis, GC (подраздел)
- [Concurrency & Performance](./concurrency-and-performance/) — goroutines/channels, sync-примитивы, worker pool, context (подраздел)
- [Profiling](./profiling/) — pprof, CPU/memory/goroutine/block/mutex профили, execution tracer, benchmarks (подраздел)

## Runtime Scheduler (подраздел)

Как горутины исполняются на OS-потоках и почему I/O не блокирует scheduler. Файлы тесно связаны, читать по порядку:

- [01. Scheduler And Preemption](./runtime-scheduler/01-scheduler-and-preemption.md) — G/M/P mental model, run queues, work stealing, async preemption и container-aware `GOMAXPROCS` в Go 1.25+
- [02. Syscall](./runtime-scheduler/02-syscall.md) — blocking syscall flow, P handoff, реальная cancellation, cgo, `LockOSThread` и thread exhaustion
- [03. Netpoller](./runtime-scheduler/03-netpoller.md) — readiness/completion model, parking/wakeup G, framing, deadlines, backpressure и DNS resolver
- [04. Timers](./runtime-scheduler/04-timers.md) — `time.Sleep`/Timer/Ticker, per-P timers, связь с netpoll и timer semantics Go 1.23+

## Map Internals (подраздел)

Сначала актуальная реализация и наблюдаемое поведение; старая hmap оставлена как исторический контекст:

- [02. Swiss Tables](./map-internals/02-swiss-tables-since-1.24.md) — с Go 1.24: groups, control bytes, H1/H2, probing, tombstones и directory
- [03. Задачки и подводные камни](./map-internals/03-puzzles-and-gotchas.md) — nil map, addressability, range, comparable keys, NaN и concurrency
- [04. sync.Map](./map-internals/04-sync-map.md) — выбор структуры, API и concurrent hash-trie с Go 1.24
- [01. Историческая hmap + bmap](./map-internals/01-hmap-before-1.24.md) — Go 1.23 и ниже: buckets, tophash, overflow chains и incremental evacuation

## Memory Internals (подраздел)

Материалы про управление памятью вынесены в отдельный подраздел, потому что они связаны между собой:

- [01. Stack And Heap](./memory-internals/01-stack-and-heap.md) — goroutine stack, heap arenas, scavenger, RSS vs VSZ
- [02. Allocator](./memory-internals/02-allocator.md) — size classes, mcache/mcentral/mheap, tiny allocator, noscan
- [03. Escape Analysis](./memory-internals/03-escape-analysis.md) — stack vs heap решение компилятора, `-gcflags=-m`
- [04. Garbage Collector](./memory-internals/04-garbage-collector.md) — tri-color, write barrier, GOGC, GOMEMLIMIT, gctrace

## Concurrency & Performance (подраздел)

Конкурентность — фундамент Go, поэтому держим рядом с остальными основами:

- [01. Memory Model](./concurrency-and-performance/01-memory-model.md) — happens-before, channel/mutex/Once/atomic гарантии, data race, race detector
- [02. Goroutines And Channels](./concurrency-and-performance/02-goroutines-and-channels.md) — lifecycle, buffered/unbuffered, pipeline, fan-out/fan-in, goroutine leak, select
- [03. Sync Primitives](./concurrency-and-performance/03-sync-primitives.md) — Mutex/RWMutex, WaitGroup, Once, Cond, Pool, Map, atomic, singleflight
- [04. Context Patterns](./concurrency-and-performance/04-context-patterns.md) — WithCancel/Timeout/Deadline, propagation, context.Value анти-паттерны
- Worker pool — в практике: [coding-tasks/concurrency/07-worker-pool-debug](../12-interview-practice/coding-tasks/concurrency/07-worker-pool-debug.md) — баги типовой реализации, errCh, graceful shutdown, semaphore

## Profiling (подраздел)

Профилирование общее для всего Go (не только concurrency), поэтому вынесено отдельным подразделом:

- [01. pprof: инструменты и workflow](./profiling/01-pprof-tools-and-workflow.md)
- [02. CPU Profiling](./profiling/02-cpu-profiling.md)
- [03. Memory Profiling](./profiling/03-memory-profiling.md)
- [04. Goroutine & Concurrency Profiling](./profiling/04-goroutine-concurrency-profiling.md)
- [05. Execution Tracer](./profiling/05-execution-tracer.md)
- [06. Benchmarks](./profiling/06-benchmarks.md)
- [07. Case Studies](./profiling/07-case-studies.md)

## Вопросы senior-уровня

- почему `s2 := s1` не копирует данные slice и как это приводит к неожиданным изменениям
- когда append создаёт новый backing array, а когда нет — и почему это важно
- почему `copy(dst, src)` может скопировать 0 элементов даже с непустым src
- чем nil slice отличается от empty slice и где это важно
- почему sub-slice может держать большой массив в памяти
- почему `len("привет")` == 12, а не 6, и чем `byte` отличается от `rune`
- почему `s[i]` возвращает байт, а `range` идёт по рунам
- почему substring держит всю исходную строку в памяти и при чём тут `strings.Clone`
- как GMP модель объясняет, почему миллион горутин не означает миллион threads
- почему goroutine stack начинается с 2 KB и как растёт
- как устроен Go аллокатор: mcache/mcentral/mheap
- что такое write barrier и зачем он нужен при concurrent GC
- почему `nil` interface отличается от interface с `nil` внутри
- как happens-before объясняет корректность channel-based синхронизации
- почему `new(T)` не гарантирует heap allocation
- как GOMAXPROCS влияет на CPU throttling в контейнерах
- что происходит с P когда горутина уходит в blocking syscall
- почему 100k соединений не требуют 100k OS threads
- когда `sync.Pool` полезен, а когда нет
- как современная map использует groups, H1/H2 и control bytes для lookup
- почему порядок итерации по map случаен
- зачем tombstone сохраняет корректность probe sequence после delete
- чем Swiss Tables отличаются от исторического chaining через overflow buckets
- когда `sync.Map` подходит лучше typed map под lock

## Подборка

- [Go Documentation](https://go.dev/doc)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Language Specification](https://go.dev/ref/spec)
- [The Go Memory Model](https://go.dev/ref/mem)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
- [Go FAQ](https://go.dev/doc/faq)
