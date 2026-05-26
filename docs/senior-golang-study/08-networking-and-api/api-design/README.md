# API Design

Методичка по проектированию HTTP/REST + gRPC API: как из кирпичей протоколов
(HTTP, gRPC, protobuf) собрать долго живущий, удобный и безопасный API.

Раздел отвечает на вопросы:

- как назвать URL, чтобы они не превратились в RPC-имена функций;
- какой HTTP-метод когда брать;
- что класть в payload, а что — в headers/metadata;
- как избежать дублирования external/internal proto-сообщений и не писать мапперы;
- как версионировать, эволюционировать, делать backward-compatible изменения;
- как с первого дня закрыть пагинацию, ошибки, идемпотентность.

Основано на Google AIP (API Improvement Proposals), Stripe API Reference,
Microsoft / Zalando REST Guidelines и реальном опыте поддержки legacy API,
где половина проблем была спроектирована в первый месяц жизни.

## Три аксиомы

Эти три правила закрывают примерно 80% типичных ошибок дизайна. Подробно — в
[01-principles.md](./01-principles.md).

1. **Payload — это только доменные данные.** Cross-cutting (`userId`,
   `tenantId`, `locale`, `traceId`, `idempotencyKey`) живёт в metadata/headers,
   не в request-message. Это правило одно убирает большую часть необходимости в
   external/internal мапперах.
2. **URL — это имена ресурсов, HTTP-метод — действие.** Если в пути появляется
   глагол (`update`, `refresh`, `list`, `find`, `replace`) — что-то спроектировано
   неправильно. Custom action — через нотацию `resource/{id}:verb`.
3. **Один источник правды на доменный тип.** `Order`, `Bundle`, `User` определены
   ровно один раз и переиспользуются и в external, и в internal сервисах.
   `FieldMask` для частичных обновлений вместо отдельных `UpdateXRequest` с
   повторением полей.

## Материалы

### Принципы

- [01. Принципы](./01-principles.md) — три аксиомы дизайна и почему они закрывают
  большую часть типичных ошибок.

### URL и HTTP

- [02. URL-дизайн](./02-url-design.md) — множественное число, kebab-case, ID в
  path, custom actions через `:verb`, антипаттерны.
- [03. HTTP-методы](./03-http-methods.md) — GET/POST/PUT/PATCH/DELETE: семантика,
  идемпотентность, кешируемость, «POST as search».

### Моделирование данных

- [04. Моделирование ресурсов](./04-resource-modeling.md) — sub-resource vs
  top-level, глубина вложенности, bulk-операции.
- [05. Payload и типы](./05-payloads-and-types.md) — `Timestamp`/`Date`/`Money`,
  enum naming, `FieldMask`, `field_behavior`, `oneof`, `reserved`.

### Сквозные темы

- [06. Cross-cutting concerns](./06-cross-cutting-concerns.md) — `userId`,
  `locale`, `traceId`, `Idempotency-Key` через metadata, а не payload.
- [07. Пагинация и фильтрация](./07-pagination-and-filtering.md) — cursor-based
  (AIP-158), `maxPageSize`, единый стиль фильтров (AIP-160).
- [08. Ошибки](./08-errors.md) — HTTP-коды, единая `Error { code, message, details }`,
  `429 + Retry-After`, RFC 9457, антипаттерн «двойного канала».

### Эволюция и инструменты

- [09. Версионирование и эволюция](./09-versioning-and-evolution.md) —
  `/v1/`/`/v2/` через URL, `option deprecated`, что считается breaking change.
- [10. Структура proto-репозитория](./10-protobuf-repo-layout.md) —
  `common/` + `requests/` + `services/external/internal/` без дублирования и
  без мапперов.
- [11. Tooling](./11-tooling.md) — `buf lint`/`buf breaking`,
  `protoc-gen-openapiv2`, `protoc-gen-validate`, CI-правила.

### Шаблоны и ссылки

- [12. Greenfield-шаблон](./12-greenfield-template.md) — пошаговый пример нового
  домена `Payment`: domain type → requests → external + internal services.
- [13. Ссылки и источники](./13-references.md) — AIP, RFC, публичные стайлгайды.

## Рекомендованный порядок чтения

Если читаешь впервые подряд: 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09 → 10 →
11 → 12. Файл 13 — справочник, открывается по мере необходимости.

Если приходишь решить конкретную проблему:

- «как назвать URL» → [02](./02-url-design.md), [04](./04-resource-modeling.md)
- «какой метод выбрать» → [03](./03-http-methods.md)
- «как не дублировать message'и между external/internal» →
  [06](./06-cross-cutting-concerns.md), [10](./10-protobuf-repo-layout.md)
- «как с первого дня сделать update'ы» → [05](./05-payloads-and-types.md) §FieldMask
- «как обрабатывать ошибки» → [08](./08-errors.md)
- «как не сломать API при изменениях» → [09](./09-versioning-and-evolution.md)
- «делаю новый сервис, дайте шаблон» → [12](./12-greenfield-template.md)

## Вопросы для самопроверки

После прочтения должны быть готовые ответы на:

- почему `POST /v1/orders/123` для чтения — плохо, даже если технически работает;
- что должно случиться при сетевом ретрае на `POST /v1/payments`;
- зачем `FieldMask` нужнее, чем два отдельных Update/Patch endpoints;
- куда положить `userId`, если хочется один и тот же proto-message в external и
  internal сервисах;
- почему `success: false` в HTTP 200 — это анти-паттерн;
- как сделать поиск, когда фильтры не помещаются в URL.
