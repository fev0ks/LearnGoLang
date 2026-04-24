# HTTP Server in Go

`net/http` — стандартная библиотека, которой хватает для production серверов. Знание её internals объясняет почему third-party фреймворки (Chi, Gin, Echo) добавляют так мало.

---

## Базовый сервер

```go
func main() {
    mux := http.NewServeMux()
    
    mux.HandleFunc("GET /users/{id}", getUser)
    mux.HandleFunc("POST /users", createUser)
    mux.HandleFunc("GET /health", healthCheck)
    
    srv := &http.Server{
        Addr:         ":8080",
        Handler:      mux,
        ReadTimeout:  5 * time.Second,   // max время чтения request
        WriteTimeout: 10 * time.Second,  // max время записи response
        IdleTimeout:  60 * time.Second,  // max время keep-alive
    }
    
    log.Println("starting server on :8080")
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Fatal(err)
    }
}
```

### Path patterns (Go 1.22+)

Go 1.22 добавил method-based routing и path wildcards:

```go
// Метод в паттерне
mux.HandleFunc("GET /users/{id}", getUser)    // только GET
mux.HandleFunc("DELETE /users/{id}", deleteUser)

// Wildcard
mux.HandleFunc("GET /files/{path...}", serveFile) // {path...} = catch-all

// Извлечение параметра
func getUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")  // Go 1.22+
    // ...
}
```

---

## Middleware chain

Middleware — функция `func(http.Handler) http.Handler`.

```go
type Middleware func(http.Handler) http.Handler

// Chain: применить middleware в порядке m1 → m2 → m3 → handler
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        h = middlewares[i](h)
    }
    return h
}
```

### Примеры middleware

```go
// Логирование
func Logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        // Wrapping ResponseWriter чтобы поймать статус
        lw := &logResponseWriter{ResponseWriter: w, status: http.StatusOK}
        next.ServeHTTP(lw, r)
        
        log.Printf("%s %s %d %v", r.Method, r.URL.Path, lw.status, time.Since(start))
    })
}

type logResponseWriter struct {
    http.ResponseWriter
    status int
}
func (lw *logResponseWriter) WriteHeader(code int) {
    lw.status = code
    lw.ResponseWriter.WriteHeader(code)
}

// Recovery
func Recovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                log.Printf("panic: %v\n%s", err, debug.Stack())
                http.Error(w, "internal server error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// Request ID
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reqID := r.Header.Get("X-Request-ID")
        if reqID == "" {
            reqID = generateID()
        }
        ctx := context.WithValue(r.Context(), requestIDKey, reqID)
        w.Header().Set("X-Request-ID", reqID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Auth
func Auth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if token == "" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        userID, err := validateToken(token)
        if err != nil {
            http.Error(w, "invalid token", http.StatusUnauthorized)
            return
        }
        ctx := context.WithValue(r.Context(), userIDKey, userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Применение
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser)

handler := Chain(mux,
    Recovery,
    RequestID,
    Logging,
    Auth,
)
```

---

## Timeouts: зачем и какие

```
Client → [ReadHeaderTimeout] → Headers parsed
       → [ReadTimeout]       → Full request body read
       → [Handler runs]
       → [WriteTimeout]      → Response sent
       
Connection idle → [IdleTimeout] → Connection closed
```

```go
srv := &http.Server{
    ReadHeaderTimeout: 2 * time.Second,  // защита от Slowloris attack
    ReadTimeout:       5 * time.Second,  // включает ReadHeaderTimeout
    WriteTimeout:      10 * time.Second, // от конца request до конца response
    IdleTimeout:       60 * time.Second, // keep-alive timeout
}
```

**Почему важны таймауты:**
- `ReadTimeout` защищает от медленных клиентов, которые тянут соединение
- `WriteTimeout` предотвращает зависание горутины при медленном клиенте
- `IdleTimeout` освобождает соединения keep-alive от неактивных клиентов
- Без таймаутов → goroutine leak под нагрузкой

---

## Graceful shutdown

```go
func run() error {
    srv := &http.Server{
        Addr:    ":8080",
        Handler: buildHandler(),
    }
    
    // Канал для сигналов ОС
    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
    
    // Запускаем сервер в горутине
    errCh := make(chan error, 1)
    go func() {
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
    }()
    
    // Ждём сигнала или ошибки
    select {
    case err := <-errCh:
        return fmt.Errorf("server error: %w", err)
    case sig := <-stop:
        log.Printf("received signal %v, shutting down", sig)
    }
    
    // Graceful shutdown: ждём завершения in-flight запросов
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        return fmt.Errorf("shutdown: %w", err)
    }
    log.Println("server stopped gracefully")
    return nil
}
```

`srv.Shutdown(ctx)`:
1. Прекращает принимать новые соединения
2. Закрывает idle соединения
3. Ждёт завершения активных запросов (до timeout)

---

## JSON response helpers

```go
// Типичные helper функции
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(v); err != nil {
        log.Printf("writeJSON encode error: %v", err)
    }
}

func writeError(w http.ResponseWriter, status int, msg string) {
    writeJSON(w, status, map[string]string{"error": msg})
}

// Handler
func getUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    
    user, err := userRepo.FindByID(r.Context(), id)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            writeError(w, http.StatusNotFound, "user not found")
            return
        }
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }
    
    writeJSON(w, http.StatusOK, user)
}
```

---

## Читать request body безопасно

```go
func createUser(w http.ResponseWriter, r *http.Request) {
    // Ограничиваем размер тела
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
    
    var req CreateUserRequest
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields() // строгий режим
    
    if err := dec.Decode(&req); err != nil {
        var maxBytesErr *http.MaxBytesError
        if errors.As(err, &maxBytesErr) {
            writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
            return
        }
        writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
        return
    }
    
    // Валидация
    if req.Name == "" {
        writeError(w, http.StatusBadRequest, "name is required")
        return
    }
    
    // ...
}
```

---

## Важные детали `net/http`

### Каждый запрос — отдельная горутина

`http.Server` запускает горутину на каждое соединение. При keep-alive — одна горутина на соединение, мультиплексирует все запросы этого клиента.

### `http.Handler` интерфейс

```go
type Handler interface {
    ServeHTTP(ResponseWriter, *Request)
}
```

Всё — Handler: `http.ServeMux`, `http.HandlerFunc`, кастомный тип. Это делает composition тривиальным.

### Не писать в `w` после `WriteHeader`

```go
// Плохо
func handler(w http.ResponseWriter, r *http.Request) {
    if err != nil {
        http.Error(w, "bad request", 400)
        // WriteHeader и тело уже отправлены
        return // ОБЯЗАТЕЛЬНО return после записи ошибки
    }
    json.NewEncoder(w).Encode(result) // была бы двойная запись без return
}
```

### `http.ResponseWriter` не thread-safe

Не обращайся к `ResponseWriter` из горутин — только из горутины, обслуживающей запрос.

---

## Организация кода (типичная структура)

```
cmd/
  api/
    main.go           ← создаём server, wiring

internal/
  api/
    server.go         ← http.Server + routing
    middleware/
      auth.go
      logging.go
      recovery.go
    handlers/
      users.go        ← handler functions
      orders.go
  service/
    user_service.go   ← бизнес-логика
  repo/
    user_repo.go      ← БД
```

```go
// internal/api/server.go
type Server struct {
    http *http.Server
    svc  *service.UserService
}

func New(svc *service.UserService) *Server {
    s := &Server{svc: svc}
    mux := http.NewServeMux()
    
    mux.HandleFunc("GET /users/{id}", s.getUser)
    mux.HandleFunc("POST /users", s.createUser)
    
    handler := middleware.Chain(mux,
        middleware.Recovery,
        middleware.Logging,
        middleware.RequestID,
    )
    
    s.http = &http.Server{
        Handler:      handler,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    return s
}

func (s *Server) Start(addr string) error {
    s.http.Addr = addr
    return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
    return s.http.Shutdown(ctx)
}
```

---

## Interview-ready answer

**Q: Что нужно обязательно настроить для production HTTP сервера в Go?**

Четыре обязательных вещи:
1. **Таймауты**: `ReadTimeout`, `WriteTimeout`, `IdleTimeout` — без них goroutine leak под нагрузкой.
2. **Graceful shutdown**: `signal.Notify` + `srv.Shutdown(ctx)` — чтобы in-flight запросы завершились.
3. **Recovery middleware**: recover от паники в handlers — иначе один panic крашит goroutine запроса.
4. **MaxBytesReader**: ограничить размер request body — защита от DoS с огромным body.

**Q: Как работает middleware в `net/http`?**

Middleware — функция `func(http.Handler) http.Handler`. Оборачивает handler: делает что-то до и/или после вызова `next.ServeHTTP`. Все middleware — обычные функции без магии, composable через Chain helper. Порядок важен: Recovery должен быть первым (внешним), иначе паника выше него не поймается.
