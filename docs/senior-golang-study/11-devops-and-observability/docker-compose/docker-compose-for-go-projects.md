# Docker Compose For Go Projects

## Зачем нужен Docker Compose

`docker compose` полезен, когда надо быстро поднять локальный стек из нескольких сервисов:
- Go API;
- Postgres;
- Redis;
- Kafka/Redpanda;
- миграции;
- Prometheus/Grafana/OTel collector.

Это особенно удобно для:
- local development;
- integration tests;
- ручной отладки сквозных сценариев;
- onboarding новых разработчиков.

## Когда compose хорош

- один репозиторий или небольшой локальный стек;
- нет задачи полноценной production orchestration;
- нужно быстро поднимать зависимости одной командой.

## Когда compose уже мало

- нужны rollout/rollback стратегии;
- нужен self-healing;
- нужен node scheduling;
- важны multi-node deployment и service mesh-like поведение;
- нужна production-grade orchestration.

В этот момент уже смотришь в сторону `Kubernetes`.

## На что смотреть в compose-файле

### `depends_on`

Полезно, но:
- не заменяет нормальный retry в приложении;
- не гарантирует, что dependency действительно готова к работе.

### `healthcheck`

Очень полезен:
- позволяет дождаться readiness сервиса;
- удобен для `postgres`, `redis`, `api`, `otel-collector`.

### `profiles`

Полезны, когда:
- часть инфраструктуры нужна не всегда;
- хочется optional observability/debug stack.

### `volumes`

Нужны для:
- persistent data;
- bind mounts с кодом в dev-режиме;
- config files.

### `networks`

Обычно достаточно одной общей сети для local stack.

## Practical rule

Compose-файл должен помогать локальной разработке, а не превращаться в слабую копию production orchestration.

То есть:
- только нужные сервисы;
- понятные имена;
- healthchecks;
- минимальный, но полезный набор volumes;
- optional profiles для тяжелых сервисов.

## Что смотреть в примере

В [Complex Compose Example](./compose-go-stack.example.yaml):
- `api` и `worker` собираются из одного репозитория;
- `migrator` живет отдельным одноразовым сервисом;
- `postgres`, `redis`, `redpanda` поднимаются как зависимости;
- observability вынесена в profile `observability`;
- healthchecks помогают не стартовать слишком рано.
