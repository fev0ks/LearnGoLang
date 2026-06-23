# Backend Engineer TL — вопросы скрининга (Interview-ready)

Список вопросов из скрининга на роль **Backend Engineer / Tech Lead** с короткими ответами «как сказать на собесе». Уклон в системный дизайн, надёжность и процессы, а не в синтаксис языка. Где есть профильный материал в `docs/senior-golang-study/`, дана ссылка на файл.

## Содержание

- [Релизы и процессы](#релизы-и-процессы)
- [Консистентность и архитектура](#консистентность-и-архитектура)
- [Нагрузка и надёжность](#нагрузка-и-надёжность)
- [Тестирование и CI/CD](#тестирование-и-cicd)

---

## Релизы и процессы

**Много пользователей; как выкатывать фичи постепенно, чтобы не задеть всех сразу?**

- **Feature flags / toggles** — фича катится в код «выключенной», включается рантайм-конфигом без передеплоя; даёт мгновенный kill switch и развязывает деплой от релиза.
- **Canary release** — выкатка на 1% → 5% → 25% → 100% с проверкой метрик (error rate, latency, бизнес-метрики) на каждом шаге и автоматическим откатом при деградации.
- **Targeted / percentage rollout** — включение по сегменту или проценту пользователей (hash userID, регион, internal staff first — dogfooding).
- **Blue-green / shadow traffic** — переключение трафика между двумя средами; shadow — дублируем реальный трафик на новую версию без отдачи ответа пользователю.
- Ключевое: у каждого этапа должны быть **метрики успеха и автооткат**, а флаги обязательно вычищать потом (flag debt).

**Какие практики обеспечения качества кода знаете?**

- **Code review** обязательный, маленькие PR, чек-лист; защита веток (мерж только через зелёный pipeline).
- **Definition of Done** (тесты, доки, метрики), коддинг-стандарты, trunk-based / GitFlow.
- **Тестовая пирамида** и TDD; pair/mob programming на сложных участках. См. [01-testing-strategy.md](../09-testing-and-quality/01-testing-strategy.md).
- **Постмортемы без обвинений** — учимся на инцидентах.
- Как TL подчеркнуть: качество — это **процесс и культура**, инструменты лишь автоматизируют рутину.

**Какие инструменты и процессы для качества кода команды?**

- **Линтеры/форматтеры**: golangci-lint, gofmt/goimports. Стандарт де-факто для Go.
- **Статический анализ и security**: staticcheck, `go vet`, gosec, Semgrep, SonarQube.
- **Coverage gate** и **race detector** (`go test -race`) в CI.
- **CI на каждый PR**: lint + build + тесты, блокировка мержа при падении.
- **Dependency/vuln scanning**: govulncheck, Dependabot, Snyk; pre-commit hooks.
- Главное — проверки живут **в CI и блокируют мерж**, а не «на совести разработчика».

## Консистентность и архитектура

**Event-driven + кэширование: какие вопросы консистентности учитывать?**

- **Eventual consistency** — явно решить, где допустимо отставание, а где нужна строгая консистентность.
- **Stale cache** — стратегии инвалидации: cache-aside + TTL, write-through/write-behind, event-driven invalidation.
- **Dual-write problem** — атомарно записать в БД и опубликовать событие нельзя напрямую; решение — **Transactional Outbox** + CDC (Debezium) или event sourcing.
- **Идемпотентность консьюмеров** — события приходят дважды (at-least-once); нужен dedup по event id. См. [06-idempotency.md](../05-system-design/reliability-patterns/06-idempotency.md).
- **Порядок событий** — гарантирован только внутри партиции (Kafka); ключ партиционирования по entity id.
- **Saga / компенсации** вместо распределённых транзакций (2PC); **read-your-writes** для собственных данных пользователя.
- Главная мысль: выбрать **уровень консистентности под каждый сценарий**, а не «строгую везде».

**Какие распространённые паттерны проектирования в микросервисной архитектуре?**

- **Коммуникация:** API Gateway, BFF, Service Discovery, sync (REST/gRPC) vs async (события/брокер).
- **Данные:** Database per service, **Saga** (хореография/оркестрация), **CQRS**, Event Sourcing, **Transactional Outbox / CDC**.
- **Надёжность:** Circuit Breaker, Retry + backoff + jitter, Bulkhead, Timeout, Rate limiting, Idempotency. См. [reliability-patterns](../05-system-design/reliability-patterns/).
- **Эксплуатация/observability:** distributed tracing (correlation id, OpenTelemetry), Sidecar / Service Mesh, **Strangler Fig** для миграции из монолита.
- Принципы декомпозиции — по bounded context (DDD), а не по техническим слоям.

**Синхронный вызов, вторая сторона не отвечает, но данные важны — как быть?**

- **Защита вызова:** обязательный **timeout** (`context.WithTimeout`), **retry с backoff + jitter** только для идемпотентных операций, **circuit breaker**, чтобы не добивать мёртвую зависимость. См. [01-timeouts-and-deadlines.md](../05-system-design/reliability-patterns/01-timeouts-and-deadlines.md).
- **Чтобы не потерять важные данные (суть вопроса):** снять зависимость от живости второй стороны — положить запрос в **очередь / Transactional Outbox** и доставить, когда она оживёт. Sync RPC превращается в надёжную доставку. См. [03-write-request-with-queue-and-async-processing.md](../05-system-design/external-request-flows/03-write-request-with-queue-and-async-processing.md).
- **Persist + retry позже** фоновым воркером; at-least-once + **идемпотентность** на приёмнике; **Dead Letter Queue** для недоставленного вместо потери. См. [06-idempotency.md](../05-system-design/reliability-patterns/06-idempotency.md).
- **Деградация:** отдать кэш/частичный ответ или `202 Accepted` вместо ошибки.
- Ключевая мысль: если данные критичны — запрос становится **персистентным сообщением с гарантированной доставкой и идемпотентным приёмом**, а не блокирующим RPC.

## Нагрузка и надёжность

**Приложение генерирует огромный трафик; как обработать поток запросов, превышающий доступные ресурсы?**

- **Защита (деградировать управляемо):** rate limiting/throttling (token/leaky bucket) на gateway и per-client; **load shedding** — отбрасывать низкоприоритетные запросы; **backpressure** — очереди с лимитом; **circuit breaker**; **graceful degradation** (урезанный ответ/из кэша).
- **Масштабирование:** horizontal scaling + **autoscaling** (HPA по RPS/custom-метрикам), load balancing, **async-обработка** через очередь (отвечаем `202`, обрабатываем фоном), кэширование на всех уровнях (CDN/edge/app/DB), read replicas, sharding, connection pooling.
- **Заранее:** capacity planning и **load testing** (k6, Locust), приоритизация трафика (QoS) — платящие/критичные операции важнее.
- Как TL: при превышении ёмкости система должна **деградировать предсказуемо** (rate limit + load shedding + queue), а не падать каскадно. Автоскейлинг не мгновенный — между ростом нагрузки и подъёмом нод нужны буферы (очереди, кэш, shedding).

## Тестирование и CI/CD

**Объясните пирамиду тестирования.**

- Распределение тестов: много дешёвых быстрых внизу, мало дорогих медленных вверху.
- **Unit (~70%)** — изолированные, быстрые (мс), без I/O, на каждый коммит.
- **Integration / service (~20%)** — связка компонентов: код + БД/брокер, два сервиса; в Go удобно через testcontainers.
- **E2E / UI (~10%)** — сквозной сценарий через всю систему: медленные, хрупкие (flaky), только критичные user-journeys.
- **Зачем:** скорость и стабильность фидбэка. **Антипаттерны:** ice cream cone (перевёрнутая пирамида — много e2e, мало unit) и hourglass (дыра в интеграционных, где живёт большинство багов).
- Для микросервисов уместно упомянуть «testing trophy» — упор на integration. См. [01-testing-strategy.md](../09-testing-and-quality/01-testing-strategy.md).

**Greenfield-проект: как подойти к CI/CD pipeline?**

- **Принципы:** pipeline as code + IaC (Terraform), fast feedback (параллелим, кэшируем), shift-left (качество и security рано), trunk-based + защита веток.
- **CI на каждый PR:** lint/format → build → unit + `go test -race` + coverage gate → integration (testcontainers) → SAST/vuln (govulncheck, gosec, dependency scan) → сборка **immutable Docker image** (тег = git sha) → push в registry.
- **CD:** автодеплой на staging, на prod — через гейт/кнопку (особенно на старте); стратегии canary / blue-green / rolling + автооткат по метрикам; **feature flags** (деплой ≠ релиз); версионируемые backward-compatible миграции БД (expand/contract); secrets management (Vault), не в репозитории.
- **Observability с первого дня:** логи, метрики, трейсы, алерты, smoke-тесты после деплоя.
- Как TL: заложить **right-sized pipeline** — без over-engineering (не k8s + service mesh ради одного сервиса), но сразу: тест-гейт, immutable-артефакты, один прод-путь деплоя и rollback. Дешевле заложить культуру «main всегда деплоится» сразу, чем внедрять потом.
