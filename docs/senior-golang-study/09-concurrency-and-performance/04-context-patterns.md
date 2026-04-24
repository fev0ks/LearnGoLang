# Context Patterns

`context.Context` — стандартный способ передавать сигналы отмены, таймауты и request-scoped данные через цепочку вызовов. Один из немногих интерфейсов, который стоит изучить до конца.

---

## `context.Background()` vs `context.TODO()`

Оба возвращают пустой non-nil context без cancel / timeout / values. Разница — **семантическая**:

```go
// Background — корневой контекст. Используй:
// - в main/init
// - в тестах как основу для дочерних
// - в долгоживущих горутинах верхнего уровня
ctx := context.Background()

// TODO — заглушка. Используй:
// - когда контекст нужен, но откуда его взять — ещё непонятно
// - при рефакторинге: пометить место, которое нужно исправить
// - в тестах, которые ещё не написаны
ctx := context.TODO()
```

`go vet` и `staticcheck` предупреждают если `context.TODO()` остаётся в production-коде.

---

## WithCancel, WithTimeout, WithDeadline

### `WithCancel` — явная отмена

```go
ctx, cancel := context.WithCancel(parent)
defer cancel() // ВСЕГДА defer cancel() — иначе goroutine leak

go longRunning(ctx)

// Отмена: когда нужно
cancel() // ctx.Done() закроется, ctx.Err() = context.Canceled
```

### `WithTimeout` — относительный таймаут

```go
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel() // нужен даже если timeout сработал — освобождает ресурсы

result, err := doHTTPRequest(ctx, url)
if errors.Is(err, context.DeadlineExceeded) {
    // запрос не уложился в 5 секунд
}
```

### `WithDeadline` — абсолютное время

```go
deadline := time.Now().Add(5 * time.Second)
ctx, cancel := context.WithDeadline(parent, deadline)
defer cancel()

// Эквивалентно WithTimeout, но с абсолютным временем
// Полезно когда deadline вычислен заранее
```

### Разница Timeout vs Deadline

```go
// WithTimeout(parent, 5s) → deadline = time.Now() + 5s
// WithDeadline(parent, t) → deadline = t (абсолютное)

// Если parent уже имеет deadline раньше — он не перезаписывается
parentCtx, _ := context.WithTimeout(ctx, 2*time.Second)
childCtx, _ := context.WithTimeout(parentCtx, 10*time.Second)
// childCtx истечёт через 2 секунды, не 10 — берётся минимум
```

### Проверка deadline

```go
if dl, ok := ctx.Deadline(); ok {
    remaining := time.Until(dl)
    if remaining < 100*time.Millisecond {
        return errors.New("not enough time for operation")
    }
}
```

---

## Propagation: почему ctx первый аргумент, не поле struct

### Правило

```go
// Правильно: ctx — первый параметр каждой функции, выполняющей I/O
func (s *Service) GetUser(ctx context.Context, id int) (*User, error) {
    return s.repo.FindByID(ctx, id)
}

// Неправильно: ctx в поле struct
type Service struct {
    ctx context.Context // антипаттерн
    // ...
}
```

**Почему ctx — параметр, не поле:**

1. **Разные запросы — разные контексты.** Struct существует дольше одного запроса; если сохранить ctx в struct, все будущие запросы получат контекст первого запроса (с его deadline, cancel, values).

2. **Явность:** caller видит, что функция будет учитывать отмену.

3. **Тестируемость:** легко передать `context.Background()` или мок-context.

4. **Go convention:** все stdlib и популярные библиотеки используют `ctx context.Context` как первый параметр.

```go
// Типичный стек вызовов
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // context от HTTP сервера
    
    user, err := h.svc.GetUser(ctx, userID) // передаём дальше
    if err != nil {
        // ...
    }
}

func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
    return s.repo.FindByID(ctx, id) // ещё дальше
}

func (r *UserRepo) FindByID(ctx context.Context, id int) (*User, error) {
    return r.db.QueryRowContext(ctx, query, id).Scan(&u) // в SQL query
}
```

---

## `context.Value` — когда допустимо, когда анти-паттерн

### Синтаксис

```go
// Всегда используй неэкспортируемый тип ключа!
type contextKey string

const (
    requestIDKey contextKey = "request_id"
    userIDKey    contextKey = "user_id"
)

// Store
ctx = context.WithValue(ctx, requestIDKey, "req-123")

// Load
if reqID, ok := ctx.Value(requestIDKey).(string); ok {
    log.Printf("request_id=%s", reqID)
}
```

**Почему неэкспортируемый тип ключа?** Предотвращает коллизии: два пакета не могут случайно использовать одинаковый ключ, так как типы из разных пакетов разные даже при одинаковом underlying value.

### Когда `context.Value` допустимо

1. **Request-scoped метаданные** для observability, не для бизнес-логики:
   - request ID / trace ID
   - authenticated user (только ID, не полный объект)
   - correlation ID для логов

2. **Cross-cutting concerns**, которые нежелательно тащить через все слои как параметры:
   - трейсинг span
   - logger с полями

```go
// Хороший use case: trace ID для логов
type traceIDKey struct{}

func WithTraceID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, traceIDKey{}, id)
}

func TraceIDFrom(ctx context.Context) string {
    id, _ := ctx.Value(traceIDKey{}).(string)
    return id
}
```

### Когда `context.Value` — анти-паттерн

❌ **Бизнес-данные через контекст** — скрывает зависимости, усложняет тестирование:

```go
// Плохо: productID прячется в контексте
ctx = context.WithValue(ctx, "product_id", productID)
price := calculatePrice(ctx) // откуда берёт productID — непонятно

// Хорошо: явный параметр
price := calculatePrice(ctx, productID)
```

❌ **Опциональные параметры функций** — создаёт hidden coupling:

```go
// Плохо
func createOrder(ctx context.Context) error {
    userID := ctx.Value("user_id").(int) // молчаливая зависимость
    // ...
}

// Хорошо
func createOrder(ctx context.Context, userID int) error { ... }
```

### Правило: контекст несёт "кто это вызывает" (identity, tracing), не "что делать" (data)

---

## Отмена и cleanup: `defer cancel()` всегда

```go
// Правило: всегда defer cancel() сразу после создания контекста
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel() // ← сразу, на следующей строке

// Зачем defer cancel() даже когда timeout уже сработал?
// WithTimeout создаёт горутину-таймер внутри.
// Без cancel() горутина-таймер и ресурсы (goroutine, channel) не освобождаются
// до истечения timeout или завершения parent.
// При большом количестве коротких запросов → goroutine leak.
```

### Явная vs неявная отмена

```go
func fetchWithRetry(parent context.Context, url string, maxRetries int) error {
    for attempt := range maxRetries {
        // Создаём timeout для каждой попытки отдельно
        ctx, cancel := context.WithTimeout(parent, 2*time.Second)
        err := fetch(ctx, url)
        cancel() // явный cancel — не defer, чтобы не накапливать
        
        if err == nil {
            return nil
        }
        if errors.Is(err, context.Canceled) {
            return err // parent отменён — прерываем retry
        }
        
        // exponential backoff
        select {
        case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
        case <-parent.Done():
            return parent.Err()
        }
    }
    return errors.New("max retries exceeded")
}
```

---

## context в HTTP сервере

### `r.Context()` — context запроса

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // отменяется когда клиент разрывает соединение
    
    result, err := db.QueryContext(ctx, "SELECT ...")
    if err != nil {
        if errors.Is(err, context.Canceled) {
            // клиент ушёл — тихо игнорируем
            return
        }
        http.Error(w, err.Error(), 500)
        return
    }
    // ...
}
```

### Добавление timeout к запросу

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    // Теперь весь pipeline имеет 30-секундный лимит
    result, err := s.processRequest(ctx, r)
    // ...
}
```

### Клиентские таймауты через context

```go
// http.Client timeout — для всего запроса
client := &http.Client{Timeout: 10 * time.Second}

// context timeout — можно дифференцировать
func callAPI(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
    defer cancel()
    
    req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("api call: %w", err)
    }
    defer resp.Body.Close()
    return nil
}
```

---

## Типичные ошибки

### 1. Сохранение ctx в struct

```go
// Плохо
type Handler struct {
    ctx context.Context // будет устаревшим при следующем запросе
}

// Хорошо
func (h *Handler) Handle(ctx context.Context, req *Request) { ... }
```

### 2. Создание без передачи

```go
// Плохо
func processAll(items []Item) error {
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
    defer cancel()
    
    for _, item := range items {
        if err := process(item); err != nil { // ctx не передаётся!
            return err
        }
    }
    return nil
}

// Хорошо
func processAll(ctx context.Context, items []Item) error {
    ctx, cancel := context.WithTimeout(ctx, time.Minute)
    defer cancel()
    
    for _, item := range items {
        if err := process(ctx, item); err != nil {
            return err
        }
    }
    return nil
}
```

### 3. Использование context.Background() глубоко в стеке

```go
// Плохо: теряем отмену от caller
func (r *Repo) FindByID(id int) (*User, error) {
    ctx := context.Background() // игнорируем отмену!
    return r.db.QueryRowContext(ctx, query, id).Scan(...)
}

// Хорошо
func (r *Repo) FindByID(ctx context.Context, id int) (*User, error) {
    return r.db.QueryRowContext(ctx, query, id).Scan(...)
}
```

### 4. Забытый cancel — goroutine/timer leak

```go
// Плохо
func makeRequest() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    // забыли cancel!
    doHTTP(ctx)
    // таймер живёт ещё 5 секунд даже если запрос завершился за 10мс
}

// Хорошо
func makeRequest() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    doHTTP(ctx)
}
```

---

## Производные контексты — дерево

```go
// Контексты образуют дерево отмены:
// Отмена parent → отмена всех children

root := context.Background()
  ├── reqCtx (WithCancel) — отменяется при disconnect
  │     ├── dbCtx (WithTimeout, 100ms)
  │     └── extCtx (WithTimeout, 200ms)
  └── bgCtx (долгоживущий background job)
        └── jobCtx (WithCancel) — отменяется при shutdown
```

```go
// Пример: request-scoped дерево
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // root для этого запроса

    // Добавляем metadata
    ctx = WithTraceID(ctx, generateTraceID())
    ctx = WithUserID(ctx, extractUserID(r))

    // Создаём timeout для I/O операций
    ioCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
    defer cancel()

    // Передаём дальше
    h.svc.Handle(ioCtx, parseRequest(r))
}
```

---

## `context.WithoutCancel` (Go 1.21+)

Иногда нужно продолжить работу после отмены родительского контекста — например, записать audit log после завершения запроса.

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    result, err := process(ctx, r)
    
    // Даже если ctx отменён (клиент ушёл) — записываем аудит
    auditCtx := context.WithoutCancel(ctx) // наследует values, но не cancel
    go audit.Log(auditCtx, r, result, err)
}
```

---

## Interview-ready answer

**Q: Почему ctx — первый параметр, а не поле struct?**

Struct существует дольше одного запроса. Если сохранить ctx в struct, все последующие запросы получат устаревший контекст первого запроса — с его deadline, значениями, и отменой. Context — это per-request данные, а не per-service. Кроме того, передача через параметр делает зависимость явной и упрощает тестирование.

**Q: Когда использовать context.Value?**

Только для request-scoped метаданных, которые нужны cross-cutting concerns: trace ID, request ID, authenticated user ID для логов. Нельзя передавать через context бизнес-данные — это прячет зависимости, усложняет тестирование, нарушает явность интерфейсов.

**Q: Зачем defer cancel() если timeout всё равно истечёт?**

`WithTimeout` создаёт внутреннюю горутину-таймер и channel. Без `cancel()` эти ресурсы не освобождаются до истечения timeout или отмены parent context. В сервисе с высоким RPS каждый запрос без `cancel()` накапливает goroutine leak — через несколько минут счётчик горутин растёт, увеличивается GC pressure.
