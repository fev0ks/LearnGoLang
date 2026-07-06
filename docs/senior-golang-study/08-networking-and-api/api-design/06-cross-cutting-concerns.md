# 06. Cross-cutting concerns

Cross-cutting — это «сквозные» поля, которые одинаково присутствуют в большинстве
ручек API, но не являются доменными данными конкретной операции. Они либо
приходят из контекста сессии (auth), либо описывают **как** обработать запрос
(locale, idempotency), а не **что** обработать.

**Правило: cross-cutting concerns живут в metadata/headers, не в payload.**

Это правило одно убирает огромное количество проблем — security, контракт,
дублирование external/internal message'ей.

## Список cross-cutting полей

| Поле | Канал передачи |
|---|---|
| `userId`, `tenantId`, `actorId` | `Authorization: Bearer <JWT>` → metadata → ctx |
| `locale`, `language` | `Accept-Language: en-GB` (RFC 5646) |
| `currency` (user preference) | `X-Currency: EUR` или из профиля по JWT |
| `traceId`, `spanId` | `traceparent` (W3C Trace Context) |
| `requestId` | `X-Request-Id` (генерится клиентом или gateway) |
| `idempotencyKey` | `Idempotency-Key` |
| `userAgent`, `clientVersion` | `User-Agent`, `X-Client-Version` |
| `deadline`/`timeout` | gRPC deadline / `Request-Timeout` header |
| `apiVersion` (если динамическая) | через URL `/v1/`, не в headers |

Признак, что поле — cross-cutting: оно одинаково для всех ручек, и клиент его
**не выбирает** — оно принадлежит сессии/среде.

## userId, tenantId — самое важное

В payload:

```protobuf
message GetOrderRequest {
  string userId = 1;   // <-- источник всей боли
  string orderId = 2;
}
```

Проблемы:

1. **Security.** Клиент может подменить чужой `userId`. Защита висит на gateway,
   который перезаписывает поле из JWT. Один забытый middleware = privilege
   escalation.
2. **Дублирование external/internal.** Внешний контракт «не должен» принимать
   `userId` от клиента, а внутреннему сервису он нужен. Появляются два набора
   одинаковых типов и mapper.
3. **Контракт врёт.** OpenAPI показывает `userId` как input — клиенты учатся
   неверно.
4. **PII в логах.** Запрос клиента содержит чужой `userId` в теле.

Без поля:

```protobuf
message GetOrderRequest {
  string orderId = 1;
}
```

Gateway middleware:

```go
// Извлекаем userId из JWT, кладём в gRPC metadata
ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
    "user-id", claims.UserID,
    "tenant-id", claims.TenantID,
))
```

Internal сервис:

```go
func (s *Server) GetOrder(ctx context.Context, req *GetOrderRequest) {
    userID := auth.UserIDFromContext(ctx)  // не из req!
    // ...
}
```

Один message работает в обоих сервисах: и в external (где gateway проставляет
metadata из JWT), и в internal (где service-mesh middleware проставляет
metadata из service identity).

### Что если уже есть `userId` в payload

Не обязательно сразу удалять — может сломать клиентов. Поэтапно:

1. Пометить как `OUTPUT_ONLY`:

   ```protobuf
   import "google/api/field_behavior.proto";

   message GetOrderRequest {
     string userId = 1 [(google.api.field_behavior) = OUTPUT_ONLY];
     string orderId = 2;
   }
   ```

   `OUTPUT_ONLY` скрывает поле из request schema в OpenAPI — клиенты перестают
   видеть его. На сервере поле остаётся, gateway его заполняет, хендлеры читают.

2. Зафиксировать инвариант на gateway: «для всех external endpoint'ов userId в
   request перезаписывается значением из токена, всегда».

3. Линтер на проекте: запрет на чтение `request.UserId` в external-хендлерах
   (только `ctx`).

4. Через 1-2 версии — удалить поле и зарезервировать номер.

## locale / language

Антипаттерн (в каждом request-message):

```protobuf
message GetOrderRequest {
  string orderId = 1;
  string lang = 2;     // "en", "ru"
}

message GetBundleRequest {
  string bundleId = 1;
  string lang = 2;     // повтор
}
```

`lang` нужен почти всем ручкам — он cross-cutting.

Стандарт:

```text
GET /v1/orders/{orderId}
Accept-Language: en-GB, en;q=0.9, ru;q=0.8
```

Это RFC 5646 (BCP 47) language tag + RFC 9110 content negotiation. На сервере:

```go
lang := middleware.AcceptLanguage(ctx)  // парсит заголовок и матчит с поддерживаемыми
```

В gRPC — через metadata:

```text
accept-language: en-GB
```

grpc-gateway автоматически пробрасывает HTTP-заголовки в gRPC metadata.

## currency

Тонкий момент: currency — это иногда **данные** ресурса (валюта прайса,
зафиксирована в момент создания заказа), а иногда — **preference** клиента
(«покажи мне в EUR»).

| Тип | Где |
|---|---|
| Валюта данных ресурса (зафиксированная) | в payload (`Money { currencyCode, amountMinor }`) |
| Предпочтение клиента (что показывать) | header `X-Currency` или из user profile |

Не смешивать. Если клиент хочет «пересчитать в EUR» — это **новая операция**
(`POST /v1/orders/{id}:convertCurrency` или query `?displayCurrency=EUR`), а не
поле в каждом request.

## traceId, requestId

Антипаттерн:

```protobuf
message GetOrderRequest {
  string orderId = 1;
  string traceId = 2;   // <-- нет!
}
```

Стандарт — W3C Trace Context:

```text
GET /v1/orders/abc
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
tracestate: ...
```

OpenTelemetry SDK (для Go, Python, JS) автоматически читает/пишет эти headers.
В gRPC то же через metadata.

`X-Request-Id` — для request-level логирования, обычно генерируется gateway,
если не пришёл от клиента.

## Idempotency-Key

Для всех мутаций, которые опасны при повторе (платежи, создание ресурсов).

Передача — header `Idempotency-Key`. Клиент генерирует UUID v4 на каждый
логический запрос. Сервер хранит ключ + ответ N часов:

```text
POST /v1/payments
Idempotency-Key: 9e3b5c8a-...
Body: { ... }
```

При повторе с тем же ключом — сервер возвращает кешированный ответ, не
повторяя побочный эффект.

Если очень нужно в proto (для удобства internal обработки):

```protobuf
message CreatePaymentRequest {
  Payment payment = 1;
  string idempotencyKey = 2 [(google.api.field_behavior) = OPTIONAL];
}
```

Но лучше — через header + middleware, чтобы поле не загромождало каждый request.

Подробно — в [../protocols/07-idempotency.md](../protocols/07-idempotency.md).

## API key / альтернативные токены

Если способов авторизации несколько — никогда не класть в payload:

```protobuf
// плохо
message GetOrderRequest {
  string orderId = 1;
  string paymentAuthToken = 2;  // альтернативный токен
}
```

Заголовок:

```text
GET /v1/orders/abc
Authorization: Bearer <jwt>
X-Payment-Token: <payment-specific-token>
```

Или два разных endpoint'а с разной авторизацией:

- `/v1/orders/{orderId}` — JWT.
- `/v1/orders/{orderId}/by-token` — другая авторизация (через header `X-Payment-Token`).

В любом случае токен не в теле/query — иначе он попадает в access-log.

## Deadline / timeout

gRPC имеет встроенный `Deadline` через context. grpc-gateway транслирует его в
HTTP-заголовок `Grpc-Timeout`.

Если нужно client-specified timeout в HTTP API — добавляйте `Request-Timeout`
header. **Не** добавляйте `timeoutMs` в каждый message.

## Передача cross-cutting через grpc-gateway

grpc-gateway автоматически пробрасывает многие headers в gRPC metadata:

```go
// gateway.go
mux := runtime.NewServeMux(
    runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
        switch strings.ToLower(key) {
        case "accept-language", "x-currency", "idempotency-key",
             "x-request-id", "traceparent", "tracestate",
             "authorization":
            return key, true
        }
        return "", false
    }),
)
```

Без явного матчинга gateway пропускает только префикс `Grpc-Metadata-*`.

На стороне gRPC-хендлера:

```go
md, _ := metadata.FromIncomingContext(ctx)
langs := md.Get("accept-language")
idempotencyKey := md.Get("idempotency-key")
```

Лучше — обернуть в типизированные функции:

```go
package auth

func UserIDFromContext(ctx context.Context) string {
    md, _ := metadata.FromIncomingContext(ctx)
    vals := md.Get("user-id")
    if len(vals) == 0 {
        return ""
    }
    return vals[0]
}
```

## Frontend / mobile client считает поля «настолько важными, что должны быть в API»

Бывает соблазн положить в API поля типа `clientVersion`, `platform`, `country`,
`abExperiment`. Это всё metadata/headers:

- `User-Agent: MyApp/1.5.2 (iOS 17.0)`
- `X-Client-Version: 1.5.2`
- `X-Platform: ios`
- `X-Country: GB` (если не выводится из IP/locale)
- `X-Experiment: feed_v2_enabled`

Они часто нужны на серверной стороне для A/B-тестов, feature flags, аналитики.
Но **не** для бизнес-логики операции. Поэтому — headers.

## Связанные документы

- [01-principles.md](./01-principles.md) — Аксиома 1.
- [03-http-methods.md](./03-http-methods.md) — `Idempotency-Key` на платежах.
- [05-payloads-and-types.md](./05-payloads-and-types.md) — `field_behavior` для
  postponed deletion.
- [10-protobuf-repo-layout.md](./10-protobuf-repo-layout.md) — как это помогает
  объединить external/internal message'и.
- [../protocols/07-idempotency.md](../protocols/07-idempotency.md) — реализация
  идемпотентности.
