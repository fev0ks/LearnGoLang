# Networking And API

Сюда собирай темы про протоколы, API и сетевое поведение сервисов.

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
