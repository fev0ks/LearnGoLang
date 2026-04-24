# gRPC

gRPC — фреймворк удалённых вызовов от Google на базе HTTP/2 и Protocol Buffers. Типобезопасный контракт, binary сериализация, streaming из коробки.

---

## Protobuf — основа gRPC

### Определение сервиса

```protobuf
// proto/user/user.proto
syntax = "proto3";
package user.v1;

option go_package = "gen/user/v1;userv1";

// Сообщения
message User {
    string id         = 1;
    string name       = 2;
    string email      = 3;
    repeated string roles = 4;  // слайс
}

message GetUserRequest {
    string user_id = 1;
}

message ListUsersRequest {
    int32 page_size   = 1;
    string page_token = 2;
    string filter     = 3;
}

// Сервис с 4 типами RPC
service UserService {
    // Unary: один запрос → один ответ
    rpc GetUser(GetUserRequest) returns (User);
    
    // Server streaming: один запрос → поток ответов
    rpc ListUsers(ListUsersRequest) returns (stream User);
    
    // Client streaming: поток запросов → один ответ
    rpc ImportUsers(stream User) returns (ImportResult);
    
    // Bidirectional streaming: поток ↔ поток
    rpc Chat(stream ChatMessage) returns (stream ChatMessage);
}
```

### Кодогенерация

```bash
# Установить плагины
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Сгенерировать код
protoc \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/user/user.proto

# Генерирует:
# proto/user/user.pb.go       — структуры данных
# proto/user/user_grpc.pb.go  — client/server интерфейсы
```

### Эволюция schema: backward compatibility

```protobuf
// Правила безопасного изменения:

// ✅ Добавить новое поле — OK (старый клиент игнорирует)
message User {
    string id    = 1;
    string name  = 2;
    string email = 3;   // новое — OK
}

// ❌ Изменить номер поля — СЛОМАЕТ wire format
message User {
    string id    = 2;   // было 1 — всё сломается
    string name  = 3;   // было 2
}

// ✅ Удалить поле — можно (зарезервировать номер)
message User {
    string id    = 1;
    // string old_field = 2; — удалено
    reserved 2;          // нельзя переиспользовать
    reserved "old_field"; // нельзя переиспользовать имя
    string name  = 3;
}
```

---

## 4 типа gRPC RPC

### 1. Unary — стандартный request/response

```go
// Сервер
type userServer struct {
    userv1.UnimplementedUserServiceServer
    repo UserRepository
}

func (s *userServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
    user, err := s.repo.FindByID(ctx, req.UserId)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return nil, status.Errorf(codes.NotFound, "user %s not found", req.UserId)
        }
        return nil, status.Errorf(codes.Internal, "get user: %v", err)
    }
    return toProto(user), nil
}

// Клиент
resp, err := client.GetUser(ctx, &userv1.GetUserRequest{UserId: "123"})
if err != nil {
    st, _ := status.FromError(err)
    log.Printf("code=%v message=%s", st.Code(), st.Message())
}
```

### 2. Server streaming — поток ответов

```go
// Сервер
func (s *userServer) ListUsers(req *userv1.ListUsersRequest, stream userv1.UserService_ListUsersServer) error {
    users, err := s.repo.List(stream.Context(), req.Filter)
    if err != nil {
        return status.Errorf(codes.Internal, "list users: %v", err)
    }
    
    for _, u := range users {
        if err := stream.Context().Err(); err != nil {
            return err // клиент отменил
        }
        if err := stream.Send(toProto(u)); err != nil {
            return err // клиент отключился
        }
    }
    return nil
}

// Клиент
stream, err := client.ListUsers(ctx, &userv1.ListUsersRequest{Filter: "active"})
for {
    user, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(user.Name)
}
```

### 3. Client streaming — поток запросов

```go
// Клиент
stream, err := client.ImportUsers(ctx)
for _, user := range usersToImport {
    stream.Send(userToProto(user))
}
result, err := stream.CloseAndRecv() // закрыть и получить ответ
fmt.Println("imported:", result.Count)
```

### 4. Bidirectional streaming — полный дуплекс

```go
stream, err := client.Chat(ctx)

// Отправка в отдельной горутине
go func() {
    for _, msg := range messages {
        stream.Send(msg)
    }
    stream.CloseSend()
}()

// Приём
for {
    msg, err := stream.Recv()
    if err == io.EOF { break }
    if err != nil { log.Fatal(err) }
    fmt.Println(msg.Text)
}
```

---

## gRPC в Go: сервер с interceptors

### Запуск сервера

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/health"
    healthv1 "google.golang.org/grpc/health/grpc_health_v1"
    "google.golang.org/grpc/reflection"
)

func main() {
    lis, _ := net.Listen("tcp", ":50051")
    
    s := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            loggingInterceptor,
            recoveryInterceptor,
            authInterceptor,
        ),
        grpc.ChainStreamInterceptor(
            streamLoggingInterceptor,
        ),
    )
    
    // Регистрируем сервисы
    userv1.RegisterUserServiceServer(s, &userServer{})
    
    // Health check (для Kubernetes liveness/readiness)
    healthSrv := health.NewServer()
    healthv1.RegisterHealthServer(s, healthSrv)
    healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
    
    // Reflection (для grpcurl, Postman)
    reflection.Register(s)
    
    log.Println("gRPC server on :50051")
    s.Serve(lis)
}
```

### Unary interceptor

```go
func loggingInterceptor(
    ctx context.Context,
    req any,
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (any, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    
    st, _ := status.FromError(err)
    log.Printf("method=%s code=%v duration=%v",
        info.FullMethod, st.Code(), time.Since(start))
    
    return resp, err
}

func recoveryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("panic in %s: %v\n%s", info.FullMethod, r, debug.Stack())
            err = status.Errorf(codes.Internal, "internal error")
        }
    }()
    return handler(ctx, req)
}
```

---

## gRPC Клиент

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/keepalive"
)

func newGRPCClient(addr string) (userv1.UserServiceClient, error) {
    conn, err := grpc.NewClient(addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                10 * time.Second, // ping каждые 10 сек
            Timeout:             5 * time.Second,
            PermitWithoutStream: true,
        }),
        grpc.WithChainUnaryInterceptor(
            clientLoggingInterceptor,
        ),
    )
    if err != nil {
        return nil, err
    }
    return userv1.NewUserServiceClient(conn), nil
}
```

### Таймауты и retry

```go
// Таймаут через context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := client.GetUser(ctx, req)

// Retry через service config (grpc.DialOption)
serviceConfig := `{
    "methodConfig": [{
        "name": [{"service": "user.v1.UserService"}],
        "retryPolicy": {
            "maxAttempts": 4,
            "initialBackoff": "0.1s",
            "maxBackoff": "1s",
            "backoffMultiplier": 2,
            "retryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED"]
        }
    }]
}`
conn, _ := grpc.NewClient(addr,
    grpc.WithDefaultServiceConfig(serviceConfig),
)
```

---

## gRPC status codes

```go
// Правильное использование кодов
codes.OK           // успех
codes.NotFound     // ресурс не найден (404)
codes.InvalidArgument  // ошибка валидации (400)
codes.AlreadyExists    // конфликт (409)
codes.Unauthenticated  // нет авторизации (401)
codes.PermissionDenied // нет прав (403)
codes.ResourceExhausted // rate limit (429)
codes.Internal         // внутренняя ошибка (500)
codes.Unavailable      // сервис недоступен, можно retry (503)
codes.DeadlineExceeded // таймаут (504)
codes.Unimplemented    // метод не реализован (501)

// Создание ошибки
return nil, status.Errorf(codes.NotFound, "user %s not found", id)

// С деталями (rich errors)
st := status.New(codes.InvalidArgument, "validation failed")
st, _ = st.WithDetails(&errdetails.BadRequest{
    FieldViolations: []*errdetails.BadRequest_FieldViolation{
        {Field: "email", Description: "invalid format"},
    },
})
return nil, st.Err()
```

---

## gRPC vs REST

| | REST/HTTP | gRPC |
|---|---|---|
| Protocol | HTTP/1.1 (обычно) | HTTP/2 |
| Serialization | JSON (текст) | Protobuf (binary) |
| Schema | OpenAPI (опционально) | .proto (обязательно) |
| Streaming | SSE / WebSocket (отдельно) | Встроено (4 типа) |
| Browser support | ✅ нативно | ❌ нужен grpc-web proxy |
| Performance | ниже | выше (binary, multiplexing) |
| Human-readable | ✅ | ❌ |
| Code generation | OpenAPI codegen | protoc (надёжнее) |
| Use case | Public API, mobile, web | Service-to-service, внутренние API |

**Когда gRPC:**
- Сервис-сервис взаимодействие (не публичный API)
- Нужен streaming (real-time, large datasets)
- Строгий контракт важнее гибкости
- High-performance (latency < 10ms важна)

**Когда REST:**
- Публичный API (browser, mobile, third-party)
- Команда не готова к proto и codegen
- Нужна простая интеграция (curl, Postman)

---

## Health Check и Reflection

```go
// Health check для K8s probes
healthSrv.SetServingStatus("UserService", healthpb.HealthCheckResponse_SERVING)
healthSrv.SetServingStatus("UserService", healthpb.HealthCheckResponse_NOT_SERVING)

// Проверить из командной строки
grpc_health_probe -addr=:50051
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

// Reflection — список методов без .proto файла
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 list user.v1.UserService
grpcurl -plaintext -d '{"user_id":"123"}' localhost:50051 user.v1.UserService/GetUser
```

---

## Interview-ready answer

**Q: Чем gRPC лучше REST?**

gRPC использует HTTP/2 (multiplexing, header compression, binary framing) и Protobuf (binary serialization — в 3–10 раз компактнее JSON). Это даёт меньшую latency и выше throughput. Protobuf schema — строгий контракт с кодогенерацией: клиент и сервер компилируются с одним .proto файлом → типобезопасность на уровне компиляции. gRPC поддерживает streaming из коробки (4 типа). Минус — нет browser поддержки без grpc-web, сложнее дебажить чем JSON.

**Q: Что такое interceptor и зачем он нужен?**

Interceptor — middleware для gRPC, аналог HTTP middleware. Обернует каждый вызов: логирование, авторизация, recovery от паники, метрики. Unary interceptor работает с request/response, Stream interceptor — с установкой потока. Через `grpc.ChainUnaryInterceptor` можно составить цепочку.
