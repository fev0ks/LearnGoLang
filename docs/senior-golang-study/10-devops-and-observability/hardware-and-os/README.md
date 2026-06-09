# Hardware и OS internals

Что происходит под Go: как работает CPU, как устроена память от регистра до диска, как OS управляет процессами и потоками. Senior backend должен понимать этот уровень не для того чтобы писать ассемблер, а чтобы:

- видеть реальную стоимость операций (RAM 100нс vs SSD 100мкс — разница ×1000)
- понимать почему concurrent код иногда тормозит из-за невидимых причин (cache coherence, false sharing)
- читать профили (page faults, cache misses, context switches) и делать выводы
- проектировать с пониманием физических ограничений (NUMA, network round trips)

## Структура раздела

### Группа 1 — Память: от железа до Go

- [02. Иерархия памяти](./02-memory-hierarchy.md) — пирамида от регистра до сети, latency numbers, DRAM устройство, SSD vs HDD, локальность доступа
- [05. Виртуальная память и paging](./05-virtual-memory-and-paging.md) — VA → PA, page tables, MMU, TLB, page faults, COW, mmap, swap, huge pages, VIRT/RSS/USS
- [03. Cache coherence и MESI](./03-cache-coherence-and-mesi.md) — cache lines, MESI protocol, cache coherence traffic, false sharing с demo в Go, padding, NUMA

### Группа 2 — CPU и concurrency

- [01. CPU architecture](./01-cpu-architecture.md) — pipeline, superscalar, OoO, branch prediction (с примером sorted/unsorted), speculative execution и Spectre, SIMD, SMT/Hyperthreading
- [04. Atomics и memory ordering](./04-atomics-and-memory-ordering.md) — store buffers, StoreLoad reordering (с примером (0,0)), x86 TSO vs ARM weak, fences, LOCK prefix, CAS, acquire/release, sync/atomic в Go, как Mutex использует atomics

### Группа 3 — OS threading

- [06. Процессы и потоки](./06-processes-and-threads.md) — PID/TID, fork/exec/clone, kernel vs user mode, syscalls, task_struct, /proc, goroutines vs threads vs processes, когда Go создаёт OS thread, LockOSThread
- [07. Context switching и scheduling](./07-context-switching-and-scheduling.md) — что сохраняется при switch, стоимость через cold cache, CFS, nice values, I/O vs CPU bound, Go M:N scheduler, async preemption в 1.14+, когда goroutine паркуется, GOMAXPROCS в контейнерах

## Что должен знать senior

- порядки величин: L1 (1нс) vs RAM (100нс) vs SSD (100мкс) vs network (мс)
- разницу между VIRT и RSS, почему swap убивает latency
- что такое page fault и как minor отличается от major
- как работает COW и почему fork() — дешёвая операция
- что такое cache line (64 байта) и почему она единица coherence
- что такое false sharing и как его избежать через padding
- почему один и тот же atomic counter под нагрузкой может стать узким местом
- что такое memory ordering и почему даже на x86 возможен (0, 0) в IRIW (см. файл 04)

## Связанные разделы

- [Linux primitives](../linux/) — Linux-specific детали (virtual memory tuning, signals, sockets, namespaces)
- [Go memory internals](../../01-go-core/memory-internals/) — Go runtime: stack/heap, escape analysis, GC
- [Go scheduler](../../01-go-core/runtime-scheduler/01-scheduler-and-preemption.md) — M:N scheduler Go runtime
- [Concurrency primitives](../../01-go-core/concurrency-and-performance/03-sync-primitives.md) — mutex, atomic в Go
- [Profiling](../../01-go-core/profiling/) — pprof, perf, измерение реальных эффектов

## Внешние ссылки

- [What Every Programmer Should Know About Memory](https://www.akkadia.org/drepper/cpumemory.pdf) — Ulrich Drepper, классика
- [Latency Numbers Every Programmer Should Know](https://gist.github.com/jboner/2841832) — Jeff Dean's list
- [The Architecture of Open Source Applications: LMDB](http://www.lmdb.tech/doc/) — пример работы с виртуальной памятью через mmap
- [PostgreSQL: Architecture](https://www.postgresql.org/docs/current/tutorial-arch.html) — как БД использует знание hardware
