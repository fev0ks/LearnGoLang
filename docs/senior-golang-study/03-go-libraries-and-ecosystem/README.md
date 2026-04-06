# Go Libraries And Ecosystem

Сюда лучше складывать сравнения библиотек по сценариям, а не просто списки.

Категории:
- HTTP routers: `chi`, `gin`, `echo`, stdlib router;
- config: `envconfig`, `viper`, manual parsing;
- logging: `slog`, `zap`, `zerolog`;
- database access: `sqlx`, `pgx`, `gorm`, `bun`, `ent`;
- validation: `go-playground/validator`;
- DI/wiring: `google/wire`, `uber/fx`, manual composition;
- testing: `testify`, `go-cmp`, `gomock`, `testcontainers-go`;
- messaging: official clients for Kafka, RabbitMQ, NATS, Redis streams;
- observability: OpenTelemetry SDKs and exporters.

Для каждой библиотеки фиксируй:
- сильные стороны;
- слабые стороны;
- где уместна;
- почему выбрал бы ее в production;
- какие проблемы она создает команде через полгода.

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

## Вопросы

- почему ты выбрал бы `chi`, `gin` или stdlib router для нового сервиса;
- когда `pgx` лучше, чем `database/sql`, а когда разница не окупается;
- в каком случае ORM ускоряет команду, а в каком скрывает слишком много;
- почему `zap` или `zerolog` могут быть лучше `slog`, и наоборот;
- когда DI-фреймворк оправдан, а когда manual wiring проще и надежнее;
- какую библиотеку ты бы точно не стал тянуть в core path и почему.
