# DDoS защита и Rate Limiting

DDoS (Distributed Denial of Service) — атака на доступность: цель не украсть данные, а сделать сервис недоступным. Rate limiting — смежный механизм на уровне приложения, который ограничивает злоупотребление API.

## Содержание

- [Слои защиты](#слои-защиты)
- [Rate limiting в Go](#rate-limiting-в-go)
- [Rate limiting по IP](#rate-limiting-по-ip)
- [Rate limiting по пользователю](#rate-limiting-по-пользователю)
- [Ответ при превышении лимита](#ответ-при-превышении-лимита)
- [DDoS на уровне инфраструктуры](#ddos-на-уровне-инфраструктуры)

---

## Слои защиты

```
Internet traffic
  → CDN / Cloud DDoS shield     ← volumetric, SYN flood, L3/L4
  → Load balancer / Proxy       ← connection limits, slow clients
  → API Gateway / Ingress       ← rate limits, auth, WAF
  → Application                 ← per-endpoint limits, backpressure
```

Каждый слой дешевле отбрасывает трафик чем следующий. Backend — последний рубеж, не первый.

**DDoS vs Rate limiting:**
- DDoS — перегрузить инфраструктуру потоком трафика (миллионы rps, volumetric)
- Rate limiting — ограничить API usage: 100 req/min per user, throttling дорогих endpoints

Rate limit помогает против части abuse, но не заменяет DDoS protection на уровне CDN/перimetра.

---

## Rate limiting в Go

Стандартный инструмент — `golang.org/x/time/rate` (token bucket алгоритм):

```go
import "golang.org/x/time/rate"

// rate.NewLimiter(r, b): r — токенов в секунду, b — burst (максимальный всплеск)
limiter := rate.NewLimiter(rate.Limit(100), 200)  // 100 rps, burst до 200

// В хэндлере
if !limiter.Allow() {
    http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
    return
}
```

**Token bucket:** каждую секунду в корзину добавляется `r` токенов (максимум `b`). Каждый запрос тратит 1 токен. Если корзина пуста — запрос отклоняется.

---

## Rate limiting по IP

Глобальный лимитер не подходит — один быстрый клиент не должен блокировать остальных. Нужен лимитер на IP.

```go
import (
    "sync"
    "time"
    "golang.org/x/time/rate"
)

type IPRateLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    r        rate.Limit
    b        int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
    rl := &IPRateLimiter{
        limiters: make(map[string]*rate.Limiter),
        r:        r,
        b:        b,
    }
    // Очищать устаревшие записи
    go rl.cleanup()
    return rl
}

func (rl *IPRateLimiter) Allow(ip string) bool {
    rl.mu.Lock()
    l, ok := rl.limiters[ip]
    if !ok {
        l = rate.NewLimiter(rl.r, rl.b)
        rl.limiters[ip] = l
    }
    rl.mu.Unlock()
    return l.Allow()
}

func (rl *IPRateLimiter) cleanup() {
    ticker := time.NewTicker(10 * time.Minute)
    for range ticker.C {
        rl.mu.Lock()
        for ip, l := range rl.limiters {
            // Удалить если лимитер не использовался (корзина полна)
            if l.Tokens() == float64(rl.b) {
                delete(rl.limiters, ip)
            }
        }
        rl.mu.Unlock()
    }
}

// Middleware
func RateLimitByIP(rl *IPRateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := clientIP(r)
            if !rl.Allow(ip) {
                w.Header().Set("Retry-After", "1")
                http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

func clientIP(r *http.Request) string {
    // За proxy — брать из X-Forwarded-For или X-Real-IP
    if ip := r.Header.Get("X-Real-IP"); ip != "" {
        return ip
    }
    if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
        // Первый IP в списке — клиент (остальные — proxy)
        return strings.Split(fwd, ",")[0]
    }
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

**Важно:** `X-Forwarded-For` можно подделать. Доверять только если трафик приходит через контролируемый proxy. Если есть сомнения — использовать `RemoteAddr`.

---

## Rate limiting по пользователю

Для аутентифицированных API — лимит по `user_id`, а не по IP (пользователи за NAT делят один IP):

```go
func RateLimitByUser(rl *IPRateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Взять user ID из контекста (после auth middleware)
            userID, ok := r.Context().Value(userIDKey).(string)
            if !ok || userID == "" {
                // Анонимные запросы — по IP
                userID = clientIP(r)
            }

            if !rl.Allow(userID) {
                http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### Redis-based rate limiting (распределённый)

In-memory лимитер не работает при нескольких репликах — каждая реплика считает свой лимит. Для горизонтально масштабируемого сервиса нужен Redis:

```go
import "github.com/redis/go-redis/v9"

// Lua скрипт — атомарная операция (не нужен WATCH/MULTI)
const rateLimitScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])

local current = redis.call("INCR", key)
if current == 1 then
    redis.call("EXPIRE", key, window)
end
if current > limit then
    return 0
end
return 1
`

type RedisRateLimiter struct {
    client *redis.Client
    limit  int
    window time.Duration
}

func (rl *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
    result, err := rl.client.Eval(ctx, rateLimitScript,
        []string{"ratelimit:" + key},
        rl.limit,
        int(rl.window.Seconds()),
    ).Int()
    if err != nil {
        // Redis недоступен — fail open (пропустить запрос)
        return true, err
    }
    return result == 1, nil
}
```

**Sliding window vs fixed window:**  
Код выше использует fixed window (сбрасывается каждые N секунд). Пользователь может сделать двойной burst на границе окон. Для строгих лимитов — sliding window через Redis sorted sets.

---

## Ответ при превышении лимита

RFC 6585 определяет `429 Too Many Requests`. Полезные заголовки:

```go
func rateLimitExceeded(w http.ResponseWriter, retryAfter int) {
    w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
    w.Header().Set("X-RateLimit-Limit", "100")
    w.Header().Set("X-RateLimit-Remaining", "0")
    w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Duration(retryAfter)*time.Second).Unix(), 10))
    http.Error(w, `{"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
}
```

---

## DDoS на уровне инфраструктуры

Go-код не поможет при volumetric DDoS — трафик забивает сеть до сервера. Это решается инфраструктурно:

| Слой | Инструменты |
|---|---|
| CDN + edge | Cloudflare, Fastly, Akamai — absorb volumetric |
| Cloud perimeter | GCP Cloud Armor, AWS Shield, Azure DDoS Protection |
| L4 балансировщик | TCP connection limits, SYN cookies |
| Ingress | nginx rate_limit_zone, Envoy rate limiting filter |

**Что должен делать Go-сервис при перегрузке:**

```go
// Graceful degradation — быстро отвечать при перегрузке вместо зависания
func overloadMiddleware(next http.Handler) http.Handler {
    sem := make(chan struct{}, 1000)  // максимум 1000 параллельных запросов
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        select {
        case sem <- struct{}{}:
            defer func() { <-sem }()
            next.ServeHTTP(w, r)
        default:
            // Немедленно отвечать 503 вместо накопления goroutine-ов
            w.Header().Set("Retry-After", "5")
            http.Error(w, "service overloaded", http.StatusServiceUnavailable)
        }
    })
}
```
