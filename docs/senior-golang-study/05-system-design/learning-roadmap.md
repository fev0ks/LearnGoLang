# System Design: пятинедельный учебный план

## Содержание

- [Как пользоваться планом](#как-пользоваться-планом)
- [Неделя 1: интервью, сеть и доставка](#неделя-1-интервью-сеть-и-доставка)
- [Неделя 2: базы данных](#неделя-2-базы-данных)
- [Неделя 3: кэш, очереди и устойчивость](#неделя-3-кэш-очереди-и-устойчивость)
- [Неделя 4: социальные и медиасистемы](#неделя-4-социальные-и-медиасистемы)
- [Неделя 5: конкуренция, поиск и геоданные](#неделя-5-конкуренция-поиск-и-геоданные)
- [Как завершить подготовку](#как-завершить-подготовку)

План собран по мотивам
[программы курса System Design](https://guide.olezhek28.courses/system_design/?utm_source=youtube&utm_medium=organic&utm_campaign=open-lesson-1&utm_content=27-08-2026)
и связывает её темы с материалами репозитория. Порядок идёт от структуры
интервью и пути сетевого запроса к хранилищам, устойчивости и полным проектным
задачам.

---

## Как пользоваться планом

Неделя здесь означает логический модуль, а не строгий календарный срок. Если тема
новая, на неё можно потратить несколько дней; если уже знакома — пройти быстрее.

Для каждой темы полезен один и тот же цикл:

1. Прочитать основной материал и выписать главную модель своими словами.
2. Ответить без конспекта: какую проблему решает механизм и чем за него платим.
3. Нарисовать ключевой поток или переходы состояний на чистом листе.
4. Открыть дополнительные материалы только для непонятных деталей.
5. В конце недели разобрать практический кейс под таймером.

В таблицах ссылки расположены в рекомендуемом порядке. Первый материал задаёт
основную модель, дополнительные углубляют отдельные механизмы или показывают их в
другом контексте.

---

## Неделя 1: интервью, сеть и доставка

Цель недели — научиться вести первые фазы System Design Interview и объяснять
путь запроса от клиента до backend-сервиса.

| № | Тема | Основной материал | Дополнительно |
| ---: | --- | --- | --- |
| 1 | Как устроено System Design Interview | [Как проходить System Design Interview](./interview-cases/00-how-to-approach.md) | — |
| 2 | Что оценивает интервьюер | [Как проходить System Design Interview](./interview-cases/00-how-to-approach.md) | — |
| 3 | Пять фаз разбора задачи | [Как проходить System Design Interview](./interview-cases/00-how-to-approach.md) | [C4-диаграммы: уровни и interview playbook](./c4-model/README.md) |
| 4 | Уточняющие вопросы и границы задачи | [Как проходить System Design Interview](./interview-cases/00-how-to-approach.md) | — |
| 5 | Оценка нагрузки в уме | [Back-of-envelope и вывод capacity](./interview-cases/00-how-to-approach.md) | — |
| 6 | REST API, идемпотентность и выбор топологии | [API design](../08-networking-and-api/api-design/README.md) | [Idempotency](./reliability-patterns/06-idempotency.md), [Монолит и микросервисы](../04-architecture-and-patterns/service-topologies/01-monolith-vs-modular-monolith-vs-microservices.md) |
| 7 | Сквозной разбор ленты и каталога | [Avito / Classifieds](./interview-cases/13-avito-classifieds.md) | [Twitter / Social Feed](./interview-cases/08-twitter-social-feed.md) |
| 8 | Балансировка L4/L7 и gRPC | [CDN, балансировщик и reverse proxy](../08-networking-and-api/request-lifecycle/04-cdn-load-balancer-reverse-proxy.md) | [gRPC](../08-networking-and-api/protocols/03-api-styles/02-grpc.md) |
| 9 | Consistent hashing и перенос данных | [Highload: шардирование](./highload-design-patterns.md) | [Шардирование PostgreSQL](../06-databases/database-systems-catalog/postgresql/12-sharding.md) |
| 10 | API Gateway, reverse proxy и BFF | [Роли edge-компонентов](./external-request-flows/edge-and-proxy-patterns/01-edge-roles-and-terms.md) | [Сравнение API-протоколов и BFF](../08-networking-and-api/protocols/00-protocol-comparison.md) |
| 11 | CDN: push/pull, cache key, invalidation и signed URL | [CDN deep dive](../08-networking-and-api/request-lifecycle/08-cdn-deep-dive.md) | [Read-heavy flow](./external-request-flows/02-read-heavy-request-with-cdn-and-cache.md), [File upload и signed URL](./external-request-flows/05-file-upload-and-background-processing-flow.md) |
| 12 | DNS: resolution, records и TTL | [DNS resolution](../08-networking-and-api/request-lifecycle/02-dns-resolution-and-getting-ip.md) | — |

### Практика недели

[Emergency App Rollout](./interview-cases/28-emergency-app-rollout.md): доставить
критическое обновление на 50 млн устройств за 6 часов. Нужно посчитать bandwidth,
разделить control plane и data plane, выбрать staged rollout, regional throttling,
kill switch и rollback.

После кейса отдельно повторить [Feature rollout types](./experimentation-and-feature-rollouts/01-experimentation-and-rollout-types.md)
и [Retries and backoff](./reliability-patterns/02-retries-and-backoff.md).

### Результат недели

- Умеешь начинать интервью с требований и чисел, а не с названий технологий.
- Объясняешь путь запроса через DNS, edge, gateway, backend и хранилище.
- Различаешь управляющий поток и доставку тяжёлых данных через CDN.

---

## Неделя 2: базы данных

Цель недели — выбирать модель хранения через инварианты и шаблоны доступа, а
затем объяснять поведение базы при конкуренции и отказах.

| № | Тема | Основной материал | Дополнительно |
| ---: | --- | --- | --- |
| 13 | Модели данных и выбор БД | [Сравнение хранилищ](../06-databases/database-systems-catalog/01-comparison-table.md) | [Database interview cases](../06-databases/database-fundamentals/04-interview-cases.md) |
| 14 | Диск, WAL, commit и crash recovery | [Репликация PostgreSQL: WAL и recovery](../06-databases/database-systems-catalog/postgresql/06-replication.md) | — |
| 15 | B-tree, LSM и составные индексы | [Индексы PostgreSQL](../06-databases/database-systems-catalog/postgresql/02-indexes.md) | [Cassandra: LSM write/read path](../06-databases/database-systems-catalog/05-cassandra.md) |
| 16 | Транзакции, ACID и BASE | [ACID](../06-databases/database-fundamentals/01-acid.md) | [CAP, BASE и eventual consistency](../06-databases/database-fundamentals/02-cap-and-base.md) |
| 17 | Locks, Two-Phase Locking, deadlock, MVCC и vacuum | [Транзакции и блокировки](../06-databases/database-systems-catalog/postgresql/04-transactions-and-locking.md) | [MVCC и Vacuum](../06-databases/database-systems-catalog/postgresql/01-mvcc-and-vacuum.md) |
| 18 | Уровни изоляции и аномалии | [Транзакции и блокировки](../06-databases/database-systems-catalog/postgresql/04-transactions-and-locking.md) | [ACID](../06-databases/database-fundamentals/01-acid.md) |
| 19 | Single-leader, leaderless, lag, quorum и fencing | [Модели репликации](../06-databases/database-fundamentals/05-replication-models.md) | [PostgreSQL replication](../06-databases/database-systems-catalog/postgresql/06-replication.md), [Cassandra consistency](../06-databases/database-systems-catalog/05-cassandra.md) |
| 20 | Multi-leader replication и разрешение конфликтов | [Модели репликации](../06-databases/database-fundamentals/05-replication-models.md) | [CAP и conflict resolution](../06-databases/database-fundamentals/02-cap-and-base.md) |
| 21 | Consensus: Raft и Paxos | [Consensus: Raft и границы сравнения с Paxos](../06-databases/database-fundamentals/06-consensus-raft-and-paxos.md) | — |
| 22 | Партиционирование и шардирование | [Highload: шардирование](./highload-design-patterns.md) | [PostgreSQL: partitioning](../06-databases/database-systems-catalog/postgresql/05-partitioning.md), [PostgreSQL: sharding](../06-databases/database-systems-catalog/postgresql/12-sharding.md) |
| 23 | CAP и PACELC | [CAP, BASE и PACELC](../06-databases/database-fundamentals/02-cap-and-base.md) | — |

### Практика недели

[Ad Budget Service](./interview-cases/29-ad-budget-service.md): обработать 1,2 млрд
списаний в сутки при skew `1% аккаунтов → 60% трафика`. Нужно защитить лимиты
аккаунта и кампаний, выбрать ключи шардирования, разнести hot account через escrow
leases и объяснить ledger с reconciliation.

Для углубления использовать [Payment System](./interview-cases/11-payment-system.md),
[Promo Code Service](./interview-cases/22-promo-code-service.md) и
[Hot rows and counters](../06-databases/database-systems-catalog/postgresql/highload-scenarios/04-hot-rows-and-counters.md).

### Результат недели

- Выбираешь БД и индекс от операций чтения/записи и бизнес-инвариантов.
- Различаешь репликацию, consensus, транзакцию, lock и шардирование.
- Объясняешь, почему hot key остаётся горячим после обычного шардирования.

---

## Неделя 3: кэш, очереди и устойчивость

Цель недели — сделать повторы безопасными, ограничить область отказа и объяснить,
как система деградирует при перегрузке.

| № | Тема | Основной материал | Дополнительно |
| ---: | --- | --- | --- |
| 24 | Cache-aside, write-through, eviction и два уровня кэша | [Redis как кэш](../06-databases/caching/01-redis-as-cache.md) | [Highload: многоуровневое кэширование](./highload-design-patterns.md) |
| 25 | Инвалидация, hot key, stampede и cold cache | [Redis как кэш](../06-databases/caching/01-redis-as-cache.md) | [Highload: hot key и thundering herd](./highload-design-patterns.md) |
| 26 | Queue против log, delivery guarantees и DLQ | [Сравнение брокеров](../07-message-brokers-and-streaming/00-comparison.md) | [Kafka](../07-message-brokers-and-streaming/01-kafka.md), [RabbitMQ](../07-message-brokers-and-streaming/02-rabbitmq.md) |
| 27 | Idempotency и transactional outbox | [Idempotency](./reliability-patterns/06-idempotency.md) | [PostgreSQL: outbox и idempotency](../06-databases/database-systems-catalog/postgresql/14-outbox-and-idempotency.md) |
| 28 | Two-Phase Commit, Three-Phase Commit, Saga и Try-Confirm-Cancel | [Распределённые транзакции](../04-architecture-and-patterns/patterns/11-distributed-transactions-2pc-3pc-tcc.md) | [Saga и Outbox](../04-architecture-and-patterns/patterns/09-saga-and-outbox.md) |
| 29 | Timeout, retry, backoff и jitter | [Timeouts and deadlines](./reliability-patterns/01-timeouts-and-deadlines.md) | [Retries and backoff](./reliability-patterns/02-retries-and-backoff.md) |
| 30 | Circuit breaker и bulkhead | [Circuit breaker](./reliability-patterns/03-circuit-breaker.md) | [Bulkhead](./reliability-patterns/07-bulkhead.md) |
| 31 | Rate limiting | [Rate Limiter case](./interview-cases/03-rate-limiter.md) | [Reliability: rate limiting](./reliability-patterns/04-rate-limiting.md) |
| 32 | Масштабирование и плавная деградация | [Highload Design Patterns](./highload-design-patterns.md) | [Backpressure and load shedding](./reliability-patterns/05-backpressure-and-shedding.md) |

### Практика недели

[Black Friday Marketplace](./interview-cases/30-black-friday-marketplace.md):
собрать каталог, exact quote, Stock, Promo, Order и Payment в один 45-минутный
разбор. Главная часть упражнения — admission control, защита hot SKU/campaign и
порядок отключения необязательных функций при пике ×20.

Перед практикой полезно повторить [Stock / Inventory Service](./interview-cases/14-stock-inventory-service.md),
[Promo Code Service](./interview-cases/22-promo-code-service.md) и
[Backpressure and load shedding](./reliability-patterns/05-backpressure-and-shedding.md).

### Результат недели

- Выбираешь между синхронным вызовом, очередью и журналом событий.
- Проектируешь повторяемые операции через idempotency, outbox и reconciliation.
- Заранее задаёшь admission, backpressure и порядок graceful degradation.

---

## Неделя 4: социальные и медиасистемы

Цель недели — применить базовые приёмы к WebSocket, fan-out, media pipeline и
доставке больших файлов.

| № | Тема | Основной материал | Дополнительно |
| ---: | --- | --- | --- |
| 33 | Messenger: соединения, порядок, большие чаты и presence | [Chat / Messaging](./interview-cases/04-chat-messaging.md) | [WebSocket Chat at Scale](./interview-cases/04.1-websocket-chat-capacity.md), [Marketplace Messenger](./interview-cases/18-marketplace-messenger.md) |
| 34 | Avito: каталог, выдача и hot source | [Avito / Classifieds](./interview-cases/13-avito-classifieds.md) | [Twitter / Social Feed](./interview-cases/08-twitter-social-feed.md) |
| 35 | Фото-соцсеть и Stories | [Photo Stories Service](./interview-cases/25-photo-stories-service.md) | — |
| 36 | Видеоплатформа: upload, transcoding, ABR, CDN и views | [YouTube / Video Platform](./interview-cases/07-youtube-video-platform.md) | [Netflix / Streaming](./interview-cases/09-netflix-streaming.md) |
| 37 | Музыкальный streaming и playback | [Music Playlist Service](./interview-cases/17-music-playlist-service.md) | [Music Streaming Delivery](./interview-cases/17.1-music-streaming-delivery.md) |

### Практика недели

[Twitch / Live Streaming Platform](./interview-cases/19-live-streaming-platform.md):
спроектировать live video для 5 млн зрителей, а затем подключить
[WebSocket Chat at Scale](./interview-cases/04.1-websocket-chat-capacity.md).
Нужно отдельно посчитать media delivery и chat fan-out, а связь подсистем
построить через события `stream.started` и `stream.ended`.

### Результат недели

- Разделяешь тяжёлый media payload и управляющие metadata/API.
- Выбираешь fan-out on write, fan-out on read или гибрид по форме аудитории.
- Объясняешь upload, processing, CDN delivery и realtime updates как разные пути.

---

## Неделя 5: конкуренция, поиск и геоданные

Цель недели — проектировать дефицитные ресурсы, распределённый поиск и
геопространственный matching, не теряя структуру ответа под таймером.

| № | Тема | Основной материал | Дополнительно |
| ---: | --- | --- | --- |
| 38 | Билеты и защита от double booking | [Ticket Booking Service](./interview-cases/26-ticket-booking-service.md) | [Meeting Room Booking](./interview-cases/23-meeting-room-booking.md) |
| 39 | Search и autocomplete | [Search / Autocomplete Service](./interview-cases/27-search-autocomplete-service.md) | [Elasticsearch/OpenSearch](../06-databases/database-systems-catalog/09-elasticsearch-and-opensearch.md) |
| 40 | Taxi: Order, Trip, геоиндекс и matching | [Uber / Ride-Sharing](./interview-cases/06-uber-ride-sharing.md) | — |
| 41 | Поведение на System Design Interview | [Фразы, deep dive, тупик и тренировка](./interview-cases/00-how-to-approach.md) | — |

### Практика недели

[Airbnb Booking](./interview-cases/31-airbnb-booking.md): связать поиск по карте,
приблизительную availability projection, точный календарь, price snapshot, hold,
Payment Saga и отмену. Отдельно объяснить, почему stale search допустим, а booking
последнего доступного жилья работает только через exact authority.

Для сравнения использовать [Uber / Ride-Sharing](./interview-cases/06-uber-ride-sharing.md),
[Avito / Classifieds](./interview-cases/13-avito-classifieds.md),
[Stock / Inventory](./interview-cases/14-stock-inventory-service.md) и
[Meeting Room Booking](./interview-cases/23-meeting-room-booking.md).

### Результат недели

- Отделяешь приблизительный candidate source от точного бизнес-решения.
- Защищаешь дефицитный ресурс условной транзакцией и идемпотентным hold.
- Проектируешь геопоиск, шардирование и Saga как части одного сквозного потока.

---

## Как завершить подготовку

После пяти недель повторно пройти пять практических задач без открытого конспекта:

1. [Emergency App Rollout](./interview-cases/28-emergency-app-rollout.md).
2. [Ad Budget Service](./interview-cases/29-ad-budget-service.md).
3. [Black Friday Marketplace](./interview-cases/30-black-friday-marketplace.md).
4. [Twitch / Live Streaming Platform](./interview-cases/19-live-streaming-platform.md).
5. [Airbnb Booking](./interview-cases/31-airbnb-booking.md).

На каждый разбор отводить 45–60 минут:

- первые 5–7 минут — требования и границы задачи;
- следующие 5–8 минут — расчёты, которые влияют на архитектуру;
- около 15 минут — две читаемые схемы и роли компонентов;
- около 15 минут — один или два ключевых deep dive;
- последние 5 минут — отказы, trade-offs и двухминутное резюме.

После тренировки сверяться с [общим фреймворком интервью](./interview-cases/00-how-to-approach.md),
записывать места, где решение появилось без числа или инварианта, и повторять
только связанные материалы. Цель плана — не запомнить набор технологий, а
научиться последовательно выводить архитектуру из требований, нагрузки и цены
ошибки.
