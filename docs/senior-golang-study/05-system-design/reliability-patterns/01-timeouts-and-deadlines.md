# Timeouts и Deadlines

Каждый внешний вызов без таймаута — потенциальный вектор зависания всего сервиса. Одна медленная зависимость без таймаутов кладёт все goroutines в ожидание и убивает сервис.

## Содержание

- [Timeout vs Deadline](#timeout-vs-deadline)
- [Latency budget](#latency-budget)
- [Deadline propagation](#deadline-propagation)
- [Настройка таймаутов в Go](#настройка-таймаутов-в-go)
- [Антипаттерны](#антипаттерны)

---

## Timeout vs Deadline

**Timeout** — относительный: "подожди не более 500ms".
**Deadline** — абсолютный: "завершись не позже 14:05:00.500".

В Go `context.WithTimeout` создаёт дедлайн под капотом:
```go
// Эти два вызова эквивалентны:
ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
ctx, cancel := context.WithDeadline(parent, time.Now().Add(500*time.Millisecond))
```

Преимущество дедлайнов при propagation: если upstream получил запрос с дедлайном 14:05:00.500, и уже прошло 200ms, downstream получает оставшиеся 300ms — не новый 500ms таймаут.

---

## Latency budget

Latency budget — полное время которое есть на запрос от клиента до ответа. Каждый шаг в цепочке "тратит" часть бюджета.

```
Клиент → SLO: ответить за 1000ms

HTTP handler (10ms overhead)     → 990ms остаток
  Auth service (max 50ms)        → 940ms остаток
  DB query: get user (max 20ms)  → 920ms остаток
  Payment service (max 200ms)    → 720ms остаток
  DB query: save order (max 20ms)→ 700ms остаток
  Response serialization (5ms)   → буфер: 695ms
```

Формализованный latency budget помогает правильно расставить таймауты на каждый вызов. Без него разработчики ставят таймауты наугад.

---

## Deadline propagation

В распределённой системе дедлайн должен течь через всю цепочку вызовов. В Go это делается через `context.Context`.

```go
// Входящий HTTP запрос — установить корневой дедлайн
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
    defer cancel()

    // ctx передаётся во все downstream вызовы
    user, err := h.authService.GetUser(ctx, userID)
    if err != nil {
        // ...
    }
    order, err := h.paymentService.CreateOrder(ctx, orderReq)
    // ...
}
```

```go
// auth service — принимает ctx с уже уменьшенным бюджетом
func (s *AuthService) GetUser(ctx context.Context, id string) (*User, error) {
    // НЕ создаём новый context.WithTimeout(context.Background(), ...)
    // Используем переданный ctx — он уже содержит дедлайн

    // Можно ужесточить (но не расширить) дедлайн:
    subCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
    defer cancel()

    return s.db.QueryRow(subCtx, "SELECT * FROM users WHERE id=$1", id)
}
```

### gRPC deadline propagation

gRPC автоматически передаёт дедлайн из context в заголовке `grpc-timeout`. Сервер получает его и восстанавливает в context:

```go
// Клиент
ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
defer cancel()
resp, err := client.GetUser(ctx, req)
// Заголовок grpc-timeout: 498m (осталось ~498ms) уходит серверу

// Сервер получает ctx с уже установленным дедлайном
func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
    deadline, ok := ctx.Deadline()
    if ok {
        remaining := time.Until(deadline)
        // remaining ≈ 495ms — немного меньше из-за network latency
    }
    // Передавать ctx дальше — propagation работает автоматически
    return s.repo.Get(ctx, req.Id)
}
```

### HTTP deadline propagation

В HTTP дедлайн не стандартизирован, передают кастомным заголовком:

```go
// Отправитель
const deadlineHeader = "X-Deadline"

func withDeadlineHeader(ctx context.Context, req *http.Request) *http.Request {
    if deadline, ok := ctx.Deadline(); ok {
        req.Header.Set(deadlineHeader, deadline.Format(time.RFC3339Nano))
    }
    return req.WithContext(ctx)
}

// Получатель (middleware)
func DeadlinePropagation(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if h := r.Header.Get(deadlineHeader); h != "" {
            if deadline, err := time.Parse(time.RFC3339Nano, h); err == nil {
                // Только если дедлайн ещё не истёк
                if time.Until(deadline) > 0 {
                    ctx, cancel := context.WithDeadline(r.Context(), deadline)
                    defer cancel()
                    r = r.WithContext(ctx)
                }
            }
        }
        next.ServeHTTP(w, r)
    })
}
```

---

## Настройка таймаутов в Go

### HTTP Client

```go
// Никогда не используй http.DefaultClient в production — у него нет таймаута
client := &http.Client{
    Timeout: 5 * time.Second,  // полный таймаут запроса включая чтение body
    Transport: &http.Transport{
        DialContext: (&net.Dialer{
            Timeout:   2 * time.Second,  // TCP соединение
            KeepAlive: 30 * time.Second,
        }).DialContext,
        TLSHandshakeTimeout:   3 * time.Second,
        ResponseHeaderTimeout: 3 * time.Second,  // ждать первый байт ответа
        IdleConnTimeout:       90 * time.Second,
        MaxIdleConns:          100,
        MaxIdleConnsPerHost:   10,
    },
}

// Дополнительный per-request таймаут через context
ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
defer cancel()
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)
```

### PostgreSQL (pgx)

```go
// Таймаут на соединение
poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second

// Таймаут на конкретный запрос — через context
ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
defer cancel()
row := pool.QueryRow(ctx, "SELECT * FROM users WHERE id = $1", id)
```

### Redis (go-redis)

```go
rdb := redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    DialTimeout:  2 * time.Second,
    ReadTimeout:  500 * time.Millisecond,
    WriteTimeout: 500 * time.Millisecond,
    PoolTimeout:  1 * time.Second,  // ждать свободное соединение из пула
})
```

---

## Антипаттерны

**Создавать новый `context.Background()` вместо передачи родительского** — дедлайн теряется, downstream не знает что пора остановиться.

```go
// плохо — новый background context, дедлайн потерян
result, err := s.db.Query(context.Background(), query)

// хорошо — передать ctx из аргумента
result, err := s.db.Query(ctx, query)
```

**Не вызывать cancel()** — утечка goroutine таймера:
```go
// плохо
ctx, _ := context.WithTimeout(parent, 5*time.Second)

// хорошо
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel()
```

**Передавать ctx с истёкшим дедлайном** — downstream сразу получает `context.DeadlineExceeded`. Проверяй перед вызовом:
```go
if ctx.Err() != nil {
    return nil, ctx.Err()  // уже истёк — не тратить ресурсы
}
result, err := callDownstream(ctx, req)
```

**Один глобальный таймаут на всё** — `context.WithTimeout(r.Context(), 30*time.Second)` и везде `ctx` без уточнений. Нет контроля над бюджетом отдельных шагов.

**Слишком короткие таймауты на старте** — лучше начать с более длинных (5-10s), собрать реальные p99 latency данные, потом ужесточить. Агрессивные таймауты на старте вызывают ложные ошибки.
