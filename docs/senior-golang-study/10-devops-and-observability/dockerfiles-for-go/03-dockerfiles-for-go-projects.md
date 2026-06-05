# Dockerfiles For Go Projects

## Главная идея

У Go-проектов нет одного "правильного" Dockerfile. Выбор зависит от:
- нужен ли `CGO`;
- нужен ли shell/debug tools;
- production это или local dev;
- нужен ли минимальный образ или удобство сопровождения.

## Основные паттерны

### 1. Multi-stage + `scratch`

Подходит, когда:
- бинарь статически собран;
- нужен очень маленький runtime image;
- не нужен shell и debug tooling.

Смотри пример:
- [Multi-stage Scratch Example](./Dockerfile.scratch.example)

### 2. Multi-stage + `distroless`

Подходит, когда:
- нужен минимальный production image;
- хочется безопаснее и практичнее, чем `scratch`;
- нужны CA certs и нормальный non-root runtime.

Смотри пример:
- [Distroless Example](./Dockerfile.distroless.example)

### 3. Dev image с hot reload

Подходит, когда:
- локальная разработка важнее минимального размера image;
- нужен `air`, shell и bind mount с кодом.

Смотри пример:
- [Dev Hot Reload Example](./Dockerfile.dev-hot-reload.example)

### 4. `CGO`-сервис с runtime layer

Подходит, когда:
- используются SQLite, librdkafka, image libs, DNS/system deps или другой `CGO`;
- статический бинарь не подходит;
- runtime должен содержать нужные shared libraries.

Смотри пример:
- [CGO Runtime Example](./Dockerfile.cgo-runtime.example)

## Что важно помнить

`scratch`:
- минимально;
- очень мало surface area;
- неудобно дебажить.

`distroless`:
- production-friendly компромисс;
- меньше мусора, чем обычный Linux runtime;
- удобнее, чем `scratch`.

`alpine`:
- компактный;
- но с `musl`, что не всегда удобно для `CGO` и некоторых зависимостей.

`debian`/`ubuntu`-based runtime:
- тяжелее;
- но часто практичнее при `CGO`, shell access и сложных runtime dependencies.

## Practical rule

Если нет специальных причин:
- production stateless Go service без `CGO` часто хорошо живет на `distroless`;
- ultra-minimal вариант это `scratch`;
- local dev почти всегда требует отдельный dev-oriented Dockerfile;
- `CGO` почти всегда толкает в сторону non-scratch runtime.
