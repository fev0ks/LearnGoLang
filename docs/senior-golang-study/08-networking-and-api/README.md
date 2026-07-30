# Networking And API

Раздел про протоколы, проектирование API и сетевое поведение сервисов.

## Материалы

### [Протоколы и паттерны](./protocols/README.md)

Что здесь протокол, что стиль, что формат описания контракта, а что прикладной паттерн — разобрано в [README каталога](./protocols/README.md).

Каталог разбит на подпапки по уровню стека: чем меньше номер, тем ниже уровень.

**Точка входа**

- [00. Сравнение протоколов](./protocols/00-protocol-comparison.md) — большая таблица REST/gRPC/GraphQL/WebSocket/SSE/Webhooks/WebRTC/SOAP, decision tree, смешанные архитектуры (Gateway/BFF)

**[01. Транспорт](./protocols/01-transport/README.md)**

- [TCP: надёжность, окна и перегрузка](./protocols/01-transport/01-tcp-mechanics.md) — подтверждения и повторные передачи, окно получателя против окна перегрузки, произведение полосы на задержку, MSS и MTU

**[02. HTTP](./protocols/02-http/README.md)**

- [Версии HTTP: 1.1, 2 и 3](./protocols/02-http/01-http-versions.md) — head-of-line blocking на разных уровнях, streams/frames/HPACK, h2c и ALPN, QUIC, как gRPC ложится на HTTP/2
- [HTTP-сервер в Go](./protocols/02-http/02-server-in-go.md) — `net/http` server, middleware chain, таймауты, graceful shutdown
- [HTTP-клиент в Go](./protocols/02-http/03-client-in-go.md) — Transport, пул соединений, таймауты, повторные попытки с backoff, circuit breaker

**[03. Стили API и контракты](./protocols/03-api-styles/README.md)**

- [REST и семантика HTTP](./protocols/03-api-styles/01-rest-and-http-semantics.md) — ограничения Филдинга, уровни зрелости Ричардсона, HATEOAS, полная карта кодов ответов с тонкостями, согласование представлений, условные запросы и оптимистичная блокировка, редиректы, Range, CORS
- [gRPC](./protocols/03-api-styles/02-grpc.md) — Protobuf, четыре типа вызовов, кодогенерация, interceptors, health check, reflection, gRPC против REST
- [GraphQL](./protocols/03-api-styles/03-graphql.md) — schema/query/mutation/subscription, проблема N+1 и DataLoader, gqlgen, интроспекция, GraphQL против REST
- [OpenAPI и Swagger](./protocols/03-api-styles/04-openapi-and-swagger.md) — спецификация, кодогенерация, spec-first против code-first, проверка совместимости, contract testing
- [SOAP](./protocols/03-api-styles/05-soap.md) — WSDL, конверт, заголовки, Fault, SOAP из Go, почему проиграл

**[04. Реалтайм](./protocols/04-realtime/README.md)**

- [WebSocket](./protocols/04-realtime/01-websocket.md) — Upgrade-рукопожатие, кадры и opcodes, паттерн read/write-горутин, Hub, backplane через pub/sub
- [SSE и реалтайм-протоколы](./protocols/04-realtime/02-sse.md) — server-sent events, Last-Event-ID, прокси и балансировщики, сравнение с WebSocket и long polling
- [WebRTC](./protocols/04-realtime/03-webrtc.md) — сигнализация, ICE/STUN/TURN, SDP offer/answer, Pion в Go, P2P против SFU

**[05. Интеграционные паттерны](./protocols/05-integration-patterns/README.md)**

- [Webhooks](./protocols/05-integration-patterns/01-webhooks.md) — механика, at-least-once, подпись HMAC-SHA256, ключ идемпотентности, outbox
- [Идемпотентность запросов](./protocols/05-integration-patterns/02-idempotency.md) — заголовок Idempotency-Key, Redis SETNX, PostgreSQL ON CONFLICT, конкурентная безопасность, дедупликация на потребителе
- [Ограничение частоты запросов](./protocols/05-integration-patterns/03-rate-limiting.md) — fixed window, sliding window, token bucket, реализация в Redis без гонок, fail-open против fail-closed
- [Рабочие реализации ограничителей](./protocols/05-integration-patterns/examples/README.md) — компилируемый Go с тестами: три алгоритма за одним интерфейсом

### [API Design](./api-design/)

Методичка по проектированию REST + protobuf API с нуля: URL, HTTP-методы,
моделирование ресурсов, payload, pagination/errors/idempotency, versioning,
организация proto-репозитория без дублирования external/internal message'ей,
tooling, готовый greenfield-шаблон. Применима к любому HTTP/gRPC API.

- [README](./api-design/README.md) — оглавление + три аксиомы
- [01. Принципы](./api-design/01-principles.md) — три аксиомы дизайна
- [02. URL-дизайн](./api-design/02-url-design.md) — plural, kebab-case, no verbs
- [03. HTTP-методы](./api-design/03-http-methods.md) — семантика, идемпотентность, POST-as-search
- [04. Моделирование ресурсов](./api-design/04-resource-modeling.md) — sub-resource vs top-level
- [05. Payload и типы](./api-design/05-payloads-and-types.md) — Timestamp, Money, FieldMask, field_behavior
- [06. Cross-cutting concerns](./api-design/06-cross-cutting-concerns.md) — userId/locale/idempotency через metadata
- [07. Пагинация и фильтрация](./api-design/07-pagination-and-filtering.md) — cursor-based, AIP-158
- [08. Ошибки](./api-design/08-errors.md) — HTTP-коды + единая Error, RFC 9457
- [09. Версионирование и эволюция](./api-design/09-versioning-and-evolution.md) — /v1/, deprecated, breaking changes
- [10. Структура proto-репозитория](./api-design/10-protobuf-repo-layout.md) — без дублирования и мапперов
- [11. Tooling](./api-design/11-tooling.md) — buf, OpenAPI gen, validators, CI
- [12. Greenfield-шаблон](./api-design/12-greenfield-template.md) — полный пример Payment-домена
- [13. Ссылки и источники](./api-design/13-references.md) — AIP, RFC, публичные стайлгайды

### [Что происходит при открытии google.com](./request-lifecycle/README.md)

Путь одного запроса целиком: ввод адреса в браузере, DNS, TCP и TLS, край сети, приложение, ответ и отрисовка. Здесь — последовательность этапов и вклад каждого в общее время, тогда как в [протоколах](./protocols/README.md) — устройство отдельных механизмов.

Темы:
- HTTP/1.1, HTTP/2, keep-alive, connection pooling;
- REST, gRPC, async APIs;
- protobuf, backward compatibility, field evolution;
- timeouts, retries, circuit breakers;
- load balancers, service discovery;
- pagination, filtering, sorting, API consistency;
- idempotency keys;
- webhooks и подпись запросов;
- rate limiting и quota design.

Полезные сравнения:
- REST vs gRPC;
- synchronous call vs async event;
- server-side timeout vs client-side timeout;
- polling vs push.

## Подборка

- [RFC 9110 HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110)
- [gRPC Documentation](https://grpc.io/docs/)
- [gRPC Guides](https://grpc.io/docs/guides/)
- [Protocol Buffers Overview](https://protobuf.dev/overview/)
- [OWASP Web Service Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Web_Service_Security_Cheat_Sheet.html)
- [OWASP gRPC Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/gRPC_Security_Cheat_Sheet.html)

## Вопросы

- когда выбрать REST, а когда gRPC;
- почему timeout без retry policy почти так же плох, как retry без timeout;
- как не сломать backward compatibility в protobuf schema;
- где именно должны жить retry, circuit breaker и rate limit;
- как проектировать idempotent write API;
- чем опасны бесконтрольные synchronous chain calls между сервисами.
