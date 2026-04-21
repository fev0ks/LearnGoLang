# Redis

Redis это in-memory data store с набором структур данных.

Конкретные production-сценарии с Go-кодом — в [08a-redis-real-scenarios.md](./08a-redis-real-scenarios.md).  
Реализации rate limiters (Fixed Window, Sliding Window, Token Bucket) — в [08b-redis-rate-limiters.md](./08b-redis-rate-limiters.md).

## Содержание

- [Где используется](#где-используется)
- [Persistence: RDB vs AOF](#persistence-rdb-vs-aof)
- [Eviction policies](#eviction-policies)
- [Redis Sentinel vs Redis Cluster](#redis-sentinel-vs-redis-cluster)
- [Distributed lock: подводные камни](#distributed-lock-подводные-камни)
- [Сильные стороны](#сильные-стороны)
- [Слабые стороны](#слабые-стороны)
- [Когда выбирать](#когда-выбирать)
- [Когда не выбирать](#когда-не-выбирать)
- [Типичные ошибки](#типичные-ошибки)
- [Go: go-redis](#go-go-redis)
- [Interview-ready answer](#interview-ready-answer)
- [Query examples](#query-examples)

## Где используется

- cache;
- rate limiting;
- counters;
- sessions;
- distributed locks (с оговорками);
- leaderboards;
- queues-lite / pub-sub;
- ephemeral fast state.

## Persistence: RDB vs AOF

Redis предлагает несколько режимов persistence. Выбор зависит от требований к durability.

**RDB (snapshotting)**: Redis делает полный снапшот данных на диск раз в N секунд (или при M изменениях). Быстрый restart — загружается один файл. Минус: при сбое теряются изменения с момента последнего snapshot.

**AOF (Append-Only File)**: каждая write-команда дописывается в лог. Настраивается через `appendfsync`:
- `always` — fsync на каждую команду (безопасно, медленно);
- `everysec` (рекомендуется) — fsync раз в секунду, теряется максимум 1 секунда данных;
- `no` — fsync на усмотрение ОС.

**No persistence**: только в памяти. Подходит для pure cache — при рестарте данные теряются, основная DB является source of truth.

**RDB + AOF**: можно включить оба. AOF используется при restart.

Правило: если Redis — cache, persistence может быть отключена. Если Redis — primary storage (сессии, очереди), нужен AOF `everysec` как минимум.

## Eviction policies

При достижении `maxmemory` Redis должен что-то удалять. Политика задается `maxmemory-policy`:

| Policy | Что удаляет |
|---|---|
| `noeviction` | отказывает в записи (error) |
| `allkeys-lru` | самый давно неиспользуемый из всех ключей |
| `volatile-lru` | самый давно неиспользуемый из ключей с TTL |
| `allkeys-lfu` | наименее часто используемый из всех |
| `volatile-ttl` | с наименьшим оставшимся TTL |
| `allkeys-random` | случайный из всех |

Для cache: обычно `allkeys-lru` или `allkeys-lfu`.  
Для mixed storage (и cache, и persistent data без TTL): `volatile-lru`, чтобы не удалялись ключи без TTL.  
Для critical data: `noeviction` + алерт на использование памяти.

## Redis Sentinel vs Redis Cluster

**Redis Sentinel** — high availability для single-primary конфигурации:
- несколько нод (1 primary + N replicas);
- Sentinel процессы мониторят primary и делают failover при его падении;
- клиент подключается через Sentinel и получает адрес актуального primary.

Подходит для: HA без горизонтального масштабирования, умеренные объемы данных.

**Redis Cluster** — горизонтальное шардирование:
- данные делятся на 16384 hash slots между несколькими master-нодами;
- каждый мастер может иметь реплики;
- клиент направляет команды на правильную ноду по ключу.

Ограничение Cluster: multi-key операции работают только если все ключи на одном shard — используй `{hash tags}` для группировки: `{user:42}:session` и `{user:42}:profile` попадут на одну ноду.

Подходит для: объемы данных больше одной ноды, очень высокий throughput.

## Distributed lock: подводные камни

Базовая реализация через `SET NX PX` (атомарная операция):

```text
SET lock:resource 42 NX PX 5000
```

`NX` — только если не существует. `PX 5000` — TTL 5 секунд.

Освобождение должно быть через Lua-скрипт (атомарно проверить и удалить):

```lua
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
```

Проблемы:
- TTL истек, но процесс ещё работает — другой процесс захватил lock, оба думают что держат его;
- при pause в GC или slow syscall lock может истечь неожиданно.

**Redlock** — алгоритм для lock на нескольких независимых Redis нодах. Спорный: критики (в частности Мартин Клеперман) указывают, что при pause процесса lock может быть перехвачен другим узлом, что делает Redlock небезопасным при строгих гарантиях. Для некритичного rate limiting и advisory locks — достаточно; для финансовых операций — нет.

## Сильные стороны

- очень низкая latency (sub-millisecond);
- богатые data structures: String, Hash, List, Set, Sorted Set, Stream, HyperLogLog;
- TTL из коробки;
- atomic commands и Lua scripting;
- удобен для hot data и cache-aside;
- pipeline для batch команд.

## Слабые стороны

- memory cost — все данные в RAM;
- не всегда правильный source of truth;
- cache invalidation сложно делать правильно;
- hot keys могут стать bottleneck (single-threaded command loop);
- distributed lock — не серебряная пуля.

## Когда выбирать

Выбирай Redis, если:
- нужен быстрый cache с TTL;
- нужны counters и rate limiting;
- надо хранить ephemeral state (сессии, временные lock'и);
- primary DB не должна обслуживать hot read path.

## Когда не выбирать

Не лучший выбор, если:
- нужна сложная transactional model;
- данные должны быть durable source of truth без потерь;
- объем больше доступной RAM;
- нужны ad-hoc relational queries.

## Типичные ошибки

- считать Redis "просто быстрой базой" без продумывания persistence и eviction;
- хранить критичные данные без persistence или с `noeviction` без мониторинга памяти;
- не ставить TTL — memory растет без ограничений;
- делать unbounded keys (ключ содержит `*` или неограниченный counter);
- не думать о hot keys при высокой нагрузке на один ключ;
- использовать `KEYS *` в production — блокирует event loop;
- плохая реализация distributed lock (не атомарное освобождение).

## Go: go-redis

```go
import "github.com/redis/go-redis/v9"

rdb := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
    PoolSize: 10,
})

// простая операция
err := rdb.Set(ctx, "key", "value", time.Hour).Err()
val, err := rdb.Get(ctx, "key").Result()

// pipeline — несколько команд за один RTT
pipe := rdb.Pipeline()
pipe.Set(ctx, "k1", "v1", time.Minute)
pipe.Set(ctx, "k2", "v2", time.Minute)
pipe.Incr(ctx, "counter")
_, err = pipe.Exec(ctx)

// distributed lock (простой)
ok, err := rdb.SetNX(ctx, "lock:resource", uniqueID, 5*time.Second).Result()
if !ok {
    return ErrLocked
}
defer releaseLock(ctx, rdb, "lock:resource", uniqueID)
```

Для Sentinel:

```go
rdb := redis.NewFailoverClient(&redis.FailoverOptions{
    MasterName:    "mymaster",
    SentinelAddrs: []string{"sentinel1:26379", "sentinel2:26379"},
})
```

## Interview-ready answer

Redis — in-memory store для cache, счётчиков, rate limiting и ephemeral state. Для senior уровня важно понимать persistence: RDB теряет данные с момента последнего snapshot, AOF с `everysec` теряет максимум секунду. Eviction policy определяет поведение при заполнении памяти — для pure cache `allkeys-lru`; для mixed storage `volatile-lru`. Sentinel дает HA, Cluster — горизонтальное шардирование, но multi-key операции работают только в рамках одного shard. Distributed lock через `SET NX PX` работает для advisory lock, но Redlock спорен при строгих гарантиях. Hot keys — single point of bottleneck, Redis однопоточен для команд.

## Query examples

String key с TTL:

```text
SET session:abc123 "user-42" EX 3600
GET session:abc123
TTL session:abc123
```

Rate limiting counter:

```text
INCR rate:user:42:2026-04-20-10:00
EXPIRE rate:user:42:2026-04-20-10:00 60
```

Hash для профиля:

```text
HSET user:42 email user@example.com status active plan pro
HGETALL user:42
HGET user:42 plan
```

Sorted Set для leaderboard:

```text
ZADD leaderboard 1500 user:42
ZADD leaderboard 2000 user:99
ZREVRANGE leaderboard 0 9 WITHSCORES
ZRANK leaderboard user:42
```

HyperLogLog для уникальных посетителей:

```text
PFADD visitors:2026-04-20 user:1 user:2 user:3
PFCOUNT visitors:2026-04-20
```
