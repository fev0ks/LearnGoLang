# connect-go

`connectrpc/connect-go` — альтернативная реализация gRPC которая запускается как обычный `net/http` handler. Поддерживает три протокола на одном порту без proxy.

## Содержание

- [Три протокола на одном порту](#три-протокола-на-одном-порту)
- [Сравнение с grpc-go архитектурой](#сравнение-с-grpc-go-архитектурой)
- [Сервер](#сервер)
- [Клиент](#клиент)
- [Error handling](#error-handling)
- [Interceptors](#interceptors)
- [Streaming](#streaming)
- [Browser-friendly RPC](#browser-friendly-rpc)
- [Кодогенерация для connect-go](#кодогенерация-для-connect-go)

---

## Три протокола на одном порту

grpc-go запускает собственный HTTP/2 сервер, не совместимый с обычным HTTP/1.1. Это означает:
- gRPC требует HTTP/2
- Браузеры не могут обращаться напрямую без proxy (Envoy, grpc-gateway)
- Нельзя использовать обычные HTTP middleware без специальных адаптеров

connect-go решает это запуском на стандартном `net/http`:

```
один порт :8080
  ├── Connect protocol (HTTP/1.1 или HTTP/2, JSON или protobuf)
  │     → curl, браузеры, fetch API
  ├── gRPC (HTTP/2 + protobuf)
  │     → grpc-go клиенты, grpcurl
  └── gRPC-Web (HTTP/1.1 + protobuf)
        → браузеры через generated TypeScript клиент
```

Клиент выбирает протокол через `Content-Type` header:
- `application/json` → Connect/JSON
- `application/proto` → Connect/protobuf
- `application/grpc` → gRPC
- `application/grpc-web` → gRPC-Web

---

## Сравнение с grpc-go архитектурой

| | grpc-go | connect-go |
|---|---|---|
| HTTP engine | собственный HTTP/2 | стандартный `net/http` |
| net/http middleware | нет (нужны адаптеры) | да, любые |
| Браузер без proxy | нет | да (Connect/JSON) |
| gRPC совместимость | да | да |
| TLS конфигурация | через grpc.Creds | через стандартный tls.Config |
| Рефлексия | grpc/reflection | connectrpc/grpcreflect |
| Зрелость | продакшн с 2016 | продакшн с 2022 |

---

## Сервер

```go
import (
    "net/http"

    "connectrpc.com/connect"
    userv1 "github.com/myorg/myrepo/gen/user/v1"
    "github.com/myorg/myrepo/gen/user/v1/userv1connect"
)

// Реализация сервиса — те же сгенерированные интерфейсы
type UserServiceHandler struct {
    repo UserRepository
}

func (h *UserServiceHandler) GetUser(
    ctx context.Context,
    req *connect.Request[userv1.GetUserRequest],
) (*connect.Response[userv1.GetUserResponse], error) {
    user, err := h.repo.Get(ctx, req.Msg.GetId())
    if err != nil {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
    }
    return connect.NewResponse(&userv1.GetUserResponse{
        User: toProto(user),
    }), nil
}

func main() {
    mux := http.NewServeMux()

    // path — "/user.v1.UserService/"
    path, handler := userv1connect.NewUserServiceHandler(
        &UserServiceHandler{repo: repo},
        connect.WithInterceptors(
            NewLoggingInterceptor(),
            NewAuthInterceptor(),
        ),
    )
    mux.Handle(path, handler)

    // Reflection — для grpcurl
    reflector := grpcreflect.NewStaticReflector(
        "user.v1.UserService",
    )
    mux.Handle(grpcreflect.NewHandlerV1(reflector))
    mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

    // Обычный net/http сервер — с h2c для gRPC без TLS
    srv := &http.Server{
        Addr:    ":8080",
        Handler: h2c.NewHandler(mux, &http2.Server{}),
    }
    srv.ListenAndServe()
}
```

`h2c` — HTTP/2 cleartext (без TLS). Нужен чтобы grpc-go клиенты могли подключаться без TLS в dev/internal сети.

---

## Клиент

```go
import "connectrpc.com/connect"
import "github.com/myorg/myrepo/gen/user/v1/userv1connect"

// HTTP клиент — обычный net/http
httpClient := &http.Client{
    Transport: &http2.Transport{
        AllowHTTP: true,  // h2c для локальной разработки
        DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
            return net.Dial(network, addr)
        },
    },
}

client := userv1connect.NewUserServiceClient(
    httpClient,
    "http://localhost:8080",
)

// Вызов
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := client.GetUser(ctx, connect.NewRequest(&userv1.GetUserRequest{
    Id: "abc123",
}))
if err != nil {
    var connectErr *connect.Error
    if errors.As(err, &connectErr) {
        fmt.Println(connectErr.Code(), connectErr.Message())
    }
    return
}
fmt.Println(resp.Msg.GetUser().GetName())
```

Добавить заголовок к запросу:
```go
req := connect.NewRequest(&userv1.GetUserRequest{Id: "abc"})
req.Header().Set("Authorization", "Bearer " + token)

resp, err := client.GetUser(ctx, req)

// Прочитать заголовок ответа
fmt.Println(resp.Header().Get("x-request-id"))
```

---

## Error handling

```go
import "connectrpc.com/connect"

// Сервер: создать ошибку
return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user %s not found", id))

// С оборачиванием cause
err := fmt.Errorf("db query failed: %w", dbErr)
return nil, connect.NewError(connect.CodeInternal, err)

// С деталями (совместимо с google.rpc.Status)
connectErr := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed"))
detail, _ := connect.NewErrorDetail(&errdetails.BadRequest{
    FieldViolations: []*errdetails.BadRequest_FieldViolation{
        {Field: "email", Description: "must be valid email"},
    },
})
connectErr.AddDetail(detail)
return nil, connectErr
```

```go
// Клиент: обработка
resp, err := client.GetUser(ctx, req)
if err != nil {
    var connectErr *connect.Error
    if errors.As(err, &connectErr) {
        switch connectErr.Code() {
        case connect.CodeNotFound:
            // ...
        case connect.CodeUnauthenticated:
            // ...
        }
        // Достать детали ошибки
        for _, detail := range connectErr.Details() {
            msg, _ := detail.Value()
            if br, ok := msg.(*errdetails.BadRequest); ok {
                for _, v := range br.FieldViolations {
                    fmt.Printf("field %s: %s\n", v.Field, v.Description)
                }
            }
        }
    }
}
```

Connect codes — те же что gRPC codes, просто другой тип:

| connect.Code | grpc.Code | HTTP (Connect protocol) |
|---|---|---|
| `CodeOK` | `codes.OK` | 200 |
| `CodeNotFound` | `codes.NotFound` | 404 |
| `CodeInvalidArgument` | `codes.InvalidArgument` | 400 |
| `CodeUnauthenticated` | `codes.Unauthenticated` | 401 |
| `CodePermissionDenied` | `codes.PermissionDenied` | 403 |
| `CodeInternal` | `codes.Internal` | 500 |
| `CodeUnavailable` | `codes.Unavailable` | 503 |

---

## Interceptors

```go
import "connectrpc.com/connect"

// Unary interceptor
func NewLoggingInterceptor() connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            start := time.Now()
            resp, err := next(ctx, req)
            slog.Info("rpc",
                "procedure", req.Spec().Procedure,  // "/user.v1.UserService/GetUser"
                "dur",       time.Since(start),
                "err",       err,
            )
            return resp, err
        }
    }
}

func NewAuthInterceptor(secret string) connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            token := req.Header().Get("Authorization")
            userID, err := validateJWT(token, secret)
            if err != nil {
                return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
            }
            ctx = context.WithValue(ctx, userIDKey, userID)
            return next(ctx, req)
        }
    }
}

// Подключение к handler
path, handler := userv1connect.NewUserServiceHandler(
    &UserServiceHandler{},
    connect.WithInterceptors(
        NewLoggingInterceptor(),
        NewAuthInterceptor(secret),
    ),
)

// Или глобально для всего mux через net/http middleware
mux.Handle(path, authMiddleware(handler))  // обычный http.Handler middleware!
```

---

## Streaming

```go
// Server streaming
func (h *UserServiceHandler) WatchUser(
    ctx context.Context,
    req *connect.Request[userv1.WatchUserRequest],
    stream *connect.ServerStream[userv1.UserEvent],
) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        case event := <-h.events:
            if err := stream.Send(&userv1.UserEvent{Type: event.Type}); err != nil {
                return err
            }
        }
    }
}

// Клиент читает server stream
stream, err := client.WatchUser(ctx, connect.NewRequest(&userv1.WatchUserRequest{
    UserId: "abc",
}))
for stream.Receive() {
    fmt.Println(stream.Msg().GetType())
}
if err := stream.Err(); err != nil {
    return err
}
```

```go
// Bidirectional streaming
func (h *Handler) SyncUsers(
    ctx context.Context,
    stream *connect.BidiStream[userv1.SyncRequest, userv1.SyncResponse],
) error {
    for {
        req, err := stream.Receive()
        if errors.Is(err, io.EOF) {
            return nil
        }
        if err != nil {
            return err
        }
        if err := stream.Send(&userv1.SyncResponse{...}); err != nil {
            return err
        }
    }
}
```

---

## Browser-friendly RPC

Главное практическое преимущество connect-go — TypeScript/JavaScript клиенты могут вызывать сервисы напрямую через `fetch`, без proxy.

`buf.gen.yaml` для фронтенда:
```yaml
plugins:
  - remote: buf.build/connectrpc/es      # connect-es TypeScript codegen
    out: frontend/src/gen
    opt:
      - target=ts

  - remote: buf.build/bufbuild/es        # protobuf-es для типов
    out: frontend/src/gen
    opt:
      - target=ts
```

TypeScript клиент:
```typescript
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UserService } from "./gen/user/v1/user_service_connect";

const transport = createConnectTransport({
    baseUrl: "http://localhost:8080",
});

const client = createClient(UserService, transport);

// Вызов — обычный fetch под капотом, никакого proxy
const response = await client.getUser({ id: "abc123" });
console.log(response.user?.name);
```

Это работает потому что Connect protocol использует обычный HTTP POST с JSON или protobuf — браузер умеет это делать нативно. В отличие от gRPC, который требует HTTP/2 с trailers (браузеры не поддерживают HTTP/2 trailers напрямую).

---

## Кодогенерация для connect-go

`buf.gen.yaml` для проекта с connect-go:
```yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/myorg/myrepo/gen

plugins:
  # Protobuf типы
  - remote: buf.build/protocolbuffers/go
    out: gen
    opt:
      - paths=source_relative

  # connect-go вместо grpc-go
  - remote: buf.build/connectrpc/go
    out: gen
    opt:
      - paths=source_relative
```

`protoc-gen-go-grpc` не нужен — `protoc-gen-connect-go` генерирует и client, и server интерфейсы.

Сгенерированный файл `user_service_grpc.pb.go` (grpc-go) заменяется на `user_service.connect.go`:
```go
// user_service.connect.go (connect-go generated)
const UserServiceGetUserProcedure = "/user.v1.UserService/GetUser"

type UserServiceClient interface {
    GetUser(context.Context, *connect.Request[userv1.GetUserRequest]) (*connect.Response[userv1.GetUserResponse], error)
    WatchUser(context.Context, *connect.Request[userv1.WatchUserRequest]) (*connect.ServerStreamForClient[userv1.UserEvent], error)
}

type UserServiceHandler interface {
    GetUser(context.Context, *connect.Request[userv1.GetUserRequest]) (*connect.Response[userv1.GetUserResponse], error)
    WatchUser(context.Context, *connect.Request[userv1.WatchUserRequest], *connect.ServerStream[userv1.UserEvent]) error
}

func NewUserServiceHandler(svc UserServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
    // возвращает path + http.Handler
}
```
