# Incident Investigation And Profiling

Этот подпакет про практический production debugging:
- как искать источник деградации;
- как отличать проблему сети от проблемы приложения;
- как использовать traces, metrics, logs и профили;
- как искать утечки, contention и performance regressions в Go.

Материалы:
- [How To Investigate Production Issues](./how-to-investigate-production-issues.md)
- [Go Profiling, Tracing And Performance Debugging](./go-profiling-tracing-and-performance-debugging.md)
- [Finding Leaks, Contention And Memory Problems](./finding-leaks-contention-and-memory-problems.md)

Что важно уметь объяснить:
- с чего начинать расследование инцидента;
- зачем нужны logs, metrics и traces вместе;
- чем pprof отличается от distributed tracing;
- как искать goroutine leak, memory leak и lock contention.
