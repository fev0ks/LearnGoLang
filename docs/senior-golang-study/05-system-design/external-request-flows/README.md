# External Request Flows

Этот подпакет про типовые system design схемы: что происходит, когда внешний запрос приходит в систему и через какие слои он проходит.

Фокус:
- не на browser-level деталях;
- а на production path внутри системы: edge, gateway, auth, services, cache, DB, queues, object storage.

Материалы:
- [01 Basic Public API Request Flow](./01-basic-public-api-request-flow.md)
- [02 Read Heavy Request With CDN And Cache](./02-read-heavy-request-with-cdn-and-cache.md)
- [03 Write Request With Queue And Async Processing](./03-write-request-with-queue-and-async-processing.md)
- [04 Authenticated Request Through API Gateway](./04-authenticated-request-through-api-gateway.md)
- [05 File Upload And Background Processing Flow](./05-file-upload-and-background-processing-flow.md)
- [06 Where Latency And Failures Appear](./06-where-latency-and-failures-appear.md)
- [Edge And Proxy Patterns](./edge-and-proxy-patterns/README.md)

Как читать:
- начать с basic synchronous flow;
- потом посмотреть cached read path и async write path;
- затем разобрать auth flow и upload flow;
- в конце пройтись по bottlenecks, чтобы связать схемы с диагностикой.

Что важно уметь объяснить:
- где заканчивается edge и начинается application layer;
- зачем нужны gateway, LB, cache, queue и object storage;
- где запрос синхронный, а где уже асинхронный;
- как меняется путь запроса в read-heavy и write-heavy сценариях.
