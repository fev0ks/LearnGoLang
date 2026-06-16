# Go Libraries And Ecosystem

Сравнения библиотек по сценариям: trade-offs, когда уместны, какие проблемы создают через полгода.

## Материалы

- [shopspring/decimal](./01-shopspring-decimal.md) — точная десятичная арифметика; почему float64 нельзя для денег
- [google/uuid](./02-google-uuid.md) — UUID v4 vs v7; почему v7 лучше для primary keys
- [samber/lo](./03-samber-lo.md) — generics-утилиты для коллекций: Map, Filter, GroupBy, Chunk
- [pkg/errors](./04-pkg-errors.md) — ошибки со стектрейсом; совместимость с errors.Is

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

- почему ты выбрал бы `chi`, `gin` или stdlib router для нового сервиса
- когда `pgx` лучше, чем `database/sql`, а когда разница не окупается
- в каком случае ORM ускоряет команду, а в каком скрывает слишком много
- почему `zap` или `zerolog` могут быть лучше `slog`, и наоборот
- когда DI-фреймворк оправдан, а когда manual wiring проще и надёжнее
- почему нельзя хранить деньги в `float64`
- чем UUID v7 лучше v4 для primary keys

## Подборка

- [go-chi/chi](https://github.com/go-chi/chi)
- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [jackc/pgx](https://github.com/jackc/pgx)
- [sqlx](https://github.com/jmoiron/sqlx)
- [GORM Docs](https://gorm.io/docs/)
- [Bun Guide](https://bun.uptrace.dev/guide/)
- [Ent Docs](https://entgo.io/docs/getting-started)
- [zap](https://github.com/uber-go/zap)
- [zerolog](https://github.com/rs/zerolog)
- [validator](https://github.com/go-playground/validator)
- [Testcontainers for Go](https://golang.testcontainers.org/)
- [google/wire](https://github.com/google/wire)
- [uber-go/fx](https://github.com/uber-go/fx)
- [shopspring/decimal](https://github.com/shopspring/decimal)
- [google/uuid](https://github.com/google/uuid)
- [samber/lo](https://github.com/samber/lo)
- [pkg/errors](https://github.com/pkg/errors)
- [x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup)
- [x/sync/semaphore](https://pkg.go.dev/golang.org/x/sync/semaphore)
- [sourcegraph/conc](https://github.com/sourcegraph/conc)
- [panjf2000/ants](https://github.com/panjf2000/ants)
- [alitto/pond](https://github.com/alitto/pond)
- [x/time/rate](https://pkg.go.dev/golang.org/x/time/rate)
- [sony/gobreaker](https://github.com/sony/gobreaker)
- [Netflix concurrency-limits](https://github.com/Netflix/concurrency-limits)
