# Networking And API

Сюда собирай темы про протоколы, API и сетевое поведение сервисов.

## Материалы

### [Протоколы и паттерны](./protocols/README.md)

Что здесь протокол, что стиль, что формат описания контракта, а что прикладной паттерн — разобрано в [README каталога](./protocols/README.md).

**Точка входа**

- [00. Сравнение протоколов](./protocols/00-protocol-comparison.md) — большая таблица REST/gRPC/GraphQL/WebSocket/SSE/Webhooks/WebRTC/SOAP, decision tree, смешанные архитектуры (Gateway/BFF)

**Транспорт и HTTP**

- [01. TCP: надёжность, окна и перегрузка](./protocols/01-tcp-mechanics.md) — подтверждения и повторные передачи, окно получателя против окна перегрузки, произведение полосы на задержку, MSS и MTU
- [02. HTTP/1.1, HTTP/2 и HTTP/3](./protocols/02-http2-and-http3.md) — head-of-line blocking на разных уровнях, streams/frames/HPACK, h2c и ALPN, QUIC, как gRPC ложится на HTTP/2
- [03. HTTP-сервер в Go](./protocols/03-http-server.md) — `net/http` server, middleware chain, таймауты, graceful shutdown
- [04. HTTP-клиент в Go](./protocols/04-http-client.md) — Transport, пул соединений, таймауты, повторные попытки с backoff, circuit breaker

**Стили API и контракты**

- [05. REST и семантика HTTP](./protocols/05-rest-and-http-semantics.md) — ограничения Филдинга, уровни зрелости Ричардсона, HATEOAS, полная карта кодов ответов с тонкостями, согласование представлений, условные запросы и оптимистичная блокировка, редиректы, Range, CORS
- [06. gRPC](./protocols/06-grpc.md) — Protobuf, четыре типа вызовов, кодогенерация, interceptors, health check, reflection, gRPC против REST
- [07. GraphQL](./protocols/07-graphql.md) — schema/query/mutation/subscription, проблема N+1 и DataLoader, gqlgen, интроспекция, GraphQL против REST
- [08. OpenAPI и Swagger](./protocols/08-openapi-and-swagger.md) — спецификация, кодогенерация, spec-first против code-first, проверка совместимости, contract testing
- [09. SOAP](./protocols/09-soap.md) — WSDL, конверт, заголовки, Fault, SOAP из Go, почему проиграл

**Реалтайм**

- [10. WebSocket](./protocols/10-websocket.md) — Upgrade-рукопожатие, кадры и opcodes, паттерн read/write-горутин, Hub, backplane через pub/sub
- [11. SSE и реалтайм-протоколы](./protocols/11-sse-and-realtime.md) — server-sent events, Last-Event-ID, прокси и балансировщики, сравнение с WebSocket и long polling
- [12. WebRTC](./protocols/12-webrtc.md) — сигнализация, ICE/STUN/TURN, SDP offer/answer, Pion в Go, P2P против SFU

**Интеграционные паттерны**

- [13. Webhooks](./protocols/13-webhooks.md) — механика, at-least-once, подпись HMAC-SHA256, ключ идемпотентности, outbox
- [14. Идемпотентность запросов](./protocols/14-idempotency.md) — заголовок Idempotency-Key, Redis SETNX, PostgreSQL ON CONFLICT, конкурентная безопасность, дедупликация на потребителе
- [15. Ограничение частоты запросов](./protocols/15-rate-limiting.md) — fixed window, sliding window, token bucket, реализация в Redis без гонок, fail-open против fail-closed

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
