# 08. Ошибки

## Содержание

- [Антипаттерн: «двойной канал»](#антипаттерн-двойной-канал)
- [Правильно: HTTP-код + единая Error-структура](#правильно-http-код--единая-error-структура)
- [Какой HTTP-код возвращать](#какой-http-код-возвращать)
- [Машинно-читаемые коды](#машинно-читаемые-коды)
- [details для form errors](#details-для-form-errors)
- [Rate limiting → HTTP 429](#rate-limiting--http-429)
- [5xx — серверные ошибки](#5xx--серверные-ошибки)
- [gRPC ↔ HTTP-коды](#grpc--http-коды)
- [Кастомизация error response](#кастомизация-error-response)
- [Сводка](#сводка)
- [Связанные документы](#связанные-документы)

Один источник истины — HTTP-код плюс единая структурированная ошибка. Никаких
двойных каналов и никаких `success bool` внутри тела ответа.
Файл про формат ошибки и работу с ней: как выглядит тело, откуда берутся
стабильные машинные коды, что класть в `details`, как это ложится на gRPC.
Выбор самого кода ответа — тема соседнего материала
[REST и семантика HTTP](../protocols/03-api-styles/01-rest-and-http-semantics.md).

---

## Антипаттерн: «двойной канал»

```protobuf
message ApplyPromoCodeResponse {
  bool success = 1;
  string message = 2;
  string appliedCode = 3;
}
```

Клиент обязан проверять и HTTP-код, и `success`. Что значит:

- HTTP 200 + `success: false` — успех или ошибка?
- HTTP 400 + тело без `success` — кто прав?
- Что писать в логе?

Это везде создаёт неоднозначность. Stripe, GitHub, Google никогда так не
делают.

---

## Правильно: HTTP-код + единая Error-структура

При успехе:

- HTTP 200 / 201 / 204.
- Тело — нормальный response без флагов успеха.

При ошибке:

- HTTP 4xx или 5xx.
- Тело — единая структура Error.

```protobuf
message Error {
  // Машинно-читаемый код, стабильный по версиям API.
  // Пример: "promo_code.invalid", "order.not_found".
  string code = 1;

  // Человекочитаемое сообщение, в локали запроса.
  string message = 2;

  // Дополнительные детали (form errors, конкретные поля).
  google.protobuf.Struct details = 3;
}
```

Это совместимо с RFC 9457 (Problem Details for HTTP APIs) и с
Google AIP-193.

В Go-handler через grpc-gateway:

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

func (s *Server) ApplyPromoCode(ctx context.Context, req *ApplyPromoCodeRequest) (*ApplyPromoCodeResponse, error) {
    if !validPromoCode(req.Code) {
        return nil, status.Errorf(codes.InvalidArgument, "promo_code.invalid: %s", req.Code)
    }
    // ...
}
```

grpc-gateway автоматически мапит gRPC `codes.InvalidArgument` в HTTP 400 и
сериализует `status.Status` как JSON.

---

## Какой HTTP-код возвращать

Полная карта кодов по классам, тонкости выбора (400 против 422, 401 против 403, 404 против 403, 409 против 412) и семантика заголовков вроде `WWW-Authenticate`, `Allow` и `Retry-After` вынесены в [REST и семантика HTTP](../protocols/03-api-styles/01-rest-and-http-semantics.md) — это общая тема, не привязанная к дизайну конкретного API.

Здесь остаётся то, что относится к телу ответа, и одно правило, которое связывает две части: код и тело не должны противоречить друг другу. Ответ `200 OK` с телом `{"error": ...}` ломает кэширование, автоматические повторы и мониторинг, а `500` с подробным описанием ошибки валидации вводит в заблуждение и клиента, и дежурного.

Короткая шпаргалка для этого файла:

- `2xx` — тело содержит результат, никаких флагов успеха внутри;
- `4xx` — тело содержит структуру ошибки со стабильным машинным кодом, повтор без изменений не поможет;
- `5xx` — тело содержит обезличенное сообщение и идентификатор запроса, подробности только в логах.

---

## Машинно-читаемые коды

`code` в `Error` — стабильный идентификатор для клиента, не зависящий от текста.

Формат: `<domain>.<error>`:

```
order.not_found
order.expired
order.invalid_state
payment.declined
payment.insufficient_funds
promo_code.invalid
promo_code.already_used
quiz.session_not_found
auth.token_expired
```

Клиент пишет:

```ts
if (error.code === 'payment.declined') { showRetryDialog() }
else if (error.code === 'payment.insufficient_funds') { showTopUpDialog() }
```

Не зависит от локализации текста, не ломается при изменении сообщения.

---

## details для form errors

Когда нужно сообщить клиенту, какие именно поля невалидны:

```json
{
  "code": "validation.failed",
  "message": "Some fields are invalid",
  "details": {
    "fieldViolations": [
      {"field": "contactEmail", "code": "invalid_email", "description": "Invalid email format"},
      {"field": "members[0].dateOfBirth", "code": "required", "description": "Required field"}
    ]
  }
}
```

Google AIP-193 / `google.rpc.BadRequest`:

```protobuf
message BadRequest {
  repeated FieldViolation field_violations = 1;

  message FieldViolation {
    string field = 1;          // "contact_email" или "members[0].date_of_birth"
    string description = 2;
    string reason = 3;         // enum-like code: "INVALID_EMAIL"
  }
}
```

Это поднимается в `status.Status.Details` и сериализуется в JSON
автоматически.

---

## Rate limiting → HTTP 429

Антипаттерн (поле в payload):

```protobuf
message AttemptState {
  int32 attempts = 1;
  bool isRateLimited = 3;     // <-- HTTP 429
  bool isTempBlocked = 4;     // <-- HTTP 429
  int64 unblockTS = 2;        // <-- Retry-After header
}
```

Правильно — HTTP 429 + headers:

```text
HTTP/1.1 429 Too Many Requests
Retry-After: 60
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1734567890

Body:
{
  "code": "rate_limit.exceeded",
  "message": "Too many requests. Retry after 60 seconds.",
  "details": { "retryAfterSeconds": 60 }
}
```

`Retry-After` — стандартный HTTP header, поддерживается всеми клиентами и
ретраеры используют его автоматически. Это аналог `unblockTS` через стандарт.

---

## 5xx — серверные ошибки

Не дай 500 утечь клиенту с stacktrace. Стандартный handler:

- В логи: подробная ошибка со стеком и контекстом.
- Клиенту: generic message + machine code + traceId.

```json
{
  "code": "internal.error",
  "message": "Internal server error",
  "details": { "requestId": "abc-123", "traceId": "..." }
}
```

`requestId`/`traceId` нужны, чтобы клиент мог сообщить «у меня была ошибка с
этим id», и поддержка нашла её в логах.

---

## gRPC ↔ HTTP-коды

grpc-gateway мапит автоматически:

| gRPC code | HTTP |
|---|---|
| `OK` | 200 |
| `CANCELLED` | 499 |
| `UNKNOWN` | 500 |
| `INVALID_ARGUMENT` | 400 |
| `DEADLINE_EXCEEDED` | 504 |
| `NOT_FOUND` | 404 |
| `ALREADY_EXISTS` | 409 |
| `PERMISSION_DENIED` | 403 |
| `UNAUTHENTICATED` | 401 |
| `RESOURCE_EXHAUSTED` | 429 |
| `FAILED_PRECONDITION` | 400 (по умолчанию) |
| `ABORTED` | 409 |
| `OUT_OF_RANGE` | 400 |
| `UNIMPLEMENTED` | 501 |
| `INTERNAL` | 500 |
| `UNAVAILABLE` | 503 |
| `DATA_LOSS` | 500 |

Используй типизированные ошибки в gRPC:

```go
return nil, status.Errorf(codes.NotFound, "order.not_found: %s", orderID)
```

grpc-gateway отдаст 404. Можно прокидывать details:

```go
st := status.New(codes.InvalidArgument, "validation failed")
st, _ = st.WithDetails(&errdetails.BadRequest{
    FieldViolations: []*errdetails.BadRequest_FieldViolation{
        {Field: "email", Description: "invalid format"},
    },
})
return nil, st.Err()
```

В HTTP это отдастся как:

```json
{
  "code": 3,
  "message": "validation failed",
  "details": [{"@type": "type.googleapis.com/google.rpc.BadRequest", ...}]
}
```

---

## Кастомизация error response

Дефолтная JSON-схема grpc-gateway — `google.rpc.Status`. Если хочется
custom-формат — используется `runtime.WithErrorHandler`:

```go
mux := runtime.NewServeMux(
    runtime.WithErrorHandler(customErrorHandler),
)

func customErrorHandler(ctx context.Context, mux *runtime.ServeMux,
                       m runtime.Marshaler, w http.ResponseWriter,
                       r *http.Request, err error) {
    st, _ := status.FromError(err)
    httpStatus := runtime.HTTPStatusFromCode(st.Code())

    body := map[string]any{
        "code":    grpcCodeToAppCode(st.Code()),  // "order.not_found"
        "message": st.Message(),
        "details": extractDetails(st),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(httpStatus)
    json.NewEncoder(w).Encode(body)
}
```

---

## Сводка

| Правило | Как |
|---|---|
| Один канал ошибок | HTTP-код + единая `Error { code, message, details }`. Без `success bool`. |
| Машинно-читаемые коды | `<domain>.<reason>` (snake_case). |
| Rate limit | HTTP 429 + `Retry-After`. |
| Form validation | `Error.details.fieldViolations`. |
| 5xx | Generic message + requestId, без stacktrace. |
| gRPC ↔ HTTP | Через стандартный grpc-gateway маппинг. |
| Выбор кода | Карта кодов и разбор спорных пар — в [REST и семантика HTTP](../protocols/03-api-styles/01-rest-and-http-semantics.md). |

---

## Связанные документы

- [REST и семантика HTTP](../protocols/03-api-styles/01-rest-and-http-semantics.md) — какой код возвращать, спорные пары кодов, условные запросы.
- [01-principles.md](./01-principles.md)
- [03-http-methods.md](./03-http-methods.md) — идемпотентность ретраев.
- [06-cross-cutting-concerns.md](./06-cross-cutting-concerns.md) — `Idempotency-Key`.
- [13-references.md](./13-references.md) — RFC 9457, AIP-193.
