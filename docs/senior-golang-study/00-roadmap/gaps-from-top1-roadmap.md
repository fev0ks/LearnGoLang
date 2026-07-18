# Гэп-анализ: Топ-1% Backend Roadmap 2026

Сравнение тем из PDF-роадмэпа с текущими материалами.
Документ обновляется по мере закрытия гэпов.

---

## Статусы

- ✅ Есть — тема покрыта
- 🟡 Частично — тема есть, но неполно
- ❌ Нет — тема отсутствует, нужно создать

---

## Сводка прогресса

**Закрыто полностью (✅):** все темы из фаз 01-02 (кроме Git), почти вся фаза 03, большая часть фазы 04.

**Закрыто высокого приоритета:** Redis cache, OWASP Top 10 (SQL injection, XSS, SSRF), SLO/SLI/error budgets, постмортемы.

**Закрыто среднего приоритета:** SSE/realtime, OpenAPI/Swagger, AWS core services, cloud cost and architecture, chaos engineering.

**Сверх плана:** раздел `hardware-and-os/` целиком (7 файлов про CPU, память, atomics, scheduling), JWT/RBAC, начат раздел `17-llm-and-ai-integration/` с RAG fundamentals.

**Осталось:** 2 файла среднего приоритета (Git) + 8 файлов низкого (LLM API, RAG детали, serverless).

---

## Фаза 01 — Фундамент (Недели 1–6)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | Один язык, на глубину (Go) | ✅ | `01-go-core/` — глубоко |
| 2 | Как реально работает интернет (TCP/IP, DNS, HTTP/HTTPS) | ✅ | `08-networking-and-api/request-lifecycle/` |
| 3 | Сырой HTTP-сервер | ✅ | `08-networking-and-api/protocols/02-http-server.md`, `03-go-libraries-and-ecosystem/http-servers/` |
| 4 | Linux и командная строка (SSH, права, cron, grep, curl, jq) | ✅ | `10-devops-and-observability/linux/` (01–06) |
| 5 | SQL и базы данных (PostgreSQL, EXPLAIN, индексы) | ✅ | `06-databases/` — очень глубоко |
| 6 | **Git глубже базы** (branching, rebase, merge conflicts, git bisect) | ❌ | **Создать:** `10-devops-and-observability/git/` — branching, rebase, bisect, advanced |
| 7 | AI-pair: правила работы | — | Не технический топик, вне scope |

---

## Фаза 02 — Core Backend (Недели 7–18)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | Дизайн REST-API (именование, версионирование, пагинация, фильтры, ошибки) | ✅ | `04-architecture-and-patterns/patterns/07-rest-api-design.md`, `03-api-versioning.md` |
| 2 | Один фреймворк до автоматизма (chi/gin/echo) | ✅ | `03-go-libraries-and-ecosystem/http-servers/` |
| 3 | Дизайн БД (нормализация, индексы, транзакции, ACID, миграции, N+1) | ✅ | `06-databases/` |
| 4 | Кэширование — Redis (cache-aside, TTL, инвалидация, сессии, когда НЕ кэшировать) | ✅ | `06-databases/caching/01-redis-as-cache.md` |
| 5 | Type safety (TypeScript/Pydantic/mypy) | — | Go-специфично не применимо; покрыто через систему типов |
| 6 | Аутентификация (OAuth 2.0, bcrypt, JWT refresh, CSRF, session fixation) | ✅ | `11-security/authentication/` (01–07) |
| 7 | Async, очереди, фоновые задачи (горутины, Redis Streams, Kafka, воркеры) | ✅ | `01-go-core/concurrency-and-performance/`, `07-message-brokers-and-streaming/`, `04-architecture-and-patterns/patterns/04-background-workers.md` |
| 8 | Тесты (unit, интеграционные, API, моки внешних сервисов) | ✅ | `09-testing-and-quality/` (01–11) |
| 9 | Реалтайм-протоколы (WebSockets, SSE, long polling — когда что выбирать) | ✅ | WebSocket: `08-networking-and-api/protocols/05-websocket.md`. SSE и long polling: `08-networking-and-api/protocols/12-sse-and-realtime.md` |
| 10 | Docker — контейнеризуй всё (Dockerfile, Compose, multi-stage, слои) | ✅ | `10-devops-and-observability/docker*/`, `dockerfiles-for-go/`, `docker-compose/` |

---

## Фаза 03 — Production-Grade (Недели 19–28)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | CI/CD пайплайны (GitHub Actions/GitLab CI, тесты на PR, secrets, zero-downtime) | ✅ | `10-devops-and-observability/ci-cd/` (GitHub Actions, GitLab CI) |
| 2 | Логи и observability (JSON, correlation IDs, уровни, Grafana+Loki/Datadog, алерты) | ✅ | `10-devops-and-observability/logging-and-log-shipping/`, `tracing-and-opentelemetry/`, `prometheus-and-metrics/` |
| 3 | Облако — одна платформа (AWS: EC2, RDS, S3, SQS, IAM, VPC, security groups, managed vs self-hosted) | ✅ | `10-devops-and-observability/cloud/01-aws-core-services.md` + `02-cloud-cost-and-architecture.md` |
| 4 | Профилировка и нагрузка (flame graphs, slow query log, p95/p99) | ✅ | `01-go-core/profiling/`, `10-devops-and-observability/incident-investigation-and-profiling/` |
| 4a | **Нагрузочное тестирование** (k6/Locust, bottleneck → фикс → перетест) | ❌ | **Создать:** `09-testing-and-quality/12-load-testing.md` — k6 примеры, интерпретация результатов, p95/p99 |
| 5 | Безопасность вглубь — OWASP Top 10 (SQL injection, XSS, SSRF, IDOR + AI-аудит) | ✅ | `11-security/owasp-top10/` (SQL injection, XSS, SSRF). IDOR — в `authentication/07-authorization-and-rbac.md` |
| 6 | **Edge и Serverless** (Cloudflare Workers, AWS Lambda, cold start, лимиты, биллинг) | ❌ | **Создать:** `10-devops-and-observability/serverless/` или `05-system-design/edge-and-serverless/` |
| 7 | API-дизайн на масштаб (idempotency, webhook design, версионирование, OpenAPI spec, backward compatibility) | ✅ | Idempotency: `05-system-design/reliability-patterns/06-idempotency.md`. Webhooks: `08-networking-and-api/protocols/06-webhooks.md`. Версионирование: `04-architecture-and-patterns/patterns/03-api-versioning.md`. OpenAPI: `08-networking-and-api/protocols/13-openapi-and-swagger.md` |

---

## Фаза 04 — Распределёнка и AI (Недели 29–42)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | System design фундамент (CAP, горизонтальное/вертикальное, LB, CDN, "Спроектируй Twitter") | ✅ | `05-system-design/` (CAP, external-request-flows, interview-cases) |
| 2 | БД на масштаб (read replicas, PgBouncer, шардирование, денормализация, партиционирование, zero-downtime миграции) | ✅ | `06-databases/database-systems-catalog/postgresql/` (06-replication, 05-partitioning, 12-sharding, 09-connection-pooling) |
| 3 | **LLM как часть бэкенда** (OpenAI/Anthropic API, streaming, function calling, токеномика, кэш промптов, деградация провайдера) | ❌ | **Создать:** `17-llm-and-ai-integration/` — раздел создан. Нужны файлы: API basics, streaming/function calling, prompt engineering, reliability/fallback |
| 4 | **RAG и vector БД** (pgvector, Qdrant, Weaviate, эмбеддинги, чанкинг, гибридный поиск, стейл-данные, prompt injection через документы) | 🟡 | `17-llm-and-ai-integration/rag/01-rag-fundamentals.md` есть. **Создать:** vector БД, chunking/embeddings, hybrid search, pitfalls |
| 5 | Очереди и event streaming (Kafka: партиции, consumer groups, exactly-once, event sourcing, DLQ) | ✅ | `07-message-brokers-and-streaming/01-kafka.md` |
| 6 | Микросервисы — когда и как (gRPC vs REST, service mesh, distributed tracing, "монолит сначала") | ✅ | `04-architecture-and-patterns/service-topologies/`, `03-go-libraries-and-ecosystem/grpc/`, `10-devops-and-observability/tracing-and-opentelemetry/` |
| 7 | Kubernetes — основы (pods, services, deployments, ingress, ConfigMaps, secrets, HPA) | ✅ | `10-devops-and-observability/kubernetes/` (01–07) |
| 8 | Reliability engineering (SLO, SLI, error budgets, circuit breakers, chaos engineering, graceful degradation, постмортемы) | ✅ | `05-system-design/reliability-patterns/` — все включая `08-slo-sli-error-budgets.md`, `09-postmortem.md`, `10-chaos-engineering.md`, circuit breaker, retries, backoff, idempotency |

---

## Фаза 05 — Привычки топ-1% (Недели 43–52)

Все темы этой фазы — soft skills и практики (читай кодбазы, OSS, пиши блог, специализация, системное мышление). Не технические топики — вне scope материалов.

---

## Что было сделано сверх плана PDF

Это темы которых не было в PDF, но мы их сделали для глубины senior уровня.

### Hardware и OS internals (раздел `10-devops-and-observability/hardware-and-os/`)

7 файлов, ~4400 строк:
- `01-cpu-architecture.md` — pipeline, OoO, branch prediction, SIMD, SMT
- `02-memory-hierarchy.md` — latency numbers, DRAM, SSD vs HDD, locality
- `03-cache-coherence-and-mesi.md` — cache lines, MESI, false sharing с demo в Go
- `04-atomics-and-memory-ordering.md` — store buffers, x86 TSO vs ARM weak, fences, CAS
- `05-virtual-memory-and-paging.md` — VA→PA, MMU/TLB, COW, mmap, swap, VIRT/RSS
- `06-processes-and-threads.md` — fork/exec/clone, kernel/user mode, syscalls
- `07-context-switching-and-scheduling.md` — voluntary/nonvoluntary context switches, CFS/EEVDF, `nice`, cgroups и диагностика планирования

### Углубление аутентификации

- `11-security/authentication/05-authentication-methods-overview.md`
- `11-security/authentication/06-jwt.md`
- `11-security/authentication/07-authorization-and-rbac.md`

### Container security deep-dive

В `10-devops-and-observability/docker/02-containers.md` разобраны векторы атак, примеры уязвимостей `runc`, последствия компрометации и чек-лист защиты.

---

## Итоговый список файлов к созданию

### Средний приоритет (3 файла осталось)

| Файл | Раздел | Что покрывает |
|---|---|---|
| `10-devops-and-observability/git/01-branching-and-workflow.md` | DevOps | Trunk-based, gitflow, branching стратегии, rebase vs merge, git bisect |
| `10-devops-and-observability/git/02-advanced-git.md` | DevOps | Interactive rebase, cherry-pick, reflog, коммит-сообщения |
| `09-testing-and-quality/12-load-testing.md` | Testing | k6 basics, сценарии, интерпретация p95/p99, bottleneck → фикс → перетест |

### Низкий приоритет — LLM и AI (7 файлов)

**LLM API integration (4 файла):**

| Файл | Что покрывает |
|---|---|
| `17-llm-and-ai-integration/api-integration/01-llm-api-basics.md` | OpenAI/Anthropic API, модели, токены, стоимость |
| `17-llm-and-ai-integration/api-integration/02-streaming-and-function-calling.md` | Streaming ответов (SSE), function/tool calling, structured output |
| `17-llm-and-ai-integration/prompt-engineering/01-prompts-for-backend.md` | System prompts, контекстное окно, prompt caching, temperature |
| `17-llm-and-ai-integration/reliability/01-fallback-and-degradation.md` | Деградация при отказе провайдера, timeout, fallback на другую модель, retry |

**RAG детальнее (3 файла):**

| Файл | Что покрывает |
|---|---|
| `17-llm-and-ai-integration/rag/02-embeddings.md` | Модели эмбеддингов, размерности, OpenAI vs open-source |
| `17-llm-and-ai-integration/rag/03-vector-databases.md` | pgvector, Qdrant, Weaviate — сравнение, когда что выбирать |
| `17-llm-and-ai-integration/rag/04-chunking-strategies.md` | Стратегии чанкинга, влияние на качество |
| `17-llm-and-ai-integration/rag/05-hybrid-search.md` | Vector + BM25, reranking |
| `17-llm-and-ai-integration/rag/06-rag-pitfalls.md` | Стейл-данные, prompt injection через документы, галлюцинации, evaluation |

### Низкий приоритет — прочее (1 файл)

| Файл | Раздел | Что покрывает |
|---|---|---|
| `10-devops-and-observability/serverless/01-edge-and-serverless.md` | DevOps | Cloudflare Workers, AWS Lambda, cold start, лимиты, когда VPS избыточен |

---

## Что точно НЕ нужно добавлять

- Type safety (TypeScript/Pydantic/mypy) — это про другие языки, Go покрывает через систему типов
- AI-pair правила работы — не технический топик
- Контрибьюти в OSS, пиши блог, специализация — не технический топик

---

## История обновлений

- **Изначальная версия** — гэп-анализ по PDF, 15 файлов в плане
- **После высокого приоритета** — закрыто 6 файлов: Redis cache, OWASP (SQL/XSS/SSRF), SLO/SLI, postmortems
- **После hardware-and-os** — добавлен сверх-плановый раздел из 7 файлов про CPU/память/OS
- **После RAG fundamentals** — начат раздел `17-llm-and-ai-integration/`
- **После среднего приоритета** — закрыто 5 файлов: SSE/realtime, OpenAPI, chaos engineering, AWS core, cloud cost. Осталось 3 средних (Git × 2, k6) + 7 LLM/RAG + 1 serverless
