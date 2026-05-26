# 10. Структура proto-репозитория

Самая частая боль больших proto-кодовых баз — **дублирование message'ей между
external и internal сервисами** и обилие мапперов между ними. Этот файл — как
организовать proto-репозиторий так, чтобы дубли не плодились, а мапперы не
писались вручную.

## Корневой принцип

**Domain types** (`Order`, `Bundle`, `Payment`) определяются один раз и
переиспользуются.

**Request/response shapes** определяются один раз для пары операций (Get,
List, Create, Update, Delete) и переиспользуются между external и internal
сервисами.

**Различие external vs internal** живёт только в `service`-определении и
HTTP-аннотациях, не в payload-типах.

## Структура директорий

```
proto/
  v1/
    common/
      booking.proto         # domain types: Order, OrderItem, OrderStatus
      rec.proto             # Bundle, OfferBundle, SearchCriteria
      quiz.proto            # Quiz, QuizSession
      user.proto            # User, UserProfile
      money.proto           # Money, CurrencyInfo
      errors.proto          # Error, FieldViolation
    requests/
      booking_requests.proto    # Get/List/Create/Update/Delete request/response
      rec_requests.proto
      quiz_requests.proto
    services/
      external/
        booking.proto       # service + google.api.http annotations
        rec.proto
        quiz.proto
        user.proto
        admin_booking.proto # admin service в отдельном файле
      internal/
        booking.proto       # service без HTTP-аннотаций
        rec.proto
        provider/
          hotel.proto       # внутренние интеграции
    events/
      booking_events.proto  # Kafka/Pub-Sub события (BookingPaidEvent, etc.)
```

Логика:

- `common/` — типы, которые ни от чего не зависят (или зависят только от других
  common).
- `requests/` — request/response, которые используются в нескольких сервисах.
- `services/external/` — публичные HTTP-сервисы с `google.api.http`.
- `services/internal/` — gRPC-сервисы для service-mesh.
- `events/` — event-схемы для async-коммуникации.

## Один доменный тип, один файл

```protobuf
// common/booking.proto
syntax = "proto3";
package common.v1;

option go_package = "github.com/example/proto/gen/go/common/v1;commonv1";

import "google/protobuf/timestamp.proto";
import "google/api/field_behavior.proto";
import "common/money.proto";

message Order {
  string id = 1 [(google.api.field_behavior) = OUTPUT_ONLY];
  string shortId = 2 [(google.api.field_behavior) = OUTPUT_ONLY];
  string userId = 3 [(google.api.field_behavior) = OUTPUT_ONLY];  // server-set

  OrderStatus status = 4 [(google.api.field_behavior) = OUTPUT_ONLY];
  Money total = 5 [(google.api.field_behavior) = OUTPUT_ONLY];

  // Mutable fields
  string contactEmail = 10 [(google.api.field_behavior) = REQUIRED];
  string contactFirstName = 11;
  string contactLastName = 12;
  repeated OrderMember members = 13;

  google.protobuf.Timestamp createdAt = 20 [(google.api.field_behavior) = OUTPUT_ONLY];
  google.protobuf.Timestamp expiresAt = 21 [(google.api.field_behavior) = OUTPUT_ONLY];

  reserved 30 to 99;  // запас на будущие основные поля
  reserved 100 to 199; // запас на nested / addons
}

message OrderMember { ... }

enum OrderStatus {
  ORDER_STATUS_UNSPECIFIED = 0;
  ORDER_STATUS_NEW = 1;
  ORDER_STATUS_PAID = 2;
  ORDER_STATUS_CANCELED = 3;
}
```

`Order` здесь — единственный «правдивый» тип во всём API. И external HTTP, и
internal gRPC, и события — все ссылаются на него.

## Один request/response, один файл

```protobuf
// requests/booking_requests.proto
syntax = "proto3";
package requests.v1;

option go_package = "github.com/example/proto/gen/go/requests/v1;requestsv1";

import "google/protobuf/field_mask.proto";
import "google/api/field_behavior.proto";
import "common/booking.proto";

message GetOrderRequest {
  string orderId = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListOrdersRequest {
  int32 pageSize = 1;
  string pageToken = 2;

  // Filters
  repeated common.v1.OrderStatus status = 3;
}

message ListOrdersResponse {
  repeated common.v1.Order orders = 1;
  string nextPageToken = 2;
}

message CreateOrderRequest {
  common.v1.Order order = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateOrderRequest {
  common.v1.Order order = 1 [(google.api.field_behavior) = REQUIRED];
  google.protobuf.FieldMask updateMask = 2;
}

message CancelOrderRequest {
  string orderId = 1 [(google.api.field_behavior) = REQUIRED];
  string reason = 2;
}
```

Один набор — для всех сервисов, что работают с Order.

## External service

```protobuf
// services/external/booking.proto
syntax = "proto3";
package services.external.v1;

option go_package = "github.com/example/proto/gen/go/services/external/v1;externalbooking";

import "google/api/annotations.proto";
import "common/booking.proto";
import "requests/booking_requests.proto";

service BookingService {
  rpc GetOrder(requests.v1.GetOrderRequest) returns (common.v1.Order) {
    option (google.api.http) = { get: "/v1/orders/{orderId}" };
  }

  rpc ListOrders(requests.v1.ListOrdersRequest) returns (requests.v1.ListOrdersResponse) {
    option (google.api.http) = { get: "/v1/orders" };
  }

  rpc CreateOrder(requests.v1.CreateOrderRequest) returns (common.v1.Order) {
    option (google.api.http) = {
      post: "/v1/orders"
      body: "order"
    };
  }

  rpc UpdateOrder(requests.v1.UpdateOrderRequest) returns (common.v1.Order) {
    option (google.api.http) = {
      patch: "/v1/orders/{order.id}"
      body: "order"
    };
  }

  rpc CancelOrder(requests.v1.CancelOrderRequest) returns (common.v1.Order) {
    option (google.api.http) = {
      post: "/v1/orders/{orderId}:cancel"
      body: "*"
    };
  }
}
```

Никаких своих request/response. Только импорт `requests/` + `common/` + HTTP-аннотации.

## Internal service

```protobuf
// services/internal/booking.proto
syntax = "proto3";
package services.internal.v1;

option go_package = "github.com/example/proto/gen/go/services/internal/v1;internalbooking";

import "common/booking.proto";
import "requests/booking_requests.proto";

service BookingInternalService {
  // Те же rpc, что в external — для тех же запросов из других сервисов.
  rpc GetOrder(requests.v1.GetOrderRequest) returns (common.v1.Order);
  rpc ListOrders(requests.v1.ListOrdersRequest) returns (requests.v1.ListOrdersResponse);
  rpc CreateOrder(requests.v1.CreateOrderRequest) returns (common.v1.Order);
  rpc UpdateOrder(requests.v1.UpdateOrderRequest) returns (common.v1.Order);
  rpc CancelOrder(requests.v1.CancelOrderRequest) returns (common.v1.Order);

  // Плюс внутренние rpc, которые наружу не торчат:
  rpc ApplyRefund(ApplyRefundRequest) returns (common.v1.Order);
  rpc GetInternalAuditLog(GetAuditLogRequest) returns (AuditLogResponse);
}

message ApplyRefundRequest {
  string orderId = 1;
  int64 amountMinor = 2;
  string reason = 3;
}
// ... остальные internal-only типы
```

`BookingInternalService` использует **те же** request-типы. Различие:

- Нет HTTP-аннотаций (это gRPC-only в mesh).
- Есть дополнительные rpc, которые недоступны через external.

**Никаких мапперов между external и internal.** Один `common.v1.Order` — везде
один и тот же.

## Где живёт userId

Не в payload. Через gateway → gRPC metadata → ctx:

```go
// gateway middleware
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims := parseJWT(r.Header.Get("Authorization"))
        md := metadata.Pairs(
            "user-id", claims.UserID,
            "tenant-id", claims.TenantID,
        )
        ctx := metadata.NewOutgoingContext(r.Context(), md)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// handler
func (s *Server) GetOrder(ctx context.Context, req *requestsv1.GetOrderRequest) (*commonv1.Order, error) {
    userID := auth.UserIDFromContext(ctx)  // из metadata
    return s.repo.GetOrder(ctx, userID, req.OrderId)
}
```

Поле `Order.userId` остаётся в типе для consistency, но помечено `OUTPUT_ONLY` —
клиент его не присылает.

## additional_bindings для нескольких HTTP-форм

Один rpc может иметь несколько HTTP-маппингов:

```protobuf
rpc GetOrder(GetOrderRequest) returns (Order) {
  option (google.api.http) = {
    get: "/v1/orders/{orderId}"
    additional_bindings {
      get: "/v1/admin/orders/{orderId}"
    }
  };
}
```

Один rpc, два пути. Middleware на gateway различает по path: для `/v1/admin/`
проверяется admin-роль, для `/v1/orders/` — обычная JWT-auth.

Не нужно дублировать `GetOrder` в `BookingService` и `AdminBookingService`,
если логика та же.

## Когда дублирование message'ей всё-таки оправдано

Не всегда «один тип — на всё». Иногда нужна явная anti-corruption boundary:

### 1. Жирная разница в данных

Internal Payment service знает поля, которые **нельзя** отдавать наружу даже
случайно:

```protobuf
// common/payment_internal.proto
message InternalPayment {
  Payment basic = 1;            // публичные поля

  string stripeAccountId = 10;       // секретное
  int64 internalRiskScore = 11;      // секретное
  ProcessorFeesBreakdown fees = 12;  // секретное
}
```

В external — `common.v1.Payment` без этих полей. В internal —
`InternalPayment`, который содержит `Payment` + sensitive. Mapper между ними —
это **явная** boundary, защищающая от утечки.

### 2. Версионная развязка

External `/v1/` застрял (клиенты не обновляются), internal эволюционирует:

```protobuf
// public-facing v1 (frozen)
message PublicOrderV1 { ... }

// internal evolves freely
message Order { ... }

// adapter
PublicOrderV1 ConvertToPublic(Order o) { ... }
```

Mapper становится трансляцией между поколениями.

### 3. PII / GDPR-фильтрация

Для unauthenticated / low-trust клиентов — sanitized response:

```protobuf
// internal
message FullUser { ...полный профиль с PII... }

// public response
message PublicUser { ...только публичные поля... }
```

### Правило для маппинга

- Mapper кладётся в **один** edge-слой (BFF/gateway), не в каждом сервисе.
- Mapper **генерируется** (codegen), а не пишется руками.
- Mapper покрыт тестами на field-coverage (если в Internal появится новое
  PII-поле, тест должен предупредить).

## Codegen для мапперов

В Go популярные варианты:

- **jinzhu/copier:** структурное копирование по тегам/именам.

  ```go
  var publicUser PublicUser
  copier.Copy(&publicUser, &fullUser)
  // плюс ручное обнуление PII
  ```

- **goverter (jmattheis/goverter):** генерирует typed Go-функции маппинга по
  определениям интерфейсов.

  ```go
  // @goverter:converter
  type FullToPublic interface {
      Convert(source *FullUser) *PublicUser
  }
  ```

- **mapstructure (HashiCorp):** для динамических случаев (map → struct).

- **Свой buf-плагин:** на 100 строк — генерация map-функций по proto-аннотациям.

Главное — **не вручную**. 50 строк `dst.Foo = src.Foo` в каждом обработчике —
гарантированный источник багов.

## События и async

Для Kafka / Pub-Sub / Cloud Events схемы тоже proto. Жить должны параллельно:

```
proto/v1/events/
  booking_events.proto      # BookingCreatedEvent, BookingPaidEvent, BookingCanceledEvent
  payment_events.proto      # PaymentSucceededEvent, PaymentFailedEvent
```

И **используют те же domain-типы** из `common/`:

```protobuf
import "common/booking.proto";

message BookingPaidEvent {
  common.v1.Order order = 1;
  google.protobuf.Timestamp paidAt = 2;
}
```

Не нужно делать `OrderInEvent` отдельно от `Order`. Если действительно нужно
поднять консистентность (event имеет полный snapshot, а API — нет) — это
дополнительные поля внутри события, а не отдельный тип ордера.

## Antipattern: «общий types.proto на всё»

```
common/
  types.proto    # 5000 строк со всеми типами
```

Один файл с десятками типов = постоянные конфликты при merge, медленный
buf-compile, плохая навигация. Разбивай по доменам:
`booking.proto`, `rec.proto`, `quiz.proto`, `money.proto`.

## Antipattern: «один сервис = один файл со всеми типами»

```
services/external/booking.proto:
  service BookingService { ... }
  message Order { ... }       # <-- domain-тип внутри файла сервиса!
  message GetOrderRequest { ... }
  message ListOrdersResponse { ... }
```

Тогда `Order` живёт в `services/external/`, и не может быть импортирован в
`services/internal/` без circular dependency. Domain-типы — в `common/`,
request-shapes — в `requests/`.

## Чек-лист для нового proto-файла

1. Это domain-тип? → `common/<domain>.proto`.
2. Это request/response, который будет в нескольких сервисах? → `requests/<domain>_requests.proto`.
3. Это service-определение? → `services/external/` или `services/internal/`.
4. Это событие? → `events/<domain>_events.proto`.
5. `userId`, `tenantId`, `locale` есть в новых message'ах? → пометить
   `OUTPUT_ONLY` или (лучше) убрать.
6. Update*Request с повторением полей? → переделать на `FieldMask` (AIP-134).
7. Запустить `buf lint` локально перед PR.

## Связанные документы

- [01-principles.md](./01-principles.md) — три аксиомы.
- [05-payloads-and-types.md](./05-payloads-and-types.md) — FieldMask,
  field_behavior.
- [06-cross-cutting-concerns.md](./06-cross-cutting-concerns.md) — почему
  userId не в payload.
- [11-tooling.md](./11-tooling.md) — buf, codegen.
- [12-greenfield-template.md](./12-greenfield-template.md) — полный пример
  Payment-домена.
