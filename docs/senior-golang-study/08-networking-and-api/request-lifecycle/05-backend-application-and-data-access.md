# Backend Application And Data Access

После edge слоя запрос наконец доходит до application backend. Здесь рождается основная application latency — но сначала важно понимать, как Go обрабатывает соединения.

## Содержание

- [Go net/http internals: goroutine model](#go-nethttp-internals-goroutine-model)
- [http.Server: важные параметры](#httpserver-важные-параметры)
- [Middleware chain](#middleware-chain)
- [Context: deadline propagation](#context-deadline-propagation)
- [Доступ к данным и connection pool](#доступ-к-данным-и-connection-pool)
- [Fan-out к downstream сервисам](#fan-out-к-downstream-сервисам)
- [Формирование ответа](#формирование-ответа)
- [Где здесь бывают проблемы](#где-здесь-бывают-проблемы)
- [Interview-ready answer](#interview-ready-answer)

## Go net/http internals: goroutine model

В Go `net/http` каждое TCP-соединение обслуживается отдельной горутиной. Handler вызывается в этой же горутине. При HTTP/2 — одна горутина на соединение, по одной дополнительной на каждый stream (запрос).

```text
HTTP/1.1:
  conn goroutine: read request → run handler → write response → read next request (loop)

HTTP/2:
  conn goroutine: manage HPACK, stream framing
  stream goroutines (per request): run handler concurrently
```

Горутины дешёвые (~2 KB stack), поэтому 10k concurrent connections — нормально для Go. Но если handler делает блокирующий вызов (DB, downstream) — горутина блокируется. Goroutine count растёт пропорционально in-flight запросам.

## http.Server: важные параметры

```go
srv := &http.Server{
    Addr:    ":8080",
    Handler: handler,

    // Время на чтение всего запроса (headers + body).
    // Защита от slow-read атак.
    ReadTimeout: 10 * time.Second,

    // Время на запись ответа клиенту.
    // Должен быть >= максимального времени обработки запроса.
    WriteTimeout: 30 * time.Second,

    // Время keep-alive соединения в idle состоянии.
    IdleTimeout: 120 * time.Second,

    // Максимальный размер header (защита от header injection).
    MaxHeaderBytes: 1 << 20, // 1 MB
}
```

`ReadTimeout` vs `ReadHeaderTimeout`: если выставить только `ReadTimeout`, загрузка больших файлов упрётся в него. Для streaming используй `ReadHeaderTimeout` + отдельный timeout на уровне handler.

Graceful shutdown (обязателен в Kubernetes — детали в [kubernetes/04-probes-and-graceful-shutdown.md](../../../11-devops-and-observability/kubernetes/04-probes-and-graceful-shutdown.md)):

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
defer stop()

go func() {
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Fatal(err)
    }
}()

<-ctx.Done()

shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
srv.Shutdown(shutdownCtx)
```

## Middleware chain

До handler выполняется цепочка middleware. Каждый middleware — wrapper над `http.Handler`:

```go
type Middleware func(http.Handler) http.Handler

func chain(h http.Handler, middlewares ...Middleware) http.Handler {
    // применяем в обратном порядке, чтобы первый middleware был внешним
    for i := len(middlewares) - 1; i >= 0; i-- {
        h = middlewares[i](h)
    }
    return h
}

// Типичный порядок: recover → tracing → logging → timeout → auth → handler
handler := chain(
    mux,
    recoverMiddleware,      // поймать panic, вернуть 500
    tracingMiddleware,      // извлечь/создать trace context
    loggingMiddleware,      // structured request log
    timeoutMiddleware(5*time.Second), // deadline на весь запрос
    authMiddleware,         // проверить JWT/session
)
```

Пример timeout middleware — добавляет deadline к context запроса:

```go
func timeoutMiddleware(d time.Duration) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx, cancel := context.WithTimeout(r.Context(), d)
            defer cancel()
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

Порядок middleware важен:
- `recover` должен быть первым (внешним), чтобы поймать panic из любого последующего слоя.
- `tracing` — раньше `logging`, чтобы в логах уже был trace ID.
- `auth` — позже timeout, чтобы медленный auth-сервис не обходил ограничение.

## Context: deadline propagation

Context с deadline должен пройти через весь стек — от входящего HTTP request до последнего downstream вызова. Это ключевой паттерн для Go backend.

```go
func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // уже содержит deadline из timeoutMiddleware

    // propagate к DB — если request отменён, query тоже отменяется
    order, err := h.repo.CreateOrder(ctx, req)
    if err != nil {
        switch {
        case errors.Is(err, context.DeadlineExceeded):
            http.Error(w, "request timeout", http.StatusGatewayTimeout)
        case errors.Is(err, context.Canceled):
            // клиент отключился — можно просто не отвечать
            return
        default:
            http.Error(w, "internal error", http.StatusInternalServerError)
        }
        return
    }

    // propagate к downstream HTTP-сервису
    notifyReq, _ := http.NewRequestWithContext(ctx, "POST", h.notifyURL, body)
    resp, err := h.client.Do(notifyReq)
    // ...
}
```

Если context не пробрасывать — DB query продолжится после timeout, тратя ресурсы на результат, который уже никому не нужен.

## Доступ к данным и connection pool

### Database connection pool

`database/sql` (и `pgxpool` для PostgreSQL) управляет пулом соединений. Ключевые параметры:

```go
db, _ := sql.Open("pgx", dsn)
db.SetMaxOpenConns(25)      // максимум открытых соединений к БД
db.SetMaxIdleConns(10)      // держать в idle (не закрывать)
db.SetConnMaxLifetime(30 * time.Minute) // пересоздавать соединения
db.SetConnMaxIdleTime(10 * time.Minute) // закрывать долго idle
```

**Connection pool exhaustion** — один из самых частых bottleneck:

```text
Scenario: 25 max connections, каждый запрос держит соединение 500 ms
→ throughput = 25 / 0.5s = 50 req/s максимум
→ при 100 req/s горутины встают в очередь на получение connection
→ latency растёт, затем WriteTimeout срабатывает
→ 504 Gateway Timeout для клиентов
```

Мониторить: `db.Stats().WaitCount` (запросы, ждавшие свободного соединения), `db.Stats().WaitDuration`.

### Redis и другие клиенты

go-redis тоже управляет пулом — `PoolSize` опция. При ошибке Redis и fail-open стратегии бизнес-логика продолжает работу без cache.

## Fan-out к downstream сервисам

Fan-out (несколько параллельных вызовов) быстро съедает latency budget:

```go
// Sequential: latency = A + B + C
userResp, _ := h.userSvc.Get(ctx, userID)
ordersResp, _ := h.orderSvc.List(ctx, userID)
prefsResp, _ := h.prefsSvc.Get(ctx, userID)

// Parallel: latency = max(A, B, C)
var wg sync.WaitGroup
var mu sync.Mutex
var errs []error

wg.Add(3)
go func() { defer wg.Done(); /* fetch user */ }()
go func() { defer wg.Done(); /* fetch orders */ }()
go func() { defer wg.Done(); /* fetch prefs */ }()
wg.Wait()
```

При параллельном fan-out: если один upstream медленный, он держит весь запрос. Используй `context.WithTimeout` с разумным deadline на каждый downstream call.

Паттерн для partial failure — не падать, если некритичные данные не пришли:

```go
// prefs — некритичные, timeout меньше
prefsCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()
prefs, err := h.prefsSvc.Get(prefsCtx, userID)
if err != nil {
    prefs = defaultPrefs // деградация, не ошибка
}
```

## Формирование ответа

Backend устанавливает статус, заголовки и тело. Порядок важен: заголовки должны быть записаны до `WriteHeader`, тело — после.

```go
func (h *Handler) respond(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Request-Id", getRequestID(w))
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}
```

`json.NewEncoder(w).Encode` пишет напрямую в `ResponseWriter` — это streaming, не буферизованный. Для большого JSON это хорошо. Для маленького ответа с нужным `Content-Length` — лучше `json.Marshal` → `w.Write`.

Cache headers для CDN — важны: `Cache-Control: public, max-age=60` позволяет CDN кэшировать и снять нагрузку с origin. `Cache-Control: private` — только browser cache.

## Где здесь бывают проблемы

- **slow SQL query**: N+1 запросы, missing index, lock contention на строке.
- **connection pool exhaustion**: в `db.Stats()` растёт `WaitCount` и `WaitDuration`.
- **context not propagated**: DB/downstream query продолжает работу после timeout.
- **downstream timeout misconfiguration**: timeout в proxy меньше, чем в handler — прокси разрывает соединение раньше, backend тратит ресурсы зря.
- **panic без recover**: без `recoverMiddleware` один panic убивает горутину обработки запроса, Go logss stack trace, клиент получает разрыв соединения (не 500).
- **serialization overhead**: глубокая JSON-сериализация больших объектов может быть CPU-bound. Profile с `pprof`.

## Interview-ready answer

Go `net/http` создаёт одну горутину на TCP-соединение (HTTP/1.1) или на stream (HTTP/2). Middleware chain — последовательные обёртки над `http.Handler`; порядок важен: recover снаружи, auth внутри. Context с deadline из входящего запроса должен пробрасываться во все downstream вызовы — это единственный способ гарантировать отмену операций при timeout. Connection pool exhaustion — частая причина роста latency: `db.Stats().WaitCount` показывает проблему. Fan-out к нескольким сервисам нужно делать параллельно через горутины с общим context deadline, с graceful degradation для некритичных данных.
