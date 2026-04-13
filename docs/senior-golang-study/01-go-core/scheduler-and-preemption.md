# Scheduler And Preemption

Scheduler Go определяет, как горутины распределяются по потокам и CPU. Это core-тема, потому что она влияет на throughput, fairness, latency и поведение под нагрузкой.

## Ментальная модель

Упрощенная схема runtime:
- `G` - goroutine;
- `M` - machine, то есть OS thread;
- `P` - processor, контекст выполнения runtime.

Важно помнить:
- одновременно исполняться в user code могут не больше `GOMAXPROCS` горутин;
- blocked goroutine не обязана блокировать весь runtime;
- goroutine намного дешевле thread, но не бесплатна.

## Как это работает на практике

Runtime пытается:
- держать работу локально для лучшего cache locality;
- балансировать очереди задач;
- парковать и будить goroutines при блокировках;
- не давать одной goroutine monopolize CPU слишком долго.

Есть локальные очереди у `P` и глобальная очередь. Work stealing помогает перераспределять нагрузку между процессорами runtime.

## Почему `GOMAXPROCS` важен

`GOMAXPROCS` задает число логических процессоров runtime, которые могут одновременно исполнять Go code.

Практически это влияет на:
- parallelism CPU-bound задач;
- конкуренцию за CPU;
- поведение в контейнерах;
- throughput и tail latency.

Нужно понимать:
- больше не всегда лучше;
- при жестком CPU limit завышенный parallelism может ухудшать latency из-за contention и throttling;
- для I/O-bound системы эффект другой, чем для CPU-bound.

## Preemption

Preemption нужна, чтобы long-running goroutine не удерживала CPU слишком долго.

Без этого были бы проблемы:
- плохая fairness;
- рост latency у других задач;
- сложность для GC и runtime service work.

Senior-level понимание:
- preemption не делает код автоматически отзывчивым во всех случаях;
- tight CPU loops, heavy cgo и некоторые blocking patterns все равно требуют внимания;
- "у меня же goroutines легкие" не означает, что scheduler сам решит все проблемы.

## Где scheduler чаще всего ощущается в production

### CPU-bound обработка

Симптомы:
- высокое CPU usage;
- скачки latency;
- неравномерная утилизация cores.

Что проверять:
- `GOMAXPROCS`;
- contention по mutex/atomic;
- не слишком ли крупные batch jobs;
- нет ли long-running loops без естественных yield points.

### I/O-heavy сервисы

Симптомы:
- много горутин;
- периодические хвосты latency;
- рост memory usage из-за накопления blocked goroutines.

Что проверять:
- timeouts и cancellation;
- backpressure;
- bounded worker pools;
- утечки горутин.

## Типовые антипаттерны

- бесконтрольный запуск goroutines "на каждый запрос";
- отсутствие лимитов в fan-out/fan-in схемах;
- ожидание, что scheduler исправит плохой backpressure design;
- CPU-heavy task внутри request path без ограничения concurrency;
- блокирующие операции без timeout и without context propagation.

## Как диагностировать

- `pprof` goroutine profile;
- `pprof` CPU profile;
- `go tool trace`;
- runtime metrics;
- анализ blocked states и очередей работы.

Полезный вопрос:
- latency вызвана scheduler behavior, GC, lock contention или внешним I/O?

## Что могут спросить на интервью

- что такое модель `G-M-P`;
- почему `GOMAXPROCS` не равен "числу горутин";
- как scheduler влияет на fairness и tail latency;
- почему миллион горутин возможен технически, но не всегда разумен архитектурно.

## Связанные темы

- [Garbage Collector](./garbage-collector.md)
- [Memory Model](./memory-model.md)
