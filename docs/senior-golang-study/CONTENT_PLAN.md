# Content Plan

Рабочий план дополнений справочника. Не коммитить.

Источники:
- Примеры кода из `/Users/fev0ks/Projects/personal/lrn-streams` — RabbitMQ, Redis Streams, Redis Pub/Sub, gRPC bidi streaming
- Задача с собеседования: `topics/08-interview-prep/interview/before/task_before.go` (до) и `after_gpt/task_after_gpt.go` (после)

---

## 1. Go Core — дополнения в `01-go-core/`

### Generics → `01-go-core/09-generics.md`
- [ ] Type parameters и constraints — синтаксис, `any`, `comparable`, кастомные
- [ ] Когда generics vs `interface{}` vs кодогенерация — trade-offs
- [ ] Generic data structures: Set, Stack, слайс-утилиты (Map, Filter, Reduce)
- [ ] Пакеты `slices`, `maps`, `cmp` из stdlib — практическое использование
- [ ] Подводные камни: нельзя generic methods, type inference ограничения
- [ ] Производительность: когда generics медленнее interface из-за devirtualization

### Error Handling → `01-go-core/10-error-handling.md`
- [ ] `errors.Is` / `errors.As` — механика wrapping chain, примеры
- [ ] Sentinel errors vs типизированные ошибки — когда что и почему
- [ ] Оборачивание с контекстом: `fmt.Errorf("doing X: %w", err)` — правила
- [ ] Кастомные типы ошибок: `type ValidationError struct{}` с методом `Error()`
- [ ] Типичные анти-паттерны: проглатывание, `panic` вместо error, двойной return
- [ ] `errgroup` — параллельные задачи с первой ошибкой
- [ ] Ошибки в конкурентном коде — как передавать через каналы, errCh паттерн
- [ ] `errors.Join` (Go 1.20+) — объединение нескольких ошибок

---

## 2. Конкурентность — наполнить `09-concurrency-and-performance/`

### Goroutines & Channels → `09-concurrency-and-performance/01-goroutines-and-channels.md`
- [ ] Goroutine lifecycle: запуск, стек (~2KB), рост стека, завершение
- [ ] Unbuffered vs buffered channel: семантика, когда что
- [ ] Pipeline паттерн: gen → stage → stage → sink
- [ ] Fan-out / Fan-in: распределение и сборка результатов
- [ ] Done-channel для отмены: `select { case <-done: return }`
- [ ] Goroutine leak: причины (блокировка на канале, забытый receive) и как ловить
- [ ] `context` для отмены: передача через цепочку вызовов, `ctx.Done()`

### Sync Primitives → `09-concurrency-and-performance/02-sync-primitives.md`
- [ ] `sync.Mutex` vs `sync.RWMutex` — когда RW выгоден, когда нет
- [ ] Типичные ошибки: копирование Mutex (go vet ловит), lock без unlock
- [ ] `sync.WaitGroup` — Add/Done/Wait, почему Add перед go
- [ ] `sync.Once` — ленивая инициализация, паника внутри Once
- [ ] `sync.Cond` — когда нужен вместо канала, Broadcast vs Signal
- [ ] `sync.Pool` — снижение аллокаций, поведение при GC
- [ ] `sync.Map` — когда лучше map+mutex, когда хуже
- [ ] `atomic` пакет — операции на int32/int64/pointer, когда вместо mutex

### Worker Pool Pattern → `09-concurrency-and-performance/03-worker-pool.md`
- [ ] Разбор задачи с собеседования: task_before.go — все баги по пунктам
  - nil channels (var jobs chan int — не инициализирован)
  - race condition на f.cache (нет mutex)
  - нет WaitGroup → out никогда не закрывается → range зависнет
  - context создан но не передан в FetchAll
  - горутины утекают (producer пишет в nil jobs → panic)
- [ ] Правильная реализация: task_after_gpt.go с комментариями
- [ ] Worker pool шаблон: jobs channel + WaitGroup + closer goroutine
- [ ] Паттерн errCh: `chan error, 1` — первая ошибка wins
- [ ] Graceful shutdown: ctx.Done() в producer и workers
- [ ] Semaphore через buffered channel как альтернатива pool

### Context → `09-concurrency-and-performance/04-context-patterns.md`
- [ ] `context.Background()` vs `context.TODO()` — когда что
- [ ] WithCancel, WithTimeout, WithDeadline — примеры и отличия
- [ ] Propagation: почему ctx первый аргумент, не поле struct
- [ ] `context.Value` — когда допустимо, когда анти-паттерн (не для бизнес-данных)
- [ ] Отмена и cleanup: `defer cancel()` всегда
- [ ] context в HTTP сервере: `r.Context()`, клиентские таймауты
- [ ] Типичные ошибки: сохранение ctx в struct, создание без передачи

---

## 3. Message Brokers — наполнить `07-message-brokers-and-streaming/`

### Kafka → `07-message-brokers-and-streaming/01-kafka.md`
*(нет готового материала — писать с нуля)*
- [ ] Архитектура: broker, topic, partition, offset, consumer group, ISR
- [ ] Delivery semantics: at-most-once, at-least-once, exactly-once (и почему последнее дорого)
- [ ] Producer: batching, compression, `acks` (0/1/all) — trade-offs
- [ ] Consumer: poll loop, commit offset (auto vs manual), rebalance
- [ ] Kafka в Go: franz-go vs sarama vs confluent-kafka-go — сравнение
- [ ] Практика: producer с retry, consumer с manual commit + idempotency
- [ ] DLQ паттерн для Kafka
- [ ] Log compaction vs retention — когда что
- [ ] Когда Kafka не нужен

### RabbitMQ → `07-message-brokers-and-streaming/02-rabbitmq.md`
*(основа: lrn-streams/docs/rabbitmq.md + internal/transport/rabbitmq/)*
- [ ] Архитектура: exchange, queue, binding, routing key
- [ ] Типы exchange: fanout, direct, topic, headers — с примерами
- [ ] Delivery: ack/nack, prefetch, DLQ
- [ ] Go код: publisher и subscriber из lrn-streams (адаптировать)
- [ ] Когда RabbitMQ, когда Kafka

### Redis Streams → `07-message-brokers-and-streaming/03-redis-streams.md`
*(основа: lrn-streams/docs/redis-streams.md + internal/transport/redisstream/)*
- [ ] XADD, XREADGROUP, XACK — механика
- [ ] Consumer groups: как работают, одно сообщение → один consumer
- [ ] Pending entries list, XCLAIM для восстановления
- [ ] Go код: producer и consumer из lrn-streams (адаптировать)
- [ ] Когда Redis Streams vs Kafka vs RabbitMQ

### Redis Pub/Sub → `07-message-brokers-and-streaming/04-redis-pubsub.md`
*(основа: lrn-streams/docs/redis-pubsub.md + internal/transport/redispubsub/)*
- [ ] PUBLISH/SUBSCRIBE/PSUBSCRIBE — механика, at-most-once
- [ ] Отличие от Redis Streams: нет персистентности, нет consumer groups
- [ ] Go код из lrn-streams (адаптировать)
- [ ] Use cases: backplane между инстансами, cache invalidation, presence

### Cloud Pub/Sub → `07-message-brokers-and-streaming/05-cloud-pubsub.md`
- [ ] Google Cloud Pub/Sub: topics, subscriptions, ack deadline, dead letter
- [ ] AWS SNS + SQS: fan-out паттерн, SNS → multiple SQS
- [ ] Когда cloud vs self-hosted

### gRPC Streaming → `07-message-brokers-and-streaming/06-grpc-streaming.md`
*(основа: lrn-streams/docs/grpc-bidi-stream.md + internal/transport/grpcstream/)*
- [ ] 4 типа gRPC: unary, server-side, client-side, bidirectional
- [ ] Когда gRPC streaming как замена message broker
- [ ] Go код: server и client из lrn-streams (адаптировать + объяснить)
- [ ] Backpressure в gRPC streams
- [ ] Multi-broker backplane через Redis (уже есть в lrn-streams)

### Сравнение → `07-message-brokers-and-streaming/07-comparison.md`
- [ ] Большая таблица: Kafka / RabbitMQ / Redis Streams / Redis Pub/Sub / gRPC Stream / Cloud Pub/Sub
  - Персистентность, delivery semantics, consumer groups, replay, throughput, latency, сложность
- [ ] Decision tree: когда что выбирать
- [ ] Типичные ошибки выбора (Redis Pub/Sub для надёжной доставки и т.п.)

---

## 4. API Protocols — добавить в `08-networking-and-api/`

### gRPC → `08-networking-and-api/grpc/`
- [ ] `01-grpc-overview.md` — Protobuf, HTTP/2, 4 типа RPC, когда vs REST
- [ ] `02-grpc-in-go.md` — кодогенерация, grpc-go, interceptors, health check, reflection
- [ ] `03-grpc-streaming.md` — ссылка/краткое резюме (детали в 07-message-brokers)

### REST → `08-networking-and-api/rest/`
- [ ] `01-rest-principles.md` — stateless, uniform interface, ресурсы (теория + практика)
- [ ] `02-http-server-in-go.md` — net/http server, middleware chain, timeouts, graceful shutdown
- [ ] `03-http-client-in-go.md` — Transport, Connection pooling, таймауты, retry
- [ ] Заметка: REST API design уже есть в `04-architecture-and-patterns/patterns/07-rest-api-design.md` — ссылка

### WebSocket → `08-networking-and-api/websocket.md`
- [ ] Handshake (Upgrade), framing, opcodes (text/binary/ping/pong/close)
- [ ] Go: gorilla/websocket vs nhooyr.io/websocket — сравнение
- [ ] Паттерн: read-goroutine + write-goroutine + hub
- [ ] Scaling: sticky sessions vs pub/sub backplane (Redis)
- [ ] Отличие от SSE и long polling

### Webhooks → `08-networking-and-api/webhooks.md`
- [ ] Механика: POST на URL потребителя при событии
- [ ] Delivery guarantees: at-least-once, idempotency key
- [ ] Security: HMAC-SHA256 signature verification (пример на Go)
- [ ] Retry стратегия: exponential backoff, dead letter
- [ ] Отличие от polling и WebSocket

### GraphQL → `08-networking-and-api/graphql.md`
- [ ] Schema, query, mutation, subscription
- [ ] N+1 проблема и DataLoader как решение
- [ ] Fragments, introspection, persisted queries
- [ ] Go: gqlgen vs graphql-go
- [ ] Когда GraphQL, когда REST — честные trade-offs

### WebRTC → `08-networking-and-api/webrtc.md`
*(overview уровень, не deep dive)*
- [ ] Peer-to-peer соединение: signaling, ICE, STUN/TURN
- [ ] SDP offer/answer обмен
- [ ] Когда WebRTC (реальное время, P2P) vs WebSocket (клиент-сервер)
- [ ] Pion — Go библиотека для WebRTC

### SOAP → `08-networking-and-api/soap.md`
*(legacy, но встречается)*
- [ ] WSDL, конверт, заголовки, fault
- [ ] Когда ещё встречается в 2025 (банки, legacy enterprise)
- [ ] Как вызывать SOAP из Go: soap клиент, генерация из WSDL
- [ ] Сравнение с REST/gRPC: почему SOAP проиграл

### Сравнение протоколов → `08-networking-and-api/protocol-comparison.md`
- [ ] Большая таблица: REST / gRPC / GraphQL / WebSocket / WebHooks / WebRTC / SOAP
  - Transport, формат, streaming, browser support, сложность, use case
- [ ] Decision tree: клиентское взаимодействие vs сервис-сервис vs real-time vs events

---

## Порядок выполнения (рекомендуемый)

1. **Error handling** — короткий файл, высокая ценность, никаких зависимостей
2. **Generics** — самодостаточно, часто спрашивают
3. **Goroutines & Channels + Worker Pool** — используем задачу с собеседования
4. **Sync Primitives + Context** — продолжение темы конкурентности
5. **Kafka** — с нуля, важнее всего из brokers
6. **RabbitMQ + Redis Streams + Redis Pub/Sub** — адаптация из lrn-streams
7. **gRPC** (08-networking) — overview + Go implementation
8. **REST/HTTP в Go** — server + client deep dive
9. **WebSocket + Webhooks** — практические, часто спрашивают
10. **Cloud Pub/Sub + gRPC Streaming** — на основе lrn-streams
11. **GraphQL + WebRTC + SOAP** — менее приоритетны
12. **Comparison documents** — в конце, когда все части готовы

---

## Статус

- [x] 01. Error handling → `01-go-core/10-error-handling.md`
- [x] 02. Generics → `01-go-core/11-generics.md`
- [x] 03. Goroutines & Channels → `09-concurrency-and-performance/01-goroutines-and-channels.md`
- [x] 04. Worker Pool (+ разбор задачи с собеседования) → `09-concurrency-and-performance/03-worker-pool.md`
- [x] 05. Sync Primitives → `09-concurrency-and-performance/02-sync-primitives.md`
- [x] 06. Context Patterns → `09-concurrency-and-performance/04-context-patterns.md`
- [x] 07. Kafka → `07-message-brokers-and-streaming/01-kafka.md`
- [x] 08. RabbitMQ → `07-message-brokers-and-streaming/02-rabbitmq.md`
- [x] 09. Redis Streams → `07-message-brokers-and-streaming/03-redis-streams.md`
- [x] 10. Redis Pub/Sub → `07-message-brokers-and-streaming/04-redis-pubsub.md`
- [ ] 11. Cloud Pub/Sub
- [x] 12. gRPC Streaming (07-message-brokers) → `07-message-brokers-and-streaming/06-grpc-streaming.md`
- [x] 13. Brokers Comparison → `07-message-brokers-and-streaming/07-comparison.md`
- [x] 14. gRPC Overview + Go (08-networking) → `08-networking-and-api/grpc-overview.md`
- [x] 15. REST/HTTP Server in Go → `08-networking-and-api/http-server-in-go.md`
- [x] 16. REST/HTTP Client in Go → `08-networking-and-api/http-client-in-go.md`
- [x] 17. WebSocket → `08-networking-and-api/websocket.md`
- [x] 18. Webhooks → `08-networking-and-api/webhooks.md`
- [x] 19. GraphQL → `08-networking-and-api/graphql.md`
- [ ] 20. WebRTC
- [ ] 21. SOAP
- [x] 22. Protocol Comparison → `08-networking-and-api/protocol-comparison.md`
