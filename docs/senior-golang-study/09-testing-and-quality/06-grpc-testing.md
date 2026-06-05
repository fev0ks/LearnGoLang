# Тестирование gRPC-сервера

gRPC тестируется через `bufconn` — in-memory соединение без реального сетевого порта. Тест запускает настоящий gRPC-сервер и ходит к нему через настоящий gRPC-клиент.

## Содержание

- [bufconn — in-memory gRPC соединение](#bufconn--in-memory-grpc-соединение)
- [Тест унарного метода](#тест-унарного-метода)
- [Тест ошибок и статусов](#тест-ошибок-и-статусов)
- [Тест server-streaming](#тест-server-streaming)
- [Тестирование interceptor'ов](#тестирование-interceptorов)
- [Вспомогательная инфраструктура](#вспомогательная-инфраструктура)

---

## bufconn — in-memory gRPC соединение

```go
import (
    "context"
    "net"
    "testing"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func newTestServer(t *testing.T, svc UserServiceServer) *grpc.ClientConn {
    t.Helper()

    lis := bufconn.Listen(bufSize)

    srv := grpc.NewServer()
    RegisterUserServiceServer(srv, svc)

    go func() {
        if err := srv.Serve(lis); err != nil {
            // грубо — сервер закрывается при t.Cleanup
        }
    }()

    t.Cleanup(func() {
        srv.GracefulStop()
        lis.Close()
    })

    conn, err := grpc.NewClient(
        "passthrough:///bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        t.Fatalf("dial bufconn: %v", err)
    }
    t.Cleanup(func() { conn.Close() })

    return conn
}
```

---

## Тест унарного метода

```go
func TestUserService_GetUser(t *testing.T) {
    repo := newFakeUserRepo()
    repo.users["user-1"] = User{ID: "user-1", Email: "alice@example.com", Name: "Alice"}

    conn := newTestServer(t, NewUserGRPCServer(NewUserService(repo)))
    client := NewUserServiceClient(conn)

    t.Run("existing user", func(t *testing.T) {
        resp, err := client.GetUser(context.Background(), &GetUserRequest{UserId: "user-1"})
        require.NoError(t, err)
        assert.Equal(t, "alice@example.com", resp.Email)
        assert.Equal(t, "Alice", resp.Name)
    })

    t.Run("not found", func(t *testing.T) {
        _, err := client.GetUser(context.Background(), &GetUserRequest{UserId: "missing"})
        require.Error(t, err)

        st, ok := status.FromError(err)
        require.True(t, ok)
        assert.Equal(t, codes.NotFound, st.Code())
    })
}
```

---

## Тест ошибок и статусов

gRPC использует `google.golang.org/grpc/status` и `google.golang.org/grpc/codes` вместо HTTP-кодов.

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

func TestUserService_CreateUser_DuplicateEmail(t *testing.T) {
    repo := newFakeUserRepo()
    conn := newTestServer(t, NewUserGRPCServer(NewUserService(repo)))
    client := NewUserServiceClient(conn)

    req := &CreateUserRequest{Email: "alice@example.com", Name: "Alice"}

    // Первый запрос — успешно
    _, err := client.CreateUser(context.Background(), req)
    require.NoError(t, err)

    // Второй запрос — конфликт
    _, err = client.CreateUser(context.Background(), req)
    require.Error(t, err)

    st, ok := status.FromError(err)
    require.True(t, ok, "expected gRPC status error")
    assert.Equal(t, codes.AlreadyExists, st.Code())
    assert.Contains(t, st.Message(), "email")
}

// Вспомогательная функция проверки gRPC-ошибки
func requireGRPCError(t *testing.T, err error, code codes.Code) {
    t.Helper()
    require.Error(t, err)
    st, ok := status.FromError(err)
    require.True(t, ok, "expected gRPC status error, got: %T", err)
    assert.Equal(t, code, st.Code(), "gRPC status code mismatch: %s", st.Message())
}

func TestUserService_GetUser_InvalidArgument(t *testing.T) {
    conn := newTestServer(t, NewUserGRPCServer(NewUserService(newFakeUserRepo())))
    client := NewUserServiceClient(conn)

    _, err := client.GetUser(context.Background(), &GetUserRequest{UserId: ""})
    requireGRPCError(t, err, codes.InvalidArgument)
}
```

---

## Тест server-streaming

```go
func TestUserService_ListUsers_Streaming(t *testing.T) {
    repo := newFakeUserRepo()
    for i := 0; i < 5; i++ {
        id := fmt.Sprintf("user-%d", i)
        repo.users[id] = User{ID: id, Email: fmt.Sprintf("user%d@example.com", i)}
    }

    conn := newTestServer(t, NewUserGRPCServer(NewUserService(repo)))
    client := NewUserServiceClient(conn)

    stream, err := client.ListUsers(context.Background(), &ListUsersRequest{})
    require.NoError(t, err)

    var users []*UserResponse
    for {
        resp, err := stream.Recv()
        if errors.Is(err, io.EOF) {
            break
        }
        require.NoError(t, err)
        users = append(users, resp)
    }

    assert.Len(t, users, 5)
}

// Тест отмены stream
func TestUserService_ListUsers_ContextCancelled(t *testing.T) {
    repo := newFakeUserRepo()
    // много пользователей чтобы stream не закончился сразу
    for i := 0; i < 1000; i++ {
        id := fmt.Sprintf("user-%d", i)
        repo.users[id] = User{ID: id, Email: fmt.Sprintf("u%d@example.com", i)}
    }

    conn := newTestServer(t, NewUserGRPCServer(NewUserService(repo)))
    client := NewUserServiceClient(conn)

    ctx, cancel := context.WithCancel(context.Background())

    stream, err := client.ListUsers(ctx, &ListUsersRequest{})
    require.NoError(t, err)

    // Получить несколько и отменить
    _, err = stream.Recv()
    require.NoError(t, err)
    cancel()

    // Следующий Recv должен вернуть ошибку отмены
    _, err = stream.Recv()
    require.Error(t, err)

    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.Canceled, st.Code())
}
```

---

## Тестирование interceptor'ов

Interceptor тестируется в изоляции через `grpc.UnaryHandler`.

```go
// Interceptor добавляет request-id в контекст
func RequestIDInterceptor(
    ctx context.Context,
    req any,
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (any, error) {
    id := metautils.ExtractIncoming(ctx).Get("x-request-id")
    if id == "" {
        id = uuid.New().String()
    }
    ctx = context.WithValue(ctx, requestIDKey, id)
    return handler(ctx, req)
}

func TestRequestIDInterceptor(t *testing.T) {
    t.Run("passes existing request id to context", func(t *testing.T) {
        var capturedID string
        handler := func(ctx context.Context, req any) (any, error) {
            capturedID = ctx.Value(requestIDKey).(string)
            return nil, nil
        }

        md := metadata.Pairs("x-request-id", "my-id-123")
        ctx := metadata.NewIncomingContext(context.Background(), md)

        _, err := RequestIDInterceptor(ctx, nil, nil, handler)
        require.NoError(t, err)
        assert.Equal(t, "my-id-123", capturedID)
    })

    t.Run("generates id when missing", func(t *testing.T) {
        var capturedID string
        handler := func(ctx context.Context, req any) (any, error) {
            capturedID = ctx.Value(requestIDKey).(string)
            return nil, nil
        }

        _, err := RequestIDInterceptor(context.Background(), nil, nil, handler)
        require.NoError(t, err)
        assert.NotEmpty(t, capturedID)
    })
}
```

### Interceptor через полный сервер с цепочкой

```go
func newTestServerWithInterceptors(t *testing.T, svc UserServiceServer) *grpc.ClientConn {
    t.Helper()

    lis := bufconn.Listen(bufSize)

    srv := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            RequestIDInterceptor,
            LoggingInterceptor,
        ),
    )
    RegisterUserServiceServer(srv, svc)

    go srv.Serve(lis)
    t.Cleanup(func() {
        srv.GracefulStop()
        lis.Close()
    })

    conn, err := grpc.NewClient(
        "passthrough:///bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    require.NoError(t, err)
    t.Cleanup(func() { conn.Close() })
    return conn
}
```

---

## Вспомогательная инфраструктура

### Переиспользуемый setup в TestMain

```go
var testConn *grpc.ClientConn

func TestMain(m *testing.M) {
    repo := newFakeUserRepo()
    seedTestData(repo)

    lis := bufconn.Listen(bufSize)
    srv := grpc.NewServer()
    RegisterUserServiceServer(srv, NewUserGRPCServer(NewUserService(repo)))
    go srv.Serve(lis)

    conn, err := grpc.NewClient(
        "passthrough:///bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        log.Fatalf("dial: %v", err)
    }
    testConn = conn

    code := m.Run()

    conn.Close()
    srv.GracefulStop()
    os.Exit(code)
}
```

### Передача metadata (auth, tracing)

```go
import "google.golang.org/grpc/metadata"

func TestUserService_WithAuthToken(t *testing.T) {
    conn := newTestServer(t, NewUserGRPCServer(NewUserService(newFakeUserRepo())))
    client := NewUserServiceClient(conn)

    // Передать metadata вместо HTTP заголовков
    ctx := metadata.AppendToOutgoingContext(context.Background(),
        "authorization", "Bearer my-token",
        "x-request-id", "req-123",
    )

    _, err := client.GetUser(ctx, &GetUserRequest{UserId: "user-1"})
    // Проверить что сервер корректно прочитал token
    requireGRPCError(t, err, codes.NotFound)  // user-1 не добавлен, но auth прошёл
}
```
