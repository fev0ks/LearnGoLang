# Задача 6: Singleflight

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Базовое решение](#базовое-решение)
- [Библиотечная реализация](#библиотечная-реализация-golangorgxsyncsingleflight)
  - [`DoChan` для async API](#dochan-для-async-api)
- [Singleflight для cache stampede](#singleflight-для-cache-stampede)
- [Подводный камень: shared result mutation](#подводный-камень-shared-result-mutation)
- [Singleflight + context](#singleflight--context)
- [Singleflight и errors](#singleflight-и-errors)
- [Тесты](#тесты)
- [Подводные камни](#подводные-камни)
- [Возможные расширения](#возможные-расширения)
- [Реальные применения](#реальные-применения)
- [Interview-ready answer](#interview-ready-answer)
- [Связки](#связки)

Singleflight дедуплицирует одновременно выполняемые вызовы с одинаковым ключом.
Один caller запускает функцию, остальные ожидают тот же результат. После
завершения результат не кэшируется.

---

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

import (
    "fmt"
    "runtime/debug"
    "sync"
)

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

func (g *Group) Do(key string, fn func() (any, error)) (val any, err error) {
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

    // Cleanup обязан выполниться и при panic callback.
    defer func() {
        if recovered := recover(); recovered != nil {
            c.res.err = fmt.Errorf("singleflight panic: %v\n%s", recovered, debug.Stack())
        }

        g.mu.Lock()
        if g.m[key] == c {
            delete(g.m, key)
        }
        g.mu.Unlock()

        c.wg.Done()
        val, err = c.res.val, c.res.err
    }()

    c.res.val, c.res.err = fn()
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

## Библиотечная реализация: `golang.org/x/sync/singleflight`

В реальной работе обычно используют готовую реализацию из внешнего модуля
`golang.org/x/sync`:

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
        // Результат был разделён хотя бы между двумя callers.
        // shared может быть true и у caller, который выполнял fn.
        s.metrics.SingleflightHit.Inc()
    }
    return v.(*User), nil
}
```

**Что даёт библиотечная версия:**
- `Do(key, fn) (val any, err error, shared bool)` — `shared` показывает был ли результат разделён
- `DoChan(key, fn) <-chan Result` — async API
- `Forget(key)` — забыть ещё выполняющийся call и разрешить новый независимый запуск

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

Полезно когда хочешь иметь возможность отменить ожидание через `ctx`, но **сам
`fn` не отменяется**. Возвращаемый channel содержит ровно один результат и не
закрывается, поэтому читать из него повторно нельзя. Panic callback библиотека
не превращает в обычный `error`: он распространяется как panic.

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
        sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
        defer cancel()

        // Повторная проверка закрывает окно между внешним miss и входом в Do.
        if data, err := r.cache.Get(sharedCtx, key).Bytes(); err == nil {
            var cached User
            if err := json.Unmarshal(data, &cached); err == nil {
                return &cached, nil
            }
        }

        u, err := r.db.GetUser(sharedCtx, id)
        if err != nil {
            return nil, err
        }
        // Сохраняем в кэш
        if data, err := json.Marshal(u); err == nil {
            // Cache fill — best effort; в production ошибку нужно записать в
            // лог/метрику, но успешный DB result из-за неё не теряется.
            _ = r.cache.Set(sharedCtx, key, data, 5*time.Minute).Err()
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

### 4. Возвращать immutable-представление

Например, вернуть `string` с сериализованным JSON и отдельно декодировать его у
каждого caller. `[]byte` не является immutable и без копирования остаётся общей
изменяемой памятью.

---

## Singleflight + context

`singleflight.Do` **сам** не принимает context. Но fn может принять и проверить:

```go
func (s *Service) GetUser(ctx context.Context, id int64) (*User, error) {
    v, err, _ := s.g.Do(key, func() (any, error) {
        // Наивный вариант: shared work зависит от ctx первого caller'а.
        return s.repo.GetByID(ctx, id)
    })
    if err != nil {
        return nil, err
    }
    return v.(*User), nil
}
```

**Проблема:** если первый waiter имеет timeout 1 секунду, второй 10 секунд — второй "наследует" timeout первого (он отменится через 1 секунду).

**Решение:** отделить lifetime shared work от отмены первого caller, но ограничить
его собственным timeout; ожидание каждого caller сделать через `DoChan`:

```go
ch := g.DoChan(key, func() (any, error) {
    // Сохраняем values, но отделяемся от cancellation первого caller.
    sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
    defer cancel()
    return loadFromDB(sharedCtx, id)
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

После завершения вызов автоматически удаляется, поэтому следующий `Do` и без
`Forget` снова выполнит `fn`. `Forget(key)` нужен, пока старый вызов ещё
выполняется: он разрешает следующему caller запустить второй независимый вызов с
тем же ключом. Это осознанно нарушает дедупликацию и может увеличить нагрузку.

---

## Тесты

```go
import (
    "context"
    "errors"
    "fmt"
    "sync/atomic"
    "testing"
    "time"

    "golang.org/x/sync/singleflight"
)

func TestSingleflight_Dedups(t *testing.T) {
    var g singleflight.Group
    var calls atomic.Int32
    release := make(chan struct{})

    fn := func() (any, error) {
        calls.Add(1)
        <-release
        return "value", nil
    }

    channels := make([]<-chan singleflight.Result, 0, 100)
    for i := 0; i < 100; i++ {
        channels = append(channels, g.DoChan("key", fn))
    }
    close(release)

    for _, ch := range channels {
        result := <-ch
        if result.Err != nil || result.Val != "value" {
            t.Errorf("got (%v, %v), want (value, nil)", result.Val, result.Err)
        }
    }
    if calls.Load() != 1 {
        t.Errorf("fn called %d times, expected 1", calls.Load())
    }
}

func TestSingleflight_SharedFlag(t *testing.T) {
    var g singleflight.Group
    release := make(chan struct{})

    fn := func() (any, error) {
        <-release
        return 42, nil
    }

    first := g.DoChan("k", fn)
    second := g.DoChan("k", fn)
    close(release)

    r1, r2 := <-first, <-second
    if !r1.Shared || !r2.Shared {
        t.Fatalf("shared flags = (%v, %v), want both true", r1.Shared, r2.Shared)
    }
}

func TestSingleflight_DifferentKeys(t *testing.T) {
    var g singleflight.Group
    var calls atomic.Int32
    release := make(chan struct{})

    fn := func() (any, error) {
        calls.Add(1)
        <-release
        return "value", nil
    }

    channels := make([]<-chan singleflight.Result, 0, 100)
    for i := 0; i < 100; i++ {
        key := fmt.Sprintf("key-%d", i%5)
        channels = append(channels, g.DoChan(key, fn))
    }
    close(release)
    for _, ch := range channels {
        <-ch
    }

    if calls.Load() != 5 {
        t.Errorf("calls %d, expected 5", calls.Load())
    }
}

func TestSingleflight_ErrorShared(t *testing.T) {
    var g singleflight.Group
    expectedErr := errors.New("fail")
    release := make(chan struct{})

    fn := func() (any, error) {
        <-release
        return nil, expectedErr
    }

    channels := make([]<-chan singleflight.Result, 0, 10)
    for i := 0; i < 10; i++ {
        channels = append(channels, g.DoChan("key", fn))
    }
    close(release)

    for _, ch := range channels {
        if result := <-ch; !errors.Is(result.Err, expectedErr) {
            t.Errorf("got %v, want %v", result.Err, expectedErr)
        }
    }
}

func TestSingleflight_DoChan(t *testing.T) {
    var g singleflight.Group
    release := make(chan struct{})

    fn := func() (any, error) {
        <-release
        return "result", nil
    }

    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    ch := g.DoChan("k", fn)

    select {
    case r := <-ch:
        t.Fatalf("shouldn't receive before release: %v", r)
    case <-ctx.Done():
        // Expected — caller timeout'ed
    }

    // Дожидаемся shared work, чтобы тест не оставлял фоновую goroutine.
    close(release)
    if result := <-ch; result.Err != nil || result.Val != "result" {
        t.Fatalf("result = (%v, %v)", result.Val, result.Err)
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

### 4. `Do` блокирует caller

```go
v, _, _ := g.Do(key, fn)  // ← блокирует пока не закончится fn
```

Если fn зависнет — все waiter'ы зависнут. Используй DoChan + select с ctx если нужен timeout.

### 5. `Forget` не отменяет старый call

```go
g.Do(k, fn)  // fn выполнился, call удалён из map
g.Forget(k)  // ← already removed, no-op
g.Do(k, fn)  // новый call (правильно)
```

`Forget` безопасен для структуры данных, но не останавливает старый `fn`. Пока он
ещё работает, следующий `Do` может запустить второй `fn` с тем же key.

### 6. Не подходит для distributed scenarios

Singleflight — **local** (в одном процессе). На 10 pod'ах будет 10 параллельных DB calls (по одному на pod). Для distributed dedup — нужен distributed lock (Redis SET NX) или leader election.

### 7. Custom channel implementation может блокировать рассылку

В обычном Do — все waiter'ы синхронны. Slow waiter не блокирует других (они уже получили `wg`). Но если используешь свою реализацию через channel — slow consumer может block.

### 8. Не для long-running operations

Чем дольше операция, тем больше waiter'ов и ресурсов она удерживает. Для задач на
минуты обычно лучше job queue с persistent status, retry и независимым polling.

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

Простой `SET NX` с константой `"1"` недостаточен: владелец может потерять lease,
а затем удалить уже чужой lock; polling без context может зависнуть; TTL может
истечь раньше вычисления.

Безопаснее использовать уникальный ownership token, lease с контролируемым
продлением и compare-and-delete через Lua. Waiters повторно проверяют result с
bounded backoff и своим context. Для side effects одного lock всё равно мало:
после истечения lease старый владелец способен продолжить работу, поэтому нужны
fencing token или идемпотентная запись. Часто проще принять «один вызов на pod»
или вынести работу в очередь, чем строить distributed singleflight.

### 4. Metrics

Counter: singleflight hits (shared=true). Показывает насколько эффективно работает.

### 5. Per-key timeout

Shared work получает собственный timeout. `Forget` можно использовать, чтобы
разрешить новый запуск, но старый `fn` он не останавливает.

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

## Interview-ready answer

**1. Что делает singleflight?**

- Он объединяет одновременные вызовы с одинаковым key: один caller выполняет
  функцию, остальные получают тот же результат.
- Это не cache: после завершения следующий вызов снова запустит функцию.

**2. Что означает `shared`?**

- Результат был выдан нескольким callers. Флаг может быть `true` и у caller,
  который фактически выполнял функцию.
- Объект результата общий, поэтому callers не должны конкурентно изменять его.

**3. Как работать с context?**

- `Do` блокирует и не принимает context. `DoChan` позволяет каждому caller ждать
  через свой `select`, но само shared-вычисление нужно запускать с отдельным
  ограниченным context.

**4. Как singleflight защищает cache?**

- После внешнего cache miss код входит в `Do` и повторно проверяет cache внутри,
  затем только один caller обращается к DB и заполняет cache.
- Группа локальна процессу; между pods нужны другой coordination mechanism или
  допустимая дедупликация «один запрос на pod».

---

## Связки

- [Redis caching](../../../06-databases/caching/01-redis-as-cache.md) — cache stampede и singleflight в кэше
- [Concurrency и channels](../../../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md)
- [Reliability patterns: rate limiting](../../../05-system-design/reliability-patterns/04-rate-limiting.md) — другой способ защиты от наплыва
- [DDoS protection](../../../11-security/perimeter-and-traffic-protection/01-ddos-protection.md)
- [`golang.org/x/sync/singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight) — официальный API и семантика `DoChan`/`Forget`
