# Задача 2: Rate Limiter

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Сравнение алгоритмов](#алгоритмы--теория)
- [Token bucket](#базовое-решение-token-bucket)
- [Sliding window, окно 1 секунда](#реализация-sliding-window-окно-1-секунда)
- [Пакет x/time/rate](#пакет-golangorgxtimerate)
- [Лимиты по ключам](#per-key-rate-limiter)
- [Распределённый limiter](#distributed-rate-limiter-redis)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связки)

Rate limiter управляет скоростью допуска операций. До выбора алгоритма нужно
определить burst, ключ лимита, реакцию на превышение и допустимую согласованность
между процессами.

---

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
   "Token bucket точно реализует собственную модель накопления токенов, но не
   обещает точное число запросов в каждом произвольном скользящем окне."

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
elapsed = 0.7 окна
count_now + count_prev * (1 - elapsed) ≤ limit
```

**Плюсы:** константная память.
**Минусы:** результат зависит от распределения событий в предыдущем окне;
универсальной точности в процентах нет.

### Fixed Window

Просто counter за фиксированное окно (текущая секунда). Reset на границе.

**Плюсы:** простой.
**Минусы:** "boundary problem" — 100 req в конце окна + 100 в начале следующего = 200 за 100 мс.

---

## Базовое решение: Token Bucket

```go
package ratelimit

import (
    "errors"
    "sync"
    "time"
)

var ErrInvalidConfig = errors.New("ratelimit: rate and burst must be positive")

// TokenBucket — простой token bucket rate limiter.
type TokenBucket struct {
    mu         sync.Mutex
    tokens     float64       // текущее количество (float для accurate refill)
    capacity   float64       // максимум (burst)
    refillRate float64       // tokens per second
    lastRefill time.Time
    now        func() time.Time
}

func NewTokenBucket(rate float64, burst float64) (*TokenBucket, error) {
    return newTokenBucket(rate, burst, time.Now)
}

func newTokenBucket(rate float64, burst float64, now func() time.Time) (*TokenBucket, error) {
    if rate <= 0 || burst <= 0 || now == nil {
        return nil, ErrInvalidConfig
    }
    current := now()
    return &TokenBucket{
        tokens:     burst,  // стартуем полным
        capacity:   burst,
        refillRate: rate,
        lastRefill: current,
        now:        now,
    }, nil
}

// Allow возвращает true если запрос разрешён.
// Не блокирует.
func (tb *TokenBucket) Allow() bool {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    // Refill — добавить токены за прошедшее время
    now := tb.now()
    elapsed := now.Sub(tb.lastRefill).Seconds()
    if elapsed > 0 {
        tb.tokens += elapsed * tb.refillRate
        if tb.tokens > tb.capacity {
            tb.tokens = tb.capacity
        }
        tb.lastRefill = now
    }

    if tb.tokens >= 1 {
        tb.tokens--
        return true
    }
    return false
}
```

**Использование:**

```go
limiter, err := ratelimit.NewTokenBucket(100, 200)
if err != nil {
    log.Fatal(err)
}

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

## Реализация: Sliding Window (окно 1 секунда)

Token bucket отвечает на вопрос «есть ли сейчас накопленная квота». Sliding window
отвечает на другой вопрос — «сколько запросов реально прошло за последнюю секунду».
Разница проявляется на границе: fixed window с лимитом 100 пропускает 100 запросов
в 23:59:59.9 и ещё 100 в 00:00:00.0 — 200 событий за 100 мс. Sliding window такой
всплеск не пропускает, потому что окно двигается вместе с текущим временем, а не
сбрасывается по календарной границе.

Ниже — два варианта окна в одну секунду: точный (log) и приближённый (counter).

### Вариант 1: Sliding Window Log на ring buffer

Наивная реализация хранит slice timestamp'ов и на каждом запросе выбрасывает
устаревшие — это O(N) работы и постоянные аллокации (см. [подводный камень 4](#4-sliding-window--память-on)).
Если лимит фиксирован, тех же гарантий достигает кольцевой буфер ровно на `limit`
элементов: `limit + 1`-й запрос физически некуда записать, пока самый старый не
выйдет из окна.

```go
package ratelimit

import (
    "sync"
    "time"
)

// slidingWindowSize — ширина окна лимитера: одна секунда.
const slidingWindowSize = time.Second

// SlidingWindowLog разрешает не более limit запросов за последнюю секунду.
// Окно — полуинтервал (now-1s, now]: запись возрастом ровно 1s уже вышла из окна.
type SlidingWindowLog struct {
    mu     sync.Mutex
    window time.Duration
    ring   []time.Time // кольцевой буфер на limit отметок
    next   int         // индекс самой старой отметки — её же перезапишет следующий Allow
    now    func() time.Time
}

// NewSlidingWindowLog создаёт лимитер на limit запросов в секунду.
func NewSlidingWindowLog(limit int) (*SlidingWindowLog, error) {
    return newSlidingWindowLog(limit, slidingWindowSize, time.Now)
}

func newSlidingWindowLog(limit int, window time.Duration, now func() time.Time) (*SlidingWindowLog, error) {
    if limit <= 0 || window <= 0 || now == nil {
        return nil, ErrInvalidConfig
    }
    return &SlidingWindowLog{
        window: window,
        ring:   make([]time.Time, limit),
        now:    now,
    }, nil
}

// Allow возвращает true, если за последнюю секунду прошло меньше limit запросов.
// Не блокирует, работает за O(1).
func (s *SlidingWindowLog) Allow() bool {
    s.mu.Lock()
    defer s.mu.Unlock()

    now := s.now()
    oldest := s.ring[s.next]

    // Нулевой time.Time — слот ещё не использован, окно не заполнено.
    if !oldest.IsZero() && now.Sub(oldest) < s.window {
        return false
    }

    s.ring[s.next] = now
    s.next = (s.next + 1) % len(s.ring)
    return true
}
```

**Ключевые моменты:**
- **Ring buffer вместо slice** — O(1) на запрос, ноль аллокаций после конструктора,
  память не зависит от нагрузки.
- **Проверяется только самая старая отметка** — если она внутри окна, то и все
  остальные `limit - 1` тоже: записи в буфере монотонно возрастают.
- **`s.next` играет две роли** — это одновременно позиция самой старой отметки и
  слот для записи новой, поэтому отдельный счётчик размера не нужен.
- **Память O(limit)** — `time.Time` занимает 24 байта, лимит 100 RPS → 2.4 КБ на
  ключ. Для лимитов порядка миллиона запросов вместо `time.Time` хранят
  `int64` unix-nanos (8 байт) либо переходят на counter-вариант.
- **Монотонные часы** — `time.Now()` несёт монотонное показание, и `Sub` использует
  именно его. Перевод системных часов (NTP-скачок) окно не ломает.

### Вариант 2: Sliding Window Counter (O(1) памяти)

Точный log хранит отметку на каждый разрешённый запрос. Counter хранит два числа:
счётчик текущего окна и счётчик предыдущего, взвешенный по тому, какая его часть
ещё попадает в скользящее окно.

```
                  сколько прошлого окна ещё внутри
                  ↓
estimated = prev * (1 - elapsed) + cur
                              ↑
                     доля текущего окна, которая уже истекла (0.0 … 1.0)
```

```go
// SlidingWindowCounter — приближённый лимитер: два счётчика вместо журнала отметок.
type SlidingWindowCounter struct {
    mu          sync.Mutex
    limit       int
    window      time.Duration
    windowStart time.Time // начало текущего окна
    cur         int       // счётчик текущего окна
    prev        int       // счётчик предыдущего окна
    now         func() time.Time
}

func NewSlidingWindowCounter(limit int) (*SlidingWindowCounter, error) {
    return newSlidingWindowCounter(limit, slidingWindowSize, time.Now)
}

func newSlidingWindowCounter(limit int, window time.Duration, now func() time.Time) (*SlidingWindowCounter, error) {
    if limit <= 0 || window <= 0 || now == nil {
        return nil, ErrInvalidConfig
    }
    return &SlidingWindowCounter{
        limit:       limit,
        window:      window,
        windowStart: now(),
        now:         now,
    }, nil
}

func (s *SlidingWindowCounter) Allow() bool {
    s.mu.Lock()
    defer s.mu.Unlock()

    now := s.now()
    s.rotate(now)

    elapsed := float64(now.Sub(s.windowStart)) / float64(s.window) // 0.0 … 1.0
    estimated := float64(s.prev)*(1-elapsed) + float64(s.cur)
    if estimated >= float64(s.limit) {
        return false
    }

    s.cur++
    return true
}

// rotate сдвигает окна вперёд, если с прошлого вызова прошла хотя бы одна ширина окна.
func (s *SlidingWindowCounter) rotate(now time.Time) {
    passed := int64(now.Sub(s.windowStart) / s.window)
    switch {
    case passed <= 0: // ещё внутри текущего окна
        return
    case passed == 1: // соседнее окно — текущий счётчик становится предыдущим
        s.prev, s.cur = s.cur, 0
    default: // была пауза дольше двух окон — история неактуальна
        s.prev, s.cur = 0, 0
    }
    s.windowStart = s.windowStart.Add(time.Duration(passed) * s.window)
}
```

**Ключевые моменты:**
- **Память O(1) на ключ** — два `int` вместо журнала на `limit` отметок. Именно
  этот вариант используют, когда ключей миллионы (per-user лимиты).
- **Lazy rotation** — окна сдвигаются внутри `Allow`, фоновый ticker не нужен.
- **`windowStart.Add(...)` вместо `Truncate`** — `Truncate` выравнивает время по
  календарной сетке и при этом **срезает монотонное показание**; дальнейшие `Sub`
  начали бы считать по настенным часам и ломались бы на NTP-коррекции. Сложение
  монотонность сохраняет.
- **Приближение** — оценка предполагает равномерное распределение запросов внутри
  предыдущего окна. Если весь предыдущий трафик пришёлся на его начало, лимитер
  переоценит нагрузку и отклонит лишнее; если на конец — недооценит и пропустит
  лишнее. Обещать конкретный процент точности нельзя, он зависит от формы трафика.

### Использование

```go
limiter, err := ratelimit.NewSlidingWindowLog(100) // 100 запросов за последнюю секунду
if err != nil {
    log.Fatal(err)
}

if !limiter.Allow() {
    w.Header().Set("Retry-After", "1")
    http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
    return
}
```

Для sliding window с окном в секунду `Retry-After: 1` — честная оценка: слот
освобождается не позже чем через секунду после самого старого запроса.

### Что выбрать

| Критерий | Sliding window log | Sliding window counter | Token bucket |
|---|---|---|---|
| Гарантия | точно ≤ limit за любую секунду | приближённо | точно по своей модели, но не по окну |
| Память на ключ | O(limit) | O(1) | O(1) |
| Время на запрос | O(1) на ring buffer | O(1) | O(1) |
| Burst | не разрешает сверх limit | зависит от оценки | разрешает до `capacity` |
| Типичное применение | биллинг, квоты по договору, строгие внешние лимиты | миллионы per-user ключей | защита сервиса от перегрузки |

Практическое правило: если превышение лимита стоит денег или нарушает контракт —
log; если лимит нужен как защита и небольшая погрешность допустима — counter или
token bucket.

### Distributed-вариант

В Redis точное окно кладут в sorted set: score — timestamp, member — уникальный id
запроса. Один Lua-скрипт чистит окно и считает оставшееся:

```lua
local key, now, window, limit = KEYS[1], tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)   -- выбросить всё старше окна
if redis.call('ZCARD', key) >= limit then
    return 0
end
redis.call('ZADD', key, now, ARGV[4])                  -- ARGV[4] — уникальный id запроса
redis.call('PEXPIRE', key, window)
return 1
```

Цена — память под `limit` элементов на каждый активный ключ и `ZREMRANGEBYSCORE` на
каждом запросе. Counter-вариант в Redis обходится двумя `INCR` по ключам вида
`key:<номер секунды>` и памятью O(1) на ключ.

---

## Пакет `golang.org/x/time/rate`

`golang.org/x/time/rate` — внешний Go-модуль, а не стандартная библиотека. Он
поддерживает неблокирующую проверку, ожидание с context и reservation.

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

// Дождаться N токенов с отменой через context.
if err := limiter.WaitN(ctx, 5); err != nil {
    return err
}
```

### HTTP Middleware

```go
func RateLimitMiddleware(rps float64, burst int) (func(http.Handler) http.Handler, error) {
    if rps <= 0 || burst <= 0 {
        return nil, ErrInvalidConfig
    }
    limiter := rate.NewLimiter(rate.Limit(rps), burst)
    retryAfterSeconds := max(1, int(math.Ceil(1/rps)))

    middleware := func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
                http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
    return middleware, nil
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

func NewPerKeyLimiter(ctx context.Context, rps float64, burst int) (*PerKeyLimiter, error) {
    if ctx == nil || rps <= 0 || burst <= 0 {
        return nil, ErrInvalidConfig
    }
    pk := &PerKeyLimiter{
        limiters: make(map[string]*rate.Limiter),
        lastSeen: make(map[string]time.Time),
        rate:     rate.Limit(rps),
        burst:    burst,
    }
    go pk.cleanup(ctx)
    return pk, nil
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
func (pk *PerKeyLimiter) cleanup(ctx context.Context) {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case now := <-ticker.C:
            pk.mu.Lock()
            cutoff := now.Add(-10 * time.Minute)
            for key, lastSeen := range pk.lastSeen {
                if lastSeen.Before(cutoff) {
                    delete(pk.limiters, key)
                    delete(pk.lastSeen, key)
                }
            }
            pk.mu.Unlock()
        }
    }
}

// HTTP middleware с per-IP rate limit
func PerIPRateLimit(ctx context.Context, rps float64, burst int) (func(http.Handler) http.Handler, error) {
    pk, err := NewPerKeyLimiter(ctx, rps, burst)
    if err != nil {
        return nil, err
    }

    middleware := func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := clientIP(r)
            if !pk.Allow(ip) {
                http.Error(w, "rate limit exceeded", 429)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
    return middleware, nil
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
    "time"

    "github.com/redis/go-redis/v9"
)

// Token bucket в Redis через Lua script (атомарно).
const tokenBucketScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])         -- tokens per second
local capacity = tonumber(ARGV[2])      -- burst
local now = tonumber(ARGV[3])           -- current unix time (milliseconds)
local requested = tonumber(ARGV[4])     -- requested tokens (обычно 1)

local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1]) or capacity
local last_refill = tonumber(data[2]) or now

-- Refill
local elapsed = math.max(0, now - last_refill) / 1000
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = 0
if tokens >= requested then
    tokens = tokens - requested
    allowed = 1
end

redis.call('HSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, math.max(1, math.ceil(capacity / rate * 2)))

return allowed
`

type RedisRateLimiter struct {
    client   *redis.Client
    script   *redis.Script
    rate     float64
    capacity float64
}

func NewRedisRateLimiter(c *redis.Client, rate, capacity float64) (*RedisRateLimiter, error) {
    if c == nil || rate <= 0 || capacity <= 0 {
        return nil, ErrInvalidConfig
    }
    return &RedisRateLimiter{
        client:   c,
        script:   redis.NewScript(tokenBucketScript),
        rate:     rate,
        capacity: capacity,
    }, nil
}

func (r *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
    now := time.Now().UnixMilli()
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
- **Hash в Redis** (`HSET`) — храним два поля (`tokens` и `last_refill`) в одном key

### Когда нужен distributed

- Multi-pod deployment в Kubernetes (one user → любой pod)
- Strict global limits (e.g., "никогда не более 1000 RPS на наш downstream API")

### Когда in-memory достаточно

- Single pod (development, low scale)
- Soft per-pod limits (если 5 pod'ов, каждый по 100 RPS = approximate 500 RPS глобально, OK)

Client time на разных pods может расходиться. Для строгого общего лимита время
лучше получать внутри Redis через `TIME`. Распределённый вариант добавляет
сетевой round-trip и зависимость от доступности Redis.

---

## Тесты

```go
func TestTokenBucket_Allow(t *testing.T) {
    now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    tb, err := newTokenBucket(10, 5, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }

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

    now = now.Add(100 * time.Millisecond)
    if !tb.Allow() {
        t.Error("after 100ms 1 token should be available")
    }
}

func TestTokenBucket_Concurrent(t *testing.T) {
    fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    tb, err := newTokenBucket(100, 100, func() time.Time { return fixed })
    if err != nil {
        t.Fatal(err)
    }

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

    if allowed.Load() != 100 {
        t.Errorf("allowed %d, want 100", allowed.Load())
    }
}
```

```go
func TestSlidingWindowLog_Allow(t *testing.T) {
    now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    sw, err := newSlidingWindowLog(3, time.Second, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }

    for i := 0; i < 3; i++ {
        if !sw.Allow() {
            t.Errorf("request %d should be allowed", i)
        }
    }
    if sw.Allow() {
        t.Error("4th request should be rejected")
    }

    // Окно ещё не сдвинулось настолько, чтобы освободить слот.
    now = now.Add(900 * time.Millisecond)
    if sw.Allow() {
        t.Error("request at +900ms should be rejected")
    }

    // Ровно на границе все три отметки выходят из окна.
    now = now.Add(100 * time.Millisecond)
    for i := 0; i < 3; i++ {
        if !sw.Allow() {
            t.Errorf("request %d after full window should be allowed", i)
        }
    }
    if sw.Allow() {
        t.Error("4th request in new window should be rejected")
    }
}

func TestSlidingWindowCounter_NoBoundaryBurst(t *testing.T) {
    now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    start := now
    sw, err := newSlidingWindowCounter(100, time.Second, func() time.Time { return now })
    if err != nil {
        t.Fatal(err)
    }

    // Весь лимит выбран в конце первого окна.
    now = start.Add(900 * time.Millisecond)
    for i := 0; i < 100; i++ {
        if !sw.Allow() {
            t.Fatalf("request %d should be allowed", i)
        }
    }

    // Начало следующего окна: fixed window пропустил бы ещё 100 (boundary problem),
    // sliding window видит полный вес предыдущего окна.
    now = start.Add(time.Second)
    if sw.Allow() {
        t.Error("request right after window boundary should be rejected")
    }

    // Через 900 мс нового окна вес предыдущего — 10%, значит доступно 90 запросов.
    now = start.Add(1900 * time.Millisecond)
    allowed := 0
    for i := 0; i < 200; i++ {
        if sw.Allow() {
            allowed++
        }
    }
    if allowed != 90 {
        t.Errorf("allowed %d, want 90", allowed)
    }
}
```

Управляемые часы делают проверку refill и сдвига окон детерминированной и не
зависящей от скорости CI. Тест на границе окна — главный аргумент в пользу sliding
window: тот же сценарий на fixed window пропустил бы 200 запросов за 100 мс.

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
- [Ring buffer на `limit` отметок](#вариант-1-sliding-window-log-на-ring-buffer) —
  точность log'а без роста памяти и аллокаций

### 5. Возвращать только `bool` без `Retry-After`

```go
// ❌ Клиент не знает когда retry
if !limiter.Allow() {
    return ErrRateLimited
}
```

Простой conservative hint для token bucket:
```go
retryAfter := max(1, int(math.Ceil(1/rps)))
w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
```

`Reserve` нельзя вызывать только ради вычисления header и забывать: reservation
меняет состояние limiter'а. Если используешь reservation, ненужную квоту нужно
корректно отменить через `CancelAt`.

### 6. Distributed rate limiter без atomic operation

```python
# ❌ Не атомарно — race condition
tokens = redis.GET(key)
if tokens > 0:
    redis.DECR(key)  # ← между GET и DECR могла прийти другая goroutine
```

Для token bucket используй одну Lua/Function-операцию. Для fixed-window counter
подходит атомарный `INCR`, но установку TTL тоже нужно согласовать так, чтобы
сбой между командами не оставил вечный ключ.

### 7. Не учитывать context

```go
// ❌ Ожидание не связано с отменой caller'а.
limiter.Wait(context.Background())

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

Burst `10 000` разрешает почти мгновенно допустить до десяти тысяч накопленных
операций. Его выбирают из допустимой мгновенной нагрузки downstream, а не
универсальным множителем среднего RPS.

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

Проверка проходит **все три уровня**; такой contract типичен для API gateway и
multi-tenant сервисов.

### 4. Different algorithms per endpoint

Token bucket для regular API. Leaky bucket для downstream protection. Sliding window для exact counting (биллинг).

### 5. Distributed с in-memory cache

Кэшировать "у меня есть токены" локально, синхронизироваться с Redis периодически. Меньше latency, чуть менее точно.

### 6. Soft vs hard limit

Soft — warning header, но запрос проходит. Hard — запрос отклоняется, обычно с
`429 Too Many Requests`.

### 7. Backpressure для воркеров

Worker pool читает из канала, lim'итер на потребителе:
```go
for task := range tasks {
    if err := limiter.Wait(ctx); err != nil {
        return err
    }
    process(task)
}
```

### 8. Rate limit info в headers

Не выдавай legacy `X-RateLimit-*` за общий стандарт: их семантика зависит от API.
В актуальном IETF draft предлагаются structured fields:

```
RateLimit-Policy: "default";q=5000;w=3600
RateLimit: "default";r=4999;t=3590
```

На август 2026 года это всё ещё Internet-Draft, а не опубликованный RFC, поэтому
формат нужно зафиксировать в контракте своего API. Для ответа `429` отдельно
полезен стандартный `Retry-After`.

---

## Interview-ready answer

**1. Как работает token bucket?**

- Состояние — число токенов ограничено ёмкостью `burst`.
- Refill — прошедшее время умножается на скорость и лениво добавляет токены.
- Allow — операция списывает стоимость запроса либо немедленно отказывает.
- Семантика — алгоритм допускает накопленный burst, но не обещает точный лимит в
  каждом произвольном sliding window.

**2. Как гарантировать «не более N запросов за любую секунду»?**

- Token bucket такой гарантии не даёт — накопленный burst проходит мгновенно.
- Sliding window log хранит отметку на каждый разрешённый запрос и сравнивает
  самую старую с границей окна.
- Ring buffer на `limit` отметок даёт O(1) по времени и фиксированную память:
  проверяется только самая старая запись, она же перезаписывается новой.
- Sliding window counter меняет точность на O(1) памяти — берётся, когда ключей
  слишком много для журнала.

**3. Что важно для per-key limiter?**

- Память — map ключей требует удаления idle-записей.
- Lifecycle — cleanup goroutine должна останавливаться вместе с приложением.
- Identity — proxy headers доверяют только после проверки доверенного proxy.
- Contention — sharding уменьшает общий lock, если ключей много.

**4. Чем распределённый limiter сложнее локального?**

- Атомарность — refill, проверка и списание выполняются одной server-side
  операцией.
- Время — часы разных pods могут расходиться, поэтому нужен согласованный
  источник.
- Доступность — нужно выбрать fail-open или fail-closed при ошибке Redis.
- Цена — каждый строгий допуск добавляет сетевой round-trip.

---

## Связки

- [DDoS protection](../../../11-security/perimeter-and-traffic-protection/01-ddos-protection.md) — production rate limiter в Go
- [Rate limiting в reliability patterns](../../../05-system-design/reliability-patterns/04-rate-limiting.md) — system design
- [HTTP rate limit middleware](../../../08-networking-and-api/protocols/05-integration-patterns/03-rate-limiting.md) — детальный разбор
- [Rate limiter interview case](../../../05-system-design/interview-cases/03-rate-limiter.md) — system design версия задачи
- [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) — API локального token bucket
- [Redis `TIME`](https://redis.io/docs/latest/commands/time/) — server-side источник времени
- [IETF RateLimit header fields](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/) — актуальный draft формата HTTP headers
