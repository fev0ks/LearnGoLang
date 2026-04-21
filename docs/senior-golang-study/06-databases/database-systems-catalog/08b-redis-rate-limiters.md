# Redis Rate Limiters

Четыре алгоритма rate limiting с реализацией на Go + Redis. Каждый — trade-off между точностью, сложностью и потреблением памяти.

## Содержание

- [Зачем rate limiting и где он живёт](#зачем-rate-limiting-и-где-он-живёт)
- [Алгоритм 1: Fixed Window Counter](#алгоритм-1-fixed-window-counter)
- [Алгоритм 2: Sliding Window Log](#алгоритм-2-sliding-window-log)
- [Алгоритм 3: Sliding Window Counter (approximate)](#алгоритм-3-sliding-window-counter-approximate)
- [Алгоритм 4: Token Bucket](#алгоритм-4-token-bucket)
- [Сравнение алгоритмов](#сравнение-алгоритмов)
- [HTTP middleware на Go](#http-middleware-на-го)
- [Практические вопросы при внедрении](#практические-вопросы-при-внедрении)
- [Interview-ready answer](#interview-ready-answer)

## Зачем rate limiting и где он живёт

Rate limiting защищает сервис от перегрузки и злоупотреблений. Типичные применения:
- ограничение числа API-запросов на пользователя/IP/API key;
- защита login endpoint от brute force;
- ограничение expensive операций (email send, export, search);
- SLA enforcement для разных планов (free: 100 req/min, pro: 1000 req/min).

Redis — стандартный выбор для distributed rate limiting: атомарные операции, TTL, низкая latency.

## Алгоритм 1: Fixed Window Counter

Самый простой. Делим время на фиксированные окна (например, по минутам). Считаем запросы в текущем окне.

```text
Окно 10:00–10:01: |||||| 6 запросов
Окно 10:01–10:02: ||     2 запроса
Лимит: 5 в минуту → отклонить с 7-го запроса в первом окне
```

```go
type FixedWindowLimiter struct {
    redis  *redis.Client
    limit  int
    window time.Duration
}

func (l *FixedWindowLimiter) Allow(ctx context.Context, key string) (bool, error) {
    now := time.Now()
    // ключ включает номер текущего окна
    windowKey := fmt.Sprintf("rl:fw:%s:%d", key, now.Truncate(l.window).Unix())

    pipe := l.redis.Pipeline()
    incr := pipe.Incr(ctx, windowKey)
    pipe.Expire(ctx, windowKey, l.window*2) // небольшой запас
    if _, err := pipe.Exec(ctx); err != nil {
        return true, err // fail open при ошибке Redis
    }

    return incr.Val() <= int64(l.limit), nil
}
```

**Проблема: граничный spike**

```text
Лимит: 5 req/min
10:00:59 → 5 запросов (конец окна 10:00)
10:01:00 → 5 запросов (начало окна 10:01)
```

За 2 секунды прошло 10 запросов — в 2 раза больше лимита.

**Когда использовать**: простые случаи, где граничный spike допустим или маловероятен. Очень дёшево по памяти.

## Алгоритм 2: Sliding Window Log

Храним timestamp каждого запроса в Sorted Set. При каждом запросе удаляем старые (вне окна) и считаем оставшиеся.

```text
Запрос в 10:01:30 → удалить всё до 10:00:30 → посчитать оставшееся
```

```go
type SlidingWindowLogLimiter struct {
    redis  *redis.Client
    limit  int
    window time.Duration
}

var slidingLogScript = redis.NewScript(`
    local key    = KEYS[1]
    local now    = tonumber(ARGV[1])
    local window = tonumber(ARGV[2])
    local limit  = tonumber(ARGV[3])
    local req_id = ARGV[4]

    -- удалить запросы вне окна
    redis.call("ZREMRANGEBYSCORE", key, 0, now - window)

    -- посчитать текущие
    local count = redis.call("ZCARD", key)

    if count < limit then
        -- добавить этот запрос
        redis.call("ZADD", key, now, req_id)
        redis.call("EXPIRE", key, math.ceil(window / 1000) + 1)
        return 1
    else
        return 0
    end
`)

func (l *SlidingWindowLogLimiter) Allow(ctx context.Context, key string) (bool, error) {
    now := time.Now().UnixMilli()
    windowMs := l.window.Milliseconds()
    reqID := fmt.Sprintf("%d-%d", now, rand.Int63())

    result, err := slidingLogScript.Run(ctx, l.redis,
        []string{"rl:swl:" + key},
        now, windowMs, l.limit, reqID,
    ).Int()
    if err != nil {
        return true, err // fail open
    }
    return result == 1, nil
}
```

**Плюс**: абсолютно точный — нет граничного spike.

**Минус**: хранит timestamp каждого запроса. При лимите 1000 req/min — до 1000 записей в Sorted Set на каждого пользователя. При миллионах пользователей это значительная память.

**Когда использовать**: когда нужна точность и число пользователей умеренное (тысячи, не миллионы).

## Алгоритм 3: Sliding Window Counter (approximate)

Компромисс между Fixed Window и Sliding Window Log. Используем два соседних окна и взвешенное среднее.

```text
Запрос в момент T, окна по 1 минуте:
elapsed = T - начало_текущего_окна   // например, 45 секунд
weight_prev = 1 - elapsed / window  // 1 - 45/60 = 0.25
approximate_count = prev_window_count * weight_prev + current_window_count
```

Идея: чем ближе к концу окна, тем меньше вес предыдущего окна.

```go
type SlidingWindowCounterLimiter struct {
    redis  *redis.Client
    limit  int
    window time.Duration
}

var slidingCounterScript = redis.NewScript(`
    local curr_key  = KEYS[1]
    local prev_key  = KEYS[2]
    local limit     = tonumber(ARGV[1])
    local now       = tonumber(ARGV[2])
    local window_ms = tonumber(ARGV[3])

    -- вес предыдущего окна
    local elapsed   = now % window_ms
    local weight    = 1 - (elapsed / window_ms)

    local prev_count = tonumber(redis.call("GET", prev_key) or 0)
    local curr_count = tonumber(redis.call("GET", curr_key) or 0)

    local approx = math.floor(prev_count * weight) + curr_count

    if approx < limit then
        local new_count = redis.call("INCR", curr_key)
        if new_count == 1 then
            redis.call("EXPIRE", curr_key, math.ceil(window_ms / 1000) * 2)
        end
        return 1
    else
        return 0
    end
`)

func (l *SlidingWindowCounterLimiter) Allow(ctx context.Context, key string) (bool, error) {
    now := time.Now().UnixMilli()
    windowMs := l.window.Milliseconds()

    // ключи для текущего и предыдущего окна
    currWindow := now / windowMs
    currKey := fmt.Sprintf("rl:swc:%s:%d", key, currWindow)
    prevKey := fmt.Sprintf("rl:swc:%s:%d", key, currWindow-1)

    result, err := slidingCounterScript.Run(ctx, l.redis,
        []string{currKey, prevKey},
        l.limit, now, windowMs,
    ).Int()
    if err != nil {
        return true, err
    }
    return result == 1, nil
}
```

**Погрешность**: теоретически до ~0.003% при равномерном трафике в предыдущем окне. На практике — приемлемо для большинства API rate limiting.

**Память**: всего 2 ключа на пользователя независимо от числа запросов.

**Когда использовать**: production default для большинства API rate limiting. Используют Nginx, Cloudflare, многие API gateway.

## Алгоритм 4: Token Bucket

Bucket с токенами. Токены добавляются с постоянной скоростью `rate` (tokens/second). Каждый запрос потребляет один токен. Если bucket пуст — отказ.

Преимущество перед window-based: **разрешает burst** в пределах размера bucket, при этом долгосрочная скорость ограничена.

```text
rate = 10 tokens/sec, bucket_size = 50
Можно сделать 50 запросов мгновенно (burst),
но затем только 10 в секунду.
```

```go
type TokenBucketLimiter struct {
    redis      *redis.Client
    rate       float64 // tokens per second
    bucketSize float64
}

var tokenBucketScript = redis.NewScript(`
    local key         = KEYS[1]
    local rate        = tonumber(ARGV[1])
    local bucket_size = tonumber(ARGV[2])
    local now         = tonumber(ARGV[3])

    local data = redis.call("HMGET", key, "tokens", "last_refill")
    local tokens      = tonumber(data[1]) or bucket_size
    local last_refill = tonumber(data[2]) or now

    -- пополнить токены пропорционально прошедшему времени
    local elapsed = (now - last_refill) / 1000  -- в секундах
    tokens = math.min(bucket_size, tokens + elapsed * rate)

    if tokens >= 1 then
        tokens = tokens - 1
        redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
        redis.call("EXPIRE", key, math.ceil(bucket_size / rate) + 10)
        return 1
    else
        -- обновить last_refill даже при отказе (чтобы не накапливались токены пока ключ "спит")
        redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
        redis.call("EXPIRE", key, math.ceil(bucket_size / rate) + 10)
        return 0
    end
`)

func (l *TokenBucketLimiter) Allow(ctx context.Context, key string) (bool, error) {
    now := time.Now().UnixMilli()

    result, err := tokenBucketScript.Run(ctx, l.redis,
        []string{"rl:tb:" + key},
        l.rate, l.bucketSize, now,
    ).Int()
    if err != nil {
        return true, err
    }
    return result == 1, nil
}
```

**Почему Lua-скрипт**: операция "прочитать tokens и last_refill, вычислить, записать обратно" должна быть атомарной. Без Lua — race condition между несколькими инстансами сервиса.

**Когда использовать**: когда нужно разрешить burst (например, загрузка SDK после долгого молчания), но ограничить sustained rate.

## Сравнение алгоритмов

| Алгоритм | Граничный spike | Память на ключ | Burst | Сложность |
|---|---|---|---|---|
| Fixed Window Counter | да | O(1) | нет | минимальная |
| Sliding Window Log | нет | O(лимит) | нет | средняя |
| Sliding Window Counter | ~нет | O(1) | нет | средняя |
| Token Bucket | нет | O(1) | да | средняя |

**Рекомендации**:
- API rate limiting (100 req/min per user) — **Sliding Window Counter**: точность без memory overhead.
- Login brute force protection — **Fixed Window Counter**: простота, spike не критичен.
- Streaming / bulk upload ограничение — **Token Bucket**: разрешает burst начала сессии.
- Высокоточный биллинг по запросам — **Sliding Window Log**: точность важнее памяти.

## HTTP middleware на Go

```go
type RateLimiterMiddleware struct {
    limiter interface {
        Allow(ctx context.Context, key string) (bool, error)
    }
    keyFunc func(r *http.Request) string
}

func (m *RateLimiterMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        key := m.keyFunc(r)

        allowed, err := m.limiter.Allow(r.Context(), key)
        if err != nil {
            // fail open: при ошибке Redis пропускаем запрос
            // для критичных endpoint можно fail closed
            next.ServeHTTP(w, r)
            return
        }

        if !allowed {
            w.Header().Set("Retry-After", "60")
            http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    })
}

// стратегии для keyFunc

// по API key из заголовка
func keyByAPIKey(r *http.Request) string {
    return "apikey:" + r.Header.Get("X-API-Key")
}

// по IP
func keyByIP(r *http.Request) string {
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    return "ip:" + ip
}

// по пользователю из контекста
func keyByUser(r *http.Request) string {
    userID := r.Context().Value(ctxKeyUserID).(string)
    return "user:" + userID
}

// разные лимиты для разных endpoint
func keyByUserAndEndpoint(r *http.Request) string {
    userID := r.Context().Value(ctxKeyUserID).(string)
    return fmt.Sprintf("user:%s:endpoint:%s", userID, r.URL.Path)
}
```

**Fail open vs fail closed**: при недоступности Redis — пропускать запрос (fail open) или блокировать (fail closed)? Для публичного API обычно fail open — не ломать пользователей из-за инфраструктурного сбоя. Для защиты от abuse критичных endpoint — fail closed.

## Практические вопросы при внедрении

**Что использовать как ключ**:
- `user_id` — per-user лимиты, нужна аутентификация;
- `api_key` — для B2B API;
- `IP` — для unauthenticated endpoint (login, register), но может ломать NAT/proxy;
- комбинация: `user_id + endpoint` для разных лимитов на разные операции.

**Заголовки ответа**: стандартные заголовки для 429 Too Many Requests:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1714569600
Retry-After: 60
```

**Разные лимиты по планам**: передавать limit как параметр в limiter, получая его из user context (из token или БД с кэшем):

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    user := r.Context().Value(ctxKeyUser).(*User)
    limit := planLimits[user.Plan] // map[string]int{"free": 60, "pro": 1000}

    allowed, _ := h.limiter.AllowN(r.Context(), "user:"+user.ID, limit)
    // ...
}
```

**Тестирование**: мокировать Redis через `miniredis` для unit-тестов:

```go
import "github.com/alicebob/miniredis/v2"

func TestFixedWindowLimiter(t *testing.T) {
    mr := miniredis.RunT(t)
    rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    limiter := &FixedWindowLimiter{redis: rdb, limit: 3, window: time.Minute}

    for i := 0; i < 3; i++ {
        allowed, err := limiter.Allow(context.Background(), "user:42")
        require.NoError(t, err)
        require.True(t, allowed)
    }

    // 4-й запрос должен быть отклонён
    allowed, err := limiter.Allow(context.Background(), "user:42")
    require.NoError(t, err)
    require.False(t, allowed)

    // другой пользователь не должен быть затронут
    allowed, _ = limiter.Allow(context.Background(), "user:99")
    require.True(t, allowed)
}
```

## Interview-ready answer

Для rate limiting через Redis чаще всего используют Sliding Window Counter: два счётчика для соседних окон, взвешенное среднее — O(1) память, нет граничного spike. Fixed Window проще, но уязвим к 2x burst на границе окна. Sliding Window Log абсолютно точен, но хранит timestamp каждого запроса. Token Bucket подходит когда нужно разрешить burst — накопленные токены позволяют краткосрочный всплеск при соблюдении долгосрочного rate. Все алгоритмы реализуются через Lua-скрипты для атомарности операций read-modify-write в распределённой среде. При недоступности Redis — fail open: пропускать запросы, а не ломать пользователей.
