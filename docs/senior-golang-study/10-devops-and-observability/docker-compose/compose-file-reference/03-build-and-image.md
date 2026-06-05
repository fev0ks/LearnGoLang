# Build And Image

`image` и `build` отвечают за то, откуда сервис получает свой контейнерный образ.

## Содержание

- [`image`](#image)
- [`build`](#build)
- [Часто используемые build keys](#часто-используемые-build-keys)
- [Когда задают и `image`, и `build`](#когда-задают-и-image-и-build)
- [`pull_policy`](#pull_policy)
- [Когда что использовать](#когда-что-использовать)
- [Practical rules](#practical-rules)

## `image`

`image` говорит Compose взять уже готовый образ.

Примеры:

```yaml
image: postgres:16-alpine
image: redis:7
image: ghcr.io/acme/shortener-api:1.3.0
image: ghcr.io/acme/shortener-api@sha256:abcdef...
```

Практически:
- внешние зависимости почти всегда описываются через `image`;
- свои сервисы тоже можно запускать через `image`, если образ уже собран CI или registry.

## `build`

`build` говорит Compose собрать образ из исходников.

### Короткий синтаксис

```yaml
build: .
```

Допустимые варианты:
- относительный путь к build context;
- абсолютный путь;
- Git URL.

Git context пример:

```yaml
build: https://github.com/mycompany/example.git#main:subdirectory
```

### Полный синтаксис

```yaml
build:
  context: .
  dockerfile: Dockerfile.dev
  target: runtime
  args:
    GO_VERSION: "1.25"
```

## Часто используемые build keys

### `context`

Каталог или Git context, из которого идет сборка.

Чаще всего:
- `.`
- `./cmd/api`
- Git URL

### `dockerfile`

Путь к Dockerfile относительно `context`.

Пример:

```yaml
dockerfile: Dockerfile.dev
```

### `dockerfile_inline`

Позволяет описать Dockerfile прямо внутри compose-файла.

Практически:
- для реальных Go-проектов используется редко;
- удобен для маленьких demo и self-contained примеров.

### `target`

Выбирает stage из multi-stage Dockerfile.

Пример:

```yaml
target: runtime
```

Полезно:
- для dev/runtime split;
- когда `api` и `worker` используют один Dockerfile, но разные target.

### `args`

Передает Dockerfile `ARG` значения.

Варианты:
- mapping
- list

Пример:

```yaml
args:
  GO_VERSION: "1.25"
  APP_VERSION: "dev"
```

### `additional_contexts`

Дополнительные named contexts для builder.

Полезно:
- в более сложных buildx-сценариях;
- когда build использует несколько контекстов.

### `ssh`

Дает builder доступ к SSH authentication.

Полезно:
- для приватных Git dependencies на этапе build.

### `secrets`

Дает build-time доступ к секретам из top-level `secrets`.

Полезно:
- для `NPM_TOKEN`, приватных registry credentials и подобных сценариев;
- лучше, чем хардкодить секрет в Dockerfile.

### `cache_from`, `cache_to`

Настраивают импорт и экспорт build cache.

Полезно:
- для CI;
- для ускорения частых сборок.

### `no_cache`

Отключает использование cache.

Допустимые значения:
- `true`
- `false`

### `pull`

Говорит builder тянуть более свежие base images перед сборкой.

Допустимые значения:
- `true`
- `false`

### `platforms`

Целевые платформы для build.

Примеры:
- `linux/amd64`
- `linux/arm64`

## Когда задают и `image`, и `build`

Так делать можно.

Частый сценарий:

```yaml
services:
  api:
    image: ghcr.io/acme/shortener-api:dev
    build:
      context: .
```

Зачем это бывает нужно:
- Compose знает, какой tag дать собранному образу;
- этот же образ потом удобно пушить или переиспользовать.

Поведение при одновременном использовании `image` и `build` зависит от `pull_policy`.

## `pull_policy`

`pull_policy` управляет тем, как Compose решает, когда тянуть образ из registry.

Актуальные значения:

`always`:
- всегда тянуть образ.

`never`:
- не тянуть образ вообще;
- использовать только локальный cache.

`missing`:
- тянуть только если образа нет локально;
- это default для обычного `image` сценария.

`if_not_present`:
- backward-compatible alias для `missing`.

`build`:
- вместо pull форсировать build.

`daily`:
- проверять обновления раз в 24 часа.

`weekly`:
- проверять обновления раз в 7 дней.

`every_<duration>`:
- проверять обновления через указанный интервал;
- примеры: `every_12h`, `every_3d`, `every_1w2d`.

## Когда что использовать

`image`:
- Postgres, Redis, Grafana, Prometheus;
- любой dependency из публичного или внутреннего registry.

`build`:
- твои Go-сервисы в local dev;
- integration stack, который собирается прямо из репозитория.

`image + build`:
- когда нужен и local build, и стабильный tag имени образа.

## Practical rules

- для local Go-проекта `build` у `api` и `worker`, `image` у инфраструктурных сервисов это нормальный default;
- не завязывайся на `latest`, если хочешь воспроизводимость;
- если важно стабильное поведение на ARM Mac, явно проверяй `platform`;
- если в build нужны секреты, используй build `secrets`, а не `ARG` для токенов.
