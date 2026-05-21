# Задача 4: Distributed Lock

В одном процессе достаточно `sync.Mutex`. Но что если **10 pod'ов** делят одну работу, и нужно чтобы **только один** делал её одновременно? Это distributed lock — координация между процессами через shared store (обычно Redis или etcd).

## Формулировка

> "Реализуй distributed lock через Redis: Acquire(key, ttl), Release(key). Несколько pod'ов одновременно вызывают Acquire — должен пройти только один."

Use cases:
- **Leader election** — один pod из 5 делает scheduled task
- **Exclusive job** — только один воркер обрабатывает batch
- **Idempotency lock** — предотвратить параллельную обработку одного `idempotency_key`
- **Cache stampede protection** — один пересчитывает, остальные ждут

---

## Уточняющие вопросы

1. **Какой store — Redis, etcd, ZooKeeper?**
   "Redis — простой, быстрый, чаще всего. Etcd — гарантирует consistency, для critical."

2. **TTL обязателен?**
   "Да. Защита от падения holder'а — без TTL lock висит навсегда."

3. **Что если lock истёк, а holder продолжает работать?**
   "Это **fencing problem** — нужен fencing token. См. ниже."

4. **Auto-renewal (lease extension)?**
   "Опционально. Если работа > TTL — нужен renewal. Иначе достаточно big TTL."

5. **Blocking или non-blocking?**
   "Обычно non-blocking + retry в caller. Для blocking — нужна subscription (Redis Pub/Sub)."

6. **Strong consistency или OK с edge cases?**
   "Большинству OK с Redis (single instance). Для финансовых — Redlock или etcd."

---

## Базовое решение: Redis SET NX

```go
package dlock

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "time"

    "github.com/redis/go-redis/v9"
)

var ErrLockNotAcquired = errors.New("lock not acquired")
var ErrLockNotHeld = errors.New("lock not held by this client")

type Lock struct {
    client *redis.Client
    key    string
    value  string  // уникальный для данного holder'а
}

type Locker struct {
    client *redis.Client
}

func New(client *redis.Client) *Locker {
    return &Locker{client: client}
}

// Acquire пытается взять lock. Non-blocking.
// Возвращает Lock с уникальным token'ом для последующего Release.
func (l *Locker) Acquire(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
    // Уникальный value — чтобы только наш Release разлочил
    val, err := randomToken()
    if err != nil {
        return nil, err
    }

    // SET key value NX EX ttl — атомарно
    ok, err := l.client.SetNX(ctx, key, val, ttl).Result()
    if err != nil {
        return nil, err
    }
    if !ok {
        return nil, ErrLockNotAcquired
    }

    return &Lock{client: l.client, key: key, value: val}, nil
}

// Release освобождает lock. Проверяет что мы — owner (через value).
func (lock *Lock) Release(ctx context.Context) error {
    // Lua script: атомарно "если value == ours, delete; иначе ничего"
    script := redis.NewScript(`
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("DEL", KEYS[1])
        else
            return 0
        end
    `)

    result, err := script.Run(ctx, lock.client, []string{lock.key}, lock.value).Int()
    if err != nil {
        return err
    }
    if result == 0 {
        return ErrLockNotHeld  // уже expired или кем-то другим перевзят
    }
    return nil
}

func randomToken() (string, error) {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return hex.EncodeToString(b), nil
}
```

**Использование:**

```go
locker := dlock.New(redisClient)

lock, err := locker.Acquire(ctx, "leader-election", 30*time.Second)
if errors.Is(err, dlock.ErrLockNotAcquired) {
    // Кто-то другой держит — нам не делать
    return
}
if err != nil {
    return err
}
defer lock.Release(ctx)

// Critical section — только мы здесь
doExclusiveWork()
```

**Ключевые моменты:**

### Почему SET NX, а не отдельный SETNX + EXPIRE

```redis
# ❌ Не атомарно
SETNX key value
EXPIRE key 30  # ← если client crash'нется между ними, key без TTL
```

Атомарно через одну команду:
```redis
SET key value NX EX 30
```

### Почему уникальный value

```redis
# Без unique value
SET lock-key "1" NX EX 30

# A acquired
# A is slow (GC pause, network), TTL expires
# B acquires (SET NX succeeds)
# A wakes up, calls DEL lock-key
# B's lock released by A!
```

С unique value — Release проверяет "это мой lock?" через Lua. Кто-то другой — не трогаем.

### Почему Lua

```redis
# Не атомарно — race condition
val = GET key
if val == "my-token":
    DEL key  # ← в этот момент мог expire и другой acquire
```

Lua script выполняется атомарно на Redis side.

---

## Production-grade: с lease renewal

Если работа дольше TTL — нужно **продлевать** lock'у жизнь.

```go
package dlock

import (
    "context"
    "sync"
    "time"
)

// LockWithRenew автоматически продлевает TTL в background.
type LockWithRenew struct {
    Lock

    cancel   context.CancelFunc
    done     chan struct{}
    mu       sync.Mutex
    released bool
}

func (l *Locker) AcquireWithRenew(
    ctx context.Context,
    key string,
    ttl time.Duration,
    renewInterval time.Duration,
) (*LockWithRenew, error) {
    base, err := l.Acquire(ctx, key, ttl)
    if err != nil {
        return nil, err
    }

    renewCtx, cancel := context.WithCancel(context.Background())
    lwr := &LockWithRenew{
        Lock:   *base,
        cancel: cancel,
        done:   make(chan struct{}),
    }

    go lwr.renewLoop(renewCtx, ttl, renewInterval)
    return lwr, nil
}

func (l *LockWithRenew) renewLoop(ctx context.Context, ttl, interval time.Duration) {
    defer close(l.done)

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := l.renew(context.Background(), ttl); err != nil {
                // Не смогли renew — lock потерян
                return
            }
        }
    }
}

func (l *LockWithRenew) renew(ctx context.Context, ttl time.Duration) error {
    script := redis.NewScript(`
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("PEXPIRE", KEYS[1], ARGV[2])
        else
            return 0
        end
    `)

    result, err := script.Run(ctx, l.client,
        []string{l.key}, l.value, ttl.Milliseconds()).Int()
    if err != nil {
        return err
    }
    if result == 0 {
        return ErrLockNotHeld
    }
    return nil
}

func (l *LockWithRenew) Release(ctx context.Context) error {
    l.mu.Lock()
    if l.released {
        l.mu.Unlock()
        return nil
    }
    l.released = true
    l.mu.Unlock()

    l.cancel()  // остановить renewLoop
    <-l.done    // подождать пока renewLoop вышел

    return l.Lock.Release(ctx)
}
```

**Использование:**

```go
lock, err := locker.AcquireWithRenew(ctx, "long-job",
    30*time.Second,   // TTL
    10*time.Second,   // renew interval — < TTL/2 для надёжности
)
if err != nil { ... }
defer lock.Release(context.Background())

// Long-running work — TTL продляется автоматом каждые 10s
longRunningJob()
```

**Правило:** `renewInterval < TTL / 2` — если один renew пропустится (network blip), следующий ещё успеет до expiry.

---

## Fencing tokens — защита от false ownership

Сценарий "GC pause" — известная проблема distributed locks:

```
1. Client A acquires lock (TTL=30s)
2. Client A: GC pause 35 секунд
3. TTL expires, lock released
4. Client B acquires same lock
5. Client A wakes up, продолжает критическую секцию
6. Оба клиента работают одновременно — BAD
```

Решение — **fencing token**: монотонно растущее число, которое **shared resource** (БД, файл) **проверяет**.

```
1. A acquires lock → token=42
2. A doing work, sends update to DB with token=42
3. A pauses (GC)
4. TTL expires
5. B acquires → token=43
6. B updates DB with token=43 → DB accepts
7. A wakes up, sends update with token=42 → DB rejects (43 > 42)
```

```go
// Lock с fencing token
type FencedLock struct {
    Lock
    Token int64
}

func (l *Locker) AcquireWithFencing(ctx context.Context, key string, ttl time.Duration) (*FencedLock, error) {
    // Atomic incr counter для token
    token, err := l.client.Incr(ctx, key+":fence").Result()
    if err != nil {
        return nil, err
    }

    // Acquire с token как value
    val := fmt.Sprintf("token-%d", token)
    ok, err := l.client.SetNX(ctx, key, val, ttl).Result()
    if err != nil {
        return nil, err
    }
    if !ok {
        return nil, ErrLockNotAcquired
    }

    return &FencedLock{
        Lock:  Lock{client: l.client, key: key, value: val},
        Token: token,
    }, nil
}

// Использование
lock, _ := locker.AcquireWithFencing(ctx, "db-write", 30*time.Second)
defer lock.Release(ctx)

err := db.UpdateWithFence(ctx, data, lock.Token)
if errors.Is(err, ErrStaleFencingToken) {
    // Кто-то другой уже взял lock — мы устарели
}
```

**В БД:**
```sql
UPDATE rows SET ..., fence_token = $1 WHERE id = $2 AND fence_token < $1
```

Если update affected 0 rows — наш token устарел.

См. знаменитый пост Martin Kleppmann: ["How to do distributed locking"](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html).

---

## Redlock — Redis cluster

Если Redis = single instance, его падение = потеря всех locks. **Redlock** — алгоритм для distributed Redis (N независимых instances).

Принцип:
1. Acquire на **большинстве** instances (e.g., 3 из 5)
2. Если acquire'нул >= N/2 + 1 — lock наш
3. Release на всех (даже где не acquire'нул)

Реализация — [go-redsync/redsync](https://github.com/go-redsync/redsync).

```go
import "github.com/go-redsync/redsync/v4"

rs := redsync.New(pools...)
mutex := rs.NewMutex("my-lock", redsync.WithExpiry(30*time.Second))

if err := mutex.LockContext(ctx); err != nil {
    return err
}
defer mutex.UnlockContext(ctx)
```

**Споры:** Kleppmann критикует Redlock как "недостаточно safe". Для **critical** operations (платежи, deletions) — лучше etcd или PostgreSQL advisory lock.

---

## Альтернативы Redis

### PostgreSQL advisory locks

```sql
SELECT pg_try_advisory_lock(hashtext('my-lock'));  -- non-blocking
SELECT pg_advisory_lock(hashtext('my-lock'));      -- blocking

-- Когда закончил
SELECT pg_advisory_unlock(hashtext('my-lock'));
```

Плюсы:
- Strong consistency через Postgres
- Не нужен Redis
- Auto-release при disconnect

Минусы:
- Привязан к Postgres
- Не отличный API для long-held locks

### etcd

```go
import "go.etcd.io/etcd/client/v3/concurrency"

s, _ := concurrency.NewSession(etcdClient)
defer s.Close()

m := concurrency.NewMutex(s, "/my-lock")
if err := m.Lock(ctx); err != nil { ... }
defer m.Unlock(ctx)
```

Plюс: strong consistency, lease-based (auto-release при disconnect).
Минус: операционная сложность etcd кластера.

### ZooKeeper

Старая школа, ephemeral nodes как locks. Сложно operate.

---

## Тесты

```go
func TestLock_Acquire(t *testing.T) {
    redis := setupRedis(t)
    locker := New(redis)
    ctx := context.Background()

    lock, err := locker.Acquire(ctx, "test-key", 5*time.Second)
    if err != nil {
        t.Fatal(err)
    }
    defer lock.Release(ctx)

    // Второй acquire должен fail
    _, err = locker.Acquire(ctx, "test-key", 5*time.Second)
    if !errors.Is(err, ErrLockNotAcquired) {
        t.Errorf("got %v, want ErrLockNotAcquired", err)
    }
}

func TestLock_ReleaseOnlyByOwner(t *testing.T) {
    redis := setupRedis(t)
    locker := New(redis)
    ctx := context.Background()

    lock1, _ := locker.Acquire(ctx, "key", 10*time.Second)

    // Fake — другой клиент
    lock2 := &Lock{
        client: redis,
        key:    "key",
        value:  "different-token",
    }

    // lock2 не должен разлочить
    err := lock2.Release(ctx)
    if !errors.Is(err, ErrLockNotHeld) {
        t.Errorf("got %v, want ErrLockNotHeld", err)
    }

    // lock1 всё ещё держит
    val, _ := redis.Get(ctx, "key").Result()
    if val == "" {
        t.Error("lock1 was released")
    }

    lock1.Release(ctx)
}

func TestLock_TTLExpires(t *testing.T) {
    redis := setupRedis(t)
    locker := New(redis)
    ctx := context.Background()

    _, err := locker.Acquire(ctx, "k", 100*time.Millisecond)
    if err != nil {
        t.Fatal(err)
    }

    time.Sleep(150 * time.Millisecond)

    // После TTL — новый acquire должен пройти
    _, err = locker.Acquire(ctx, "k", time.Second)
    if err != nil {
        t.Errorf("after TTL expected acquire OK, got %v", err)
    }
}
```

Нужна реальная Redis. Используй testcontainers-go.

---

## Подводные камни

### 1. Lock без TTL

```go
SET lock-key "1" NX  // ← нет TTL → лежит навсегда
```

Если client crash — lock висит навсегда. **Всегда EX/PX**.

### 2. TTL слишком короткий

```go
locker.Acquire(ctx, "k", 1*time.Second)
// Работа занимает 5 секунд → lock expires → другой acquire → race
```

TTL > worst-case duration. Или renewal.

### 3. Lock без unique value

```redis
SET lock-key "1" NX EX 30
```

Любой `DEL lock-key` released. После TTL и захвата другим — наш DEL released его lock. Используй token.

### 4. Release без Lua

```python
# Не атомарно
val = GET key
if val == my_token: DEL key  # ← race condition
```

Lua script атомарен.

### 5. Polling instead of subscription

```go
for {
    lock, err := acquire(ctx, key)
    if err == nil { break }
    time.Sleep(100 * time.Millisecond)  // ← busy loop
}
```

Лучше — exponential backoff, или Redis subscription к event "lock released" (etcd watch).

### 6. Long-held lock без renewal

```go
lock.Acquire(ctx, "k", 30*time.Second)
longJob()  // 60 секунд работает
// Lock истёк на 30-й секунде! Другой клиент мог acquire.
```

Renewal goroutine или fencing token.

### 7. GC pause проблема

Уже обсуждалось. Защита — fencing tokens.

### 8. Clock skew между Redis серверами

Redlock полагается на time-based reasoning. Если часы рассинхрон — алгоритм небезопасен. Kleppmann's критика.

### 9. Использовать lock для consistency, а не consistency для lock

Lock защищает от **concurrent modifications**, но БД transaction уже это делает. Если данные в одной БД — обычная transaction сильнее distributed lock.

### 10. Не закрывать renewal goroutine

```go
go lock.renewLoop(...)
// Defer Release вызывается, но renewLoop никогда не выходит
```

Cancel renewal context при Release.

---

## Возможные расширения

### 1. Read-Write distributed lock

Multiple readers OR one writer. Redis: shared counter для readers + exclusive flag для writer.

### 2. Reentrant lock

Same holder может Acquire дважды. Store recursion depth в value.

### 3. Lock с queue

Несколько waiter'ов — fair ordering. Использовать sorted set с timestamp.

### 4. Distributed semaphore

Не 1 holder, а N. Counter в Redis с atomic increment/decrement.

### 5. Leader election (specific case of lock)

Один из N pod'ов берёт lock с long TTL + auto-renewal. Если падает → другой возьмёт.

---

## Что важно показать на собеседовании

1. **SET NX EX** атомарно — один command, не SETNX + EXPIRE
2. **Unique value (token)** — защита от чужого Release
3. **Lua script для Release** — атомарный check + DEL
4. **TTL обязателен** — защита от висящих locks
5. **Lease renewal** — для long-running locks
6. **Fencing tokens** — защита от GC pause
7. **Trade-offs Redis vs etcd vs Postgres** — Redis fast но not strict; etcd safe но complex
8. **Kleppmann's critique** Redlock — знать что есть споры

## Связки

- [Redis caching](../../../06-databases/caching/01-redis-as-cache.md) — другие Redis primitives
- [Idempotency handler](./05-idempotency-handler.md) — lock для предотвращения concurrent одинаковых запросов
- [Saga и Outbox](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md) — distributed coordination
- [Kleppmann: How to do distributed locking](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html) — must read
- [go-redsync/redsync](https://github.com/go-redsync/redsync) — Redlock for Go
