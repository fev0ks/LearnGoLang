# Rate Limiting

Rate limiting защищает сервис от перегрузки и обеспечивает fair use между клиентами. Бывает server-side (защищаю себя) и client-side (не перегружаю downstream).

## Содержание

- [Алгоритмы](#алгоритмы)
- [Token Bucket в Go](#token-bucket-в-go)
- [Sliding Window Counter](#sliding-window-counter)
- [Distributed Rate Limiting через Redis](#distributed-rate-limiting-через-redis)
- [gRPC interceptor](#grpc-interceptor)
- [HTTP middleware](#http-middleware)
- [Client-side rate limiting](#client-side-rate-limiting)
- [Антипаттерны](#антипаттерны)

---

## Алгоритмы

### Fixed Window Counter

```
[0s─────10s] requests: 95   → OK
[0s─────10s] requests: 100  → limit hit
[10s────20s] requests: 0    → counter reset
```

Проблема: burst на границе окна. 100 запросов в последние 100ms окна + 100 запросов в первые 100ms следующего = 200 запросов за 200ms при лимите 100/10s.

### Sliding Window Log

Точный алгоритм: храним timestamps всех запросов, считаем сколько попало в окно [now-10s, now]. Точен, но дорого по памяти (O(requests)).

### Sliding Window Counter (рекомендуется)

Компромисс: два fixed window счётчика + интерполяция:
```
rate = prev_count * (1 - elapsed/window) + curr_count
```

Приближённо точен, O(1) по памяти.

### Token Bucket (рекомендуется)

Ведро с токенами. Токены добавляются с постоянной скоростью (rate). Каждый запрос забирает токен. Если токенов нет — отказ или ожидание.

- Позволяет burst до ёмкости ведра (burst)
- Равномерное потребление при нормальной нагрузке
- Простая семантика: "X requests per second, burst up to Y"

### Leaky Bucket

Очередь фиксированного размера. Запросы обрабатываются с постоянной скоростью. Избыток — в очередь или отказ.

Хорош для сглаживания трафика на выходе (outbound rate limiting), менее удобен для ingress.

---

## Token Bucket в Go

`golang.org/x/time/rate` — стандартная реализация token bucket:

```go
import "golang.org/x/time/rate"

// 100 requests/sec, burst до 200
limiter := rate.NewLimiter(rate.Limit(100), 200)

// rate.Every — альтернативный способ задать rate
limiter = rate.NewLimiter(rate.Every(10*time.Millisecond), 200)  // 1 req / 10ms = 100/s

// Проверить без блокировки
if !limiter.Allow() {
    return errors.New("rate limit exceeded")
}

// Зарезервировать токен с deadline (рекомендуется)
ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
defer cancel()
if err := limiter.Wait(ctx); err != nil {
    return err  // context cancelled или deadline exceeded
}

// Для множества токенов (batch запросы)
if !limiter.AllowN(time.Now(), 10) {
    return errors.New("rate limit exceeded")
}
```

### Per-key rate limiting (per user, per IP)

```go
type PerKeyLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    rate     rate.Limit
    burst    int
}

func NewPerKeyLimiter(r rate.Limit, burst int) *PerKeyLimiter {
    return &PerKeyLimiter{
        limiters: make(map[string]*rate.Limiter),
        rate:     r,
        burst:    burst,
    }
}

func (l *PerKeyLimiter) Allow(key string) bool {
    l.mu.Lock()
    limiter, ok := l.limiters[key]
    if !ok {
        limiter = rate.NewLimiter(l.rate, l.burst)
        l.limiters[key] = limiter
    }
    l.mu.Unlock()
    return limiter.Allow()
}
```

**Проблема**: map растёт неограниченно. В production нужна TTL-очистка или LRU cache:

```go
import "github.com/hashicorp/golang-lru/v2/expirable"

// LRU cache с TTL — старые ключи вытесняются автоматически
cache := expirable.NewLRU[string, *rate.Limiter](10000, nil, 1*time.Hour)

func (l *PerKeyLimiter) Allow(key string) bool {
    limiter, ok := cache.Get(key)
    if !ok {
        limiter = rate.NewLimiter(l.rate, l.burst)
        cache.Add(key, limiter)
    }
    return limiter.Allow()
}
```

---

## Sliding Window Counter

Redis-based sliding window — точный счётчик за последние N секунд:

```lua
-- KEYS[1] = "ratelimit:{userID}"
-- ARGV[1] = current timestamp (ms)
-- ARGV[2] = window size (ms)
-- ARGV[3] = limit
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- Удалить старые записи
redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)

-- Посчитать текущие
local count = redis.call('ZCARD', key)

if count < limit then
    -- Добавить текущий запрос
    redis.call('ZADD', key, now, now .. '-' .. math.random())
    redis.call('PEXPIRE', key, window)
    return 1  -- allowed
end
return 0  -- denied
```

```go
const slidingWindowScript = `...`  // lua script выше

var slidingWindowSHA string

func init() {
    // Загрузить скрипт один раз
    sha, err := rdb.ScriptLoad(context.Background(), slidingWindowScript).Result()
    if err != nil {
        panic(err)
    }
    slidingWindowSHA = sha
}

func checkRateLimit(ctx context.Context, userID string, limit int, window time.Duration) (bool, error) {
    now := time.Now().UnixMilli()
    key := "ratelimit:" + userID

    result, err := rdb.EvalSha(ctx, slidingWindowSHA,
        []string{key},
        now,
        window.Milliseconds(),
        limit,
    ).Int()
    if err != nil {
        // При ошибке Redis — пропустить запрос (fail open) или отклонить (fail closed)
        return true, nil  // fail open: Redis упал → не блокируем пользователей
    }
    return result == 1, nil
}
```

---

## Distributed Rate Limiting через Redis

Для нескольких инстансов сервиса локальный in-memory limiter не работает — каждый инстанс считает независимо.

```go
// Token bucket через Redis (INCR + EXPIRE)
func redisTokenBucket(ctx context.Context, rdb *redis.Client, key string, limit int, window time.Duration) (bool, error) {
    pipe := rdb.Pipeline()
    incr := pipe.Incr(ctx, key)
    pipe.Expire(ctx, key, window)
    _, err := pipe.Exec(ctx)
    if err != nil {
        return true, nil  // fail open
    }
    return incr.Val() <= int64(limit), nil
}
```

Или через готовую библиотеку `github.com/go-redis/redis_rate/v10`:

```go
import "github.com/go-redis/redis_rate/v10"

limiter := redis_rate.NewLimiter(rdb)

res, err := limiter.Allow(ctx, "user:"+userID, redis_rate.PerSecond(100))
if err != nil {
    // Redis недоступен
}
if res.Allowed == 0 {
    // Rate limit exceeded
    // res.RetryAfter — через сколько можно повторить
}
```

---

## gRPC interceptor

```go
func RateLimitInterceptor(limiter *rate.Limiter) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        if !limiter.Allow() {
            return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
        }
        return handler(ctx, req)
    }
}

// Per-user rate limiting
func PerUserRateLimitInterceptor(limiters *PerKeyLimiter) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        userID := extractUserID(ctx)
        if !limiters.Allow(userID) {
            return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded for user %s", userID)
        }
        return handler(ctx, req)
    }
}
```

---

## HTTP middleware

```go
func RateLimit(limiter *rate.Limiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                w.Header().Set("Retry-After", "1")
                http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// chi
r := chi.NewRouter()
r.Use(RateLimit(rate.NewLimiter(100, 200)))
```

---

## Client-side rate limiting

Не перегружать downstream сервис — ставить limiter на клиент:

```go
type RateLimitedClient struct {
    client  PaymentClient
    limiter *rate.Limiter
}

func (c *RateLimitedClient) Charge(ctx context.Context, req *ChargeRequest) (*ChargeResponse, error) {
    // Ждать токен перед каждым вызовом
    if err := c.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limiter: %w", err)
    }
    return c.client.Charge(ctx, req)
}
```

Особенно важно для batch-обработки где легко случайно создать burst:

```go
// Обработка 10000 записей с rate limit 100/s
limiter := rate.NewLimiter(100, 100)
for _, record := range records {
    if err := limiter.Wait(ctx); err != nil {
        return err
    }
    go processRecord(ctx, record)
}
```

---

## Антипаттерны

**In-memory rate limiting при нескольких инстансах** — каждый инстанс считает отдельно, реальный лимит = limit × N инстансов.

**Отклонять без `Retry-After`** — клиент не знает когда повторить, делает агрессивный retry:
```go
w.Header().Set("Retry-After", "1")          // через 1 секунду
w.Header().Set("X-RateLimit-Limit", "100")
w.Header().Set("X-RateLimit-Remaining", "0")
w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
```

**Одинаковый лимит для всех клиентов** — внутренние сервисы и внешние пользователи должны иметь разные квоты.

**Fail closed при недоступном Redis** — если Redis упал и все запросы блокируются, сам Redis становится single point of failure. Обычно лучше fail open (пропустить) с алертом.
