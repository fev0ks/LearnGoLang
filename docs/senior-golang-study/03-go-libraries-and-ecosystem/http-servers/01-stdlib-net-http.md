# stdlib net/http

Go 1.22 существенно улучшил стандартный роутер — для многих сервисов он теперь достаточен без сторонних зависимостей.

## Содержание

- [ServeMux до и после 1.22](#servemux-до-и-после-122)
- [Routing patterns](#routing-patterns)
- [Handler и HandlerFunc](#handler-и-handlerfunc)
- [Middleware pattern](#middleware-pattern)
- [Чтение запроса и запись ответа](#чтение-запроса-и-запись-ответа)
- [Полный пример сервера](#полный-пример-сервера)
- [Когда stdlib достаточно](#когда-stdlib-достаточно)

---

## ServeMux до и после 1.22

До Go 1.22 `http.ServeMux` умел только prefix-matching по path. Path params — не поддерживались, метод в паттерне — не поддерживался.

```go
// до 1.22 — нет path params, нет метода в паттерне
mux := http.NewServeMux()
mux.HandleFunc("/users/", usersHandler)  // ловит /users/, /users/123, /users/abc/extra
```

Начиная с Go 1.22 `ServeMux` поддерживает:
- **Method prefix**: `GET /path`, `POST /path`
- **Path parameters**: `{id}` — точное совпадение одного сегмента
- **Wildcard remainder**: `{rest...}` — остаток пути

```go
// Go 1.22+
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUserHandler)
mux.HandleFunc("POST /users", createUserHandler)
mux.HandleFunc("DELETE /users/{id}", deleteUserHandler)
```

---

## Routing patterns

```go
mux.HandleFunc("GET /api/v1/users", listUsers)
mux.HandleFunc("GET /api/v1/users/{id}", getUser)
mux.HandleFunc("PUT /api/v1/users/{id}", updateUser)

// {id...} — захватывает всё оставшееся (включая слэши)
mux.HandleFunc("GET /files/{path...}", serveFile)

// Фиксированный путь без метода — любой метод
mux.HandleFunc("/healthz", healthCheck)

// Host-based routing
mux.HandleFunc("api.example.com/users", apiUsersHandler)
```

Извлечение path params:
```go
func getUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")    // Go 1.22+
    // ...
}
```

**Приоритет паттернов**: более специфичный паттерн выигрывает.
```go
mux.HandleFunc("GET /users/{id}", getUser)
mux.HandleFunc("GET /users/me", getCurrentUser)  // точное совпадение выигрывает у {id}
```

---

## Handler и HandlerFunc

```go
// Интерфейс
type Handler interface {
    ServeHTTP(ResponseWriter, *Request)
}

// Адаптер — приводит func(w, r) к интерфейсу Handler
type HandlerFunc func(ResponseWriter, *Request)
func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) { f(w, r) }
```

Любая структура, реализующая `ServeHTTP`, — handler:
```go
type UserHandler struct {
    repo UserRepository
}

func (h *UserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // h.repo доступен через closure
}

mux.Handle("GET /users/{id}", &UserHandler{repo: repo})
```

---

## Middleware pattern

Middleware в stdlib — функция `func(http.Handler) http.Handler`.

```go
func Logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
    })
}

func Auth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if !validateToken(token) {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Цепочка middleware — вложенный вызов:
```go
handler := Logging(Auth(mux))
// или через helper-функцию
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        h = middlewares[i](h)
    }
    return h
}

handler := Chain(mux, Logging, Auth, RequestID)
```

Передача данных через context:
```go
type contextKey string

const userIDKey contextKey = "userID"

func Auth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID := extractUserID(r)
        ctx := context.WithValue(r.Context(), userIDKey, userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func getUser(w http.ResponseWriter, r *http.Request) {
    userID := r.Context().Value(userIDKey).(string)
}
```

---

## Чтение запроса и запись ответа

```go
// Декодирование JSON body
func createUser(w http.ResponseWriter, r *http.Request) {
    var input CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        http.Error(w, "invalid JSON", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    // Query params
    page := r.URL.Query().Get("page")

    // Headers
    contentType := r.Header.Get("Content-Type")
    _ = contentType

    // Запись JSON ответа
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(UserResponse{ID: "123"})
}
```

Хелпер для JSON-ответов (обычно делают в каждом проекте):
```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(v); err != nil {
        // log error — header уже отправлен, изменить нельзя
    }
}

func writeError(w http.ResponseWriter, status int, msg string) {
    writeJSON(w, status, map[string]string{"error": msg})
}
```

---

## Полный пример сервера

```go
package main

import (
    "context"
    "encoding/json"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)

type Server struct {
    mux  *http.ServeMux
    repo UserRepository
}

func NewServer(repo UserRepository) *Server {
    s := &Server{
        mux:  http.NewServeMux(),
        repo: repo,
    }
    s.routes()
    return s
}

func (s *Server) routes() {
    s.mux.HandleFunc("GET /healthz", s.handleHealth)
    s.mux.HandleFunc("GET /api/v1/users", s.handleListUsers)
    s.mux.HandleFunc("GET /api/v1/users/{id}", s.handleGetUser)
    s.mux.HandleFunc("POST /api/v1/users", s.handleCreateUser)
}

func (s *Server) Handler() http.Handler {
    return Chain(s.mux,
        RequestID,
        Logging,
        Recovery,
    )
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    user, err := s.repo.Get(r.Context(), id)
    if err != nil {
        writeError(w, http.StatusNotFound, "user not found")
        return
    }
    writeJSON(w, http.StatusOK, user)
}

func main() {
    repo := NewUserRepository()
    srv := &http.Server{
        Addr:         ":8080",
        Handler:      NewServer(repo).Handler(),
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            slog.Error("server error", "err", err)
            os.Exit(1)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    srv.Shutdown(ctx)
}
```

---

## Когда stdlib достаточно

**Подходит:**
- Внутренние сервисы без сложной маршрутизации
- Сервисы с небольшим числом эндпоинтов (< 20-30)
- Команды, которые хотят минимум зависимостей
- Простые API без сложного binding/validation
- Go 1.22+ — уже есть path params и method routing

**Не хватит:**
- Нужны subrouters с shared middleware для группы маршрутов
- Сложная иерархия роутов (`/api/v1/users/{userID}/posts/{postID}`)
- Удобный binding + validation из коробки
- Много built-in middleware (rate limiting, CORS, compression)

**Ключевой вопрос**: если нужно только "настроить маршруты и middleware" — stdlib 1.22 справится. Если нужна экосистема готовых middleware и удобный binding — подойдут chi или gin.
