# Roadmap

Репозиторий большой — этот файл помогает понять, что здесь есть и в каком порядке читать.

Приоритеты материалов: `★★★` — спрашивают почти везде, `★★` — важно, `★` — полезно.

---

## Карта разделов

| Раздел | Что внутри | Статус |
|--------|------------|--------|
| [01 Go Core](../01-go-core/README.md) | типы/nil, slices, strings, memory model, scheduler+syscall+netpoller+timers, map internals, memory internals (stack/heap, allocator, escape, GC), concurrency & performance (goroutines, channels, sync, context, pprof) | ✅ 30+ файлов + примеры |
| [02 Go Stdlib](../02-go-stdlib-and-tools/README.md) | net/http, context, sync, encoding/json, pprof | темы + ссылки |
| [03 Go Libraries](../03-go-libraries-and-ecosystem/README.md) | chi, pgx, zap, testify, wire/fx — сравнения | темы + ссылки |
| [04 Architecture](../04-architecture-and-patterns/README.md) | Go patterns, service topologies, DDD, SOLID, API versioning, background workers | ✅ 10 файлов |
| [05 System Design](../05-system-design/README.md) | request flows, feature flags, A/B tests, edge proxy | ✅ 16 файлов |
| [06 Databases](../06-databases/README.md) | SQL/NoSQL, indexes, transactions, Redis, Go DB libraries | ✅ 29 файлов |
| [07 Message Brokers](../07-message-brokers-and-streaming/README.md) | Kafka, RabbitMQ, NATS, delivery semantics | темы + ссылки |
| [08 Networking & API](../08-networking-and-api/README.md) | HTTP/TLS, request lifecycle, DNS, CDN, rate limiting | ✅ 9 файлов |
| [09 Testing](../09-testing-and-quality/README.md) | unit/integration/e2e, test doubles, race/fuzz, linters | ✅ 7 файлов |
| [10 DevOps & Observability](../10-devops-and-observability/README.md) | Linux, Docker, Kubernetes, metrics, traces, logs, profiling | ✅ 40+ файлов |
| [11 Security](../11-security/README.md) | secrets, TLS/mTLS, CORS, DDoS protection | ✅ 8 файлов |
| [12 Interview Practice](../12-interview-practice/README.md) | behavioral кейсы, system design drills | темы + ссылки |
| [15 Go Version Differences](../15-go-version-differences/README.md) | Go 1.24, 1.25, 1.26 — что изменилось | ✅ 3 файла |
| [16 Algorithms And Data Structures](../16-algorithms-and-data-structures/README.md) | O-нотация, two pointers, binary search, DP, graphs, heap, backtracking | ✅ 8 файлов |

> **Разделы "темы + ссылки"** — содержат только README с темами и внешними ссылками; конспекты для них еще не написаны.

---

## Маршрут прохождения

### Фаза 1 — Go runtime и конкурентность

**Цель:** объяснить, как Go работает под капотом — планировщик, GC, interfaces/nil, memory model.

#### 01 Go Core — основы языка

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01. Primitive Types, Sizes And Overflow](../01-go-core/01-primitive-types-and-zero-values.md) | zero values, nil slice/map/chan, размеры, диапазоны, int vs int64, overflow, конверсии | ★ |
| [02. Value vs Pointer Semantics](../01-go-core/02-value-vs-pointer-semantics.md) | когда копировать, mutex copy bug, slice aliasing | ★★ |
| [03. Interfaces, Method Sets And Nil](../01-go-core/03-interfaces-method-sets-and-nil.md) | iface/eface layout, itab vtable, typed nil trap | ★★★ |
| [04. Slices](../01-go-core/04-slices.md) | slice header, shared backing array, append реаллокация, copy ловушки, memory retention | ★★★ |
| [05. Error Handling](../01-go-core/05-error-handling.md) | errors.Is/As, wrapping chain, sentinel vs typed, errgroup, errors.Join | ★★ |
| [06. Generics](../01-go-core/06-generics.md) | type parameters, constraints, ~underlying, slices/maps/cmp, производительность | ★★ |
| [07. Strings](../01-go-core/07-strings.md) | string header, immutability, byte vs rune, UTF-8, range по рунам, конверсии, substring retention | ★★ |
| [08. Unsafe And Low-Level](../01-go-core/08-unsafe-and-low-level.md) | unsafe.Pointer vs uintptr, Sizeof/Alignof/Offsetof, padding, zero-copy string↔[]byte | ★ |

#### 01 Go Core — Runtime Scheduler (подраздел)

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01. Scheduler And Preemption](../01-go-core/runtime-scheduler/01-scheduler-and-preemption.md) | GMP, LRQ/GRQ, work stealing, async preemption, sysmon, GOMAXPROCS | ★★★ |
| [02. Syscall](../01-go-core/runtime-scheduler/02-syscall.md) | entersyscall/exitsyscall, P handoff, sysmon retake, vDSO, CGo, thread exhaustion | ★★★ |
| [03. Netpoller](../01-go-core/runtime-scheduler/03-netpoller.md) | epoll/kqueue, pollDesc, parking/wakeup, куда приходят данные, SetDeadline | ★★ |
| [04. Timers](../01-go-core/runtime-scheduler/04-timers.md) | time.Sleep/Timer/Ticker, per-P timer heap, почему не syscall, утечки | ★★ |
| 🧪 [examples/schedtrace](../01-go-core/runtime-scheduler/examples/schedtrace/) | запускаемое демо: `GODEBUG=schedtrace=1000`, work stealing, spinning M | — |

#### 01 Go Core — Memory Internals (подраздел)

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01. Stack And Heap](../01-go-core/memory-internals/01-stack-and-heap.md) | goroutine stack (2KB, рост копированием), heapArena, scavenger, RSS vs VSZ | ★★ |
| [02. Allocator](../01-go-core/memory-internals/02-allocator.md) | size classes, mcache/mcentral/mheap, tiny allocator, noscan, large objects | ★★ |
| [03. Escape Analysis](../01-go-core/memory-internals/03-escape-analysis.md) | stack vs heap, причины escape, `-gcflags=-m`, goroutine capture | ★★★ |
| [04. Garbage Collector](../01-go-core/memory-internals/04-garbage-collector.md) | tri-color, write barrier, фазы GC, GOGC, GOMEMLIMIT, Green Tea GC, gctrace | ★★★ |
| 🧪 [examples/gctrace](../01-go-core/memory-internals/examples/gctrace/) | запускаемое демо: `GODEBUG=gctrace=1`, формула NextGC, GOGC/GOMEMLIMIT | — |

#### 01 Go Core — Map Internals (подраздел)

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01. Swiss Tables (1.24+)](../01-go-core/map-internals/01-swiss-tables-since-1.24.md) | open addressing, ctrl bytes, matchH2 bitset, directory | ★★ |
| [02. hmap + bmap (до 1.24)](../01-go-core/map-internals/02-hmap-before-1.24.md) | bucket layout, tophash, overflow chains, incremental evacuation | ★★ |

#### 01 Go Core — Concurrency & Performance (подраздел)

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01. Memory Model](../01-go-core/concurrency-and-performance/01-memory-model.md) | happens-before, channel/mutex/Once/atomic гарантии, data race, race detector | ★★★ |
| [02. Goroutines And Channels](../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md) | lifecycle, buffered/unbuffered, pipeline, fan-out/fan-in, goroutine leak, select | ★★★ |
| [03. Sync Primitives](../01-go-core/concurrency-and-performance/03-sync-primitives.md) | Mutex/RWMutex, WaitGroup, Once, Cond, Pool, Map, atomic, singleflight | ★★★ |
| [04. Context Patterns](../01-go-core/concurrency-and-performance/04-context-patterns.md) | WithCancel/Timeout/Deadline, propagation, context.Value анти-паттерны | ★★ |
| [Worker Pool (debug)](../12-interview-practice/coding-tasks/concurrency/07-worker-pool-debug.md) | баги типовой реализации, errCh, graceful shutdown, semaphore — в практике | ★★ |

→ [README с темами и вопросами](../01-go-core/concurrency-and-performance/README.md)

#### 15 Go Version Differences

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [go1.24.md](../15-go-version-differences/go1.24.md) | изменения в Go 1.24 | ★★ |
| [go1.25.md](../15-go-version-differences/go1.25.md) | изменения в Go 1.25 | ★★ |
| [go1.26.md](../15-go-version-differences/go1.26.md) | изменения в Go 1.26 | ★ |

---

### Фаза 2 — Архитектура, паттерны и system design

**Цель:** обосновать выбор топологии сервиса, проектировать с учетом отказов, объяснить outbox/saga/idempotency и feature rollout.

#### 04 Architecture And Patterns

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [go-code-patterns/](../04-architecture-and-patterns/patterns/go-code-patterns/) | functional options, small interfaces, middleware, decorator, strategy, repository, UoW, context, errors (4 файла по типам) | ★★★ |
| [02-architecture-patterns.md](../04-architecture-and-patterns/patterns/02-architecture-patterns.md) | hexagonal, DDD lite, layered, clean arch — trade-offs | ★★★ |
| [05-ddd-in-go.md](../04-architecture-and-patterns/patterns/05-ddd-in-go.md) | Entity, Value Object, Aggregate, Domain Events, Repository, Application Service | ★★★ |
| [06-solid-in-go.md](../04-architecture-and-patterns/patterns/06-solid-in-go.md) | SRP, OCP, LSP, ISP, DIP — с Go-примерами | ★★★ |
| [03-api-versioning.md](../04-architecture-and-patterns/patterns/03-api-versioning.md) | REST/gRPC versioning, Protobuf rules, deprecation lifecycle | ★★ |
| [04-background-workers.md](../04-architecture-and-patterns/patterns/04-background-workers.md) | worker pool, graceful shutdown, distributed lease, idempotent workers | ★★ |
| [01-monolith-vs-modular-monolith-vs-microservices.md](../04-architecture-and-patterns/service-topologies/01-monolith-vs-modular-monolith-vs-microservices.md) | когда что выбирать, стоимость распределенности | ★★★ |
| [02-typical-problems-and-how-to-mitigate-them.md](../04-architecture-and-patterns/service-topologies/02-typical-problems-and-how-to-mitigate-them.md) | outbox, saga, idempotency, retry storms, distributed tx | ★★★ |
| [04-modular-monolith-in-depth.md](../04-architecture-and-patterns/service-topologies/04-modular-monolith-in-depth.md) | module.go паттерн, cross-module коммуникация, enforcement, эволюция | ★★ |
| [03-go-project-layout.md](../04-architecture-and-patterns/service-topologies/03-go-project-layout.md) | структура папок для layered/hexagonal/modular/monorepo | ★★ |

#### 05 System Design: External Request Flows

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-basic-public-api-request-flow.md](../05-system-design/external-request-flows/01-basic-public-api-request-flow.md) | базовый flow: LB → API → DB | ★★★ |
| [02-read-heavy-request-with-cdn-and-cache.md](../05-system-design/external-request-flows/02-read-heavy-request-with-cdn-and-cache.md) | CDN, cache-aside, cache warming | ★★★ |
| [03-write-request-with-queue-and-async-processing.md](../05-system-design/external-request-flows/03-write-request-with-queue-and-async-processing.md) | async через очередь, at-least-once, idempotency | ★★★ |
| [04-authenticated-request-through-api-gateway.md](../05-system-design/external-request-flows/04-authenticated-request-through-api-gateway.md) | API gateway, JWT, rate limiting на edge | ★★ |
| [05-file-upload-and-background-processing-flow.md](../05-system-design/external-request-flows/05-file-upload-and-background-processing-flow.md) | presigned URL, S3, worker queue | ★★ |
| [06-where-latency-and-failures-appear.md](../05-system-design/external-request-flows/06-where-latency-and-failures-appear.md) | где в пайплайне теряется время и появляются ошибки | ★★★ |

#### 05 System Design: Edge And Proxy Patterns

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-edge-roles-and-terms.md](../05-system-design/external-request-flows/edge-and-proxy-patterns/01-edge-roles-and-terms.md) | reverse proxy, LB, API gateway, CDN — чем отличаются | ★★ |
| [02-edge-tools-comparison-table.md](../05-system-design/external-request-flows/edge-and-proxy-patterns/02-edge-tools-comparison-table.md) | nginx vs Envoy vs Traefik vs Caddy | ★★ |
| [03-where-nginx-can-stand.md](../05-system-design/external-request-flows/edge-and-proxy-patterns/03-where-nginx-can-stand.md) | позиции nginx в разных топологиях | ★ |
| [04-typical-edge-topologies.md](../05-system-design/external-request-flows/edge-and-proxy-patterns/04-typical-edge-topologies.md) | типовые edge-топологии | ★ |

#### 05 System Design: Feature Flags And Experimentation

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-experimentation-and-rollout-types.md](../05-system-design/experimentation-and-feature-rollouts/01-experimentation-and-rollout-types.md) | canary, blue-green, dark launch, A/B | ★★ |
| [02-feature-flags-in-practice.md](../05-system-design/experimentation-and-feature-rollouts/02-feature-flags-in-practice.md) | targeting, percentage rollout, fallback, lifecycle | ★★★ |
| [02a-feature-flags-golang-client.md](../05-system-design/experimentation-and-feature-rollouts/02a-feature-flags-golang-client.md) | Go реализация: atomic.Value, FNV bucketing, graceful shutdown | ★★★ |
| [03-ab-test-design-and-assignment.md](../05-system-design/experimentation-and-feature-rollouts/03-ab-test-design-and-assignment.md) | assignment service, stable hashing, anti-patterns | ★★ |
| [04-ui-backend-implementation.md](../05-system-design/experimentation-and-feature-rollouts/04-ui-backend-implementation.md) | SSR, UI flags, bootstrap endpoint | ★ |
| [05-metrics-analysis-and-pitfalls.md](../05-system-design/experimentation-and-feature-rollouts/05-metrics-analysis-and-pitfalls.md) | statistical significance, p-value pitfalls, SRM | ★ |

---

### Фаза 3 — Базы данных и хранилища

**Цель:** объяснить ACID, выбрать индекс, прочитать EXPLAIN, объяснить Redis eviction и Kafka partitions.

#### 06 Database Fundamentals

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-acid.md](../06-databases/database-fundamentals/01-acid.md) | ACID, atomicity, isolation levels, 2PC | ★★★ |
| [02-cap-and-base.md](../06-databases/database-fundamentals/02-cap-and-base.md) | CAP теорема, eventual consistency, trade-offs | ★★★ |
| [03-oltp-vs-olap.md](../06-databases/database-fundamentals/03-oltp-vs-olap.md) | OLTP vs OLAP, columnar storage | ★★ |
| [04-interview-cases.md](../06-databases/database-fundamentals/04-interview-cases.md) | практические кейсы на основе фундаментальных концептов | ★★★ |

#### 06 Relational Databases And SQL

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [00-sql-basics-and-syntax.md](../06-databases/database-systems-catalog/postgresql/00-sql-basics-and-syntax.md) | нормализация, joins, CTE, window functions | ★★ |
| [04-transactions-and-locking.md](../06-databases/database-systems-catalog/postgresql/04-transactions-and-locking.md) | isolation levels, MVCC, locks, deadlocks | ★★★ |
| [02-indexes.md](../06-databases/database-systems-catalog/postgresql/02-indexes.md) | B-tree, partial, composite, covering, EXPLAIN ANALYZE | ★★★ |
| [13-pagination.md](../06-databases/database-systems-catalog/postgresql/13-pagination.md) | keyset vs offset pagination, cursor-based | ★★ |
| [09-connection-pooling.md](../06-databases/database-systems-catalog/postgresql/09-connection-pooling.md) | pgxpool, pool exhaustion, production проблемы | ★★★ |
| [14-outbox-and-idempotency.md](../06-databases/database-systems-catalog/postgresql/14-outbox-and-idempotency.md) | outbox pattern, exactly-once, payment flow | ★★★ |

#### 06 Database Systems Catalog

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-comparison-table.md](../06-databases/database-systems-catalog/01-comparison-table.md) | сравнение всех СУБД по use case | ★★★ |
| [postgresql/README.md](../06-databases/database-systems-catalog/postgresql/README.md) | PostgreSQL internals, MVCC, WAL, partitioning | ★★★ |
| [08-redis.md](../06-databases/database-systems-catalog/08-redis.md) | структуры данных, persistence, eviction, cluster | ★★★ |
| [08a-redis-real-scenarios.md](../06-databases/database-systems-catalog/08a-redis-real-scenarios.md) | cache, session, pub/sub, distributed lock | ★★★ |
| [08b-redis-rate-limiters.md](../06-databases/database-systems-catalog/08b-redis-rate-limiters.md) | token bucket, sliding window на Redis + Lua | ★★ |
| [04-mongodb.md](../06-databases/database-systems-catalog/04-mongodb.md) | MongoDB, document model, aggregation pipeline | ★★ |
| [04a-mongodb-real-scenarios.md](../06-databases/database-systems-catalog/04a-mongodb-real-scenarios.md) | реальные паттерны MongoDB | ★★ |
| [05-cassandra.md](../06-databases/database-systems-catalog/05-cassandra.md) | Cassandra, wide-column, consistent hashing | ★★ |
| [06-clickhouse.md](../06-databases/database-systems-catalog/06-clickhouse.md) | ClickHouse, columnar, MergeTree | ★★ |
| [09-elasticsearch-and-opensearch.md](../06-databases/database-systems-catalog/09-elasticsearch-and-opensearch.md) | inverted index, full-text search, relevance | ★★ |
| [03-mysql.md](../06-databases/database-systems-catalog/03-mysql.md) | MySQL, InnoDB, отличия от PostgreSQL | ★ |
| [07-couchbase.md](../06-databases/database-systems-catalog/07-couchbase.md) | Couchbase | ★ |
| [10-dynamodb.md](../06-databases/database-systems-catalog/10-dynamodb.md) | DynamoDB, GSI/LSI, capacity modes | ★ |

#### 06 Go Database Libraries

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-comparison-table.md](../06-databases/go-database-libraries/01-comparison-table.md) | database/sql vs pgx vs sqlx vs sqlc vs ORM | ★★★ |
| [02-standard-library-database-sql.md](../06-databases/go-database-libraries/02-standard-library-database-sql.md) | database/sql, типичные ошибки и подводные камни | ★★★ |
| [03-pgx-and-pgxpool.md](../06-databases/go-database-libraries/03-pgx-and-pgxpool.md) | pgx, pgxpool, named params, batch queries | ★★★ |
| [04-sqlx-and-sqlc.md](../06-databases/go-database-libraries/04-sqlx-and-sqlc.md) | sqlx, sqlc — type-safe queries без ORM | ★★ |
| [05-orm-and-query-builder-options.md](../06-databases/go-database-libraries/05-orm-and-query-builder-options.md) | GORM, ent, sqlboiler — trade-offs | ★★ |
| [06-choosing-a-library-for-a-go-service.md](../06-databases/go-database-libraries/06-choosing-a-library-for-a-go-service.md) | decision framework: что выбрать и почему | ★★★ |
| [migrations-in-go.md](../06-databases/migrations/migrations-in-go.md) | goose vs golang-migrate vs Atlas vs dbmate | ★★ |

#### 07 Message Brokers (конспекты в разработке)

Темы: Kafka vs RabbitMQ vs NATS, partitions/consumer groups, at-least-once, DLQ, outbox/inbox pattern.
→ [README с темами и вопросами](../07-message-brokers-and-streaming/README.md)

---

### Фаза 4 — Networking и API

**Цель:** объяснить, что происходит от ввода URL до ответа сервера — DNS, TCP, TLS, HTTP/2, CDN, кэш.

#### 08 Request Lifecycle

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [02-dns-resolution-and-getting-ip.md](../08-networking-and-api/request-lifecycle/02-dns-resolution-and-getting-ip.md) | TTL иерархия, CoreDNS, search domains в Kubernetes | ★★★ |
| [03-tcp-tls-and-http-request.md](../08-networking-and-api/request-lifecycle/03-tcp-tls-and-http-request.md) | TLS 1.2 vs 1.3, 0-RTT, HTTP/1.1 vs /2 vs /3, RTT table | ★★★ |
| [04-cdn-load-balancer-reverse-proxy.md](../08-networking-and-api/request-lifecycle/04-cdn-load-balancer-reverse-proxy.md) | L4/L7 LB, active/passive health checks, circuit breaker | ★★★ |
| [05-backend-application-and-data-access.md](../08-networking-and-api/request-lifecycle/05-backend-application-and-data-access.md) | Go middleware chain, context propagation, timeout handling | ★★★ |
| [06-response-return-caching-and-browser-rendering.md](../08-networking-and-api/request-lifecycle/06-response-return-caching-and-browser-rendering.md) | Cache-Control, ETag, stale-while-revalidate, CDN invalidation | ★★★ |
| [07-end-to-end-timeline-and-where-it-breaks.md](../08-networking-and-api/request-lifecycle/07-end-to-end-timeline-and-where-it-breaks.md) | реальные числа latency, `curl -w` breakdown | ★★★ |
| [01-browser-input-and-navigation-start.md](../08-networking-and-api/request-lifecycle/01-browser-input-and-navigation-start.md) | HSTS, HTTP cache flow, Navigation Timing API | ★★ |

#### 08 Rate Limiting

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [05-integration-patterns/03-rate-limiting.md](../08-networking-and-api/protocols/05-integration-patterns/03-rate-limiting.md) | token bucket, leaky bucket, sliding window, fixed window | ★★★ |

---

### Фаза 5 — Production: observability, DevOps, тестирование, безопасность

**Цель:** уметь поставить сервис в prod, расследовать инцидент, объяснить linux-основы контейнеров.

#### 10 Linux Internals

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [05-namespaces-and-cgroups.md](../10-devops-and-observability/linux/05-namespaces-and-cgroups.md) | 8 типов namespaces, cgroups v2, как Docker собирает контейнер | ★★★ |
| [04-signals-and-processes.md](../10-devops-and-observability/linux/04-signals-and-processes.md) | SIGTERM/SIGKILL, PID 1 в контейнере, zombie/orphan | ★★★ |
| [02-file-descriptors-and-io.md](../10-devops-and-observability/linux/02-file-descriptors-and-io.md) | fd tables, epoll O(ready), Go netpoller, 100k connections | ★★★ |
| [01-virtual-memory.md](../10-devops-and-observability/linux/01-virtual-memory.md) | page fault, mmap, OOM killer, GOMEMLIMIT | ★★★ |
| [03-tcp-sockets.md](../10-devops-and-observability/linux/03-tcp-sockets.md) | TCP states, TIME_WAIT, CLOSE_WAIT, SO_REUSEPORT, Nagle | ★★★ |

#### 10 Docker

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-container-vs-virtual-machine.md](../10-devops-and-observability/docker/01-container-vs-virtual-machine.md) | короткое сравнение контейнера и VM, выбор подхода | ★★★ |
| [02-containers.md](../10-devops-and-observability/docker/02-containers.md) | процессы, namespaces, cgroups, слои образа, OCI, безопасность | ★★★ |
| [03-virtual-machines.md](../10-devops-and-observability/docker/03-virtual-machines.md) | гипервизор, гостевая ОС, виртуальные ресурсы, эмуляция | ★★★ |
| [04-docker-for-go-services.md](../10-devops-and-observability/docker/04-docker-for-go-services.md) | multi-stage build, distroless/scratch, GOMEMLIMIT, automaxprocs | ★★★ |
| [02-dockerfile-anatomy.md](../10-devops-and-observability/dockerfiles-for-go/02-dockerfile-anatomy.md) | слои, кэш layers, порядок инструкций | ★★ |
| [03-dockerfiles-for-go-projects.md](../10-devops-and-observability/dockerfiles-for-go/03-dockerfiles-for-go-projects.md) | паттерны prod/dev Dockerfile | ★★ |

> Docker Compose — справочный раздел для локального окружения:
> [02-docker-compose-for-go-projects.md](../10-devops-and-observability/docker-compose/02-docker-compose-for-go-projects.md) · [справочник полей](../10-devops-and-observability/docker-compose/compose-file-reference/README.md)

#### 10 Kubernetes

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-kubernetes-architecture.md](../10-devops-and-observability/kubernetes/01-kubernetes-architecture.md) | компоненты Kubernetes, их роли и взаимодействие | ★★★ |
| [02-kubernetes-cluster-and-ha.md](../10-devops-and-observability/kubernetes/02-kubernetes-cluster-and-ha.md) | границы кластера, топология control plane, кворум etcd | ★★★ |
| [07-probes-and-graceful-shutdown.md](../10-devops-and-observability/kubernetes/07-probes-and-graceful-shutdown.md) | liveness, readiness, startup, SIGTERM grace period | ★★★ |
| [03-core-objects-and-deployment-flow.md](../10-devops-and-observability/kubernetes/03-core-objects-and-deployment-flow.md) | Deployment, ReplicaSet, StatefulSet, Service и связи между объектами | ★★★ |
| [11-persistent-storage-pv-pvc-and-storageclass.md](../10-devops-and-observability/kubernetes/11-persistent-storage-pv-pvc-and-storageclass.md) | PV, PVC, StorageClass, CSI, режимы доступа и жизненный цикл данных | ★★★ |
| [08-node-failure-and-disruptions.md](../10-devops-and-observability/kubernetes/08-node-failure-and-disruptions.md) | отказ узла, drain, выселение, PodDisruptionBudget | ★★ |
| [09-update-strategies.md](../10-devops-and-observability/kubernetes/09-update-strategies.md) | RollingUpdate, Recreate, OnDelete, canary и blue-green | ★★ |
| [10-config-and-secret-delivery.md](../10-devops-and-observability/kubernetes/10-config-and-secret-delivery.md) | ConfigMap, Secret, доставка значений в процесс | ★★ |
| [04-pod-vs-container.md](../10-devops-and-observability/kubernetes/04-pod-vs-container.md) | sidecar, init container, shared network namespace | ★★ |
| [13-practical-manifest-review.md](../10-devops-and-observability/kubernetes/13-practical-manifest-review.md) | сквозной разбор Ingress, Service, Deployment, StatefulSet, PVC и конфигурации | ★★ |

#### 10 Metrics: Prometheus

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-metric-types-and-design.md](../10-devops-and-observability/prometheus-and-metrics/01-metric-types-and-design.md) | counter, gauge, histogram, summary — когда что | ★★★ |
| [05-promql-cheatsheet.md](../10-devops-and-observability/prometheus-and-metrics/05-promql-cheatsheet.md) | rate(), histogram_quantile(), aggregations | ★★★ |
| [http-request-rate-counters.md](../10-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/http-request-rate-counters.md) | как считать RPS через counter | ★★★ |
| [latency-histograms.md](../10-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/latency-histograms.md) | p50/p95/p99, правильные bucket boundaries | ★★★ |
| [http-error-rate.md](../10-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/http-error-rate.md) | error rate по статус-кодам | ★★★ |
| [gauges-inflight-queue-depth.md](../10-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/gauges-inflight-queue-depth.md) | in-flight requests, queue depth | ★★ |
| [storage-operation-metrics.md](../10-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/storage-operation-metrics.md) | метрики DB и cache операций | ★★ |
| [02-prometheus-metrics-flow.md](../10-devops-and-observability/prometheus-and-metrics/02-prometheus-metrics-flow.md) | scrape flow, pull model, alertmanager | ★★ |
| [how-prometheus-discovers-and-scrapes-multiple-pods.md](../10-devops-and-observability/prometheus-and-metrics/how-prometheus-discovers-and-scrapes-multiple-pods.md) | service discovery в Kubernetes | ★★ |
| [04-prometheus-ui-and-grafana.md](../10-devops-and-observability/prometheus-and-metrics/04-prometheus-ui-and-grafana.md) | dashboards, alerts | ★★ |
| [03-prometheus-relabeling-and-target-labels.md](../10-devops-and-observability/prometheus-and-metrics/03-prometheus-relabeling-and-target-labels.md) | relabeling, label management | ★ |

#### 10 Tracing

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-opentelemetry-and-tracing-flow.md](../10-devops-and-observability/tracing-and-opentelemetry/01-opentelemetry-and-tracing-flow.md) | spans, trace context propagation, sampling | ★★★ |
| [02-opentelemetry-in-go-services.md](../10-devops-and-observability/tracing-and-opentelemetry/02-opentelemetry-in-go-services.md) | instrumentation в Go, SDK setup | ★★★ |
| [04-push-model-traceid-and-spans-example.md](../10-devops-and-observability/tracing-and-opentelemetry/04-push-model-traceid-and-spans-example.md) | TraceID, SpanID, push vs pull model | ★★ |
| [03-tempo-and-trace-investigation.md](../10-devops-and-observability/tracing-and-opentelemetry/03-tempo-and-trace-investigation.md) | Grafana Tempo, расследование по трейсам | ★★ |

#### 10 Logging

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [02-logging-in-go-and-why-wrap-logger.md](../10-devops-and-observability/logging-and-log-shipping/02-logging-in-go-and-why-wrap-logger.md) | slog, structured logging, зачем обёртка над логгером | ★★★ |
| [01-logs-pipeline-overview.md](../10-devops-and-observability/logging-and-log-shipping/01-logs-pipeline-overview.md) | как логи попадают из контейнера в хранилище | ★★ |
| [03-log-platforms-comparison-table.md](../10-devops-and-observability/logging-and-log-shipping/03-log-platforms-comparison-table.md) | ELK vs Loki vs Cloud logging — trade-offs | ★★ |
| [07-loki-log-pipeline.md](../10-devops-and-observability/logging-and-log-shipping/07-loki-log-pipeline.md) | Loki + Promtail + Grafana | ★★ |
| [04-elasticsearch-log-pipeline.md](../10-devops-and-observability/logging-and-log-shipping/04-elasticsearch-log-pipeline.md) | ELK/EFK stack | ★★ |
| [08-grafana-overview-and-functionality.md](../10-devops-and-observability/logging-and-log-shipping/08-grafana-overview-and-functionality.md) | Grafana: dashboards, alerting, explore | ★★ |
| [05-kibana-and-elasticsearch.md](../10-devops-and-observability/logging-and-log-shipping/05-kibana-and-elasticsearch.md) | поиск в Kibana, KQL | ★★ |
| [10-promtail-vs-grafana-alloy-vs-fluent-bit.md](../10-devops-and-observability/logging-and-log-shipping/10-promtail-vs-grafana-alloy-vs-fluent-bit.md) | сравнение агентов доставки логов | ★ |
| [09-grafana-vs-kibana-and-similar-tools.md](../10-devops-and-observability/logging-and-log-shipping/09-grafana-vs-kibana-and-similar-tools.md) | сравнение инструментов визуализации | ★ |
| [06-kibana-and-elasticsearch-cheatsheet.md](../10-devops-and-observability/logging-and-log-shipping/06-kibana-and-elasticsearch-cheatsheet.md) | KQL cheatsheet | ★ |
| [11-cloud-log-delivery-aws-and-google-cloud.md](../10-devops-and-observability/logging-and-log-shipping/11-cloud-log-delivery-aws-and-google-cloud.md) | CloudWatch, Google Cloud Logging | ★ |

#### 10 Incident Response And Investigation

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-incident-response-workflow.md](../10-devops-and-observability/incident-response-and-investigation/01-incident-response-workflow.md) | impact, severity, роли, mitigation, локализация и проверка восстановления | ★★★ |
| [02-symptom-driven-troubleshooting.md](../10-devops-and-observability/incident-response-and-investigation/02-symptom-driven-troubleshooting.md) | symptom → signal → tool: latency, CPU, memory, backlog, pools и сеть | ★★★ |
| [03-cross-layer-incident-cases.md](../10-devops-and-observability/incident-response-and-investigation/03-cross-layer-incident-cases.md) | сквозные кейсы приложения, БД, очередей, контейнеров и cache | ★★★ |

#### 09 Testing And Quality

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-testing-strategy.md](../09-testing-and-quality/01-testing-strategy.md) | пирамида тестов, trade-offs между уровнями | ★★★ |
| [02-unit-tests-in-go.md](../09-testing-and-quality/02-unit-tests-in-go.md) | table-driven, subtests, parallel, testable design | ★★★ |
| [03-test-doubles-and-test-design.md](../09-testing-and-quality/03-test-doubles-and-test-design.md) | mock vs fake vs stub, когда что использовать | ★★★ |
| [10-integration-and-e2e.md](../09-testing-and-quality/10-integration-and-e2e.md) | testcontainers, contract tests, e2e | ★★ |
| [11-race-fuzz-and-benchmarks.md](../09-testing-and-quality/11-race-fuzz-and-benchmarks.md) | race detector, fuzzing, benchmarks — когда нужны | ★★ |
| [04-testing-libraries-in-go.md](../09-testing-and-quality/04-testing-libraries-in-go.md) | testify, gomock, go-cmp — сравнение | ★★ |

#### 11 Security

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-tls-termination-re-encryption-and-mtls.md](../11-security/service-to-service-tls/01-tls-termination-re-encryption-and-mtls.md) | TLS termination, re-encryption, mTLS между сервисами | ★★★ |
| [01-secrets-delivery-options.md](../11-security/secrets-management/01-secrets-delivery-options.md) | Vault, k8s secrets, env, file mounts — trade-offs | ★★★ |
| [01-cors-basics-and-where-to-configure-it.md](../11-security/cors-and-browser-api-security/01-cors-basics-and-where-to-configure-it.md) | CORS, preflight, где настраивать (LB vs middleware) | ★★★ |
| [01-ddos-protection.md](../11-security/perimeter-and-traffic-protection/01-ddos-protection.md) | DDoS, perimeter protection, WAF | ★★ |
| [04-kubernetes-secrets-and-external-managers.md](../11-security/secrets-management/04-kubernetes-secrets-and-external-managers.md) | k8s Secrets, External Secrets Operator, Vault Agent | ★★ |
| [02-local-development-secrets.md](../11-security/secrets-management/02-local-development-secrets.md) | .env, direnv, как не утечь в git | ★★ |
| [02-cors-middleware-example.md](../11-security/cors-and-browser-api-security/02-cors-middleware-example.md) | реализация CORS middleware на Go | ★★ |
| [03-docker-compose-and-container-secrets.md](../11-security/secrets-management/03-docker-compose-and-container-secrets.md) | secrets в compose и контейнерах | ★ |

---

### Финал — Алгоритмы и подготовка к интервью

**Цель:** отработать алгоритмические задачи, собрать рассказ о себе, сделать design drills.

#### 16 Algorithms And Data Structures

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-time-and-space-complexity.md](../16-algorithms-and-data-structures/01-time-and-space-complexity.md) | O-нотация, таблица классов, диаграммы роста, амортизированная сложность | ★★★ |
| [02-patterns-overview.md](../16-algorithms-and-data-structures/02-patterns-overview.md) | таблица распознавания паттернов, фреймворк для интервью | ★★★ |
| [03-two-pointers-and-sliding-window.md](../16-algorithms-and-data-structures/03-two-pointers-and-sliding-window.md) | opposite ends, fast/slow, variable window с 9 задачами | ★★★ |
| [04-binary-search.md](../16-algorithms-and-data-structures/04-binary-search.md) | classic, lower/upper bound, rotated array, binary search on answer | ★★★ |
| [05-trees-and-graphs.md](../16-algorithms-and-data-structures/05-trees-and-graphs.md) | обходы дерева, BFS/DFS, топосортировка, Union-Find | ★★★ |
| [06-dynamic-programming.md](../16-algorithms-and-data-structures/06-dynamic-programming.md) | memoization vs tabulation, 1D/2D DP, knapsack | ★★ |
| [07-sorting-and-heap.md](../16-algorithms-and-data-structures/07-sorting-and-heap.md) | merge/quick sort, container/heap, top-K задачи | ★★ |
| [08-backtracking-and-linked-list.md](../16-algorithms-and-data-structures/08-backtracking-and-linked-list.md) | backtracking шаблон, permutations, операции со списками | ★★ |

#### 12 Interview Practice

→ [README раздела](../12-interview-practice/README.md) — рекомендации по behavioral вопросам, storytelling, design drills.

---

## Если мало времени — приоритеты

Если до собеседования осталось 1–2 недели, фокус на `★★★` в таком порядке:

1. **Go runtime** — scheduler, GC, interfaces/nil, memory model `→ 01-go-core`
2. **Concurrency** — goroutine leak, channel vs mutex, worker pool `→ 09`
3. **Databases** — transactions/isolation, indexes/EXPLAIN, connection pooling, outbox `→ 06`
4. **Request lifecycle** — TLS 1.3, HTTP/2, DNS, CDN, Go middleware `→ 08`
5. **Kubernetes** — probes, graceful shutdown, Deployment rollout `→ 11/kubernetes`
6. **Observability** — RED metrics, histogram_quantile, structured logs, tracing `→ 11/prometheus + tracing + logging`
7. **Linux** — namespaces/cgroups, signals/PID1, epoll, OOM killer `→ 11/linux`
8. **Architecture** — monolith vs microservices, outbox, idempotency, DDD, SOLID `→ 04`
9. **Algorithms** — two pointers, binary search, BFS/DFS, DP basics `→ 17`

---

## Внешние источники

- [Go Documentation](https://go.dev/doc)
- [Go Language Specification](https://go.dev/ref/spec)
- [The Go Memory Model](https://go.dev/ref/mem)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
- [Google SRE Resources](https://sre.google/resources/)
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/current/index.htm)
- [Redis Docs](https://redis.io/docs/latest/)
- [Apache Kafka Documentation](https://kafka.apache.org/documentation/)
- [RabbitMQ Documentation](https://www.rabbitmq.com/docs)
- [NATS Docs](https://docs.nats.io/)
- [gRPC Documentation](https://grpc.io/docs/)
- [OpenTelemetry Docs](https://opentelemetry.io/docs/)
- [Kubernetes Concepts](https://kubernetes.io/docs/concepts/index.html)
- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/)

---

## Сквозные вопросы

В любом разделе стоит уметь ответить на:
- какие trade-offs у этого решения;
- что сломается под ростом нагрузки;
- где здесь bottleneck по latency, throughput и operability;
- как это мониторить и дебажить в production;
- как обеспечить backward compatibility;
- как протестировать не только happy path, но и деградацию;
- как решение поменяется при росте команды или требований.
