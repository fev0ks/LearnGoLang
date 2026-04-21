# Redis: реальные сценарии использования

Companion к [08-redis.md](./08-redis.md). Конкретные production-паттерны с Go-кодом.

## Содержание

- [Сценарий 1: Cache-aside](#сценарий-1-cache-aside)
- [Сценарий 2: Session storage](#сценарий-2-session-storage)
- [Сценарий 3: Idempotency keys](#сценарий-3-idempotency-keys)
- [Сценарий 4: Distributed advisory lock](#сценарий-4-distributed-advisory-lock)
- [Сценарий 5: Leaderboard с Sorted Set](#сценарий-5-leaderboard-с-sorted-set)
- [Сценарий 6: Pub/Sub для real-time уведомлений](#сценарий-6-pubsub-для-real-time-уведомлений)
- [Сценарий 7: Job queue](#сценарий-7-job-queue)
- [Сценарий 8: Счётчики и аналитика](#сценарий-8-счётчики-и-аналитика)
- [Антипаттерны](#антипаттерны)
- [Interview-ready answer](#interview-ready-answer)

## Сценарий 1: Cache-aside

**Проблема**: каждый запрос `/products/{id}` идёт в PostgreSQL. При 500 RPS это 500 запросов в секунду к БД, хотя продукты меняются редко.

**Паттерн cache-aside**: приложение сначала проверяет Redis, при промахе идёт в БД и кладёт результат в кэш.

```go
func (s *ProductService) GetProduct(ctx context.Context, id string) (*Product, error) {
    cacheKey := "product:" + id

    // 1. попытка из кэша
    cached, err := s.redis.Get(ctx, cacheKey).Bytes()
    if err == nil {
        var p Product
        if err := json.Unmarshal(cached, &p); err == nil {
            return &p, nil
        }
    }

    // 2. промах — идём в БД
    p, err := s.db.GetProduct(ctx, id)
    if err != nil {
        return nil, err
    }

    // 3. кладём в кэш с TTL
    data, _ := json.Marshal(p)
    s.redis.Set(ctx, cacheKey, data, 5*time.Minute)

    return p, nil
}

// инвалидация при обновлении
func (s *ProductService) UpdateProduct(ctx context.Context, p *Product) error {
    if err := s.db.UpdateProduct(ctx, p); err != nil {
        return err
    }
    s.redis.Del(ctx, "product:"+p.ID)
    return nil
}
```

**Инвалидация — главная сложность**:
- `TTL-based`: при изменении ждём, пока TTL истечёт. Просто, но данные временно устаревшие.
- `Delete on write`: при обновлении сразу удаляем ключ. Следующий запрос промахнётся и обновит кэш.
- `Write-through`: при обновлении одновременно пишем в БД и в кэш. Сложнее, но кэш всегда свежий.

**Thundering herd**: если кэш истёк и одновременно пришло 100 запросов — все 100 пойдут в БД. Решение: probabilistic early expiration или distributed lock на время прогрева кэша.

```go
// защита от thundering herd через lock
func (s *ProductService) getWithLock(ctx context.Context, id string) (*Product, error) {
    lockKey := "lock:product:" + id
    ok, _ := s.redis.SetNX(ctx, lockKey, "1", 3*time.Second).Result()
    if !ok {
        // кто-то уже грузит — небольшой wait и повторная попытка из кэша
        time.Sleep(50 * time.Millisecond)
        return s.GetProduct(ctx, id)
    }
    defer s.redis.Del(ctx, lockKey)

    p, err := s.db.GetProduct(ctx, id)
    if err != nil {
        return nil, err
    }
    data, _ := json.Marshal(p)
    s.redis.Set(ctx, "product:"+id, data, 5*time.Minute)
    return p, nil
}
```

## Сценарий 2: Session storage

**Проблема**: HTTP-сервисы stateless, но сессия пользователя должна где-то жить. БД для каждого запроса — дорого. Если держать в памяти процесса — не работает при нескольких репликах.

**Redis как session store**: хранит сессию по session token, TTL автоматически инвалидирует истёкшие сессии.

```go
type Session struct {
    UserID    string    `json:"user_id"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}

func (s *SessionStore) Create(ctx context.Context, userID, email string) (string, error) {
    token := generateSecureToken() // crypto/rand
    session := Session{
        UserID:    userID,
        Email:     email,
        CreatedAt: time.Now(),
    }
    data, _ := json.Marshal(session)

    key := "session:" + token
    if err := s.redis.Set(ctx, key, data, 24*time.Hour).Err(); err != nil {
        return "", err
    }
    return token, nil
}

func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
    data, err := s.redis.Get(ctx, "session:"+token).Bytes()
    if errors.Is(err, redis.Nil) {
        return nil, ErrSessionNotFound
    }
    if err != nil {
        return nil, err
    }

    var session Session
    if err := json.Unmarshal(data, &session); err != nil {
        return nil, err
    }
    return &session, nil
}

func (s *SessionStore) Delete(ctx context.Context, token string) error {
    return s.redis.Del(ctx, "session:"+token).Err()
}

// продление TTL при каждом запросе (sliding expiration)
func (s *SessionStore) Refresh(ctx context.Context, token string) error {
    return s.redis.Expire(ctx, "session:"+token, 24*time.Hour).Err()
}
```

**Хранить в Hash** — если нужно обновлять отдельные поля без сериализации всего объекта:

```go
s.redis.HSet(ctx, "session:"+token,
    "user_id", userID,
    "last_seen", time.Now().Unix(),
)
s.redis.Expire(ctx, "session:"+token, 24*time.Hour)
```

## Сценарий 3: Idempotency keys

**Проблема**: клиент отправил запрос на списание денег, не получил ответа (таймаут), отправил снова. Без idempotency — двойное списание.

**Паттерн**: клиент генерирует уникальный `idempotency-key` и прикладывает к запросу. Сервер запоминает результат в Redis по этому ключу.

```go
func (s *PaymentService) Charge(ctx context.Context, req ChargeRequest) (*ChargeResult, error) {
    if req.IdempotencyKey == "" {
        return nil, ErrMissingIdempotencyKey
    }

    redisKey := "idempotency:" + req.IdempotencyKey

    // проверяем, не выполняли ли уже
    cached, err := s.redis.Get(ctx, redisKey).Bytes()
    if err == nil {
        var result ChargeResult
        json.Unmarshal(cached, &result)
        return &result, nil // возвращаем прошлый результат
    }

    // выполняем операцию
    result, err := s.processCharge(ctx, req)
    if err != nil {
        return nil, err
    }

    // сохраняем результат на 24 часа
    data, _ := json.Marshal(result)
    s.redis.Set(ctx, redisKey, data, 24*time.Hour)

    return result, nil
}
```

TTL idempotency key должен совпадать с тем, сколько клиент может повторять запрос. Обычно 24 часа.

## Сценарий 4: Distributed advisory lock

**Когда нужен**: несколько инстансов сервиса, и нужно гарантировать, что одна операция выполняется только одним инстансом одновременно. Например: cron-задача, которая должна запускаться строго одним воркером.

```go
type RedisLock struct {
    client *redis.Client
    key    string
    value  string // уникальный ID этого инстанса
    ttl    time.Duration
}

func NewLock(client *redis.Client, key string, ttl time.Duration) *RedisLock {
    return &RedisLock{
        client: client,
        key:    "lock:" + key,
        value:  uuid.New().String(),
        ttl:    ttl,
    }
}

func (l *RedisLock) Acquire(ctx context.Context) (bool, error) {
    ok, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
    return ok, err
}

// освобождение через Lua — атомарно проверить и удалить
var releaseScript = redis.NewScript(`
    if redis.call("get", KEYS[1]) == ARGV[1] then
        return redis.call("del", KEYS[1])
    else
        return 0
    end
`)

func (l *RedisLock) Release(ctx context.Context) error {
    _, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.value).Result()
    return err
}

// использование
func runDailyJob(ctx context.Context, lock *RedisLock) error {
    acquired, err := lock.Acquire(ctx)
    if err != nil {
        return err
    }
    if !acquired {
        return nil // другой инстанс уже запустил задачу
    }
    defer lock.Release(ctx)

    return doExpensiveWork(ctx)
}
```

**Важные оговорки**:
- TTL должен быть заведомо больше, чем время выполнения задачи.
- При GC pause или медленном syscall процесс может продолжить работу после истечения TTL — другой инстанс мог уже захватить lock.
- Для финансовых операций и других случаев, где важна строгая взаимная исключённость, нужны более надёжные механизмы (database-level lock, ZooKeeper, etcd).

## Сценарий 5: Leaderboard с Sorted Set

**Проблема**: real-time рейтинг игроков по очкам. `SELECT * FROM scores ORDER BY points DESC LIMIT 10` — при частых обновлениях это дорогой запрос к БД.

**Sorted Set**: каждый элемент имеет числовой score, структура поддерживает O(log N) вставку и O(log N + K) выборку топ-K.

```go
const leaderboardKey = "leaderboard:global"

// обновить очки игрока
func (s *GameService) AddScore(ctx context.Context, userID string, delta float64) error {
    return s.redis.ZIncrBy(ctx, leaderboardKey, delta, userID).Err()
}

// топ-10
func (s *GameService) GetTopPlayers(ctx context.Context) ([]PlayerScore, error) {
    result, err := s.redis.ZRevRangeWithScores(ctx, leaderboardKey, 0, 9).Result()
    if err != nil {
        return nil, err
    }

    players := make([]PlayerScore, len(result))
    for i, z := range result {
        players[i] = PlayerScore{UserID: z.Member.(string), Score: z.Score}
    }
    return players, nil
}

// позиция конкретного игрока (0-indexed, nil если не в рейтинге)
func (s *GameService) GetRank(ctx context.Context, userID string) (int64, error) {
    rank, err := s.redis.ZRevRank(ctx, leaderboardKey, userID).Result()
    if errors.Is(err, redis.Nil) {
        return -1, nil
    }
    return rank + 1, err // +1 для 1-indexed
}
```

**Сегментированные leaderboard**: отдельный ключ на период или регион — `leaderboard:2026-04`, `leaderboard:region:eu`.

## Сценарий 6: Pub/Sub для real-time уведомлений

**Когда подходит**: fan-out событий внутри одного сервиса или между сервисами, когда допустима потеря сообщений (Redis Pub/Sub не persistent — если подписчик отключился, сообщения теряются).

```go
// publisher
func (s *NotificationService) Publish(ctx context.Context, userID string, msg Notification) error {
    data, _ := json.Marshal(msg)
    return s.redis.Publish(ctx, "notifications:"+userID, data).Err()
}

// subscriber (WebSocket handler)
func (h *WSHandler) Subscribe(ctx context.Context, userID string, conn *websocket.Conn) {
    sub := h.redis.Subscribe(ctx, "notifications:"+userID)
    defer sub.Close()

    ch := sub.Channel()
    for {
        select {
        case msg, ok := <-ch:
            if !ok {
                return
            }
            conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
        case <-ctx.Done():
            return
        }
    }
}
```

**Ограничение**: при отключении подписчика сообщения теряются. Для надёжной доставки используй Redis Streams или Kafka.

**Redis Streams** — персистентная альтернатива Pub/Sub с группами потребителей и acknowledgment:

```go
// publish в stream
s.redis.XAdd(ctx, &redis.XAddArgs{
    Stream: "notifications",
    Values: map[string]interface{}{
        "user_id": userID,
        "payload": string(data),
    },
})

// consume с группой
s.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    "notification-workers",
    Consumer: workerID,
    Streams:  []string{"notifications", ">"},
    Count:    10,
})
```

## Сценарий 7: Job queue

**Простая очередь через List**: LPUSH для отправки, BRPOP для получения (блокирующий pop).

```go
const queueKey = "queue:email-jobs"

// producer
func (p *JobProducer) Enqueue(ctx context.Context, job EmailJob) error {
    data, _ := json.Marshal(job)
    return p.redis.LPush(ctx, queueKey, data).Err()
}

// consumer
func (w *Worker) Run(ctx context.Context) {
    for {
        result, err := w.redis.BRPop(ctx, 5*time.Second, queueKey).Result()
        if errors.Is(err, redis.Nil) {
            continue // таймаут, нет задач
        }
        if err != nil {
            if ctx.Err() != nil {
                return
            }
            time.Sleep(time.Second)
            continue
        }

        var job EmailJob
        json.Unmarshal([]byte(result[1]), &job)
        w.process(ctx, job)
    }
}
```

**Ограничение List-queue**: при краше воркера между BRPOP и обработкой задача теряется. Для надёжности — Redis Streams с acknowledgment, или полноценная очередь (Asynq, BullMQ, Sidekiq).

## Сценарий 8: Счётчики и аналитика

**Уникальные посетители с HyperLogLog**: оценка мощности множества с ~1% погрешностью при фиксированном потреблении памяти (~12 KB независимо от числа элементов).

```go
// добавить посетителя
s.redis.PFAdd(ctx, "visitors:"+date, userID)

// приблизительное число уникальных
count, _ := s.redis.PFCount(ctx, "visitors:2026-04-20").Result()

// объединить за неделю
s.redis.PFMerge(ctx, "visitors:week:2026-17",
    "visitors:2026-04-14",
    "visitors:2026-04-15",
    // ...
)
```

**Счётчики просмотров** с pipeline для батчинга:

```go
func (s *AnalyticsService) RecordViews(ctx context.Context, pageIDs []string) error {
    pipe := s.redis.Pipeline()
    for _, id := range pageIDs {
        pipe.Incr(ctx, "views:page:"+id)
    }
    _, err := pipe.Exec(ctx)
    return err
}
```

**Sliding window метрика с Sorted Set** (точная, но дороже HyperLogLog):

```go
// запомнить событие с timestamp как score
now := time.Now().UnixMilli()
s.redis.ZAdd(ctx, "events:"+eventType, redis.Z{Score: float64(now), Member: requestID})

// удалить старые события
windowStart := float64(time.Now().Add(-time.Hour).UnixMilli())
s.redis.ZRemRangeByScore(ctx, "events:"+eventType, "0", fmt.Sprintf("%f", windowStart))

// количество за последний час
count, _ := s.redis.ZCard(ctx, "events:"+eventType).Result()
```

## Антипаттерны

**Redis как primary database** — при рестарте без persistence данные теряются. Пользователи, заказы, платежи должны жить в PostgreSQL.

**KEYS * в production** — блокирует event loop. Вместо этого используй `SCAN` с cursor:

```go
var cursor uint64
for {
    keys, nextCursor, err := s.redis.Scan(ctx, cursor, "session:*", 100).Result()
    // обработка keys
    cursor = nextCursor
    if cursor == 0 {
        break
    }
}
```

**Хранить большие объекты** — Redis однопоточен для команд. GET/SET 1 MB блока блокирует других клиентов на время сериализации. Максимальный разумный размер value — единицы килобайт.

**Не ставить TTL** — memory растёт без ограничений. Каждый ключ должен иметь TTL или явное удаление.

## Interview-ready answer

Redis уместен в нескольких production-сценариях. Cache-aside: кэшируем дорогие запросы к БД с TTL, инвалидируем при обновлении — основная сложность в thundering herd и инвалидации. Session storage: session token → JSON с TTL, sliding expiration через Expire при каждом запросе. Idempotency keys: сохраняем результат операции по ключу клиента на 24 часа — защита от дублирования при retry. Distributed lock: SetNX + Lua release script — для advisory lock (cron-задачи), но не для финансовых инвариантов. Sorted Set для leaderboard: O(log N) обновление, O(log N + K) выборка топ-K. Pub/Sub для fan-out уведомлений — без гарантий доставки; для надёжности нужны Redis Streams. Во всех сценариях Redis — дополнительный слой поверх primary storage, не замена ему.
