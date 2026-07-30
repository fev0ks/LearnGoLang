# 13. Ссылки и источники

## Содержание

- [Google AIP — API Improvement Proposals](#google-aip--api-improvement-proposals)
- [RFC](#rfc)
- [Публичные стайлгайды](#публичные-стайлгайды)
- [Документация конкретных API (как пример)](#документация-конкретных-api-как-пример)
- [Tooling](#tooling)
- [Книги и статьи](#книги-и-статьи)
- [Полезные блоги](#полезные-блоги)
- [Внутренние ссылки](#внутренние-ссылки)
- [Где практиковаться](#где-практиковаться)

Куда смотреть дальше, когда нужна детализация или авторитетный источник.

---

## Google AIP — API Improvement Proposals

[google.aip.dev](https://google.aip.dev) — публичный сборник правил
проектирования API, который Google использует внутри. Самый ценный
систематизированный источник.

Ключевые AIP, на которые ссылается этот раздел:

| AIP | Тема |
|---|---|
| [AIP-121](https://google.aip.dev/121) | Resource-oriented design |
| [AIP-122](https://google.aip.dev/122) | Resource names (плюрализация, иерархия) |
| [AIP-126](https://google.aip.dev/126) | Enums |
| [AIP-131](https://google.aip.dev/131) | Standard methods: Get |
| [AIP-132](https://google.aip.dev/132) | Standard methods: List (включая `orderBy`) |
| [AIP-133](https://google.aip.dev/133) | Standard methods: Create |
| [AIP-134](https://google.aip.dev/134) | Standard methods: Update (FieldMask) |
| [AIP-135](https://google.aip.dev/135) | Standard methods: Delete |
| [AIP-136](https://google.aip.dev/136) | Custom methods (`:verb` notation) |
| [AIP-140](https://google.aip.dev/140) | Field names |
| [AIP-143](https://google.aip.dev/143) | Standardized codes (language, currency) |
| [AIP-148](https://google.aip.dev/148) | Standard fields |
| [AIP-154](https://google.aip.dev/154) | Resource freshness validation (ETag) |
| [AIP-155](https://google.aip.dev/155) | Request identification (idempotency) |
| [AIP-157](https://google.aip.dev/157) | Partial responses (read_mask) |
| [AIP-158](https://google.aip.dev/158) | Pagination (page_token / page_size) |
| [AIP-160](https://google.aip.dev/160) | Filtering (filter string DSL) |
| [AIP-161](https://google.aip.dev/161) | Field masks |
| [AIP-162](https://google.aip.dev/162) | Resource revisions |
| [AIP-180](https://google.aip.dev/180) | Backwards compatibility |
| [AIP-193](https://google.aip.dev/193) | Errors (google.rpc.Status, Error details) |
| [AIP-203](https://google.aip.dev/203) | Field behavior documentation |
| [AIP-211](https://google.aip.dev/211) | Authorization checks |
| [AIP-216](https://google.aip.dev/216) | States (enum-style status fields) |
| [AIP-231](https://google.aip.dev/231) | Batch methods: Get |
| [AIP-233](https://google.aip.dev/233) | Batch methods: Create |
| [AIP-234](https://google.aip.dev/234) | Batch methods: Update |
| [AIP-235](https://google.aip.dev/235) | Batch methods: Delete |

---

## RFC

| RFC | Тема |
|---|---|
| [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) | HTTP Semantics (методы, коды, headers) |
| [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111) | HTTP Caching |
| [RFC 9112](https://www.rfc-editor.org/rfc/rfc9112) | HTTP/1.1 |
| [RFC 9113](https://www.rfc-editor.org/rfc/rfc9113) | HTTP/2 |
| [RFC 9114](https://www.rfc-editor.org/rfc/rfc9114) | HTTP/3 |
| [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) | Problem Details for HTTP APIs |
| [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594) | Sunset HTTP Header (deprecation) |
| [RFC 7231](https://www.rfc-editor.org/rfc/rfc7231) | (устарел, теперь 9110) HTTP/1.1 Semantics |
| [RFC 7233](https://www.rfc-editor.org/rfc/rfc7233) | Range Requests (для пагинации) |
| [RFC 5646](https://www.rfc-editor.org/rfc/rfc5646) | BCP 47 Language Tags |
| [RFC 3339](https://www.rfc-editor.org/rfc/rfc3339) | Date and Time on the Internet |
| [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986) | URI Generic Syntax |
| [RFC 7396](https://www.rfc-editor.org/rfc/rfc7396) | JSON Merge Patch |
| [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) | JSON Patch |
| [W3C Trace Context](https://www.w3.org/TR/trace-context/) | `traceparent`/`tracestate` headers |

---

## Публичные стайлгайды

- [Microsoft REST API Guidelines](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md) — подробный гайд от Microsoft Azure.
- [Zalando RESTful API Guidelines](https://opensource.zalando.com/restful-api-guidelines/) — известный публичный стайлгайд с конкретными правилами.
- [PayPal API Style Guide](https://github.com/paypal/api-standards/blob/master/api-style-guide.md) — много про идемпотентность и платежи.
- [Heroku Platform API Reference](https://devcenter.heroku.com/categories/platform-api) — идеоматичный REST.
- [JSON:API specification](https://jsonapi.org/) — стандарт для JSON-ответов с relationships, фильтрацией, пагинацией.

---

## Документация конкретных API (как пример)

- [Stripe API Reference](https://stripe.com/docs/api) — эталон REST API: pagination, idempotency, versioning, errors.
- [GitHub REST API](https://docs.github.com/en/rest) — большой публичный API с продуманным дизайном.
- [Twilio API Docs](https://www.twilio.com/docs/usage/api) — единый стиль ошибок, idempotency.
- [Square API](https://developer.squareup.com/docs) — pagination, idempotency, явные коды ошибок.

---

## Tooling

- [buf.build](https://buf.build/) — proto-репозиторий, lint, breaking change detector.
- [buf CLI docs](https://buf.build/docs/cli) — буфовые команды.
- [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) — REST поверх gRPC.
- [protoc-gen-openapiv2 docs](https://github.com/grpc-ecosystem/grpc-gateway/tree/main/protoc-gen-openapiv2) — генерация OpenAPI из proto.
- [protoc-gen-validate](https://github.com/bufbuild/protoc-gen-validate) — валидация полей через аннотации.
- [protoc-gen-validate-go](https://github.com/bufbuild/protovalidate-go) — runtime валидация.
- [OpenAPI Specification](https://swagger.io/specification/) — формат OpenAPI 3.
- [openapi-generator](https://openapi-generator.tech/) — генерация клиентских SDK из OpenAPI.

---

## Книги и статьи

- **«REST API Design Rulebook»**, Mark Massé — короткая практичная книга.
- **«API Design Patterns»**, JJ Geewax (Manning) — от Google-инженера, очень
  подробно по тем же AIP, с примерами и trade-offs.
- **«Build APIs You Won't Hate»**, Phil Sturgeon — практичный REST.
- **«Building Microservices»**, Sam Newman — главы про API между сервисами.
- [Roy Fielding's dissertation](https://ics.uci.edu/~fielding/pubs/dissertation/top.htm) — оригинальное определение REST. Читать редко, ссылаться часто.

---

## Полезные блоги

- [Brandur Leach's blog](https://brandur.org/) — много статей про Stripe API
  design.
- [API Evangelist](https://apievangelist.com/) — каталог практик и инструментов.
- [Nordic APIs Blog](https://nordicapis.com/blog/) — обзорные статьи.
- [Microsoft REST API Guidelines blog posts](https://github.com/microsoft/api-guidelines/blob/vNext/Guidelines.md#references) — обоснования правил.

---

## Внутренние ссылки

- [Раздел про gRPC и protobuf](../protocols/06-grpc.md)
- [HTTP-сервер на Go](../protocols/03-http-server.md)
- [Webhooks](../protocols/13-webhooks.md)
- [Idempotency](../protocols/14-idempotency.md)
- [OpenAPI и Swagger](../protocols/08-openapi-and-swagger.md)
- [Раздел про архитектуру и паттерны](../../04-architecture-and-patterns/)

---

## Где практиковаться

- Найти любой публичный API (Stripe, GitHub, Twilio), пройти по его документации
  с этим разделом в руках, отметить какие AIP/правила применены.
- Спроектировать API для абстрактного домена (TodoMVC, ChatApp, e-commerce) с
  нуля, по шаблону из [12-greenfield-template.md](./12-greenfield-template.md).
- Взять любой legacy-API, с которым идёт работа, и провести ревью по
  чек-листам из этого раздела. Большинство проектов имеют 30-50% несоответствий,
  и это нормально — главное понимать, что и почему.
