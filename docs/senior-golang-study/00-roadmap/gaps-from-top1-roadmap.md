# Гэп-анализ: Топ-1% Backend Roadmap 2026

Сравнение тем из PDF-роадмэпа с текущими материалами.
Документ создан для планирования новых файлов.

---

## Статусы

- ✅ Есть — тема покрыта
- 🟡 Частично — тема есть, но неполно
- ❌ Нет — тема отсутствует, нужно создать

---

## Фаза 01 — Фундамент (Недели 1–6)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | Один язык, на глубину (Go) | ✅ | `01-go-core/` — глубоко |
| 2 | Как реально работает интернет (TCP/IP, DNS, HTTP/HTTPS) | ✅ | `08-networking-and-api/request-lifecycle/` |
| 3 | Сырой HTTP-сервер | ✅ | `08-networking-and-api/protocols/02-http-server.md`, `03-go-libraries-and-ecosystem/http-servers/` |
| 4 | Linux и командная строка (SSH, права, cron, grep, curl, jq) | ✅ | `11-devops-and-observability/linux/` (01–06) |
| 5 | SQL и базы данных (PostgreSQL, EXPLAIN, индексы) | ✅ | `06-databases/` — очень глубоко |
| 6 | **Git глубже базы** (branching, rebase, merge conflicts, git bisect) | ❌ | **Создать:** `02-go-stdlib-and-tools/git/` — отдельный раздел или файлы в `11-devops-and-observability/` |
| 7 | AI-pair: правила работы | — | Не технический топик, вне scope |

---

## Фаза 02 — Core Backend (Недели 7–18)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | Дизайн REST-API (именование, версионирование, пагинация, фильтры, ошибки) | ✅ | `04-architecture-and-patterns/patterns/07-rest-api-design.md`, `03-api-versioning.md` |
| 2 | Один фреймворк до автоматизма (chi/gin/echo) | ✅ | `03-go-libraries-and-ecosystem/http-servers/` |
| 3 | Дизайн БД (нормализация, индексы, транзакции, ACID, миграции, N+1) | ✅ | `06-databases/` |
| 4 | **Кэширование — Redis** (cache-aside, TTL, инвалидация, сессии, когда НЕ кэшировать) | ❌ | **Создать:** `06-databases/caching/` или отдельный раздел. Упоминания есть в разных местах, но нет единого документа |
| 5 | Type safety (TypeScript/Pydantic/mypy) | — | Go-специфично не применимо; частично покрыто через generics и интерфейсы |
| 6 | Аутентификация (OAuth 2.0, bcrypt, JWT refresh, CSRF, session fixation) | ✅ | `12-security/authentication/` (01–07) |
| 7 | Async, очереди, фоновые задачи (горутины, Redis Streams, Kafka, воркеры) | ✅ | `09-concurrency-and-performance/`, `07-message-brokers-and-streaming/`, `04-architecture-and-patterns/patterns/04-background-workers.md` |
| 8 | Тесты (unit, интеграционные, API, моки внешних сервисов) | ✅ | `10-testing-and-quality/` (01–11) |
| 9 | **Реалтайм-протоколы** (WebSockets, SSE, long polling — когда что выбирать) | 🟡 | WebSocket есть: `08-networking-and-api/protocols/05-websocket.md`. **Создать:** SSE и long polling, сравнение протоколов реалтайма |
| 10 | Docker — контейнеризуй всё (Dockerfile, Compose, multi-stage, слои) | ✅ | `11-devops-and-observability/docker*/`, `dockerfiles-for-go/`, `docker-compose/` |

---

## Фаза 03 — Production-Grade (Недели 19–28)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | CI/CD пайплайны (GitHub Actions/GitLab CI, тесты на PR, secrets, zero-downtime) | ✅ | `11-devops-and-observability/ci-cd/` (GitHub Actions, GitLab CI) |
| 2 | Логи и observability (JSON, correlation IDs, уровни, Grafana+Loki/Datadog, алерты) | ✅ | `11-devops-and-observability/logging-and-log-shipping/`, `tracing-and-opentelemetry/`, `prometheus-and-metrics/` |
| 3 | **Облако — одна платформа** (AWS: EC2, RDS, S3, SQS, IAM, VPC, security groups, managed vs self-hosted) | ❌ | **Создать:** `11-devops-and-observability/cloud/` — AWS (или GCP) как платформа. Terraform есть (GCP), но нет AWS-обзора |
| 4 | Профилировка и нагрузка (flame graphs, slow query log, p95/p99) | ✅ | `09-concurrency-and-performance/profiling/`, `11-devops-and-observability/incident-investigation-and-profiling/` |
| 4a | **Нагрузочное тестирование** (k6/Locust, bottleneck → фикс → перетест) | ❌ | **Создать:** `10-testing-and-quality/12-load-testing.md` — k6 примеры, интерпретация результатов, p95/p99 |
| 5 | **Безопасность вглубь — OWASP Top 10** (SQL injection, XSS, SSRF, IDOR + AI-аудит) | 🟡 | CORS/CSRF/auth/rate-limit есть. **Создать:** `12-security/owasp-top10/` — SQL injection, XSS, SSRF как отдельные документы |
| 6 | **Edge и Serverless** (Cloudflare Workers, AWS Lambda, cold start, лимиты, биллинг) | ❌ | **Создать:** `11-devops-and-observability/serverless/` или `05-system-design/edge-and-serverless/` |
| 7 | **API-дизайн на масштаб** (idempotency, webhook design, версионирование, OpenAPI spec, backward compatibility) | 🟡 | Idempotency: `05-system-design/reliability-patterns/06-idempotency.md`. Webhooks: `08-networking-and-api/protocols/06-webhooks.md`. Версионирование: `04-architecture-and-patterns/patterns/03-api-versioning.md`. **Создать:** `08-networking-and-api/protocols/12-openapi-and-api-spec.md` — OpenAPI/Swagger spec, codegen |

---

## Фаза 04 — Распределёнка и AI (Недели 29–42)

| # | Тема из PDF | Статус | Наши файлы / Что нужно |
|---|---|---|---|
| 1 | System design фундамент (CAP, горизонтальное/вертикальное, LB, CDN, "Спроектируй Twitter") | ✅ | `05-system-design/` (CAP, external-request-flows, interview-cases) |
| 2 | БД на масштаб (read replicas, PgBouncer, шардирование, денормализация, партиционирование, zero-downtime миграции) | ✅ | `06-databases/database-systems-catalog/postgresql/` (06-replication, 05-partitioning, 12-sharding, 09-connection-pooling) |
| 3 | **LLM как часть бэкенда** (OpenAI/Anthropic API, streaming, function calling, токеномика, кэш промптов, деградация провайдера) | ❌ | **Создать:** `13-llm-and-ai-integration/` — новый раздел. Файлы: интеграция API, streaming ответов, function calling, prompt caching, fallback при отказе |
| 4 | **RAG и vector БД** (pgvector, Qdrant, Weaviate, эмбеддинги, чанкинг, гибридный поиск, стейл-данные, prompt injection через документы) | ❌ | **Создать:** `13-llm-and-ai-integration/rag/` — внутри нового раздела |
| 5 | Очереди и event streaming (Kafka: партиции, consumer groups, exactly-once, event sourcing, DLQ) | ✅ | `07-message-brokers-and-streaming/01-kafka.md` |
| 6 | Микросервисы — когда и как (gRPC vs REST, service mesh, distributed tracing, "монолит сначала") | ✅ | `04-architecture-and-patterns/service-topologies/`, `03-go-libraries-and-ecosystem/grpc/`, `11-devops-and-observability/tracing-and-opentelemetry/` |
| 7 | Kubernetes — основы (pods, services, deployments, ingress, ConfigMaps, secrets, HPA) | ✅ | `11-devops-and-observability/kubernetes/` (01–07) |
| 8 | **Reliability engineering** (SLO, SLI, error budgets, circuit breakers, chaos engineering, graceful degradation, постмортемы) | 🟡 | Circuit breaker/retries/backoff/idempotency: `05-system-design/reliability-patterns/`. **Создать:** `05-system-design/reliability-patterns/08-slo-sli-error-budgets.md`, `09-chaos-engineering.md`, `10-postmortem.md` |

---

## Фаза 05 — Привычки топ-1% (Недели 43–52)

Все темы этой фазы — soft skills и практики (читай кодбазы, OSS, пиши блог, специализация, системное мышление). Не технические топики — вне scope материалов.

---

## Итоговый список файлов к созданию

Упорядочен по приоритету (сначала более фундаментальные).

### Высокий приоритет

| Файл / Раздел | Раздел | Что покрывает |
|---|---|---|
| `06-databases/caching/01-redis-as-cache.md` | Databases | Cache-aside, TTL, инвалидация, прогрев, когда НЕ кэшировать, session store в Redis |
| `12-security/owasp-top10/01-sql-injection.md` | Security | Параметризованные запросы, ORM pitfalls, примеры в Go |
| `12-security/owasp-top10/02-xss.md` | Security | Reflected/stored/DOM XSS, Content-Security-Policy, html/template в Go |
| `12-security/owasp-top10/03-ssrf.md` | Security | SSRF атака, allowlist URL, блокировка metadata endpoint, примеры в Go |
| `05-system-design/reliability-patterns/08-slo-sli-error-budgets.md` | System Design | SLO/SLI/SLA определения, error budget, alerting на burn rate |
| `05-system-design/reliability-patterns/09-postmortem.md` | System Design | Структура постмортема, blame-free культура, шаблон |

### Средний приоритет

| Файл / Раздел | Раздел | Что покрывает |
|---|---|---|
| `11-devops-and-observability/git/01-branching-and-workflow.md` | DevOps | Trunk-based, gitflow, branching стратегии, rebase vs merge, git bisect |
| `11-devops-and-observability/git/02-advanced-git.md` | DevOps | Interactive rebase, cherry-pick, reflog, коммит-сообщения |
| `08-networking-and-api/protocols/12-sse-and-realtime.md` | Networking | SSE vs WebSocket vs long polling, когда что выбирать, SSE в Go |
| `10-testing-and-quality/12-load-testing.md` | Testing | k6 basics, сценарии, интерпретация p95/p99, bottleneck → фикс → перетест |
| `08-networking-and-api/protocols/13-openapi-and-swagger.md` | Networking | OpenAPI 3.0 spec, swagger codegen в Go, contract testing |
| `11-devops-and-observability/cloud/01-aws-core-services.md` | DevOps | EC2, RDS, S3, SQS, IAM, VPC, security groups, managed vs self-hosted |
| `11-devops-and-observability/cloud/02-cloud-cost-and-architecture.md` | DevOps | Стоимость инфры, выбор типа инстанса, reserved vs spot, cost optimization |

### Низкий приоритет (LLM/AI — отдельный большой блок)

| Файл / Раздел | Раздел | Что покрывает |
|---|---|---|
| `13-llm-and-ai-integration/01-llm-api-basics.md` | LLM (новый) | OpenAI/Anthropic API, модели, токены, стоимость |
| `13-llm-and-ai-integration/02-streaming-and-function-calling.md` | LLM (новый) | Streaming ответов (SSE), function/tool calling, structured output |
| `13-llm-and-ai-integration/03-prompt-engineering-for-backend.md` | LLM (новый) | System prompts, контекстное окно, prompt caching, temperature |
| `13-llm-and-ai-integration/04-reliability-and-fallback.md` | LLM (новый) | Деградация при отказе провайдера, timeout, fallback на другую модель, retry |
| `13-llm-and-ai-integration/rag/01-rag-fundamentals.md` | LLM (новый) | Что такое RAG, pipeline: chunk → embed → store → retrieve → generate |
| `13-llm-and-ai-integration/rag/02-vector-databases.md` | LLM (новый) | pgvector, Qdrant, Weaviate — сравнение, когда что выбирать |
| `13-llm-and-ai-integration/rag/03-chunking-and-embeddings.md` | LLM (новый) | Стратегии чанкинга, модели эмбеддингов, размерность |
| `13-llm-and-ai-integration/rag/04-rag-pitfalls.md` | LLM (новый) | Стейл-данные, prompt injection через документы, галлюцинации, оценка качества |
| `11-devops-and-observability/serverless/01-edge-and-serverless.md` | DevOps | Cloudflare Workers, AWS Lambda, cold start, лимиты, когда VPS избыточен |
| `05-system-design/reliability-patterns/10-chaos-engineering.md` | System Design | Chaos Monkey, fault injection, gamedays, инструменты |

---

## Что точно НЕ нужно добавлять

- Type safety (TypeScript/Pydantic/mypy) — это про другие языки, Go покрывает через систему типов
- AI-pair правила работы — не технический топик
- Контрибьюти в OSS, пиши блог, специализация — не технический топик
