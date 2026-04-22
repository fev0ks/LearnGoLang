# URL Shortener

Разбор задачи "Спроектируй URL Shortener (сервис сокращения ссылок)". Один из самых частых примеров на system design — чистый, хорошо ограниченный объём работы.

---

## Фаза 1: Уточнение требований

### Функциональные требования

```
Кандидат: Давайте уточню scope. Основные сценарии:
  1. Пользователь передаёт длинный URL → получает короткий (например, https://svc.io/abc123)
  2. Пользователь переходит по короткому URL → редирект на оригинальный

Вопросы:
  - Нужна ли кастомизация alias? ("https://svc.io/my-promo" вместо рандомного)
  - Нужен ли срок жизни ссылки (TTL/expiration)?
  - Нужна ли аналитика (click count, geo, device)?
  - Авторизация нужна для создания ссылок?
```

**Договорились (MVP scope):**
- Создать короткую ссылку (random alias, без кастомизации)
- Редирект по короткой ссылке
- TTL: ссылки живут 5 лет
- Аналитика: только click count (опционально, не blocking)
- Auth: публичный API для создания (rate limiting вместо авторизации)

**Out of scope:** кастомные alias, аналитика по geo/device, bulk creation, QR коды.

### Нефункциональные требования

```
- DAU: 100M пользователей
- Read/Write ratio: 100:1 (redirect >> create)
- Latency: redirect < 10ms p99 (критично — пользователь ждёт)
- Availability: 99.9% (до 8.7 часов downtime в год)
- Durability: потеря ссылки недопустима
- Consistency: eventual OK для click count; strong для redirect (ссылка должна работать сразу)
```

---

## Фаза 2: Оценка нагрузки

```
DAU = 100M
Creates:
  100M users × 0.1 create/day = 10M creates/day
  10M / 86400 ≈ 115 creates/sec
  Peak ≈ 350 creates/sec

Redirects (100:1 ratio):
  10M × 100 = 1B redirects/day
  1B / 86400 ≈ 11500 RPS среднее
  Peak ≈ 35000 RPS

Storage:
  Одна запись: short_code (8B) + long_url (200B avg) + created_at + ttl + user_id ≈ ~250B
  10M создаётся в день × 365 × 5 лет = 18.25B записей
  18.25B × 250B ≈ 4.5 TB за 5 лет
  → Реляционная база справится, но нужно думать об индексах
```

**Выводы:**
- 35K RPS на redirect — один сервер не справится, нужно горизонтальное масштабирование
- Read-heavy (100:1) — **кеш критичен**
- 4.5 TB — умеренный объём, шардирование необязательно в первые годы
- Short code generation должна быть быстрой и без коллизий

---

## Фаза 3: Высокоуровневый дизайн

```
                    ┌─────────────┐
                    │   Client    │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  API Gateway │  ← rate limiting, TLS termination
                    └──────┬──────┘
               ┌───────────┴───────────┐
               │                       │
        ┌──────▼──────┐         ┌──────▼──────┐
        │  Write API  │         │  Read API   │
        │  (create)   │         │  (redirect) │
        └──────┬──────┘         └──────┬──────┘
               │                       │
               │                ┌──────▼──────┐
               │                │    Cache    │  ← Redis
               │                │ (hot URLs)  │
               │                └──────┬──────┘
               │                       │ miss
        ┌──────▼───────────────────────▼──────┐
        │              Database               │
        │           (PostgreSQL)              │
        └─────────────────────────────────────┘
```

**API:**
```
POST /api/v1/shorten
Body: { "url": "https://very-long-url.com/..." }
Response: { "short_url": "https://svc.io/abc123ef" }

GET /{code}
Response: 301 Redirect → original URL
```

---

## Фаза 4: Deep Dive

### Short Code Generation

Требования: уникальный, короткий, URL-safe.

**Вариант 1: Base62 от ID**
```
characters = [0-9A-Za-z]  → 62 символа
8 символов → 62^8 = 218 триллионов комбинаций

Алгоритм:
  1. Получить auto-increment ID из БД (или distributed sequence)
  2. Перевести в Base62

Плюсы: детерминировано, нет коллизий
Минусы: ID предсказуем (можно перебрать), нужен центральный генератор ID
```

**Вариант 2: Random + collision check**
```
Алгоритм:
  1. Генерировать 8 случайных Base62 символов
  2. Проверить уникальность в БД
  3. При коллизии — повторить

Плюсы: непредсказуемость, нет централизации
Минусы: вероятность коллизии растёт с заполнением таблицы
  При 1B записях из 218T возможных → вероятность коллизии ~0.0005%
  Приемлемо, но retries нужны
```

**Вариант 3: Hash (MD5/SHA) + truncate**
```
short_code = base62(md5(long_url + salt))[:8]

Плюсы: детерминировано для одного URL (дедупликация)
Минусы: hash collision теоретически возможен, нужна проверка
```

**Выбор: Вариант 2 (Random)** — простота реализации, нет зависимости от централизованного ID. При 35K RPS создания на пике, collision rate незначительна.

---

### База данных и схема

```sql
CREATE TABLE urls (
  id          BIGSERIAL PRIMARY KEY,
  short_code  VARCHAR(10) NOT NULL UNIQUE,
  long_url    TEXT NOT NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
  expires_at  TIMESTAMP NOT NULL,
  user_id     BIGINT  -- nullable, для авторизованных пользователей
);

CREATE INDEX idx_short_code ON urls(short_code);  -- covering index для redirect
CREATE INDEX idx_expires_at ON urls(expires_at);  -- для cleanup job
```

**Почему PostgreSQL, а не NoSQL?**
- Данные структурированы, схема стабильна
- ACID нужен для уникальности short_code (уникальный индекс + транзакция)
- 4.5 TB — PostgreSQL справится без проблем
- При 10x росте → шардирование по `short_code` (hash sharding)

---

### Стратегия кеширования

**Что кешировать:** горячие URL (redirect path)

```
Cache key:   short_code
Cache value: long_url
TTL в кеше: min(URL expires_at, 24h)

Cache hit:  short_code → Redis → 301 Redirect (< 1ms)
Cache miss: short_code → Redis miss → PostgreSQL → заполнить кеш → 301 Redirect
```

**Cache invalidation:**
- При удалении/истечении URL → `DEL short_code` из Redis
- TTL-based expiry как safety net

**Размер кеша:**
```
Допустим, 20% URL дают 80% трафика (Zipf distribution)
1B redirects/day, top 20% URLs = 200M × 300 bytes ≈ 60GB
Redis cluster с 3 нодами по 32GB = 96GB → хватит
```

**Cache warming:** при запуске — загрузить топ-1000 URL по click count в кеш.

---

### Redirect: 301 vs 302

```
301 Permanent Redirect:
  + Браузер кеширует → меньше нагрузки на сервер
  - Нельзя изменить URL после кеширования браузером
  - Аналитика не работает (браузер не обращается к нам)

302 Temporary Redirect:
  + Каждый переход проходит через нас → точная аналитика
  + Можно изменить destination URL
  - Больше нагрузки (нет browser cache)
```

**Выбор:** 302 — аналитика важна для продукта. Компенсируем кешем на нашей стороне (Redis).

---

### Click Counter (асинхронный)

**Проблема:** при 35K RPS increment на каждый redirect → write hotspot в БД.

**Решение: async batch update**
```
Redirect service:
  1. Вернуть redirect немедленно
  2. Послать событие в Kafka/Redis Stream: { short_code, timestamp }

Analytics worker:
  - Читает из очереди батчами
  - UPDATE urls SET click_count += N WHERE short_code = X (раз в секунду)
  - Или писать в отдельную таблицу clicks для детальной аналитики
```

Это decouples critical path (redirect) от аналитики.

---

### Cleanup Expired URLs

```
Background job (cron, ежедневно):
  DELETE FROM urls WHERE expires_at < NOW() LIMIT 10000;

  -- Батчевое удаление, чтобы не лочить таблицу
  -- Индекс idx_expires_at делает поиск дешёвым
```

---

## Трейдоффы и альтернативы

| Решение | Принятое | Альтернатива | Когда выбирать альтернативу |
|---|---|---|---|
| Short code gen | Random Base62 | Base62 от sequence ID | Если нужна строгая уникальность без retries |
| БД | PostgreSQL | Cassandra/DynamoDB | Если 10x больше writes или глобальное geo-распределение |
| Кеш | Redis | Memcached | Если нужны только simple KV без persistence |
| Redirect type | 302 | 301 | Если аналитика не нужна и нужно снизить нагрузку |
| Click counter | Async Kafka | Sync DB update | Если click count некритичен для latency |

---

## Масштабирование до 10x

```
Текущее: 35K RPS redirect
10x: 350K RPS redirect

Что меняется:
  - Redis cluster: горизонтальный шардинг (Redis Cluster)
  - Read API: добавить реплики (stateless, легко)
  - PostgreSQL: read replicas для redirect miss path
  - При 10x writes (3500 creates/sec): рассмотреть sharding по short_code
  - CDN: кешировать redirects на edge (если глобальный трафик)
```

---

## Финальная архитектура

```
Client
  │
  ├── POST /shorten → Write API → PostgreSQL (INSERT + уникальный индекс)
  │                                     └── генерация Random Base62
  │
  └── GET /{code}  → Read API → Redis → hit: 302 Redirect
                                      → miss: PostgreSQL → Redis SET → 302 Redirect
                                                    └── async event → Kafka → Analytics Worker
```

---

## Interview-ready ответ (2 минуты)

> "URL shortener — это сервис с сильным read/write skew: на 1 создание приходится 100 редиректов. Поэтому ключевые решения вокруг read path.
>
> Short code — 8 символов Base62 (62^8 = 218T комбинаций), генерирую случайно с проверкой уникальности. Коллизии на таком объёме единичны.
>
> Хранилище — PostgreSQL с уникальным индексом на short_code. Объём данных — порядка 4.5 TB за 5 лет, это решаемо без шардирования.
>
> Critical path: GET /{code} → Redis → при miss → PostgreSQL → заполнить кеш → 302. Цель — p99 < 10ms, Redis даёт < 1ms. Кеш покрывает 80% трафика (Zipf distribution).
>
> 302 вместо 301 — потому что нужна аналитика. Click count обновляю асинхронно через очередь, чтобы не блокировать redirect.
>
> Для масштабирования — stateless Read API горизонтально, Redis Cluster, PostgreSQL read replicas."
