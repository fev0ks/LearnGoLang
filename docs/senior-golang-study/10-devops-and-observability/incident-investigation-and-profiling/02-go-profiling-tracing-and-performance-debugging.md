# Go Profiling, Tracing And Performance Debugging

Профилирование и tracing решают разные задачи.

## Traces

Distributed tracing нужен, чтобы понять путь внешнего запроса через систему.

Он отвечает на вопросы:
- через какие сервисы прошел запрос;
- где самый длинный span;
- где network hop;
- где downstream call.

Traces особенно полезны для:
- multi-service systems;
- request-level latency;
- dependency graph.

## Profiling

Profiling нужен, чтобы понять, что происходит внутри процесса.

Он отвечает на вопросы:
- где тратится CPU;
- кто аллоцирует память;
- где растут goroutines;
- где lock contention;
- где блокировки и wait.

В Go для этого обычно используют `pprof`.

## Что смотреть в Go

CPU profile:
- какие функции реально жрут CPU.

Heap profile:
- кто держит память;
- где allocation hotspots.

Goroutine profile:
- сколько горутин;
- на чем они висят.

Block profile:
- где горутины ждут blocking operations.

Mutex profile:
- где lock contention.

## Когда использовать tracing, а когда profiling

Tracing:
- "какой сервис или span медленный"

Profiling:
- "почему этот процесс медленный внутри себя"

Часто это два последовательных шага:
- trace показывает, что тормозит `user-service`;
- profile показывает, что там slow JSON encode или heavy allocation.

## Practical rule

Если проблема end-to-end:
- начни с metrics и traces.

Если проблема локализована в одном Go процессе:
- переходи к pprof и runtime diagnostics.
