# Задача 2: Rate Limiter

Классическая задача — реализовать ограничение частоты вызовов. Спрашивают почти на каждом Go-собеседовании, потому что в production rate limiter нужен везде: API gateway, защита от abuse, fairness между клиентами.

## Формулировка

> "Реализуй rate limiter — структуру, которая разрешает не более N действий в единицу времени."

Вариации:
- "Сделай middleware которое ограничивает 100 RPS"
- "Token bucket algorithm — реализуй"
- "Распределённый rate limiter через Redis"
- "Rate limiter per-user"

---

## Уточняющие вопросы

1. **Сколько и за какое время?**
   "100 RPS — это глобально или per-IP? Что считать 'RPS' — за последнюю секунду или sliding window?"

2. **Что делать при превышении — блокировать или отбрасывать?**
   - Block (`Wait()`) — клиент ждёт пока появится квота
   - Reject (`Allow()` → false) — сразу отказ, например HTTP 429

3. **Allow burst или strict rate?**
   "Token bucket разрешает burst до размера ведра. Leaky bucket — нет."

4. **Per-key или global?**
   "Per-user, per-IP, per-tenant — нужно несколько limiter'ов."

5. **Single-process или distributed?**
   "В одном процессе — in-memory. Несколько pod'ов в k8s — Redis или другой shared store."

6. **Точность важна?**
   "Token bucket — approximate но быстрый. Sliding window log — точный но память O(N)."

---

## Алгоритмы — теория

### Token Bucket (самый частый)

```
┌─ Bucket (capacity = burst) ─┐
│                              │
│  ●●●● tokens                 │  ← refill rate: N tokens/sec
│                              │
└──────────────────────────────┘

Request:
1. Refill bucket (по времени с прошлого refill'a)
2. Если есть >= 1 token — взять, allow
3. Иначе — reject (или wait пока появится)
```

**Плюсы:** разрешает burst (до capacity), за счёт того что bucket накапливает токены в idle время.
**Минусы:** burst может перегрузить downstream.

### Leaky Bucket

```
Request → [Bucket] → (с фиксированной скоростью N/sec)
              ↓
            Overflow → reject
```

Запросы накапливаются в очереди, выпускаются с постоянной скоростью. Если очередь полна — отбрасываются.

**Плюсы:** выходной поток равномерный, нет burst'ов на downstream.
**Минусы:** не разрешает burst — клиент с накопленной "квотой" не может её использовать.

### Sliding Window Log

Хранить timestamp'ы всех запросов в окне:
```
window = 1s, limit = 100
[t1, t2, ..., t100]  ← если все в последней секунде, reject
```

**Плюсы:** точный.
**Минусы:** память O(limit), медленнее.

### Sliding Window Counter (approximate)

Два counter'а: текущая секунда + предыдущая. Вес предыдущей пропорционален позиции внутри окна:
```
count_now * 0.7 + count_prev * 0.3 ≤ limit
```

**Плюсы:** константная память, точность ~99%.
**Минусы:** approximate.

### Fixed Window

Просто counter за фиксированное окно (текущая секунда). Reset на границе.

**Плюсы:** простой.
**Минусы:** "boundary problem" — 100 req в конце окна + 100 в начале следующего = 200 за 100 мс.

---

## Базовое решение: Token Bucket

```go
package ratelimit

import (
    "sync"
    "time"
)

// TokenBucket — простой token bucket rate limiter.
type TokenBucket struct {
    mu         sync.Mutex
    tokens     float64       // текущее количество (float для accurate refill)
    capacity   float64       // максимум (burst)
    refillRate float64       // tokens per second
    lastRefill time.Time
}

func NewTokenBucket(rate float64, burst float64) *TokenBucket {
    return &TokenBucket{
        tokens:     burst,  // стартуем полным
        capacity:   burst,
        refillRate: rate,
        lastRefill: time.Now(),
    }
}

// Allow возвращает true если запрос разрешён.
// Не блокирует.
func (tb *TokenBucket) Allow() bool {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    // Refill — добавить токены за прошедшее время
    now := time.Now()
    elapsed := now.Sub(tb.lastRefill).Seconds()
    tb.tokens += elapsed * tb.refillRate
    if tb.tokens > tb.capacity {
        tb.tokens = tb.capacity
    }
    tb.lastRefill = now

    if tb.tokens >= 1 {
        tb.tokens--
        return true
    }
    return false
}
```

**Использование:**

```go
limiter := ratelimit.NewTokenBucket(100, 200)  // 100/sec rate, burst 200

if !limiter.Allow() {
    http.Error(w, "rate limit exceeded", 429)
    return
}
// ... обработка
```

**Ключевые моменты:**
- `tokens` хранится как `float64` — для **lazy refill** (не нужен timer, refill при каждом `Allow()`)
- Refill вычисляется как `elapsed * rate` — math, не timer
- `mu` защищает от concurrent access (Allow вызывается из множества goroutine)

---

## Production-grade решение

Используй стандартную библиотеку — `golang.org/x/time/rate`. Она проверенная, эффективная, имеет всё что нужно.

```go
import "golang.org/x/time/rate"

// 100 событий в секунду, burst 200
limiter := rate.NewLimiter(rate.Limit(100), 200)

// Non-blocking check
if !limiter.Allow() {
    return ErrRateLimited
}

// Blocking wait
if err := limiter.Wait(ctx); err != nil {
    return err  // context cancelled или таймаут
}

// Зарезервировать N токенов (для batch обработки)
r := limiter.ReserveN(time.Now(), 5)
if !r.OK() {
    return ErrRateLimited
}
time.Sleep(r.Delay())
// ... выполняй 5 операций
```

### HTTP Middleware

```go
func RateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Limit(rps), burst)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                w.Header().Set("Retry-After", "1")
                http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### Per-key rate limiter

Для per-user / per-IP — map limiter'ов:

```go
type PerKeyLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    rate     rate.Limit
    burst    int

    // Cleanup для idle limiters
    lastSeen map[string]time.Time
}

func NewPerKeyLimiter(rps float64, burst int) *PerKeyLimiter {
    pk := &PerKeyLimiter{
        limiters: make(map[string]*rate.Limiter),
        lastSeen: make(map[string]time.Time),
        rate:     rate.Limit(rps),
        burst:    burst,
    }
    go pk.cleanup()
    return pk
}

func (pk *PerKeyLimiter) Allow(key string) bool {
    pk.mu.Lock()
    defer pk.mu.Unlock()

    lim, ok := pk.limiters[key]
    if !ok {
        lim = rate.NewLimiter(pk.rate, pk.burst)
        pk.limiters[key] = lim
    }
    pk.lastSeen[key] = time.Now()
    return lim.Allow()
}

// cleanup удаляет idle limiters чтобы map не рос бесконечно
func (pk *PerKeyLimiter) cleanup() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        pk.mu.Lock()
        cutoff := time.Now().Add(-10 * time.Minute)
        for key, lastSeen := range pk.lastSeen {
            if lastSeen.Before(cutoff) {
                delete(pk.limiters, key)
                delete(pk.lastSeen, key)
            }
        }
        pk.mu.Unlock()
    }
}

// HTTP middleware с per-IP rate limit
func PerIPRateLimit(rps float64, burst int) func(http.Handler) http.Handler {
    pk := NewPerKeyLimiter(rps, burst)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := clientIP(r)
            if !pk.Allow(ip) {
                http.Error(w, "rate limit exceeded", 429)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

**Ключевой момент:** **cleanup** для idle keys. Без него map растёт бесконечно (memory leak), если IP больше не запрашивает.

---

## Distributed rate limiter (Redis)

Для multi-pod сценария: один пользователь может попасть на любой pod, нужен общий counter.

```go
package ratelimit

import (
    "context"
    "errors"
    "time"

    "github.com/redis/go-redis/v9"
)

// Token bucket в Redis через Lua script (атомарно).
const tokenBucketScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])         -- tokens per second
local capacity = tonumber(ARGV[2])      -- burst
local now = tonumber(ARGV[3])           -- current unix time (seconds)
local requested = tonumber(ARGV[4])     -- requested tokens (обычно 1)

local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1]) or capacity
local last_refill = tonumber(data[2]) or now

-- Refill
local elapsed = math.max(0, now - last_refill)
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = 0
if tokens >= requested then
    tokens = tokens - requested
    allowed = 1
end

redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, math.ceil(capacity / rate * 2))

return allowed
`

type RedisRateLimiter struct {
    client   *redis.Client
    script   *redis.Script
    rate     float64
    capacity float64
}

func NewRedisRateLimiter(c *redis.Client, rate, capacity float64) *RedisRateLimiter {
    return &RedisRateLimiter{
        client:   c,
        script:   redis.NewScript(tokenBucketScript),
        rate:     rate,
        capacity: capacity,
    }
}

func (r *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
    now := time.Now().Unix()
    result, err := r.script.Run(ctx, r.client, []string{key},
        r.rate, r.capacity, now, 1,
    ).Int()
    if err != nil {
        return false, err
    }
    return result == 1, nil
}
```

**Ключевые моменты:**
- **Lua script** — атомарность операции "проверь + уменьши" в одной операции на Redis
- **EXPIRE** — авто-cleanup idle keys
- **Hash в Redis** (HMSET) — храним два поля (tokens + last_refill) в одной key

### Когда нужен distributed

- Multi-pod deployment в Kubernetes (one user → любой pod)
- Strict global limits (e.g., "никогда не более 1000 RPS на наш downstream API")

### Когда in-memory достаточно

- Single pod (development, low scale)
- Soft per-pod limits (если 5 pod'ов, каждый по 100 RPS = approximate 500 RPS глобально, OK)

**Trade-off:** distributed добавляет latency (Redis round-trip ~1ms) и точку failure.

---

## Тесты

```go
func TestTokenBucket_Allow(t *testing.T) {
    tb := NewTokenBucket(10, 5)  // 10/sec, burst 5

    // Первые 5 — должны пройти (burst)
    for i := 0; i < 5; i++ {
        if !tb.Allow() {
            t.Errorf("request %d should be allowed (burst)", i)
        }
    }

    // 6-й — должен быть отвергнут
    if tb.Allow() {
        t.Error("6th request should be rejected")
    }

    // Через 100ms должен накопиться 1 токен (10/sec)
    time.Sleep(150 * time.Millisecond)
    if !tb.Allow() {
        t.Error("after 150ms 1 token should be available")
    }
}

func TestTokenBucket_Concurrent(t *testing.T) {
    tb := NewTokenBucket(100, 100)

    var allowed atomic.Int32
    var wg sync.WaitGroup

    for i := 0; i < 1000; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if tb.Allow() {
                allowed.Add(1)
            }
        }()
    }
    wg.Wait()

    // Burst = 100, поэтому первые 100 должны пройти
    // (refill за время теста минимальный)
    if allowed.Load() > 105 || allowed.Load() < 95 {
        t.Errorf("allowed %d, want ~100", allowed.Load())
    }
}
```

Тестировать **timing-зависимый** код сложно. Дай tolerance в ±5%.

---

## Подводные камни

### 1. Memory leak в per-key limiter

```go
// ❌ Map растёт бесконечно
limiters := make(map[string]*rate.Limiter)
limiters[ip] = rate.NewLimiter(...)
```

Без cleanup для idle keys — каждый новый IP добавляет запись навсегда. Через месяц — миллионы entries.

### 2. Lock contention в high-throughput

```go
// ❌ Все горутины бьются за один mutex
type Limiter struct {
    mu sync.Mutex
    tokens int
}
```

Под нагрузкой 100k RPS один global mutex становится bottleneck'ом. Решения:
- `golang.org/x/time/rate` использует mutex внутри но оптимизирован
- Per-key sharding (lock per-shard)
- Atomic операции вместо mutex (сложнее)

### 3. Не учитывать `r.RemoteAddr` за прокси

```go
// ❌ Все клиенты "из одного IP" если за load balancer
ip := r.RemoteAddr
```

За nginx/AWS ELB — используй `X-Forwarded-For` или `X-Real-IP`. Но осторожно — клиент может подделать. См. [perimeter-and-traffic-protection/01-ddos-protection.md](../../../11-security/perimeter-and-traffic-protection/01-ddos-protection.md).

### 4. Sliding window — память O(N)

```go
// ❌ На high RPS список timestamp'ов огромный
log := []time.Time{}
log = append(log, time.Now())
```

При 10k RPS список — десятки тысяч записей. CPU тратится на slice операции. Лучше:
- Token bucket — O(1) memory
- Sliding window counter — O(1) memory, approximate

### 5. Возвращать только `bool` без `Retry-After`

```go
// ❌ Клиент не знает когда retry
if !limiter.Allow() {
    return ErrRateLimited
}
```

Лучше:
```go
reservation := limiter.Reserve()
if !reservation.OK() {
    return ErrRateLimited
}
delay := reservation.Delay()
w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())))
```

### 6. Distributed rate limiter без atomic operation

```python
# ❌ Не атомарно — race condition
tokens = redis.GET(key)
if tokens > 0:
    redis.DECR(key)  # ← между GET и DECR могла прийти другая goroutine
```

Всегда **Lua script** или native Redis команды (`INCR`).

### 7. Не учитывать context

```go
// ❌ Wait блокирует навсегда
limiter.Wait()

// ✓ С context
if err := limiter.Wait(ctx); err != nil {
    // context cancelled или таймаут
    return err
}
```

### 8. Burst слишком большой

```go
limiter := rate.NewLimiter(100, 10000)  // 10k burst!
```

10k burst означает что **внезапно** может прийти 10k запросов мгновенно. Downstream может не выдержать. Burst обычно = 1-2x от среднего RPS.

### 9. Считать ошибку 429 как "норму"

429 — это **сигнал** что что-то не так. Возможно, клиент глючит, возможно — реальная атака. Логируй и мониторь количество 429.

### 10. Один rate limiter для всех endpoints

```go
// ❌ /api/health и /api/expensive-query — один limiter
```

`/api/health` для k8s probe — не должен лимитироваться. `/api/expensive-query` — должен жёстче чем `/api/list`. Разные limiters для разных endpoints.

---

## Возможные расширения

### 1. Adaptive rate limiting

Лимит динамически меняется в зависимости от latency downstream:
- p99 latency растёт → снижаем rate
- Latency низкая → можем увеличить

См. AWS adaptive rate limiting, Netflix Concurrency Limits.

### 2. Cost-based rate limiting

Не все запросы равны. `GET /list` стоит 1, `POST /transcode-video` стоит 100. Каждый запрос consume'ит разное число токенов:
```go
limiter.AllowN(time.Now(), cost)
```

### 3. Hierarchical limits

- Global: 10k RPS
- Per-tenant: 1k RPS
- Per-user: 100 RPS

Проверка проходит **все три уровня**. Используется в Stripe, AWS API Gateway.

### 4. Different algorithms per endpoint

Token bucket для regular API. Leaky bucket для downstream protection. Sliding window для exact counting (биллинг).

### 5. Distributed с in-memory cache

Кэшировать "у меня есть токены" локально, синхронизироваться с Redis периодически. Меньше latency, чуть менее точно.

### 6. Soft vs hard limit

Soft — warning header, но запрос проходит. Hard — 429. Используется в GitHub API.

### 7. Backpressure для воркеров

Worker pool читает из канала, lim'итер на потребителе:
```go
for task := range tasks {
    limiter.Wait(ctx)
    process(task)
}
```

### 8. Rate limit info в headers

GitHub-style:
```
X-RateLimit-Limit: 5000
X-RateLimit-Remaining: 4999
X-RateLimit-Reset: 1620000000
```

---

## Что важно показать на собеседовании

1. **Знать `golang.org/x/time/rate`** — стандарт де-факто. Не писать свой когда не просят.
2. **Объяснить разницу алгоритмов** — token bucket vs leaky bucket vs sliding window.
3. **Per-key cleanup** — без него memory leak.
4. **Atomic operations в distributed** — Lua script, объяснить зачем.
5. **Context cancellation** — `Wait(ctx)` а не просто `Wait()`.
6. **429 + Retry-After header** — клиенту нужно знать когда retry.
7. **Trade-offs** — burst vs steady, approximate vs exact, in-memory vs distributed.

## Связки

- [DDoS protection](../../../11-security/perimeter-and-traffic-protection/01-ddos-protection.md) — production rate limiter в Go
- [Rate limiting в reliability patterns](../../../05-system-design/reliability-patterns/04-rate-limiting.md) — system design
- [HTTP rate limit middleware](../../../08-networking-and-api/protocols/04-rate-limiting.md) — детальный разбор
- [Rate limiter interview case](../../../05-system-design/interview-cases/03-rate-limiter.md) — system design версия задачи
