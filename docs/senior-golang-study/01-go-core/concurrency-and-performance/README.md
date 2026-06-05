# Concurrency And Performance

Это один из самых важных разделов для senior Go.

## Материалы

- [01. Goroutines And Channels](./01-goroutines-and-channels.md) — lifecycle, unbuffered/buffered, pipeline, fan-out/fan-in, done-channel, goroutine leak, select
- [02. Sync Primitives](./02-sync-primitives.md) — Mutex/RWMutex, WaitGroup, Once, Cond, Pool, Map, atomic
- [03. Worker Pool](./03-worker-pool.md) — разбор task_before.go (5 багов), правильная реализация, errCh паттерн, graceful shutdown, semaphore
- [04. Context Patterns](./04-context-patterns.md) — Background/TODO, WithCancel/Timeout/Deadline, propagation, context.Value anti-patterns, defer cancel()

> Профилирование (pprof, CPU/memory/goroutine профили, execution tracer, benchmarks) вынесено в отдельный подраздел go-core: [../profiling/](../profiling/) — оно общее для всего Go, не только для concurrency.

---

Темы (конспекты в разработке):
- goroutine lifecycle;
- channels, buffering, cancellation;
- worker pools и bounded concurrency;
- mutex, atomic, condition variables;
- race detector и memory visibility;
- allocation hotspots;
- CPU-bound vs IO-bound workloads;
- profiling через `pprof`;
- benchmark methodology;
- latency spikes, GC pauses, queue buildup.

Практические вопросы:
- когда channel хуже mutex;
- как ограничить fan-out;
- как найти goroutine leak;
- как измерять производительность до и после оптимизации;
- почему throughput вырос, а p99 стал хуже.

## Подборка

- [The Go Memory Model](https://go.dev/ref/mem)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [runtime/pprof](https://pkg.go.dev/runtime/pprof)
- [runtime/trace](https://pkg.go.dev/runtime/trace)
- [Profile-guided optimization](https://go.dev/doc/pgo)

## Вопросы

- когда канал нужен как coordination primitive, а когда mutex проще;
- как bounded concurrency защищает сервис от самоуничтожения под нагрузкой;
- почему race detector не находит все concurrency bugs;
- как GC pressure влияет на tail latency;
- как отличить CPU bottleneck от lock contention;
- что именно ты посмотришь в pprof при росте latency;
- почему microbenchmark может не отражать поведение production path.
