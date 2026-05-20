# Обёртки поведения

Три паттерна, объединённые общей идеей: **обернуть существующий код** дополнительным поведением, не меняя его реализацию. Все они работают через интерфейсы или функциональные типы.

- **Middleware** — chain обёрток вокруг HTTP/gRPC handler'а
- **Adapter** — изоляция внешнего SDK за domain-интерфейсом
- **Decorator** — расширение поведения через тот же интерфейс

Они **похожи** структурно, но решают разные задачи. Middleware специфичен для request/response chain'ов. Adapter — про конвертацию внешнего API. Decorator — про добавление слоёв (кэш, retry, метрики) к одному и тому же интерфейсу.

## Содержание

- [Middleware](#middleware)
- [Adapter](#adapter)
- [Decorator](#decorator)
- [Middleware vs Adapter vs Decorator](#middleware-vs-adapter-vs-decorator)
- [Anti-patterns](#anti-patterns)

---

## Middleware

Идея: обернуть обработчик общей технической логикой без изменения кода handler'а.

```go
// Тип middleware
type Middleware func(http.Handler) http.Handler

// Logging
func Logging(log *slog.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            rw := &responseWriter{ResponseWriter: w}
            next.ServeHTTP(rw, r)
            log.Info("request",
                "method", r.Method,
                "path", r.URL.Path,
                "status", rw.status,
                "duration", time.Since(start),
            )
        })
    }
}

// Auth
func Auth(verifier TokenVerifier) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.Header.Get("Authorization")
            claims, err := verifier.Verify(token)
            if err != nil {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            ctx := context.WithValue(r.Context(), claimsKey, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### Цепочка middleware

```
Request
  │
  ▼
┌─────────────┐
│  Recovery   │  ← паника не роняет сервер
└──────┬──────┘
       ▼
┌─────────────┐
│   Tracing   │  ← открыть span
└──────┬──────┘
       ▼
┌─────────────┐
│   Logging   │  ← залогировать запрос
└──────┬──────┘
       ▼
┌─────────────┐
│    Auth     │  ← проверить токен
└──────┬──────┘
       ▼
┌─────────────┐
│  RateLimit  │  ← ограничить частоту
└──────┬──────┘
       ▼
┌─────────────┐
│   Handler   │  ← бизнес-логика
└─────────────┘
```

### Порядок middleware важен

```go
// Правильный порядок
chain := Alice(
    Recovery(),       // 1. ловить любые паники
    Tracing(),        // 2. трассировать запрос (включая Auth ошибки)
    Logging(log),     // 3. логировать запрос
    Auth(verifier),   // 4. проверить auth
    RateLimit(limiter), // 5. лимит per user (после auth — знаем кто)
)
http.Handle("/api/orders", chain(ordersHandler))
```

**Почему такой порядок:**
- `Recovery` — внешний, чтобы поймать panic из любого внутреннего middleware
- `Tracing/Logging` — раньше Auth, чтобы видеть auth failures в логах
- `Auth` — раньше Rate limit, чтобы лимитировать per-user, не per-IP
- `Rate limit` — последний из middleware, ближе всего к handler

### Что middleware делает хорошо

- **Cross-cutting concerns:** logging, tracing, metrics, auth, rate limit, request ID
- **Recovery:** panic не должна валить сервер
- **Response normalization:** добавить headers (CORS, security)
- **Compression:** gzip перед отправкой

### Что middleware делать НЕ должно

- **Бизнес-логика** — никогда. "Скидка для VIP пользователей" — это handler, не middleware
- **Domain validation** — это handler / use case layer
- **Базовые technical ops** — оставь их где они принадлежат (DB → repository, broker → publisher)

**Типичная ошибка:** класть бизнес-логику в middleware. Middleware — для технических cross-cutting concerns.

### gRPC interceptors

gRPC использует те же концепции, но называет их **interceptors**:

```go
func LoggingInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        start := time.Now()
        resp, err := handler(ctx, req)
        log.Info("grpc request",
            "method", info.FullMethod,
            "duration", time.Since(start),
            "error", err,
        )
        return resp, err
    }
}

// Применение
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(LoggingInterceptor(log), AuthInterceptor(verifier)),
)
```

Тот же паттерн, разные сигнатуры.

---

## Adapter

Идея: привести внешний API к внутреннему интерфейсу, чтобы внешний SDK не проникал в домен.

```
Внешний мир          Adapter               Домен
─────────────────    ──────────────────    ─────────────────
stripe.Client   →    StripeAdapter    →    PaymentProvider
sendgrid.Client →    SendGridAdapter  →    EmailSender
s3.Client       →    S3Adapter        →    FileStorage
```

```go
// Интерфейс домена (не знает о Stripe)
type PaymentProvider interface {
    Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}

// Адаптер (знает о Stripe, изолирует детали)
type StripeAdapter struct {
    client *stripe.Client
}

func (a *StripeAdapter) Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error) {
    params := &stripe.ChargeParams{
        Amount:   stripe.Int64(req.Amount.Cents()),
        Currency: stripe.String(string(req.Currency)),
        Source:   &stripe.SourceParams{Token: stripe.String(req.Token)},
    }
    ch, err := a.client.Charges.New(params)
    if err != nil {
        return ChargeResult{}, mapStripeError(err)  // нормализация ошибок
    }
    return ChargeResult{ID: ch.ID, Status: mapStripeStatus(ch.Status)}, nil
}
```

### Что адаптер делает обязательно

**1. Конвертирует типы** (domain model ↔ external model).

Stripe оперирует своими типами: `*stripe.ChargeParams`, `*stripe.Charge`. Domain имеет свои: `ChargeRequest`, `ChargeResult`. Адаптер mappит между ними.

**2. Нормализует ошибки** (stripe.Error → domain.PaymentError).

Внешний SDK возвращает свои error types. Адаптер mappит на стандартные domain errors:

```go
func mapStripeError(err error) error {
    var stripeErr *stripe.Error
    if errors.As(err, &stripeErr) {
        switch stripeErr.Code {
        case "card_declined":
            return domain.ErrPaymentDeclined
        case "insufficient_funds":
            return domain.ErrInsufficientFunds
        }
    }
    return fmt.Errorf("stripe: %w", err)
}
```

**3. Изолирует изменения внешнего API в одном месте.**

Если Stripe выпустит v2 SDK или ты решишь сменить на PayPal — изменения только в адаптере. Domain и use cases не знают.

### Когда adapter оправдан

- **Внешний платный сервис** (Stripe, SendGrid, Twilio) — может меняться, нужно мокать в тестах
- **Vendor lock-in риск** — хочешь возможность сменить провайдера
- **Сложный SDK** с своими типами — domain должен говорить на своём языке

### Когда adapter лишний

- **Тонкая обёртка над stdlib** — например, `net/http` напрямую обычно нормально
- **Простой внутренний сервис** — внутри одной команды, контролируемый API
- **One-shot integration** — никогда не меняется, не тестируется в изоляции

### Hexagonal architecture

Adapter — центральный паттерн в **hexagonal (ports and adapters) architecture**. Domain определяет "ports" (интерфейсы), adapter'ы соединяют domain с external world. См. [../02-architecture-patterns.md](../02-architecture-patterns.md).

```
                  ┌───────────────────┐
                  │      DOMAIN       │
                  │  (бизнес-логика)  │
                  │                   │
                  │   PaymentProvider │  ← port (интерфейс)
                  │   EmailSender     │
                  └───────────────────┘
                       ↑          ↑
                  StripeAdapter   SendGridAdapter   ← adapters
                       │              │
                   stripe.io      sendgrid.com
```

---

## Decorator

Идея: добавить поведение к существующей реализации без изменения её кода, оборачивая через тот же интерфейс.

```
         ┌──────────────────────────────────────┐
         │   MetricsUserStore                   │
         │   ┌──────────────────────────────┐   │
         │   │   CachedUserStore            │   │
         │   │   ┌──────────────────────┐   │   │
         │   │   │   PostgresUserStore   │   │   │
         │   │   └──────────────────────┘   │   │
         │   └──────────────────────────────┘   │
         └──────────────────────────────────────┘
```

```go
// Кеш-декоратор
type CachedUserStore struct {
    next  UserStore
    cache Cache
    ttl   time.Duration
}

func (s *CachedUserStore) GetByID(ctx context.Context, id int64) (User, error) {
    key := fmt.Sprintf("user:%d", id)
    if user, ok := s.cache.Get(key); ok {
        return user.(User), nil
    }
    user, err := s.next.GetByID(ctx, id)
    if err != nil {
        return User{}, err
    }
    s.cache.Set(key, user, s.ttl)
    return user, nil
}

// Метрики-декоратор
type InstrumentedUserStore struct {
    next    UserStore
    metrics Metrics
}

func (s *InstrumentedUserStore) GetByID(ctx context.Context, id int64) (User, error) {
    start := time.Now()
    user, err := s.next.GetByID(ctx, id)
    s.metrics.RecordDuration("user_store.get_by_id", time.Since(start), err != nil)
    return user, err
}

// Сборка в main.go
var store UserStore = postgres.NewUserStore(db)
store = &CachedUserStore{next: store, cache: redisCache, ttl: 5 * time.Minute}
store = &InstrumentedUserStore{next: store, metrics: metrics}
```

### Типичные применения decorator

| Поведение | Описание |
|---|---|
| Cache | Кешировать результат в Redis/memory |
| Retry | Повторить при transient ошибке |
| Circuit breaker | Не вызывать при высоком error rate |
| Tracing | Добавить span вокруг вызова |
| Metrics | Замерить latency и error rate |
| Logging | Залогировать входные/выходные данные |

### Полный пример — decorator stack

```go
// main.go

// 1. Базовая реализация
var store UserStore = postgres.NewUserStore(db)

// 2. Inner decorators первыми, outer — потом
// Tracing — внутри (логирует только реальные DB calls, не cache hits)
store = &TracedUserStore{next: store, tracer: tracer}

// Retry — следующий слой (ретраит только при DB ошибке, не при cache miss)
store = &RetryingUserStore{next: store, maxRetries: 3}

// Cache — следующий (если есть в cache — не вызывает retry/trace)
store = &CachedUserStore{next: store, cache: redisCache, ttl: 5 * time.Minute}

// Metrics — outer (мерит total time, включая cache hit)
store = &InstrumentedUserStore{next: store, metrics: metrics}

// Теперь store обладает всеми поведениями
```

**Порядок важен:** что снаружи — то выполняется первым. Cache outside retry — cache hit не вызывает retry. Cache inside retry — retry будет включать cache lookup.

### Когда decorator оправдан

- **Одно поведение нужно к нескольким реализациям интерфейса.** Например, кеш для `UserStore`, `OrderStore`, `ProductStore` — три декоратора с одинаковой логикой.
- **Поведения комбинируются разнообразно.** Тестовая среда: `store := postgres + metrics`. Production: `+cache, +retry, +tracing`. Easy mix.
- **Расширение без изменения базы.** Третий разработчик добавил circuit breaker — без правки базового кода.

### Когда decorator избыточен

- **Поведение нужно в одном месте.** Просто добавь в код.
- **Тонкий декоратор без логики.** "Decorator который только пишет log.Println" — лишний слой.

### Decorator vs Middleware

Они **очень похожи**. Разница:

- **Middleware** работает с request/response chain'ами (HTTP, gRPC). Сигнатура фиксированная.
- **Decorator** работает с любым domain-интерфейсом. Может иметь любые методы.

Можно сказать: middleware — это частный случай decorator для HTTP handler'ов.

---

## Middleware vs Adapter vs Decorator

Все три "обёртки", но решают разные задачи:

| | Middleware | Adapter | Decorator |
|---|---|---|---|
| **Цель** | Cross-cutting в request chain | Конвертация external API | Расширение поведения |
| **Интерфейс** | http.Handler / gRPC handler | Domain interface (создаём мы) | Существующий domain interface |
| **Композиция** | Цепочка (chain) | Один между внешним и внутренним | Стек вложенных |
| **Где живёт** | Transport layer | Между transport и domain | Любой слой |
| **Пример** | Logging, auth, rate limit | StripeAdapter, S3Adapter | CachedRepo, InstrumentedRepo |

**Когда что:**
- HTTP-request обвязка → **Middleware**
- Изоляция Stripe/AWS SDK → **Adapter**
- Cache/Retry/Metrics вокруг repository → **Decorator**

В реальном коде они **дополняют** друг друга:
```
Request → [Middleware: auth, log] → Handler → Service → [Decorator: cache, metrics] → Repository → [Adapter: pg, redis] → External
```

---

## Anti-patterns

**1. Middleware с бизнес-логикой.**
```go
// ❌ Скидка для VIP в middleware
func VIPDiscount(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if isVIP(r) { ... apply discount ... }
        next.ServeHTTP(w, r)
    })
}
```
Бизнес-логика принадлежит use case, не middleware.

**2. Decorator без поведения.**
```go
// ❌ Декоратор который просто прокидывает вызов
type LoggingRepo struct { next Repo }
func (r *LoggingRepo) Get(id int) (User, error) {
    log.Println("Get called")  // ← это единственная польза?
    return r.next.Get(id)
}
```
Если декоратор только пишет "called" — не стоит того.

**3. Adapter без нормализации ошибок.**
```go
// ❌ Возвращает stripe.Error напрямую
func (a *StripeAdapter) Charge(...) (Result, error) {
    return ..., err  // вот этот err — *stripe.Error, протекает в domain
}
```
Domain не должен знать про Stripe error types. Mapping обязателен.

**4. Глубокий decorator stack.**
```go
store := base
store = decorator1(store)
store = decorator2(store)
// ... 8 декораторов
```
Отладка становится кошмаром. Если 4+ декоратора — возможно, нужен другой подход (явный сервис с инжектируемыми зависимостями).

**5. Middleware который меняет request body.**
```go
// ❌ Прочитал body, не вернул назад
func ParseJSON(next http.Handler) http.Handler { ... }
```
`http.Request.Body` — single-read stream. Если middleware прочитал — handler получит empty body. Если читаешь — нужно вернуть `io.NopCloser(bytes.NewReader(body))`.

**6. Adapter скрывающий критичные ошибки.**
```go
// ❌ Маппит все Stripe errors на одно "PaymentFailed"
func mapStripeError(err error) error {
    return errors.New("payment failed")  // потеряли тип, нельзя retry vs decline
}
```
Сохраняй semantic — declined vs transient — это разные действия.

---

## Чек-лист

```
□ Middleware занимается только технической логикой?
□ Middleware chain в правильном порядке (Recovery first)?
□ Adapter нормализует ошибки внешнего API на domain types?
□ Adapter изолирует все обращения к внешнему SDK в одном файле?
□ Decorator stack меньше 4 уровней?
□ Каждый decorator имеет реальное поведение, не только pass-through?
□ Порядок decorator'ов осмысленный (cache outside / inside retry в зависимости от семантики)?
```

---

См. также:
- [01-dependencies-and-composition.md](./01-dependencies-and-composition.md) — small interfaces, DI
- [03-domain-and-data-access.md](./03-domain-and-data-access.md) — repository, на которое часто навешиваются decorator'ы
- [../02-architecture-patterns.md](../02-architecture-patterns.md) — hexagonal architecture (где adapter'ы — центральная концепция)
