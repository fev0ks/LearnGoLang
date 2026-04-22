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

## Перекрёстные ссылки

- [Общие паттерны системного дизайна](../patterns/) — кеширование, очереди, шардирование
- [Как проходить System Design Interview](./00-how-to-approach.md) — фреймворк и тайминг
