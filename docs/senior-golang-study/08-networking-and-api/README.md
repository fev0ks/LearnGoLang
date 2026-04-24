# Networking And API

Сюда собирай темы про протоколы, API и сетевое поведение сервисов.

## Материалы

- [gRPC Overview](./grpc-overview.md) — Protobuf, 4 типа RPC, кодогенерация, interceptors, health check, reflection, gRPC vs REST
- [HTTP Server in Go](./http-server-in-go.md) — `net/http` server, middleware chain, timeouts, graceful shutdown
- [HTTP Client in Go](./http-client-in-go.md) — Transport, connection pooling, таймауты, retry с backoff, circuit breaker
- [WebSocket](./websocket.md) — Upgrade handshake, framing, opcodes, read/write goroutine паттерн, Hub, pub/sub backplane
- [Webhooks](./webhooks.md) — механика, at-least-once, HMAC-SHA256 signature, idempotency key, outbox паттерн
- [GraphQL](./graphql.md) — schema/query/mutation/subscription, N+1 + DataLoader, gqlgen, introspection, GraphQL vs REST
- [WebRTC](./webrtc.md) — signaling, ICE/STUN/TURN, SDP offer/answer, Pion в Go, P2P vs SFU
- [SOAP](./soap.md) — WSDL, конверт, заголовки, Fault, SOAP из Go (ручной + gowsdl), почему проиграл
- [Protocol Comparison](./protocol-comparison.md) — большая таблица REST/gRPC/GraphQL/WebSocket/Webhooks/WebRTC/SOAP, decision tree
- [Rate Limiting](./01-rate-limiting.md) — алгоритмы, token bucket, sliding window
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
