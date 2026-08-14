# System Design Interview Cases

Разборы популярных задач system design этапа с полным прохождением по фазам: уточнение требований → оценка нагрузки → высокоуровневый дизайн → deep dive → финальное резюме.

## Как использовать

Каждый кейс написан в формате **интервью-симуляции**: не просто "ответ", а что говорить, что спрашивать, как обосновывать решения. Перед разбором конкретных задач — изучи общий фреймворк.

## Материалы

### Фреймворк
- [00. Как проходить System Design Interview](./00-how-to-approach.md) — структура интервью, тайминг, что оценивает интервьюер, типичные ошибки

### Базовые кейсы
- [01. URL Shortener](./01-url-shortener.md) — Base62, кеш, redirect 301 vs 302, click counter async
- [02. Notification Service](./02-notification-service.md) — fan-out, Kafka per channel, retry + DLQ, transactional vs marketing
- [03. Rate Limiter](./03-rate-limiter.md) — алгоритмы (fixed/sliding/token bucket), Redis Lua, fail-open
- [04. Chat / Messaging](./04-chat-messaging.md) — WebSocket, Kafka-first запись и seq из порядка партиции, Cassandra, fan-out для групп, presence, watermark-статусы
- [04.1 WebSocket Chat at Scale](./04.1-websocket-chat-capacity.md) — capacity drill на 1M соединений: reconnect blast radius, hot-group fan-out, route cache, backpressure и tiered history
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
- [14. Stock / Inventory Service](./14-stock-inventory-service.md) — interview-case на 45–60 минут: условный резерв без overselling, multi-warehouse allocations, exact read vs preview, saga и batch writer для hot SKU
- [14.1 Stock / Inventory Service — расширенный разбор](./14.1-stock-inventory-service.md) — подробные DDL, capacity-расчёты, shortage/recovery, failure scenarios, SLO и альтернативы hot-shard design
- [15. TMS / Transport Management](./15-tms-transport-management.md) — нормализация разнородных источников (anti-corruption adapters), заказ↔маршрут many-to-many (консолидация ~1300/маршрут), типы маршрутов (line-haul/первая/последняя миля/смешанный), составной заказ как бизнес-сага (DAG этапов через хабы), назначение исполнителя (Redis geo + NX-лок), трекинг 10K водителей (stateless WS-gateway)
- [16. Gmail / Email Service](./16-gmail-email-service.md) — тонкий SMTP-приём + durable-лог (250 OK = Kafka ack), immutable blob + mutable metadata, дедуп рассылок по body_hash, labels вместо папок, wide-column шард по user_id (ящик = один range-scan), per-user поисковый индекс, threading по RFC-заголовкам, outbound-ретраи с backoff до 24ч
- [17. Music Playlist Service](./17-music-playlist-service.md) — versioned playlist, materialized shuffle queue, Fisher–Yates, стабильная playback session и handoff между устройствами через epoch
- [18. Marketplace Messenger](./18-marketplace-messenger.md) — durable-first сообщения, строгий per-chat seq, WebSocket + push, listing snapshot, outbox fan-out и бессрочная hot/cold история
- [19. Twitch / Live Streaming](./19-live-streaming-platform.md) — контент, которого нет до запроса: транскодирование быстрее реального времени как главная статья расходов, лесенка качеств только популярным каналам (160 серверов вместо 3100), LL-HLS и цена низкой задержки в запросах к CDN, двухуровневая сеть доставки против длинного хвоста, приблизительный счётчик зрителей по сердцебиениям
- [20. Банковские реквизиты поставщиков](./20-vendor-bank-details.md) — задача без нагрузки (0,002 записи/с): три канала обновления с правами на поля вместо «последний прав», изменение счёта как предложение с выдержкой против подмены, битемпоральные версии, витрины департаментов с разной временнóй семантикой, заморозка отчётности через курсор системного времени вместо запрета записи

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

Фаза 5: Финал
  → 2-минутный summary для реального интервью

Interview-ready answer
  → короткий тренажёр уточняющих вопросов по решению
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
9. **Числа пересчитывать, а не переписывать.** Сплошная вычитка этих кейсов дала около полусотни содержательных ошибок, и почти все — однотипные: перепутанные единицы, пик вместо среднего при накоплении, вывод без расчёта («это много»), несогласованные разделы, невозможные в конкретном продукте конструкции (`COUNTER` рядом с обычными колонками в Cassandra, TTL на поле хеша в Redis, `FOR UPDATE` вне транзакции). Чек-лист перед правкой кейса — в [AGENTS.md](../../../../AGENTS.md), раздел «Проверка технической точности»; краткая версия под интервью — в [00. Как проходить](./00-how-to-approach.md), раздел «Как вычитывать собственный разбор».
10. **Меняя число, синхронизировать файл целиком** — оно обычно встречается ещё в диаграмме, ролях компонентов, сквозных потоках, трейдоффах и interview-ready.

## Перекрёстные ссылки

- [Highload Design Patterns](../highload-design-patterns.md) — шардирование, репликация, многоуровневое кеширование, горячий ключ, fan-out, backpressure
- [Reliability Patterns](../reliability-patterns/README.md) — таймауты, повторные попытки, circuit breaker, идемпотентность, SLO
- [Брокеры сообщений и стриминг](../../07-message-brokers-and-streaming/00-comparison.md) — выбор брокера под задачу, разборы с числами
- [Как проходить System Design Interview](./00-how-to-approach.md) — фреймворк и тайминг
