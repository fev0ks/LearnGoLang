# grpc-gateway

`grpc-ecosystem/grpc-gateway` — protoc плагин, который генерирует HTTP/JSON reverse-proxy поверх gRPC сервиса. Позволяет один сервис отдавать и gRPC, и REST из одного `.proto` определения.

## Содержание

- [Как работает](#как-работает)
- [google.api.http аннотации](#googleapihttp-аннотации)
- [Кодогенерация с buf](#кодогенерация-с-buf)
- [Запуск gateway сервера](#запуск-gateway-сервера)
- [Маппинг HTTP ↔ gRPC](#маппинг-http--grpc)
- [OpenAPI генерация](#openapi-генерация)
- [grpc-gateway vs connect-go](#grpc-gateway-vs-connect-go)
- [Антипаттерны](#антипаттерны)

---

## Как работает

grpc-gateway генерирует Go-код который:
1. Поднимает HTTP/1.1 сервер (обычный `net/http`)
2. Принимает REST запросы
3. Транслирует их в gRPC вызовы к upstream gRPC серверу
4. Возвращает ответ как JSON

```
браузер/curl
    │ HTTP/1.1 + JSON
    ▼
grpc-gateway (сгенерированный reverse-proxy)
    │ gRPC + protobuf (HTTP/2)
    ▼
gRPC-сервер
```

Два варианта деплоя:
- **Отдельный процесс**: gateway и gRPC сервер — разные бинарники (sidecar pattern)
- **In-process**: оба в одном бинарнике на разных портах (или на одном через `cmux`)

---

## google.api.http аннотации

Маппинг HTTP ↔ gRPC задаётся прямо в `.proto` через аннотации:

```protobuf
syntax = "proto3";

package user.v1;

import "google/api/annotations.proto";

service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse) {
    option (google.api.http) = {
      get: "/v1/users/{id}"
    };
  }

  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse) {
    option (google.api.http) = {
      post: "/v1/users"
      body: "*"   // весь body маппится на request message
    };
  }

  rpc UpdateUser(UpdateUserRequest) returns (UpdateUserResponse) {
    option (google.api.http) = {
      put: "/v1/users/{id}"
      body: "user"   // только поле user из body
      additional_bindings {
        patch: "/v1/users/{id}"
        body: "user"
      }
    };
  }

  rpc DeleteUser(DeleteUserRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/v1/users/{id}"
    };
  }

  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse) {
    option (google.api.http) = {
      get: "/v1/users"
      // query params: ?page_size=10&page_token=abc
    };
  }
}
```

Поля request message которые не попали в path и не в body — автоматически становятся query params.

```protobuf
message ListUsersRequest {
  int32  page_size  = 1;   // → ?page_size=20
  string page_token = 2;   // → ?page_token=xxx
  string filter     = 3;   // → ?filter=name:alice
}
```

---

## Кодогенерация с buf

Добавить в `buf.yaml` зависимость:
```yaml
deps:
  - buf.build/googleapis/googleapis        # для google.api.http
  - buf.build/grpc-ecosystem/grpc-gateway  # для gateway аннотаций
```

`buf.gen.yaml`:
```yaml
version: v2
plugins:
  # Обычные protobuf типы
  - remote: buf.build/protocolbuffers/go
    out: gen
    opt:
      - paths=source_relative

  # grpc-go server/client stubs
  - remote: buf.build/grpc/go
    out: gen
    opt:
      - paths=source_relative

  # grpc-gateway HTTP reverse-proxy
  - remote: buf.build/grpc-ecosystem/gateway
    out: gen
    opt:
      - paths=source_relative

  # OpenAPI v2 (Swagger) спецификация
  - remote: buf.build/grpc-ecosystem/openapiv2
    out: gen/openapi
```

После `buf generate` появится `user_service.pb.gw.go` — сгенерированный gateway handler.

---

## Запуск gateway сервера

### In-process на разных портах

```go
package main

import (
    "context"
    "net"
    "net/http"

    "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    userv1 "github.com/myorg/myrepo/gen/user/v1"
)

func main() {
    // gRPC сервер
    grpcSrv := grpc.NewServer()
    userv1.RegisterUserServiceServer(grpcSrv, &UserServiceServer{})

    lis, _ := net.Listen("tcp", ":50051")
    go grpcSrv.Serve(lis)

    // HTTP gateway
    mux := runtime.NewServeMux(
        // Прокидывать Authorization header как gRPC metadata
        runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
            switch key {
            case "Authorization", "X-Request-Id":
                return key, true
            }
            return runtime.DefaultHeaderMatcher(key)
        }),
        // Кастомный error handler
        runtime.WithErrorHandler(customErrorHandler),
    )

    opts := []grpc.DialOption{
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    }

    ctx := context.Background()
    userv1.RegisterUserServiceHandlerFromEndpoint(ctx, mux, "localhost:50051", opts)

    http.ListenAndServe(":8080", mux)
}
```

### In-process на одном порту через cmux

```go
import "github.com/soheilhy/cmux"

lis, _ := net.Listen("tcp", ":8080")
m := cmux.New(lis)

// HTTP/2 с Content-Type: application/grpc → gRPC
grpcL := m.MatchWithWriters(
    cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"),
)
// Всё остальное → HTTP/1.1 gateway
httpL := m.Match(cmux.Any())

grpcSrv := grpc.NewServer()
userv1.RegisterUserServiceServer(grpcSrv, &UserServiceServer{})
go grpcSrv.Serve(grpcL)

gwMux := runtime.NewServeMux()
// RegisterHandlerServer — без отдельного grpc соединения, напрямую
userv1.RegisterUserServiceHandlerServer(ctx, gwMux, &UserServiceServer{})
go http.Serve(httpL, gwMux)

m.Serve()
```

`RegisterUserServiceHandlerServer` (vs `HandlerFromEndpoint`) — регистрирует handler напрямую без gRPC round-trip. Быстрее, но теряешь gRPC interceptors на HTTP-пути.

---

## Маппинг HTTP ↔ gRPC

**Path params** → поля request message:
```
GET /v1/users/abc123
→ GetUserRequest{Id: "abc123"}
```

**Query params** → оставшиеся поля:
```
GET /v1/users?page_size=20&page_token=xyz
→ ListUsersRequest{PageSize: 20, PageToken: "xyz"}
```

**Body** → поля request message:
```
POST /v1/users
{"name": "Alice", "email": "alice@example.com"}
→ CreateUserRequest{Name: "Alice", Email: "alice@example.com"}
```

**Response** → JSON из proto message:
```json
{"user": {"id": "abc", "name": "Alice", "createdAt": "2024-01-15T10:00:00Z"}}
```

**Errors** → HTTP статус из gRPC code:
```
codes.NotFound     → 404
codes.InvalidArg   → 400
codes.Unauthenticated → 401
codes.Internal     → 500
```

Тело ошибки по умолчанию:
```json
{"code": 5, "message": "user abc123 not found", "details": []}
```

---

## OpenAPI генерация

`protoc-gen-openapiv2` генерирует Swagger/OpenAPI v2 спецификацию:

```protobuf
import "protoc-gen-openapiv2/options/annotations.proto";

option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_swagger) = {
  info: {
    title: "User Service API"
    version: "1.0"
  }
  schemes: HTTPS
  consumes: "application/json"
  produces: "application/json"
};

service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse) {
    option (google.api.http) = {get: "/v1/users/{id}"};
    option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {
      summary: "Get a user by ID"
      tags: ["Users"]
      responses: {
        key: "404"
        value: {description: "User not found"}
      }
    };
  }
}
```

Сгенерированный `user_service.swagger.json` можно подать в Swagger UI или импортировать в Postman.

---

## grpc-gateway vs connect-go

Оба решают одну задачу — дать REST/JSON клиентам доступ к gRPC сервису. Но по-разному:

| | grpc-gateway | connect-go |
|---|---|---|
| Подход | reverse-proxy (отдельный слой) | один сервер, несколько протоколов |
| Архитектура | gRPC сервер + gateway процесс/порт | один `net/http` handler |
| .proto аннотации | нужны (`google.api.http`) | не нужны |
| OpenAPI генерация | да (protoc-gen-openapiv2) | нет из коробки |
| URL маппинг | гибкий (любой HTTP route → любой RPC) | фиксированный (`/package.Service/Method`) |
| gRPC совместимость | через upstream gRPC сервер | нативная |
| Браузерный клиент | curl/fetch с кастомными URL | connect-es с фиксированным URL |
| Сложность настройки | выше | ниже |

**Выбирай grpc-gateway когда:**
- Нужен красивый REST API с произвольными URL (`GET /users/{id}` вместо `/user.v1.UserService/GetUser`)
- Нужна OpenAPI/Swagger документация из .proto
- Публичный API, клиенты ожидают REST конвенций
- Уже используешь grpc-go и не хочешь менять

**Выбирай connect-go когда:**
- Нужен браузерный TypeScript клиент с typesafe codegen
- Хочешь минимум инфраструктуры (один процесс, один порт)
- Не нужен кастомный URL-маппинг
- Новый проект без legacy

---

## Антипаттерны

**Gateway как единственный вход для gRPC клиентов** — gRPC клиенты должны ходить напрямую на gRPC сервер, а не через HTTP gateway. Gateway — только для REST клиентов.

**Бизнес-логика в gateway** — gateway должен быть тонким proxy. Никакой трансформации данных кроме маппинга полей.

**`RegisterHandlerServer` с streaming RPC** — server streaming через `RegisterHandlerServer` не работает корректно с HTTP/1.1 (нет chunked streaming). Для streaming используй `RegisterHandlerFromEndpoint` или connect-go.

**Отсутствие таймаутов на gateway** — если gRPC сервер завис, gateway будет держать HTTP соединение открытым. Добавляй `context.WithTimeout` в gateway обработчике или настраивай `runtime.WithForwardResponseOption`.
