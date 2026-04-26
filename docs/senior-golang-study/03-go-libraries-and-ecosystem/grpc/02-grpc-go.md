# grpc-go

`google.golang.org/grpc` — официальная Go реализация gRPC от Google. Стандартный выбор для gRPC-сервисов.

## Содержание

- [Сервер](#сервер)
- [Клиент](#клиент)
- [Error handling: Status codes](#error-handling-status-codes)
- [Metadata (заголовки gRPC)](#metadata-заголовки-grpc)
- [Interceptors](#interceptors)
- [Streaming](#streaming)
- [TLS и credentials](#tls-и-credentials)
- [Reflection и grpcurl](#reflection-и-grpcurl)
- [Graceful shutdown](#graceful-shutdown)

---

## Сервер

```go
import (
    "google.golang.org/grpc"
    userv1 "github.com/myorg/myrepo/gen/user/v1"
)

// Реализация сервиса
type UserServiceServer struct {
    userv1.UnimplementedUserServiceServer  // embed — forward compatibility
    repo UserRepository
}

func (s *UserServiceServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
    user, err := s.repo.Get(ctx, req.GetId())
    if err != nil {
        return nil, status.Errorf(codes.NotFound, "user %s not found", req.GetId())
    }
    return &userv1.GetUserResponse{User: toProto(user)}, nil
}

func (s *UserServiceServer) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
    if req.GetName() == "" {
        return nil, status.Error(codes.InvalidArgument, "name is required")
    }
    user, err := s.repo.Create(ctx, fromProto(req))
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
    }
    return &userv1.CreateUserResponse{User: toProto(user)}, nil
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    srv := grpc.NewServer(
        grpc.MaxRecvMsgSize(4 * 1024 * 1024),  // 4MB
        grpc.MaxSendMsgSize(4 * 1024 * 1024),
    )

    userv1.RegisterUserServiceServer(srv, &UserServiceServer{repo: repo})

    // gRPC health check (стандартный протокол)
    grpc_health_v1.RegisterHealthServer(srv, health.NewServer())

    if err := srv.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
```

`UnimplementedUserServiceServer` — обязательный embed. Без него код не скомпилируется если сервис определён с `require_unimplemented_servers` (дефолт). При добавлении нового RPC в .proto существующие серверы не сломаются — метод вернёт `codes.Unimplemented`.

---

## Клиент

```go
// Создание соединения
conn, err := grpc.NewClient(
    "localhost:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()),  // без TLS (dev)
)
if err != nil {
    log.Fatalf("failed to connect: %v", err)
}
defer conn.Close()

client := userv1.NewUserServiceClient(conn)

// Unary вызов
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: "abc123"})
if err != nil {
    // Обработка gRPC ошибок
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.NotFound:
            fmt.Println("user not found")
        case codes.DeadlineExceeded:
            fmt.Println("request timed out")
        default:
            fmt.Printf("gRPC error: %v\n", st.Message())
        }
    }
    return
}
fmt.Println(resp.GetUser().GetName())
```

**`grpc.Dial` vs `grpc.NewClient`**: в grpc-go v1.58+ появился `grpc.NewClient` — рекомендуемая замена устаревшему `grpc.Dial`. `NewClient` не устанавливает соединение немедленно (lazy), `Dial` с `WithBlock()` блокировал до соединения.

---

## Error handling: Status codes

gRPC использует собственные status codes вместо HTTP статусов:

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

// Создание ошибок
return nil, status.Error(codes.NotFound, "user not found")
return nil, status.Errorf(codes.InvalidArgument, "invalid id: %s", id)

// Ошибка с деталями (structured error details)
st, _ := status.New(codes.InvalidArgument, "validation failed").
    WithDetails(&errdetails.BadRequest{
        FieldViolations: []*errdetails.BadRequest_FieldViolation{
            {Field: "email", Description: "must be valid email"},
        },
    })
return nil, st.Err()
```

| Code | HTTP аналог | Когда использовать |
|---|---|---|
| `OK` | 200 | успех |
| `InvalidArgument` | 400 | невалидный запрос от клиента |
| `NotFound` | 404 | ресурс не найден |
| `AlreadyExists` | 409 | дублирование |
| `PermissionDenied` | 403 | нет прав |
| `Unauthenticated` | 401 | не аутентифицирован |
| `ResourceExhausted` | 429 | rate limit, квота |
| `FailedPrecondition` | 400 | состояние не то (не 400 — это бизнес-логика) |
| `Unavailable` | 503 | сервис временно недоступен (retry safe) |
| `DeadlineExceeded` | 504 | таймаут |
| `Internal` | 500 | внутренняя ошибка |
| `Unimplemented` | 501 | метод не реализован |

```go
// Проверка кода на клиенте
st, ok := status.FromError(err)
if ok && st.Code() == codes.NotFound {
    // resource not found
}

// errors.Is не работает для gRPC ошибок — нужен status.FromError
```

---

## Metadata (заголовки gRPC)

Metadata — аналог HTTP headers в gRPC. Передаётся через `context`.

```go
import "google.golang.org/grpc/metadata"

// Клиент: отправить metadata
ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
    "authorization", "Bearer " + token,
    "x-request-id",  requestID,
))
resp, err := client.GetUser(ctx, req)

// Клиент: получить metadata из ответа (trailer)
var header, trailer metadata.MD
resp, err := client.GetUser(ctx, req,
    grpc.Header(&header),
    grpc.Trailer(&trailer),
)
fmt.Println(header.Get("x-request-id"))
```

```go
// Сервер: прочитать входящую metadata
func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, status.Error(codes.Unauthenticated, "missing metadata")
    }
    tokens := md.Get("authorization")
    if len(tokens) == 0 {
        return nil, status.Error(codes.Unauthenticated, "missing token")
    }
    // ...
}

// Сервер: отправить metadata в ответе
grpc.SendHeader(ctx, metadata.Pairs("x-request-id", id))   // header (до первого сообщения)
grpc.SetTrailer(ctx, metadata.Pairs("x-custom", "value"))  // trailer (после последнего)
```

---

## Interceptors

Interceptors — middleware для gRPC. Два вида: Unary и Stream.

```go
// Unary interceptor (для обычных RPC вызовов)
func LoggingInterceptor(
    ctx context.Context,
    req interface{},
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (interface{}, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    slog.Info("grpc request",
        "method", info.FullMethod,
        "dur",    time.Since(start),
        "err",    err,
    )
    return resp, err
}

func AuthInterceptor(jwtSecret string) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        md, _ := metadata.FromIncomingContext(ctx)
        tokens := md.Get("authorization")
        if len(tokens) == 0 {
            return nil, status.Error(codes.Unauthenticated, "missing token")
        }
        userID, err := validateJWT(tokens[0], jwtSecret)
        if err != nil {
            return nil, status.Error(codes.Unauthenticated, "invalid token")
        }
        ctx = context.WithValue(ctx, userIDKey, userID)
        return handler(ctx, req)
    }
}

// Несколько interceptors — через ChainUnaryInterceptor
srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        RecoveryInterceptor,
        LoggingInterceptor,
        AuthInterceptor(jwtSecret),
    ),
)
```

```go
// Stream interceptor
func StreamLoggingInterceptor(
    srv interface{},
    ss grpc.ServerStream,
    info *grpc.StreamServerInfo,
    handler grpc.StreamHandler,
) error {
    start := time.Now()
    err := handler(srv, ss)
    slog.Info("grpc stream",
        "method",      info.FullMethod,
        "client_stream", info.IsClientStream,
        "server_stream", info.IsServerStream,
        "dur",         time.Since(start),
    )
    return err
}

srv := grpc.NewServer(
    grpc.ChainStreamInterceptor(StreamLoggingInterceptor),
)
```

Популярная библиотека готовых interceptors: `github.com/grpc-ecosystem/go-grpc-middleware/v2` — logging, recovery, auth, validation, ratelimit.

---

## Streaming

```go
// Server streaming: сервер шлёт поток событий
func (s *Server) WatchUser(req *userv1.WatchUserRequest, stream userv1.UserService_WatchUserServer) error {
    ctx := stream.Context()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case event := <-s.events:
            if err := stream.Send(&userv1.UserEvent{...}); err != nil {
                return err  // клиент отключился
            }
        }
    }
}

// Клиент читает поток
stream, err := client.WatchUser(ctx, &userv1.WatchUserRequest{UserId: "abc"})
for {
    event, err := stream.Recv()
    if err == io.EOF {
        break  // сервер закрыл поток
    }
    if err != nil {
        return fmt.Errorf("stream error: %w", err)
    }
    fmt.Println(event.GetType())
}
```

```go
// Bidirectional streaming
func (s *Server) SyncUsers(stream userv1.UserService_SyncUsersServer) error {
    for {
        req, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return err
        }
        // Обработать req и отправить ответ
        if err := stream.Send(&userv1.SyncResponse{...}); err != nil {
            return err
        }
    }
}
```

---

## TLS и credentials

```go
// Сервер с TLS
creds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
srv := grpc.NewServer(grpc.Creds(creds))

// Клиент с TLS (системный CA pool)
creds, err := credentials.NewClientTLSFromFile("ca.crt", "")
conn, err := grpc.NewClient("api.example.com:443", grpc.WithTransportCredentials(creds))

// Клиент с TLS (системный CA, валидация по hostname)
creds := credentials.NewTLS(&tls.Config{
    ServerName: "api.example.com",
})
conn, err := grpc.NewClient("api.example.com:443", grpc.WithTransportCredentials(creds))
```

---

## Reflection и grpcurl

gRPC reflection — сервер отдаёт описание своих методов (как OpenAPI, только в runtime).

```go
import "google.golang.org/grpc/reflection"

srv := grpc.NewServer()
userv1.RegisterUserServiceServer(srv, &UserServiceServer{})
reflection.Register(srv)  // включить reflection (только для dev/staging)
```

Тестирование с `grpcurl`:
```bash
# Список сервисов
grpcurl -plaintext localhost:50051 list

# Список методов сервиса
grpcurl -plaintext localhost:50051 list user.v1.UserService

# Вызов метода
grpcurl -plaintext \
  -H "authorization: Bearer mytoken" \
  -d '{"id": "abc123"}' \
  localhost:50051 user.v1.UserService/GetUser

# Describe — структура request/response
grpcurl -plaintext localhost:50051 describe user.v1.UserService.GetUser
```

---

## Graceful shutdown

```go
srv := grpc.NewServer()
// ... регистрация сервисов

go func() {
    if err := srv.Serve(lis); err != nil {
        log.Fatal(err)
    }
}()

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit

// GracefulStop ждёт завершения активных RPC (streaming в том числе)
// Stop() — немедленное завершение
srv.GracefulStop()
```

`GracefulStop()` ждёт пока все in-flight RPC завершатся. Для streaming RPC это может быть долго — стоит добавить таймаут:

```go
stopped := make(chan struct{})
go func() {
    srv.GracefulStop()
    close(stopped)
}()

select {
case <-stopped:
case <-time.After(30 * time.Second):
    srv.Stop()
}
```
