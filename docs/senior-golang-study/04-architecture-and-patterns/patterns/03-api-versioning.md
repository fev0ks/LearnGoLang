# API Versioning и обратная совместимость

Как развивать API без поломки клиентов. Актуально для HTTP REST, gRPC и event-driven систем.

## Содержание

- [Стратегии версионирования](#стратегии-версионирования)
- [Backward compatibility: что ломает, что нет](#backward-compatibility-что-ломает-что-нет)
- [REST API versioning](#rest-api-versioning)
- [gRPC и Protobuf](#grpc-и-protobuf)
- [Event schema versioning](#event-schema-versioning)
- [Deprecation lifecycle](#deprecation-lifecycle)
- [Interview-ready answer](#interview-ready-answer)

---

## Стратегии версионирования

| Стратегия | Как выглядит | Плюсы | Минусы |
|---|---|---|---|
| URL path | `/api/v1/users`, `/api/v2/users` | Явно, кешируется | Дублирование кода, URL загрязнение |
| Query param | `/api/users?version=2` | Просто добавить | Легко забыть, плохо кешируется |
| Header | `Accept: application/vnd.myapi.v2+json` | Чистые URL | Менее видимо, сложнее тестировать |
| Content negotiation | `Accept: application/json; version=2` | RESTful | Сложная реализация |
| No versioning (additive only) | Никогда не ломать контракт | Нет версий = нет проблем | Требует строгой backward compat |

**Рекомендация для большинства API:** URL path versioning (`/v1/`, `/v2/`) — явно, просто, хорошо кешируется CDN.

---

## Backward compatibility: что ломает, что нет

### REST / JSON

**Безопасные изменения (non-breaking):**

```json
// Добавить новое поле в ответ — OK
// v1 клиент просто проигнорирует его
{
  "id": 123,
  "name": "Alice",
  "email": "alice@example.com",  // новое поле — безопасно
  "created_at": "2024-01-01"     // новое поле — безопасно
}
```

```
✓ Добавить новое опциональное поле в ответ
✓ Добавить новый endpoint
✓ Добавить новый опциональный query параметр
✓ Расширить enum новым значением (если клиент делает default case)
✓ Изменить внутреннюю логику без изменения контракта
```

**Ломающие изменения (breaking):**

```
✗ Удалить или переименовать поле
✗ Изменить тип поля (string → int)
✗ Сделать опциональное поле обязательным
✗ Изменить семантику существующего поля
✗ Изменить HTTP статус-код для существующего кейса
✗ Удалить значение из enum
✗ Изменить структуру URL
```

---

### Protobuf / gRPC

Protobuf специально разработан для forward и backward compatibility при соблюдении правил.

**Безопасные изменения:**

```protobuf
// v1
message User {
  int64  id   = 1;
  string name = 2;
}

// v2 — safe changes
message User {
  int64  id    = 1;
  string name  = 2;
  string email = 3;  // ✓ новое поле с новым номером
  // int32 age = 4;  // ✓ добавить новое поле
}
```

**Ломающие изменения:**

```protobuf
// НИКОГДА не делать:
message User {
  int64  id    = 1;
  // string name = 2;  ✗ удалено — старые клиенты сломаются
  string email = 2;    // ✗ переиспользование номера поля — КАТАСТРОФА
  string name  = 3;    // ✗ изменён номер поля
}

// Правильно при "удалении":
message User {
  int64  id    = 1;
  reserved 2;          // ✓ зарезервировать номер
  reserved "name";     // ✓ зарезервировать имя
  string email = 3;
}
```

**Правила Protobuf:**
- Номера полей неизменны навсегда
- Удалённые номера → `reserved`
- Изменить тип поля нельзя (кроме совместимых: int32 ↔ int64 ↔ uint32 ...)
- Добавлять новые поля всегда можно

---

## REST API versioning

### Параллельные версии

```go
// router.go
v1 := router.PathPrefix("/api/v1").Subrouter()
v1.HandleFunc("/users/{id}", v1Handler.GetUser)
v1.HandleFunc("/orders", v1Handler.CreateOrder)

v2 := router.PathPrefix("/api/v2").Subrouter()
v2.HandleFunc("/users/{id}", v2Handler.GetUser)  // новая версия
v2.HandleFunc("/orders", v1Handler.CreateOrder)  // v1 повторно используется если не менялось
```

**Версионирование через адаптеры (избегать дублирования):**

```go
// domain модель одна
type Order struct {
    ID         OrderID
    CustomerID UserID
    Items      []OrderItem
    TotalPrice Money
    Status     OrderStatus
    CreatedAt  time.Time
}

// v1 response: старый формат
type OrderResponseV1 struct {
    ID       string `json:"id"`
    Customer string `json:"customer_id"`  // просто ID
    Total    int64  `json:"total_cents"`  // в центах
    Status   string `json:"status"`
}

// v2 response: новый формат
type OrderResponseV2 struct {
    ID         string      `json:"id"`
    Customer   CustomerDTO `json:"customer"`  // объект с деталями
    Total      MoneyDTO    `json:"total"`     // { amount, currency }
    Status     string      `json:"status"`
    CreatedAt  time.Time   `json:"created_at"`
}
```

### API Gateway versioning

```
Client v1 ──► /api/v1/* ──► API Gateway ──► OrderService (внутренний, без версии)
                                               ↑
Client v2 ──► /api/v2/* ──► API Gateway ──────┘ (с трансформацией payload)
```

Версионирование на уровне Gateway позволяет поддерживать несколько внешних версий с одним внутренним сервисом.

---

## gRPC и Protobuf

### Package versioning

```protobuf
// orders/v1/orders.proto
syntax = "proto3";
package orders.v1;
option go_package = "github.com/myco/api/orders/v1;ordersv1";

service OrderService {
  rpc CreateOrder(CreateOrderRequest) returns (CreateOrderResponse);
  rpc GetOrder(GetOrderRequest) returns (GetOrderResponse);
}

// orders/v2/orders.proto  ← новая версия при breaking change
syntax = "proto3";
package orders.v2;
option go_package = "github.com/myco/api/orders/v2;ordersv2";

service OrderService {
  rpc CreateOrder(CreateOrderRequest) returns (CreateOrderResponse);
  rpc GetOrder(GetOrderRequest) returns (GetOrderResponse);
  rpc BatchGetOrders(BatchGetOrdersRequest) returns (BatchGetOrdersResponse);  // новое
}
```

### gRPC Server reflection + versioning

```go
// Сервер поддерживает обе версии
grpcServer := grpc.NewServer()
ordersv1.RegisterOrderServiceServer(grpcServer, v1Handler)
ordersv2.RegisterOrderServiceServer(grpcServer, v2Handler)
reflection.Register(grpcServer)  // для grpcurl и tooling
```

---

## Event schema versioning

События в Kafka/брокере — особый случай: потребители могут читать старые сообщения из retention.

### Schema Registry (Avro/Protobuf)

```
Producer ──► Schema Registry ──► Kafka
                  ↑
Consumer ◄────────┘ (валидирует schema при чтении)
```

**Совместимость схем (Confluent Schema Registry):**

| Режим | Правило | Когда использовать |
|---|---|---|
| BACKWARD | Новая схема читает старые данные | Консьюмеры обновляются первыми |
| FORWARD | Старая схема читает новые данные | Продюсеры обновляются первыми |
| FULL | Оба направления | Максимальная безопасность |
| NONE | Без проверок | Dev/staging только |

### Envelope pattern (без Schema Registry)

```go
// Событие с версией в payload
type Event struct {
    ID        string          `json:"id"`
    Type      string          `json:"type"`       // "order.created"
    Version   string          `json:"version"`    // "v1", "v2"
    OccurredAt time.Time      `json:"occurred_at"`
    Payload   json.RawMessage `json:"payload"`    // версионированный payload
}

// Consumer с multi-version handling
func (h *Handler) Handle(ctx context.Context, event Event) error {
    switch event.Type + "@" + event.Version {
    case "order.created@v1":
        var p OrderCreatedV1
        json.Unmarshal(event.Payload, &p)
        return h.handleV1(ctx, p)
    case "order.created@v2":
        var p OrderCreatedV2
        json.Unmarshal(event.Payload, &p)
        return h.handleV2(ctx, p)
    default:
        // unknown version — log и skip (не fatal)
        return nil
    }
}
```

---

## Deprecation lifecycle

```
Timeline:

v1 launch ──► v2 launch ──► v1 deprecation ──► v1 sunset ──► v1 removed
    │               │               │                │
    │               │         Announce +         Breaking
    │               │         Deprecation         change!
    │               │         header
    │               │
    │         Migrate clients
    │         during overlap
    │
 Clients use v1
```

**HTTP Deprecation header (RFC 8594):**

```http
HTTP/1.1 200 OK
Deprecation: true
Sunset: Sat, 01 Jun 2025 00:00:00 GMT
Link: <https://api.example.com/v2/users>; rel="successor-version"
```

**Минимальный overlap period:**
- Внутренние API: 1-3 месяца
- Публичные API: 6-12 месяцев
- Крупные платформы (AWS, Stripe): 1-2 года

**Monitoring deprecation usage:**

```go
// Счётчик использования deprecated endpoint
func DeprecationMiddleware(version, sunset string) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Deprecation", "true")
            w.Header().Set("Sunset", sunset)
            metrics.Inc("api.deprecated.calls", "version", version, "path", r.URL.Path)
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## Consumer-driven contract testing

Проблема микросервисов: как убедиться что изменение не сломает потребителей?

```
Стандартный подход (ненадёжный):
  Service A меняет API → деплоит → Service B ломается в production

Contract testing (Pact):
  1. Service B (consumer) пишет contract: "я ожидаю такой ответ"
  2. Contract публикуется в Pact Broker
  3. Service A (provider) верифицирует свой API против контрактов
  4. CI: Service A не деплоится если нарушает контракт

  Provider ──verify──► Pact Broker ◄──publish── Consumer
```

```go
// Pact consumer test (в Service B)
func TestOrderServiceContract(t *testing.T) {
    pact := dsl.Pact{Consumer: "service-b", Provider: "order-service"}
    
    pact.AddInteraction().
        Given("order 123 exists").
        UponReceiving("a request for order 123").
        WithRequest(dsl.Request{
            Method: "GET",
            Path:   dsl.String("/api/v1/orders/123"),
        }).
        WillRespondWith(dsl.Response{
            Status: 200,
            Body: dsl.Like(map[string]interface{}{
                "id":     "123",
                "status": "pending",
            }),
        })
    // ...
}
```

---

## Interview-ready answer

> "API versioning — это баланс между стабильностью для клиентов и скоростью развития. Для REST я использую URL path versioning (`/v1/`, `/v2/`) — явно и хорошо кешируется. Ключевое правило: backward-compatible изменения (новые опциональные поля, новые endpoints) не требуют новой версии — просто добавляй. Новая версия нужна только при breaking changes.
>
> Для Protobuf: номера полей неизменны навсегда, удалённые номера → `reserved`. При соблюдении этих правил backward compatibility встроена в формат.
>
> Для событий в Kafka: версия в payload + multi-version handler на стороне consumer. Schema Registry если нужна централизованная валидация.
>
> Deprecation: объявить, добавить header с датой sunset, мониторить использование deprecated endpoints, дать клиентам достаточно времени для миграции."
