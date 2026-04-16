# Dockerfile Anatomy

Эта заметка нужна, чтобы понимать `Dockerfile` не как набор случайных команд, а как описание процесса сборки image и runtime-окружения контейнера.

## Содержание

- [Что такое Dockerfile](#что-такое-dockerfile)
- [Как мыслить про Dockerfile](#как-мыслить-про-dockerfile)
- [Основные директивы](#основные-директивы)
- [Layers и cache](#layers-и-cache)
- [Build context](#build-context)
- [`.dockerignore`](#dockerignore)
- [Multi-stage build](#multi-stage-build)
- [`scratch`, `distroless`, `alpine`, `debian`](#scratch-distroless-alpine-debian)
- [`CGO` и почему он меняет Dockerfile](#cgo-и-почему-он-меняет-dockerfile)
- [Что важно для Go-проектов](#что-важно-для-go-проектов)
- [Частые ошибки](#частые-ошибки)
- [Practical rule of thumb](#practical-rule-of-thumb)
- [Связанные темы](#связанные-темы)

## Что такое Dockerfile

`Dockerfile`:
- описывает, как собрать image;
- задает base image, файлы, команды сборки и runtime command;
- определяет, что именно попадет внутрь контейнера.

Практически это значит:
- `Dockerfile` отвечает не только за "как собрать Go-бинарь";
- он еще определяет размер image, скорость сборки, безопасность и удобство эксплуатации.

## Как мыслить про Dockerfile

Удобная mental model:

```text
base image -> build steps -> filesystem layers -> final runtime image
```

Для Go-проектов почти всегда полезно разделять:
- build stage;
- runtime stage.

## Основные директивы

### `FROM`

`FROM` задает базовый image.

Примеры:

```dockerfile
FROM golang:1.25 AS builder
FROM scratch
FROM gcr.io/distroless/static-debian12:nonroot
```

Что важно:
- первый `FROM` начинает stage;
- новый `FROM` начинает новый stage;
- multi-stage build позволяет собирать тяжелым образом, а запускать в легком.

Для Go это очень удобно:
- builder stage содержит toolchain;
- runtime stage содержит только бинарь и нужные runtime-файлы.

### `WORKDIR`

`WORKDIR` задает рабочую директорию внутри image.

Пример:

```dockerfile
WORKDIR /src
```

После этого:
- `COPY . .` копирует в `/src`;
- `RUN go build ...` выполняется из `/src`.

Практически:
- почти всегда лучше явно задавать `WORKDIR`, а не полагаться на root directory.

### `COPY`

`COPY` копирует файлы из build context внутрь image.

Примеры:

```dockerfile
COPY go.mod go.sum ./
COPY . .
COPY --from=builder /out/app /app
```

Что важно:
- `COPY` влияет на cache layers;
- порядок `COPY` сильно влияет на скорость пересборки;
- `COPY --from=builder` позволяет взять артефакт из предыдущего stage.

Для Go-проектов частый паттерн:

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build ...
```

Зачем так:
- если код меняется, но `go.mod`/`go.sum` нет, слой с `go mod download` можно переиспользовать из cache.

### `RUN`

`RUN` выполняет команду во время сборки image.

Примеры:

```dockerfile
RUN go mod download
RUN go build -o /out/app ./cmd/app
RUN apt-get update && apt-get install -y ca-certificates
```

Что важно:
- `RUN` создает новый layer;
- все, что ты делаешь через `RUN`, влияет на итоговый image;
- плохой порядок `RUN` может сделать image тяжелым и build медленным.

Практическое правило:
- все, что нужно только на этапе сборки, не должно утечь в final runtime image.

### `ENV`

`ENV` задает переменные окружения внутри image.

Пример:

```dockerfile
ENV APP_ENV=production
```

Но для Go-сервисов важно помнить:
- environment-specific runtime config обычно лучше задавать не в Dockerfile, а при запуске контейнера;
- не стоит зашивать секреты и deployment-specific адреса в образ.

### `ARG`

`ARG` задает build-time argument.

Пример:

```dockerfile
ARG GO_VERSION=1.25
```

Что важно:
- `ARG` доступен на этапе build;
- он не обязательно остается как runtime env внутри контейнера.

Полезно для:
- версий;
- feature flags сборки;
- параметров, влияющих только на build.

### `EXPOSE`

`EXPOSE` документирует порт, который слушает приложение внутри контейнера.

Пример:

```dockerfile
EXPOSE 8080
```

Важно:
- `EXPOSE` сам по себе не публикует порт наружу;
- публикация порта происходит через `docker run -p` или `docker compose ports`.

То есть:
- `EXPOSE` это metadata и подсказка;
- не замена runtime network config.

### `CMD`

`CMD` задает команду по умолчанию.

Пример:

```dockerfile
CMD ["air", "-c", ".air.toml"]
```

Обычно:
- удобно для dev image;
- легко переопределяется при запуске.

### `ENTRYPOINT`

`ENTRYPOINT` задает основной исполняемый файл контейнера.

Пример:

```dockerfile
ENTRYPOINT ["/app"]
```

Для production Go-сервисов это очень частый вариант.

### `CMD` vs `ENTRYPOINT`

Полезная practical разница:

`ENTRYPOINT`:
- "что именно является приложением контейнера"

`CMD`:
- "какие аргументы или дефолтная команда идут к нему"

Типичный production pattern:

```dockerfile
ENTRYPOINT ["/app"]
```

Типичный dev pattern:

```dockerfile
CMD ["air", "-c", ".air.toml"]
```

## Layers и cache

Это одна из самых важных тем.

Каждая инструкция вроде:
- `RUN`
- `COPY`
- `ADD`

создает слой.

Если ранний слой изменился:
- все следующие слои обычно пересобираются.

Поэтому порядок инструкций важен.

Для Go хороший cache-friendly паттерн:

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build ...
```

Если сначала сделать:

```dockerfile
COPY . .
RUN go mod download
```

то любое изменение в исходниках будет ломать cache для зависимостей.

## Build context

Когда ты делаешь:

```bash
docker build -f Dockerfile .
```

последняя точка это build context.

Все `COPY` берут файлы только из этого контекста.

Это важно, потому что:
- случайно можно отправить в build огромный context;
- лишние файлы замедляют build;
- секреты и мусор могут утечь внутрь image.

## `.dockerignore`

Для Go-проекта `.dockerignore` почти так же важен, как `.gitignore`.

Обычно туда стоит класть:
- `.git`
- `tmp`
- `bin`
- `dist`
- `coverage`
- локальные IDE-файлы
- большие артефакты и кэши

Зачем:
- уменьшить build context;
- ускорить сборку;
- не тащить мусор в image.

## Multi-stage build

Это стандартный хороший паттерн для Go.

Идея:
- в первом stage собрать бинарь;
- во втором stage оставить только runtime.

Пример:

```dockerfile
FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/app

FROM scratch
COPY --from=builder /out/app /app
ENTRYPOINT ["/app"]
```

Плюсы:
- smaller image;
- меньше attack surface;
- нет toolchain в runtime;
- быстрее deploy и pull.

## `scratch`, `distroless`, `alpine`, `debian`

### `scratch`

Подходит, когда:
- бинарь статический;
- нужен минимальный runtime image.

Минусы:
- неудобно дебажить;
- нет shell;
- нужно помнить про сертификаты и другие runtime-файлы.

### `distroless`

Подходит, когда:
- нужен production-friendly minimal runtime;
- хочется non-root и более практичный вариант, чем `scratch`.

### `alpine`

Плюсы:
- компактный;
- привычный для многих.

Минусы:
- `musl` иногда дает сюрпризы;
- не всегда лучший выбор для `CGO`.

### `debian`/`ubuntu`-based runtime

Подходит, когда:
- нужен `CGO`;
- нужны shared libraries;
- нужен shell/debug tooling;
- runtime dependencies сложнее обычного статического бинаря.

## `CGO` и почему он меняет Dockerfile

Если `CGO_ENABLED=0`:
- часто можно жить со `scratch` или `distroless/static`.

Если `CGO` нужен:
- бинарь может зависеть от shared libraries;
- runtime image должен содержать нужные системные библиотеки;
- `scratch` часто уже не подходит.

Поэтому `CGO`-сервисы часто живут на:
- `debian-slim`;
- иногда `alpine`, если совместимость проверена;
- других runtime images с нужными libs.

## Что важно для Go-проектов

### 1. Separate build and runtime

Builder image:
- heavy;
- содержит toolchain.

Runtime image:
- легкий;
- содержит только нужное для запуска.

### 2. Non-root runtime

Если возможно, сервис лучше запускать не от root.

Это особенно удобно в:
- `distroless:nonroot`;
- кастомных runtime images с `USER`.

### 3. Logs в stdout

Dockerfile не должен подталкивать сервис к file-based logging внутри контейнера.

Для Go-сервиса почти всегда лучше:
- писать в stdout/stderr;
- собирать логи на уровне runtime/orchestrator.

### 4. Dev и prod Dockerfile часто разные

Это нормально.

Dev image:
- shell;
- hot reload;
- bind mount;
- удобство.

Prod image:
- минимальный runtime;
- безопасность;
- быстрый startup;
- меньше мусора.

## Частые ошибки

- копировать весь репозиторий слишком рано и ломать cache;
- тащить toolchain в runtime image;
- хардкодить runtime config и секреты в Dockerfile;
- использовать `latest` без контроля версии;
- забывать про `.dockerignore`;
- выбирать `scratch`, когда нужен `CGO`;
- использовать один и тот же Dockerfile для dev и prod, хотя цели разные.

## Practical rule of thumb

Для stateless Go API без `CGO`:
- чаще всего хороший default это multi-stage + `distroless`.

Если нужен ultra-minimal image:
- смотри в сторону `scratch`.

Если это local dev:
- отдельный Dockerfile с hot reload обычно лучше.

Если есть `CGO`:
- почти всегда сначала думай о runtime dependencies, а потом уже о минимальном размере.

## Связанные темы

- [Dockerfiles For Go Projects](./dockerfiles-for-go-projects.md)
- [Multi-stage Scratch Example](./Dockerfile.scratch.example)
- [Distroless Example](./Dockerfile.distroless.example)
- [Dev Hot Reload Example](./Dockerfile.dev-hot-reload.example)
- [CGO Runtime Example](./Dockerfile.cgo-runtime.example)
