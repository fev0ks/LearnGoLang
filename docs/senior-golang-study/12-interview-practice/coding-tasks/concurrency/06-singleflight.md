# Задача 6: Singleflight

Singleflight — паттерн дедупликации **одновременных** одинаковых запросов. Если 100 goroutines одновременно просят "загрузить user 42", только **один** запрос реально идёт в БД, остальные ждут и получают тот же результат.

Это решение против **cache stampede** и **thundering herd** — когда кэш истёк и сотни клиентов одновременно ломятся в БД за одним ключом.

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Базовое решение](#базовое-решение)
- [Production-grade: `x/sync/singleflight`](#production-grade-используй-golangorgxsyncsingleflight)
  - [`DoChan` для async API](#dochan-для-async-api)
- [Singleflight для cache stampede](#singleflight-для-cache-stampede)
- [Подводный камень: shared result mutation](#подводный-камень-shared-result-mutation)
- [Singleflight + context](#singleflight--context)
- [Singleflight и errors](#singleflight-и-errors)
- [Тесты](#тесты)
- [Подводные камни](#подводные-камни)
- [Возможные расширения](#возможные-расширения)
- [Реальные применения](#реальные-применения)
- [Что важно показать на собеседовании](#что-важно-показать-на-собеседовании)
- [Связки](#связки)

## Формулировка

> "Реализуй паттерн, который дедуплицирует одновременные одинаковые вызовы. Например, если 100 запросов параллельно вызывают `GetUser(42)`, в БД должен пойти только один."

Вариации:
- "Cache stampede protection"
- "Шторм запросов после deploy'а — что делать?"
- "Сделай `func Do(key string, fn func() (any, error)) (any, error)`, чтобы одинаковые ключи дедуплицировались"

---

## Уточняющие вопросы

1. **Идентичность по key — как сравнивать?**
   "По string key — простой случай. По request fingerprint (метод+URL+params) — сложнее."

2. **Результат шарится всем waiter'ам?**
   "Да — это весь смысл. Один call, N waiters получают тот же результат и ошибку."

3. **Что если первый запрос fail'ит?**
   "Все waiter'ы получают ту же ошибку. Не retry автоматически — это другой паттерн."

4. **Кэшировать результат после fail?**
   "По-разному. Стандартный singleflight — нет. Можешь добавить short TTL для negative caching."

5. **Что если в-flight call зависнет?**
   "Caller отвечает за timeout через context. Singleflight сам timeout не делает."

6. **Хочешь shared result или каждый waiter получает copy?**
   "Shared — risk mutation race. Документируй immutability результата."

---

## Базовое решение

Минимальная реализация:

```go
package singleflight

import "sync"

type result struct {
    val any
    err error
}

type call struct {
    wg  sync.WaitGroup
    res result
}

type Group struct {
    mu sync.Mutex
    m  map[string]*call
}

func (g *Group) Do(key string, fn func() (any, error)) (any, error) {
    g.mu.Lock()
    if g.m == nil {
        g.m = make(map[string]*call)
    }

    // Уже есть in-flight call для этого key — ждём его
    if c, ok := g.m[key]; ok {
        g.mu.Unlock()
        c.wg.Wait()
        return c.res.val, c.res.err
    }

    // Создаём новый call, регистрируем
    c := new(call)
    c.wg.Add(1)
    g.m[key] = c
    g.mu.Unlock()

    // Выполняем (без lock — другие могут join'иться)
    c.res.val, c.res.err = fn()
    c.wg.Done()

    // Удаляем call из map (новые вызовы будут делать new call)
    g.mu.Lock()
    delete(g.m, key)
    g.mu.Unlock()

    return c.res.val, c.res.err
}
```

**Как работает:**
1. Первый caller с ключом X создаёт `call` и start'ит fn
2. Параллельный caller видит существующий `call`, ждёт через `wg.Wait()`
3. Когда первый закончил, `wg.Done()` будит всех ожидающих
4. Все получают тот же result

**Использование:**

```go
var g singleflight.Group

// 100 параллельных GetUser(42)
for i := 0; i < 100; i++ {
    go func() {
        user, err := g.Do("user-42", func() (any, error) {
            return loadUserFromDB(42)
        })
        // ... use user
    }()
}
// В БД пойдёт ОДИН запрос, не 100
```

---

## Production-grade: используй `golang.org/x/sync/singleflight`

В реальной работе **не пиши свой** — есть стандартная реализация:

```go
import "golang.org/x/sync/singleflight"

var g singleflight.Group

func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    key := fmt.Sprintf("user:%d", id)
    v, err, shared := g.Do(key, func() (any, error) {
        return s.repo.GetByID(ctx, id)
    })
    if err != nil {
        return nil, err
    }
    if shared {
        // Это был "ждал чужой call" — useful for metrics
        s.metrics.SingleflightHit.Inc()
    }
    return v.(*User), nil
}
```

**Что даёт стандартная версия:**
- `Do(key, fn) (val any, err error, shared bool)` — `shared` показывает был ли результат разделён
- `DoChan(key, fn) <-chan Result` — async API
- `Forget(key)` — принудительно забыть in-flight call (полезно если хотим retry)

### `DoChan` для async API

```go
ch := g.DoChan("user-42", func() (any, error) {
    return loadUser(42)
})

select {
case r := <-ch:
    return r.Val.(*User), r.Err
case <-ctx.Done():
    return nil, ctx.Err()
}
```

Полезно когда хочешь иметь возможность отменить через `ctx` — но **сам fn не отменяется**, просто ты перестаёшь ждать. Другие waiter'ы продолжают ждать.

> Разбор устройства `x/sync/singleflight` (структуры `Group`/`call`, `Do` vs `DoChan`, почему канал `cap 1`, `Forget`, обработка паники) — в теории: [03-sync-primitives](../../../01-go-core/concurrency-and-performance/03-sync-primitives.md), раздел про `singleflight`.

---

## Singleflight для cache stampede

Главное использование — защита от **thundering herd** при cache miss.

```go
type CachedRepo struct {
    cache *redis.Client
    db    *DB
    g     singleflight.Group
}

func (r *CachedRepo) GetUser(ctx context.Context, id int64) (*User, error) {
    key := fmt.Sprintf("user:%d", id)

    // 1. Try cache
    if data, err := r.cache.Get(ctx, key).Bytes(); err == nil {
        var u User
        if err := json.Unmarshal(data, &u); err == nil {
            return &u, nil
        }
    }

    // 2. Cache miss — используем singleflight для DB lookup
    v, err, _ := r.g.Do(key, func() (any, error) {
        u, err := r.db.GetUser(ctx, id)
        if err != nil {
            return nil, err
        }
        // Сохраняем в кэш
        if data, err := json.Marshal(u); err == nil {
            r.cache.Set(ctx, key, data, 5*time.Minute)
        }
        return u, nil
    })
    if err != nil {
        return nil, err
    }
    return v.(*User), nil
}
```

**Сценарий:**
- Cache истёк
- 1000 параллельных запросов GetUser(42)
- 999 находят cache empty одновременно
- БЕЗ singleflight: 1000 запросов в БД → перегрузка
- С singleflight: 1 запрос в БД, 999 ждут результат

Подробнее про cache stampede: [06-databases/caching/01-redis-as-cache.md](../../../06-databases/caching/01-redis-as-cache.md).

---

## Подводный камень: shared result mutation

Все waiter'ы получают **тот же** object. Если кто-то mutate'ит — все видят:

```go
v, _, _ := g.Do("user-42", func() (any, error) {
    return loadUser(42)  // *User
})

user := v.(*User)
user.Name = "Mutated"  // ← все остальные waiter'ы видят это
```

**Решения:**

### 1. Документируй immutability

```go
// Do invokes fn. The result must not be mutated by callers.
func (g *Group) Do(...) ...
```

### 2. Глубокое копирование в caller'е

```go
v, _, _ := g.Do(key, fn)
return cloneUser(v.(*User)), nil
```

### 3. Return immutable types

Используй immutable domain objects (uncommon в Go).

### 4. Возвращать `[]byte` (JSON)

Все парсят свой результат — нет shared mutable state.

---

## Singleflight + context

`singleflight.Do` **сам** не принимает context. Но fn может принять и проверить:

```go
func (s *Service) GetUser(ctx context.Context, id int64) (*User, error) {
    v, err, _ := s.g.Do(key, func() (any, error) {
        // fn НЕ получает ctx первого waiter'а автоматически
        // Используй свой context (например, ctx из background)
        bgCtx := context.Background()
        return s.repo.GetByID(bgCtx, id)
    })
    return v.(*User), err
}
```

**Проблема:** если первый waiter имеет timeout 1 секунду, второй 10 секунд — второй "наследует" timeout первого (он отменится через 1 секунду).

**Решение:** использовать background context внутри fn, или DoChan + select с per-caller ctx:

```go
ch := g.DoChan(key, func() (any, error) {
    // Не привязан к ctx ни одного caller'а
    return loadFromDB(context.Background(), id)
})

select {
case r := <-ch:
    return r.Val.(*User), r.Err
case <-ctx.Done():
    return nil, ctx.Err()
}
```

Caller с коротким timeout уйдёт раньше, остальные продолжат ждать. Но если **все** caller'ы уйдут — fn всё равно завершится в background.

---

## Singleflight и errors

Если fn возвращает error, **все waiter'ы** получают эту ошибку. Это OK для transient errors (БД недоступна — все увидят), но плохо для bug'ов.

```go
// Если fn панует в processing 1, retry в processing 2 не помогает
// Все ждут результат первого call'а
```

**Решение для retry:**
```go
v, err, _ := g.Do(key, fn)
if err != nil {
    g.Forget(key)  // ← следующий Do(key, fn) создаст новый call
    return nil, err
}
```

`Forget` полезен для negative caching control: "ошибочный результат не надо кэшировать в singleflight на длительное время".

---

## Тесты

```go
import "golang.org/x/sync/singleflight"

func TestSingleflight_Dedups(t *testing.T) {
    var g singleflight.Group
    var calls atomic.Int32

    fn := func() (any, error) {
        calls.Add(1)
        time.Sleep(50 * time.Millisecond)
        return "value", nil
    }

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            v, _, _ := g.Do("key", fn)
            if v.(string) != "value" {
                t.Errorf("got %v, want value", v)
            }
        }()
    }
    wg.Wait()

    if calls.Load() != 1 {
        t.Errorf("fn called %d times, expected 1", calls.Load())
    }
}

func TestSingleflight_SharedFlag(t *testing.T) {
    var g singleflight.Group

    fn := func() (any, error) {
        time.Sleep(50 * time.Millisecond)
        return 42, nil
    }

    var sharedCount, notSharedCount atomic.Int32
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, _, shared := g.Do("k", fn)
            if shared {
                sharedCount.Add(1)
            } else {
                notSharedCount.Add(1)
            }
        }()
    }
    wg.Wait()

    // Один caller "владеет" вызовом, у него shared=false (либо true — implementation detail)
    // Главное — total = 10
    total := sharedCount.Load() + notSharedCount.Load()
    if total != 10 {
        t.Errorf("total %d, want 10", total)
    }
}

func TestSingleflight_DifferentKeys(t *testing.T) {
    var g singleflight.Group
    var calls atomic.Int32

    fn := func() (any, error) {
        calls.Add(1)
        time.Sleep(50 * time.Millisecond)
        return "value", nil
    }

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(key string) {
            defer wg.Done()
            g.Do(key, fn)
        }(fmt.Sprintf("key-%d", i%5))  // 5 разных ключей
    }
    wg.Wait()

    // 5 ключей → 5 calls (по одному на ключ)
    if calls.Load() != 5 {
        t.Errorf("calls %d, expected 5", calls.Load())
    }
}

func TestSingleflight_ErrorShared(t *testing.T) {
    var g singleflight.Group
    expectedErr := errors.New("fail")

    fn := func() (any, error) {
        time.Sleep(50 * time.Millisecond)
        return nil, expectedErr
    }

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, err, _ := g.Do("key", fn)
            if !errors.Is(err, expectedErr) {
                t.Errorf("got %v, want %v", err, expectedErr)
            }
        }()
    }
    wg.Wait()
}

func TestSingleflight_DoChan(t *testing.T) {
    var g singleflight.Group

    fn := func() (any, error) {
        time.Sleep(100 * time.Millisecond)
        return "result", nil
    }

    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    ch := g.DoChan("k", fn)

    select {
    case r := <-ch:
        t.Errorf("shouldn't receive yet: %v", r)
    case <-ctx.Done():
        // Expected — caller timeout'ed
    }
}
```

---

## Подводные камни

### 1. Singleflight НЕ кэширует — только дедуплицирует in-flight

```go
// Caller 1
v1, _, _ := g.Do("key", fn)  // 100ms — реальный call
// fn вернулся, call удалён из map

// Caller 2 (через 5 секунд)
v2, _, _ := g.Do("key", fn)  // 100ms — НОВЫЙ реальный call
```

Singleflight — для **concurrent** дедупликации, не для cache. Если нужно кэшировать — используй cache (Redis, in-memory) + singleflight перед ним.

### 2. Shared mutable result

См. выше — все waiter'ы получают same instance. Mutation → race.

### 3. Context "наследование"

Первый waiter имеет ctx с timeout 100ms — если fn использует этот ctx и он истёк, второй caller (с большим timeout) получит cancelled error.

### 4. Никогда не возвращает channel — `Do` блокирует

```go
v, _, _ := g.Do(key, fn)  // ← блокирует пока не закончится fn
```

Если fn зависнет — все waiter'ы зависнут. Используй DoChan + select с ctx если нужен timeout.

### 5. Forget может race с new call

```go
g.Do(k, fn)  // fn'у выполнился, c удалён из map
g.Forget(k)  // ← already removed, no-op
g.Do(k, fn)  // новый call (правильно)
```

`Forget` безопасен. Главное — понимать его semantics: "забудь in-flight (если есть)".

### 6. Не подходит для distributed scenarios

Singleflight — **local** (в одном процессе). На 10 pod'ах будет 10 параллельных DB calls (по одному на pod). Для distributed dedup — нужен distributed lock (Redis SET NX) или leader election.

### 7. Возможный starvation для slow consumer

В обычном Do — все waiter'ы синхронны. Slow waiter не блокирует других (они уже получили `wg`). Но если используешь свою реализацию через channel — slow consumer может block.

### 8. Не для long-running operations

Singleflight отлично подходит для milliseconds-level операций (DB query, API call). Для long-running (минуты) — другие подходы (job queue, persistent task).

---

## Возможные расширения

### 1. With TTL (positive cache)

Не только дедуплицируй concurrent, но и кэшируй результат на короткое время:

```go
type CachedSingleflight struct {
    g     singleflight.Group
    cache sync.Map  // key → *cacheEntry
    ttl   time.Duration
}

type cacheEntry struct {
    val   any
    expAt time.Time
}

func (c *CachedSingleflight) Do(key string, fn func() (any, error)) (any, error) {
    if entry, ok := c.cache.Load(key); ok {
        e := entry.(*cacheEntry)
        if time.Now().Before(e.expAt) {
            return e.val, nil
        }
    }

    v, err, _ := c.g.Do(key, func() (any, error) {
        v, err := fn()
        if err != nil {
            return nil, err
        }
        c.cache.Store(key, &cacheEntry{val: v, expAt: time.Now().Add(c.ttl)})
        return v, nil
    })
    return v, err
}
```

### 2. Negative caching

Кэшировать errors на короткое время — не делать retry слишком часто.

### 3. Distributed singleflight (Redis-based)

```go
// Через Redis SETNX
ok := redis.SetNX(ctx, "lock:"+key, "1", 10*time.Second)
if ok {
    // We own the lock — do the work
    val := compute()
    redis.Set(ctx, "result:"+key, val, ttl)
} else {
    // Someone else — poll until result appears
    for {
        if val, ok := redis.Get("result:"+key); ok {
            return val
        }
        time.Sleep(50*time.Millisecond)
    }
}
```

Не идеально, но работает для cross-pod дедупликации.

### 4. Metrics

Counter: singleflight hits (shared=true). Показывает насколько эффективно работает.

### 5. Per-key timeout

Если конкретный key зависнет — timeout и Forget.

### 6. Hierarchical (group of groups)

Несколько групп singleflight по типу ресурса.

---

## Реальные применения

- **Cache stampede protection** — самое частое
- **Configuration reload** — один поток перечитывает config, остальные ждут
- **Token refresh** — N goroutine видят expired token, только один делает refresh
- **Lazy initialization** — singleton initialization без `sync.Once` race
- **Dedup external API calls** — два места в коде попросили те же данные одновременно

---

## Что важно показать на собеседовании

1. **Знать что есть `golang.org/x/sync/singleflight`** — не писать свой когда не просят
2. **Понимать что singleflight ≠ cache** — он только дедуплицирует concurrent
3. **Shared result mutation** — предупредить о риске
4. **Cache stampede как мотивирующий пример** — почему это критично
5. **Distributed случай требует другого подхода** — Redis lock или брокер
6. **Forget()** для retry — знать про него
7. **DoChan + select** для ctx integration
8. **Внутренности** (если копнут): `cap 1` у `DoChan`-канала, рассылка под мьютексом, обработка паники — разобрано в [03-sync-primitives](../../../01-go-core/concurrency-and-performance/03-sync-primitives.md)

## Связки

- [Redis caching](../../../06-databases/caching/01-redis-as-cache.md) — cache stampede и singleflight в кэше
- [Concurrency и channels](../../../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md)
- [Reliability patterns: rate limiting](../../../05-system-design/reliability-patterns/04-rate-limiting.md) — другой способ защиты от наплыва
- [DDoS protection](../../../11-security/perimeter-and-traffic-protection/01-ddos-protection.md)
