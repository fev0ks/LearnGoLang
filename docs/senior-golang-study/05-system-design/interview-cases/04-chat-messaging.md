# Chat / Messaging System

Разбор задачи "Спроектируй мессенджер (chat system)". Сложная задача, проверяет знание WebSocket/long-polling, fan-out в реальном времени, storage для истории и online-presence.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Кандидат: Уточняю scope — мессенджер большой.

Вопросы:
  - Личные чаты (1-on-1) или групповые тоже?
    → Оба, но группы до 500 человек
  - Реального времени (WebSocket) или asynchronous (email-like)?
    → Real-time, с доставкой "почти мгновенно"
  - Историю сообщений нужно хранить? Сколько?
    → Да, вся история, не удаляем
  - Статусы сообщений (sent/delivered/read)?
    → sent + delivered (read receipts — опционально, in scope если успеем)
  - Файлы/медиа?
    → Пока только текст; медиа — out of scope
  - Online presence ("был в сети 5 мин назад")?
    → Да, нужен
```

**Договорились (scope):**
- 1-on-1 и групповые чаты (до 500 участников)
- Real-time delivery (WebSocket)
- Полная история сообщений
- Статусы: sent, delivered, read
- Online presence
- Push notifications для offline пользователей

**Out of scope:** медиафайлы, голосовые/видео звонки, боты, реакции.

### Нефункциональные требования

```
- DAU: 50M пользователей
- Одновременно online: 10M (20% DAU)
- Latency: сообщение доставлено < 500ms при обоих online
- Message ordering: строгий порядок в рамках чата
- Durability: потеря сообщения недопустима
- Availability: 99.99% (мессенджер = критичный сервис)
- Consistency: eventual OK для статусов delivered/read
```

---

## Фаза 2: Оценка нагрузки

```
DAU = 50M
Среднее: 50 сообщений/день на пользователя
Total messages/day = 50M × 50 = 2.5B сообщений

Writes (новые сообщения):
  2.5B / 86400 ≈ 29000 msg/sec среднее
  Peak ≈ 3x = 90000 msg/sec

Одновременных WebSocket соединений: 10M
  → Это главный challenge: держать 10M persistent connections

Storage:
  1 сообщение: ~1KB (текст 500 chars + metadata)
  2.5B × 365 дней × 3 года хранения = 2.7T сообщений
  2.7T × 1KB = 2.7 PB — нужно распределённое хранилище (Cassandra/ScyllaDB)

Fan-out:
  Групповой чат 500 человек × 10 msg/min = 5000 deliveries/min на чат
  Если таких активных групп 10K → 50M deliveries/min ≈ 833K/sec
  → Нужен эффективный fan-out механизм
```

---

## Фаза 3: Высокоуровневый дизайн

```
                              ┌──────────────────────────────┐
                              │    Chat Service Cluster      │
                              │                              │
Mobile/Web    WebSocket       │  ┌────────┐  ┌────────┐     │
 Client A ────────────────────►  │Chat    │  │Chat    │     │
                              │  │Server 1│  │Server 2│ ... │
 Client B ────────────────────►  │(conn A)│  │(conn B)│     │
                              │  └───┬────┘  └───┬────┘     │
                              └──────┼────────────┼──────────┘
                                     │            │
                              ┌──────▼────────────▼──────────┐
                              │      Message Bus (Kafka)     │
                              └──────┬────────────────────────┘
                        ┌───────────┼────────────────┐
                        │           │                │
               ┌────────▼───┐ ┌─────▼──────┐ ┌──────▼──────┐
               │  Message   │ │  Presence  │ │  Push Notif │
               │  Store     │ │  Service   │ │  Service    │
               │(ScyllaDB)  │ │  (Redis)   │ │             │
               └────────────┘ └────────────┘ └─────────────┘
```

---

## Фаза 4: Deep Dive

### WebSocket управление соединениями

**Проблема:** 10M одновременных соединений.

```
Одна нода держит ~50K WebSocket соединений (Go: ~10KB на goroutine × 50K = 500MB RAM)
10M / 50K = 200 нод Chat Server

Mapping: user_id → chat_server_id хранится в Redis
  "ws_node:{user_id}" → "chat-server-42"

При подключении клиента:
  1. LB направляет на любой Chat Server
  2. Chat Server: SET ws_node:{user_id} chat-server-42 EX 300
  3. При разрыве: DEL ws_node:{user_id}
```

**Heartbeat:**
```
Клиент → сервер: ping каждые 30 сек
Сервер: при отсутствии ping 60 сек → считать offline, закрыть соединение
```

---

### Message Flow (отправка сообщения)

```
Отправитель (User A, подключён к Chat-Server-1):

1. A отправляет через WebSocket:
   { "type": "send_message", "chat_id": 42, "content": "Hello!", "client_msg_id": "abc-123" }

2. Chat-Server-1:
   a. Валидация (user A — участник chat 42?)
   b. Assign message_id (глобально уникальный, ordered)
   c. Сохранить в ScyllaDB:
      INSERT INTO messages (chat_id, message_id, sender_id, content, created_at)
   d. Publish в Kafka: topic=chat.messages, key=chat_id, value=message

3. Kafka → Fan-out Worker:
   Получить список участников chat 42
   Для каждого участника B:
     - Если B online → найти его Chat-Server через Redis → deliver via pub/sub
     - Если B offline → поставить в очередь push notification

4. Delivery confirmation:
   B получил → B отправляет ACK → статус = DELIVERED
   A получает WebSocket event: { "type": "status_update", "message_id": X, "status": "delivered" }
```

---

### Message ID: порядок сообщений

**Требование:** строгий порядок в рамках чата.

```
Проблема с UUID v4: случайные, нельзя сортировать по времени.
Проблема с timestamp: миллисекунды могут совпасть.

Решение: Snowflake-подобный ID

Структура (64 бит):
  41 бит: milliseconds since epoch (69 лет)
   5 бит: datacenter_id
   5 бит: machine_id
  12 бит: sequence (4096 msg/ms на одной ноде)

  → Монотонно возрастающий
  → Уникальный
  → Можно извлечь timestamp

Реализация: centralized ID generator service (или per-node sequence в пределах chat_id)
```

**Альтернатива:** использовать Cassandra UUID Type 1 (time-based) или ScyllaDB TIMEUUID — они уже time-ordered.

---

### Хранилище сообщений (ScyllaDB/Cassandra)

**Почему не PostgreSQL?**
- 2.7 PB данных → шардирование обязательно, Cassandra/ScyllaDB designed for this
- Write-heavy workload (29K msg/sec)
- Partitioning по chat_id даёт locality для пагинации истории

```sql
-- Cassandra CQL schema
CREATE TABLE messages (
  chat_id     UUID,
  message_id  TIMEUUID,      -- time-ordered, уникальный
  sender_id   BIGINT,
  content     TEXT,
  status      TINYINT,       -- 1=sent, 2=delivered, 3=read
  created_at  TIMESTAMP,
  PRIMARY KEY (chat_id, message_id)
) WITH CLUSTERING ORDER BY (message_id DESC)
  AND compaction = {'class': 'LeveledCompactionStrategy'}
  AND gc_grace_seconds = 864000;
```

**Чтение истории:**
```sql
-- Последние 50 сообщений
SELECT * FROM messages WHERE chat_id = ? ORDER BY message_id DESC LIMIT 50;

-- Пагинация (загрузить старее)
SELECT * FROM messages WHERE chat_id = ? AND message_id < ? ORDER BY message_id DESC LIMIT 50;
```

**Hot partition problem:**
- Активный чат с 500 участниками = много writes в одну partition
- Решение: partition key = (chat_id, bucket) где bucket = message_id / 1000 (сегментирование по времени)

---

### Fan-out для групповых чатов

**Для маленьких групп (< 100 человек): push at send time**
```
При отправке → сразу доставить всем N участникам
При 100 × 29K msg/sec = 2.9M deliveries/sec → нагрузка управляема
```

**Для больших групп (100-500 человек): pull + inbox**
```
Проблема: 500 участников × 90K peak msg/sec = 45M deliveries/sec → слишком много

Решение: inbox model
  1. Сообщение сохраняется в messages table
  2. В user_inbox_{user_id} пишется только pointer: { chat_id, message_id }
     (не полное сообщение)
  3. Клиент при подключении загружает inbox → fetches messages by chat_id/message_id
  4. WebSocket event для online пользователей: { "type": "new_message", "chat_id", "message_id" }
     → клиент сам делает fetch
```

---

### Online Presence

```
Хранение:
  Redis: HSET presence:{user_id} status "online" last_seen {timestamp}
  TTL: 60 сек (обновляется heartbeat)

  При heartbeat от клиента:
    HSET presence:{user_id} status "online" last_seen {now}
    EXPIRE presence:{user_id} 60

  При disconnect:
    DEL presence:{user_id}
    → или: HSET presence:{user_id} status "offline" last_seen {now} (для "был N мин назад")

Запрос presence:
  GET presence:{user_id} → { status: "online" } или null (offline)

Для групп (присутствие 500 человек):
  MGET presence:{u1} presence:{u2} ... presence:{u500}
  → пайплайн Redis, ~1-2ms

Масштабирование:
  10M online users × ~100 bytes = 1GB в Redis
  Легко, один Redis достаточен (+ replica)
```

---

### Push Notifications для offline пользователей

```
Fan-out Worker:
  Если recipient offline (нет в Redis presence):
    → отправить событие в Kafka: topic=notifications.push
    → Push Notification Service (из нашего notification design!)
       → FCM/APNs с payload:
          { "type": "new_message", "chat_id": X, "sender": "Alice", "preview": "Hello..." }
```

---

### Read Receipts

```
Client при прочтении чата:
  WebSocket: { "type": "mark_read", "chat_id": 42, "last_read_message_id": "..." }

Server:
  UPDATE read_receipts SET last_read_message_id = ? WHERE chat_id = ? AND user_id = ?
  Notify sender через WebSocket: { "type": "read_receipt", "chat_id": 42, "user_id": B, "up_to": "..." }

Schema:
  CREATE TABLE read_receipts (
    chat_id         UUID,
    user_id         BIGINT,
    last_read_id    TIMEUUID,
    updated_at      TIMESTAMP,
    PRIMARY KEY (chat_id, user_id)
  );
```

---

### Reconnect и missed messages

```
При reconnect клиент отправляет:
  { "type": "sync", "last_seen_message_id": "XYZ" }

Server:
  Для каждого чата пользователя:
    SELECT * FROM messages WHERE chat_id = ? AND message_id > 'XYZ' LIMIT 100
  → Вернуть пропущенные сообщения
```

---

## Трейдоффы

| Решение | Принятое | Альтернатива | Причина |
|---|---|---|---|
| Протокол | WebSocket | Long-polling, SSE | Bidirectional, persistent |
| Хранилище | ScyllaDB | PostgreSQL + sharding | Built-in partitioning, write throughput |
| Message ID | Snowflake | UUID v4 | Time-ordered, sortable |
| Fan-out большие группы | Pull (inbox pointer) | Push всем | Контроль write amplification |
| Presence | Redis TTL | DB с polling | Latency < 1ms, volatility OK |

---

## Что если Chat Server падает?

```
10M соединений на 200 нод → ~50K соединений на ноду.
Нода падает:
  1. 50K клиентов теряют соединение
  2. Клиенты начинают reconnect (exponential backoff: 1s, 2s, 4s, ...)
  3. LB направляет на другие ноды
  4. При reconnect: sync пропущенных сообщений через Kafka/ScyllaDB

Важно: никаких in-memory state для соединений — только routing в Redis.
  Все сообщения персистентно в ScyllaDB/Kafka.
  → Падение ноды = потеря in-flight соединений, не данных.
```

---

## Interview-ready ответ (2 минуты)

> "Мессенджер — это два главных challenge: 10M persistent WebSocket соединений и эффективный fan-out для групп.
>
> WebSocket: 200+ нод, каждая держит ~50K соединений. Routing user→node через Redis — любая нода знает, на каком сервере подключён пользователь. Heartbeat каждые 30 секунд, падение ноды → клиенты переподключаются + sync пропущенных сообщений.
>
> Storage: ScyllaDB, partitioned по chat_id, clustered по time-ordered message_id (Snowflake). Write throughput 30K msg/sec, объём ~2.7 PB за 3 года — Cassandra-совместимые БД для этого и созданы.
>
> Fan-out: для малых групп (< 100) — push всем при отправке. Для больших — inbox model: храним только pointer, клиент сам тянет сообщение при получении события.
>
> Presence через Redis с TTL — не база данных. Offline → push notification через тот же Notification Service.
>
> Ordering: Snowflake ID, монотонно возрастающий в рамках ноды, сортируется без дополнительного поля."
