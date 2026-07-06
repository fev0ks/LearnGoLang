# Networking And API

Сюда собирай темы про протоколы, API и сетевое поведение сервисов.

## Материалы

### [Протоколы и паттерны](./protocols/)

- [01. gRPC](./protocols/01-grpc.md) — Protobuf, 4 типа RPC, кодогенерация, interceptors, health check, reflection, gRPC vs REST
- [02. HTTP Server in Go](./protocols/02-http-server.md) — `net/http` server, middleware chain, timeouts, graceful shutdown
- [03. HTTP Client in Go](./protocols/03-http-client.md) — Transport, connection pooling, таймауты, retry с backoff, circuit breaker
- [04. Rate Limiting](./protocols/04-rate-limiting.md) — алгоритмы, token bucket, sliding window
- [05. WebSocket](./protocols/05-websocket.md) — Upgrade handshake, framing, opcodes, read/write goroutine паттерн, Hub, pub/sub backplane
- [06. Webhooks](./protocols/06-webhooks.md) — механика, at-least-once, HMAC-SHA256 signature, idempotency key, outbox паттерн
- [07. Idempotency](./protocols/07-idempotency.md) — Idempotency-Key header, генерация (UUID/hash), Redis SETNX, PostgreSQL ON CONFLICT, concurrent safety, consumer dedup
- [08. GraphQL](./protocols/08-graphql.md) — schema/query/mutation/subscription, N+1 + DataLoader, gqlgen, introspection, GraphQL vs REST
- [09. WebRTC](./protocols/09-webrtc.md) — signaling, ICE/STUN/TURN, SDP offer/answer, Pion в Go, P2P vs SFU
- [10. SOAP](./protocols/10-soap.md) — WSDL, конверт, заголовки, Fault, SOAP из Go (ручной + gowsdl), почему проиграл
- [11. Protocol Comparison](./protocols/11-protocol-comparison.md) — большая таблица REST/gRPC/GraphQL/WebSocket/SSE/Webhooks/WebRTC/SOAP, Mermaid decision tree, смешанные архитектуры (Gateway/BFF)
- [12. SSE и Realtime](./protocols/12-sse-and-realtime.md) — server-sent events, сравнение с WebSocket/polling
- [13. OpenAPI и Swagger](./protocols/13-openapi-and-swagger.md) — спецификация, кодогенерация, документирование REST API
- [14. HTTP/1.1, HTTP/2 и HTTP/3](./protocols/14-http2-and-http3.md) — head-of-line blocking на разных уровнях, streams/frames/HPACK, h2c и ALPN, QUIC, как gRPC ложится на HTTP/2, HTTP/2 в Go

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

### Подразделы

- [Rate Limiting Examples](./rate-limiting-examples/README.md)
- [What Happens When You Open google.com](./request-lifecycle/README.md)

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
