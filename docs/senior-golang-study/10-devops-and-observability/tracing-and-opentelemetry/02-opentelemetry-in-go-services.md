# OpenTelemetry In Go Services

Эта заметка про то, как tracing выглядит именно в Go-сервисе.

## Содержание

- [Базовая схема](#базовая-схема)
- [Что обычно инициализируют](#что-обычно-инициализируют)
- [Middleware layer](#middleware-layer)
- [Service and repository spans](#service-and-repository-spans)
- [Что класть в span attributes](#что-класть-в-span-attributes)
- [Propagation через разные границы](#propagation-через-разные-границы)
- [Sampling](#sampling)
- [Common mistakes](#common-mistakes)
- [Как обычно выглядит минимальный useful набор spans](#как-обычно-выглядит-минимальный-useful-набор-spans)
- [Когда tracing особенно окупается](#когда-tracing-особенно-окупается)
- [Practical Rule](#practical-rule)

## Базовая схема

Обычно есть три части:

1. bootstrap tracer provider
2. middleware / transport extraction
3. spans в service/repository/integration слоях

Типичный flow:

```text
main/app startup
  -> init tracer provider
  -> set global propagator
  -> HTTP middleware creates/extracts span
  -> downstream code uses ctx
  -> exporter sends spans to OTLP endpoint
```

## Что обычно инициализируют

В bootstrap коде:
- service name
- environment
- exporter endpoint
- resource attributes
- tracer provider
- propagator

Ключевая идея:
- tracer provider должен жить на уровне app/runtime;
- shutdown должен делать flush/export close.

## Middleware layer

В HTTP middleware обычно делают:
- `Extract` trace context из incoming headers;
- `Start` root span для request;
- положить новый `ctx` в request;
- вызвать handler уже с этим context;
- завершить span после ответа.

Зачем это нужно:
- чтобы весь downstream код видел один и тот же trace lineage.

## Service and repository spans

Не надо делать span на каждую строчку кода.

Хороший practical уровень:
- transport/request span
- service/use-case span
- repository/storage span
- external integration span

Плохо:
- тысячи микроспанов без operational ценности.

Нормально:
- spans на meaningful boundaries.

## Что класть в span attributes

Нормально:
- `http.method`
- `http.route`
- `db.system`
- `messaging.system`
- `operation`
- stable domain ids, если они не чувствительные и low-risk

Осторожно:
- `link_id`, `order_id`, `booking_id`

Часто допустимо, если это internal opaque id и реально помогает расследованию.

Нельзя:
- secrets
- tokens
- passwords
- full SQL with secrets/user data
- raw request/response body по умолчанию
- sensitive PII

## Propagation через разные границы

### HTTP

Контекст уходит через headers.

Это самый стандартный сценарий.

### gRPC

Контекст уходит через metadata.

### Kafka / queues

Контекст обычно сериализуют в message headers.

Это очень важно:
- async pipeline без header propagation быстро теряет trace continuity.

### Database

До БД propagation обычно не уходит как trace context наружу;
вместо этого делают child span на DB operation.

## Sampling

Если трафика мало, локально можно трейсить почти все.

В production часто делают sampling, потому что:
- полный tracing дорогой;
- не все traces одинаково полезны.

Типичные подходы:
- trace everything in local/dev
- probability sampling in prod
- tail-based sampling в collector/backend

## Common mistakes

### 1. Создать span, но не прокинуть `ctx`

В этом случае span есть, но downstream не станет child.

### 2. Использовать `context.Background()` в середине flow

Это ломает:
- cancellation
- deadlines
- trace parentage

### 3. Instrument everything blindly

Это создает шум и overhead.

Нужно instrument:
- request boundaries
- storage boundaries
- messaging boundaries
- expensive / failure-prone steps

### 4. Смешивать logging fields и tracing fields без мысли

Хороший паттерн:
- лог хранит `trace_id`
- trace хранит ключевые attributes
- metrics показывают aggregate signal

Но не надо делать traces заменой логов.

## Как обычно выглядит минимальный useful набор spans

Для API:
- request span
- service/use-case span
- Postgres span
- Redis span
- Kafka publish span

Для worker:
- consume/fetch span
- process span
- storage write span
- DLQ publish span

## Когда tracing особенно окупается

- slow request, но logs не показывают bottleneck;
- timeout происходит не всегда;
- request проходит через несколько hop'ов;
- нужно понять, тормозит DB, cache или message bus;
- distributed system, где plain logs уже не дают общей картины.

## Practical Rule

В Go tracing надо мыслить так:
- `context.Context` — carrier всего trace lineage;
- middleware открывает root span;
- сервисы и репозитории делают child spans на meaningful boundaries;
- transport boundaries должны propagat'ить context дальше;
- exporter и backend уже вторичны по сравнению с правильным `ctx` flow.
