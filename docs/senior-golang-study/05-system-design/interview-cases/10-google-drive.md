# Google Drive / File Storage System

Разбор задачи "Спроектируй Google Drive (Dropbox, iCloud)". Проверяет понимание chunked upload, file deduplication, sync protocol, conflict resolution и версионирования.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Вопросы:
  - Upload/download файлов или также редактирование (Google Docs)?
  - Нужна ли sync между устройствами (desktop client)?
  - Sharing: share link или share с конкретным пользователем?
  - Версионирование (восстановить старую версию)?
  - Совместное редактирование (Google Docs-style conflicts)?
  - Offline работа (изменить файл без сети → sync при подключении)?
```

**Договорились (scope):**
- Upload / Download файлов и папок
- Sync: изменение на одном устройстве → появляется на других (< 30 сек)
- Sharing: создать share link (read-only) и share с пользователем (read/write)
- File versioning: хранить последние 30 версий, восстановление
- Конфликты при одновременном редактировании: "conflict copy" (Dropbox-style)

**Out of scope:** collaborative real-time editing (Google Docs), full-text search внутри файлов, streaming медиа.

### Нефункциональные требования

```
- DAU: 50M пользователей
- Storage per user: 15 GB бесплатно (Google Drive реально)
- File size: до 5 GB на файл
- Sync latency: < 30 сек от изменения до появления на другом устройстве
- Availability: 99.99%
- Durability: 99.999999999% (11 nines — данные нельзя терять никогда)
- Bandwidth optimization: не загружать весь файл при маленьком изменении
```

---

## Фаза 2: Оценка нагрузки

```
Storage:
  50M users × 15 GB = 750 PB total storage
  Реально используется ~20% ёмкости = 150 PB активных данных

Uploads:
  50M × 1 upload/day × avg 5 MB = 250 TB/day upload
  250 TB / 86400 ≈ 3 GB/sec ingress

Downloads:
  50M × 3 downloads/day × avg 5 MB = 750 TB/day
  ≈ 9 GB/sec egress

Metadata operations (list files, check for updates):
  Sync clients проверяют наличие изменений
  50M × 10 checks/day = 500M/day ≈ 6K metadata ops/sec
  Peak: 5x = 30K ops/sec

Deduplication opportunity:
  Типичные файлы (photo, document) часто дублируются между пользователями
  Одинаковый README.md у 1M разработчиков → хранить один раз
  Экономия: ~30-40% storage
```

---

## Фаза 3: Высокоуровневый дизайн

```
  Desktop Client              Mobile Client             Web Client
    (sync daemon)               (app)                    (browser)
         │                        │                          │
         └────────────────────────┴──────────────────────────┘
                                  │
                         ┌────────▼────────┐
                         │   API Gateway   │
                         └────────┬────────┘
                                  │
          ┌───────────────────────┼──────────────────────────┐
          │                       │                          │
   ┌──────▼──────┐        ┌───────▼──────┐         ┌────────▼──────┐
   │  Upload     │        │  Metadata    │         │   Sync        │
   │  Service    │        │  Service     │         │   Service     │
   └──────┬──────┘        └───────┬──────┘         └────────┬──────┘
          │                       │                          │
          ▼                       ▼                          ▼
   ┌─────────────┐       ┌────────────────┐        ┌────────────────┐
   │  Block      │       │  Metadata DB   │        │  Notification  │
   │  Storage    │       │  (PostgreSQL)  │        │  Queue (Kafka) │
   │  (S3)       │       │  + Redis Cache │        └────────────────┘
   └─────────────┘       └────────────────┘
```

---

## Фаза 4: Deep Dive

### Chunked Upload: ключевая идея

**Проблема:** файл 1 GB. Загружать целиком — надёжно, но:
1. При обрыве сети → начинать заново
2. Изменили 1 строку в конце файла → снова загружать 1 GB
3. Один и тот же файл у 100 пользователей → хранить 100 копий

**Решение: Content-Addressed Storage + Chunking**

```
Алгоритм:
  1. Клиент разбивает файл на chunks (фиксированный или variable size)
  2. Вычислить SHA-256 каждого chunk → chunk_hash
  3. Перед загрузкой: спросить сервер "какие chunk_hash уже есть?"
  4. Загрузить только недостающие chunks
  5. Сообщить серверу: "файл = [chunk_hash_1, chunk_hash_2, ...]"

Хранение:
  Block Store: chunk_hash → binary data
  Metadata: file → ordered list of chunk_hashes

При изменении файла:
  Только изменённые chunks → новые hash → загрузить только их
  Неизменённые chunks → уже есть на сервере → не загружать
```

**Deduplication:**
```
Иванов загружает photo.jpg (SHA-256 = "abc123...")
Петров загружает ту же photo.jpg

Сервер: chunk с hash "abc123..." уже есть
  → Не хранить второй раз
  → Metadata Петрова просто указывает на те же chunks

Экономия: до 30-40% storage при типичном контенте
Безопасность: cross-user dedup → только по chunk hash (content-defined)
  При шифровании: user шифрует данные → hash уникален → нет cross-user dedup
  (сознательный trade-off: privacy vs storage)
```

---

### Variable-Size Chunking (Rabin Fingerprint)

```
Фиксированный размер chunk (e.g., 4 MB) проблема:
  Файл: [AAAA | BBBB | CCCC | DDDD]
  Вставить 1 байт в начало: [XAAA | ABBB | BCCC | CDDD]
  Все chunks изменились! → загрузить весь файл заново

Rabin Fingerprint (Content-Defined Chunking):
  Скользящее окно по байтам файла
  Граница chunk = когда hash(window) % M == 0
  
  Результат: вставка байта сдвигает только ближайшие границы
  Большинство chunks (середина файла) остаются неизменными
  
  Параметры: avg chunk size 4MB (min 512KB, max 32MB)
  Trade-off: мелкие chunks → больше metadata, round trips
             крупные chunks → меньше dedup возможностей
```

---

### Sync Protocol

**Задача:** изменение на устройстве A → появилось на устройстве B.

```
Desktop Sync Client (daemon):
  1. File system watcher (inotify/FSEvents/ReadDirectoryChanges)
     → Событие: файл X изменился
  2. Compute chunk hashes нового состояния
  3. POST /sync/push:
     {
       "path": "/documents/report.docx",
       "version": 42,
       "chunks": ["hash1", "hash2", "hash3_new"],
       "device_id": "laptop-uuid"
     }
  4. Сервер: сравнить с last known version
  5. Ответ: "нужны chunks: hash3_new"
  6. Клиент: загрузить только hash3_new
  7. Сервер: обновить metadata, notify другие устройства

Notification другим устройствам:
  Kafka: topic=file.changed → device notification workers
  Long-polling или WebSocket к каждому online устройству:
    { "type": "file_changed", "path": "/documents/report.docx", "version": 43 }
  
  Устройство получает уведомление → pull новые chunks → применить
```

**Sync State Machine:**
```
Device_A изменяет файл:
  LOCAL_MODIFIED → UPLOADING → UPLOADED → SYNCED

Device_B получает уведомление:
  REMOTE_MODIFIED → DOWNLOADING → DOWNLOADED → SYNCED
```

---

### Conflict Resolution

**Сценарий:** Ноутбук и телефон оба offline, оба редактируют файл. Что при sync?

```
Dropbox подход (Conflict Copy):
  1. Оба устройства загружают изменения с одинаковым parent version
  2. Первый загружает → становится версия 43
  3. Второй загружает с parent=42 → конфликт!
  
  Conflict Copy:
    - Сохранить обе версии
    - Оригинальное имя: "report.docx" (победитель — первый)
    - Конфликтная копия: "report (Conflicted copy 2024-01-15, John's laptop).docx"
    - Оба файла синхронизируются на все устройства
    - Пользователь вручную мёржит если нужно

Google Drive подход:
  Для Google Docs: operational transformation → merge автоматически
  Для binary files (Word, Excel): то же что Dropbox — conflict copy
```

**Last-Write-Wins (альтернатива):**
```
Самое простое: последнее изменение побеждает (по timestamp)
Проблема: часы на устройствах не синхронизированы (clock skew)
  → Можно потерять более "правильное" изменение

Решение: использовать logical clock (Lamport timestamp) + server-side ordering
  Клиент отправляет: { parent_version: 42, changes: ... }
  Сервер: если parent_version актуален → принять, версия 43
           если parent_version устарел → conflict
```

---

### File Versioning

```sql
CREATE TABLE file_versions (
  file_id     UUID    NOT NULL,
  version     INT     NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by  BIGINT,  -- user or device
  chunk_list  TEXT[],  -- ordered array of chunk hashes
  size_bytes  BIGINT,
  is_deleted  BOOLEAN DEFAULT FALSE,
  PRIMARY KEY (file_id, version)
);

-- Текущая версия (head)
CREATE TABLE files (
  id          UUID    PRIMARY KEY,
  owner_id    BIGINT  NOT NULL,
  parent_id   UUID,   -- папка
  name        VARCHAR(255),
  current_version INT,
  mime_type   VARCHAR(100),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Retention policy:**
```
Бесплатный план: последние 30 версий
Платный план: неограниченная история (хранить в Glacier)
  
Cleanup job:
  DELETE FROM file_versions 
  WHERE file_id = ? 
    AND version < (SELECT MAX(version) - 30 FROM file_versions WHERE file_id = ?)
  
При удалении версии: только удалить metadata entry
  Chunks: не удалять сразу — другие файлы/версии могут ссылаться
  Garbage collector: chunk → ref count = 0 → scheduled delete через 30 дней
```

---

### Block Storage: S3 с Content-Addressing

```
Структура ключей в S3:
  blocks/{first_2_chars_of_hash}/{full_hash}
  
  Например:
  blocks/ab/abcdef1234567890...
  blocks/cd/cdef1234567890ab...

  Первые 2 символа → распределение по S3 prefix (помогает избежать hotspots)

Размер блока:
  4 MB средний → 1 GB файл = ~250 chunks
  250 chunks × 1 metadata entry ≈ нормально

Upload flow:
  1. PUT blocks/{hash} content-body
  2. S3: хранить с server-side encryption (SSE-S3 или SSE-KMS)
  3. S3 хранит автоматически в 3 AZ (99.999999999% durability нативно)

Download flow:
  1. GET metadata → список chunk hashes
  2. Parallel GET blocks/{hash} × N chunks
  3. Собрать в порядке → отдать файл
  4. CDN перед S3 для часто скачиваемых файлов
```

---

### Metadata Service и индексирование

```sql
-- Filesystem tree (Materialized path или Adjacency List)
CREATE TABLE fs_nodes (
  id          UUID    PRIMARY KEY,
  owner_id    BIGINT  NOT NULL,
  parent_id   UUID    REFERENCES fs_nodes(id),
  name        VARCHAR(255) NOT NULL,
  type        VARCHAR(10) NOT NULL,  -- 'file' или 'dir'
  path        TEXT,   -- /documents/work/report.docx (materialized path)
  
  -- only for files
  current_version_id BIGINT,
  size_bytes  BIGINT,
  mime_type   VARCHAR(100),
  
  created_at  TIMESTAMPTZ,
  updated_at  TIMESTAMPTZ,
  
  UNIQUE (parent_id, name, owner_id)  -- уникальность имени в папке
);

CREATE INDEX idx_fs_path ON fs_nodes USING GIN(path gin_trgm_ops);  -- для search
CREATE INDEX idx_fs_owner_parent ON fs_nodes(owner_id, parent_id);
```

**Caching:**
```
Часто читаемое: структура папок, список файлов
  Redis: HGETALL dir:{user_id}:{dir_id} → список детей
  TTL: 5 минут, инвалидировать при изменении

Chunk existence check (нужно ли загружать chunk?):
  Bloom Filter в памяти Upload Service: "hash X уже есть?"
  False positive: иногда скажет "есть" когда нет → лишняя проверка в S3
  False negative: невозможны → надёжная дедупликация
  Размер: 150 PB / 4MB avg chunk = ~37.5B chunks × 10 bits ≈ 47 GB (слишком много)
  
  Реальный подход: Redis SISMEMBER для chunk hash lookup
  Partitioned по first byte of hash → распределить нагрузку
```

---

### Sharing

```sql
CREATE TABLE shares (
  id          UUID    PRIMARY KEY,
  node_id     UUID    NOT NULL REFERENCES fs_nodes(id),
  owner_id    BIGINT  NOT NULL,
  share_type  VARCHAR(20) NOT NULL,  -- 'link' или 'user'
  recipient_id BIGINT,               -- NULL для link-shares
  permission  VARCHAR(10) NOT NULL,  -- 'read' или 'write'
  token       VARCHAR(64) UNIQUE,    -- для link-shares
  expires_at  TIMESTAMPTZ,
  created_at  TIMESTAMPTZ
);
```

**Share link доступ:**
```
GET /s/{token} → resolve share → redirect к файлу
  Проверить: token валидный? expires_at не прошёл?
  Rate limit: 100 downloads/час с одного IP для public links
```

---

## Трейдоффы

| Компонент | Выбор | Альтернатива | Причина |
|---|---|---|---|
| Chunking | Variable (Rabin) | Fixed-size | Лучший dedup при вставках/удалениях |
| Dedup | Cross-user (hash) | Per-user only | 30-40% storage savings |
| Conflict | Conflict copy | Last-write-wins | Нет потери данных |
| Storage | S3 content-addressed | Dedicated FS | Durability 11 nines out-of-box |
| Sync | Push (server notify) | Client polling | Latency: 30 сек vs poll interval |
| Versions | Last 30 (free) | Unlimited | Cost control |

---

## Interview-ready ответ (2 минуты)

> "Google Drive — это задача на chunking, deduplication и sync протокол.
>
> Ключевая идея: Content-Addressed Storage. Файл = список SHA-256 хешей chunk'ов. При загрузке спрашиваем сервер 'какие chunks уже есть' → загружаем только новые. При изменении 1 байта в конце 1GB файла → загружается один chunk, не весь файл. Cross-user deduplication: одинаковые chunks хранятся один раз.
>
> Variable-size chunking через Rabin Fingerprint — границы определяются содержимым файла, поэтому вставка байта не инвалидирует все последующие chunks.
>
> Sync protocol: file watcher → вычислить diff → POST только изменённые chunks → Kafka → WebSocket/long-polling уведомление другим устройствам → pull.
>
> Конфликты: conflict copy (Dropbox-style). При concurrent edit с разных устройств — сохранить обе версии, дать пользователю разрулить вручную.
>
> Хранение: S3 с content-addressed ключами (hash prefix для распределения). Metadata в PostgreSQL. 11 nines durability из коробки от S3."
