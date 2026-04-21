# Roadmap

Репозиторий большой — этот файл помогает понять, что здесь есть и в каком порядке читать.

Приоритеты материалов: `★★★` — спрашивают почти везде, `★★` — важно, `★` — полезно.

---

## Карта разделов

| Раздел | Что внутри | Статус |
|--------|------------|--------|
| [01 Go Core](../01-go-core/README.md) | scheduler, GC, interfaces/nil, memory model, escape analysis | ✅ 8 файлов |
| [02 Go Stdlib](../02-go-stdlib-and-tools/README.md) | net/http, context, sync, encoding/json, pprof | темы + ссылки |
| [03 Go Libraries](../03-go-libraries-and-ecosystem/README.md) | chi, pgx, zap, testify, wire/fx — сравнения | темы + ссылки |
| [04 Architecture](../04-architecture-and-patterns/README.md) | Go patterns, service topologies, idempotency, outbox | ✅ 4 файла |
| [05 System Design](../05-system-design/README.md) | request flows, feature flags, A/B tests, edge proxy | ✅ 16 файлов |
| [06 Databases](../06-databases/README.md) | SQL/NoSQL, indexes, transactions, Redis, Go DB libraries | ✅ 29 файлов |
| [07 Message Brokers](../07-message-brokers-and-streaming/README.md) | Kafka, RabbitMQ, NATS, delivery semantics | темы + ссылки |
| [08 Networking & API](../08-networking-and-api/README.md) | HTTP/TLS, request lifecycle, DNS, CDN, rate limiting | ✅ 9 файлов |
| [09 Concurrency](../09-concurrency-and-performance/README.md) | goroutines, channels, mutex, pprof, benchmarks | темы + ссылки |
| [10 Testing](../10-testing-and-quality/README.md) | unit/integration/e2e, test doubles, race/fuzz, linters | ✅ 7 файлов |
| [11 DevOps & Observability](../11-devops-and-observability/README.md) | Linux, Docker, Kubernetes, metrics, traces, logs, profiling | ✅ 40+ файлов |
| [12 Security](../12-security/README.md) | secrets, TLS/mTLS, CORS, DDoS protection | ✅ 8 файлов |
| [13 Interview Practice](../13-interview-practice/README.md) | алгоритмы, behavioral кейсы, system design drills | ✅ 2 файла |
| [16 Go Version Differences](../16-go-version-differences/README.md) | Go 1.24, 1.25, 1.26 — что изменилось | ✅ 3 файла |

> **Разделы "темы + ссылки"** — содержат только README с темами и внешними ссылками; конспекты для них еще не написаны.

---

## Маршрут прохождения

### Фаза 1 — Go runtime и конкурентность

**Цель:** объяснить, как Go работает под капотом — планировщик, GC, interfaces/nil, memory model.

#### 01 Go Core

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [scheduler-and-preemption.md](../01-go-core/scheduler-and-preemption.md) | GMP-модель, work stealing, preemption points | ★★★ |
| [garbage-collector.md](../01-go-core/garbage-collector.md) | tri-color mark-and-sweep, GOGC, GC-паузы, GOMEMLIMIT | ★★★ |
| [interfaces-method-sets-and-nil.md](../01-go-core/interfaces-method-sets-and-nil.md) | iface/eface, nil interface pitfall, method sets | ★★★ |
| [memory-model.md](../01-go-core/memory-model.md) | happens-before, visibility, sync primitives | ★★★ |
| [escape-analysis.md](../01-go-core/escape-analysis.md) | stack vs heap, `go build -gcflags=-m` | ★★ |
| [value-vs-pointer-semantics.md](../01-go-core/value-vs-pointer-semantics.md) | когда копировать, когда брать указатель | ★★ |
| [primitive-types-and-zero-values.md](../01-go-core/primitive-types-and-zero-values.md) | zero values, string internals | ★ |
| [numeric-types-integer-sizes-and-overflow.md](../01-go-core/numeric-types-integer-sizes-and-overflow.md) | int sizes, overflow, конверсии | ★ |

#### 09 Concurrency (конспекты в разработке)

Темы: goroutine lifecycle, channel vs mutex, worker pool, race detector, allocation hotspots, pprof.
→ [README с темами и вопросами](../09-concurrency-and-performance/README.md)

#### 16 Go Version Differences

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [go1.24.md](../16-go-version-differences/go1.24.md) | изменения в Go 1.24 | ★★ |
| [go1.25.md](../16-go-version-differences/go1.25.md) | изменения в Go 1.25 | ★★ |
| [go1.26.md](../16-go-version-differences/go1.26.md) | изменения в Go 1.26 | ★ |

---

### Фаза 2 — Архитектура, паттерны и system design

**Цель:** обосновать выбор топологии сервиса, проектировать с учетом отказов, объяснить outbox/saga/idempotency и feature rollout.

#### 04 Architecture And Patterns

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-go-code-patterns.md](../04-architecture-and-patterns/patterns/01-go-code-patterns.md) | functional options, small interfaces, middleware, decorator | ★★★ |
| [02-architecture-patterns.md](../04-architecture-and-patterns/patterns/02-architecture-patterns.md) | hexagonal, DDD lite, layered, clean arch — trade-offs | ★★★ |
| [01-monolith-vs-modular-monolith-vs-microservices.md](../04-architecture-and-patterns/service-topologies/01-monolith-vs-modular-monolith-vs-microservices.md) | когда что выбирать, стоимость распределенности | ★★★ |
| [02-typical-problems-and-how-to-mitigate-them.md](../04-architecture-and-patterns/service-topologies/02-typical-problems-and-how-to-mitigate-them.md) | outbox, saga, idempotency, retry storms, distributed tx | ★★★ |

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
| [01-relational-model-and-sql-basics.md](../06-databases/relational-databases-and-sql/01-relational-model-and-sql-basics.md) | нормализация, joins, CTE, window functions | ★★ |
| [02-transactions-isolation-and-locks.md](../06-databases/relational-databases-and-sql/02-transactions-isolation-and-locks.md) | isolation levels, MVCC, locks, deadlocks | ★★★ |
| [03-indexes-and-query-plans.md](../06-databases/relational-databases-and-sql/03-indexes-and-query-plans.md) | B-tree, partial, composite, covering, EXPLAIN ANALYZE | ★★★ |
| [04-pagination-and-query-patterns.md](../06-databases/relational-databases-and-sql/04-pagination-and-query-patterns.md) | keyset vs offset pagination, cursor-based | ★★ |
| [05-connection-pooling-and-production-issues.md](../06-databases/relational-databases-and-sql/05-connection-pooling-and-production-issues.md) | pgxpool, pool exhaustion, production проблемы | ★★★ |
| [06-outbox-idempotency-and-payment-flow.md](../06-databases/relational-databases-and-sql/06-outbox-idempotency-and-payment-flow.md) | outbox pattern, exactly-once, payment flow | ★★★ |

#### 06 Database Systems Catalog

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [01-comparison-table.md](../06-databases/database-systems-catalog/01-comparison-table.md) | сравнение всех СУБД по use case | ★★★ |
| [02-postgresql.md](../06-databases/database-systems-catalog/02-postgresql.md) | PostgreSQL internals, MVCC, WAL, partitioning | ★★★ |
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
| [migrations-in-go.md](../06-databases/migrations-in-go.md) | goose vs golang-migrate vs Atlas vs dbmate | ★★ |

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
| [rate-limiting.md](../08-networking-and-api/rate-limiting.md) | token bucket, leaky bucket, sliding window, fixed window | ★★★ |

---

### Фаза 5 — Production: observability, DevOps, тестирование, безопасность

**Цель:** уметь поставить сервис в prod, расследовать инцидент, объяснить linux-основы контейнеров.

#### 11 Linux Internals

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [namespaces-and-cgroups.md](../11-devops-and-observability/linux/namespaces-and-cgroups.md) | 8 типов namespaces, cgroups v2, как Docker собирает контейнер | ★★★ |
| [signals-and-processes.md](../11-devops-and-observability/linux/signals-and-processes.md) | SIGTERM/SIGKILL, PID 1 в контейнере, zombie/orphan | ★★★ |
| [file-descriptors-and-io.md](../11-devops-and-observability/linux/file-descriptors-and-io.md) | fd tables, epoll O(ready), Go netpoller, 100k connections | ★★★ |
| [virtual-memory.md](../11-devops-and-observability/linux/virtual-memory.md) | page fault, mmap, OOM killer, GOMEMLIMIT | ★★★ |
| [tcp-sockets.md](../11-devops-and-observability/linux/tcp-sockets.md) | TCP states, TIME_WAIT, CLOSE_WAIT, SO_REUSEPORT, Nagle | ★★★ |

#### 11 Docker

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [container-vs-virtual-machine.md](../11-devops-and-observability/docker/container-vs-virtual-machine.md) | container = namespaces + cgroups + overlay FS | ★★★ |
| [docker-for-go-services.md](../11-devops-and-observability/docker/docker-for-go-services.md) | multi-stage build, distroless/scratch, GOMEMLIMIT, automaxprocs | ★★★ |
| [dockerfile-anatomy.md](../11-devops-and-observability/dockerfiles-for-go/dockerfile-anatomy.md) | слои, кэш layers, порядок инструкций | ★★ |
| [dockerfiles-for-go-projects.md](../11-devops-and-observability/dockerfiles-for-go/dockerfiles-for-go-projects.md) | паттерны prod/dev Dockerfile | ★★ |

> Docker Compose — справочный раздел для локального окружения:
> [docker-compose-for-go-projects.md](../11-devops-and-observability/docker-compose/docker-compose-for-go-projects.md) · [справочник полей](../11-devops-and-observability/docker-compose/compose-file-reference/README.md)

#### 11 Kubernetes

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [kubernetes-basics-for-backend.md](../11-devops-and-observability/kubernetes/kubernetes-basics-for-backend.md) | Pod, Deployment, Service, ConfigMap, основы для backend | ★★★ |
| [probes-and-graceful-shutdown.md](../11-devops-and-observability/kubernetes/probes-and-graceful-shutdown.md) | liveness, readiness, startup, SIGTERM grace period | ★★★ |
| [core-objects-and-deployment-flow.md](../11-devops-and-observability/kubernetes/core-objects-and-deployment-flow.md) | ReplicaSet, Deployment rollout, revision history | ★★★ |
| [node-failure-rollout-and-config-delivery.md](../11-devops-and-observability/kubernetes/node-failure-rollout-and-config-delivery.md) | rollout strategy, node failure, ConfigMap/Secret delivery | ★★ |
| [pod-vs-container.md](../11-devops-and-observability/kubernetes/pod-vs-container.md) | sidecar, init container, shared network namespace | ★★ |

#### 11 Metrics: Prometheus

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [metric-types-and-design.md](../11-devops-and-observability/prometheus-and-metrics/metric-types-and-design.md) | counter, gauge, histogram, summary — когда что | ★★★ |
| [promql-cheatsheet.md](../11-devops-and-observability/prometheus-and-metrics/promql-cheatsheet.md) | rate(), histogram_quantile(), aggregations | ★★★ |
| [http-request-rate-counters.md](../11-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/http-request-rate-counters.md) | как считать RPS через counter | ★★★ |
| [latency-histograms.md](../11-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/latency-histograms.md) | p50/p95/p99, правильные bucket boundaries | ★★★ |
| [http-error-rate.md](../11-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/http-error-rate.md) | error rate по статус-кодам | ★★★ |
| [gauges-inflight-queue-depth.md](../11-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/gauges-inflight-queue-depth.md) | in-flight requests, queue depth | ★★ |
| [storage-operation-metrics.md](../11-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/storage-operation-metrics.md) | метрики DB и cache операций | ★★ |
| [prometheus-metrics-flow.md](../11-devops-and-observability/prometheus-and-metrics/prometheus-metrics-flow.md) | scrape flow, pull model, alertmanager | ★★ |
| [how-prometheus-discovers-and-scrapes-multiple-pods.md](../11-devops-and-observability/prometheus-and-metrics/how-prometheus-discovers-and-scrapes-multiple-pods.md) | service discovery в Kubernetes | ★★ |
| [prometheus-ui-and-grafana.md](../11-devops-and-observability/prometheus-and-metrics/prometheus-ui-and-grafana.md) | dashboards, alerts | ★★ |
| [prometheus-relabeling-and-target-labels.md](../11-devops-and-observability/prometheus-and-metrics/prometheus-relabeling-and-target-labels.md) | relabeling, label management | ★ |

#### 11 Tracing

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [opentelemetry-and-tracing-flow.md](../11-devops-and-observability/tracing-and-opentelemetry/opentelemetry-and-tracing-flow.md) | spans, trace context propagation, sampling | ★★★ |
| [opentelemetry-in-go-services.md](../11-devops-and-observability/tracing-and-opentelemetry/opentelemetry-in-go-services.md) | instrumentation в Go, SDK setup | ★★★ |
| [01-push-model-traceid-and-spans-example.md](../11-devops-and-observability/tracing-and-opentelemetry/01-push-model-traceid-and-spans-example.md) | TraceID, SpanID, push vs pull model | ★★ |
| [tempo-and-trace-investigation.md](../11-devops-and-observability/tracing-and-opentelemetry/tempo-and-trace-investigation.md) | Grafana Tempo, расследование по трейсам | ★★ |

#### 11 Logging

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [logging-in-go-and-why-wrap-logger.md](../11-devops-and-observability/logging-and-log-shipping/logging-in-go-and-why-wrap-logger.md) | slog, structured logging, зачем обёртка над логгером | ★★★ |
| [logs-pipeline-overview.md](../11-devops-and-observability/logging-and-log-shipping/logs-pipeline-overview.md) | как логи попадают из контейнера в хранилище | ★★ |
| [log-platforms-comparison-table.md](../11-devops-and-observability/logging-and-log-shipping/log-platforms-comparison-table.md) | ELK vs Loki vs Cloud logging — trade-offs | ★★ |
| [loki-log-pipeline.md](../11-devops-and-observability/logging-and-log-shipping/loki-log-pipeline.md) | Loki + Promtail + Grafana | ★★ |
| [elasticsearch-log-pipeline.md](../11-devops-and-observability/logging-and-log-shipping/elasticsearch-log-pipeline.md) | ELK/EFK stack | ★★ |
| [grafana-overview-and-functionality.md](../11-devops-and-observability/logging-and-log-shipping/grafana-overview-and-functionality.md) | Grafana: dashboards, alerting, explore | ★★ |
| [kibana-and-elasticsearch.md](../11-devops-and-observability/logging-and-log-shipping/kibana-and-elasticsearch.md) | поиск в Kibana, KQL | ★★ |
| [promtail-vs-grafana-alloy-vs-fluent-bit.md](../11-devops-and-observability/logging-and-log-shipping/promtail-vs-grafana-alloy-vs-fluent-bit.md) | сравнение агентов доставки логов | ★ |
| [grafana-vs-kibana-and-similar-tools.md](../11-devops-and-observability/logging-and-log-shipping/grafana-vs-kibana-and-similar-tools.md) | сравнение инструментов визуализации | ★ |
| [kibana-and-elasticsearch-cheatsheet.md](../11-devops-and-observability/logging-and-log-shipping/kibana-and-elasticsearch-cheatsheet.md) | KQL cheatsheet | ★ |
| [cloud-log-delivery-aws-and-google-cloud.md](../11-devops-and-observability/logging-and-log-shipping/cloud-log-delivery-aws-and-google-cloud.md) | CloudWatch, Google Cloud Logging | ★ |

#### 11 Incident Investigation And Profiling

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [how-to-investigate-production-issues.md](../11-devops-and-observability/incident-investigation-and-profiling/how-to-investigate-production-issues.md) | методология расследования: logs → metrics → traces → pprof | ★★★ |
| [go-profiling-tracing-and-performance-debugging.md](../11-devops-and-observability/incident-investigation-and-profiling/go-profiling-tracing-and-performance-debugging.md) | pprof, runtime/trace, GODEBUG | ★★★ |
| [finding-leaks-contention-and-memory-problems.md](../11-devops-and-observability/incident-investigation-and-profiling/finding-leaks-contention-and-memory-problems.md) | goroutine leak, lock contention, memory leak — как найти | ★★★ |

#### 10 Testing And Quality

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [automated-testing-strategy.md](../10-testing-and-quality/automated-testing-strategy.md) | пирамида тестов, trade-offs между уровнями | ★★★ |
| [unit-tests-in-go.md](../10-testing-and-quality/unit-tests-in-go.md) | table-driven, subtests, parallel, testable design | ★★★ |
| [test-doubles-and-test-design.md](../10-testing-and-quality/test-doubles-and-test-design.md) | mock vs fake vs stub, когда что использовать | ★★★ |
| [integration-contract-and-e2e-tests.md](../10-testing-and-quality/integration-contract-and-e2e-tests.md) | testcontainers, contract tests, e2e | ★★ |
| [race-fuzz-and-benchmarks.md](../10-testing-and-quality/race-fuzz-and-benchmarks.md) | race detector, fuzzing, benchmarks — когда нужны | ★★ |
| [testing-libraries-in-go.md](../10-testing-and-quality/testing-libraries-in-go.md) | testify, gomock, go-cmp — сравнение | ★★ |
| [testing-cheatsheet.md](../10-testing-and-quality/testing-cheatsheet.md) | быстрая шпаргалка | ★ |

#### 12 Security

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [tls-termination-re-encryption-and-mtls.md](../12-security/service-to-service-tls/tls-termination-re-encryption-and-mtls.md) | TLS termination, re-encryption, mTLS между сервисами | ★★★ |
| [secrets-delivery-options.md](../12-security/secrets-management/secrets-delivery-options.md) | Vault, k8s secrets, env, file mounts — trade-offs | ★★★ |
| [cors-basics-and-where-to-configure-it.md](../12-security/cors-and-browser-api-security/cors-basics-and-where-to-configure-it.md) | CORS, preflight, где настраивать (LB vs middleware) | ★★★ |
| [ddos-protection.md](../12-security/perimeter-and-traffic-protection/ddos-protection.md) | DDoS, perimeter protection, WAF | ★★ |
| [kubernetes-secrets-and-external-managers.md](../12-security/secrets-management/kubernetes-secrets-and-external-managers.md) | k8s Secrets, External Secrets Operator, Vault Agent | ★★ |
| [local-development-secrets.md](../12-security/secrets-management/local-development-secrets.md) | .env, direnv, как не утечь в git | ★★ |
| [cors-middleware-example.md](../12-security/cors-and-browser-api-security/cors-middleware-example.md) | реализация CORS middleware на Go | ★★ |
| [docker-compose-and-container-secrets.md](../12-security/secrets-management/docker-compose-and-container-secrets.md) | secrets в compose и контейнерах | ★ |

---

### Финал — Подготовка к интервью

**Цель:** собрать рассказ о себе, отработать алгоритмические задачи, сделать design drills.

#### 13 Interview Practice

| Файл | Что внутри | Приоритет |
|------|-----------|-----------|
| [time-and-space-complexity.md](../13-interview-practice/algorithms-and-complexity/time-and-space-complexity.md) | big O, примеры, как объяснять | ★★ |
| [common-algorithm-patterns-and-examples-in-go.md](../13-interview-practice/algorithms-and-complexity/common-algorithm-patterns-and-examples-in-go.md) | sliding window, two pointers, binary search на Go | ★★ |

→ [README раздела](../13-interview-practice/README.md) — рекомендации по behavioral вопросам, storytelling, design drills.

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
8. **Architecture** — monolith vs microservices, outbox, idempotency `→ 04`

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
