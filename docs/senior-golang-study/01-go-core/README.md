# Go Core

Язык и runtime Go. Читать по порядку — каждый файл строится на предыдущем.

## Материалы

- [01. Primitive Types And Zero Values](./01-primitive-types-and-zero-values.md) — встроенные типы, zero values, поведение nil slice/map/chan
- [02. Numeric Types, Sizes And Overflow](./02-numeric-types-integer-sizes-and-overflow.md) — int vs int64, диапазоны, overflow
- [03. Value vs Pointer Semantics](./03-value-vs-pointer-semantics.md) — когда копировать, когда брать указатель; mutex copy bug; slice aliasing
- [04. Interfaces, Method Sets And Nil](./04-interfaces-method-sets-and-nil.md) — iface/eface layout, itab vtable, typed nil trap, method sets
- [05. Slices](./05-slices.md) — slice header (ptr/len/cap), shared backing array, append реаллокация, sub-slice, copy ловушки, nil vs empty, memory retention
- [06. Memory Model](./06-memory-model.md) — happens-before, channel/mutex/Once/atomic гарантии, data race, race detector
- [07. Scheduler And Preemption](./07-scheduler-and-preemption.md) — GMP модель, work stealing, async preemption, syscall handoff, GOMAXPROCS в контейнерах
- [08. Syscall](./08-syscall.md) — entersyscall/exitsyscall, P handoff, sysmon retake, CGo цена, LockOSThread, thread exhaustion
- [09. Netpoller](./09-netpoller.md) — epoll/kqueue интеграция, pollDesc, горутина parking/wakeup, SetDeadline, DNS resolver
- [Map Internals](./map-internals/) — hmap+bmap (до 1.24), Swiss Tables (1.24+), ctrl bytes, matchH2, tombstones (подраздел)
- [Memory Internals](./memory-internals/) — стек и heap, аллокатор, escape analysis, GC (подраздел)

## Map Internals (подраздел)

Детальный разбор внутренней реализации map:

- [01. hmap + bmap](./map-internals/01-hmap-before-1.24.md) — до Go 1.24: bucket layout, tophash, overflow chains, incremental evacuation
- [02. Swiss Tables](./map-internals/02-swiss-tables-since-1.24.md) — с Go 1.24: open addressing, ctrl bytes, matchH2 bitset, directory

## Memory Internals (подраздел)

Материалы про управление памятью вынесены в отдельный подраздел, потому что они связаны между собой:

- [01. Stack And Heap](./memory-internals/01-stack-and-heap.md) — goroutine stack, heap arenas, scavenger, RSS vs VSZ
- [02. Allocator](./memory-internals/02-allocator.md) — size classes, mcache/mcentral/mheap, tiny allocator, noscan
- [03. Escape Analysis](./memory-internals/03-escape-analysis.md) — stack vs heap решение компилятора, `-gcflags=-m`
- [04. Garbage Collector](./memory-internals/04-garbage-collector.md) — tri-color, write barrier, GOGC, GOMEMLIMIT, gctrace

## Вопросы senior-уровня

- почему `s2 := s1` не копирует данные slice и как это приводит к неожиданным изменениям
- когда append создаёт новый backing array, а когда нет — и почему это важно
- почему `copy(dst, src)` может скопировать 0 элементов даже с непустым src
- чем nil slice отличается от empty slice и где это важно
- почему sub-slice может держать большой массив в памяти
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
- как устроен bucket в hmap: tophash, раздельное хранение ключей и значений
- почему порядок итерации по map случаен
- что такое Swiss Tables и чем они лучше chaining через overflow buckets
- как matchH2 проверяет 8 слотов одной битовой операцией

## Подборка

- [Go Documentation](https://go.dev/doc)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Language Specification](https://go.dev/ref/spec)
- [The Go Memory Model](https://go.dev/ref/mem)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
- [Go FAQ](https://go.dev/doc/faq)
