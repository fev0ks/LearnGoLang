# Concurrency And Performance

Это один из самых важных разделов для senior Go.

Темы:
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
