# 12. Greenfield-шаблон

Шаблон нового домена с нуля. Применяет все принципы из предыдущих файлов.
Берём пример «Payment» — он закрывает все интересные сценарии (Create, Update
через FieldMask, custom actions, idempotency, internal-only поля,
sub-resources).

## Шаг 1. Domain-тип

```protobuf
// proto/v1/common/payment.proto
syntax = "proto3";
package common.v1;

option go_package = "github.com/example/proto/gen/go/common/v1;commonv1";

import "google/protobuf/timestamp.proto";
import "google/api/field_behavior.proto";
import "common/money.proto";

message Payment {
  // Глобально уникальный ID, сервер генерирует.
  string id = 1 [(google.api.field_behavior) = OUTPUT_ONLY];

  // FK на ордер. Immutable — нельзя поменять привязку.
  string orderId = 2 [(google.api.field_behavior) = IMMUTABLE];

  // Сумма платежа.
  Money amount = 3 [(google.api.field_behavior) = REQUIRED];

  // Статус, сервер управляет.
  PaymentStatus status = 4 [(google.api.field_behavior) = OUTPUT_ONLY];

  // Метод оплаты.
  oneof method {
    StripeMethod stripe = 10;
    PaypalMethod paypal = 11;
  }

  // Описание для клиента / выписки.
  string description = 20;

  // Timestamps.
  google.protobuf.Timestamp createdAt = 30 [(google.api.field_behavior) = OUTPUT_ONLY];
  google.protobuf.Timestamp succeededAt = 31 [(google.api.field_behavior) = OUTPUT_ONLY];
  google.protobuf.Timestamp canceledAt = 32 [(google.api.field_behavior) = OUTPUT_ONLY];

  reserved 100 to 199;  // запас под будущие поля
}

enum PaymentStatus {
  PAYMENT_STATUS_UNSPECIFIED = 0;
  PAYMENT_STATUS_PENDING = 1;
  PAYMENT_STATUS_PROCESSING = 2;
  PAYMENT_STATUS_SUCCEEDED = 3;
  PAYMENT_STATUS_FAILED = 4;
  PAYMENT_STATUS_CANCELED = 5;
  PAYMENT_STATUS_REFUNDED = 6;
}

message StripeMethod {
  string paymentMethodId = 1 [(google.api.field_behavior) = REQUIRED];  // pm_xxx
  string clientSecret = 2 [(google.api.field_behavior) = OUTPUT_ONLY];  // для 3DS
}

message PaypalMethod {
  string token = 1 [(google.api.field_behavior) = REQUIRED];
}
```

Замечаний:

- `OUTPUT_ONLY` на server-managed полях — клиент не видит их в request schema.
- `IMMUTABLE` на `orderId` — можно при create, нельзя в update.
- `oneof` для метода — расширяется новыми вариантами без breaking change.
- Enum в canonical стиле.
- `Timestamp` для всех момент-полей.
- `Money` — отдельный тип, никаких `int64 amount`.

## Шаг 2. Request/response

```protobuf
// proto/v1/requests/payment_requests.proto
syntax = "proto3";
package requests.v1;

option go_package = "github.com/example/proto/gen/go/requests/v1;requestsv1";

import "google/protobuf/field_mask.proto";
import "google/api/field_behavior.proto";
import "common/payment.proto";

message GetPaymentRequest {
  string paymentId = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListPaymentsRequest {
  int32 pageSize = 1;
  string pageToken = 2;

  // Фильтры
  string orderId = 3;
  repeated common.v1.PaymentStatus status = 4;
}

message ListPaymentsResponse {
  repeated common.v1.Payment payments = 1;
  string nextPageToken = 2;
}

message CreatePaymentRequest {
  // Idempotency-Key передаётся header'ом, не полем.
  common.v1.Payment payment = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdatePaymentRequest {
  common.v1.Payment payment = 1 [(google.api.field_behavior) = REQUIRED];
  google.protobuf.FieldMask updateMask = 2;
}

message CancelPaymentRequest {
  string paymentId = 1 [(google.api.field_behavior) = REQUIRED];
  string reason = 2;
}

message ConfirmPaymentRequest {
  string paymentId = 1 [(google.api.field_behavior) = REQUIRED];
}

message RefundPaymentRequest {
  string paymentId = 1 [(google.api.field_behavior) = REQUIRED];
  // Если не указано — full refund.
  common.v1.Money amount = 2;
  string reason = 3;
}
```

Замечаний:

- Никаких `userId` в request'ах. Не нужен.
- `Idempotency-Key` — header, не поле.
- `UpdatePaymentRequest` использует FieldMask — никаких 15 повторяющихся полей.

## Шаг 3. External service

```protobuf
// proto/v1/services/external/payment.proto
syntax = "proto3";
package services.external.v1;

option go_package = "github.com/example/proto/gen/go/services/external/v1;externalpayment";

import "google/api/annotations.proto";
import "common/payment.proto";
import "requests/payment_requests.proto";

service PaymentService {
  // Standard CRUD.

  rpc GetPayment(requests.v1.GetPaymentRequest) returns (common.v1.Payment) {
    option (google.api.http) = { get: "/v1/payments/{paymentId}" };
  }

  rpc ListPayments(requests.v1.ListPaymentsRequest) returns (requests.v1.ListPaymentsResponse) {
    option (google.api.http) = { get: "/v1/payments" };
  }

  rpc CreatePayment(requests.v1.CreatePaymentRequest) returns (common.v1.Payment) {
    option (google.api.http) = {
      post: "/v1/payments"
      body: "payment"
    };
  }

  rpc UpdatePayment(requests.v1.UpdatePaymentRequest) returns (common.v1.Payment) {
    option (google.api.http) = {
      patch: "/v1/payments/{payment.id}"
      body: "payment"
    };
  }

  // Custom actions.

  rpc ConfirmPayment(requests.v1.ConfirmPaymentRequest) returns (common.v1.Payment) {
    option (google.api.http) = {
      post: "/v1/payments/{paymentId}:confirm"
      body: "*"
    };
  }

  rpc CancelPayment(requests.v1.CancelPaymentRequest) returns (common.v1.Payment) {
    option (google.api.http) = {
      post: "/v1/payments/{paymentId}:cancel"
      body: "*"
    };
  }

  rpc RefundPayment(requests.v1.RefundPaymentRequest) returns (common.v1.Payment) {
    option (google.api.http) = {
      post: "/v1/payments/{paymentId}:refund"
      body: "*"
    };
  }
}
```

Замечаний:

- `paymentId` в path, не в query.
- Custom actions `:confirm`, `:cancel`, `:refund` — потому что их семантика не
  CRUD.
- `UpdatePayment` — PATCH с FieldMask, на path `{payment.id}` (id ссылается на
  поле внутри body).
- Каждый rpc возвращает `common.v1.Payment` — единый ресурс.

## Шаг 4. Internal service

```protobuf
// proto/v1/services/internal/payment.proto
syntax = "proto3";
package services.internal.v1;

option go_package = "github.com/example/proto/gen/go/services/internal/v1;internalpayment";

import "common/payment.proto";
import "requests/payment_requests.proto";

service PaymentInternalService {
  // Те же rpc, что в external — для использования из других mesh-сервисов.
  rpc GetPayment(requests.v1.GetPaymentRequest) returns (common.v1.Payment);
  rpc ListPayments(requests.v1.ListPaymentsRequest) returns (requests.v1.ListPaymentsResponse);
  rpc CreatePayment(requests.v1.CreatePaymentRequest) returns (common.v1.Payment);

  // Internal-only operations.
  rpc HandleWebhook(HandleWebhookRequest) returns (HandleWebhookResponse);
  rpc GetInternalAuditLog(GetAuditLogRequest) returns (AuditLogResponse);
  rpc RecordProcessorFee(RecordProcessorFeeRequest) returns (common.v1.Payment);
}

message HandleWebhookRequest {
  bytes verifiedPayload = 1;     // от Stripe, уже верифицировано на edge
  string source = 2;             // "stripe" | "paypal"
}
message HandleWebhookResponse { string eventId = 1; }

message GetAuditLogRequest { string paymentId = 1; int32 pageSize = 2; string pageToken = 3; }
message AuditLogResponse { repeated AuditLogEntry entries = 1; string nextPageToken = 2; }
message AuditLogEntry { /* ... */ }

message RecordProcessorFeeRequest {
  string paymentId = 1;
  common.v1.Money fee = 2;
  string processorName = 3;
}
```

Замечаний:

- Использует те же `requests.v1.*` типы. Никаких мапперов.
- Добавляет internal-only rpc, которых нет в external.
- `HandleWebhook` принимает `verifiedPayload` — уже верифицированный на edge.

## Шаг 5. Events

```protobuf
// proto/v1/events/payment_events.proto
syntax = "proto3";
package events.v1;

option go_package = "github.com/example/proto/gen/go/events/v1;eventsv1";

import "google/protobuf/timestamp.proto";
import "common/payment.proto";

message PaymentSucceededEvent {
  common.v1.Payment payment = 1;
  google.protobuf.Timestamp eventTime = 2;
  string idempotencyKey = 3;   // для consumer dedup
}

message PaymentFailedEvent {
  common.v1.Payment payment = 1;
  string failureCode = 2;       // "card_declined", "insufficient_funds"
  string failureMessage = 3;
  google.protobuf.Timestamp eventTime = 4;
  string idempotencyKey = 5;
}

message PaymentRefundedEvent {
  common.v1.Payment payment = 1;
  common.v1.Money refundedAmount = 2;
  google.protobuf.Timestamp eventTime = 3;
  string idempotencyKey = 4;
}
```

Замечаний:

- Используется `common.v1.Payment` — событие содержит полный snapshot.
- `idempotencyKey` — для at-least-once семантики Kafka/PubSub (consumer dedup).
- Events изолированы от service-определений (не зависят от gRPC).

## Шаг 6. Handler (Go)

```go
// internal/handlers/payment.go
package handlers

import (
    "context"

    externalv1 "github.com/example/proto/gen/go/services/external/v1"
    commonv1 "github.com/example/proto/gen/go/common/v1"
    requestsv1 "github.com/example/proto/gen/go/requests/v1"

    "github.com/example/myservice/internal/auth"
    "github.com/example/myservice/internal/service"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type PaymentHandler struct {
    externalv1.UnimplementedPaymentServiceServer
    svc *service.PaymentService
}

func (h *PaymentHandler) GetPayment(ctx context.Context, req *requestsv1.GetPaymentRequest) (*commonv1.Payment, error) {
    userID := auth.UserIDFromContext(ctx)  // из metadata, не из req
    if userID == "" {
        return nil, status.Error(codes.Unauthenticated, "auth.required")
    }

    p, err := h.svc.GetPayment(ctx, userID, req.PaymentId)
    if err != nil {
        if errors.Is(err, service.ErrNotFound) {
            return nil, status.Errorf(codes.NotFound, "payment.not_found: %s", req.PaymentId)
        }
        return nil, status.Errorf(codes.Internal, "internal.error")
    }
    return paymentToProto(p), nil
}

func (h *PaymentHandler) CreatePayment(ctx context.Context, req *requestsv1.CreatePaymentRequest) (*commonv1.Payment, error) {
    userID := auth.UserIDFromContext(ctx)
    idempotencyKey := auth.IdempotencyKeyFromContext(ctx)  // из header'а

    p, err := h.svc.CreatePayment(ctx, userID, idempotencyKey, paymentFromProto(req.Payment))
    if err != nil {
        return nil, mapServiceError(err)
    }
    return paymentToProto(p), nil
}
```

## Чек-лист для нового домена

Когда добавляешь новый домен `X`:

- [ ] `common/x.proto` — domain-тип с field_behavior аннотациями.
- [ ] `requests/x_requests.proto` — Get/List/Create/Update/[custom] requests.
- [ ] `services/external/x.proto` — public service с `google.api.http`.
- [ ] `services/internal/x.proto` — internal service + extra internal rpc.
- [ ] `events/x_events.proto` — async events, если есть.
- [ ] Field_behavior расставлены (OUTPUT_ONLY, REQUIRED, IMMUTABLE).
- [ ] Enum с `_UNSPECIFIED = 0` и canonical naming.
- [ ] Timestamps — `google.protobuf.Timestamp`, не int64.
- [ ] Money — `Money { currencyCode, amountMinor }`, не float.
- [ ] Update — через FieldMask, не отдельным `UpdateXRequest` с полями.
- [ ] Никаких `userId`/`tenantId` в request'ах. Через metadata.
- [ ] Custom actions — `:verb`, не `/verb-segment` в path.
- [ ] ID в path, фильтры в query.
- [ ] Pagination на каждой коллекции.
- [ ] Idempotency-Key на платежах и других опасных мутациях.
- [ ] `buf lint` и `buf breaking` зелёные.

## Антипример того же домена

Чтобы было контрастно — как **не** надо:

```protobuf
// плохой пример
message CreatePaymentRequest {
  string userId = 1;              // <-- из payload!
  string orderId = 2;
  int64 amount = 3;               // <-- int64 без currency и без Minor
  string currencyCode = 4;
  string paymentMethod = 5;       // <-- string вместо oneof
  string idempotencyKey = 6;      // <-- в payload вместо header
  string lang = 7;                // <-- locale в payload
}

message UpdatePaymentRequest {
  string id = 1;
  string status = 2;              // <-- string вместо enum
  int64 amount = 3;               // <-- повтор всех полей
  string currencyCode = 4;
  // ... ещё 10 полей повтором
}

service PaymentService {
  rpc CreatePayment(CreatePaymentRequest) returns (CreatePaymentResponse) {
    option (google.api.http) = {
      post: "/v1/payment/create"        // <-- глагол в URL
      body: "*"
    };
  }
  rpc UpdatePayment(UpdatePaymentRequest) returns (UpdatePaymentResponse) {
    option (google.api.http) = {
      post: "/v1/payment/update"        // <-- POST вместо PATCH, глагол в URL
      body: "*"
    };
  }
  rpc GetPayment(GetPaymentRequest) returns (GetPaymentResponse) {
    option (google.api.http) = {
      get: "/v1/payment?paymentId={paymentId}"   // <-- id в query, singular
    };
  }
  rpc ConfirmPayment(ConfirmPaymentRequest) returns (ConfirmPaymentResponse) {
    option (google.api.http) = {
      post: "/v1/payment/{paymentId}/confirm"   // <-- глагол сегментом, не :confirm
      body: "*"
    };
  }
}
```

Каждая строка нарушает какое-то правило из этого раздела. Получившийся API
будет тяжело документировать, поддерживать, версионировать и валидировать.

## Связанные документы

- Все предыдущие файлы раздела применяются здесь.
- [10-protobuf-repo-layout.md](./10-protobuf-repo-layout.md) — где какой файл
  лежит.
- [11-tooling.md](./11-tooling.md) — что проверяет CI.
