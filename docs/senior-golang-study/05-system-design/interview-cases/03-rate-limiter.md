# Rate Limiter

Разбор задачи "Спроектируй Rate Limiter". Проверяет понимание алгоритмов, распределённого состояния, latency требований и failure modes. Часто идёт как компонент более крупной системы или как самостоятельная задача.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Кандидат: Несколько уточнений прежде чем начать.

Вопросы:
  - На каком уровне применять rate limiting?
    → API Gateway (централизованно) или в каждом сервисе?
  - По чему лимитируем?
    → По IP? По user_id? По API key? Комбинация?
  - Тип лимита?
    → Фиксированное окно (100 req/min) или sliding window?
    → Разные лимиты для разных endpoint'ов?
  - Что происходит при превышении?
    → 429 Too Many Requests с Retry-After header?
  - Hard limit или soft (throttle, но пропустить)?
```

**Договорились:**
- Уровень: API Gateway (один централизованный компонент)
- Ключ: комбинация (user_id если есть, иначе IP + API key)
- Тип: sliding window — точнее fixed window (нет "burst" на границе окна)
- Лимиты: конфигурируемые по-разному для разных endpoint-групп
- При превышении: 429 + заголовок `X-RateLimit-Retry-After`
- Hard limit (отклонить запрос)

**Out of scope:** балансировка нагрузки, DDoS-защита (это другой уровень), per-tenant billing.

### Нефункциональные требования

```
- Latency: overhead rate limiter < 1ms p99 (критично — стоит на каждом запросе)
- Accuracy: +/- 0.1% (небольшая погрешность допустима)
- Availability: если rate limiter упал → fail-open (пропустить запрос) или fail-closed?
  → Обсудить: для большинства API — fail-open (не блокировать легитимных пользователей)
- Scale: 100K RPS через gateway
- Consistency: eventual OK (небольшой burst при нескольких нодах допустим)
```

---

## Фаза 2: Оценка нагрузки

```
Трафик через Gateway: 100K RPS
  → 100K проверок rate limiter / sec

Данные в Redis:
  Ключ per user: "rl:{user_id}:{window}" → timestamp list или counter
  Уникальных пользователей: 10M
  Данные на пользователя: ~1KB (для sliding window log)
  10M × 1KB = 10GB → умещается в Redis

Команды Redis на запрос:
  Fixed window: 1 INCR + EXPIRE → 2 команды
  Sliding window log: ZADD + ZREMRANGEBYSCORE + ZCARD → 3-4 команды
  Token bucket: GET + SET → 2 команды (с Lua для атомарности)

При 100K RPS × 3 команды = 300K Redis ops/sec
  → Redis справится (1M ops/sec одна нода)
```

---

## Фаза 3: Алгоритмы Rate Limiting

Прежде чем рисовать архитектуру — важно понять алгоритмы, потому что они влияют на хранилище и точность.

### 1. Fixed Window Counter

```
Окно: 1 минута
Лимит: 100 запросов

Ключ: rl:{user_id}:{minute_timestamp}
Операция: INCR key; EXPIRE key 60

Плюсы: O(1) memory, простота
Минусы: burst на границе окна

Пример проблемы:
  23:59:59 → 100 запросов (в рамках минуты X)
  00:00:01 → ещё 100 запросов (новая минута Y)
  → 200 запросов за 2 секунды — окно позволяет!
```

### 2. Sliding Window Log

```
Лимит: 100 запросов за последние 60 секунд

Redis: Sorted Set, score = timestamp
  ZADD rl:{user_id} {now_ms} {request_id}
  ZREMRANGEBYSCORE rl:{user_id} 0 {now_ms - 60000}  // удалить старые
  ZCARD rl:{user_id}  // текущий count

Плюсы: точный sliding window, нет burst проблемы
Минусы: O(N) memory (растёт с количеством запросов), дороже по Redis ops
```

### 3. Sliding Window Counter (гибрид — выбираем этот)

```
Компромисс между точностью и памятью:

Идея: хранить счётчики двух соседних окон + интерполировать

current_window_count = counter[current_window]
prev_window_count    = counter[prev_window]
overlap              = elapsed_time_in_current_window / window_size
estimated_count = prev_window_count × (1 - overlap) + current_window_count

Пример:
  window = 1 min, limit = 100
  prev окно: 80 запросов
  current окно: 20 запросов, прошло 30 сек (overlap = 0.5)
  estimated = 80 × 0.5 + 20 = 60 — в норме

Плюсы: O(1) memory (только 2 счётчика), точность ~99.9%
Минусы: небольшая погрешность (приемлемо по условиям)
```

### 4. Token Bucket

```
Bucket: N токенов, заполняется со скоростью R tokens/sec

Плюсы: естественный burst (можно использовать N накопленных токенов сразу)
Минусы: нужен атомарный GET+SET+conditional update → Lua script в Redis

Когда выбирать: если нужно разрешить кратковременные bursts
  (API позволяет 100 req/min но допускает 10 запросов за 1 сек)
```

**Выбор: Sliding Window Counter** — баланс между точностью и эффективностью.

---

## Фаза 4: Deep Dive

### Архитектура

```
    Client
      │
      ▼
┌──────────────────────────────────┐
│         API Gateway              │
│                                  │
│  ┌────────────┐                  │
│  │ Rate Limit │                  │
│  │ Middleware │                  │
│  └─────┬──────┘                  │
│        │  1. extract key         │
│        │     (user_id / IP)      │
│        │  2. check_and_increment │ ─────► Redis Cluster
│        │     (Lua script)        │ ◄─────  (allowed/denied)
│        │  3. set headers         │
│        │  4. pass or reject      │
│  ┌─────▼──────┐                  │
│  │  Upstream  │                  │
│  │  Services  │                  │
│  └────────────┘                  │
└──────────────────────────────────┘
         │
         ▼
    Config Service  ← лимиты для разных endpoints
```

---

### Redis Lua Script (атомарность)

Проблема: ZREMRANGEBYSCORE + ZCARD + ZADD — три операции. Между ними может вклиниться другой запрос (race condition).

**Решение: Lua script выполняется атомарно в Redis:**

```lua
-- sliding_window_counter.lua
local key = KEYS[1]
local now = tonumber(ARGV[1])          -- текущее время (ms)
local window = tonumber(ARGV[2])       -- размер окна (ms)
local limit = tonumber(ARGV[3])        -- лимит

local current_window_key  = key .. ":" .. math.floor(now / window)
local prev_window_key     = key .. ":" .. math.floor(now / window) - 1
local elapsed             = now % window
local overlap             = 1 - (elapsed / window)

local prev_count    = tonumber(redis.call("GET", prev_window_key) or "0")
local current_count = tonumber(redis.call("GET", current_window_key) or "0")

local estimated = math.floor(prev_count * overlap) + current_count

if estimated >= limit then
  -- вернуть оставшееся время до сброса окна
  local retry_after = math.ceil((window - elapsed) / 1000)
  return {0, retry_after}  -- denied
end

-- increment current window
redis.call("INCR", current_window_key)
redis.call("PEXPIRE", current_window_key, window * 2)  -- TTL = 2 окна

return {1, 0}  -- allowed
```

---

### Конфигурация лимитов

```yaml
rate_limits:
  default:
    window: 60s
    limit: 100

  rules:
    - pattern: "POST /api/v1/auth/*"
      window: 60s
      limit: 10              # строже для auth endpoint

    - pattern: "POST /api/v1/send-otp"
      window: 300s
      limit: 5               # 5 OTP за 5 минут

    - pattern: "GET /api/v1/feed"
      window: 60s
      limit: 300             # читающие endpoint — свободнее

  tiers:
    free:     { window: 60s, limit: 100  }
    pro:      { window: 60s, limit: 1000 }
    enterprise: { window: 60s, limit: 10000 }
```

Конфигурация хранится в Config Service (например, consul/etcd/DB), кешируется в памяти API Gateway с TTL 30 секунд.

---

### Заголовки ответа

```
Нормальный запрос (лимит не превышен):
  X-RateLimit-Limit:     100
  X-RateLimit-Remaining: 73
  X-RateLimit-Reset:     1716213600  (unix timestamp когда сбросится)

При превышении (429):
  X-RateLimit-Limit:     100
  X-RateLimit-Remaining: 0
  X-RateLimit-Retry-After: 47         (секунды до сброса)
  
  Body: {"error": "rate_limit_exceeded", "retry_after": 47}
```

---

### Distributed Rate Limiting: проблема и решение

**Проблема:** несколько нод API Gateway, каждая смотрит в один Redis.

```
Node 1: проверяет → 99/100 → пропускает
Node 2: проверяет → 99/100 (данные ещё не обновились) → тоже пропускает
→ 101 запрос прошёл

Решение: centralized Redis (не per-node в памяти).
  Lua script гарантирует атомарность на уровне Redis.
  Race condition возможен между EVAL и следующим EVAL,
  но Lua script атомарен в рамках одного вызова → OK.
```

**Альтернатива — локальный rate limiter + синхронизация:**
```
Каждая нода хранит локальный bucket.
Периодически (каждые 100ms) синхронизирует с Redis.
→ Меньше нагрузки на Redis, но временно допускает burst при N нодах.
→ Подходит если небольшая погрешность (< 10%) приемлема.
```

---

### Failure Modes

**Что если Redis недоступен?**

```
Fail-open (пропустить запрос):
  + Пользователи не страдают от проблем инфраструктуры
  - Злоумышленник может обойти лимиты во время сбоя

Fail-closed (отклонить запрос):
  + Безопаснее
  - Недоступность Redis = недоступность всего API

Рекомендация: fail-open с логированием и алертингом.
  "Rate limiter unavailable — bypassing check" в метриках.
  При восстановлении Redis — состояние сбрасывается (лимиты могут временно быть мягче).
```

**Degraded mode:**
```go
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
    allowed, err := r.redis.CheckAndIncrement(ctx, key)
    if err != nil {
        // Redis недоступен
        r.metrics.IncCounter("rate_limiter.bypass")
        return true, nil  // fail-open
    }
    return allowed, nil
}
```

---

### Защита от обхода

```
1. IP Spoofing:
   Использовать несколько заголовков: X-Forwarded-For, X-Real-IP, CF-Connecting-IP
   Но доверять только последнему hop от trusted proxy

2. Burst via distributed clients:
   Один пользователь с 1000 IP → rate limit по user_id (не по IP)
   Для незарегистрированных → IP-based, но жёстче (10 req/min)

3. Slowloris / connection exhaustion:
   Это задача для другого уровня (LB, nginx) — не rate limiter
```

---

## Расширение: Rate Limiter как отдельный сервис

Если нужен не middleware в Gateway, а отдельный микросервис:

```
Other services → gRPC → Rate Limiter Service → Redis
                        ↑
                Centralized decisions

API:
  rpc CheckRateLimit(CheckRequest) returns (CheckResponse) {}

  message CheckRequest {
    string key    = 1;  // "user:{id}" или "ip:{addr}"
    string policy = 2;  // "default" или "auth"
  }

  message CheckResponse {
    bool   allowed      = 1;
    int32  remaining    = 2;
    int64  reset_at     = 3;
    int32  retry_after  = 4;  // если не allowed
  }
```

Overhead: один gRPC call + Redis = ~1-2ms. Нужно кешировать в сервисе-клиенте при строгих latency требованиях.

---

## Трейдоффы

| Алгоритм | Memory | Точность | Сложность | Burst handling |
|---|---|---|---|---|
| Fixed Window | O(1) | Низкая (boundary burst) | Простой | Нет |
| Sliding Window Log | O(N) | Высокая | Средний | Нет |
| Sliding Window Counter | O(1) | Высокая (~99.9%) | Средний | Нет |
| Token Bucket | O(1) | Высокая | Сложный (Lua) | Да |
| Leaky Bucket | O(1) | Высокая | Средний | Сглаживает |

---

## Interview-ready ответ (2 минуты)

> "Rate limiter стоит на каждом запросе, поэтому latency overhead критичен — цель < 1ms.
>
> Алгоритм: Sliding Window Counter — два счётчика (текущее и предыдущее окно) с интерполяцией. O(1) память, погрешность < 0.1%. Выполняется атомарно через Lua script в Redis, что исключает race condition между операциями.
>
> Ключ для лимитирования: user_id для аутентифицированных запросов, IP для анонимных. Лимиты конфигурируемые по endpoint-группам и tier пользователя.
>
> При превышении: 429 с заголовком Retry-After. При недоступности Redis: fail-open — пропускаем запрос с метрикой 'rate_limiter.bypass'.
>
> Распределённость: все ноды API Gateway смотрят в один Redis Cluster. Lua script атомарен — race condition минимален. Альтернатива — локальные bucket с периодической синхронизацией для снижения нагрузки на Redis, но с небольшой погрешностью."
