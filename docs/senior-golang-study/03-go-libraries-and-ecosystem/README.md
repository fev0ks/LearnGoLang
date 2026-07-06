# Go Libraries And Ecosystem

Сравнения библиотек по сценариям: trade-offs, когда уместны, какие проблемы создают через полгода.

## Материалы

- [shopspring/decimal](./01-shopspring-decimal.md) — точная десятичная арифметика; почему float64 нельзя для денег
- [google/uuid](./02-google-uuid.md) — UUID v4 vs v7; почему v7 лучше для primary keys
- [samber/lo](./03-samber-lo.md) — generics-утилиты для коллекций: Map, Filter, GroupBy, Chunk
- [pkg/errors](./04-pkg-errors.md) — ошибки со стектрейсом; совместимость с errors.Is
- [alitto/pond](./05-alitto-pond.md) — worker pool: динамический лимит, ResultPool[T], backpressure, метрики
- [panjf2000/ants](./06-panjf2000-ants.md) — пул горутин фиксированной ёмкости; переиспользование горутин, throughput на масштабе

## HTTP и RPC

### HTTP Servers

- [HTTP Servers →](./http-servers/README.md)
  - stdlib net/http (Go 1.22+)
  - chi, gin, echo, fiber
  - gorilla/mux legacy
  - Таблица сравнения и когда что выбирать

### gRPC

- [gRPC →](./grpc/README.md)
  - Protobuf и кодогенерация (buf)
  - grpc-go: сервер, клиент, interceptors, streaming
  - connect-go: три протокола на одном порту, браузерный клиент
  - gRPC vs REST, grpc-go vs connect-go

## Категории (материалы готовятся)
- **Concurrency, worker pools & resilience:**
  - bounded fan-out / батч: `golang.org/x/sync/errgroup` (`SetLimit`), `golang.org/x/sync/semaphore`
  - worker pools: `sourcegraph/conc`, `panjf2000/ants` (высоконагруженный, реюз горутин), `alitto/pond`
  - rate limiting: `golang.org/x/time/rate` (per-process), распределённый — через Redis
  - resilience: `sony/gobreaker` (circuit breaker), adaptive concurrency (Netflix `concurrency-limits`)
  - живой пример junior/middle/senior + разбор собес-вопросов → [topics/02-concurrency/workerpool](../../../topics/02-concurrency/workerpool/README.md)
- **Config:** `envconfig`, `viper`, manual parsing
- **Logging:** `slog`, `zap`, `zerolog`
- **Database access:** `sqlx`, `pgx`, `gorm`, `bun`, `ent` → см. [06-databases/go-database-libraries](../06-databases/go-database-libraries/)
- **Validation:** `go-playground/validator`
- **DI/wiring:** `google/wire`, `uber/fx`, manual composition
- **Testing:** `testify`, `go-cmp`, `gomock`, `testcontainers-go`
- **Messaging:** Kafka, RabbitMQ, NATS, Redis streams
- **Observability:** OpenTelemetry SDKs and exporters

## Для каждой библиотеки важно понимать

- сильные стороны
- слабые стороны
- где уместна
- почему выбрал бы в production
- какие проблемы создаёт команде через полгода

## Вопросы

- почему для нового сервиса стоит выбрать `chi`, `gin` или stdlib router
- когда `pgx` лучше, чем `database/sql`, а когда разница не окупается
- в каком случае ORM ускоряет команду, а в каком скрывает слишком много
- почему `zap` или `zerolog` могут быть лучше `slog`, и наоборот
- когда DI-фреймворк оправдан, а когда manual wiring проще и надёжнее
- почему нельзя хранить деньги в `float64`
- чем UUID v7 лучше v4 для primary keys

## Подборка

Популярность — звёзды на GitHub (округлённо, на июнь 2026). Для пакетов из `golang.org/x` звёзды не приводятся: они живут в общих репозиториях-зонтиках, поэтому цифра не отражает популярность конкретного пакета — на деле это часть официального инструментария Go и используется повсеместно.

| Библиотека | Категория | Назначение | ⭐ GitHub | Статья |
| --- | --- | --- | --- | --- |
| [go-chi/chi](https://github.com/go-chi/chi) | HTTP-роутер | Лёгкий идиоматичный роутер на базе `net/http`, middleware без фреймворка | ~22.4k | [chi](./http-servers/02-chi.md) |
| [gin-gonic/gin](https://github.com/gin-gonic/gin) | HTTP-фреймворк | Самый популярный веб-фреймворк, быстрый роутинг, рендеринг, binding | ~88.7k | [gin](./http-servers/03-gin.md) |
| [jackc/pgx](https://github.com/jackc/pgx) | PostgreSQL-драйвер | Нативный драйвер и toolkit для PostgreSQL, быстрее `database/sql` | ~13.9k | — |
| [jmoiron/sqlx](https://github.com/jmoiron/sqlx) | SQL-хелпер | Расширение `database/sql`: маппинг строк в структуры без полноценного ORM | ~17.7k | — |
| [GORM](https://gorm.io/docs/) | ORM | Самый распространённый ORM: ассоциации, миграции, хуки | ~39.8k | — |
| [Bun](https://bun.uptrace.dev/guide/) | ORM / query builder | Лёгкий SQL-first ORM поверх `database/sql`, ближе к чистому SQL | ~4.9k | — |
| [Ent](https://entgo.io/docs/getting-started) | ORM (codegen) | Граф-ориентированный ORM с генерацией типобезопасного кода из схемы | ~17.1k | — |
| [uber-go/zap](https://github.com/uber-go/zap) | Логирование | Структурированный логгер с нулевыми аллокациями, де-факто стандарт | ~24.5k | — |
| [rs/zerolog](https://github.com/rs/zerolog) | Логирование | Zero-allocation JSON-логгер, чейнинг-API | ~12.4k | — |
| [go-playground/validator](https://github.com/go-playground/validator) | Валидация | Валидация структур по тегам, основа для большинства веб-стеков | ~20.0k | — |
| [Testcontainers for Go](https://golang.testcontainers.org/) | Тестирование | Поднимает зависимости (БД, брокеры) в Docker для интеграционных тестов | ~4.9k | — |
| [google/wire](https://github.com/google/wire) | DI (compile-time) | Генерация кода для внедрения зависимостей без рантайм-рефлексии | ~14.4k | — |
| [uber-go/fx](https://github.com/uber-go/fx) | DI (runtime) | DI-фреймворк с жизненным циклом приложения через рефлексию | ~7.6k | — |
| [shopspring/decimal](https://github.com/shopspring/decimal) | Числа | Произвольная точность для денег, без ошибок `float64` | ~7.4k | [shopspring/decimal](./01-shopspring-decimal.md) |
| [google/uuid](https://github.com/google/uuid) | Идентификаторы | Генерация и парсинг UUID (включая v7 для PK) | ~6.1k | [google/uuid](./02-google-uuid.md) |
| [samber/lo](https://github.com/samber/lo) | Утилиты | Lodash-стиль: `Map`/`Filter`/`Reduce` и др. на дженериках | ~21.3k | [samber/lo](./03-samber-lo.md) |
| [pkg/errors](https://github.com/pkg/errors) | Ошибки | Обёртки со стектрейсом; архивирован — в новом коде `errors`/`fmt.Errorf` | ~8.3k | [pkg/errors](./04-pkg-errors.md) |
| [x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) | Конкурентность | Группа горутин с распространением первой ошибки и отменой контекста | — | — |
| [x/sync/semaphore](https://pkg.go.dev/golang.org/x/sync/semaphore) | Конкурентность | Взвешенный семафор для ограничения параллелизма | — | — |
| [sourcegraph/conc](https://github.com/sourcegraph/conc) | Конкурентность | Структурированная конкурентность: пулы, `WaitGroup`, безопасные паники | ~10.4k | — |
| [panjf2000/ants](https://github.com/panjf2000/ants) | Worker pool | Переиспользуемый пул горутин с ограничением и реюзом | ~14.4k | [panjf2000/ants](./06-panjf2000-ants.md) |
| [alitto/pond](https://github.com/alitto/pond) | Worker pool | Лёгкий типобезопасный worker pool с метриками | ~2.2k | [alitto/pond](./05-alitto-pond.md) |
| [x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) | Rate limiting | Token bucket лимитер из официального `x`-репозитория | — | — |
| [sony/gobreaker](https://github.com/sony/gobreaker) | Resilience | Реализация паттерна Circuit Breaker | ~3.6k | — |
| [Netflix concurrency-limits](https://github.com/Netflix/concurrency-limits) | Resilience | Адаптивные лимиты конкурентности (TCP-style congestion control) | ~3.6k | — |
