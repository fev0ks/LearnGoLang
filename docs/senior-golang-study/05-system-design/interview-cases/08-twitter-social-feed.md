# Twitter / Social Feed

Разбор задачи "Спроектируй Twitter". Ключевой challenge — news feed generation: fan-out on write vs fan-out on read, проблема celebrity (аккаунты с 100M+ подписчиков). Проверяет понимание компромиссов между latency и consistency.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Вопросы:
  - Что именно: tweet + feed, или полный Twitter с DM, trending, search?
  - Ленту персонализировать (алгоритм) или chronological?
  - Retweet, reply, like — в scope?
  - Уведомления о новых твитах от подписок?
  - Поиск по тексту?
```

**Договорились (scope):**
- Написать твит (текст, до 280 символов)
- Читать home feed (твиты от тех на кого подписан)
- Follow / Unfollow
- Like, Retweet
- Счётчики: likes, retweets, replies
- Базовый поиск по хэштегам

**Out of scope:** DM, trending topics, ads, Spaces, персонализированный алгоритм ранжирования, notifications (есть в кейсе 02).

### Нефункциональные требования

```
- DAU: 200M пользователей
- Timeline read: 200M × 10 reads/day = 2B reads/day ≈ 23K reads/sec
- Tweet write: 200M × 5 tweets/day = 1B tweets/day ≈ 12K writes/sec
- Follow graph: ~300 подписок на среднего пользователя
  Celebrity (Elon Musk, Obama) → 100M+ подписчиков
- Feed freshness: новые твиты должны появляться < 10 сек
- Read latency: feed load < 200ms p99
- Availability: 99.99%
- Storage: твиты хранить вечно
```

---

## Фаза 2: Оценка нагрузки

```
Fan-out при публикации твита:
  Обычный пользователь: 300 подписчиков → 300 feed insertions
  Celebrity: 100M подписчиков → 100M feed insertions на 1 твит!
  
  12K tweets/sec × 300 avg followers = 3.6M feed insertions/sec
  (без учёта celebrity — с ними пики намного выше)

Storage (tweets):
  1 tweet: ~300 bytes (text + metadata)
  1B tweets/day × 365 × 5 лет = 1.825T tweets
  1.825T × 300B ≈ 550 TB → распределённое хранилище

Feed storage (если precomputed):
  Если хранить по 1000 последних твитов для 200M users:
  200M × 1000 × 8 bytes (tweet_id) = 1.6 TB в Redis
  → Это управляемо
```

---

## Фаза 3: Ключевое решение — Fan-out Strategy

Это центральный architectural decision для Twitter.

### Вариант 1: Fan-out on Write (Push model)

```
При публикации твита:
  1. Сохранить твит в Tweets DB
  2. Найти всех подписчиков
  3. Добавить tweet_id в feed каждого подписчика (Redis List)
  
При чтении ленты:
  LRANGE feed:{user_id} 0 99  → мгновенно (список уже готов)
  + fetch tweet content по IDs

Плюсы:
  + Чтение O(1) — просто взять из Redis
  + Низкая latency чтения
  
Минусы:
  - Запись медленная: 1 tweet × 300 followers = 300 writes
  - Celebrity problem: 100M writes при одном твите Илона Маска
  - Memory: хранить precomputed feeds для 200M users
```

### Вариант 2: Fan-out on Read (Pull model)

```
При публикации твита:
  1. Сохранить твит в Tweets DB
  Всё! 

При чтении ленты:
  1. Получить список подписок user → [followed_1, followed_2, ..., followed_300]
  2. Для каждого: SELECT tweet_id FROM tweets WHERE user_id = X ORDER BY created_at DESC LIMIT N
  3. Merge sort (N×300 записей) → топ 100
  4. Fetch tweet content

Плюсы:
  + Запись простая, нет fan-out
  + Нет Memory hotspot для celebrity
  
Минусы:
  - Чтение: N запросов (300 подписок × 1 запрос) = 300 SELECT
  - Latency при 23K reads/sec × 300 queries = 7M queries/sec → нереалистично без кеша
  - Для 300 подписок — ещё приемлемо; для 5000 подписок — нет
```

### Hybrid approach (реальное решение Twitter)

```
Проблема: Fan-out on Write не работает для celebrity
Проблема: Fan-out on Read не работает при большом количестве подписок

Решение: комбинация

Fan-out on Write ДЛЯ:
  - Обычных пользователей (< N followers, например < 1M)
  - Их твиты сразу расталкиваются в feeds подписчиков

Fan-out on Read ДЛЯ:
  - Celebrity-аккаунтов (> 1M followers)
  - При загрузке ленты: fetch recent tweets от celebrity + merge с precomputed feed

При загрузке ленты пользователя:
  1. LRANGE feed:{user_id} 0 99  (precomputed feed, без celebrity твитов)
  2. Определить какие из подписок — celebrity
  3. Для каждой celebrity: GET recent_tweets:{celebrity_id} (кешированы отдельно)
  4. Merge sort: precomputed + celebrity tweets
  5. Вернуть топ 100
```

---

## Фаза 4: Deep Dive

### Архитектура

```
  User
   │
   ├── POST /tweets ──────────────────────────────────►
   │                                                   │
   └── GET  /timeline ─────────────────────────────►   │
                                                   │   │
                                            ┌──────┴───┴────┐
                                            │  API Gateway  │
                                            └───┬───────────┘
                                                │
                    ┌───────────────────────────┼──────────────────────┐
                    │                           │                      │
             ┌──────▼──────┐            ┌──────▼──────┐       ┌──────▼──────┐
             │  Tweet      │            │  Timeline   │       │   User/     │
             │  Service    │            │  Service    │       │   Follow    │
             └──────┬──────┘            └──────┬──────┘       │   Service   │
                    │                          │              └──────┬──────┘
                    │                          │                     │
             ┌──────▼──────┐          ┌────────▼────────┐    ┌──────▼──────┐
             │  Tweets DB  │          │  Feed Cache     │    │  Follow DB  │
             │(Cassandra)  │◄─────────│  (Redis)        │    │(PostgreSQL) │
             └─────────────┘          └─────────────────┘    └─────────────┘
                    │
             ┌──────▼──────┐
             │  Fan-out    │
             │  Workers    │◄── Kafka: tweet.published
             └─────────────┘
```

---

### Tweet Service и хранилище

**Почему Cassandra?**

```
Требования:
  - 1B tweets/day writes
  - Читать по user_id + time range
  - Огромный объём, горизонтальное масштабирование

Cassandra схема:
  CREATE TABLE tweets (
    user_id     BIGINT,
    tweet_id    BIGINT,      -- Snowflake ID (time-ordered)
    content     TEXT,
    like_count  COUNTER,     -- Cassandra COUNTER тип
    reply_count COUNTER,
    retweet_count COUNTER,
    created_at  TIMESTAMP,
    PRIMARY KEY (user_id, tweet_id)
  ) WITH CLUSTERING ORDER BY (tweet_id DESC);

  Partition key = user_id → все твиты одного пользователя вместе
  Clustering key = tweet_id DESC → новые первыми

  Запрос последних твитов:
    SELECT * FROM tweets WHERE user_id = ? LIMIT 20;
    → O(1) по partition, O(log N) по clustering key
```

**Tweet ID — Snowflake:**
```
64-bit ID:
  41 бит: timestamp ms (69 лет с 2010)
   5 бит: datacenter
   5 бит: machine
  12 бит: sequence

Свойство: сортируется по времени без ORDER BY поля
  Только WHERE tweet_id > {cursor_id} для пагинации
  → Эффективная cursor-based pagination
```

---

### Fan-out Workers

```
При публикации:
  Tweet Service → Kafka: topic=tweet.published
  Key=user_id (партиционирование по автору)

Fan-out Worker (консьюмер):
  1. Получить tweet
  2. Проверить: user имеет > 1M followers? → celebrity flag, не fan-out
  3. Обычный user: получить список подписчиков из Follow DB (или кеш)
  4. Для каждого подписчика:
     LPUSH feed:{follower_id} {tweet_id}
     LTRIM feed:{follower_id} 0 999  // хранить только 1000 последних

Параллельность:
  Для пользователя с 100K followers — разбить на батчи
  Каждый батч → Redis PIPELINE (батч команд за 1 round trip)
  
  100K LPUSH / 1ms RTT = ~100 сек линейно
  Батчи по 1000 + pipeline: ~100 round trips = ~100ms

Горизонтальное масштабирование:
  Несколько consumer groups в Kafka
  Partition per worker → параллельная обработка разных авторов
```

---

### Timeline Service: чтение ленты

```
GET /timeline?user_id=123&cursor=&limit=20

1. Получить precomputed feed:
   tweet_ids = LRANGE feed:{user_id} 0 99  (100 кандидатов)

2. Определить celebrity подписки пользователя:
   celebrity_follows = SMEMBERS celeb_follows:{user_id}  
   // хранить отдельно при follow celebrity

3. Fetch recent celebrity tweets:
   для каждого celebrity:
     LRANGE user_tweets:{celebrity_id} 0 19  // последние 20, кешированы отдельно

4. Merge sort по tweet_id (time-ordered Snowflake):
   merge(precomputed_ids, celebrity_tweet_ids)
   → топ 20

5. Bulk fetch tweet content:
   MGET tweet:{id1} tweet:{id2} ... tweet:{id20}
   (твиты кешированы в Redis после первого чтения)

6. Вернуть список твитов

Total Redis calls: 1 LRANGE + N LRANGE celebrity + 1 MGET
  → 3-5 round trips → ~5ms
```

**Cache warming:**
```
При первом входе пользователя после долгого отсутствия:
  feed:{user_id} пустой
  → Cold start: выполнить fan-out on read, наполнить кеш
  → Async, показать пользователю сначала загрузку
```

---

### Follow Graph

```sql
-- Простая модель
CREATE TABLE follows (
  follower_id  BIGINT NOT NULL,
  followee_id  BIGINT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (follower_id, followee_id)
);

CREATE INDEX idx_follows_followee ON follows(followee_id, created_at DESC);
-- Для получения всех подписчиков (fan-out): O(1) по followee_id
```

**Graph кеш в Redis:**
```
followers:{user_id} → Redis Set (для быстрого fan-out)
  Обновлять при follow/unfollow

celebrity_follows:{user_id} → Redis Set (только celebrity подписки пользователя)
  Обновлять при follow celebrity

Проблема больших Sets:
  100M followers Маска → Redis Set 100M × 8 bytes = 800MB на одну запись
  Решение: для celebrity fan-out использовать PostgreSQL batch query
  + кешировать подписчиков батчами (offset-based)
```

---

### Likes и Counters

**Проблема:** горячий твит (вирусный) получает 100K+ лайков за минуту.

```
Наивный: UPDATE tweets SET like_count += 1 → write hotspot

Решение: Redis INCR + async flush
  INCR like_count:{tweet_id}
  SADD liked_by:{tweet_id} {user_id}  // для проверки "лайкнул ли ты"
  
  Batch flush каждые 30 сек:
    Читать all like_count из Redis
    UPDATE tweets SET like_count = ? WHERE tweet_id = ?
    
  Проверка "лайкнул ли ты":
    SISMEMBER liked_by:{tweet_id} {user_id}  // O(1)

Альтернатива — Cassandra COUNTER:
  UPDATE tweet_counts SET like_count = like_count + 1 WHERE tweet_id = ?
  Cassandra нативно поддерживает distributed counters → нет hotspot
```

---

### Search по хэштегам

```
Elasticsearch индекс:
  {
    tweet_id: "1234...",
    content: "Привет #golang #go #programming",
    hashtags: ["golang", "go", "programming"],
    author_id: 456,
    created_at: "2024-01-15T10:00:00Z",
    like_count: 42
  }

Запрос:
  GET /search?q=%23golang&sort=recent
  
  {
    "query": { "term": { "hashtags": "golang" }},
    "sort": [{ "created_at": "desc" }],
    "size": 20
  }

Trending hashtags:
  Kafka Consumer: подсчитывать hashtag frequency в sliding window
  Flink: топ-10 за последний час → Redis
  GET /trending → читать из Redis (обновляется раз в 5 мин)
```

---

## Трейдоффы

| Компонент | Выбор | Альтернатива | Причина |
|---|---|---|---|
| Fan-out | Hybrid (write + read) | Pure push / pure pull | Celebrity problem |
| Tweet storage | Cassandra | PostgreSQL | Write throughput, horizontal scale |
| Feed storage | Redis List | Cassandra | Sub-millisecond LRANGE |
| Tweet ID | Snowflake | UUID v4 | Time-ordered, sortable |
| Counters | Redis INCR + flush | Cassandra COUNTER | Flexibility, но потеря при Redis crash |
| Search | Elasticsearch | PostgreSQL FTS | Scale: 1T+ indexed tweets |

### Fan-out threshold: почему 1M?

```
При 1M followers и fan-out on write:
  1M LPUSH ≈ 1 сек (при batch + pipeline)
  
Twitter's actual threshold: ~10K-20K followers
  (публично не раскрыто, но логика — latency бюджет публикации)
  
Чем выше threshold → меньше read-time работы, но дольше publish
Чем ниже threshold → быстрый publish, больше read-time работы

Можно настраивать динамически: если fan-out queue растёт → снижать threshold
```

---

## Failure Scenarios

```
Fan-out worker упал:
  Kafka: at-least-once, сообщение остаётся в топике
  Worker перезапустится → продолжит с последнего offset
  Возможен duplicate fan-out → LPUSH идемпотентен (duplicate tweet_id в feed)
  Дедупликация: при чтении ленты — убирать дубли (уже есть в LRANGE)

Redis упал (feed cache):
  Fallback: fan-out on read для всех (медленнее, но работает)
  Rebuild: при восстановлении Redis → warm cache для активных пользователей
  Alert: немедленно, Redis — критичный компонент

Cassandra нода упала:
  Replication factor = 3, quorum reads/writes
  Потеря одной ноды → автоматически переключается на другие реплики
  Consistency level = QUORUM → пишем в 2 из 3, читаем из 2 из 3
```

---

## Interview-ready ответ (2 минуты)

> "Twitter — это задача fan-out. Ключевой вопрос: push или pull для home feed?
>
> Pure push: при 100M followers у celebrity — 100M Redis writes на один твит. Недопустимо.
> Pure pull: 300 подписок × SELECT = 300 запросов на каждое открытие ленты. Не масштабируется.
>
> Hybrid: fan-out on write для обычных пользователей (< порог, например 10K followers). Для celebrity — fan-out on read: их твиты кешируются отдельно, при загрузке ленты мержатся с precomputed feed.
>
> Storage: Cassandra, партиционированная по user_id. Snowflake IDs для time-ordering без ORDER BY. Feed в Redis Lists (LPUSH + LTRIM до 1000 записей).
>
> Likes: Redis INCR + async flush в Cassandra каждые 30 сек. SISMEMBER для 'лайкнул ли ты'.
>
> При чтении ленты: 1 Redis LRANGE + N LRANGE для celebrity + 1 MGET для content = 3-5 round trips ≈ 5ms. Укладываемся в 200ms p99."
