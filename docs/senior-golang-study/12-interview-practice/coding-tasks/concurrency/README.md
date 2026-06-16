# Concurrency Tasks

Задачи на concurrency — самая частая категория для Go-собеседований. Знание этих паттернов обязательно для senior Go-разработчика.

## Задачи

1. [Worker Pool](./01-worker-pool.md) — ограничить количество параллельных горутин, graceful shutdown
2. [Rate Limiter](./02-rate-limiter.md) — token bucket, leaky bucket, sliding window
3. [Fan-In / Fan-Out](./03-fan-in-fan-out.md) — разделить работу и собрать результаты
4. [Pipeline](./04-pipeline.md) — цепочка обработки через каналы
5. [Pub/Sub In-Memory](./05-pubsub.md) — один publisher, много subscriber'ов
6. [Singleflight](./06-singleflight.md) — дедупликация одинаковых concurrent запросов
7. [Worker Pool (debug)](./07-worker-pool-debug.md) — найти 5 багов в типовой реализации, errCh, graceful shutdown, semaphore + паттерны

## Что важно знать

### Channels vs Mutex — когда что

**Channel:**
- Передача владения данными между goroutines ("я закончил, держи")
- Координация (сигнал "done", синхронизация фаз)
- Pipeline / fan-in / fan-out

**Mutex:**
- Защита изменяемого состояния (counter, map, cache)
- Когда несколько goroutines одновременно работают с одной структурой

**Правило большого пальца:** "Don't communicate by sharing memory; share memory by communicating." Но это **не догма**. Mutex часто проще и быстрее для простых случаев.

### Context везде

Любая долгоживущая операция должна принимать `context.Context` первым параметром:
```go
func (s *Service) Process(ctx context.Context, req Request) error
```

Без context'а нет cancellation, нет timeout, нет propagation request ID. На собеседовании отсутствие context — красный флаг.

### Channel buffering

- `make(chan T)` — unbuffered, синхронизация (send блокирует пока receive)
- `make(chan T, N)` — buffered, asynchronous до N элементов
- `make(chan T, 1)` — "signal" канал для одного события

**На собеседовании можешь объяснить почему выбрал такой буфер?** Это важный сигнал.

### Никогда не делай

```go
// ❌ Leak горутин — нет cancellation
go func() {
    for {
        process()
    }
}()

// ❌ Send в nil channel — навсегда блокирует
var ch chan int
ch <- 1

// ❌ Close channel дважды — panic
close(ch)
close(ch)  // panic!

// ❌ Send в закрытый channel — panic
close(ch)
ch <- 1  // panic!

// ❌ Захват loop variable goroutine'ой (до Go 1.22)
for _, v := range items {
    go func() { process(v) }()  // v одинаковый для всех!
}
// Go 1.22+ это исправлено; в более старых надо:
for _, v := range items {
    v := v
    go func() { process(v) }()
}
```

### Базовый "проверочный список" любого concurrent кода

```
□ Все goroutines имеют способ остановиться (context, channel, кончился input)
□ Нет deadlock — горутины не ждут друг друга бесконечно
□ Нет goroutine leak — после завершения работы все горутины завершились
□ Нет race condition — `go test -race` проходит
□ Producer не блокируется навсегда если consumer медленный (backpressure)
□ Errors собираются и возвращаются (через errgroup или error channel)
□ panic в goroutine не валит весь процесс (recover где нужно)
```

## Связки

- [Concurrency и channels](../../../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md) — детальная теория
- [Sync primitives](../../../01-go-core/concurrency-and-performance/03-sync-primitives.md) — mutex, atomic, sync.Map
- [Worker pool patterns](07-worker-pool-debug.md) — расширенные варианты
- [Context patterns](../../../01-go-core/concurrency-and-performance/04-context-patterns.md) — context propagation
