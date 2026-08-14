# Задача 4: Distributed Lock

## Содержание

- [Когда lock действительно нужен](#когда-lock-действительно-нужен)
- [Контракт Redis lease](#контракт-redis-lease)
- [Acquire и безопасный Release](#acquire-и-безопасный-release)
- [Renewal и потеря владения](#renewal-и-потеря-владения)
- [Fencing tokens](#fencing-tokens)
- [Redis Cluster и Redlock](#redis-cluster-и-redlock)
- [PostgreSQL и etcd](#postgresql-и-etcd)
- [Тестирование и метрики](#тестирование-и-метрики)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Distributed lock координирует процессы через общее хранилище. В отличие от
`sync.Mutex`, владелец может исчезнуть, сеть — разделиться, а lease — истечь,
пока код всё ещё выполняет критическую секцию. Поэтому «ключ существует» не
равно «старый владелец физически остановлен».

---

## Когда lock действительно нужен

Подходящие случаи: leader election, один rebuild cache на ключ, единственный
scheduler для job и координация доступа к ресурсу без собственной транзакции.

Сначала стоит проверить более сильные и простые варианты:

- `UNIQUE` constraint или conditional `UPDATE` для данных в одной БД;
- `SELECT ... FOR UPDATE` внутри короткой transaction;
- идемпотентный consumer и transactional inbox для сообщений;
- partition ownership в broker;
- внешний API с idempotency key.

Lock сериализует добровольно сотрудничающих клиентов, но сам по себе не делает
несколько записей атомарными и не даёт exactly-once.

---

## Контракт Redis lease

Перед реализацией нужно определить:

1. `Acquire` blocking или try-lock?
2. Как caller узнаёт, что renewal прекратился и lease потерян?
3. Что означает повторный `Release`?
4. Может ли защищаемый ресурс проверить fencing token?
5. Какой отказ допустим при failover Redis?

Один Redis подходит, когда редкое одновременное выполнение после failover
приемлемо. Для correctness-critical операции одной взаимной блокировки мало:
нужен fencing на стороне ресурса или транзакционный primitive этого ресурса.

---

## Acquire и безопасный Release

Redis рекомендует acquire одной атомарной командой `SET key value NX PX ttl`,
где value уникален для конкретной попытки захвата. Release должен атомарно
сравнить value и удалить ключ.

```go
package dlock

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

var (
    ErrNotAcquired = errors.New("lock not acquired")
    ErrNotOwner    = errors.New("lock is not owned by this lease")
)

var releaseScript = redis.NewScript(`
    if redis.call("GET", KEYS[1]) == ARGV[1] then
        return redis.call("DEL", KEYS[1])
    end
    return 0
`)

type Locker struct {
    client redis.UniversalClient
    prefix string
}

type Lease struct {
    client redis.UniversalClient
    key    string
    owner  string
}

func New(client redis.UniversalClient, prefix string) (*Locker, error) {
    if client == nil {
        return nil, fmt.Errorf("redis client is nil")
    }
    return &Locker{client: client, prefix: prefix}, nil
}

func (l *Locker) TryAcquire(
    ctx context.Context,
    resource string,
    ttl time.Duration,
) (*Lease, error) {
    if resource == "" {
        return nil, fmt.Errorf("resource is empty")
    }
    if ttl <= 0 {
        return nil, fmt.Errorf("ttl must be positive")
    }

    owner, err := randomToken()
    if err != nil {
        return nil, fmt.Errorf("generate owner token: %w", err)
    }

    key := l.prefix + resource
    ok, err := l.client.SetNX(ctx, key, owner, ttl).Result()
    if err != nil {
        return nil, fmt.Errorf("acquire %q: %w", resource, err)
    }
    if !ok {
        return nil, ErrNotAcquired
    }

    return &Lease{client: l.client, key: key, owner: owner}, nil
}

func (l *Lease) Release(ctx context.Context) error {
    deleted, err := releaseScript.Run(
        ctx,
        l.client,
        []string{l.key},
        l.owner,
    ).Int64()
    if err != nil {
        return fmt.Errorf("release lock: %w", err)
    }
    if deleted == 0 {
        return ErrNotOwner
    }
    return nil
}

func randomToken() (string, error) {
    value := make([]byte, 16)
    if _, err := rand.Read(value); err != nil {
        return "", err
    }
    return hex.EncodeToString(value), nil
}
```

Отдельные `SETNX` и `EXPIRE` оставляют вечный ключ при crash между командами.
Отдельные `GET` и `DEL` позволяют прежнему владельцу удалить lease нового.

Ошибку `Release` нельзя молча игнорировать. `ErrNotOwner` означает, что TTL уже
истёк или ключ был заменён; side effect после этого нельзя считать защищённым.

---

## Renewal и потеря владения

Renewal выполняет compare-and-`PEXPIRE` одним Lua script. Но одной фоновой
goroutine недостаточно: она обязана отменить работу владельца при первой
неустранимой ошибке или несовпадении owner token.

```lua
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
```

Практичный API возвращает context критической секции и канал причины потери:

```go
type RenewableLease interface {
    WorkContext() context.Context
    Lost() <-chan error
    Release(context.Context) error
}
```

Инварианты renewal loop:

- interval, TTL и timeout одного Redis-вызова валидируются;
- `Release` сначала останавливает loop и ждёт его завершения, затем делает
  compare-and-delete;
- потеря lease вызывает cancel `WorkContext` и один раз публикует причину;
- callback проверяет context до необратимого side effect;
- repeated `Release` имеет документированный результат;
- нет `context.Background()` без deadline для сетевого renew.

Формула `interval < TTL/2` лишь оставляет запас и не является доказательством
владения: process pause или network partition могут быть длиннее TTL. Даже
успешный renewal не отменяет уже начавшийся side effect прежнего владельца.

---

## Fencing tokens

Fencing token — монотонно растущий номер lease. Защищаемый ресурс хранит
наибольший принятый номер и отклоняет меньшие:

```sql
UPDATE account_projection
SET payload = $1, fence_token = $2
WHERE id = $3 AND fence_token < $2;
```

Если изменено ноль строк, владелец устарел. Проверять token должен именно ресурс,
где выполняется запись; сравнение только внутри lock service ничего не защищает.

Для одного Redis acquire token и lock создаются одним script:

```lua
if redis.call("EXISTS", KEYS[1]) == 0 then
    local token = redis.call("INCR", KEYS[2])
    redis.call("SET", KEYS[1], ARGV[2] .. ":" .. token, "PX", ARGV[1])
    return token
end
return 0
```

В Redis Cluster оба ключа должны передаваться через `KEYS` и попадать в один
hash slot, например `{invoice:42}:lock` и `{invoice:42}:fence`.

Такой счётчик монотонен в рамках доступной истории Redis. Если failover может
потерять подтверждённый `INCR`, старое значение способно повториться. Для
строгого fencing token нужен источник с подходящими durability/consistency
гарантиями и атомарная проверка токена целевым resource.

---

## Redis Cluster и Redlock

Redlock пытается получить lease на большинстве независимых Redis masters и
учитывает время, потраченное на acquire. Официальное описание Redis приводит
алгоритм и его assumptions. При этом вокруг гарантий Redlock есть известная
дискуссия: time-based lease не останавливает paused client и зависит от модели
отказов.

Выбор формулируют через цену нарушения:

- для cache regeneration редкий двойной расчёт обычно допустим;
- для платежа или необратимого удаления «обычно один владелец» недостаточно;
- Redlock не отменяет необходимость fencing, если stale writer опасен;
- библиотеку нужно настраивать по документации конкретной версии, а не
  воспроизводить алгоритм по памяти.

Не следует утверждать, что Redlock автоматически «строже single Redis» для
любой системы: результат зависит от assumptions и защищаемого ресурса.

---

## PostgreSQL и etcd

| Вариант | Сильная сторона | Главная ловушка |
|---|---|---|
| PostgreSQL row/unique constraint | атомарность рядом с business data | держать transaction короткой |
| `pg_advisory_xact_lock` | освобождается вместе с transaction | advisory: все writers обязаны соблюдать protocol |
| session advisory lock | живёт до unlock/disconnect | при pool нужен тот же `*sql.Conn`, не случайный session |
| etcd lease + mutex | coordination на linearizable KV | отдельный cluster и всё ещё нужен fencing для stale side effect |
| Redis lease | простой и быстрый | TTL/failover допускают потерю исключительности |

Если данные уже находятся в PostgreSQL, conditional write или transaction lock
часто проще отдельного Redis. Session-level advisory lock нельзя брать через
один вызов `*sql.DB`, а освобождать через другой: pool может выбрать разные
соединения.

---

## Тестирование и метрики

Проверить нужно не только второй неуспешный acquire:

1. чужой owner не удаляет lock;
2. прежний owner после expiry не удаляет новый lease;
3. cancel останавливает blocking retry и renewal;
4. renewal loss отменяет `WorkContext`;
5. concurrent `Release` не гоняется с renewal;
6. fencing token возрастает для успешных acquisitions;
7. Redis Cluster script использует один hash slot;
8. интеграционный тест проходит с реальным Redis или testcontainer.

TTL boundary лучше проверять управляемым серверным временем, где это возможно,
или с достаточным допуском в отдельном integration test, а не миллисекундным
`Sleep` в unit test.

Полезные метрики: acquire outcome/latency, contention, lease lost, renew errors,
critical-section duration относительно TTL и stale fencing rejection.

---

## Типичные ошибки

- `SETNX`, затем отдельный `EXPIRE`.
- `DEL` без сравнения owner token.
- Считать TTL доказательством, что прежний holder остановлен.
- Продолжать работу после ошибки renewal, оставив только log entry.
- Генерировать fencing token отдельно от успешного acquire.
- Использовать polling с постоянным коротким interval без backoff и jitter.
- Делать долгий внешний вызов внутри DB transaction ради lock.
- Использовать session advisory lock через `*sql.DB` без закреплённого
  соединения.
- Обещать exactly-once благодаря lock.

---

## Interview-ready answer

1. **Как корректно взять и отпустить Redis lock?**
   - **Acquire —** `SET key random-token NX PX ttl` одной командой.
   - **Release —** Lua atomically сравнивает token и удаляет ключ.
   - **TTL —** освобождает lease после crash, но может истечь во время работы.

2. **Почему renewal недостаточно?**
   - **Lease loss —** pause или partition может быть длиннее TTL.
   - **Cancellation —** владелец должен узнать о потере и остановить работу.
   - **Fencing —** target resource отклоняет операции устаревшего владельца.

3. **Когда лучше не использовать Redis lock?**
   - **Одна БД —** constraint, conditional update или transaction lock обычно
     дают более прямую гарантию.
   - **Critical correctness —** выбирают primitive и fencing под формальную
     модель отказов.
   - **Идемпотентность —** lock не заменяет deduplication результата операции.

---

## Связанные материалы

- [Redis: Distributed Locks](https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/)
- [PostgreSQL: Advisory Locks](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)
- [Idempotency handler](./05-idempotency-handler.md)
- [Saga и Outbox](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md)
- [Martin Kleppmann: How to do distributed locking](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html)
