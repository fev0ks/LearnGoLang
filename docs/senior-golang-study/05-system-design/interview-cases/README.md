# System Design Interview Cases

Разборы популярных задач system design этапа с полным прохождением по фазам: уточнение требований → оценка нагрузки → высокоуровневый дизайн → deep dive → трейдоффы.

## Как использовать

Каждый кейс написан в формате **интервью-симуляции**: не просто "ответ", а что говорить, что спрашивать, как обосновывать решения. Перед разбором конкретных задач — изучи общий фреймворк.

## Материалы

### Фреймворк
- [00. Как проходить System Design Interview](./00-how-to-approach.md) — структура интервью, тайминг, что оценивает интервьюер, типичные ошибки

### Базовые кейсы
- [01. URL Shortener](./01-url-shortener.md) — Base62, кеш, redirect 301 vs 302, click counter async
- [02. Notification Service](./02-notification-service.md) — fan-out, Kafka per channel, retry + DLQ, transactional vs marketing
- [03. Rate Limiter](./03-rate-limiter.md) — алгоритмы (fixed/sliding/token bucket), Redis Lua, fail-open
- [04. Chat / Messaging](./04-chat-messaging.md) — WebSocket, Snowflake IDs, ScyllaDB, fan-out для групп, presence
- [05. Task Queue](./05-task-queue.md) — Redis Streams, priority queues, delayed tasks, at-least-once, retry backoff

### Сложные кейсы
- [06. Uber / Ride-Sharing](./06-uber-ride-sharing.md) — H3 geo index, real-time location (120K updates/sec), matching с distributed lock, multi-region
- [07. YouTube / Video Platform](./07-youtube-video-platform.md) — chunked upload, transcode pipeline, HLS ABR, CDN, view counter at scale
- [08. Twitter / Social Feed](./08-twitter-social-feed.md) — hybrid fan-out (celebrity problem), Cassandra + Redis, Snowflake IDs, home timeline
- [09. Netflix / Streaming](./09-netflix-streaming.md) — Open Connect CDN, per-title encoding, playback service, Chaos Engineering
- [10. Google Drive](./10-google-drive.md) — content-addressed chunking, Rabin fingerprint, deduplication, sync protocol, conflict resolution
- [11. Payment System](./11-payment-system.md) — double-entry bookkeeping, idempotency, Saga + Outbox, reconciliation, strong consistency
- [12. Marketplace Vendor Notifications](./12-marketplace-vendor-notifications.md) — webhook delivery (Stripe-style), outbox + Kafka, per-vendor circuit breaker, HMAC signing, dead letter
- [13. Avito / Classifieds](./13-avito-classifieds.md) — фасетный поиск (Elasticsearch), category-specific атрибуты (JSONB + денорм), Outbox→ES, медиа-пайплайн, горячее чтение карточек, view counter, модерация/антифрод
- [14. Stock / Inventory Service](./14-stock-inventory-service.md) — двухфазный резерв (hold→commit/cancel) + TTL, защита от overselling (atomic условный декремент), горячий SKU при flash sale (бакетирование / Redis fast-path), шардинг по product_id, sync-репликация, чтение из кеша vs решение на записи
- [15. TMS / Transport Management](./15-tms-transport-management.md) — нормализация разнородных источников (anti-corruption adapters), заказ↔маршрут many-to-many (консолидация ~1300/маршрут), типы маршрутов (line-haul/первая/последняя миля/смешанный), составной заказ как бизнес-сага (DAG этапов через хабы), назначение исполнителя (Redis geo + NX-лок), трекинг 10K водителей (stateless WS-gateway)

## Структура каждого кейса

```
Фаза 1: Уточнение требований
  → что спрашивать, что включать в scope / out of scope

Фаза 2: Оценка нагрузки (back-of-envelope)
  → RPS, storage, выводы которые влияют на архитектуру

Фаза 3: Высокоуровневый дизайн
  → диаграмма компонентов, основной поток данных

Фаза 4: Deep Dive
  → детали ключевых компонентов, схемы данных, алгоритмы

Трейдоффы
  → сравнение альтернативных решений с обоснованием

Interview-ready ответ
  → 2-минутный summary для реального интервью
```

## Принципы написания кейсов

Поверх скелета выше — правила, которые держат кейс полезным и нераздутым (эталон — [13. Avito](./13-avito-classifieds.md)):

1. **Числа первичны.** Сначала оценка нагрузки, затем каждое архитектурное решение обосновано цифрой («read-heavy 1000:1 → отдельный поисковый индекс», «150K rps на карточку → кеш», «write hotspot → INCR+flush»). Не «потому что популярно», а «потому что <конкретная цифра>».
2. **Раздел «Роль каждого компонента».** Для каждого блока диаграммы — *Зачем* (что делает) и *Почему отдельно / почему именно он*. Сквозную идею называть явно (напр. CQRS-разделение по типу нагрузки).
3. **«Сквозные потоки».** Пронумерованные end-to-end сценарии (запись / поиск / чтение / загрузка) с «Итогом» у каждого — показывают, как компоненты работают вместе.
4. **Кейс лёгкий, глубина — в профильных доках.** Реализацию (Redis hot key, presigned-подпись, Indexer/Bulk API, GETDEL) выносить в catalog-доки и **кросс-линковать**, а не расписывать в кейсе.
5. **Каждый компонент линкуется на профильный док** (Elasticsearch, Redis, S3/object storage, брокеры и т.д.).
6. **Термины не всухую** — расшифровать или сослаться на расшифровку.
7. **Контраст-таблицы** для альтернатив (выбор / альтернатива / причина) + явное «почему X, а не Y».
8. **Кросс-ссылки без `#якоря`** (их не открывает Markdown-превью IntelliJ) — ссылаться на файл и называть нужный раздел текстом рядом; делать двунаправленно (кейс ↔ профиль).

## Перекрёстные ссылки

- [Общие паттерны системного дизайна](../patterns/) — кеширование, очереди, шардирование
- [Как проходить System Design Interview](./00-how-to-approach.md) — фреймворк и тайминг
