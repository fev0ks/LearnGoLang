# Тестирование HTTP-сервера

Два инструмента из стандартной библиотеки покрывают большинство задач: `httptest.ResponseRecorder` для тестирования хэндлеров в изоляции и `httptest.Server` для тестирования HTTP-клиентов.

## Содержание

- [httptest.NewRecorder — тест хэндлера](#httptestnewrecorder--тест-хэндлера)
- [httptest.NewServer — тест HTTP-клиента](#httptestnewserver--тест-http-клиента)
- [Тестирование middleware](#тестирование-middleware)
- [Тест через полный router](#тест-через-полный-router)
- [JSON request / response паттерн](#json-request--response-паттерн)
- [Тестирование ошибок и статус-кодов](#тестирование-ошибок-и-статус-кодов)

---

## httptest.NewRecorder — тест хэндлера

`httptest.ResponseRecorder` реализует `http.ResponseWriter` и записывает результат — без поднятия сервера.

```go
import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestGetUserHandler(t *testing.T) {
    repo := newFakeUserRepo()
    repo.users["user-1"] = User{ID: "user-1", Email: "alice@example.com", Name: "Alice"}

    h := NewUserHandler(NewUserService(repo))

    tests := []struct {
        name       string
        userID     string
        wantStatus int
        wantEmail  string
    }{
        {
            name:       "existing user",
            userID:     "user-1",
            wantStatus: http.StatusOK,
            wantEmail:  "alice@example.com",
        },
        {
            name:       "unknown user",
            userID:     "missing",
            wantStatus: http.StatusNotFound,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(http.MethodGet, "/users/"+tt.userID, nil)
            rec := httptest.NewRecorder()

            h.GetUser(rec, req)   // вызвать хэндлер напрямую

            res := rec.Result()
            assert.Equal(t, tt.wantStatus, res.StatusCode)

            if tt.wantEmail != "" {
                var got UserResponse
                require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
                assert.Equal(t, tt.wantEmail, got.Email)
            }
        })
    }
}
```

**Пробросить path-параметры** (если роутер кладёт их в контекст, например chi или net/http 1.22):

```go
// net/http >= 1.22 — PathValue
req := httptest.NewRequest(http.MethodGet, "/users/user-1", nil)
req.SetPathValue("id", "user-1")   // только Go 1.22+
rec := httptest.NewRecorder()
h.GetUser(rec, req)
```

```go
// chi — передать через контекст
req = req.WithContext(chi.NewRouteContext())
rctx := chi.RouteContext(req.Context())
rctx.URLParams.Add("id", "user-1")
```

---

## httptest.NewServer — тест HTTP-клиента

Когда нужно проверить как *ваш код* ходит во внешний HTTP-сервис — поднять фейковый сервер через `httptest.NewServer`.

```go
func TestExchangeRateClient_GetRate(t *testing.T) {
    // Фейковый сервер, который имитирует внешний API
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/v1/rates/USD/EUR", r.URL.Path)
        assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{
            "from": "USD",
            "to":   "EUR",
            "rate": 0.92,
        })
    }))
    defer srv.Close()

    client := NewExchangeRateClient(srv.URL, "test-token")  // подменить base URL

    rate, err := client.GetRate(context.Background(), "USD", "EUR")
    require.NoError(t, err)
    assert.InDelta(t, 0.92, rate, 0.001)
}
```

### Тестирование retry и ошибок сети

```go
func TestExchangeRateClient_RetryOnServerError(t *testing.T) {
    callCount := 0
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        if callCount < 3 {
            w.WriteHeader(http.StatusServiceUnavailable)
            return
        }
        json.NewEncoder(w).Encode(map[string]any{"rate": 0.92})
    }))
    defer srv.Close()

    client := NewExchangeRateClient(srv.URL, "token", WithRetries(3))

    _, err := client.GetRate(context.Background(), "USD", "EUR")
    require.NoError(t, err)
    assert.Equal(t, 3, callCount)
}
```

### TLS-сервер

```go
srv := httptest.NewTLSServer(handler)
defer srv.Close()

// srv.Client() уже содержит нужный TLS config
client := srv.Client()
resp, err := client.Get(srv.URL + "/endpoint")
```

---

## Тестирование middleware

Middleware — функция `func(http.Handler) http.Handler`. Тестировать отдельно от конкретного хэндлера.

```go
// Middleware логирует и пишет request-id в ответ
func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get("X-Request-ID")
        if id == "" {
            id = uuid.New().String()
        }
        w.Header().Set("X-Request-ID", id)
        ctx := context.WithValue(r.Context(), requestIDKey, id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func TestRequestIDMiddleware(t *testing.T) {
    t.Run("passes through existing request id", func(t *testing.T) {
        var capturedID string
        inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            capturedID = r.Context().Value(requestIDKey).(string)
            w.WriteHeader(http.StatusOK)
        })

        wrapped := RequestIDMiddleware(inner)
        req := httptest.NewRequest(http.MethodGet, "/", nil)
        req.Header.Set("X-Request-ID", "my-id-123")
        rec := httptest.NewRecorder()

        wrapped.ServeHTTP(rec, req)

        assert.Equal(t, "my-id-123", capturedID)
        assert.Equal(t, "my-id-123", rec.Header().Get("X-Request-ID"))
    })

    t.Run("generates request id when missing", func(t *testing.T) {
        inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
        })

        wrapped := RequestIDMiddleware(inner)
        req := httptest.NewRequest(http.MethodGet, "/", nil)
        rec := httptest.NewRecorder()

        wrapped.ServeHTTP(rec, req)

        id := rec.Header().Get("X-Request-ID")
        assert.NotEmpty(t, id)
    })
}
```

### Middleware цепочка

```go
func TestAuthMiddleware_RejectsUnauthorized(t *testing.T) {
    inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    wrapped := AuthMiddleware(tokenVerifier)(inner)

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    // Без Authorization header
    rec := httptest.NewRecorder()

    wrapped.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

---

## Тест через полный router

Когда важно проверить что роутинг работает корректно — регистрация маршрутов, метод, path.

```go
// NewRouter — функция которая собирает весь http.Handler вашего приложения
func NewRouter(userSvc UserService, authSvc AuthService) http.Handler {
    mux := http.NewServeMux()
    h := NewUserHandler(userSvc)
    mux.HandleFunc("GET /users/{id}", h.GetUser)
    mux.HandleFunc("POST /users", h.CreateUser)
    // middleware применяется ко всему
    return LoggingMiddleware(AuthMiddleware(authSvc)(mux))
}

func TestRouter_GetUser(t *testing.T) {
    userSvc := &stubUserService{
        user: &User{ID: "123", Email: "alice@example.com"},
    }
    authSvc := &stubAuthService{valid: true}

    router := NewRouter(userSvc, authSvc)

    req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
    req.Header.Set("Authorization", "Bearer valid-token")
    rec := httptest.NewRecorder()

    router.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_MethodNotAllowed(t *testing.T) {
    router := NewRouter(&stubUserService{}, &stubAuthService{valid: true})

    req := httptest.NewRequest(http.MethodDelete, "/users/123", nil)
    rec := httptest.NewRecorder()

    router.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
```

---

## JSON request / response паттерн

Вспомогательные функции убирают boilerplate из тестов.

```go
// Хелперы для тестов — в файле testutil_test.go или internal/testutil

func makeJSONRequest(t *testing.T, method, path string, body any) *http.Request {
    t.Helper()
    var buf strings.Builder
    if body != nil {
        if err := json.NewEncoder(&buf).Encode(body); err != nil {
            t.Fatal(err)
        }
    }
    req := httptest.NewRequest(method, path, strings.NewReader(buf.String()))
    req.Header.Set("Content-Type", "application/json")
    return req
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
    t.Helper()
    var v T
    if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
        t.Fatalf("decode response: %v", err)
    }
    return v
}

// Использование
func TestCreateUserHandler(t *testing.T) {
    h := NewUserHandler(NewUserService(newFakeUserRepo()))

    req := makeJSONRequest(t, http.MethodPost, "/users", CreateUserRequest{
        Email: "alice@example.com",
        Name:  "Alice",
    })
    rec := httptest.NewRecorder()

    h.CreateUser(rec, req)

    require.Equal(t, http.StatusCreated, rec.Code)

    got := decodeJSON[UserResponse](t, rec)
    assert.Equal(t, "alice@example.com", got.Email)
    assert.NotEmpty(t, got.ID)
}
```

---

## Тестирование ошибок и статус-кодов

```go
func TestCreateUserHandler_ValidationError(t *testing.T) {
    h := NewUserHandler(NewUserService(newFakeUserRepo()))

    req := makeJSONRequest(t, http.MethodPost, "/users", CreateUserRequest{
        Email: "not-an-email",  // невалидный
    })
    rec := httptest.NewRecorder()

    h.CreateUser(rec, req)

    assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

    var errResp ErrorResponse
    require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
    assert.Contains(t, errResp.Message, "email")
}

func TestCreateUserHandler_DuplicateEmail(t *testing.T) {
    repo := newFakeUserRepo()
    h := NewUserHandler(NewUserService(repo))

    body := CreateUserRequest{Email: "alice@example.com", Name: "Alice"}

    // Первый запрос — успешно
    req := makeJSONRequest(t, http.MethodPost, "/users", body)
    rec := httptest.NewRecorder()
    h.CreateUser(rec, req)
    require.Equal(t, http.StatusCreated, rec.Code)

    // Второй запрос — конфликт
    req = makeJSONRequest(t, http.MethodPost, "/users", body)
    rec = httptest.NewRecorder()
    h.CreateUser(rec, req)
    assert.Equal(t, http.StatusConflict, rec.Code)
}
```
