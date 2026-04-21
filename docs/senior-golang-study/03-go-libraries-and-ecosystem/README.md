# Go Libraries And Ecosystem

Сравнения библиотек по сценариям: trade-offs, когда уместны, какие проблемы создают через полгода.

## Материалы

- [shopspring/decimal](./shopspring-decimal.md) — точная десятичная арифметика; почему float64 нельзя для денег
- [google/uuid](./google-uuid.md) — UUID v4 vs v7; почему v7 лучше для primary keys
- [samber/lo](./samber-lo.md) — generics-утилиты для коллекций: Map, Filter, GroupBy, Chunk
- [pkg/errors](./pkg-errors.md) — ошибки со стектрейсом; совместимость с errors.Is

## Категории (материалы готовятся)

- **HTTP routers:** `chi`, `gin`, `echo`, stdlib router
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
