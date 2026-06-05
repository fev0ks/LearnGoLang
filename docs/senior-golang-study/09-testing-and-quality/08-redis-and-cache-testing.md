# Тестирование Redis и кэша

Два подхода: `testcontainers-go/redis` для полной совместимости и `miniredis` для быстрых unit-тестов без Docker.

## Содержание

- [testcontainers Redis](#testcontainers-redis)
- [miniredis — Redis без Docker](#miniredis--redis-без-docker)
- [Паттерны тестирования кэша](#паттерны-тестирования-кэша)
- [Тестирование TTL](#тестирование-ttl)
- [Тестирование cache-aside паттерна](#тестирование-cache-aside-паттерна)
- [Тестирование distributed lock](#тестирование-distributed-lock)

---

## testcontainers Redis

Подходит для интеграционных тестов, где важно поведение реального Redis (pub/sub, cluster, persistence).

```go
//go:build integration

package cache_test

import (
    "context"
    "log"
    "os"
    "testing"

    "github.com/redis/go-redis/v9"
    "github.com/testcontainers/testcontainers-go"
    tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

var testRedis *redis.Client

func TestMain(m *testing.M) {
    ctx := context.Background()

    rc, err := tcredis.Run(ctx, "redis:7-alpine")
    if err != nil {
        log.Fatalf("start redis container: %v", err)
    }
    defer testcontainers.TerminateContainer(rc)

    addr, err := rc.ConnectionString(ctx)
    if err != nil {
        log.Fatalf("get connection string: %v", err)
    }

    testRedis = redis.NewClient(&redis.Options{
        Addr: strings.TrimPrefix(addr, "redis://"),
    })
    defer testRedis.Close()

    os.Exit(m.Run())
}
```

---

## miniredis — Redis без Docker

[alicebob/miniredis](https://github.com/alicebob/miniredis) — in-process Redis совместимый сервер. Запускается мгновенно, поддерживает TTL, pub/sub, scripting.

```go
import (
    "testing"

    "github.com/alicebob/miniredis/v2"
    "github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
    t.Helper()
    mr := miniredis.RunT(t)  // автоматически закрывается после теста

    client := redis.NewClient(&redis.Options{
        Addr: mr.Addr(),
    })
    t.Cleanup(func() { client.Close() })

    return client, mr
}
```

**Когда miniredis, когда testcontainers:**

| | miniredis | testcontainers |
|---|---|---|
| Скорость запуска | < 1ms | 3-5s |
| Docker зависимость | нет | нужен |
| Совместимость | ~90% команд | 100% |
| TTL контроль | полный | реальное время |
| Pub/Sub | да | да |
| Lua scripting | да | да |
| Redis modules | нет | да |

---

## Паттерны тестирования кэша

### Базовые операции

```go
func TestUserCache_SetAndGet(t *testing.T) {
    client, _ := newTestRedis(t)
    cache := NewUserCache(client, 5*time.Minute)
    ctx := context.Background()

    user := User{ID: "u1", Email: "alice@example.com", Name: "Alice"}

    err := cache.Set(ctx, user)
    require.NoError(t, err)

    got, err := cache.Get(ctx, "u1")
    require.NoError(t, err)

    if diff := cmp.Diff(user, got); diff != "" {
        t.Errorf("cached user mismatch (-want +got):\n%s", diff)
    }
}

func TestUserCache_Get_Miss(t *testing.T) {
    client, _ := newTestRedis(t)
    cache := NewUserCache(client, 5*time.Minute)

    _, err := cache.Get(context.Background(), "non-existent")
    assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestUserCache_Delete(t *testing.T) {
    client, _ := newTestRedis(t)
    cache := NewUserCache(client, 5*time.Minute)
    ctx := context.Background()

    user := User{ID: "u1", Email: "alice@example.com"}
    require.NoError(t, cache.Set(ctx, user))

    require.NoError(t, cache.Delete(ctx, "u1"))

    _, err := cache.Get(ctx, "u1")
    assert.ErrorIs(t, err, ErrCacheMiss)
}
```

---

## Тестирование TTL

С miniredis можно управлять временем — не нужно реально ждать истечения TTL.

```go
func TestUserCache_Expires(t *testing.T) {
    client, mr := newTestRedis(t)
    cache := NewUserCache(client, 1*time.Minute)
    ctx := context.Background()

    user := User{ID: "u1", Email: "alice@example.com"}
    require.NoError(t, cache.Set(ctx, user))

    // Проверить что ключ есть
    _, err := cache.Get(ctx, "u1")
    require.NoError(t, err)

    // Промотать время вперёд — miniredis обновляет TTL
    mr.FastForward(2 * time.Minute)

    // Ключ должен истечь
    _, err = cache.Get(ctx, "u1")
    assert.ErrorIs(t, err, ErrCacheMiss, "cached value should have expired")
}

func TestUserCache_TTL_Correct(t *testing.T) {
    client, mr := newTestRedis(t)
    ttl := 5 * time.Minute
    cache := NewUserCache(client, ttl)
    ctx := context.Background()

    require.NoError(t, cache.Set(ctx, User{ID: "u1"}))

    // Проверить TTL через miniredis напрямую
    remaining := mr.TTL("user:u1")
    assert.InDelta(t, ttl.Seconds(), remaining.Seconds(), 1,
        "TTL should be close to configured value")
}
```

---

## Тестирование cache-aside паттерна

Cache-aside: сначала проверить кэш, при miss — загрузить из БД и положить в кэш.

```go
type UserService struct {
    repo  UserRepository
    cache UserCache
}

func (s *UserService) GetUser(ctx context.Context, id string) (User, error) {
    user, err := s.cache.Get(ctx, id)
    if err == nil {
        return user, nil
    }
    if !errors.Is(err, ErrCacheMiss) {
        return User{}, fmt.Errorf("cache get: %w", err)
    }

    user, err = s.repo.GetByID(ctx, id)
    if err != nil {
        return User{}, err
    }

    _ = s.cache.Set(ctx, user)  // cache best-effort
    return user, nil
}

// Тест: cache hit не идёт в репозиторий
func TestUserService_GetUser_CacheHit(t *testing.T) {
    ctrl := gomock.NewController(t)
    repo := NewMockUserRepository(ctrl)
    client, _ := newTestRedis(t)
    cache := NewUserCache(client, 5*time.Minute)
    svc := NewUserService(repo, cache)
    ctx := context.Background()

    cached := User{ID: "u1", Email: "alice@example.com"}
    require.NoError(t, cache.Set(ctx, cached))

    // repo не должен вызываться — данные в кэше
    repo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)

    got, err := svc.GetUser(ctx, "u1")
    require.NoError(t, err)
    assert.Equal(t, cached.Email, got.Email)
}

// Тест: cache miss идёт в репозиторий и кладёт в кэш
func TestUserService_GetUser_CacheMiss_PopulatesCache(t *testing.T) {
    ctrl := gomock.NewController(t)
    repo := NewMockUserRepository(ctrl)
    client, _ := newTestRedis(t)
    cache := NewUserCache(client, 5*time.Minute)
    svc := NewUserService(repo, cache)
    ctx := context.Background()

    user := User{ID: "u1", Email: "alice@example.com"}
    repo.EXPECT().GetByID(ctx, "u1").Return(user, nil).Times(1)

    // Первый вызов — cache miss
    got, err := svc.GetUser(ctx, "u1")
    require.NoError(t, err)
    assert.Equal(t, user.Email, got.Email)

    // Второй вызов — должен попасть в кэш, repo не вызывается
    repo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
    got2, err := svc.GetUser(ctx, "u1")
    require.NoError(t, err)
    assert.Equal(t, user.Email, got2.Email)
}
```

---

## Тестирование distributed lock

```go
type RedisLock struct {
    client *redis.Client
    key    string
    ttl    time.Duration
    token  string
}

func (l *RedisLock) Acquire(ctx context.Context) (bool, error) {
    ok, err := l.client.SetNX(ctx, l.key, l.token, l.ttl).Result()
    return ok, err
}

func (l *RedisLock) Release(ctx context.Context) error {
    // Lua: удалить только если token совпадает
    const script = `
        if redis.call("get", KEYS[1]) == ARGV[1] then
            return redis.call("del", KEYS[1])
        end
        return 0
    `
    return l.client.Eval(ctx, script, []string{l.key}, l.token).Err()
}

func TestRedisLock_Acquire(t *testing.T) {
    client, _ := newTestRedis(t)
    ctx := context.Background()

    t.Run("acquire when free", func(t *testing.T) {
        lock := &RedisLock{client: client, key: "lock:test-1", ttl: time.Minute, token: "token-1"}
        ok, err := lock.Acquire(ctx)
        require.NoError(t, err)
        assert.True(t, ok)
    })

    t.Run("second acquire fails while locked", func(t *testing.T) {
        lock1 := &RedisLock{client: client, key: "lock:test-2", ttl: time.Minute, token: "token-a"}
        lock2 := &RedisLock{client: client, key: "lock:test-2", ttl: time.Minute, token: "token-b"}

        ok1, err := lock1.Acquire(ctx)
        require.NoError(t, err)
        require.True(t, ok1)

        ok2, err := lock2.Acquire(ctx)
        require.NoError(t, err)
        assert.False(t, ok2, "second lock should not be acquired")
    })

    t.Run("acquire after release", func(t *testing.T) {
        lock := &RedisLock{client: client, key: "lock:test-3", ttl: time.Minute, token: "token-1"}

        ok, _ := lock.Acquire(ctx)
        require.True(t, ok)

        require.NoError(t, lock.Release(ctx))

        ok2, err := lock.Acquire(ctx)
        require.NoError(t, err)
        assert.True(t, ok2, "should acquire after release")
    })
}

func TestRedisLock_Expires(t *testing.T) {
    client, mr := newTestRedis(t)
    ctx := context.Background()

    lock1 := &RedisLock{client: client, key: "lock:exp", ttl: 30 * time.Second, token: "tok-1"}
    lock2 := &RedisLock{client: client, key: "lock:exp", ttl: 30 * time.Second, token: "tok-2"}

    ok, _ := lock1.Acquire(ctx)
    require.True(t, ok)

    // Промотать время
    mr.FastForward(31 * time.Second)

    ok2, err := lock2.Acquire(ctx)
    require.NoError(t, err)
    assert.True(t, ok2, "should acquire after TTL expires")
}
```
